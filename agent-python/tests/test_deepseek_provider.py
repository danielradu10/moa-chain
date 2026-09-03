import json
import logging
from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock

import openai
import pytest
from pydantic import BaseModel

from errors import AgentServiceError, ErrorCode
from providers.deepseek_provider import DeepSeekProvider
from routers.health import health


class _Schema(BaseModel):
    result: str


def _mock_response(
    content: str,
    prompt_tokens: int = 10,
    completion_tokens: int = 20,
) -> MagicMock:
    usage = MagicMock()
    usage.prompt_tokens = prompt_tokens
    usage.completion_tokens = completion_tokens
    usage.total_tokens = prompt_tokens + completion_tokens
    message = MagicMock(content=content)
    choice = MagicMock(message=message)
    return MagicMock(choices=[choice], usage=usage)


def _make_provider(response=None, error=None) -> tuple[DeepSeekProvider, MagicMock]:
    client = MagicMock(spec=openai.AsyncOpenAI)
    client.chat.completions.create = (
        AsyncMock(side_effect=error)
        if error is not None
        else AsyncMock(return_value=response)
    )
    provider = DeepSeekProvider(
        api_key="ds-test",
        model="deepseek-v4-flash",
        client=client,
    )
    return provider, client


async def test_raw_chat_extracts_text():
    provider, _ = _make_provider(_mock_response("plain text"))
    assert await provider.raw_chat("sys", "user", 5.0) == "plain text"


async def test_structured_chat_extracts_and_validates_json():
    provider, client = _make_provider(_mock_response('{"result": "ok"}'))
    result = await provider.structured_chat("return json", {}, _Schema, 5.0)

    assert result.result == "ok"
    _, kwargs = client.chat.completions.create.call_args
    assert kwargs["response_format"] == {"type": "json_object"}
    assert json.loads(kwargs["messages"][1]["content"]) == {}


async def test_structured_chat_accepts_fenced_json_defensively():
    provider, _ = _make_provider(
        _mock_response('```json\n{"result": "ok"}\n```')
    )
    result = await provider.structured_chat("return json", {}, _Schema, 5.0)
    assert result.result == "ok"


async def test_structured_chat_rejects_malformed_json():
    provider, _ = _make_provider(_mock_response("not json"))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("return json", {}, _Schema, 5.0)
    assert exc_info.value.code == ErrorCode.INVALID_MODEL_OUTPUT


async def test_structured_chat_rejects_schema_mismatch():
    provider, _ = _make_provider(_mock_response('{"wrong": "value"}'))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("return json", {}, _Schema, 5.0)
    assert exc_info.value.code == ErrorCode.INVALID_MODEL_OUTPUT


async def test_token_usage_maps_to_common_log(caplog):
    provider, _ = _make_provider(
        _mock_response('{"result": "ok"}', prompt_tokens=42, completion_tokens=17)
    )
    with caplog.at_level(logging.INFO, logger="providers.deepseek_provider"):
        await provider.structured_chat(
            "return json", {}, _Schema, 5.0, operation="label"
        )

    lines = [r.message for r in caplog.records if r.message.startswith("llm_call")]
    assert len(lines) == 1
    assert "provider=deepseek" in lines[0]
    assert "operation=label" in lines[0]
    assert "input_tokens=42" in lines[0]
    assert "output_tokens=17" in lines[0]
    assert "total_tokens=59" in lines[0]


async def test_ping_true_when_configured_model_is_listed():
    provider, client = _make_provider()
    client.models.list = AsyncMock(
        return_value=MagicMock(data=[MagicMock(id="deepseek-v4-flash")])
    )
    assert await provider.ping() is True


async def test_ping_false_when_configured_model_is_absent():
    provider, client = _make_provider()
    client.models.list = AsyncMock(
        return_value=MagicMock(data=[MagicMock(id="deepseek-v4-pro")])
    )
    assert await provider.ping() is False


async def test_ping_false_on_api_error():
    provider, client = _make_provider()
    client.models.list = AsyncMock(
        side_effect=openai.APIConnectionError(
            message="refused", request=MagicMock()
        )
    )
    assert await provider.ping() is False


async def test_health_route_reports_deepseek_model_and_reachability():
    provider = MagicMock()
    provider.ping = AsyncMock(return_value=True)
    request = MagicMock()
    request.app.state = SimpleNamespace(
        config=SimpleNamespace(
            llm_provider="deepseek", model="deepseek-v4-flash"
        ),
        provider=provider,
        prompts={},
    )

    response = await health(request)
    assert response.provider == "deepseek"
    assert response.model == "deepseek-v4-flash"
    assert response.reachable is True


async def test_api_error_maps_to_service_error_with_details():
    response = MagicMock(status_code=429, headers={}, text="Rate limited")
    provider, _ = _make_provider(
        error=openai.APIStatusError(
            "rate limit exceeded", response=response, body=None
        )
    )
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.raw_chat("sys", "user", 5.0)

    assert exc_info.value.code == ErrorCode.PROVIDER_ERROR
    assert exc_info.value.status_code == 502
    assert "429" in exc_info.value.detail
    assert "rate limit exceeded" in exc_info.value.detail


async def test_timeout_maps_to_provider_timeout():
    provider, _ = _make_provider(error=openai.APITimeoutError("timed out"))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.raw_chat("sys", "user", 5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_TIMEOUT
    assert exc_info.value.status_code == 504
