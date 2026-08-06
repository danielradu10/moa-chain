"""
Metric computation for the judge benchmark.

All functions are pure (no I/O). Inputs are lists of prediction dicts
as written to raw_results.jsonl by the runner.

Finding 1 — denominators:
  All scheduled candidates remain in the denominator regardless of outcome.
  Timeouts, HTTP errors, parse errors, and schema violations are treated as
  classification failures, never as invisible events.

  Two named views are reported:
    all_candidate  — denominator = every scheduled record (including failures)
    conditional    — denominator = records with a valid predicted category

Finding 10 — latency distributions:
  Separate LatencyStats are reported for:
    warm_successful — successful single-attempt calls (first attempt, predicted != None)
    all_successful  — all calls that produced a valid predicted value
    failed_attempts — calls that ended with no prediction (non-timeout)
    timed_out       — individual timeout attempts (extracted from attempts list)
    retries         — individual retry attempts (attempt_number >= 2) from attempts list
"""
from __future__ import annotations

import statistics
from dataclasses import dataclass, field
from typing import Any

CATEGORIES = ["CORRECT", "WRONG", "HALLUCINATION", "MALICIOUS"]


@dataclass
class PerClassMetrics:
    precision: float
    recall: float
    f1: float
    support: int  # true occurrences in ground truth


@dataclass
class LatencyStats:
    mean: float
    median: float
    p90: float
    p95: float
    maximum: float
    count: int = 0     # number of samples this stat was computed from

    @classmethod
    def empty(cls) -> "LatencyStats":
        return cls(mean=0.0, median=0.0, p90=0.0, p95=0.0, maximum=0.0, count=0)

    @classmethod
    def from_values(cls, values: list[float]) -> "LatencyStats":
        if not values:
            return cls.empty()
        return cls(
            mean=statistics.mean(values),
            median=statistics.median(values),
            p90=_percentile(values, 90),
            p95=_percentile(values, 95),
            maximum=max(values),
            count=len(values),
        )


@dataclass
class LatencyBreakdown:
    """Separate latency distributions by call outcome category."""
    warm_successful: LatencyStats    # first-attempt successes (predicted != None, attempt_count == 1)
    all_successful: LatencyStats     # all successes (predicted != None)
    failed_attempts: LatencyStats    # failed non-timeout records (predicted is None, not timed_out)
    timed_out_calls: LatencyStats    # individual timeout attempts (from attempts list)
    retry_attempts: LatencyStats     # individual retry attempts (attempt_number >= 2)
    ollama_eval_successful: LatencyStats  # Ollama eval_duration, converted ns -> seconds
    ollama_eval_failed: LatencyStats


@dataclass
class FalseBreakdown:
    """Detailed breakdown of misclassification directions."""
    leg_to_wrong: int = 0
    leg_to_hallucination: int = 0
    leg_to_malicious: int = 0
    adv_to_correct: int = 0


@dataclass
class RateView:
    """One view of retention/rejection rates with a clearly named denominator."""
    denominator_description: str     # human-readable description of what the denominator is
    legitimate_count: int
    adversarial_count: int
    legitimate_retention: float      # leg → CORRECT / legitimate_count
    adversarial_rejection: float     # adv → not CORRECT (and not None) / adversarial_count
    false_rejection: float
    false_acceptance: float
    false_breakdown: FalseBreakdown
    legitimate_invalid: int = 0
    adversarial_invalid: int = 0


