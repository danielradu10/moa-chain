import json
import sys
from datetime import datetime
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from distributed_mr2_qualified import (  # noqa: E402
    EXPECTED_MODELS,
    MATRIX,
    build_schedule,
    classify_trial,
    summarize,
    unique_experiment_root,
    validate_qualified_config,
)


def qualified_config() -> dict:
    return {
        "committeeStrategy": "full",
        "agents": [
            {
                "machine": f"moa-chain-{index}",
                "url": f"http://moa-chain-{index}:8081",
                "temperature": 0.0,
                "model": model,
            }
            for index, model in enumerate(EXPECTED_MODELS)
        ],
    }


def test_qualified_assignment_is_exact() -> None:
    validate_qualified_config(qualified_config())
    assert EXPECTED_MODELS.count("qwen3.5:9b") == 6
    assert EXPECTED_MODELS.count("gemma4:12b") == 4


def test_old_or_reordered_model_is_rejected() -> None:
    config = qualified_config()
    config["agents"][0]["model"] = "qwen2.5:7b"
    with pytest.raises(ValueError, match="assignment mismatch"):
        validate_qualified_config(config)


def test_complete_matrix_and_trial_multiplier() -> None:
    expected = [
        ("a", 0), ("b", 0), ("c", 0),
        ("d", 1), ("d", 2), ("d", 3),
        ("e", 1), ("e", 2), ("f", 1), ("f", 2),
    ]
    assert [(entry.group, entry.q) for entry in MATRIX] == expected
    schedule = build_schedule(10)
    assert len(schedule) == 100
    assert all(sum(item[0] == entry for item in schedule) == 10 for entry in MATRIX)


def test_unique_root_never_overwrites(tmp_path: Path) -> None:
    now = datetime(2026, 8, 7, 0, 15, 0)
    first = unique_experiment_root(tmp_path, now)
    first.mkdir()
    second = unique_experiment_root(tmp_path, now)
    assert first != second
    assert second.name.endswith("-01")


def test_non_finalized_is_protocol_result_even_when_go_test_fails() -> None:
    status, _ = classify_trial(
        {"finalized": False}, 1, "--- FAIL", collection_ok=True, setup_ok=True,
    )
    assert status == "NON_FINALIZED"


def test_missing_result_is_test_failure() -> None:
    status, _ = classify_trial(None, 1, "compile failed", True, True)
    assert status == "TEST_FAILED"


def test_log_loss_is_infrastructure_error() -> None:
    status, _ = classify_trial({"finalized": True}, 0, "PASS", False, True)
    assert status == "INFRASTRUCTURE_ERROR"


def test_summary_is_derived_from_persisted_trial(tmp_path: Path) -> None:
    trial = tmp_path / "group-d" / "q1" / "trial-001"
    trial.mkdir(parents=True)
    result = {
        "finalized": True,
        "bad_producer_ids": ["validator-1"],
        "tx_results": [{
            "correct_candidates": [{"producer_id": "validator-2"}],
            "wrong_candidates": [{"producer_id": "validator-1"}],
            "hallucination_candidates": [], "malicious_candidates": [],
        }],
        "classification_votes": [{
            "judge_id": "validator-2", "model": "qwen3.5:9b",
            "mocked_byzantine": False,
            "classifications": [
                {"semantic_error": False, "is_adversarial": False, "actual": "CORRECT"},
                {"semantic_error": True, "is_adversarial": True, "actual": "CORRECT"},
            ],
        }, {
            "judge_id": "validator-1", "model": "qwen3.5:9b",
            "mocked_byzantine": True,
            "classifications": [{"semantic_error": False}],
        }],
    }
    (trial / "result.json").write_text(json.dumps(result))
    (trial / "trial-status.json").write_text(json.dumps({
        "status": "FINALIZED", "duration_seconds": 12.5,
    }))
    summary = summarize(tmp_path, 1)
    data = summary["configurations"]["group-d/q1"]
    assert data["trials_attempted"] == 1
    assert data["trials_finalized"] == 1
    assert data["adversarial_rejected"] == 1
    assert data["legitimate_retained"] == 1
    assert data["error_free_canonical_rounds"] == 1
    assert data["honest_semantic_errors"] == 1
    assert summary["by_model"]["qwen3.5:9b"] == {
        "decisions": 2, "errors": 1,
        "legitimate_total": 1, "legitimate_retained": 1,
        "adversarial_total": 1, "adversarial_rejected": 0,
    }


def test_group_a_null_bad_producers_is_treated_as_empty(tmp_path: Path) -> None:
    trial = tmp_path / "group-a" / "q0" / "trial-001"
    trial.mkdir(parents=True)
    (trial / "result.json").write_text(json.dumps({
        "finalized": True,
        "bad_producer_ids": None,
        "tx_results": [{
            "correct_candidates": [{"producer_id": "validator-1"}],
            "wrong_candidates": None,
            "hallucination_candidates": None,
            "malicious_candidates": None,
        }],
        "classification_votes": None,
    }))
    (trial / "trial-status.json").write_text(json.dumps({
        "status": "FINALIZED", "duration_seconds": 1.0,
    }))

    summary = summarize(tmp_path, 1)
    data = summary["configurations"]["group-a/q0"]
    assert data["legitimate_retained"] == 1
    assert data["adversarial_accepted"] == 0
