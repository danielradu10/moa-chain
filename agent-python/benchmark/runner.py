"""
Benchmark runner: orchestrates model lifecycle, fixture shuffling, and execution.

Finding 3  — attempt persistence: every inference attempt is stored in `attempts` list;
             a successful retry does not overwrite prior attempt's raw output or latency.
Finding 5  — model lifecycle: load → warm-up (excluded) → benchmark → unload.
Finding 6  — deterministic shuffle: fixtures shuffled per trial using seed × 10000 + trial,
             same order applied for every model so cross-model comparison is valid.
Finding 8  — config-aware resume: manifest config_hash + model_digest must match;
             mismatches cause an error unless --force-restart is used.
"""
from __future__ import annotations

import json
import logging
import random
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from benchmark.client import BenchmarkConfig, OllamaBenchmarkClient
from benchmark.fixtures import Fixture
from benchmark.manifest import (
    MANIFEST_FILENAME,
    RunManifest,
    check_resume_compatible,
    create_manifest,
    load_manifest,
    save_manifest,
)
from benchmark.prompt import (
    ANSWER_JUDGE_SYSTEM_PROMPT,
    ANSWER_JUDGE_PROMPT_VERSION,
    build_user_prompt,
    parse_judge_response,
)

logger = logging.getLogger(__name__)

JSONL_FILENAME = "raw_results.jsonl"
DEFAULT_SEED = 42
REQUIRED_RESULT_FIELDS = {
    "config_hash", "model", "fixture_hash", "candidate_id", "trial",
    "group", "scenario_id", "tx_id", "perspective", "expected",
    "is_adversarial", "predicted", "attempts", "attempt_count",
}


def validate_persisted_record(record: Any, path: Path, line_number: int) -> dict[str, Any]:
    if not isinstance(record, dict):
        raise RuntimeError(f"Corrupt {path} at line {line_number}: record is not an object")
    missing = REQUIRED_RESULT_FIELDS - set(record)
    if missing:
        raise RuntimeError(
            f"Corrupt {path} at line {line_number}: missing fields {sorted(missing)}"
        )
    if not isinstance(record.get("attempts"), list):
        raise RuntimeError(f"Corrupt {path} at line {line_number}: attempts is not an array")
    return record


@dataclass
class AttemptDetail:
    """Immutable record of one inference attempt."""
    attempt_number: int        # 1-indexed
    raw_content: str | None    # verbatim model output before any parsing
    latency_s: float
    timed_out: bool
    http_error: bool
    parse_error: bool
    error_message: str | None
    ollama_eval_duration_ns: int | None = None


@dataclass
class PredictionRecord:
    # ── fixture identity ──────────────────────────────────────────────────
    model: str
    group: str
    scenario_id: str
    tx_id: str
    candidate_id: str
    perspective: str
    prompt: str
    answer: str
    expected: str
    assumption_basis: str      # "canonical" or "benchmark-assumption"

    # ── outcome ───────────────────────────────────────────────────────────
    predicted: str | None      # None when all attempts failed
    is_adversarial: bool
    is_correct: bool           # predicted == expected (False when predicted is None)

    # ── attempt history (Finding 3) ───────────────────────────────────────
    attempts: list[dict]       # serialized AttemptDetail, preserves full history
    attempt_count: int         # = len(attempts)

    # ── final outcome summary ──────────────────────────────────────────────
    final_latency_s: float     # latency of the last (successful or final failed) attempt
    total_latency_s: float     # sum of all attempt latencies
    timed_out: bool
    http_error: bool
    parse_error: bool
    error_message: str | None

    # ── run metadata ───────────────────────────────────────────────────────
    config_hash: str
    fixture_hash: str
    seed: int
    trial: int
    trial_seed: int
    timestamp: str             # ISO-8601 UTC


def _completed_key(r: dict[str, Any]) -> tuple:
    """Deduplication key including configuration identity."""
    return (
        r.get("config_hash", ""),
        r.get("model", ""),
        r.get("fixture_hash", ""),
        r.get("candidate_id", "candidate-1"),
        r.get("trial", 1),
    )


def load_completed_keys(output_dir: Path) -> set[tuple]:
    """Read existing JSONL and return the set of already-completed run keys."""
    path = output_dir / JSONL_FILENAME
    if not path.exists():
        return set()
    completed: set[tuple] = set()
    with path.open() as f:
        for line_number, line in enumerate(f, start=1):
            line = line.strip()
            if not line:
                continue
            try:
                r = validate_persisted_record(json.loads(line), path, line_number)
                completed.add(_completed_key(r))
            except json.JSONDecodeError as exc:
                raise RuntimeError(
                    f"Corrupt {path} at line {line_number}: {exc}"
                ) from exc
    return completed


