import json

import httpx
import pytest
from pydantic import BaseModel

from errors import AgentServiceError, ErrorCode
from providers.ollama_provider import OllamaProvider


class _Schema(BaseModel):
    result: str


class _MockTransport(httpx.AsyncBaseTransport):
    def __init__(self, status_code: int, body: bytes) -> None:
        self._status_code = status_code
        self._body = body

    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            status_code=self._status_code,
            headers={"content-type": "application/json"},
            content=self._body,
        )


class _TimeoutTransport(httpx.AsyncBaseTransport):
    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        raise httpx.TimeoutException("timed out", request=request)


class _ConnectErrorTransport(httpx.AsyncBaseTransport):
    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("connection refused", request=request)


def _ollama_body(content: str) -> bytes:
    return json.dumps({
        "model": "test-model",
        "message": {"role": "assistant", "content": content},
        "done": True,
    }).encode()


def _make_provider(transport: httpx.AsyncBaseTransport) -> OllamaProvider:
    client = httpx.AsyncClient(transport=transport, base_url="http://test")
    return OllamaProvider(base_url="http://test", model="test-model", client=client)


# --- structured_chat ---

async def test_structured_chat_success() -> None:
    provider = _make_provider(_MockTransport(200, _ollama_body('{"result": "ok"}')))
    result = await provider.structured_chat("sys", {"key": "val"}, _Schema, timeout_seconds=5.0)
    assert result.result == "ok"


async def test_structured_chat_invalid_json_from_model() -> None:
    provider = _make_provider(_MockTransport(200, _ollama_body("not json at all")))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.INVALID_MODEL_OUTPUT


async def test_structured_chat_schema_mismatch() -> None:
    provider = _make_provider(_MockTransport(200, _ollama_body('{"wrong_field": "value"}')))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.INVALID_MODEL_OUTPUT


async def test_structured_chat_non_200_raises_provider_error() -> None:
    provider = _make_provider(_MockTransport(500, b'{"error": "internal server error"}'))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_ERROR


async def test_structured_chat_timeout_raises_provider_timeout() -> None:
    provider = _make_provider(_TimeoutTransport())
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_TIMEOUT


async def test_structured_chat_empty_content_raises_provider_error() -> None:
    body = json.dumps({"model": "test-model", "message": {"role": "assistant", "content": ""}, "done": True}).encode()
    provider = _make_provider(_MockTransport(200, body))
    with pytest.raises(AgentServiceError) as exc_info:
        await provider.structured_chat("sys", {}, _Schema, timeout_seconds=5.0)
    assert exc_info.value.code == ErrorCode.PROVIDER_ERROR


# --- raw_chat ---

async def test_raw_chat_returns_content_string() -> None:
    provider = _make_provider(_MockTransport(200, _ollama_body('{"key": "value"}')))
    result = await provider.raw_chat("sys", "user message", timeout_seconds=5.0)
    assert result == '{"key": "value"}'


# --- ping ---

async def test_ping_returns_true_on_200() -> None:
    body = json.dumps({"version": "0.5.1"}).encode()
    provider = _make_provider(_MockTransport(200, body))
    assert await provider.ping() is True


async def test_ping_returns_false_on_connection_error() -> None:
    provider = _make_provider(_ConnectErrorTransport())
    assert await provider.ping() is False
