import json
from unittest.mock import AsyncMock, MagicMock

import anthropic
import pytest
from pydantic import BaseModel

from errors import AgentServiceError, ErrorCode
from providers.anthropic_provider import AnthropicProvider


class _Schema(BaseModel):
    result: str


class _NestedSchema(BaseModel):
    item: _Schema


# ── helpers ───────────────────────────────────────────────────────────────────

def _mock_message(
    content_text: str,
    input_tokens: int = 10,
    output_tokens: int = 20,
) -> MagicMock:
    """Build a minimal object that looks like an Anthropic Message."""
    text_block = MagicMock()
    text_block.text = content_text

    usage = MagicMock()
    usage.input_tokens = input_tokens
    usage.output_tokens = output_tokens

    msg = MagicMock()
    msg.content = [text_block] if content_text else []
    msg.usage = usage
    return msg


def _make_provider(response=None, error=None) -> tuple[AnthropicProvider, MagicMock]:
    """Return (provider, mock_client) with messages.create pre-configured."""
    mock_client = MagicMock(spec=anthropic.AsyncAnthropic)
    if error is not None:
        mock_client.messages.create = AsyncMock(side_effect=error)
    else:
        mock_client.messages.create = AsyncMock(return_value=response)
    provider = AnthropicProvider(
        api_key="sk-ant-test",
        model="claude-sonnet-4-6",
        effort="medium",
        client=mock_client,
    )
    return provider, mock_client


# ── structured_chat ───────────────────────────────────────────────────────────

async def test_structured_chat_success():
    provider, _ = _make_provider(_mock_message('{"result": "ok"}'))
    result = await provider.structured_chat("sys", {"key": "val"}, _Schema, timeout_seconds=5.0)
    assert result.result == "ok"


async def test_structured_chat_uses_anthropic_json_schema_output_format():
    provider, mock_client = _make_provider(_mock_message('{"result": "ok"}'))
    await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)

    _, kwargs = mock_client.messages.create.call_args
    assert kwargs["output_config"]["effort"] == "medium"
    output_format = kwargs["output_config"]["format"]
    assert output_format["type"] == "json_schema"
    assert output_format["schema"]["additionalProperties"] is False


async def test_structured_chat_accepts_markdown_fenced_json():
    provider, _ = _make_provider(_mock_message('```json\n{"result": "ok"}\n```'))
    result = await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert result.result == "ok"


async def test_structured_chat_makes_nested_object_schemas_strict():
    provider, mock_client = _make_provider(
        _mock_message('{"item": {"result": "ok"}}')
    )
    await provider.structured_chat("sys", {}, _NestedSchema, timeout_seconds=5.0)

    _, kwargs = mock_client.messages.create.call_args
    schema = kwargs["output_config"]["format"]["schema"]
    assert schema["additionalProperties"] is False
    assert schema["$defs"]["_Schema"]["additionalProperties"] is False


async def test_structured_chat_sends_system_as_top_level_param():
    provider, mock_client = _make_provider(_mock_message('{"result": "ok"}'))
    await provider.structured_chat("my system prompt", {}, _Schema, timeout_seconds=5.0)
    _, kwargs = mock_client.messages.create.call_args
    assert kwargs.get("system") == "my system prompt"
    # system must NOT appear inside messages[]
    for msg in kwargs.get("messages", []):
        assert msg.get("role") != "system"


async def test_structured_chat_sends_user_payload_as_json_string():
    provider, mock_client = _make_provider(_mock_message('{"result": "ok"}'))
    await provider.structured_chat("sys", {"tx": "abc", "v": 1}, _Schema, timeout_seconds=5.0)
    _, kwargs = mock_client.messages.create.call_args
    messages = kwargs.get("messages", [])
    assert len(messages) == 1
    assert messages[0]["role"] == "user"
    parsed = json.loads(messages[0]["content"])
    assert parsed == {"tx": "abc", "v": 1}


async def test_structured_chat_invalid_json_from_model():
    provider, _ = _make_provider(_mock_message("not json"))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.INVALID_MODEL_OUTPUT


async def test_structured_chat_schema_mismatch():
    provider, _ = _make_provider(_mock_message('{"wrong_field": "value"}'))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.INVALID_MODEL_OUTPUT


async def test_structured_chat_results_wrapper_is_unwrapped():
    provider, _ = _make_provider(_mock_message('{"results": [{"result": "unwrapped"}]}'))
    result = await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert result.result == "unwrapped"


async def test_structured_chat_empty_content_raises_provider_error():
    provider, _ = _make_provider(_mock_message(""))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_ERROR


# ── raw_chat ──────────────────────────────────────────────────────────────────

async def test_raw_chat_returns_content_string():
    provider, _ = _make_provider(_mock_message('{"key": "value"}'))
    result = await provider.raw_chat("sys", "user message", timeout_seconds=5.0)
    assert result == '{"key": "value"}'


async def test_raw_chat_json_format_flag_accepted_but_no_api_param():
    """json_format=True is accepted for protocol compatibility but not passed to Anthropic."""
    provider, mock_client = _make_provider(_mock_message("hello"))
    await provider.raw_chat("sys", "user", timeout_seconds=5.0, json_format=True)
    _, kwargs = mock_client.messages.create.call_args
    assert "response_format" not in kwargs


