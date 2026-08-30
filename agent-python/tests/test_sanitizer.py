from experiment.sanitizer import sanitize_error


def test_sanitizes_sk_key():
    msg = "Connection failed: Authorization sk-abc123xyz456def789"
    result = sanitize_error(msg)
    assert "sk-abc123xyz456def789" not in result
    assert "REDACTED" in result


def test_sanitizes_bearer_token():
    msg = "Request failed with header Bearer eyJhbGciOiJSUzI1NiIsIn"
    result = sanitize_error(msg)
    assert "eyJhbGciOiJSUzI1NiIsIn" not in result
    assert "Bearer REDACTED" in result


def test_sanitizes_api_key_assignment():
    msg = "api_key=sk-proj-reallylong123456789 was rejected"
    result = sanitize_error(msg)
    assert "sk-proj-reallylong123456789" not in result
    assert "api_key=REDACTED" in result


def test_sanitizes_x_api_key_header():
    msg = "x-api-key: supersecretapikey123456"
    result = sanitize_error(msg)
    assert "supersecretapikey123456" not in result
    assert "REDACTED" in result


def test_sanitizes_authorization_header():
    # Bare token (no scheme prefix) — matches the pattern
    msg = "authorization: dXNlcjpwYXNzd29yZA=="
    result = sanitize_error(msg)
    assert "dXNlcjpwYXNzd29yZA==" not in result
    assert "REDACTED" in result


def test_short_strings_not_redacted():
    # Values shorter than 8 chars should NOT be redacted
    msg = "api_key=short error occurred"
    result = sanitize_error(msg)
    # The short value does not match (fewer than 8 non-whitespace chars after =)
    assert result == msg


def test_none_returns_none():
    assert sanitize_error(None) is None


def test_plain_error_unchanged():
    msg = "Connection timeout after 30 seconds"
    result = sanitize_error(msg)
    assert result == msg


def test_multiple_secrets_in_one_message():
    msg = "sk-key123456789 and api_key=another123456789"
    result = sanitize_error(msg)
    assert "sk-key123456789" not in result
    assert "another123456789" not in result
    assert result.count("REDACTED") >= 2
