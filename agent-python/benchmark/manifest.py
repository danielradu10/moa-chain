"""
Immutable run manifest created before any inference begins.

The manifest captures everything needed to:
  - reproduce the run (config_hash, seed, all inference params)
  - verify resume compatibility (config_hash + model_digest)
  - audit results (git commit, dataset hash, prompt hash, Ollama version, model digest)

Saved to <output_dir>/manifest.json before the first inference call.
"""
from __future__ import annotations

import hashlib
import json
import logging
import random
import subprocess
import uuid
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from benchmark.fixtures import (
    DATASET_VERSION,
    Fixture,
    compute_dataset_hash,
)
from benchmark.prompt import (
    ANSWER_JUDGE_PROMPT_HASH,
    ANSWER_JUDGE_PROMPT_VERSION,
)

logger = logging.getLogger(__name__)

MANIFEST_FILENAME = "manifest.json"


@dataclass
class RunManifest:
    # ── identity ───────────────────────────────────────────────────────────
    run_id: str                    # UUID4, unique per run
    config_hash: str               # SHA-256 of inference params + dataset + prompt
    created_at: str                # ISO-8601 UTC, set before first inference

    # ── provenance ─────────────────────────────────────────────────────────
    git_commit: str | None         # HEAD commit hash, None if not in a git repo
    git_dirty: bool | None         # True if working tree has uncommitted changes

    # ── dataset ────────────────────────────────────────────────────────────
    dataset_version: str           # DATASET_VERSION constant
    dataset_hash: str              # SHA-256 of all fixture content hashes
    fixture_count: int

    # ── prompt ─────────────────────────────────────────────────────────────
    prompt_version: str            # ANSWER_JUDGE_PROMPT_VERSION
    prompt_hash: str               # SHA-256 of ANSWER_JUDGE_SYSTEM_PROMPT
    response_schema_hash: str
    fixture_hashes: list[str]
    trial_orders: dict[str, list[str]]

    # ── Ollama ─────────────────────────────────────────────────────────────
    ollama_version: str | None

    # ── model ──────────────────────────────────────────────────────────────
    model_name: str                # as passed by the caller
    model_digest: str | None       # from /api/show, None if unavailable
    model_parameter_size: str | None
    model_quantization: str | None

    # ── inference parameters ───────────────────────────────────────────────
    seed: int
    temperature: float
    num_ctx: int
    num_predict: int
    think: bool
    keep_alive: str
    timeout_s: float
    max_retries: int
    capability_status: dict[str, bool]

    # ── run parameters ─────────────────────────────────────────────────────
    trials: int
    groups: list[str] | None       # None = all groups

    # ── lifecycle timing (filled in after model loading) ───────────────────
    cold_load_s: float | None = None
    warmup_latency_s: float | None = None


def _git_info() -> tuple[str | None, bool | None]:
    """Return (commit_hash, is_dirty). Both None if not in a git repo."""
    try:
        commit = subprocess.check_output(
            ["git", "rev-parse", "HEAD"],
            stderr=subprocess.DEVNULL,
            text=True,
        ).strip()
        dirty_out = subprocess.check_output(
            ["git", "status", "--porcelain"],
            stderr=subprocess.DEVNULL,
            text=True,
        ).strip()
        return commit, bool(dirty_out)
    except Exception:
        return None, None


def compute_config_hash(
    model_name: str,
    seed: int,
    temperature: float,
    num_ctx: int,
    num_predict: int,
    think: bool,
    prompt_version: str,
    prompt_hash: str,
    dataset_version: str,
    dataset_hash: str,
    trials: int,
    groups: list[str] | None,
    keep_alive: str = "30m",
    timeout_s: float = 120.0,
    max_retries: int = 1,
    response_schema_hash: str = "",
) -> str:
    """Stable SHA-256 of all parameters that determine run compatibility.

    Two runs are compatible for resume if and only if their config_hash is identical
    AND their model_digest is identical (digest checked separately from the manifest).
    """
    payload = {
        "model_name": model_name,
        "seed": seed,
        "temperature": temperature,
        "num_ctx": num_ctx,
        "num_predict": num_predict,
        "think": think,
        "prompt_version": prompt_version,
        "prompt_hash": prompt_hash,
        "dataset_version": dataset_version,
        "dataset_hash": dataset_hash,
        "trials": trials,
        "groups": sorted(groups) if groups is not None else None,
        "keep_alive": keep_alive,
        "timeout_s": timeout_s,
        "max_retries": max_retries,
        "response_schema_hash": response_schema_hash,
    }
    serialized = json.dumps(payload, sort_keys=True, ensure_ascii=True)
    return hashlib.sha256(serialized.encode()).hexdigest()


