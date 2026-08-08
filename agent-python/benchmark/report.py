"""
Output file generation for the benchmark.

Produces:
  predictions.csv         — one row per prediction record
  model_summary.json      — global + per-group metrics per model
  pairwise_agreement.csv  — cross-model pairwise agreement matrix
  error_overlap.csv       — shared error analysis per model pair
  qualification_report.md — human-readable qualification verdicts
  run_config.json         — benchmark run configuration
"""
from __future__ import annotations

import csv
import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from benchmark.cross_model import SharedErrorAnalysis, find_shared_errors
from benchmark.metrics import LatencyStats, MetricsResult, RateView
from benchmark.qualification import QualificationResult


def _pct(v: float) -> str:
    return f"{v * 100:.1f}%"


def _fmt(v: float, decimals: int = 4) -> str:
    return f"{v:.{decimals}f}"


def _lat_str(ls: LatencyStats) -> str:
    if ls.count == 0:
        return "n/a"
    return (
        f"mean={ls.mean:.2f}s median={ls.median:.2f}s "
        f"p90={ls.p90:.2f}s p95={ls.p95:.2f}s max={ls.maximum:.2f}s n={ls.count}"
    )


# ── predictions.csv ──────────────────────────────────────────────────────────

PREDICTION_FIELDS = [
    "model", "group", "scenario_id", "tx_id", "candidate_id", "perspective",
    "assumption_basis", "expected", "predicted", "is_adversarial", "is_correct",
    "attempt_count", "final_latency_s", "total_latency_s",
    "timed_out", "http_error", "parse_error", "error_message",
    "config_hash", "fixture_hash", "seed", "trial", "trial_seed", "timestamp",
]