async def test_raw_chat_json_format_removes_markdown_fence():
    provider, _ = _make_provider(_mock_message('```json\n{"key": "value"}\n```'))
    result = await provider.raw_chat(
        "sys", "user", timeout_seconds=5.0, json_format=True
    )
    assert json.loads(result) == {"key": "value"}


async def test_raw_chat_json_format_extracts_json_after_introductory_prose():
    provider, _ = _make_provider(
        _mock_message('Here is the requested classification:\n{"category": "WRONG"}')
    )
    result = await provider.raw_chat(
        "sys", "user", timeout_seconds=5.0, json_format=True
    )
    assert json.loads(result) == {"category": "WRONG"}


async def test_raw_chat_without_json_format_preserves_markdown_fence():
    fenced = '```json\n{"key": "value"}\n```'
    provider, _ = _make_provider(_mock_message(fenced))
    result = await provider.raw_chat("sys", "user", timeout_seconds=5.0)
    assert result == fenced


async def test_raw_chat_sends_correct_message_structure():
    provider, mock_client = _make_provider(_mock_message("hello"))
    await provider.raw_chat("sys prompt", "user content", timeout_seconds=5.0)
    _, kwargs = mock_client.messages.create.call_args
    assert kwargs["system"] == "sys prompt"
    assert kwargs["messages"] == [{"role": "user", "content": "user content"}]


# ── error mapping ─────────────────────────────────────────────────────────────

async def test_timeout_raises_provider_timeout():
    provider, _ = _make_provider(error=anthropic.APITimeoutError("timed out"))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.raw_chat("sys", "user", timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_TIMEOUT
    assert exc_info.value.status_code == 504


async def test_connection_error_raises_provider_error():
    provider, _ = _make_provider(
        error=anthropic.APIConnectionError(message="refused", request=MagicMock())
    )
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.raw_chat("sys", "user", timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_ERROR
    assert exc_info.value.status_code == 502


async def test_api_status_error_raises_provider_error():
    mock_response = MagicMock()
    mock_response.status_code = 529
    mock_response.headers = {}
    mock_response.text = "Overloaded"
    provider, _ = _make_provider(
        error=anthropic.APIStatusError(
            "overloaded",
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
    provider, _ = _make_provider(_mock_message('{"result": "ok"}', input_tokens=42, output_tokens=17))
    with caplog.at_level(logging.INFO, logger="providers.anthropic_provider"):
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)

    llm_call_lines = [r.message for r in caplog.records if r.message.startswith("llm_call")]
    assert len(llm_call_lines) == 1
    line = llm_call_lines[0]
    assert "input_tokens=42" in line
    assert "output_tokens=17" in line
    assert "total_tokens=59" in line


async def test_operation_label_appears_in_log(caplog):
    import logging
    provider, _ = _make_provider(_mock_message('{"result": "ok"}'))
    with caplog.at_level(logging.INFO, logger="providers.anthropic_provider"):
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0, operation="label")

    llm_call_lines = [r.message for r in caplog.records if r.message.startswith("llm_call")]
    assert any("operation=label" in line for line in llm_call_lines)


async def test_provider_name_in_log(caplog):
    import logging
    provider, _ = _make_provider(_mock_message('{"result": "ok"}'))
    with caplog.at_level(logging.INFO, logger="providers.anthropic_provider"):
        await provider.raw_chat("sys", "user", timeout_seconds=5.0)

    llm_call_lines = [r.message for r in caplog.records if r.message.startswith("llm_call")]
    assert any("provider=anthropic" in line for line in llm_call_lines)


# ── ping ──────────────────────────────────────────────────────────────────────

async def test_ping_returns_true_when_reachable():
    mock_client = MagicMock(spec=anthropic.AsyncAnthropic)
    mock_client.messages.create = AsyncMock(return_value=_mock_message("pong"))
    provider = AnthropicProvider(api_key="sk-ant-test", model="claude-sonnet-4-6", effort="medium", client=mock_client)
    assert await provider.ping() is True


async def test_ping_returns_false_on_auth_error():
    mock_response = MagicMock()
    mock_response.status_code = 401
    mock_response.headers = {}
    mock_response.text = "Unauthorized"
    mock_client = MagicMock(spec=anthropic.AsyncAnthropic)
    mock_client.messages.create = AsyncMock(
        side_effect=anthropic.AuthenticationError(
            "invalid key",
            response=mock_response,
            body=None,
        )
    )
    provider = AnthropicProvider(api_key="sk-ant-bad", model="claude-sonnet-4-6", effort="medium", client=mock_client)
    assert await provider.ping() is False


async def test_ping_returns_false_on_connection_error():
    mock_client = MagicMock(spec=anthropic.AsyncAnthropic)
    mock_client.messages.create = AsyncMock(
        side_effect=anthropic.APIConnectionError(message="refused", request=MagicMock())
    )
    provider = AnthropicProvider(api_key="sk-ant-test", model="claude-sonnet-4-6", effort="medium", client=mock_client)
    assert await provider.ping() is False
