"""Tests for providers/parsing.py — validate_schema_with_unwrap."""
import pytest
from pydantic import BaseModel

from errors import AgentServiceError
from providers.parsing import validate_schema_with_unwrap


class _Simple(BaseModel):
    tx_hash: str
    value: int


# ── Primary path: flat valid output ──────────────────────────────────────────

def test_flat_valid_output():
    result = validate_schema_with_unwrap({"tx_hash": "abc", "value": 1}, _Simple)
    assert result.tx_hash == "abc"
    assert result.value == 1


def test_flat_invalid_output_raises():
    with pytest.raises(AgentServiceError, match="does not match expected schema"):
        validate_schema_with_unwrap({"tx_hash": "abc"}, _Simple)  # missing value


# ── Fallback path: single-item results wrapper ────────────────────────────────

def test_results_wrapper_single_item_is_unwrapped():
    result = validate_schema_with_unwrap(
        {"results": [{"tx_hash": "abc", "value": 42}]}, _Simple
    )
    assert result.tx_hash == "abc"
    assert result.value == 42


def test_results_wrapper_invalid_item_raises():
    with pytest.raises(AgentServiceError, match="after unwrapping results\\[0\\]"):
        validate_schema_with_unwrap({"results": [{"tx_hash": "abc"}]}, _Simple)


# ── Rejected wrapper variants ─────────────────────────────────────────────────

def test_results_wrapper_empty_raises():
    with pytest.raises(AgentServiceError, match="empty results array"):
        validate_schema_with_unwrap({"results": []}, _Simple)


def test_results_wrapper_multiple_items_raises():
    with pytest.raises(AgentServiceError, match="2 items"):
        validate_schema_with_unwrap(
            {"results": [{"tx_hash": "a", "value": 1}, {"tx_hash": "b", "value": 2}]},
            _Simple,
        )


def test_results_non_array_raises():
    with pytest.raises(AgentServiceError, match="non-array"):
        validate_schema_with_unwrap({"results": {"tx_hash": "abc", "value": 1}}, _Simple)


# ── Objects that are not the results wrapper ──────────────────────────────────

def test_extra_keys_alongside_results_not_unwrapped():
    # {"results": [...], "extra": "field"} — keys != {"results"}, treated as schema error.
    with pytest.raises(AgentServiceError, match="does not match expected schema"):
        validate_schema_with_unwrap(
            {"results": [{"tx_hash": "abc", "value": 1}], "extra": "field"}, _Simple
        )


def test_non_results_key_not_unwrapped():
    with pytest.raises(AgentServiceError, match="does not match expected schema"):
        validate_schema_with_unwrap({"data": [{"tx_hash": "abc", "value": 1}]}, _Simple)


def test_non_dict_raises():
    with pytest.raises(AgentServiceError, match="does not match expected schema"):
        validate_schema_with_unwrap([{"tx_hash": "abc", "value": 1}], _Simple)
