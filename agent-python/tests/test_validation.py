import pytest

from errors import AgentServiceError, ErrorCode
from schemas import LabelEntry
from validation import (
    check_confidence_range,
    check_label_list,
    check_prompt_version,
    check_subdomain_membership,
    check_tx_hash_coverage,
)


def test_check_prompt_version_match() -> None:
    check_prompt_version("labeler_v1", "labeler_v1")


def test_check_prompt_version_mismatch() -> None:
    with pytest.raises(AgentServiceError) as exc:
        check_prompt_version("labeler_v99", "labeler_v1")
    assert exc.value.code == ErrorCode.PROMPT_VERSION_MISMATCH


def test_check_tx_hash_coverage_match() -> None:
    check_tx_hash_coverage("0xabc", "0xabc")


def test_check_tx_hash_coverage_mismatch() -> None:
    with pytest.raises(AgentServiceError) as exc:
        check_tx_hash_coverage("0xwrong", "0xabc")
    assert exc.value.code == ErrorCode.COVERAGE_MISMATCH


def test_check_subdomain_membership_valid() -> None:
    check_subdomain_membership("databases", {"databases", "security"})


def test_check_subdomain_membership_unknown() -> None:
    with pytest.raises(AgentServiceError) as exc:
        check_subdomain_membership("unknown_domain", {"databases"})
    assert exc.value.code == ErrorCode.UNKNOWN_SUBDOMAIN


def test_check_confidence_range_valid_boundaries() -> None:
    check_confidence_range(0.0)
    check_confidence_range(0.5)
    check_confidence_range(1.0)


def test_check_confidence_range_too_high() -> None:
    with pytest.raises(AgentServiceError) as exc:
        check_confidence_range(1.1)
    assert exc.value.code == ErrorCode.INVALID_MODEL_OUTPUT


def test_check_confidence_range_negative() -> None:
    with pytest.raises(AgentServiceError) as exc:
        check_confidence_range(-0.1)
    assert exc.value.code == ErrorCode.INVALID_MODEL_OUTPUT


# ── check_label_list ──────────────────────────────────────────────────────────

_ALLOWED = {"databases", "security", "non_related"}


def _entries(*subdomains: str) -> list[LabelEntry]:
    return [LabelEntry(subdomain=s, confidence=0.9) for s in subdomains]


def test_check_label_list_pure_non_related_is_accepted() -> None:
    check_label_list(_entries("non_related"), "0xabc", _ALLOWED)


def test_check_label_list_single_real_label_is_accepted() -> None:
    check_label_list(_entries("databases"), "0xabc", _ALLOWED)


def test_check_label_list_exactly_three_real_labels_is_accepted() -> None:
    allowed = {"databases", "security", "systems_programming", "non_related"}
    check_label_list(_entries("databases", "security", "systems_programming"), "0xabc", allowed)


def test_check_label_list_non_related_mixed_with_real_label_is_rejected() -> None:
    with pytest.raises(AgentServiceError) as exc:
        check_label_list(_entries("non_related", "databases"), "0xabc", _ALLOWED)
    assert exc.value.code == ErrorCode.UNKNOWN_SUBDOMAIN


def test_check_label_list_non_related_mixed_with_two_real_labels_is_rejected() -> None:
    allowed = {"databases", "security", "non_related"}
    with pytest.raises(AgentServiceError) as exc:
        check_label_list(_entries("databases", "security", "non_related"), "0xabc", allowed)
    assert exc.value.code == ErrorCode.UNKNOWN_SUBDOMAIN


def test_check_label_list_more_than_three_real_labels_is_rejected() -> None:
    allowed = {"databases", "security", "systems_programming", "dev_ops", "non_related"}
    with pytest.raises(AgentServiceError) as exc:
        check_label_list(
            _entries("databases", "security", "systems_programming", "dev_ops"),
            "0xabc",
            allowed,
        )
    assert exc.value.code == ErrorCode.INVALID_MODEL_OUTPUT
