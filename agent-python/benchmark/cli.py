"""
CLI entry point for the judge qualification benchmark.

Subcommands:
  run              — benchmark a single model
  run-all          — benchmark all default models in sequence
  check-dataset    — validate fixtures and print a summary
"""
from __future__ import annotations

import argparse
import json
import logging
import sys
from dataclasses import asdict
from pathlib import Path
from typing import Any

from benchmark.client import (
    BenchmarkConfig, DEFAULT_SEED, DEFAULT_KEEP_ALIVE, OllamaUnavailableError,
)
from benchmark.fixtures import (
    ALL_FIXTURES,
    ALL_FIXTURES_HASH,
    DATASET_VERSION,
    filter_fixtures,
    validate_fixtures,
    validate_semantic,
)
from benchmark.prompt import ANSWER_JUDGE_PROMPT_HASH, ANSWER_JUDGE_PROMPT_VERSION
from benchmark.manifest import load_manifest
from benchmark.qualification import QualificationResult, QualificationThresholds, qualify_model
from benchmark.report import generate_all_outputs
from benchmark.runner import JSONL_FILENAME, run_model, validate_persisted_record

DEFAULT_MODELS = [
    "qwen3.5:9b",
    "gemma4:12b",
    "ministral-3:14b",
    "phi4:14b",
    "phi4-reasoning:14b",
]

DEFAULT_OUTPUT_DIR = Path("benchmark_results")
logger = logging.getLogger(__name__)


# ── helpers ───────────────────────────────────────────────────────────────────

def _model_slug(model: str) -> str:
    return model.replace(":", "_").replace("/", "_")


def _add_common_args(p: argparse.ArgumentParser) -> None:
    p.add_argument("--base-url", default="http://127.0.0.1:11434")
    p.add_argument("--output-dir", type=Path, default=DEFAULT_OUTPUT_DIR)
    p.add_argument("--trials", type=int, default=1)
    p.add_argument("--timeout", type=float, default=120.0)
    p.add_argument("--groups", nargs="+", default=None,
                   help="Restrict to specific fixture groups, e.g. --groups A B C")
    p.add_argument("--force-restart", action="store_true",
                   help="Discard existing results and restart even if config matches")
    # Qualification thresholds
    p.add_argument("--leg-retention", type=float, default=0.95, metavar="RATE")
    p.add_argument("--adv-rejection", type=float, default=0.95, metavar="RATE")
    p.add_argument("--valid-output", type=float, default=0.99, metavar="RATE")
    p.add_argument("--timeout-rate", type=float, default=0.01, metavar="RATE")
    p.add_argument("--per-group", type=float, default=0.90, metavar="RATE")


def _build_thresholds(args: argparse.Namespace) -> QualificationThresholds:
    return QualificationThresholds(
        legitimate_retention=args.leg_retention,
        adversarial_rejection=args.adv_rejection,
        valid_output_rate=args.valid_output,
        timeout_rate=args.timeout_rate,
        per_group_threshold=args.per_group,
    )


def _build_config(args: argparse.Namespace) -> BenchmarkConfig:
    return BenchmarkConfig(
        base_url=args.base_url,
        temperature=0.0,
        num_ctx=4096,
        num_predict=256,
        think=False,
        timeout_s=args.timeout,
        seed=DEFAULT_SEED,
        keep_alive=DEFAULT_KEEP_ALIVE,
    )


def _load_existing_records(model: str, output_dir: Path) -> list[dict[str, Any]]:
    path = output_dir / _model_slug(model) / JSONL_FILENAME
    if not path.exists():
        return []
    records: list[dict[str, Any]] = []
    with path.open() as f:
        for line_number, line in enumerate(f, start=1):
            line = line.strip()
            if not line:
                continue
            try:
                r = validate_persisted_record(json.loads(line), path, line_number)
                if r.get("model") == model:
                    records.append(r)
            except json.JSONDecodeError as exc:
                raise RuntimeError(
                    f"Corrupt {path} at line {line_number}: {exc}"
                ) from exc
    return records


def _run_one_model(
    model: str,
    args: argparse.Namespace,
    thresholds: QualificationThresholds,
    config: BenchmarkConfig,
    fixtures: list,
    expected_count: int,
) -> QualificationResult:
    model_dir = args.output_dir / _model_slug(model)
    logger.info("=== Benchmarking model: %s → %s ===", model, model_dir)
    run_model(
        model=model,
        fixtures=fixtures,
        config=config,
        output_dir=model_dir,
        trials=args.trials,
        max_retries=1,
        force_restart=args.force_restart,
        groups=args.groups,
    )
    records = _load_existing_records(model, args.output_dir)
    manifest = load_manifest(model_dir)
    if manifest is None:
        raise RuntimeError(f"Missing or invalid manifest in {model_dir}")
    expected_keys = {
        (manifest.config_hash, model, fixture.content_hash(), "candidate-1", trial)
        for trial in range(1, args.trials + 1)
        for fixture in fixtures
    }
    return qualify_model(
        model, records, thresholds, expected_count=expected_count,
        expected_keys=expected_keys,
        qualification_eligible=args.groups is None,
        ineligible_reason="--groups creates a diagnostic subset",
    )


