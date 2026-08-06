"""Tests for benchmark/cross_model.py (Finding 11)"""
import pytest

from benchmark.cross_model import (
    compute_pairwise_agreement,
    find_shared_errors,
)


def _rec(
    model: str,
    group: str = "A",
    tx_id: str = "tx1",
    perspective: str = "p1",
    expected: str = "CORRECT",
    predicted: str | None = "CORRECT",
    is_adversarial: bool = False,
    trial: int = 1,
) -> dict:
    return {
        "model": model,
        "group": group,
        "tx_id": tx_id,
        "perspective": perspective,
        "expected": expected,
        "predicted": predicted,
        "is_adversarial": is_adversarial,
        "candidate_id": "candidate-1",
        "trial": trial,
    }


class TestPairwiseAgreement:
    def test_single_model_returns_empty(self):
        assert compute_pairwise_agreement({"m1": [_rec("m1")]}) == []

    def test_perfect_agreement(self):
        records = {
            "m1": [_rec("m1", predicted="CORRECT")],
            "m2": [_rec("m2", predicted="CORRECT")],
        }
        result = compute_pairwise_agreement(records)
        assert len(result) == 1
        assert result[0].agreement_rate == pytest.approx(1.0)
        assert result[0].total_shared == 1

    def test_zero_agreement(self):
        records = {
            "m1": [_rec("m1", predicted="CORRECT")],
            "m2": [_rec("m2", predicted="WRONG")],
        }
        result = compute_pairwise_agreement(records)
        assert result[0].agreement_rate == pytest.approx(0.0)

    def test_no_shared_keys(self):
        records = {
            "m1": [_rec("m1", tx_id="tx1")],
            "m2": [_rec("m2", tx_id="tx2")],
        }
        result = compute_pairwise_agreement(records)
        assert result[0].total_shared == 0
        assert result[0].agreement_count == 0

    def test_shared_false_acceptance(self):
        records = {
            "m1": [_rec("m1", expected="WRONG", predicted="CORRECT", is_adversarial=True)],
            "m2": [_rec("m2", expected="WRONG", predicted="CORRECT", is_adversarial=True)],
        }
        result = compute_pairwise_agreement(records)
        assert result[0].shared_false_acceptances == 1

    def test_three_models_three_pairs(self):
        records = {
            "m1": [_rec("m1")],
            "m2": [_rec("m2")],
            "m3": [_rec("m3", predicted="WRONG")],
        }
        assert len(compute_pairwise_agreement(records)) == 3

    def test_records_with_no_prediction_excluded_from_agreement(self):
        """Records with predicted=None (timeout/error) are not counted for pairwise."""
        records = {
            "m1": [_rec("m1", predicted=None)],
            "m2": [_rec("m2", predicted="CORRECT")],
        }
        result = compute_pairwise_agreement(records)
        assert result[0].total_shared == 0


class TestFindSharedErrors:
    def test_failed_by_all_requires_all_declared_models(self):
        """Finding 11: 'failed by all' must use the declared model set."""
        records = {
            "m1": [_rec("m1", predicted="WRONG")],
            "m2": [_rec("m2", predicted="WRONG")],
        }
        # With default (only records_by_model keys), both models failed → failed_by_all
        analysis = find_shared_errors(records, declared_models=["m1", "m2"])
        assert len(analysis.failed_by_all) == 1

    def test_failed_by_all_excludes_when_one_model_correct(self):
        records = {
            "m1": [_rec("m1", predicted="WRONG")],
            "m2": [_rec("m2", predicted="CORRECT")],
        }
        analysis = find_shared_errors(records, declared_models=["m1", "m2"])
        assert len(analysis.failed_by_all) == 0

    def test_failed_by_all_requires_undeclared_model_to_not_count(self):
        """Declared m3 has no records for tx1 → that fixture is NOT failed_by_all."""
        records = {
            "m1": [_rec("m1", predicted="WRONG")],
            "m2": [_rec("m2", predicted="WRONG")],
        }
        # m3 is declared but has no records: report coverage, never a semantic error.
        analysis = find_shared_errors(records, declared_models=["m1", "m2", "m3"])
        assert analysis.failed_by_all == []
        assert analysis.coverage_gaps["m3"] == 1

    def test_failed_by_majority(self):
        """2/3 models wrong → failed by majority."""
        records = {
            "m1": [_rec("m1", predicted="WRONG")],
            "m2": [_rec("m2", predicted="WRONG")],
            "m3": [_rec("m3", predicted="CORRECT")],
        }
        analysis = find_shared_errors(records, declared_models=["m1", "m2", "m3"])
        assert len(analysis.failed_by_majority) == 1
        assert len(analysis.failed_by_all) == 0

    def test_model_specific_failure(self):
        records = {
            "m1": [_rec("m1", predicted="WRONG")],
            "m2": [_rec("m2", predicted="CORRECT")],
            "m3": [_rec("m3", predicted="CORRECT")],
        }
        analysis = find_shared_errors(records, declared_models=["m1", "m2", "m3"])
        assert len(analysis.model_specific_failures["m1"]) == 1
        assert len(analysis.model_specific_failures["m2"]) == 0

    def test_missing_predictions_reported_separately(self):
        """Records with predicted=None are counted as missing, not wrong."""
        records = {
            "m1": [_rec("m1", predicted=None)],
            "m2": [_rec("m2", predicted="CORRECT")],
        }
        analysis = find_shared_errors(records, declared_models=["m1", "m2"])
        assert len(analysis.missing_predictions["m1"]) == 1
        assert len(analysis.missing_predictions["m2"]) == 0

    def test_coverage_gaps_reported(self):
        records = {
            "m1": [_rec("m1", predicted=None)],
            "m2": [_rec("m2", predicted="CORRECT")],
        }
        analysis = find_shared_errors(records, declared_models=["m1", "m2"])
        assert analysis.coverage_gaps["m1"] == 1
        assert analysis.coverage_gaps["m2"] == 0

    def test_no_errors(self):
        records = {
            "m1": [_rec("m1", predicted="CORRECT")],
            "m2": [_rec("m2", predicted="CORRECT")],
        }
        analysis = find_shared_errors(records, declared_models=["m1", "m2"])
        assert analysis.failed_by_all == []
        assert analysis.failed_by_majority == []
        assert all(len(v) == 0 for v in analysis.model_specific_failures.values())

    def test_single_model_returns_empty(self):
        records = {"m1": [_rec("m1", predicted="WRONG")]}
        analysis = find_shared_errors(records, declared_models=["m1"])
        assert analysis.failed_by_all == []
        assert analysis.pairwise == []

    def test_declared_models_in_result(self):
        records = {"m1": [_rec("m1")], "m2": [_rec("m2")]}
        analysis = find_shared_errors(records, declared_models=["m1", "m2"])
        assert analysis.declared_models == ["m1", "m2"]
