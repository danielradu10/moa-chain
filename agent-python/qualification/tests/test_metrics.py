"""Tests for pure metric functions."""
from __future__ import annotations

import math
import pytest
from dataclasses import dataclass, field
from typing import Any


# Minimal stub that mimics CallRecord for metric tests without importing harness.
@dataclass
class _Rec:
    operation: str
    repetition: int
    started_at: float
    duration_ms: float
    success: bool
    error: str | None
    input_tokens: int | None
    output_tokens: int | None
    total_tokens: int | None
    data: dict = field(default_factory=dict)


from qualification.metrics import (
    AccuracyStats,
    LatencyStats,
    TokenTotals,
    compute_latency,
    compute_tokens,
    eval_accuracy,
    judge_accuracy,
    operation_summary,
)


def _rec(duration_ms=100.0, success=True, in_tok=10, out_tok=20, total_tok=30, data=None):
    return _Rec(
        operation="label",
        repetition=1,
        started_at=0.0,
        duration_ms=duration_ms,
        success=success,
        error=None if success else "err",
        input_tokens=in_tok,
        output_tokens=out_tok,
        total_tokens=total_tok,
        data=data or {},
    )


# ── compute_latency ───────────────────────────────────────────────────────────

def test_compute_latency_none_on_empty():
    assert compute_latency([]) is None


def test_compute_latency_single():
    stats = compute_latency([_rec(duration_ms=200.0)])
    assert stats is not None
    assert stats.count == 1
    assert stats.min_ms == 200.0
    assert stats.max_ms == 200.0
    assert math.isclose(stats.mean_ms, 200.0)
    assert math.isclose(stats.median_ms, 200.0)
    assert math.isclose(stats.p90_ms, 200.0)
    assert math.isclose(stats.p95_ms, 200.0)


def test_compute_latency_percentiles():
    durations = [100.0, 200.0, 300.0, 400.0, 500.0, 600.0, 700.0, 800.0, 900.0, 1000.0]
    records = [_rec(duration_ms=d) for d in durations]
    stats = compute_latency(records)
    assert stats is not None
    assert stats.min_ms == 100.0
    assert stats.max_ms == 1000.0
    # p90 of 10 evenly-spaced values: k=(10-1)*90/100 = 8.1 → sorted[8]+0.1*(sorted[9]-sorted[8])
    # = 900 + 0.1 * 100 = 910
    assert math.isclose(stats.p90_ms, 910.0, rel_tol=1e-6)
    # p95: k=9*95/100=8.55 → 900 + 0.55*100 = 955
    assert math.isclose(stats.p95_ms, 955.0, rel_tol=1e-6)


def test_compute_latency_includes_failed_records():
    records = [_rec(duration_ms=50.0, success=True), _rec(duration_ms=5000.0, success=False)]
    stats = compute_latency(records)
    assert stats is not None
    assert stats.count == 2
    assert stats.max_ms == 5000.0


# ── compute_tokens ────────────────────────────────────────────────────────────

def test_compute_tokens_sums():
    records = [_rec(in_tok=10, out_tok=20, total_tok=30), _rec(in_tok=5, out_tok=15, total_tok=20)]
    tt = compute_tokens(records)
    assert tt.total_input == 15
    assert tt.total_output == 35
    assert tt.total_all == 50


def test_compute_tokens_none_when_missing():
    records = [_rec(in_tok=None, out_tok=None, total_tok=None)]
    tt = compute_tokens(records)
    assert tt.total_input is None
    assert tt.total_output is None
    assert tt.total_all is None


def test_compute_tokens_partial():
    records = [_rec(in_tok=10, out_tok=None, total_tok=None), _rec(in_tok=5, out_tok=None, total_tok=None)]
    tt = compute_tokens(records)
    assert tt.total_input == 15
    assert tt.total_output is None


# ── operation_summary ─────────────────────────────────────────────────────────

def test_operation_summary_success_rate():
    records = [_rec(success=True), _rec(success=True), _rec(success=False)]
    summary = operation_summary(records)
    assert summary.n_calls == 3
    assert summary.n_success == 2
    assert math.isclose(summary.success_rate, 2 / 3)


def test_operation_summary_all_success():
    records = [_rec() for _ in range(5)]
    summary = operation_summary(records)
    assert summary.success_rate == 1.0


def test_operation_summary_empty():
    summary = operation_summary([])
    assert summary.n_calls == 0
    assert summary.success_rate == 0.0
    assert summary.latency is None


# ── judge_accuracy ────────────────────────────────────────────────────────────

def _judge_rec(label, expected, actual, matches, rep=1):
    return _Rec(
        operation="judge",
        repetition=rep,
        started_at=0.0,
        duration_ms=100.0,
        success=True,
        error=None,
        input_tokens=None,
        output_tokens=None,
        total_tokens=None,
        data={"fixture_label": label, "expected": expected, "actual": actual, "matches": matches},
    )


def test_judge_accuracy_perfect():
    records = [
        _judge_rec("correct", "CORRECT", "CORRECT", True),
        _judge_rec("wrong", "WRONG", "WRONG", True),
        _judge_rec("hallucination", "HALLUCINATION", "HALLUCINATION", True),
        _judge_rec("malicious", "MALICIOUS", "MALICIOUS", True),
    ]
    acc = judge_accuracy(records)
    assert acc.n_correct == 4
    assert acc.n_total == 4
    assert math.isclose(acc.accuracy, 1.0)


def test_judge_accuracy_partial():
    records = [
        _judge_rec("correct", "CORRECT", "CORRECT", True),
        _judge_rec("wrong", "WRONG", "CORRECT", False),  # misclassified
    ]
    acc = judge_accuracy(records)
    assert acc.n_correct == 1
    assert acc.n_total == 2
    assert math.isclose(acc.accuracy, 0.5)


def test_judge_accuracy_breakdown_by_label():
    records = [
        _judge_rec("correct", "CORRECT", "CORRECT", True, rep=1),
        _judge_rec("correct", "CORRECT", "WRONG", False, rep=2),
    ]
    acc = judge_accuracy(records)
    breakdown = {e["fixture_label"]: e for e in acc.breakdown}
    assert breakdown["correct"]["n_correct"] == 1
    assert breakdown["correct"]["n_total"] == 2
    assert len(breakdown["correct"]["attempts"]) == 2


# ── eval_accuracy ─────────────────────────────────────────────────────────────

def _eval_rec(label, expected_approved, actual_approved, matches, rep=1):
    return _Rec(
        operation="evaluate_synthesis",
        repetition=rep,
        started_at=0.0,
        duration_ms=100.0,
        success=True,
        error=None,
        input_tokens=None,
        output_tokens=None,
        total_tokens=None,
        data={
            "fixture_label": label,
            "expected_approved": expected_approved,
            "actual_approved": actual_approved,
            "matches": matches,
        },
    )


def test_eval_accuracy_perfect():
    records = [
        _eval_rec("correct_synthesis", True, True, True),
        _eval_rec("incorrect_synthesis", False, False, True),
    ]
    acc = eval_accuracy(records)
    assert acc.n_correct == 2
    assert acc.n_total == 2
    assert math.isclose(acc.accuracy, 1.0)


def test_eval_accuracy_all_wrong():
    records = [
        _eval_rec("correct_synthesis", True, False, False),
        _eval_rec("incorrect_synthesis", False, True, False),
    ]
    acc = eval_accuracy(records)
    assert acc.n_correct == 0
    assert math.isclose(acc.accuracy, 0.0)