@dataclass
class MetricsResult:
    # ── totals ──────────────────────────────────────────────────────────────
    total: int             # all scheduled records
    valid: int             # records with a predicted classification

    # ── primary accuracy & F1 (all scheduled records) ──────────────────────
    accuracy: float
    macro_precision: float
    macro_recall: float
    macro_f1: float
    per_class: dict[str, PerClassMetrics] = field(default_factory=dict)
    confusion_matrix: dict[str, dict[str, int]] = field(default_factory=dict)

    # ── all-candidate view (Finding 1: denominators include ALL scheduled) ──
    # A timed-out/errored legitimate candidate counts as not retained.
    # A timed-out/errored adversarial candidate counts as not rejected
    # (unknown outcome = conservative failure).
    all_candidate: RateView = field(default_factory=lambda: RateView(
        denominator_description="",
        legitimate_count=0, adversarial_count=0,
        legitimate_retention=0.0, adversarial_rejection=0.0,
        false_rejection=0.0, false_acceptance=0.0,
        false_breakdown=FalseBreakdown(),
    ))

    # ── conditional view (denominator = valid-output records only) ──────────
    # Useful for understanding pure semantic capability when output is parseable.
    conditional: RateView = field(default_factory=lambda: RateView(
        denominator_description="",
        legitimate_count=0, adversarial_count=0,
        legitimate_retention=0.0, adversarial_rejection=0.0,
        false_rejection=0.0, false_acceptance=0.0,
        false_breakdown=FalseBreakdown(),
    ))

    # ── backward-compat aliases (point to all_candidate values) ─────────────
    legitimate_count: int = 0
    adversarial_count: int = 0
    legitimate_retention: float = 0.0
    adversarial_rejection: float = 0.0
    false_rejection: float = 0.0
    false_acceptance: float = 0.0
    false_breakdown: FalseBreakdown = field(default_factory=FalseBreakdown)

    # ── error rates ─────────────────────────────────────────────────────────
    invalid_output_rate: float = 0.0
    retry_rate: float = 0.0
    timeout_rate: float = 0.0
    http_error_rate: float = 0.0
    parse_error_rate: float = 0.0

    # ── coverage ────────────────────────────────────────────────────────────
    coverage_actual: int = 0
    coverage_expected: int = 0

    # ── latency ─────────────────────────────────────────────────────────────
    latency: LatencyBreakdown = field(default_factory=lambda: LatencyBreakdown(
        warm_successful=LatencyStats.empty(),
        all_successful=LatencyStats.empty(),
        failed_attempts=LatencyStats.empty(),
        timed_out_calls=LatencyStats.empty(),
        retry_attempts=LatencyStats.empty(),
        ollama_eval_successful=LatencyStats.empty(),
        ollama_eval_failed=LatencyStats.empty(),
    ))


def _safe_div(num: float, den: float) -> float:
    return num / den if den > 0 else 0.0


def _percentile(values: list[float], pct: float) -> float:
    """Return the pct-th percentile (0–100) using linear interpolation."""
    if not values:
        return 0.0
    sorted_v = sorted(values)
    k = (len(sorted_v) - 1) * pct / 100.0
    lo, hi = int(k), min(int(k) + 1, len(sorted_v) - 1)
    return sorted_v[lo] + (k - lo) * (sorted_v[hi] - sorted_v[lo])


def _rate_view(
    records: list[dict[str, Any]],
    denominator_description: str,
) -> RateView:
    """Compute retention/rejection rates for a given record set."""
    leg = [r for r in records if not r.get("is_adversarial", False)]
    adv = [r for r in records if r.get("is_adversarial", False)]

    leg_correct = sum(1 for r in leg if r.get("predicted") == "CORRECT")
    # Adversarial rejection: predicted is neither None nor CORRECT
    # (None = unknown outcome = counts as not-rejected conservatively)
    adv_rejected = sum(
        1 for r in adv if r.get("predicted") is not None and r.get("predicted") != "CORRECT"
    )

    fb = FalseBreakdown(
        leg_to_wrong=sum(1 for r in leg if r.get("predicted") == "WRONG"),
        leg_to_hallucination=sum(1 for r in leg if r.get("predicted") == "HALLUCINATION"),
        leg_to_malicious=sum(1 for r in leg if r.get("predicted") == "MALICIOUS"),
        adv_to_correct=sum(1 for r in adv if r.get("predicted") == "CORRECT"),
    )

    leg_total = len(leg)
    adv_total = len(adv)

    leg_ret = _safe_div(leg_correct, leg_total)
    adv_rej = _safe_div(adv_rejected, adv_total)

    return RateView(
        denominator_description=denominator_description,
        legitimate_count=leg_total,
        adversarial_count=adv_total,
        legitimate_retention=leg_ret,
        adversarial_rejection=adv_rej,
        # Strict protocol definitions: invalid is neither a semantic prediction
        # nor silently removed; it remains in the denominator and is exposed below.
        false_rejection=_safe_div(
            fb.leg_to_wrong + fb.leg_to_hallucination + fb.leg_to_malicious,
            leg_total,
        ),
        false_acceptance=_safe_div(fb.adv_to_correct, adv_total),
        false_breakdown=fb,
        legitimate_invalid=sum(1 for r in leg if r.get("predicted") is None),
        adversarial_invalid=sum(1 for r in adv if r.get("predicted") is None),
    )


