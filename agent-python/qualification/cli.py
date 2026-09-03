"""
CLI entry point for the qualification harness.

Usage (from agent-python/):
    python -m qualification [--repetitions N] [--output-dir PATH]

All provider configuration is read from the environment via Settings (same env
vars as the service: LLM_PROVIDER, OPENAI_API_KEY, OPENAI_MODEL, etc.).
No API keys are read or written here.
"""
from __future__ import annotations

import argparse
import asyncio
import sys
from pathlib import Path


def _parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        prog="python -m qualification",
        description="Qualify a model as a MoA validator across all five operations.",
    )
    parser.add_argument(
        "--repetitions",
        type=int,
        default=3,
        metavar="N",
        help="Number of times to repeat each operation (default: 3).",
    )
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=Path("testresults") / "agent-qualification",
        metavar="PATH",
        help="Directory for JSON result files (default: testresults/agent-qualification/).",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    args = _parse_args(argv)

    # Import here so the module is importable without all deps installed (e.g. in tests).
    from config import Settings
    from providers.factory import create_provider
    from qualification.harness import run_qualification
    from qualification.report import build_json_report, print_summary, write_json_report

    settings = Settings()

    try:
        provider = create_provider(settings)
    except ValueError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        sys.exit(1)

    provider_name = settings.llm_provider
    model_name = settings.model

    print(f"Qualifying provider={provider_name!r} model={model_name!r} repetitions={args.repetitions}")

    run = asyncio.run(
        run_qualification(
            provider=provider,
            provider_name=provider_name,
            model_name=model_name,
            repetitions=args.repetitions,
        )
    )

    report = build_json_report(run)
    report_path = write_json_report(report, output_dir=args.output_dir)

    print_summary(report)
    print(f"Report written to: {report_path}")