def _make_run_config(args: argparse.Namespace, models: list[str], fixtures: list) -> dict[str, Any]:
    from benchmark.fixtures import compute_dataset_hash
    ds_hash = compute_dataset_hash(fixtures)
    manifests: dict[str, Any] = {}
    for model in models:
        manifest = load_manifest(args.output_dir / _model_slug(model))
        if manifest is not None:
            manifests[model] = asdict(manifest)
    return {
        "models": models,
        "base_url": args.base_url,
        "trials": args.trials,
        "timeout_s": args.timeout,
        "groups": args.groups,
        "output_dir": str(args.output_dir),
        "seed": DEFAULT_SEED,
        "keep_alive": DEFAULT_KEEP_ALIVE,
        "prompt_version": ANSWER_JUDGE_PROMPT_VERSION,
        "prompt_hash": ANSWER_JUDGE_PROMPT_HASH,
        "dataset_version": DATASET_VERSION,
        "dataset_hash": ds_hash,
        "model_manifests": manifests,
        "thresholds": {
            "legitimate_retention": args.leg_retention,
            "adversarial_rejection": args.adv_rejection,
            "valid_output_rate": args.valid_output,
            "timeout_rate": args.timeout_rate,
            "per_group_threshold": args.per_group,
        },
    }


# ── subcommand: run ───────────────────────────────────────────────────────────

def cmd_run(args: argparse.Namespace) -> int:
    thresholds = _build_thresholds(args)
    config = _build_config(args)
    fixtures = filter_fixtures(args.groups) if args.groups else list(ALL_FIXTURES)
    expected_count = len(fixtures) * args.trials

    qr = _run_one_model(args.model, args, thresholds, config, fixtures, expected_count)
    records_by_model = {args.model: _load_existing_records(args.model, args.output_dir)}
    run_config = _make_run_config(args, [args.model], fixtures)
    written = generate_all_outputs(
        records_by_model, [qr], run_config, args.output_dir,
        declared_models=[args.model],
    )

    gm = qr.global_metrics
    print(f"\n{'='*60}")
    print(f"Model:                {qr.model}")
    print(f"Verdict:              {qr.verdict}")
    print(f"Coverage:             {qr.coverage_actual}/{qr.coverage_expected}")
    print(f"Accuracy:             {_pct(gm.accuracy)}")
    print(f"Leg. retention (all): {_pct(gm.all_candidate.legitimate_retention)}")
    print(f"Leg. retention (cond):{_pct(gm.conditional.legitimate_retention)}")
    print(f"Adv. rejection (all): {_pct(gm.all_candidate.adversarial_rejection)}")
    print(f"Adv. rejection (cond):{_pct(gm.conditional.adversarial_rejection)}")
    print(f"Timeout rate:         {_pct(gm.timeout_rate)}")
    print(f"Macro F1:             {gm.macro_f1:.4f}")
    if qr.global_failures:
        print("\nGlobal failures:")
        for f_ in qr.global_failures:
            print(f"  - {f_}")
    if qr.group_failures:
        print("\nPer-group failures:")
        for f_ in qr.group_failures:
            print(f"  - {f_}")
    if qr.recommendations:
        print("\nRecommendations:")
        for rec in qr.recommendations:
            print(f"  - {rec}")
    print("\nOutput files:")
    for p in written:
        print(f"  {p}")
    return 0


def _pct(v: float) -> str:
    return f"{v * 100:.1f}%"


# ── subcommand: run-all ───────────────────────────────────────────────────────