def create_manifest(
    model: str,
    seed: int,
    temperature: float,
    num_ctx: int,
    num_predict: int,
    think: bool,
    keep_alive: str,
    timeout_s: float,
    trials: int,
    groups: list[str] | None,
    fixtures: list[Fixture],
    ollama_version: str | None,
    model_info: dict[str, Any],
    max_retries: int = 1,
    capability_status: dict[str, bool] | None = None,
) -> RunManifest:
    """Build the RunManifest from all available information.

    `model_info` is the dict returned by client.show_model(model) — may be {}.
    """
    ds_hash = compute_dataset_hash(fixtures)
    groups_for_hash = sorted(groups) if groups is not None else None

    from benchmark.prompt import RESPONSE_JSON_SCHEMA
    schema_hash = hashlib.sha256(
        json.dumps(RESPONSE_JSON_SCHEMA, sort_keys=True).encode()
    ).hexdigest()
    cfg_hash = compute_config_hash(
        model_name=model,
        seed=seed,
        temperature=temperature,
        num_ctx=num_ctx,
        num_predict=num_predict,
        think=think,
        prompt_version=ANSWER_JUDGE_PROMPT_VERSION,
        prompt_hash=ANSWER_JUDGE_PROMPT_HASH,
        dataset_version=DATASET_VERSION,
        dataset_hash=ds_hash,
        trials=trials,
        groups=groups_for_hash,
        keep_alive=keep_alive,
        timeout_s=timeout_s,
        max_retries=max_retries,
        response_schema_hash=schema_hash,
    )

    git_commit, git_dirty = _git_info()

    # Model metadata from /api/show
    # Ollama returns model details in model_info["details"]
    details = model_info.get("details", {})
    param_size = details.get("parameter_size")
    quantization = details.get("quantization_level")
    fixture_hashes = sorted(f.content_hash() for f in fixtures)
    trial_orders: dict[str, list[str]] = {}
    for trial in range(1, trials + 1):
        ordered = list(fixtures)
        random.Random(seed * 10000 + trial).shuffle(ordered)
        trial_orders[str(trial)] = [f.content_hash() for f in ordered]

    return RunManifest(
        run_id=str(uuid.uuid4()),
        config_hash=cfg_hash,
        created_at=datetime.now(timezone.utc).isoformat(),
        git_commit=git_commit,
        git_dirty=git_dirty,
        dataset_version=DATASET_VERSION,
        dataset_hash=ds_hash,
        fixture_count=len(fixtures),
        prompt_version=ANSWER_JUDGE_PROMPT_VERSION,
        prompt_hash=ANSWER_JUDGE_PROMPT_HASH,
        response_schema_hash=schema_hash,
        fixture_hashes=fixture_hashes,
        trial_orders=trial_orders,
        ollama_version=ollama_version,
        model_name=model,
        model_digest=model_info.get("digest"),
        model_parameter_size=param_size,
        model_quantization=quantization,
        seed=seed,
        temperature=temperature,
        num_ctx=num_ctx,
        num_predict=num_predict,
        think=think,
        keep_alive=keep_alive,
        timeout_s=timeout_s,
        max_retries=max_retries,
        capability_status=capability_status or {},
        trials=trials,
        groups=groups,
    )


def save_manifest(manifest: RunManifest, output_dir: Path) -> Path:
    path = output_dir / MANIFEST_FILENAME
    output_dir.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(asdict(manifest), indent=2))
    return path


def load_manifest(output_dir: Path) -> RunManifest | None:
    path = output_dir / MANIFEST_FILENAME
    if not path.exists():
        return None
    try:
        data = json.loads(path.read_text())
        return RunManifest(**data)
    except Exception as exc:
        raise RuntimeError(f"Corrupt or incompatible manifest {path}: {exc}") from exc


def check_resume_compatible(
    existing: RunManifest,
    new_config_hash: str,
    new_model_digest: str | None,
) -> list[str]:
    """Return a list of incompatibility reasons. Empty = safe to resume."""
    reasons: list[str] = []
    if existing.config_hash != new_config_hash:
        reasons.append(
            f"config_hash changed: stored={existing.config_hash[:12]}... "
            f"current={new_config_hash[:12]}..."
        )
    if existing.model_digest is None or new_model_digest is None:
        reasons.append("model_digest unavailable; resume requires immutable model identity")
    elif existing.model_digest != new_model_digest:
        reasons.append(
            f"model_digest changed: stored={existing.model_digest[:16]}... "
            f"current={new_model_digest[:16]}..."
        )
    return reasons
