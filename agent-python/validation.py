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


def check_judge_response(response: str) -> None:
    """Validate the raw judge response string returned by the LLM.

    Python does the minimum validation needed to surface model errors early.
    Go owns the full schema and parses the response itself after this check passes.
    The expected structure matches the Go protocol format:
      {"classifications": [{"candidateId": "candidate-1", "category": "CORRECT"}, ...]}
    """
    import json

    allowed_categories = {"CORRECT", "HALLUCINATION", "MALICIOUS", "WRONG"}

    try:
        data = json.loads(response)
    except json.JSONDecodeError as exc:
        raise AgentServiceError(
            ErrorCode.INVALID_MODEL_OUTPUT,
            f"judge response is not valid JSON: {exc}",
        ) from exc

    if not isinstance(data, dict) or "classifications" not in data:
        raise AgentServiceError(
            ErrorCode.INVALID_MODEL_OUTPUT,
            'judge response must be a JSON object with a "classifications" key',
        )

    classifications = data["classifications"]
    if not isinstance(classifications, list):
        raise AgentServiceError(
            ErrorCode.INVALID_MODEL_OUTPUT,
            '"classifications" must be a JSON array',
        )

    for item in classifications:
        category = item.get("category", "")
        if category not in allowed_categories:
            raise AgentServiceError(
                ErrorCode.UNKNOWN_CATEGORY,
                f"category '{category}' not in allowed set {sorted(allowed_categories)}",
            )


NON_RELATED = "non_related"
_MAX_LABELS = 3


def check_label_list(labels: list, tx_hash: str, allowed: set[str]) -> None:
    """Enforce the PR-10 labeling rules for a single transaction:

    - non_related must be the only label when present; mixing it with a real
      subdomain is rejected as UNKNOWN_SUBDOMAIN.
    - At most three real subdomains may be assigned per transaction.
    - Every subdomain must be in the allowed set.
    """
    subdomains = [entry.subdomain for entry in labels]

    has_non_related = NON_RELATED in subdomains
    has_real = any(s != NON_RELATED for s in subdomains)

    if has_non_related and has_real:
        raise AgentServiceError(
            ErrorCode.UNKNOWN_SUBDOMAIN,
            f"tx_hash '{tx_hash}': non_related cannot appear alongside a real subdomain",
        )

    if not has_non_related and len(subdomains) > _MAX_LABELS:
        raise AgentServiceError(
            ErrorCode.INVALID_MODEL_OUTPUT,
            f"tx_hash '{tx_hash}': at most {_MAX_LABELS} subdomains allowed, got {len(subdomains)}",
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