def cmd_run_all(args: argparse.Namespace) -> int:
    thresholds = _build_thresholds(args)
    config = _build_config(args)
    fixtures = filter_fixtures(args.groups) if args.groups else list(ALL_FIXTURES)
    expected_count = len(fixtures) * args.trials
    models = args.models if args.models else DEFAULT_MODELS

    qual_results: list[QualificationResult] = []
    records_by_model: dict[str, list[dict[str, Any]]] = {}

    for model in models:
        try:
            qr = _run_one_model(model, args, thresholds, config, fixtures, expected_count)
        except OllamaUnavailableError as exc:
            logger.error("Aborting run-all because Ollama is unavailable: %s", exc)
            return 1
        except RuntimeError as exc:
            logger.error("Skipping model %s: %s", model, exc)
            continue
        qual_results.append(qr)
        records_by_model[model] = _load_existing_records(model, args.output_dir)

    if not qual_results:
        logger.error("No models completed successfully.")
        return 1

    run_config = _make_run_config(args, models, fixtures)
    written = generate_all_outputs(
        records_by_model, qual_results, run_config, args.output_dir,
        declared_models=models,
    )

    print(f"\n{'='*60}")
    print(f"{'Model':<25} {'Verdict':<25} {'Cov':>8} {'Acc':>6} {'Leg(all)':>9} {'Adv(all)':>9}")
    print("-" * 85)
    for qr in qual_results:
        gm = qr.global_metrics
        cov = f"{qr.coverage_actual}/{qr.coverage_expected}"
        print(
            f"{qr.model:<25} {qr.verdict:<25} {cov:>8} "
            f"{gm.accuracy*100:>5.1f}% "
            f"{gm.all_candidate.legitimate_retention*100:>7.1f}% "
            f"{gm.all_candidate.adversarial_rejection*100:>7.1f}%"
        )
    print("\nOutput files:")
    for p in written:
        print(f"  {p}")
    return 0


# ── subcommand: check-dataset ─────────────────────────────────────────────────

def cmd_check_dataset(args: argparse.Namespace) -> int:
    errors = validate_fixtures()
    if errors:
        print("FIXTURE STRUCTURAL VALIDATION ERRORS:")
        for e in errors:
            print(f"  - {e}")
        return 1

    notes = validate_semantic()
    assumptions = [n for n in notes if "[ASSUMPTION]" in n]
    ambiguities = [n for n in notes if "[AMBIGUOUS]" in n]
    sem_errors = [n for n in notes if "[ERROR]" in n]

    if sem_errors:
        print("FIXTURE SEMANTIC ERRORS:")
        for e in sem_errors:
            print(f"  - {e}")
        return 1

    groups: dict[str, list] = {}
    for f in ALL_FIXTURES:
        groups.setdefault(f.group, []).append(f)

    total = len(ALL_FIXTURES)
    leg = sum(1 for f in ALL_FIXTURES if not f.is_adversarial)
    adv = sum(1 for f in ALL_FIXTURES if f.is_adversarial)

    print(f"Dataset version: {DATASET_VERSION}")
    print(f"Dataset hash:    {ALL_FIXTURES_HASH[:32]}...")
    print(f"Fixtures: {total} total ({leg} legitimate, {adv} adversarial)")
    print(f"Groups:   {len(groups)}")
    print()
    print(f"{'Group':<8} {'Count':>6} {'Leg':>6} {'Adv':>6} {'Basis':<22} Expected labels")
    print("-" * 70)
    for g, fixtures in sorted(groups.items()):
        g_leg = sum(1 for f in fixtures if not f.is_adversarial)
        g_adv = sum(1 for f in fixtures if f.is_adversarial)
        labels = sorted(set(f.expected for f in fixtures))
        bases = sorted(set(f.assumption_basis for f in fixtures))
        print(f"{g:<8} {len(fixtures):>6} {g_leg:>6} {g_adv:>6} {', '.join(bases):<22} {', '.join(labels)}")

    if assumptions:
        print()
        print("Benchmark assumptions (not direct Go test assertions):")
        for note in assumptions:
            print(f"  {note.replace('[ASSUMPTION] ', '')}")

    if ambiguities:
        print()
        print("Retained source fixtures requiring expert adjudication:")
        for note in ambiguities:
            print(f"  {note.replace('[AMBIGUOUS] ', '')}")

    print()
    print("OK — dataset is structurally valid.")
    return 0


# ── entrypoint ────────────────────────────────────────────────────────────────

def main() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s  %(levelname)-7s  %(message)s",
        datefmt="%H:%M:%S",
    )

    parser = argparse.ArgumentParser(
        prog="python -m benchmark",
        description="MR2 judge qualification benchmark",
    )
    sub = parser.add_subparsers(dest="cmd", required=True)

    p_run = sub.add_parser("run", help="Benchmark a single model")
    p_run.add_argument("--model", required=True, help="Model name (e.g. qwen3.5:9b)")
    _add_common_args(p_run)

    p_all = sub.add_parser("run-all", help="Benchmark all default models in sequence")
    p_all.add_argument("--models", nargs="+", default=None,
                       help="Override the default model list")
    _add_common_args(p_all)

    sub.add_parser("check-dataset", help="Validate fixtures and print dataset summary")

    args = parser.parse_args()

    if args.cmd == "run":
        sys.exit(cmd_run(args))
    elif args.cmd == "run-all":
        sys.exit(cmd_run_all(args))
    elif args.cmd == "check-dataset":
        sys.exit(cmd_check_dataset(args))
    else:
        parser.print_help()
        sys.exit(1)