def _append_record(record: PredictionRecord, output_dir: Path) -> None:
    path = output_dir / JSONL_FILENAME
    d = asdict(record)
    with path.open("a") as f:
        f.write(json.dumps(d) + "\n")


def _shuffle_fixtures(fixtures: list[Fixture], trial_seed: int) -> list[Fixture]:
    """Return a deterministically shuffled copy of fixtures for this trial."""
    copy = list(fixtures)
    random.Random(trial_seed).shuffle(copy)
    return copy


def _run_single(
    client: OllamaBenchmarkClient,
    model: str,
    fixture: Fixture,
    trial: int,
    trial_seed: int,
    config_hash: str,
    seed: int = DEFAULT_SEED,
    max_retries: int = 1,
) -> PredictionRecord:
    """Execute one (fixture, trial) unit. All attempts are preserved."""
    user_prompt = build_user_prompt(fixture.tx_id, fixture.prompt, fixture.answer)
    candidate_id = "candidate-1"

    attempts: list[AttemptDetail] = []
    predicted: str | None = None
    max_attempts = max_retries + 1  # attempt 1..max_retries+1

    for attempt_num in range(1, max_attempts + 1):
        result = client.call(model, ANSWER_JUDGE_SYSTEM_PROMPT, user_prompt)

        if result.timed_out:
            attempts.append(AttemptDetail(
                attempt_number=attempt_num,
                raw_content=result.content,
                latency_s=result.latency_s,
                timed_out=True,
                http_error=False,
                parse_error=False,
                error_message=result.error_message,
                ollama_eval_duration_ns=result.ollama_eval_duration_ns,
            ))
            logger.warning(
                "  [trial=%d attempt=%d] timeout: %s", trial, attempt_num, result.error_message
            )
            break  # no retry on timeout

        if result.http_error:
            attempts.append(AttemptDetail(
                attempt_number=attempt_num,
                raw_content=result.content,
                latency_s=result.latency_s,
                timed_out=False,
                http_error=True,
                parse_error=False,
                error_message=result.error_message,
                ollama_eval_duration_ns=result.ollama_eval_duration_ns,
            ))
            logger.warning(
                "  [trial=%d attempt=%d] http_error: %s", trial, attempt_num, result.error_message
            )
            break  # no retry on http error

        try:
            predicted = parse_judge_response(result.content, candidate_id)
            attempts.append(AttemptDetail(
                attempt_number=attempt_num,
                raw_content=result.content,
                latency_s=result.latency_s,
                timed_out=False,
                http_error=False,
                parse_error=False,
                error_message=None,
                ollama_eval_duration_ns=result.ollama_eval_duration_ns,
            ))
            break  # success

        except ValueError as exc:
            attempts.append(AttemptDetail(
                attempt_number=attempt_num,
                raw_content=result.content,
                latency_s=result.latency_s,
                timed_out=False,
                http_error=False,
                parse_error=True,
                error_message=str(exc),
                ollama_eval_duration_ns=result.ollama_eval_duration_ns,
            ))
            logger.warning(
                "  [trial=%d attempt=%d] parse_error: %s | raw: %.80s",
                trial, attempt_num, exc, result.content,
            )
            if attempt_num >= max_attempts:
                break  # exhausted retries — predicted stays None

    # Final outcome summary from last attempt
    last = attempts[-1]
    total_lat = sum(a.latency_s for a in attempts)

    return PredictionRecord(
        model=model,
        group=fixture.group,
        scenario_id=fixture.scenario_id,
        tx_id=fixture.tx_id,
        candidate_id=candidate_id,
        perspective=fixture.perspective,
        prompt=fixture.prompt,
        answer=fixture.answer,
        expected=fixture.expected,
        assumption_basis=fixture.assumption_basis,
        predicted=predicted,
        is_adversarial=fixture.is_adversarial,
        is_correct=(predicted == fixture.expected),
        attempts=[asdict(a) for a in attempts],
        attempt_count=len(attempts),
        final_latency_s=last.latency_s,
        total_latency_s=total_lat,
        timed_out=last.timed_out,
        http_error=last.http_error,
        parse_error=last.parse_error,
        error_message=last.error_message,
        config_hash=config_hash,
        fixture_hash=fixture.content_hash(),
        seed=seed,
        trial=trial,
        trial_seed=trial_seed,
        timestamp=datetime.now(timezone.utc).isoformat(),
    )


