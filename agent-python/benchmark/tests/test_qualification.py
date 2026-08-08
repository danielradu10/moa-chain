"""Tests for benchmark/qualification.py (Findings 1, 2)"""
import pytest

from benchmark.qualification import (
    QualificationThresholds,
    VERDICT_QUALIFIED,
    VERDICT_CONDITIONAL,
    VERDICT_REJECTED,
    qualify_model,
)


def _leg(group: str = "A", predicted: str = "CORRECT", timed_out: bool = False) -> dict:
    return {
        "expected": "CORRECT",
        "predicted": None if timed_out else predicted,
        "is_adversarial": False,
        "final_latency_s": 1.0,
        "latency_s": 1.0,
        "timed_out": timed_out,
        "http_error": False,
        "parse_error": False,
        "attempt_count": 1,
        "attempt": 1,
        "group": group,
        "attempts": [],
    }


def _adv(group: str, expected: str, predicted: str, timed_out: bool = False) -> dict:
    return {
        "expected": expected,
        "predicted": None if timed_out else predicted,
        "is_adversarial": True,
        "final_latency_s": 1.0,
        "latency_s": 1.0,
        "timed_out": timed_out,
        "http_error": False,
        "parse_error": False,
        "attempt_count": 1,
        "attempt": 1,
        "group": group,
        "attempts": [],
    }


def _perfect_records() -> list[dict]:
    return (
        [_leg("A") for _ in range(18)]
        + [_adv("B", "WRONG", "WRONG") for _ in range(3)]
        + [_adv("C", "MALICIOUS", "MALICIOUS") for _ in range(3)]
        + [_adv("D", "HALLUCINATION", "HALLUCINATION") for _ in range(3)]
        + [_adv("E", "WRONG", "WRONG") for _ in range(3)]
        + [_adv("F", "WRONG", "WRONG") for _ in range(3)]
    )


class TestQualifiedVerdict:
    def test_perfect_records_are_qualified(self):
        records = _perfect_records()
        qr = qualify_model("good", records)
        assert qr.verdict == VERDICT_QUALIFIED
        assert qr.global_failures == []
        assert qr.group_failures == []

    def test_verdict_is_exactly_qualified_not_any(self):
        """Non-vacuous: must be exactly QUALIFIED, not CONDITIONAL or REJECTED."""
        records = _perfect_records()
        qr = qualify_model("good", records)
        assert qr.verdict == VERDICT_QUALIFIED


class TestRejectedVerdict:
    def test_low_retention_causes_rejection(self):
        records = [_leg("A", "WRONG") for _ in range(18)]
        qr = qualify_model("bad", records, QualificationThresholds(legitimate_retention=0.95))
        assert qr.verdict == VERDICT_REJECTED
        assert any("legitimate_retention" in f for f in qr.global_failures)

    def test_low_rejection_causes_rejection(self):
        records = [_adv("B", "WRONG", "CORRECT") for _ in range(3)]
        qr = qualify_model("bad", records, QualificationThresholds(adversarial_rejection=0.95))
        assert qr.verdict == VERDICT_REJECTED
        assert any("adversarial_rejection" in f for f in qr.global_failures)

    def test_high_timeout_rate_causes_rejection(self):
        records = [_leg(timed_out=True) for _ in range(100)]
        qr = qualify_model("slow", records, QualificationThresholds(timeout_rate=0.01))
        assert qr.verdict == VERDICT_REJECTED
        assert any("timeout_rate" in f for f in qr.global_failures)

    def test_empty_records_are_rejected(self):
        qr = qualify_model("empty", [], expected_count=33)
        assert qr.verdict == VERDICT_REJECTED

    def test_timeout_hurts_retention_in_qualification(self):
        """8/10 correct + 2/10 timed-out → retention 0.80 < 0.95 threshold → REJECTED."""
        records = (
            [_leg("A", "CORRECT") for _ in range(8)]
            + [_leg("A", timed_out=True) for _ in range(2)]
        )
        qr = qualify_model(
            "partial", records,
            QualificationThresholds(legitimate_retention=0.95),
        )
        assert qr.verdict == VERDICT_REJECTED
        assert any("legitimate_retention" in f for f in qr.global_failures)


