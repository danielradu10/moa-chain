import json
import logging
import time
from typing import TypeVar

import httpx
from pydantic import BaseModel, ValidationError

from errors import AgentServiceError, ErrorCode

logger = logging.getLogger(__name__)
T = TypeVar("T", bound=BaseModel)


class OllamaProvider:
    """HTTP client for a locally running Ollama server.

    Ollama exposes a REST API at http://127.0.0.1:11434 by default.
    It loads model weights from local disk on first use and keeps them
    warm in memory between requests — no external network calls at runtime.
    """

    def __init__(
        self,
        base_url: str,
        model: str,
        temperature: float = 0.0,
        num_ctx: int = 4096,
        num_predict: int = 256,
        think: bool = False,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        self.model = model
        self.temperature = temperature
        self.num_ctx = num_ctx
        self.num_predict = num_predict
        self.think = think

        # Allow injecting a pre-configured httpx client for testing.
        self._client = client or httpx.AsyncClient(base_url=base_url)

    async def structured_chat(
        self,
        system_prompt: str,
        user_payload: dict,
        response_schema: type[T],
        timeout_seconds: float,
        operation: str = "",
    ) -> T:
        """Call Ollama with format="json" and validate the response against response_schema."""

        # Serialize the payload dict to a JSON string so the model receives
        # a structured user message it can mirror back as structured output.
        content = await self._chat(
            system_prompt=system_prompt,
            user_message=json.dumps(user_payload),
            timeout_seconds=timeout_seconds,
            json_format=True,
            operation=operation,
        )

        try:
            data = json.loads(content)
        except json.JSONDecodeError as exc:
            raise AgentServiceError(
                ErrorCode.INVALID_MODEL_OUTPUT,
                f"model returned invalid JSON: {exc}",
                status_code=502,
            ) from exc

        try:
            return response_schema.model_validate(data)
        except ValidationError as exc:
            raise AgentServiceError(
                ErrorCode.INVALID_MODEL_OUTPUT,
                f"model output does not match expected schema: {exc}",
                status_code=502,
            ) from exc

    async def raw_chat(
        self,
        system_prompt: str,
        user_message: str,
        timeout_seconds: float,
        json_format: bool = False,
        operation: str = "",
    ) -> str:
        """Call Ollama and return the raw response string without parsing.

        Used by /judge where Go owns the output schema.
        """
        return await self._chat(
            system_prompt=system_prompt,
            user_message=user_message,
            timeout_seconds=timeout_seconds,
            json_format=json_format,
            operation=operation,
        )

    async def ping(self) -> bool:
        """Check Ollama reachability and model presence without running inference."""
        try:
            response = await self._client.get("/api/tags", timeout=5.0)
            if response.status_code != 200:
                return False
            models = response.json().get("models", [])
            return any(
                self.model in {item.get("name"), item.get("model")}
                for item in models
            )
        except Exception:
            return False

    async def _chat(
        self,
        system_prompt: str,
        user_message: str,
        timeout_seconds: float,
        json_format: bool,
        operation: str = "",
    ) -> str:
        """Send a chat request to /api/chat and return the message content string."""

        payload: dict = {
            "model": self.model,
            "messages": [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_message},
            ],
            "options": {
                "temperature": self.temperature,
                "num_ctx": self.num_ctx,
                "num_predict": self.num_predict,
            },
            "think": self.think,
            # Disable streaming — we want the full response in one JSON object.
            "stream": False,
        }

        # format="json" instructs Ollama to constrain the model to valid JSON output.
        # Used for /label and /answer; omitted for /judge (Go parses the raw string).
        if json_format:
            payload["format"] = "json"

        logger.info(
            "ollama_chat_start model=%s operation=%s temperature=%.2f num_ctx=%d num_predict=%d think=%s timeout_s=%.1f json_format=%s",
            self.model,
            operation or "-",
            self.temperature,
            self.num_ctx,
            self.num_predict,
            self.think,
            timeout_seconds,
            json_format,
        )
        t0 = time.perf_counter()

        try:
            response = await self._client.post(
                "/api/chat",
                json=payload,
                timeout=timeout_seconds,
            )

        except httpx.TimeoutException as exc:
            elapsed_ms = (time.perf_counter() - t0) * 1000
            logger.error(
                "ollama_chat_timeout model=%s operation=%s elapsed_ms=%.0f timeout_s=%.1f",
                self.model, operation or "-", elapsed_ms, timeout_seconds,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_TIMEOUT,
                f"ollama request timed out after {timeout_seconds}s",
                status_code=504,
            ) from exc

        except httpx.RequestError as exc:
            elapsed_ms = (time.perf_counter() - t0) * 1000
            logger.error(
                "ollama_chat_connection_error model=%s operation=%s elapsed_ms=%.0f error=%s",
                self.model, operation or "-", elapsed_ms, exc,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                f"ollama request failed: {exc}",
                status_code=502,
            ) from exc

        elapsed_ms = (time.perf_counter() - t0) * 1000

        if response.status_code != 200:
            logger.error(
                "ollama_chat_http_error model=%s operation=%s status=%d elapsed_ms=%.0f",
                self.model, operation or "-", response.status_code, elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                f"ollama returned HTTP {response.status_code}",
                status_code=502,
            )

        try:
            data = response.json()
        except json.JSONDecodeError as exc:
            logger.error(
                "ollama_chat_invalid_json model=%s operation=%s elapsed_ms=%.0f",
                self.model, operation or "-", elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                f"ollama response is not valid JSON: {exc}",
                status_code=502,
            ) from exc

        # Extract the assistant message content from the Ollama response envelope.
        content: str = data.get("message", {}).get("content", "")
        if not content:
            logger.error(
                "ollama_chat_empty_content model=%s operation=%s elapsed_ms=%.0f",
                self.model, operation or "-", elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                "ollama returned an empty message content",
                status_code=502,
            )

        # Ollama includes token counts in non-streaming responses.
        # prompt_eval_count = input tokens, eval_count = output tokens.
        input_tokens: int | None = data.get("prompt_eval_count")
        output_tokens: int | None = data.get("eval_count")
        total_tokens: int | None = (
            input_tokens + output_tokens
            if input_tokens is not None and output_tokens is not None
            else None
        )

        logger.info(
            "llm_call provider=ollama model=%s operation=%s "
            "latency_ms=%.0f input_tokens=%s output_tokens=%s total_tokens=%s",
            self.model,
            operation or "-",
            elapsed_ms,
            input_tokens,
            output_tokens,
            total_tokens,
        )
        return content