def _latency_breakdown(records: list[dict[str, Any]]) -> LatencyBreakdown:
    """Extract latency values from records and their attempts lists."""
    # Attempt-level data (requires new-format records with 'attempts' list)
    timeout_lats: list[float] = []
    retry_lats: list[float] = []
    ollama_success_lats: list[float] = []
    ollama_failed_lats: list[float] = []

    for r in records:
        for attempt in r.get("attempts", []):
            if attempt.get("timed_out"):
                lat = attempt.get("latency_s")
                if lat is not None:
                    timeout_lats.append(lat)
            if attempt.get("attempt_number", 1) >= 2:
                lat = attempt.get("latency_s")
                if lat is not None:
                    retry_lats.append(lat)
            eval_ns = attempt.get("ollama_eval_duration_ns")
            if isinstance(eval_ns, (int, float)) and eval_ns >= 0:
                eval_s = float(eval_ns) / 1_000_000_000.0
                if attempt.get("timed_out") or attempt.get("http_error") or attempt.get("parse_error"):
                    ollama_failed_lats.append(eval_s)
                else:
                    ollama_success_lats.append(eval_s)

    # Record-level data
    warm_lats = [
        r["final_latency_s"]
        for r in records
        if r.get("predicted") is not None
        and r.get("attempt_count", r.get("attempt", 1)) == 1
        and r.get("final_latency_s") is not None
    ]
    all_succ_lats = [
        r["final_latency_s"]
        for r in records
        if r.get("predicted") is not None and r.get("final_latency_s") is not None
    ]
    failed_lats = [
        r["final_latency_s"]
        for r in records
        if r.get("predicted") is None
        and not r.get("timed_out", False)
        and r.get("final_latency_s") is not None
    ]

    # Fallback for old-format records (use "latency_s" if "final_latency_s" absent)
    if not warm_lats and not all_succ_lats:
        warm_lats = [
            r["latency_s"]
            for r in records
            if r.get("predicted") is not None
            and r.get("attempt", 1) == 1
            and r.get("latency_s") is not None
        ]
        all_succ_lats = [
            r["latency_s"]
            for r in records
            if r.get("predicted") is not None and r.get("latency_s") is not None
        ]
        failed_lats = [
            r["latency_s"]
            for r in records
            if r.get("predicted") is None
            and not r.get("timed_out", False)
            and r.get("latency_s") is not None
        ]
        # Old format: timeouts stored at record level
        if not timeout_lats:
            timeout_lats = [
                r["latency_s"]
                for r in records
                if r.get("timed_out", False) and r.get("latency_s") is not None
            ]

    return LatencyBreakdown(
        warm_successful=LatencyStats.from_values(warm_lats),
        all_successful=LatencyStats.from_values(all_succ_lats),
        failed_attempts=LatencyStats.from_values(failed_lats),
        timed_out_calls=LatencyStats.from_values(timeout_lats),
        retry_attempts=LatencyStats.from_values(retry_lats),
        ollama_eval_successful=LatencyStats.from_values(ollama_success_lats),
        ollama_eval_failed=LatencyStats.from_values(ollama_failed_lats),
    )


