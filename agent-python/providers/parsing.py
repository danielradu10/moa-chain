"""Shared schema validation with a narrow single-item results-wrapper fallback.

Some models (e.g. gpt-5-mini) occasionally wrap a single-transaction response
in {"results": [<item>]} even when asked to return a flat object. This module
provides a single validation entry point used by all providers so the fallback
is consistent and tested in one place.

The fallback is deliberately narrow:
  - only {"results": [exactly one item]} is unwrapped;
  - empty arrays, multiple items, non-list values, or objects with any key
    besides "results" are all rejected without unwrapping.
"""
from __future__ import annotations

from typing import TypeVar

from pydantic import BaseModel, ValidationError

from errors import AgentServiceError, ErrorCode

T = TypeVar("T", bound=BaseModel)


def validate_schema_with_unwrap(data: object, schema: type[T]) -> T:
    """Validate *data* against *schema*, with a single-item results-wrapper fallback.

    Algorithm:
      1. Try ``schema.model_validate(data)`` directly.
      2. If that fails AND *data* is exactly ``{"results": [one_item]}``,
         try ``schema.model_validate(one_item)``.
      3. Any other structure or additional validation failure raises
         ``AgentServiceError(INVALID_MODEL_OUTPUT)``.
    """
    # Primary attempt.
    try:
        return schema.model_validate(data)
    except ValidationError as first_exc:
        _first = first_exc

    # Only unwrap when the object is exactly {"results": [...]}.
    if not isinstance(data, dict) or set(data.keys()) != {"results"}:
        raise AgentServiceError(
            ErrorCode.INVALID_MODEL_OUTPUT,
            f"model output does not match expected schema: {_first}",
            status_code=502,
        ) from _first

    results = data["results"]

    if not isinstance(results, list):
        raise AgentServiceError(
            ErrorCode.INVALID_MODEL_OUTPUT,
            'model wrapped output in {"results": <non-array>}; expected a list',
            status_code=502,
        ) from _first

    if len(results) == 0:
        raise AgentServiceError(
            ErrorCode.INVALID_MODEL_OUTPUT,
            "model returned an empty results array",
            status_code=502,
        ) from _first

    if len(results) > 1:
        raise AgentServiceError(
            ErrorCode.INVALID_MODEL_OUTPUT,
            f"model returned {len(results)} items in results array; expected exactly 1",
            status_code=502,
        ) from _first

    # Validate the unwrapped single item.
    try:
        return schema.model_validate(results[0])
    except ValidationError as exc:
        raise AgentServiceError(
            ErrorCode.INVALID_MODEL_OUTPUT,
            f"model output does not match expected schema after unwrapping results[0]: {exc}",
            status_code=502,
        ) from exc
