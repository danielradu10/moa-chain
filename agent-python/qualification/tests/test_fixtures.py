"""Structural tests for qualification fixtures."""
from __future__ import annotations

import pytest

from qualification.fixtures import (
    ALLOWED_SUBDOMAINS,
    CANONICAL_QUESTION,
    CANONICAL_TX_HASH,
    JUDGE_FIXTURES,
    SYNTHESIS_CANDIDATES,
    SYNTHESIS_EVAL_FIXTURES,
)


def test_canonical_question_not_empty():
    assert CANONICAL_QUESTION.strip() != ""


def test_canonical_tx_hash_not_empty():
    assert CANONICAL_TX_HASH.strip() != ""


def test_allowed_subdomains_count():
    assert len(ALLOWED_SUBDOMAINS) == 13, "PossibleSubDomains must have 13 values"


def test_allowed_subdomains_no_duplicates():
    assert len(ALLOWED_SUBDOMAINS) == len(set(ALLOWED_SUBDOMAINS))


def test_allowed_subdomains_contains_non_related():
    assert "non_related" in ALLOWED_SUBDOMAINS


def test_judge_fixtures_count():
    assert len(JUDGE_FIXTURES) == 4, "One fixture per category: CORRECT, WRONG, HALLUCINATION, MALICIOUS"


def test_judge_fixtures_labels_unique():
    labels = [f["label"] for f in JUDGE_FIXTURES]
    assert len(labels) == len(set(labels))


def test_judge_fixtures_expected_categories():
    expected_categories = {"CORRECT", "WRONG", "HALLUCINATION", "MALICIOUS"}
    actual = {f["expected"] for f in JUDGE_FIXTURES}
    assert actual == expected_categories


def test_judge_fixtures_no_empty_answers():
    for fixture in JUDGE_FIXTURES:
        assert fixture["answer"].strip() != "", f"Fixture {fixture['label']} has empty answer"


def test_synthesis_candidates_count():
    assert len(SYNTHESIS_CANDIDATES) == 4


def test_synthesis_candidates_no_empty():
    for i, c in enumerate(SYNTHESIS_CANDIDATES):
        assert c.strip() != "", f"Synthesis candidate {i} is empty"


def test_synthesis_eval_fixtures_count():
    assert len(SYNTHESIS_EVAL_FIXTURES) == 2


def test_synthesis_eval_fixtures_one_approved_one_rejected():
    approved = [f["expected_approved"] for f in SYNTHESIS_EVAL_FIXTURES]
    assert True in approved and False in approved


def test_synthesis_eval_fixtures_labels_unique():
    labels = [f["label"] for f in SYNTHESIS_EVAL_FIXTURES]
    assert len(labels) == len(set(labels))
