import json
import logging
from unittest.mock import AsyncMock, MagicMock

import httpx
import pytest
from google.genai import errors as genai_errors
from pydantic import BaseModel

from errors import AgentServiceError, ErrorCode
from providers.gemini_provider import GeminiProvider


class _Schema(BaseModel):
    result: str


# ── helpers ───────────────────────────────────────────────────────────────────

def _mock_response(
    text: str,
    prompt_token_count: int = 10,
    candidates_token_count: int = 20,
) -> MagicMock:
    """Build a minimal object that looks like a Gemini GenerateContentResponse."""
    usage = MagicMock()
    usage.prompt_token_count = prompt_token_count
    usage.candidates_token_count = candidates_token_count
    usage.total_token_count = prompt_token_count + candidates_token_count

    resp = MagicMock()
    resp.text = text
    resp.usage_metadata = usage
    return resp


def _make_provider(response=None, error=None) -> tuple[GeminiProvider, MagicMock]:
    """Return (provider, mock_client) with aio.models.generate_content pre-configured."""
    mock_client = MagicMock()
    if error is not None:
        mock_client.aio.models.generate_content = AsyncMock(side_effect=error)
    else:
        mock_client.aio.models.generate_content = AsyncMock(return_value=response)
    provider = GeminiProvider(
        api_key="test-key",
        model="gemini-2.0-flash",
        client=mock_client,
    )
    return provider, mock_client


# ── structured_chat ───────────────────────────────────────────────────────────

async def test_structured_chat_success():
    provider, _ = _make_provider(_mock_response('{"result": "ok"}'))
    result = await provider.structured_chat("sys", {"key": "val"}, _Schema, timeout_seconds=5.0)
    assert result.result == "ok"


async def test_structured_chat_sets_json_mime_type():
    provider, mock_client = _make_provider(_mock_response('{"result": "ok"}'))
    await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    _, kwargs = mock_client.aio.models.generate_content.call_args
    config = kwargs["config"]
    assert config.response_mime_type == "application/json"


async def test_structured_chat_sends_user_payload_as_json_string():
    provider, mock_client = _make_provider(_mock_response('{"result": "ok"}'))
    await provider.structured_chat("sys", {"tx": "abc", "v": 1}, _Schema, timeout_seconds=5.0)
    _, kwargs = mock_client.aio.models.generate_content.call_args
    contents = kwargs["contents"]
    parsed = json.loads(contents)
    assert parsed == {"tx": "abc", "v": 1}


async def test_structured_chat_sends_system_via_config():
    provider, mock_client = _make_provider(_mock_response('{"result": "ok"}'))
    await provider.structured_chat("my system prompt", {}, _Schema, timeout_seconds=5.0)
    _, kwargs = mock_client.aio.models.generate_content.call_args
    config = kwargs["config"]
    assert config.system_instruction == "my system prompt"


async def test_structured_chat_invalid_json_from_model():
    provider, _ = _make_provider(_mock_response("not json"))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.INVALID_MODEL_OUTPUT


async def test_structured_chat_schema_mismatch():
    provider, _ = _make_provider(_mock_response('{"wrong_field": "value"}'))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.INVALID_MODEL_OUTPUT


async def test_structured_chat_results_wrapper_is_unwrapped():
    provider, _ = _make_provider(_mock_response('{"results": [{"result": "unwrapped"}]}'))
    result = await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert result.result == "unwrapped"


async def test_structured_chat_empty_response_raises_provider_error():
    provider, _ = _make_provider(_mock_response(""))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_ERROR


# ── raw_chat ──────────────────────────────────────────────────────────────────

async def test_raw_chat_returns_content_string():
    provider, _ = _make_provider(_mock_response('{"key": "value"}'))
    result = await provider.raw_chat("sys", "user message", timeout_seconds=5.0)
    assert result == '{"key": "value"}'


async def test_raw_chat_json_format_false_no_mime_type():
    provider, mock_client = _make_provider(_mock_response("hello"))
    await provider.raw_chat("sys", "user", timeout_seconds=5.0, json_format=False)
    _, kwargs = mock_client.aio.models.generate_content.call_args
    config = kwargs["config"]
    assert config.response_mime_type is None


async def test_raw_chat_json_format_true_sets_mime_type():
    provider, mock_client = _make_provider(_mock_response('{"x": 1}'))
    await provider.raw_chat("sys", "user", timeout_seconds=5.0, json_format=True)
    _, kwargs = mock_client.aio.models.generate_content.call_args
    config = kwargs["config"]
    assert config.response_mime_type == "application/json"


# ── error mapping ─────────────────────────────────────────────────────────────

async def test_timeout_raises_provider_timeout():
    provider, _ = _make_provider(error=httpx.ReadTimeout("timed out"))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.raw_chat("sys", "user", timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_TIMEOUT
    assert exc_info.value.status_code == 504


async def test_client_error_raises_provider_error():
    provider, _ = _make_provider(error=genai_errors.ClientError(429, {"error": "rate limited"}))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.raw_chat("sys", "user", timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_ERROR
    assert exc_info.value.status_code == 502


async def test_server_error_raises_provider_error():
    provider, _ = _make_provider(error=genai_errors.ServerError(503, {"error": "unavailable"}))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.raw_chat("sys", "user", timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_ERROR
    assert exc_info.value.status_code == 502


async def test_api_error_raises_provider_error():
    provider, _ = _make_provider(error=genai_errors.APIError(500, {"error": "internal"}))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.raw_chat("sys", "user", timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_ERROR
    assert exc_info.value.status_code == 502


# ── token usage extraction ────────────────────────────────────────────────────

async def test_token_usage_extracted(caplog):
    provider, _ = _make_provider(
        _mock_response('{"result": "ok"}', prompt_token_count=42, candidates_token_count=17)
    )
    with caplog.at_level(logging.INFO, logger="providers.gemini_provider"):
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)

    llm_call_lines = [r.message for r in caplog.records if r.message.startswith("llm_call")]
    assert len(llm_call_lines) == 1
    line = llm_call_lines[0]
    assert "input_tokens=42" in line
    assert "output_tokens=17" in line
    assert "total_tokens=59" in line


async def test_operation_label_appears_in_log(caplog):
    provider, _ = _make_provider(_mock_response('{"result": "ok"}'))
    with caplog.at_level(logging.INFO, logger="providers.gemini_provider"):
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0, operation="label")

    llm_call_lines = [r.message for r in caplog.records if r.message.startswith("llm_call")]
    assert any("operation=label" in line for line in llm_call_lines)


async def test_provider_name_in_log(caplog):
    provider, _ = _make_provider(_mock_response('{"result": "ok"}'))
    with caplog.at_level(logging.INFO, logger="providers.gemini_provider"):
        await provider.raw_chat("sys", "user", timeout_seconds=5.0)

    llm_call_lines = [r.message for r in caplog.records if r.message.startswith("llm_call")]
    assert any("provider=gemini" in line for line in llm_call_lines)


# ── ping ──────────────────────────────────────────────────────────────────────

async def test_ping_returns_true_when_reachable():
    mock_client = MagicMock()
    mock_client.aio.models.get = AsyncMock(return_value=MagicMock())
    provider = GeminiProvider(api_key="test-key", model="gemini-2.0-flash", client=mock_client)
    assert await provider.ping() is True


async def test_ping_returns_false_on_error():
    mock_client = MagicMock()
    mock_client.aio.models.get = AsyncMock(
        side_effect=genai_errors.ClientError(403, {"error": "forbidden"})
    )
    provider = GeminiProvider(api_key="test-key", model="gemini-2.0-flash", client=mock_client)
    assert await provider.ping() is False
