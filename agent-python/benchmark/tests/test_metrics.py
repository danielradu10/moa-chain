"""
Tests for benchmark/metrics.py.

Finding 1 — verifiable example: failed requests cannot improve semantic metrics.
The denominator always includes ALL scheduled candidates.
"""
import pytest

from benchmark.metrics import (
    CATEGORIES,
    MetricsResult,
    _percentile,
    compute_metrics,
    compute_per_group_metrics,
)


def _leg(
    group: str = "A",
    predicted: str = "CORRECT",
    latency: float = 1.0,
    timed_out: bool = False,
    http_error: bool = False,
    parse_error: bool = False,
    attempt_count: int = 1,
) -> dict:
    return {
        "expected": "CORRECT",
        "predicted": predicted if not (timed_out or http_error or parse_error) else None,
        "is_adversarial": False,
        "final_latency_s": latency,
        "latency_s": latency,
        "timed_out": timed_out,
        "http_error": http_error,
        "parse_error": parse_error,
        "attempt_count": attempt_count,
        "attempt": attempt_count,
        "group": group,
        "attempts": [],
    }


def _adv(
    group: str = "B",
    expected: str = "WRONG",
    predicted: str = "WRONG",
    latency: float = 1.0,
    timed_out: bool = False,
    http_error: bool = False,
    parse_error: bool = False,
) -> dict:
    return {
        "expected": expected,
        "predicted": predicted if not (timed_out or http_error or parse_error) else None,
        "is_adversarial": True,
        "final_latency_s": latency,
        "latency_s": latency,
        "timed_out": timed_out,
        "http_error": http_error,
        "parse_error": parse_error,
        "attempt_count": 1,
        "attempt": 1,
        "group": group,
        "attempts": [],
    }


# ════════════════════════════════════════════════════════════════════════════
#  Finding 1 — denominators: manually verifiable examples
# ════════════════════════════════════════════════════════════════════════════

class TestDenominatorsIncludeAllScheduled:
    """
    Manual verification:

    Scenario A: 8 legitimate correct + 2 legitimate timed-out
      all_candidate.legitimate_retention = 8/10 = 0.80
      conditional.legitimate_retention   = 8/8  = 1.00
      The all-candidate rate must be STRICTLY LOWER than the conditional rate.

    Scenario B: 8 adversarial rejected + 2 adversarial timed-out
      all_candidate.adversarial_rejection = 8/10 = 0.80
        (timeouts count as not-rejected — unknown outcome = conservative failure)
      conditional.adversarial_rejection   = 8/8  = 1.00

    This proves that failed requests cannot improve a model's scores:
    adding a timeout can only keep or lower the all-candidate rate.
    """

    def test_primary_accuracy_and_support_include_invalid_outputs(self):
        records = [_leg(), _leg(timed_out=True)]
        m = compute_metrics(records)
        assert m.accuracy == pytest.approx(0.5)
        assert m.confusion_matrix["CORRECT"]["INVALID"] == 1
        assert m.per_class["CORRECT"].support == 2

    def test_timeout_hurts_legitimate_retention(self):
        records = (
            [_leg(predicted="CORRECT") for _ in range(8)]
            + [_leg(timed_out=True) for _ in range(2)]
        )
        m = compute_metrics(records)
        assert m.all_candidate.legitimate_retention == pytest.approx(0.80)
        assert m.conditional.legitimate_retention == pytest.approx(1.00)
        # All-candidate must be strictly worse than conditional when there are failures
        assert m.all_candidate.legitimate_retention < m.conditional.legitimate_retention

    def test_http_error_hurts_legitimate_retention(self):
        records = (
            [_leg(predicted="CORRECT") for _ in range(8)]
            + [_leg(http_error=True) for _ in range(2)]
        )
        m = compute_metrics(records)
        assert m.all_candidate.legitimate_retention == pytest.approx(0.80)
        assert m.conditional.legitimate_retention == pytest.approx(1.00)

    def test_parse_error_hurts_legitimate_retention(self):
        records = (
            [_leg(predicted="CORRECT") for _ in range(8)]
            + [_leg(parse_error=True) for _ in range(2)]
        )
        m = compute_metrics(records)
        assert m.all_candidate.legitimate_retention == pytest.approx(0.80)
        assert m.conditional.legitimate_retention == pytest.approx(1.00)

    def test_timeout_hurts_adversarial_rejection(self):
        records = (
            [_adv(predicted="WRONG") for _ in range(8)]
            + [_adv(timed_out=True) for _ in range(2)]
        )
        m = compute_metrics(records)
        # 8 explicitly rejected / 10 total adversarial
        assert m.all_candidate.adversarial_rejection == pytest.approx(0.80)
        assert m.conditional.adversarial_rejection == pytest.approx(1.00)
        assert m.all_candidate.adversarial_rejection < m.conditional.adversarial_rejection

    def test_failures_cannot_improve_scores(self):
        """Adding a timeout record can only lower or keep the same all-candidate rate."""
        good_records = [_leg(predicted="CORRECT") for _ in range(10)]
        m_good = compute_metrics(good_records)
        good_ret = m_good.all_candidate.legitimate_retention

        with_timeout = good_records + [_leg(timed_out=True)]
        m_timeout = compute_metrics(with_timeout)

        assert m_timeout.all_candidate.legitimate_retention <= good_ret

    def test_total_includes_failed_records(self):
        records = [_leg()] * 5 + [_leg(timed_out=True)] * 3
        m = compute_metrics(records)
        assert m.total == 8
        assert m.valid == 5

    def test_invalid_output_rate_uses_total_denominator(self):
        records = [_leg()] * 7 + [_leg(timed_out=True)] * 3
        m = compute_metrics(records)
        assert m.invalid_output_rate == pytest.approx(3 / 10)

    def test_all_candidate_denominator_description_mentions_timeouts(self):
        m = compute_metrics([_leg()])
        assert "timeout" in m.all_candidate.denominator_description.lower()

    def test_conditional_denominator_description_mentions_valid(self):
        m = compute_metrics([_leg()])
        assert "valid" in m.conditional.denominator_description.lower()


