"""
Build and persist qualification reports.

JSON report is written to testresults/agent-qualification/<provider>_<model>_<timestamp>.json.
Human-readable summary is printed to stdout.
"""
from __future__ import annotations

import json
import re
from dataclasses import asdict
from datetime import datetime, timezone
from pathlib import Path
from typing import TYPE_CHECKING

from qualification.metrics import (
    AccuracyStats,
    LatencyStats,
    OperationSummary,
    TokenTotals,
    eval_accuracy,
    judge_accuracy,
    operation_summary,
)

if TYPE_CHECKING:
    from qualification.harness import QualRun


_DEFAULT_OUTPUT_DIR = Path("testresults") / "agent-qualification"


def _latency_dict(ls: LatencyStats | None) -> dict | None:
    if ls is None:
        return None
    return {
        "count": ls.count,
        "min_ms": round(ls.min_ms, 2),
        "mean_ms": round(ls.mean_ms, 2),
        "median_ms": round(ls.median_ms, 2),
        "p90_ms": round(ls.p90_ms, 2),
        "p95_ms": round(ls.p95_ms, 2),
        "max_ms": round(ls.max_ms, 2),
    }


def _tokens_dict(tt: TokenTotals) -> dict:
    return {
        "total_input": tt.total_input,
        "total_output": tt.total_output,
        "total_all": tt.total_all,
    }


def _op_dict(summary: OperationSummary) -> dict:
    return {
        "n_calls": summary.n_calls,
        "n_success": summary.n_success,
        "success_rate": round(summary.success_rate, 4),
        "latency": _latency_dict(summary.latency),
        "tokens": _tokens_dict(summary.tokens),
    }


def _accuracy_dict(acc: AccuracyStats) -> dict:
    return {
        "n_correct": acc.n_correct,
        "n_total": acc.n_total,
        "accuracy": round(acc.accuracy, 4),
        "breakdown": acc.breakdown,
    }


def build_json_report(run: "QualRun") -> dict:
    """Assemble the full qualification report as a plain dict."""
    by_op: dict[str, list] = {}
    for rec in run.records:
        by_op.setdefault(rec.operation, []).append(rec)

    operations: dict[str, dict] = {}
    for op, records in by_op.items():
        summary = operation_summary(records)
        entry = _op_dict(summary)

        if op == "judge":
            entry["accuracy"] = _accuracy_dict(judge_accuracy(records))
        elif op == "evaluate_synthesis":
            entry["accuracy"] = _accuracy_dict(eval_accuracy(records))

        operations[op] = entry

    elapsed = run.finished_at - run.started_at
    return {
        "schema_version": "qual-v1",
        "provider": run.provider_name,
        "model": run.model_name,
        "repetitions": run.repetitions,
        "started_at": datetime.fromtimestamp(run.started_at, tz=timezone.utc).isoformat(),
        "finished_at": datetime.fromtimestamp(run.finished_at, tz=timezone.utc).isoformat(),
        "elapsed_seconds": round(elapsed, 2),
        "operations": operations,
        "records": [
            {
                "operation": r.operation,
                "repetition": r.repetition,
                "started_at": datetime.fromtimestamp(r.started_at, tz=timezone.utc).isoformat(),
                "duration_ms": round(r.duration_ms, 2),
                "success": r.success,
                "error": r.error,
                "input_tokens": r.input_tokens,
                "output_tokens": r.output_tokens,
                "total_tokens": r.total_tokens,
                "data": r.data,
            }
            for r in run.records
        ],
    }


def _safe_filename(s: str) -> str:
    return re.sub(r"[^a-zA-Z0-9._-]", "_", s)


def write_json_report(report: dict, output_dir: Path = _DEFAULT_OUTPUT_DIR) -> Path:
    """Write the JSON report to disk and return the path."""
    output_dir.mkdir(parents=True, exist_ok=True)
    provider = _safe_filename(report["provider"])
    model = _safe_filename(report["model"])
    ts = datetime.now(tz=timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    path = output_dir / f"{provider}_{model}_{ts}.json"
    path.write_text(json.dumps(report, indent=2, ensure_ascii=False), encoding="utf-8")
    return path


def print_summary(report: dict) -> None:
    """Print a human-readable qualification summary to stdout."""
    sep = "=" * 72
    print(sep)
    print(f"  Qualification report: {report['provider']} / {report['model']}")
    print(f"  Repetitions: {report['repetitions']}  |  Elapsed: {report['elapsed_seconds']}s")
    print(sep)

    ops = report.get("operations", {})
    for op_name in ["label", "answer", "judge", "synthesize", "evaluate_synthesis"]:
        if op_name not in ops:
            continue
        op = ops[op_name]
        print(f"\n  [{op_name}]")
        print(f"    calls: {op['n_success']}/{op['n_calls']} succeeded  ({op['success_rate']*100:.1f}%)")

        lat = op.get("latency")
        if lat:
            print(
                f"    latency (ms):  min={lat['min_ms']:.1f}  mean={lat['mean_ms']:.1f}"
                f"  p90={lat['p90_ms']:.1f}  p95={lat['p95_ms']:.1f}  max={lat['max_ms']:.1f}"
            )

        tok = op.get("tokens", {})
        if tok.get("total_all") is not None:
            print(f"    tokens:  in={tok['total_input']}  out={tok['total_output']}  total={tok['total_all']}")

        acc = op.get("accuracy")
        if acc:
            pct = acc["accuracy"] * 100
            print(f"    accuracy: {acc['n_correct']}/{acc['n_total']} ({pct:.1f}%)")
            for entry in acc.get("breakdown", []):
                label = entry.get("fixture_label", "?")
                nc = entry.get("n_correct", 0)
                nt = entry.get("n_total", 0)
                print(f"      {label}: {nc}/{nt}")

    print(f"\n{sep}\n")