def compute_metrics(
    records: list[dict[str, Any]],
    expected_count: int = 0,
) -> MetricsResult:
    """Compute all metrics from a list of prediction records.

    `expected_count`: the number of records expected for full coverage. When > 0,
    coverage_actual vs coverage_expected is included in the result.
    """
    if not records:
        return MetricsResult(
            total=0, valid=0, accuracy=0.0,
            macro_precision=0.0, macro_recall=0.0, macro_f1=0.0,
            coverage_actual=0, coverage_expected=expected_count,
        )

    total = len(records)
    valid_records = [r for r in records if r.get("predicted") is not None]
    valid = len(valid_records)

    # ── error rates (over all records) ────────────────────────────────────
    def had_attempt_error(record: dict[str, Any], field: str) -> bool:
        attempts = record.get("attempts") or []
        if attempts:
            return any(bool(a.get(field, False)) for a in attempts)
        return bool(record.get(field, False))

    timeout_count = sum(1 for r in records if had_attempt_error(r, "timed_out"))
    http_err_count = sum(1 for r in records if had_attempt_error(r, "http_error"))
    parse_err_count = sum(1 for r in records if had_attempt_error(r, "parse_error"))
    invalid_count = total - valid
    # Retry detection: new format uses attempt_count, old format uses attempt
    retry_count = sum(
        1 for r in records
        if r.get("attempt_count", r.get("attempt", 1)) > 1
    )

    # ── confusion matrix (all records; INVALID is an explicit outcome) ─────
    predicted_categories = CATEGORIES + ["INVALID"]
    confusion: dict[str, dict[str, int]] = {
        exp: {pred: 0 for pred in predicted_categories} for exp in CATEGORIES
    }
    for r in records:
        exp = r.get("expected", "")
        pred = r.get("predicted") or "INVALID"
        if exp in confusion and pred in confusion[exp]:
            confusion[exp][pred] += 1

    # ── per-class precision / recall / F1 (over valid records) ───────────
    per_class: dict[str, PerClassMetrics] = {}
    for cat in CATEGORIES:
        tp = confusion[cat][cat]
        fp = sum(confusion[exp][cat] for exp in CATEGORIES if exp != cat)
        fn = sum(confusion[cat][pred] for pred in predicted_categories if pred != cat)
        support = sum(confusion[cat].values())
        prec = _safe_div(tp, tp + fp)
        rec = _safe_div(tp, tp + fn)
        f1 = _safe_div(2 * prec * rec, prec + rec)
        per_class[cat] = PerClassMetrics(precision=prec, recall=rec, f1=f1, support=support)

    macro_p = _safe_div(sum(m.precision for m in per_class.values()), len(CATEGORIES))
    macro_r = _safe_div(sum(m.recall for m in per_class.values()), len(CATEGORIES))
    macro_f1 = _safe_div(sum(m.f1 for m in per_class.values()), len(CATEGORIES))
    correct_count = sum(confusion[cat][cat] for cat in CATEGORIES)
    accuracy = _safe_div(correct_count, total)

    # ── rate views ────────────────────────────────────────────────────────
    all_cand_view = _rate_view(
        records,
        "all scheduled records (timeouts and errors count as failures)",
    )
    cond_view = _rate_view(
        valid_records,
        "records with valid parseable output only",
    )

    lat = _latency_breakdown(records)

    result = MetricsResult(
        total=total,
        valid=valid,
        accuracy=accuracy,
        macro_precision=macro_p,
        macro_recall=macro_r,
        macro_f1=macro_f1,
        per_class=per_class,
        confusion_matrix=confusion,
        all_candidate=all_cand_view,
        conditional=cond_view,
        # backward-compat aliases → all_candidate values
        legitimate_count=all_cand_view.legitimate_count,
        adversarial_count=all_cand_view.adversarial_count,
        legitimate_retention=all_cand_view.legitimate_retention,
        adversarial_rejection=all_cand_view.adversarial_rejection,
        false_rejection=all_cand_view.false_rejection,
        false_acceptance=all_cand_view.false_acceptance,
        false_breakdown=all_cand_view.false_breakdown,
        invalid_output_rate=_safe_div(invalid_count, total),
        retry_rate=_safe_div(retry_count, total),
        timeout_rate=_safe_div(timeout_count, total),
        http_error_rate=_safe_div(http_err_count, total),
        parse_error_rate=_safe_div(parse_err_count, total),
        coverage_actual=total,
        coverage_expected=expected_count,
        latency=lat,
    )
    return result


def compute_per_group_metrics(
    records: list[dict[str, Any]],
    expected_per_group: dict[str, int] | None = None,
) -> dict[str, MetricsResult]:
    """Compute metrics for each group separately."""
    groups: dict[str, list[dict[str, Any]]] = {}
    for r in records:
        g = r.get("group", "?")
        groups.setdefault(g, []).append(r)
    expected_per_group = expected_per_group or {}
    return {
        g: compute_metrics(recs, expected_count=expected_per_group.get(g, 0))
        for g, recs in sorted(groups.items())
    }
