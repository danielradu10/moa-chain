"""
Pure metric computation for qualification runs.

All functions are stateless and take plain Python data structures so they can
be tested without creating provider instances.

Latency distributions are computed over ALL calls (success and failure) so
that timeouts and errors do not silently disappear from the distribution. This
matches the benchmark/metrics.py philosophy (Finding 1 — denominators).
Percentiles use linear interpolation, same as benchmark/metrics.py.
"""
from __future__ import annotations

import statistics
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from qualification.harness import CallRecord


# ── Latency ──────────────────────────────────────────────────────────────────

@dataclass
class LatencyStats:
    count: int
    min_ms: float
    mean_ms: float
    median_ms: float
    p90_ms: float
    p95_ms: float
    max_ms: float


def _percentile(values: list[float], pct: float) -> float:
    """Linear interpolation percentile, matching benchmark/metrics.py."""
    if not values:
        return 0.0
    sorted_v = sorted(values)
    k = (len(sorted_v) - 1) * pct / 100.0
    lo, hi = int(k), min(int(k) + 1, len(sorted_v) - 1)
    return sorted_v[lo] + (k - lo) * (sorted_v[hi] - sorted_v[lo])


def compute_latency(records: list[Any]) -> LatencyStats | None:
    """Latency over ALL records, including failed ones.

    Including failures ensures that models with high error or timeout rates do
    not appear artificially fast in the distribution.
    """
    if not records:
        return None
    values = [r.duration_ms for r in records]
    return LatencyStats(
        count=len(values),
        min_ms=min(values),
        mean_ms=statistics.mean(values),
        median_ms=statistics.median(values),
        p90_ms=_percentile(values, 90),
        p95_ms=_percentile(values, 95),
        max_ms=max(values),
    )


# ── Tokens ───────────────────────────────────────────────────────────────────

@dataclass
class TokenTotals:
    total_input: int | None
    total_output: int | None
    total_all: int | None


def compute_tokens(records: list[Any]) -> TokenTotals:
    """Sum token counts across records.

    Returns None for a field when no record in the batch reported it.
    Partial reporting (some records have counts, some don't) sums the
    available values and returns that partial sum — the caller should
    note that the total may be an undercount.
    """
    inputs = [r.input_tokens for r in records if r.input_tokens is not None]
    outputs = [r.output_tokens for r in records if r.output_tokens is not None]
    totals = [r.total_tokens for r in records if r.total_tokens is not None]
    return TokenTotals(
        total_input=sum(inputs) if inputs else None,
        total_output=sum(outputs) if outputs else None,
        total_all=sum(totals) if totals else None,
    )


# ── Per-operation summary ────────────────────────────────────────────────────

@dataclass
class OperationSummary:
    n_calls: int
    n_success: int
    success_rate: float
    latency: LatencyStats | None
    tokens: TokenTotals


def operation_summary(records: list[Any]) -> OperationSummary:
    n = len(records)
    n_success = sum(1 for r in records if r.success)
    return OperationSummary(
        n_calls=n,
        n_success=n_success,
        success_rate=n_success / n if n > 0 else 0.0,
        latency=compute_latency(records),
        tokens=compute_tokens(records),
    )


# ── Accuracy (judge and evaluate_synthesis) ───────────────────────────────────

@dataclass
class AccuracyStats:
    n_correct: int
    n_total: int
    accuracy: float
    breakdown: list[dict]


def judge_accuracy(records: list[Any]) -> AccuracyStats:
    """Classification accuracy for judge calls.

    Each record's data dict must contain:
      fixture_label, expected, actual, matches
    """
    n_total = len(records)
    n_correct = sum(1 for r in records if r.data.get("matches") is True)

    by_label: dict[str, dict] = {}
    for r in records:
        label = r.data.get("fixture_label", "?")
        if label not in by_label:
            by_label[label] = {
                "fixture_label": label,
                "expected": r.data.get("expected"),
                "n_correct": 0,
                "n_total": 0,
                "attempts": [],
            }
        entry = by_label[label]
        entry["n_total"] += 1
        if r.data.get("matches") is True:
            entry["n_correct"] += 1
        entry["attempts"].append({
            "repetition": r.repetition,
            "actual": r.data.get("actual"),
            "matches": r.data.get("matches"),
            "success": r.success,
            "error": r.error,
        })

    return AccuracyStats(
        n_correct=n_correct,
        n_total=n_total,
        accuracy=n_correct / n_total if n_total > 0 else 0.0,
        breakdown=list(by_label.values()),
    )


def eval_accuracy(records: list[Any]) -> AccuracyStats:
    """Approval-verdict accuracy for evaluate_synthesis calls.

    Each record's data dict must contain:
      fixture_label, expected_approved, actual_approved, matches
    """
    n_total = len(records)
    n_correct = sum(1 for r in records if r.data.get("matches") is True)

    by_label: dict[str, dict] = {}
    for r in records:
        label = r.data.get("fixture_label", "?")
        if label not in by_label:
            by_label[label] = {
                "fixture_label": label,
                "expected_approved": r.data.get("expected_approved"),
                "n_correct": 0,
                "n_total": 0,
                "attempts": [],
            }
        entry = by_label[label]
        entry["n_total"] += 1
        if r.data.get("matches") is True:
            entry["n_correct"] += 1
        entry["attempts"].append({
            "repetition": r.repetition,
            "actual_approved": r.data.get("actual_approved"),
            "matches": r.data.get("matches"),
            "success": r.success,
            "error": r.error,
        })

    return AccuracyStats(
        n_correct=n_correct,
        n_total=n_total,
        accuracy=n_correct / n_total if n_total > 0 else 0.0,
        breakdown=list(by_label.values()),
    )