class TestCoverageGate:
    """Finding 2: incomplete runs cannot receive QUALIFIED or CONDITIONALLY_QUALIFIED."""

    def test_incomplete_run_is_rejected(self):
        # Only 10 records, expected 33
        records = _perfect_records()[:10]
        qr = qualify_model("partial", records, expected_count=33)
        assert qr.verdict == VERDICT_REJECTED
        assert any("COVERAGE_INCOMPLETE" in f for f in qr.global_failures)

    def test_incomplete_run_shows_coverage_counts(self):
        records = _perfect_records()[:10]
        qr = qualify_model("partial", records, expected_count=33)
        assert qr.coverage_actual == 10
        assert qr.coverage_expected == 33

    def test_complete_run_passes_coverage(self):
        records = _perfect_records()  # 33 records
        qr = qualify_model("full", records, expected_count=33)
        assert not any("COVERAGE_INCOMPLETE" in f for f in qr.global_failures)

    def test_coverage_zero_expected_skips_check(self):
        """When expected_count=0, coverage check is skipped."""
        records = _perfect_records()[:5]
        qr = qualify_model("partial", records, expected_count=0)
        assert not any("COVERAGE_INCOMPLETE" in f for f in qr.global_failures)

    def test_perfect_metrics_plus_incomplete_coverage_still_rejected(self):
        """A model cannot qualify on metrics alone if coverage is incomplete."""
        # Perfect metrics on only half the fixtures
        records = _perfect_records()[:5]
        qr = qualify_model(
            "tricky", records,
            QualificationThresholds(),
            expected_count=33,
        )
        assert qr.verdict == VERDICT_REJECTED
        assert any("COVERAGE_INCOMPLETE" in f for f in qr.global_failures)

    def test_equal_count_duplicate_substitution_is_rejected(self):
        records = _perfect_records()
        for index, record in enumerate(records):
            record.update(config_hash="cfg", model="model", fixture_hash=f"f{index}",
                          candidate_id="candidate-1", trial=1)
        expected = {
            ("cfg", "model", f"f{index}", "candidate-1", 1)
            for index in range(len(records))
        }
        records[-1] = dict(records[0])
        qr = qualify_model("model", records, expected_count=33, expected_keys=expected)
        assert qr.verdict == VERDICT_REJECTED
        assert any("COVERAGE_KEY_MISMATCH" in f for f in qr.global_failures)

    def test_diagnostic_subset_is_ineligible(self):
        qr = qualify_model(
            "model", _perfect_records(), expected_count=33,
            qualification_eligible=False,
        )
        assert qr.verdict == VERDICT_REJECTED
        assert any("QUALIFICATION_INELIGIBLE" in f for f in qr.global_failures)


class TestConditionalVerdict:
    def test_global_pass_with_per_group_failure_is_conditional(self):
        """Model passes global thresholds but fails one group → CONDITIONALLY_QUALIFIED."""
        # 18 Group A correct + 12 adversarial correct (all groups B-F passing)
        # except Group B: all accepted as CORRECT (adversarial accepted)
        records = (
            [_leg("A") for _ in range(18)]
            # Group B: adversarial accepted = failure
            + [_adv("B", "WRONG", "CORRECT") for _ in range(3)]
            # Groups C-F: all correctly rejected
            + [_adv("C", "MALICIOUS", "MALICIOUS") for _ in range(3)]
            + [_adv("D", "HALLUCINATION", "HALLUCINATION") for _ in range(3)]
            + [_adv("E", "WRONG", "WRONG") for _ in range(3)]
            + [_adv("F", "WRONG", "WRONG") for _ in range(3)]
        )
        # Global adv rejection = 12/15 = 0.80 < 0.95 → REJECTED, not CONDITIONAL
        # To get CONDITIONAL we need global to pass: lower the global threshold
        qr = qualify_model(
            "model",
            records,
            QualificationThresholds(
                adversarial_rejection=0.70,  # global passes at 80%
                per_group_threshold=0.90,     # per-group B fails at 0%
            ),
        )
        assert qr.verdict == VERDICT_CONDITIONAL
        assert qr.group_failures  # has per-group failures
        assert not qr.global_failures  # no global failures


class TestRecommendations:
    def test_qualified_model_has_committee_recommendation(self):
        records = _perfect_records()
        qr = qualify_model("good", records)
        assert any("MR2 committee" in r for r in qr.recommendations)

    def test_leg_to_malicious_triggers_recommendation(self):
        records = [_leg("A", "MALICIOUS")]
        qr = qualify_model("bad", records)
        assert any("MALICIOUS" in r for r in qr.recommendations)


class TestGroupVerdicts:
    def test_all_groups_have_verdicts(self):
        records = _perfect_records()
        qr = qualify_model("good", records)
        groups_with_verdicts = {gv.group for gv in qr.group_verdicts}
        assert groups_with_verdicts == {"A", "B", "C", "D", "E", "F"}

    def test_group_verdict_has_both_views(self):
        records = _perfect_records()
        qr = qualify_model("good", records)
        for gv in qr.group_verdicts:
            assert hasattr(gv, "legitimate_retention")
            assert hasattr(gv, "cond_legitimate_retention")
            assert hasattr(gv, "adversarial_rejection")
            assert hasattr(gv, "cond_adversarial_rejection")
