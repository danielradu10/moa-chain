import json
from unittest.mock import AsyncMock, MagicMock

import openai
import pytest
from pydantic import BaseModel

from errors import AgentServiceError, ErrorCode
from providers.openai_provider import OpenAIProvider


class _Schema(BaseModel):
    result: str


# ── helpers ───────────────────────────────────────────────────────────────────

def _mock_response(content: str, prompt_tokens: int = 10, completion_tokens: int = 20) -> MagicMock:
    """Build a minimal object that looks like an openai ChatCompletion."""
    usage = MagicMock()
    usage.prompt_tokens = prompt_tokens
    usage.completion_tokens = completion_tokens
    usage.total_tokens = prompt_tokens + completion_tokens

    message = MagicMock()
    message.content = content

    choice = MagicMock()
    choice.message = message

    resp = MagicMock()
    resp.choices = [choice]
    resp.usage = usage
    return resp


def _make_provider(response=None, error=None) -> tuple[OpenAIProvider, MagicMock]:
    """Return (provider, mock_client) with chat.completions.create pre-configured."""
    mock_client = MagicMock(spec=openai.AsyncOpenAI)
    if error is not None:
        mock_client.chat.completions.create = AsyncMock(side_effect=error)
    else:
        mock_client.chat.completions.create = AsyncMock(return_value=response)
    provider = OpenAIProvider(
        api_key="sk-test",
        model="gpt-4o-mini",
        client=mock_client,
    )
    return provider, mock_client


# ── structured_chat ───────────────────────────────────────────────────────────

async def test_structured_chat_success():
    provider, _ = _make_provider(_mock_response('{"result": "ok"}'))
    result = await provider.structured_chat("sys", {"key": "val"}, _Schema, timeout_seconds=5.0)
    assert result.result == "ok"


async def test_structured_chat_sends_json_object_format():
    provider, mock_client = _make_provider(_mock_response('{"result": "ok"}'))
    await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    _, kwargs = mock_client.chat.completions.create.call_args
    assert kwargs.get("response_format") == {"type": "json_object"}


async def test_structured_chat_invalid_json_from_model():
    provider, _ = _make_provider(_mock_response("not json at all"))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.INVALID_MODEL_OUTPUT


async def test_structured_chat_schema_mismatch():
    provider, _ = _make_provider(_mock_response('{"wrong_field": "value"}'))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.INVALID_MODEL_OUTPUT


async def test_structured_chat_empty_content_raises_provider_error():
    provider, _ = _make_provider(_mock_response(""))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_ERROR


# ── raw_chat ──────────────────────────────────────────────────────────────────

async def test_raw_chat_returns_content_string():
    provider, _ = _make_provider(_mock_response('{"key": "value"}'))
    result = await provider.raw_chat("sys", "user message", timeout_seconds=5.0)
    assert result == '{"key": "value"}'


async def test_raw_chat_no_json_format_by_default():
    provider, mock_client = _make_provider(_mock_response("hello"))
    await provider.raw_chat("sys", "user message", timeout_seconds=5.0)
    _, kwargs = mock_client.chat.completions.create.call_args
    assert "response_format" not in kwargs


async def test_raw_chat_json_format_flag():
    provider, mock_client = _make_provider(_mock_response('{"a": 1}'))
    await provider.raw_chat("sys", "user", timeout_seconds=5.0, json_format=True)
    _, kwargs = mock_client.chat.completions.create.call_args
    assert kwargs.get("response_format") == {"type": "json_object"}


# ── error mapping ─────────────────────────────────────────────────────────────

async def test_timeout_raises_provider_timeout():
    provider, _ = _make_provider(
        error=openai.APITimeoutError("timed out")
    )
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.raw_chat("sys", "user", timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_TIMEOUT
    assert exc_info.value.status_code == 504


async def test_connection_error_raises_provider_error():
    provider, _ = _make_provider(
        error=openai.APIConnectionError(message="refused", request=MagicMock())
    )
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.raw_chat("sys", "user", timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_ERROR
    assert exc_info.value.status_code == 502


async def test_api_status_error_raises_provider_error():
    mock_response = MagicMock()
    mock_response.status_code = 500
    mock_response.headers = {}
    mock_response.text = "Internal Server Error"
    provider, _ = _make_provider(
        error=openai.APIStatusError(
            "internal server error",
            response=mock_response,
            body=None,
        )
    )
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.raw_chat("sys", "user", timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_ERROR
    assert exc_info.value.status_code == 502


# ── token usage extraction ────────────────────────────────────────────────────

async def test_token_usage_is_extracted(caplog):
    import logging
    provider, _ = _make_provider(_mock_response('{"result": "ok"}', prompt_tokens=42, completion_tokens=17))
    with caplog.at_level(logging.INFO, logger="providers.openai_provider"):
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)

    llm_call_lines = [r.message for r in caplog.records if r.message.startswith("llm_call")]
    assert len(llm_call_lines) == 1
    line = llm_call_lines[0]
    assert "input_tokens=42" in line
    assert "output_tokens=17" in line
    assert "total_tokens=59" in line


async def test_operation_label_appears_in_log(caplog):
    import logging
    provider, _ = _make_provider(_mock_response('{"result": "ok"}'))
    with caplog.at_level(logging.INFO, logger="providers.openai_provider"):
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0, operation="label")

    llm_call_lines = [r.message for r in caplog.records if r.message.startswith("llm_call")]
    assert any("operation=label" in line for line in llm_call_lines)


# ── ping ──────────────────────────────────────────────────────────────────────

async def test_ping_returns_true_when_model_present():
    mock_client = MagicMock(spec=openai.AsyncOpenAI)
    mock_client.models.retrieve = AsyncMock(return_value=MagicMock())
    provider = OpenAIProvider(api_key="sk-test", model="gpt-4o-mini", client=mock_client)
    assert await provider.ping() is True


async def test_ping_returns_false_when_model_not_found():
    mock_response = MagicMock()
    mock_response.status_code = 404
    mock_response.headers = {}
    mock_response.text = "Not Found"
    mock_client = MagicMock(spec=openai.AsyncOpenAI)
    mock_client.models.retrieve = AsyncMock(
        side_effect=openai.NotFoundError(
            "model not found",
            response=mock_response,
            body=None,
        )
    )
    provider = OpenAIProvider(api_key="sk-test", model="gpt-4o-mini", client=mock_client)
    assert await provider.ping() is False


async def test_ping_returns_false_on_connection_error():
    mock_client = MagicMock(spec=openai.AsyncOpenAI)
    mock_client.models.retrieve = AsyncMock(
        side_effect=openai.APIConnectionError(message="refused", request=MagicMock())
    )
    provider = OpenAIProvider(api_key="sk-test", model="gpt-4o-mini", client=mock_client)
    assert await provider.ping() is False