def run_model(
    model: str,
    fixtures: list[Fixture],
    config: BenchmarkConfig,
    output_dir: Path,
    trials: int = 1,
    max_retries: int = 1,
    force_restart: bool = False,
    groups: list[str] | None = None,
) -> list[PredictionRecord]:
    """Run all fixtures × trials for a single model.

    Model lifecycle:
      1. Connect and introspect Ollama version + model digest.
      2. Create and save the run manifest before any inference.
      3. Check resume compatibility; abort if config changed (unless force_restart).
      4. Load model, record cold-load duration.
      5. Execute one warm-up call (not persisted).
      6. Run benchmark (shuffled per-trial, skipping completed keys).
      7. Unload model.
    """
    output_dir.mkdir(parents=True, exist_ok=True)
    records: list[PredictionRecord] = []

    with OllamaBenchmarkClient(config) as client:
        loaded = False
        try:
            if not client.model_available(model):
                raise RuntimeError(f"Model '{model}' not found in Ollama at {config.base_url}")

            ollama_version = client.get_version()
            capabilities = client.verify_capabilities()
            unsupported = sorted(name for name, supported in capabilities.items() if not supported)
            if unsupported:
                raise RuntimeError(
                    f"Ollama {ollama_version or 'unknown'} lacks required capabilities: "
                    + ", ".join(unsupported)
                )
            model_info = client.show_model(model)
            model_digest = model_info.get("digest")
            if not model_digest:
                raise RuntimeError(f"Cannot benchmark {model}: immutable model digest is unavailable")

            proposed = create_manifest(
                model=model, seed=config.seed, temperature=config.temperature,
                num_ctx=config.num_ctx, num_predict=config.num_predict,
                think=config.think, keep_alive=config.keep_alive,
                timeout_s=config.timeout_s, trials=trials, groups=groups,
                fixtures=fixtures, ollama_version=ollama_version,
                model_info=model_info, max_retries=max_retries,
                capability_status=capabilities,
            )

            existing = load_manifest(output_dir)
            resuming = existing is not None and not force_restart
            if resuming:
                reasons = check_resume_compatible(existing, proposed.config_hash, model_digest)
                if reasons:
                    raise RuntimeError(
                        "Cannot resume — run configuration has changed:\n"
                        + "\n".join(f"  - {r}" for r in reasons)
                    )
                manifest = existing
                logger.info("Resume: configuration matches existing run %s", manifest.run_id)
            else:
                if force_restart:
                    jsonl = output_dir / JSONL_FILENAME
                    if jsonl.exists():
                        jsonl.unlink()
                manifest = proposed
                save_manifest(manifest, output_dir)

            logger.info("Loading model %s into memory...", model)
            cold_load_s = client.load_model(model)
            loaded = True
            if not resuming:
                manifest.cold_load_s = cold_load_s
                save_manifest(manifest, output_dir)

            if fixtures:
                warmup_fixture = fixtures[0]
                warmup_prompt = build_user_prompt(
                    warmup_fixture.tx_id, warmup_fixture.prompt, warmup_fixture.answer
                )
                warmup_result = client.call(model, ANSWER_JUDGE_SYSTEM_PROMPT, warmup_prompt)
                if warmup_result.timed_out or warmup_result.http_error or not warmup_result.content:
                    raise RuntimeError(f"Warm-up failed: {warmup_result.error_message}")
                parse_judge_response(warmup_result.content, "candidate-1")
                if not resuming:
                    manifest.warmup_latency_s = warmup_result.latency_s
                    save_manifest(manifest, output_dir)

            completed = load_completed_keys(output_dir)
            total_expected = len(fixtures) * trials
            skipped = 0
            for trial in range(1, trials + 1):
                trial_seed = config.seed * 10000 + trial
                shuffled = _shuffle_fixtures(fixtures, trial_seed)
                expected_order = manifest.trial_orders.get(str(trial), [])
                if [f.content_hash() for f in shuffled] != expected_order:
                    raise RuntimeError(f"Persisted fixture order mismatch for trial {trial}")
                for fixture in shuffled:
                    key = (
                        manifest.config_hash, model, fixture.content_hash(),
                        "candidate-1", trial,
                    )
                    if key in completed:
                        skipped += 1
                        continue
                    logger.info(
                        "[%d/%d] model=%s group=%s tx=%.12s perspective=%s trial=%d",
                        len(records) + skipped + 1, total_expected, model, fixture.group,
                        fixture.tx_id, fixture.perspective, trial,
                    )
                    record = _run_single(
                        client, model, fixture, trial, trial_seed,
                        manifest.config_hash, config.seed, max_retries,
                    )
                    _append_record(record, output_dir)
                    records.append(record)
            if skipped:
                logger.info("Skipped %d already-completed records (resume mode).", skipped)
        finally:
            if loaded:
                logger.info("Unloading model %s...", model)
                client.unload_model(model)

    return records
