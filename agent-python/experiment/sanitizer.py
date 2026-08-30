"""Redact API keys and auth tokens from exception messages before persistence."""
from __future__ import annotations
import re

_REDACTED = "REDACTED"

_PATTERNS: list[tuple[re.Pattern, str]] = [
    # OpenAI / Anthropic / DeepSeek sk- keys
    (re.compile(r'\bsk-[A-Za-z0-9\-_]{8,}'), _REDACTED),
    # Bearer tokens
    (re.compile(r'(?i)\bBearer\s+\S{8,}'), 'Bearer ' + _REDACTED),
    # api_key= or apikey= assignments (case-insensitive)
    (re.compile(r'(?i)(api[_\-]?key\s*[:=]\s*)\S{8,}'), r'\g<1>' + _REDACTED),
    # x-api-key header value
    (re.compile(r'(?i)(x-api-key\s*[:=]\s*)\S{8,}'), r'\g<1>' + _REDACTED),
    # authorization header value (non-Bearer)
    (re.compile(r'(?i)(authorization\s*[:=]\s*)\S{8,}'), r'\g<1>' + _REDACTED),
]


def sanitize_error(message: str | None) -> str | None:
    """Replace likely secret values in message with REDACTED. Returns None if message is None."""
    if message is None:
        return None
    result = str(message)
    for pattern, replacement in _PATTERNS:
        result = pattern.sub(replacement, result)
    return result
