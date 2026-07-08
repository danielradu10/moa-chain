import pytest

from errors import AgentServiceError, ErrorCode
from validation import (
    check_confidence_range,
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
