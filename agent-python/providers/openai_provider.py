import json
import logging
import time
from typing import TypeVar

import openai
from pydantic import BaseModel, ValidationError

from errors import AgentServiceError, ErrorCode

logger = logging.getLogger(__name__)
T = TypeVar("T", bound=BaseModel)


class OpenAIProvider:
    """Async client for the OpenAI Chat Completions API.

    Implements the same LLMProvider protocol as OllamaProvider so that routers
    can use either backend without any conditional logic.

    JSON structured output uses OpenAI's json_object response format.
    The system or user prompt must mention "json" for the mode to activate —
    all existing MoA prompts already do this.
    """

    def __init__(
        self,
        api_key: str,
        model: str,
        temperature: float = 0.5,
        timeout_seconds: float = 60.0,
        client: openai.AsyncOpenAI | None = None,
    ) -> None:
        self.model = model
        self.temperature = temperature
        self.timeout_seconds = timeout_seconds

        # Allow injecting a pre-configured client for testing.
        self._client = client or openai.AsyncOpenAI(
            api_key=api_key,
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
        """Call OpenAI with json_object response format and validate against response_schema."""
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
        """Call OpenAI and return the raw response string without parsing.

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
        """Check API reachability and model availability without running inference."""
        try:
            await self._client.models.retrieve(self.model)
            return True
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

        # json_object mode constrains the model to emit valid JSON.
        # OpenAI requires the word "json" to appear somewhere in the prompt
        # when this mode is active — MoA prompts already satisfy this.
        if json_format:
            kwargs["response_format"] = {"type": "json_object"}

        logger.info(
            "openai_chat_start model=%s operation=%s temperature=%.2f json_format=%s timeout_s=%.1f",
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
                "openai_chat_timeout model=%s operation=%s elapsed_ms=%.0f",
                self.model, operation or "-", elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_TIMEOUT,
                f"OpenAI request timed out after {timeout_seconds}s",
                status_code=504,
            ) from exc

        except openai.APIConnectionError as exc:
            elapsed_ms = (time.perf_counter() - t0) * 1000
            logger.error(
                "openai_chat_connection_error model=%s operation=%s elapsed_ms=%.0f error=%s",
                self.model, operation or "-", elapsed_ms, exc,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                f"OpenAI connection error: {exc}",
                status_code=502,
            ) from exc

        except openai.APIStatusError as exc:
            elapsed_ms = (time.perf_counter() - t0) * 1000
            logger.error(
                "openai_chat_http_error model=%s operation=%s status=%d elapsed_ms=%.0f",
                self.model, operation or "-", exc.status_code, elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                f"OpenAI returned HTTP {exc.status_code}: {exc.message}",
                status_code=502,
            ) from exc

        elapsed_ms = (time.perf_counter() - t0) * 1000
        content: str = response.choices[0].message.content or ""

        if not content:
            logger.error(
                "openai_chat_empty_content model=%s operation=%s elapsed_ms=%.0f",
                self.model, operation or "-", elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                "OpenAI returned an empty message content",
                status_code=502,
            )

        # Extract token counts from the usage object (always present for non-streaming calls).
        usage = response.usage
        input_tokens = usage.prompt_tokens if usage else None
        output_tokens = usage.completion_tokens if usage else None
        total_tokens = usage.total_tokens if usage else None

        logger.info(
            "llm_call provider=openai model=%s operation=%s "
            "latency_ms=%.0f input_tokens=%s output_tokens=%s total_tokens=%s",
            self.model,
            operation or "-",
            elapsed_ms,
            input_tokens,
            output_tokens,
            total_tokens,
        )

        return content