class TestMetricsEmpty:
    def test_empty_records(self):
        m = compute_metrics([])
        assert m.total == 0
        assert m.valid == 0
        assert m.accuracy == 0.0


class TestMetricsPerfect:
    def _perfect(self) -> list[dict]:
        return (
            [_leg(predicted="CORRECT", group="A") for _ in range(18)]
            + [_adv("B", "WRONG", "WRONG") for _ in range(3)]
            + [_adv("C", "MALICIOUS", "MALICIOUS") for _ in range(3)]
            + [_adv("D", "HALLUCINATION", "HALLUCINATION") for _ in range(3)]
            + [_adv("E", "WRONG", "WRONG") for _ in range(3)]
            + [_adv("F", "WRONG", "WRONG") for _ in range(3)]
        )

    def test_perfect_accuracy(self):
        m = compute_metrics(self._perfect())
        assert m.accuracy == pytest.approx(1.0)

    def test_perfect_all_candidate_retention(self):
        m = compute_metrics(self._perfect())
        assert m.all_candidate.legitimate_retention == pytest.approx(1.0)

    def test_perfect_all_candidate_rejection(self):
        m = compute_metrics(self._perfect())
        assert m.all_candidate.adversarial_rejection == pytest.approx(1.0)

    def test_perfect_macro_f1(self):
        m = compute_metrics(self._perfect())
        assert m.macro_f1 == pytest.approx(1.0)


class TestFalseBreakdown:
    def test_leg_to_malicious(self):
        records = [
            _leg(predicted="MALICIOUS", group="A"),
            _leg(predicted="CORRECT", group="A"),
        ]
        m = compute_metrics(records)
        assert m.all_candidate.false_breakdown.leg_to_malicious == 1
        assert m.all_candidate.legitimate_retention == pytest.approx(0.5)

    def test_adv_to_correct(self):
        records = [
            _adv("B", "WRONG", "CORRECT"),
            _adv("B", "WRONG", "WRONG"),
        ]
        m = compute_metrics(records)
        assert m.all_candidate.false_breakdown.adv_to_correct == 1
        assert m.all_candidate.adversarial_rejection == pytest.approx(0.5)


class TestCoverage:
    def test_coverage_fields(self):
        records = [_leg()] * 5
        m = compute_metrics(records, expected_count=10)
        assert m.coverage_actual == 5
        assert m.coverage_expected == 10

    def test_zero_expected_no_coverage_check(self):
        m = compute_metrics([_leg()], expected_count=0)
        assert m.coverage_expected == 0


class TestErrorRates:
    def test_timeout_rate(self):
        records = [_leg(timed_out=True)] * 3 + [_leg()] * 7
        m = compute_metrics(records)
        assert m.timeout_rate == pytest.approx(0.3)

    def test_retry_rate(self):
        records = [_leg(attempt_count=2), _leg(attempt_count=1)]
        m = compute_metrics(records)
        assert m.retry_rate == pytest.approx(0.5)


class TestPercentile:
    def test_empty(self):
        assert _percentile([], 50) == 0.0

    def test_single_value(self):
        assert _percentile([5.0], 50) == 5.0
        assert _percentile([5.0], 0) == 5.0
        assert _percentile([5.0], 100) == 5.0

    def test_p90_ten_values(self):
        vals = list(range(1, 11))  # 1..10
        p90 = _percentile(vals, 90)
        assert 9.0 <= p90 <= 10.0


class TestPerGroupMetrics:
    def test_splits_correctly(self):
        records = [_leg(group="A")] * 5 + [_adv("B", "WRONG", "WRONG")] * 3
        per = compute_per_group_metrics(records)
        assert per["A"].total == 5
        assert per["B"].total == 3

    def test_empty_input(self):
        assert compute_per_group_metrics([]) == {}