def write_predictions_csv(records: list[dict[str, Any]], output_dir: Path) -> Path:
    path = output_dir / "predictions.csv"
    with path.open("w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=PREDICTION_FIELDS, extrasaction="ignore")
        w.writeheader()
        w.writerows(records)
    return path


# ── helpers for metrics serialization ────────────────────────────────────────

def _rate_view_to_dict(rv: RateView) -> dict[str, Any]:
    return {
        "denominator_description": rv.denominator_description,
        "legitimate_count": rv.legitimate_count,
        "adversarial_count": rv.adversarial_count,
        "legitimate_retention": rv.legitimate_retention,
        "adversarial_rejection": rv.adversarial_rejection,
        "false_rejection": rv.false_rejection,
        "false_acceptance": rv.false_acceptance,
        "legitimate_invalid": rv.legitimate_invalid,
        "adversarial_invalid": rv.adversarial_invalid,
        "false_breakdown": {
            "leg_to_wrong": rv.false_breakdown.leg_to_wrong,
            "leg_to_hallucination": rv.false_breakdown.leg_to_hallucination,
            "leg_to_malicious": rv.false_breakdown.leg_to_malicious,
            "adv_to_correct": rv.false_breakdown.adv_to_correct,
        },
    }


def _lat_stats_to_dict(ls: LatencyStats) -> dict[str, Any]:
    return {
        "mean_s": ls.mean,
        "median_s": ls.median,
        "p90_s": ls.p90,
        "p95_s": ls.p95,
        "maximum_s": ls.maximum,
        "count": ls.count,
    }


def _metrics_to_dict(m: MetricsResult) -> dict[str, Any]:
    return {
        "total": m.total,
        "valid": m.valid,
        "accuracy": m.accuracy,
        "macro_precision": m.macro_precision,
        "macro_recall": m.macro_recall,
        "macro_f1": m.macro_f1,
        "all_candidate": _rate_view_to_dict(m.all_candidate),
        "conditional": _rate_view_to_dict(m.conditional),
        "invalid_output_rate": m.invalid_output_rate,
        "retry_rate": m.retry_rate,
        "timeout_rate": m.timeout_rate,
        "http_error_rate": m.http_error_rate,
        "parse_error_rate": m.parse_error_rate,
        "coverage_actual": m.coverage_actual,
        "coverage_expected": m.coverage_expected,
        "latency": {
            "warm_successful": _lat_stats_to_dict(m.latency.warm_successful),
            "all_successful": _lat_stats_to_dict(m.latency.all_successful),
            "failed_attempts": _lat_stats_to_dict(m.latency.failed_attempts),
            "timed_out_calls": _lat_stats_to_dict(m.latency.timed_out_calls),
            "retry_attempts": _lat_stats_to_dict(m.latency.retry_attempts),
            "ollama_eval_successful_seconds": _lat_stats_to_dict(m.latency.ollama_eval_successful),
            "ollama_eval_failed_seconds": _lat_stats_to_dict(m.latency.ollama_eval_failed),
        },
        "per_class": {
            cat: {
                "precision": pc.precision,
                "recall": pc.recall,
                "f1": pc.f1,
                "support": pc.support,
            }
            for cat, pc in m.per_class.items()
        },
        "confusion_matrix": m.confusion_matrix,
    }


# ── model_summary.json ───────────────────────────────────────────────────────

def write_model_summary(
    qual_results: list[QualificationResult],
    output_dir: Path,
) -> Path:
    summary: dict[str, Any] = {}
    for qr in qual_results:
        summary[qr.model] = {
            "verdict": qr.verdict,
            "coverage_actual": qr.coverage_actual,
            "coverage_expected": qr.coverage_expected,
            "global_metrics": _metrics_to_dict(qr.global_metrics),
            "per_group_metrics": {
                g: _metrics_to_dict(m) for g, m in qr.per_group_metrics.items()
            },
            "global_failures": qr.global_failures,
            "group_failures": qr.group_failures,
            "recommendations": qr.recommendations,
        }
    path = output_dir / "model_summary.json"
    path.write_text(json.dumps(summary, indent=2))
    return path


# ── pairwise_agreement.csv ───────────────────────────────────────────────────

def write_pairwise_agreement(
    analysis: SharedErrorAnalysis,
    output_dir: Path,
) -> Path:
    path = output_dir / "pairwise_agreement.csv"
    fields = [
        "model_a", "model_b", "total_shared", "agreement_count",
        "agreement_rate", "shared_false_acceptances", "shared_false_rejections",
    ]
    with path.open("w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields)
        w.writeheader()
        for pr in analysis.pairwise:
            w.writerow({
                "model_a": pr.model_a,
                "model_b": pr.model_b,
                "total_shared": pr.total_shared,
                "agreement_count": pr.agreement_count,
                "agreement_rate": f"{pr.agreement_rate:.4f}",
                "shared_false_acceptances": pr.shared_false_acceptances,
                "shared_false_rejections": pr.shared_false_rejections,
            })
    return path


# ── error_overlap.csv ────────────────────────────────────────────────────────

def write_error_overlap(
    analysis: SharedErrorAnalysis,
    output_dir: Path,
) -> Path:
    path = output_dir / "error_overlap.csv"
    fields = [
        "scope", "model", "group", "tx_id", "perspective",
        "expected", "predicted_or_predictions",
    ]
    with path.open("w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields)
        w.writeheader()
        for item in analysis.failed_by_all:
            w.writerow({
                "scope": "ALL_MODELS",
                "model": "",
                "group": item.get("group"),
                "tx_id": item.get("tx_id"),
                "perspective": item.get("perspective"),
                "expected": item.get("expected"),
                "predicted_or_predictions": json.dumps(item.get("predictions", {})),
            })
        for item in analysis.failed_by_majority:
            w.writerow({
                "scope": "MAJORITY_MODELS",
                "model": "",
                "group": item.get("group"),
                "tx_id": item.get("tx_id"),
                "perspective": item.get("perspective"),
                "expected": item.get("expected"),
                "predicted_or_predictions": json.dumps(item.get("predictions", {})),
            })
        for model, errors in analysis.model_specific_failures.items():
            for item in errors:
                w.writerow({
                    "scope": "MODEL_SPECIFIC",
                    "model": model,
                    "group": item.get("group"),
                    "tx_id": item.get("tx_id"),
                    "perspective": item.get("perspective"),
                    "expected": item.get("expected"),
                    "predicted_or_predictions": item.get("predicted", ""),
                })
        for model, missing_list in analysis.missing_predictions.items():
            for item in missing_list:
                w.writerow({
                    "scope": "MISSING_PREDICTION",
                    "model": model,
                    "group": item.get("group"),
                    "tx_id": item.get("tx_id"),
                    "perspective": item.get("perspective"),
                    "expected": item.get("expected"),
                    "predicted_or_predictions": item.get("outcome", ""),
                })
    return path


# ── qualification_report.md ──────────────────────────────────────────────────

def _verdict_badge(v: str) -> str:
    return {
        "QUALIFIED": "**QUALIFIED**",
        "CONDITIONALLY_QUALIFIED": "**CONDITIONALLY QUALIFIED**",
        "REJECTED": "**REJECTED**",
    }.get(v, v)


def write_qualification_report(
    qual_results: list[QualificationResult],
    analysis: SharedErrorAnalysis,
    run_config: dict[str, Any],
    output_dir: Path,
) -> Path:
    lines: list[str] = []
    now = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")

    lines.append("# MR2 Judge Qualification Report")
    lines.append(f"\nGenerated: {now}  ")
    lines.append(f"Prompt version: `{run_config.get('prompt_version', 'answer-judge-v4')}`  ")
    lines.append(f"Prompt hash: `{run_config.get('prompt_hash', '')[:16]}...`  ")
    lines.append(f"Dataset version: `{run_config.get('dataset_version', '')}` "
                 f"hash: `{run_config.get('dataset_hash', '')[:16]}...`  ")
    lines.append(f"Seed: {run_config.get('seed', 42)}  ")
    lines.append(f"Trials per fixture: {run_config.get('trials', 1)}  ")
    lines.append(f"Models evaluated: {len(qual_results)}\n")

    # ── Summary table ──────────────────────────────────────────────────────
    lines.append("## Summary\n")
    lines.append(
        "| Model | Verdict | Coverage | Accuracy | Leg.Ret (all) | Leg.Ret (cond) "
        "| Adv.Rej (all) | Adv.Rej (cond) | Timeouts | Macro F1 |"
    )
    lines.append("|---|---|---|---|---|---|---|---|---|---|")
    for qr in qual_results:
        gm = qr.global_metrics
        cov = f"{qr.coverage_actual}/{qr.coverage_expected}" if qr.coverage_expected else str(qr.coverage_actual)
        lines.append(
            f"| `{qr.model}` | {_verdict_badge(qr.verdict)} | {cov} "
            f"| {_pct(gm.accuracy)} "
            f"| {_pct(gm.all_candidate.legitimate_retention)} "
            f"| {_pct(gm.conditional.legitimate_retention)} "
            f"| {_pct(gm.all_candidate.adversarial_rejection)} "
            f"| {_pct(gm.conditional.adversarial_rejection)} "
            f"| {_pct(gm.timeout_rate)} | {_fmt(gm.macro_f1)} |"
        )
    lines.append("")

    lines.append(
        "> **all** = all scheduled candidates (timeouts/errors count as failures in denominator)  \n"
        "> **cond** = valid-output records only\n"
    )

    # ── Per-model detail ───────────────────────────────────────────────────
    for qr in qual_results:
        gm = qr.global_metrics
        ac = gm.all_candidate
        cond = gm.conditional

        lines.append(f"## {qr.model}\n")
        lines.append(f"**Verdict:** {_verdict_badge(qr.verdict)}  ")
        lines.append(
            f"**Coverage:** {qr.coverage_actual}/{qr.coverage_expected or '?'} records\n"
        )

        if qr.global_failures:
            lines.append("**Global failures:**")
            for f_ in qr.global_failures:
                lines.append(f"- {f_}")
            lines.append("")

        if qr.group_failures:
            lines.append("**Per-group failures:**")
            for f_ in qr.group_failures:
                lines.append(f"- {f_}")
            lines.append("")

        # Per-group table
        lines.append("### Per-group metrics\n")
        lines.append(
            "| Group | Leg.Ret (all) | Leg.Ret (cond) "
            "| Adv.Rej (all) | Adv.Rej (cond) | Assumption | Pass |"
        )
        lines.append("|---|---|---|---|---|---|---|")
        for gv in qr.group_verdicts:
            assumption = "benchmark-assumption" if gv.group in ("E", "F") else "canonical"
            icon = "OK" if gv.passes else "FAIL"
            lines.append(
                f"| {gv.group} "
                f"| {_pct(gv.legitimate_retention)} "
                f"| {_pct(gv.cond_legitimate_retention)} "
                f"| {_pct(gv.adversarial_rejection)} "
                f"| {_pct(gv.cond_adversarial_rejection)} "
                f"| {assumption} | {icon} |"
            )
        lines.append("")

        # Latency breakdown
        lat = gm.latency
        lines.append("### Latency (wall-clock seconds)\n")
        lines.append(f"- Warm successful: {_lat_str(lat.warm_successful)}")
        lines.append(f"- All successful:  {_lat_str(lat.all_successful)}")
        lines.append(f"- Failed (non-timeout): {_lat_str(lat.failed_attempts)}")
        lines.append(f"- Timed out:       {_lat_str(lat.timed_out_calls)}")
        lines.append(f"- Retries:         {_lat_str(lat.retry_attempts)}")
        lines.append(f"- Ollama eval successful (ns converted to s): {_lat_str(lat.ollama_eval_successful)}")
        lines.append(f"- Ollama eval failed (ns converted to s): {_lat_str(lat.ollama_eval_failed)}")
        manifest = run_config.get("model_manifests", {}).get(qr.model, {})
        lines.append(f"- Cold load (wall-clock): {manifest.get('cold_load_s')}s")
        lines.append(f"- Excluded warm-up (wall-clock): {manifest.get('warmup_latency_s')}s")
        lines.append("")

        # Confusion matrix
        cats = ["CORRECT", "WRONG", "HALLUCINATION", "MALICIOUS"]
        predicted_cats = cats + ["INVALID"]
        lines.append("### Confusion matrix (all scheduled candidates)\n")
        lines.append("| Expected \\ Predicted | " + " | ".join(predicted_cats) + " |")
        lines.append("|" + "---|" * (len(predicted_cats) + 1))
        for exp in cats:
            row_vals = [str(gm.confusion_matrix.get(exp, {}).get(pred, 0)) for pred in predicted_cats]
            lines.append(f"| {exp} | " + " | ".join(row_vals) + " |")
        lines.append("")

        if qr.recommendations:
            lines.append("### Recommendations\n")
            for rec in qr.recommendations:
                lines.append(f"- {rec}")
            lines.append("")

    # ── Cross-model analysis ───────────────────────────────────────────────
    lines.append("## Cross-model analysis\n")
    lines.append(f"Declared models: {', '.join(f'`{m}`' for m in analysis.declared_models)}\n")

    # Coverage gaps
    if any(v > 0 for v in analysis.coverage_gaps.values()):
        lines.append("### Missing predictions\n")
        lines.append("| Model | Missing prediction count |")
        lines.append("|---|---|")
        for m, count in sorted(analysis.coverage_gaps.items()):
            lines.append(f"| `{m}` | {count} |")
        lines.append("")

    if analysis.pairwise:
        lines.append("### Pairwise agreement\n")
        lines.append("| Model A | Model B | Agreement | Shared FA | Shared FR |")
        lines.append("|---|---|---|---|---|")
        for pr in sorted(analysis.pairwise, key=lambda x: -x.agreement_rate):
            lines.append(
                f"| `{pr.model_a}` | `{pr.model_b}` | {_pct(pr.agreement_rate)} "
                f"| {pr.shared_false_acceptances} | {pr.shared_false_rejections} |"
            )
        lines.append("")

    if analysis.failed_by_all:
        lines.append(f"### Fixtures failed by ALL declared models ({len(analysis.failed_by_all)})\n")
        for item in analysis.failed_by_all:
            preds = ", ".join(
                f"`{m}:{v}`" for m, v in item.get("predictions", {}).items()
            )
            lines.append(
                f"- Group {item['group']} tx=`{str(item['tx_id'])[:12]}...` "
                f"perspective={item['perspective']} expected={item['expected']} → {preds}"
            )
        lines.append("")

    if analysis.failed_by_majority:
        lines.append(f"### Fixtures failed by majority of models ({len(analysis.failed_by_majority)})\n")
        for item in analysis.failed_by_majority:
            wrong = item.get("wrong_models", [])
            missing = item.get("missing_models", [])
            lines.append(
                f"- Group {item['group']} tx=`{str(item['tx_id'])[:12]}...` "
                f"perspective={item['perspective']} expected={item['expected']} "
                f"wrong={wrong} missing={missing}"
            )
        lines.append("")

    path = output_dir / "qualification_report.md"
    path.write_text("\n".join(lines))
    return path


# ── run_config.json ───────────────────────────────────────────────────────────

def write_run_config(config: dict[str, Any], output_dir: Path) -> Path:
    path = output_dir / "run_config.json"
    config_with_ts = {**config, "generated_at": datetime.now(timezone.utc).isoformat()}
    path.write_text(json.dumps(config_with_ts, indent=2))
    return path


# ── top-level orchestration ───────────────────────────────────────────────────

def generate_all_outputs(
    records_by_model: dict[str, list[dict[str, Any]]],
    qual_results: list[QualificationResult],
    run_config: dict[str, Any],
    output_dir: Path,
    declared_models: list[str] | None = None,
) -> list[Path]:
    """Write all non-JSONL output files and return their paths."""
    all_records: list[dict[str, Any]] = []
    for recs in records_by_model.values():
        all_records.extend(recs)

    if declared_models is None:
        declared_models = sorted(records_by_model.keys())

    analysis = find_shared_errors(records_by_model, declared_models=declared_models)

    return [
        write_predictions_csv(all_records, output_dir),
        write_model_summary(qual_results, output_dir),
        write_pairwise_agreement(analysis, output_dir),
        write_error_overlap(analysis, output_dir),
        write_qualification_report(qual_results, analysis, run_config, output_dir),
        write_run_config(run_config, output_dir),
    ]
