"""Tests for benchmark/fixtures.py (Finding 12)"""
import pytest

from benchmark.fixtures import (
    ALL_FIXTURES,
    ALL_FIXTURES_HASH,
    DATASET_VERSION,
    Fixture,
    compute_dataset_hash,
    filter_fixtures,
    validate_fixtures,
    validate_semantic,
    validate_structural,
)

ALLOWED_CATEGORIES = {"CORRECT", "WRONG", "HALLUCINATION", "MALICIOUS"}
ALLOWED_GROUPS = {"A", "B", "C", "D", "E", "F"}
ALLOWED_BASES = {"canonical", "benchmark-assumption"}


class TestFixtureStructure:
    def test_total_count(self):
        assert len(ALL_FIXTURES) == 33

    def test_group_a_all_legitimate(self):
        group_a = [f for f in ALL_FIXTURES if f.group == "A"]
        assert len(group_a) == 18
        assert all(not f.is_adversarial for f in group_a)
        assert all(f.expected == "CORRECT" for f in group_a)

    def test_adversarial_groups_count(self):
        for group in ["B", "C", "D", "E", "F"]:
            g = [f for f in ALL_FIXTURES if f.group == group]
            assert len(g) == 3, f"Expected 3 fixtures in group {group}, got {len(g)}"
            assert all(f.is_adversarial for f in g)

    def test_expected_labels_are_valid(self):
        for f in ALL_FIXTURES:
            assert f.expected in ALLOWED_CATEGORIES

    def test_groups_are_valid(self):
        for f in ALL_FIXTURES:
            assert f.group in ALLOWED_GROUPS

    def test_assumption_basis_is_valid(self):
        for f in ALL_FIXTURES:
            assert f.assumption_basis in ALLOWED_BASES, (
                f"Invalid assumption_basis {f.assumption_basis!r} for group={f.group}"
            )

    def test_only_group_a_is_directly_asserted_canonical(self):
        for f in ALL_FIXTURES:
            assert f.assumption_basis == (
                "canonical" if f.group == "A" else "benchmark-assumption"
            )

    def test_benchmark_assumption_groups_e_f(self):
        for f in ALL_FIXTURES:
            if f.group in ("E", "F"):
                assert f.assumption_basis == "benchmark-assumption", (
                    f"Group {f.group} should be benchmark-assumption, got {f.assumption_basis}"
                )

    def test_adversarial_expected_labels(self):
        expected_by_group = {
            "B": "WRONG",
            "C": "MALICIOUS",
            "D": "HALLUCINATION",
            "E": "WRONG",
            "F": "WRONG",
        }
        for f in ALL_FIXTURES:
            if f.group in expected_by_group:
                assert f.expected == expected_by_group[f.group], (
                    f"Group {f.group}: expected {expected_by_group[f.group]}, got {f.expected}"
                )

    def test_no_empty_fields(self):
        for f in ALL_FIXTURES:
            assert f.tx_id
            assert f.prompt
            assert f.answer.strip()
            assert f.scenario_id
            assert f.perspective

    def test_three_transactions(self):
        assert len({f.tx_id for f in ALL_FIXTURES}) == 3

    def test_group_a_has_six_perspectives(self):
        perspectives = {f.perspective for f in ALL_FIXTURES if f.group == "A"}
        assert len(perspectives) == 6


class TestFixtureHash:
    def test_hash_is_full_sha256(self):
        f = ALL_FIXTURES[0]
        h = f.content_hash()
        assert len(h) == 64
        assert all(c in "0123456789abcdef" for c in h)

    def test_different_fixtures_different_hashes(self):
        hashes = [f.content_hash() for f in ALL_FIXTURES]
        assert len(set(hashes)) == len(ALL_FIXTURES), "Fixture hashes must be unique"

    def test_dataset_hash_is_stable(self):
        h1 = compute_dataset_hash(ALL_FIXTURES)
        h2 = compute_dataset_hash(ALL_FIXTURES)
        assert h1 == h2

    def test_dataset_hash_order_independent(self):
        shuffled = list(reversed(ALL_FIXTURES))
        h1 = compute_dataset_hash(ALL_FIXTURES)
        h2 = compute_dataset_hash(shuffled)
        assert h1 == h2, "Dataset hash must be order-independent"

    def test_all_fixtures_hash_constant(self):
        assert ALL_FIXTURES_HASH == compute_dataset_hash(ALL_FIXTURES)

    def test_dataset_version_set(self):
        assert DATASET_VERSION == "v1.0"


class TestValidation:
    def test_structural_no_errors(self):
        errors = validate_structural()
        assert errors == [], f"Unexpected structural errors: {errors}"

    def test_validate_fixtures_delegates_to_structural(self):
        assert validate_fixtures() == validate_structural()

    def test_semantic_has_assumption_notes_for_e_and_f(self):
        notes = validate_semantic()
        assumption_notes = [n for n in notes if "[ASSUMPTION]" in n]
        assert len(assumption_notes) == 15

    def test_semantic_no_errors_on_valid_dataset(self):
        notes = validate_semantic()
        errors = [n for n in notes if "[ERROR]" in n]
        assert errors == [], f"Unexpected semantic errors: {errors}"


class TestFilterFixtures:
    def test_filter_single_group(self):
        result = filter_fixtures(["A"])
        assert all(f.group == "A" for f in result)
        assert len(result) == 18

    def test_filter_multiple_groups(self):
        result = filter_fixtures(["B", "C"])
        assert {f.group for f in result} == {"B", "C"}

    def test_filter_all_groups(self):
        result = filter_fixtures(["A", "B", "C", "D", "E", "F"])
        assert len(result) == len(ALL_FIXTURES)

    def test_filter_empty_list(self):
        assert filter_fixtures([]) == []

    def test_filter_unknown_group_returns_empty(self):
        assert filter_fixtures(["Z"]) == []
