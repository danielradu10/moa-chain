import json
import logging
import time
from typing import TypeVar

import openai
from pydantic import BaseModel

from errors import AgentServiceError, ErrorCode
from providers.parsing import validate_schema_with_unwrap

logger = logging.getLogger(__name__)
T = TypeVar("T", bound=BaseModel)


class DeepSeekProvider:
    """Async DeepSeek Chat Completions client.

    DeepSeek exposes an OpenAI-compatible HTTP API, so this provider reuses the
    installed OpenAI SDK with DeepSeek's base URL. DeepSeek-specific request,
    response, health, logging, and error behavior remains isolated here.
    """

    def __init__(
        self,
        api_key: str,
        model: str,
        base_url: str = "https://api.deepseek.com",
        temperature: float = 0.5,
        timeout_seconds: float = 60.0,
        client: openai.AsyncOpenAI | None = None,
    ) -> None:
        self.model = model
        self.base_url = base_url
        self.temperature = temperature
        self.timeout_seconds = timeout_seconds
        self._client = client or openai.AsyncOpenAI(
            api_key=api_key,
            base_url=base_url,
            timeout=timeout_seconds,
        )

    async def structured_chat(
        self,
        system_prompt: str,
        user_payload: dict,
        response_schema: type[T],
        timeout_seconds: float,
        operation: str = "",
    ) -> T:
        """Request DeepSeek JSON mode and validate against the Pydantic schema."""
        content = await self._chat(
            system_prompt=system_prompt,
            user_message=json.dumps(user_payload),
            timeout_seconds=timeout_seconds,
            json_format=True,
            operation=operation,
        )
        data = _parse_json(content)
        return validate_schema_with_unwrap(data, response_schema)

    async def raw_chat(
        self,
        system_prompt: str,
        user_message: str,
        timeout_seconds: float,
        json_format: bool = False,
        operation: str = "",
    ) -> str:
        """Return DeepSeek text, optionally constrained to a JSON object."""
        return await self._chat(
            system_prompt=system_prompt,
            user_message=user_message,
            timeout_seconds=timeout_seconds,
            json_format=json_format,
            operation=operation,
        )

    async def ping(self) -> bool:
        """Check API reachability and that the configured model is listed."""
        try:
            models = await self._client.models.list()
            return any(item.id == self.model for item in models.data)
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
        kwargs: dict = {
            "model": self.model,
            "messages": [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_message},
            ],
            "temperature": self.temperature,
            "timeout": timeout_seconds,
        }
        if json_format:
            kwargs["response_format"] = {"type": "json_object"}

        logger.info(
            "deepseek_chat_start model=%s operation=%s temperature=%.2f "
            "json_format=%s timeout_s=%.1f",
            self.model,
            operation or "-",
            self.temperature,
            json_format,
            timeout_seconds,
        )
        t0 = time.perf_counter()

        try:
            response = await self._client.chat.completions.create(**kwargs)
        except openai.APITimeoutError as exc:
            elapsed_ms = (time.perf_counter() - t0) * 1000
            logger.error(
                "deepseek_chat_timeout model=%s operation=%s elapsed_ms=%.0f",
                self.model,
                operation or "-",
                elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_TIMEOUT,
                f"DeepSeek request timed out after {timeout_seconds}s",
                status_code=504,
            ) from exc
        except openai.APIConnectionError as exc:
            elapsed_ms = (time.perf_counter() - t0) * 1000
            logger.error(
                "deepseek_chat_connection_error model=%s operation=%s "
                "elapsed_ms=%.0f error=%s",
                self.model,
                operation or "-",
                elapsed_ms,
                exc,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                f"DeepSeek connection error: {exc}",
                status_code=502,
            ) from exc
        except openai.APIStatusError as exc:
            elapsed_ms = (time.perf_counter() - t0) * 1000
            logger.error(
                "deepseek_chat_http_error model=%s operation=%s status=%d "
                "elapsed_ms=%.0f",
                self.model,
                operation or "-",
                exc.status_code,
                elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                f"DeepSeek returned HTTP {exc.status_code}: {exc.message}",
                status_code=502,
            ) from exc

        elapsed_ms = (time.perf_counter() - t0) * 1000
        content: str = response.choices[0].message.content or ""
        if not content:
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                "DeepSeek returned empty message content",
                status_code=502,
            )

        usage = response.usage
        input_tokens = usage.prompt_tokens if usage else None
        output_tokens = usage.completion_tokens if usage else None
        total_tokens = usage.total_tokens if usage else None
        logger.info(
            "llm_call provider=deepseek model=%s operation=%s "
            "latency_ms=%.0f input_tokens=%s output_tokens=%s total_tokens=%s",
            self.model,
            operation or "-",
            elapsed_ms,
            input_tokens,
            output_tokens,
            total_tokens,
        )
        return content


def _parse_json(content: str) -> object:
    """Parse DeepSeek JSON mode without weakening shared schema validation."""
    stripped = content.strip()
    if stripped.startswith("```json") and stripped.endswith("```"):
        stripped = stripped[len("```json"):-len("```")].strip()
    try:
        return json.loads(stripped)
    except json.JSONDecodeError as exc:
        raise AgentServiceError(
            ErrorCode.INVALID_MODEL_OUTPUT,
            f"model returned invalid JSON: {exc}",
            status_code=502,
        ) from exc
