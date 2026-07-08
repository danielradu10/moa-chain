from errors import AgentServiceError, ErrorCode

# Pure validation helpers — no side effects, just raise AgentServiceError on failure.
# Called by routers after receiving the model's response.


def check_prompt_version(request_version: str, loaded_version: str) -> None:
    """Ensure the caller is using the same prompt version the service has loaded.

    Go passes prompt_version in every request so it can detect if the Python
    service was restarted with a different prompt file.
    """
    if request_version != loaded_version:
        raise AgentServiceError(
            ErrorCode.PROMPT_VERSION_MISMATCH,
            f"expected prompt version '{loaded_version}', got '{request_version}'",
        )


def check_tx_hash_coverage(actual: str, expected: str) -> None:
    """Ensure the model echoed back the correct tx_hash for this transaction.

    Each LLM call processes one transaction at a time, so the returned tx_hash
    must exactly match the one we sent. A mismatch means the model hallucinated
    or mixed up transactions.
    """
    if actual != expected:
        raise AgentServiceError(
            ErrorCode.COVERAGE_MISMATCH,
            f"expected tx_hash '{expected}', got '{actual}'",
        )


def check_subdomain_membership(subdomain: str, allowed: set[str]) -> None:
    """Ensure the model only assigned subdomains from the allowed set.

    The allowed set is sent to the model in the request, but models can still
    hallucinate values outside it.
    """
    if subdomain not in allowed:
        raise AgentServiceError(
            ErrorCode.UNKNOWN_SUBDOMAIN,
            f"subdomain '{subdomain}' not in allowed set",
        )


def check_confidence_range(confidence: float) -> None:
    """Ensure the model's confidence score is a valid probability in [0.0, 1.0]."""
    if not 0.0 <= confidence <= 1.0:
        raise AgentServiceError(
            ErrorCode.INVALID_MODEL_OUTPUT,
            f"confidence {confidence} is outside [0.0, 1.0]",
        )


def check_non_empty(value: str, field: str) -> None:
    """Ensure a string field is non-empty after stripping whitespace.

    Used for answer text and judge reason fields — an empty value means
    the model returned a structurally valid response but with no useful content.
    """
    if not value.strip():
        raise AgentServiceError(
            ErrorCode.EMPTY_ANSWER,
            f"'{field}' is empty or whitespace-only",
        )
