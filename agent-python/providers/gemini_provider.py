import json
import logging
import time
from typing import TypeVar

import httpx
import google.genai as genai
from google.genai import errors as genai_errors, types
from pydantic import BaseModel

from errors import AgentServiceError, ErrorCode
from providers.parsing import validate_schema_with_unwrap

logger = logging.getLogger(__name__)
T = TypeVar("T", bound=BaseModel)


class GeminiProvider:
    """Async client for the Google Gemini API via the google-genai SDK.

    Implements the same LLMProvider protocol as the other providers so routers
    and the qualification harness can use any backend without conditional logic.

    Key Gemini specifics handled here:
      - system prompt is passed via GenerateContentConfig.system_instruction;
      - native JSON output mode is enabled via response_mime_type="application/json";
      - timeout is set via HttpOptions.timeout (milliseconds) on the client; the SDK's
        default tenacity retry loop is disabled so timeouts propagate immediately rather
        than being silently retried up to 5 times;
      - token counts come from response.usage_metadata.{prompt,candidates,total}_token_count.
    """

    def __init__(
        self,
        api_key: str,
        model: str,
        temperature: float = 0.5,
        timeout_seconds: float = 60.0,
        client: genai.Client | None = None,
    ) -> None:
        self.model = model
        self.temperature = temperature
        self.timeout_seconds = timeout_seconds
        # HttpOptions.timeout is in milliseconds (SDK converts to seconds internally).
        # retry_options=None disables the SDK's default tenacity retry loop (5 attempts
        # with exponential backoff) so that httpx.TimeoutException propagates immediately
        # instead of being silently retried for up to several minutes.
        self._client = client or genai.Client(
            api_key=api_key,
            http_options=types.HttpOptions(
                timeout=int(timeout_seconds * 1000),
                retry_options=None,
            ),
        )

    async def structured_chat(
        self,
        system_prompt: str,
        user_payload: dict,
        response_schema: type[T],
        timeout_seconds: float,
        operation: str = "",
    ) -> T:
        """Call Gemini and validate the response against response_schema.

        response_mime_type="application/json" is always set so Gemini's native
        JSON mode constrains the output. validate_schema_with_unwrap handles any
        results-wrapper normalization.
        """
        content = await self._chat(
            system_prompt=system_prompt,
            user_message=json.dumps(user_payload),
            timeout_seconds=timeout_seconds,
            operation=operation,
            json_mode=True,
        )

        try:
            data = json.loads(content)
        except json.JSONDecodeError as exc:
            raise AgentServiceError(
                ErrorCode.INVALID_MODEL_OUTPUT,
                f"model returned invalid JSON: {exc}",
                status_code=502,
            ) from exc

        return validate_schema_with_unwrap(data, response_schema)

    async def raw_chat(
        self,
        system_prompt: str,
        user_message: str,
        timeout_seconds: float,
        json_format: bool = False,
        operation: str = "",
    ) -> str:
        """Call Gemini and return the raw response string without parsing.

        When json_format=True, enables Gemini's native JSON mode via
        response_mime_type — equivalent to OpenAI's json_object response format.
        """
        return await self._chat(
            system_prompt=system_prompt,
            user_message=user_message,
            timeout_seconds=timeout_seconds,
            operation=operation,
            json_mode=json_format,
        )

    async def ping(self) -> bool:
        """Check API reachability by fetching model metadata — no tokens consumed."""
        try:
            await self._client.aio.models.get(model=self.model)
            return True
        except Exception:
            return False

    async def _chat(
        self,
        system_prompt: str,
        user_message: str,
        timeout_seconds: float,
        operation: str = "",
        json_mode: bool = False,
    ) -> str:
        config_kwargs: dict = {
            "system_instruction": system_prompt,
            "temperature": self.temperature,
        }
        if json_mode:
            config_kwargs["response_mime_type"] = "application/json"
        config = types.GenerateContentConfig(**config_kwargs)

        t0 = time.perf_counter()

        try:
            response = await self._client.aio.models.generate_content(
                model=self.model,
                contents=user_message,
                config=config,
            )

        except httpx.TimeoutException as exc:
            elapsed_ms = (time.perf_counter() - t0) * 1000
            logger.error(
                "gemini_chat_timeout model=%s operation=%s elapsed_ms=%.0f",
                self.model, operation or "-", elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_TIMEOUT,
                f"Gemini request timed out after {self.timeout_seconds}s",
                status_code=504,
            ) from exc

        except genai_errors.ClientError as exc:
            elapsed_ms = (time.perf_counter() - t0) * 1000
            logger.error(
                "gemini_chat_client_error model=%s operation=%s code=%d elapsed_ms=%.0f",
                self.model, operation or "-", exc.code, elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                f"Gemini client error {exc.code}: {exc.message}",
                status_code=502,
            ) from exc

        except genai_errors.ServerError as exc:
            elapsed_ms = (time.perf_counter() - t0) * 1000
            logger.error(
                "gemini_chat_server_error model=%s operation=%s code=%d elapsed_ms=%.0f",
                self.model, operation or "-", exc.code, elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                f"Gemini server error {exc.code}: {exc.message}",
                status_code=502,
            ) from exc

        except genai_errors.APIError as exc:
            elapsed_ms = (time.perf_counter() - t0) * 1000
            logger.error(
                "gemini_chat_api_error model=%s operation=%s code=%d elapsed_ms=%.0f",
                self.model, operation or "-", exc.code, elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                f"Gemini API error {exc.code}: {exc.message}",
                status_code=502,
            ) from exc

        elapsed_ms = (time.perf_counter() - t0) * 1000
        content = response.text or ""

        if not content:
            logger.error(
                "gemini_chat_empty_content model=%s operation=%s elapsed_ms=%.0f",
                self.model, operation or "-", elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                "Gemini returned an empty response",
                status_code=502,
            )

        usage = response.usage_metadata
        input_tokens = getattr(usage, "prompt_token_count", None)
        output_tokens = getattr(usage, "candidates_token_count", None)
        total_tokens = getattr(usage, "total_token_count", None)

        logger.info(
            "llm_call provider=gemini model=%s operation=%s "
            "latency_ms=%.0f input_tokens=%s output_tokens=%s total_tokens=%s",
            self.model,
            operation or "-",
            elapsed_ms,
            input_tokens,
            output_tokens,
            total_tokens,
        )

        return content
