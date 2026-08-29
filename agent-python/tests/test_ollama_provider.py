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
        self.last_request: httpx.Request | None = None

    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        self.last_request = request
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


async def test_raw_chat_sends_qualified_generation_options() -> None:
    transport = _MockTransport(200, _ollama_body('{"key": "value"}'))
    client = httpx.AsyncClient(transport=transport, base_url="http://test")
    provider = OllamaProvider(
        base_url="http://test", model="test-model", temperature=0.0,
        num_ctx=4096, num_predict=256, think=False, client=client,
    )
    await provider.raw_chat("sys", "user", timeout_seconds=5.0, json_format=True)
    assert transport.last_request is not None
    payload = json.loads(transport.last_request.content)
    assert payload["stream"] is False
    assert payload["think"] is False
    assert payload["options"] == {
        "temperature": 0.0, "num_ctx": 4096, "num_predict": 256,
    }
    assert payload["format"] == "json"


# --- ping ---

async def test_ping_returns_true_when_model_is_present() -> None:
    body = json.dumps({"models": [{"name": "test-model", "model": "test-model"}]}).encode()
    provider = _make_provider(_MockTransport(200, body))
    assert await provider.ping() is True


async def test_ping_returns_false_when_model_is_absent() -> None:
    body = json.dumps({"models": [{"name": "another-model"}]}).encode()
    provider = _make_provider(_MockTransport(200, body))
    assert await provider.ping() is False


async def test_ping_returns_false_on_connection_error() -> None:
    provider = _make_provider(_ConnectErrorTransport())
    assert await provider.ping() is False


# --- token instrumentation ---

def _ollama_body_with_tokens(content: str, prompt_eval_count: int, eval_count: int) -> bytes:
    return json.dumps({
        "model": "test-model",
        "message": {"role": "assistant", "content": content},
        "done": True,
        "prompt_eval_count": prompt_eval_count,
        "eval_count": eval_count,
    }).encode()


async def test_llm_call_log_contains_token_counts(caplog) -> None:
    import logging
    body = _ollama_body_with_tokens('{"result": "ok"}', prompt_eval_count=30, eval_count=15)
    provider = _make_provider(_MockTransport(200, body))
    with caplog.at_level(logging.INFO, logger="providers.ollama_provider"):
        await provider.raw_chat("sys", "user", timeout_seconds=5.0)

    llm_call_lines = [r.message for r in caplog.records if r.message.startswith("llm_call")]
    assert len(llm_call_lines) == 1
    line = llm_call_lines[0]
    assert "provider=ollama" in line
    assert "input_tokens=30" in line
    assert "output_tokens=15" in line
    assert "total_tokens=45" in line


async def test_llm_call_log_contains_operation(caplog) -> None:
    import logging
    body = _ollama_body_with_tokens("hello", prompt_eval_count=10, eval_count=5)
    provider = _make_provider(_MockTransport(200, body))
    with caplog.at_level(logging.INFO, logger="providers.ollama_provider"):
        await provider.raw_chat("sys", "user", timeout_seconds=5.0, operation="judge")

    llm_call_lines = [r.message for r in caplog.records if r.message.startswith("llm_call")]
    assert any("operation=judge" in line for line in llm_call_lines)


async def test_llm_call_log_tokens_none_when_absent(caplog) -> None:
    """Ollama may omit token counts on cached responses — None should not crash."""
    import logging
    provider = _make_provider(_MockTransport(200, _ollama_body("hello")))
    with caplog.at_level(logging.INFO, logger="providers.ollama_provider"):
        await provider.raw_chat("sys", "user", timeout_seconds=5.0)

    llm_call_lines = [r.message for r in caplog.records if r.message.startswith("llm_call")]
    assert len(llm_call_lines) == 1
    assert "input_tokens=None" in llm_call_lines[0]
    assert "output_tokens=None" in llm_call_lines[0]
