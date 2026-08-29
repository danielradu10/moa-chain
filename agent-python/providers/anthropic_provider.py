import json
import logging
import time
from typing import TypeVar

import anthropic
from pydantic import BaseModel

from errors import AgentServiceError, ErrorCode
from providers.parsing import validate_schema_with_unwrap

logger = logging.getLogger(__name__)
T = TypeVar("T", bound=BaseModel)


class AnthropicProvider:
    """Async client for the Anthropic Messages API.

    Implements the same LLMProvider protocol as OllamaProvider and OpenAIProvider
    so that routers and the qualification harness can use any backend without
    conditional logic.

    Key differences from OpenAI handled transparently here:
      - system prompt is a top-level parameter, not a message;
      - max_tokens is required by the API;
      - SDK v1.2+ removed the temperature parameter; output quality/diversity is
        controlled via output_config.effort ("low"/"medium"/"high"/"xhigh"/"max");
      - structured operations use Anthropic's native JSON-schema output format;
        raw judge output is normalized because its schema is owned by Go;
      - token counts come from response.usage.{input_tokens, output_tokens}.
    """

    def __init__(
        self,
        api_key: str,
        model: str,
        effort: str = "medium",
        max_tokens: int = 4096,
        timeout_seconds: float = 60.0,
        client: anthropic.AsyncAnthropic | None = None,
    ) -> None:
        self.model = model
        self.effort = effort
        self.max_tokens = max_tokens
        self.timeout_seconds = timeout_seconds

        self._client = client or anthropic.AsyncAnthropic(
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
        """Call Anthropic and validate the response against response_schema.

        Anthropic's JSON-schema output format guarantees valid JSON. The shared
        validate_schema_with_unwrap() handles any results-wrapper normalization.
        """
        content = await self._chat(
            system_prompt=system_prompt,
            user_message=json.dumps(user_payload),
            timeout_seconds=timeout_seconds,
            operation=operation,
            output_schema=_anthropic_json_schema(response_schema),
        )

        try:
            data = json.loads(_normalize_json_response(content))
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
        """Call Anthropic and return the raw response string without parsing.

        json_format is accepted for protocol compatibility but has no effect on
        the API call — Anthropic does not expose a json_object mode. The judge
        prompt already constrains the model to return JSON.
        """
        content = await self._chat(
            system_prompt=system_prompt,
            user_message=user_message,
            timeout_seconds=timeout_seconds,
            operation=operation,
        )
        return _normalize_json_response(content) if json_format else content

    async def ping(self) -> bool:
        """Check API reachability and model usability with a minimal call."""
        try:
            await self._client.messages.create(
                model=self.model,
                max_tokens=1,
                messages=[{"role": "user", "content": "ping"}],
            )
            return True
        except Exception:
            return False

    async def _chat(
        self,
        system_prompt: str,
        user_message: str,
        timeout_seconds: float,
        operation: str = "",
        output_schema: dict | None = None,
    ) -> str:
        logger.info(
            "anthropic_chat_start model=%s operation=%s effort=%s max_tokens=%d timeout_s=%.1f",
            self.model,
            operation or "-",
            self.effort,
            self.max_tokens,
            timeout_seconds,
        )
        t0 = time.perf_counter()

        try:
            call_kwargs: dict = {
                "model": self.model,
                "max_tokens": self.max_tokens,
                "system": system_prompt,
                "messages": [{"role": "user", "content": user_message}],
                "timeout": timeout_seconds,
            }
            if self.effort:
                call_kwargs["output_config"] = {"effort": self.effort}
            if output_schema is not None:
                call_kwargs.setdefault("output_config", {})["format"] = {
                    "type": "json_schema",
                    "schema": output_schema,
                }
            response = await self._client.messages.create(**call_kwargs)

        except anthropic.APITimeoutError as exc:
            elapsed_ms = (time.perf_counter() - t0) * 1000
            logger.error(
                "anthropic_chat_timeout model=%s operation=%s elapsed_ms=%.0f",
                self.model, operation or "-", elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_TIMEOUT,
                f"Anthropic request timed out after {timeout_seconds}s",
                status_code=504,
            ) from exc

        except anthropic.APIConnectionError as exc:
            elapsed_ms = (time.perf_counter() - t0) * 1000
            logger.error(
                "anthropic_chat_connection_error model=%s operation=%s elapsed_ms=%.0f error=%s",
                self.model, operation or "-", elapsed_ms, exc,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                f"Anthropic connection error: {exc}",
                status_code=502,
            ) from exc

        except anthropic.APIStatusError as exc:
            elapsed_ms = (time.perf_counter() - t0) * 1000
            logger.error(
                "anthropic_chat_http_error model=%s operation=%s status=%d elapsed_ms=%.0f",
                self.model, operation or "-", exc.status_code, elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                f"Anthropic returned HTTP {exc.status_code}: {exc.message}",
                status_code=502,
            ) from exc

        elapsed_ms = (time.perf_counter() - t0) * 1000

        # Extract the text content block. Anthropic returns a list of content
        # blocks; for plain text responses the first block is always a TextBlock.
        text_blocks = [b.text for b in response.content if hasattr(b, "text")]
        content = text_blocks[0] if text_blocks else ""

        if not content:
            logger.error(
                "anthropic_chat_empty_content model=%s operation=%s elapsed_ms=%.0f",
                self.model, operation or "-", elapsed_ms,
            )
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                "Anthropic returned an empty message content",
                status_code=502,
            )

        usage = response.usage
        input_tokens: int | None = getattr(usage, "input_tokens", None)
        output_tokens: int | None = getattr(usage, "output_tokens", None)
        total_tokens: int | None = (
            input_tokens + output_tokens
            if input_tokens is not None and output_tokens is not None
            else None
        )

        logger.info(
            "llm_call provider=anthropic model=%s operation=%s "
            "latency_ms=%.0f input_tokens=%s output_tokens=%s total_tokens=%s",
            self.model,
            operation or "-",
            elapsed_ms,
            input_tokens,
            output_tokens,
            total_tokens,
        )

        return content


def _normalize_json_response(content: str) -> str:
    """Return the first complete JSON value from a JSON-mode Claude response.

    Raw judge responses cannot use an API-level schema because that schema is
    owned by Go. Claude 4.5 commonly follows the requested JSON shape but wraps
    it in `````json`` fences, which makes the downstream strict parser reject
    it. Claude may also prefix the fence/value with a short introduction. The
    decoded value is serialized again so downstream consumers always receive
    strict JSON. If no value can be decoded, the original text is returned and
    downstream validation still fails visibly.
    """
    stripped = content.strip()
    decoder = json.JSONDecoder()
    for index, character in enumerate(stripped):
        if character not in "[{":
            continue
        try:
            value, _ = decoder.raw_decode(stripped[index:])
        except json.JSONDecodeError:
            continue
        return json.dumps(value, separators=(",", ":"), ensure_ascii=False)
    return stripped


def _anthropic_json_schema(schema: type[BaseModel]) -> dict:
    """Build the strict object schema required by Anthropic structured output."""
    result = schema.model_json_schema()

    def make_objects_strict(node: object) -> None:
        if isinstance(node, dict):
            if node.get("type") == "object":
                node["additionalProperties"] = False
            for value in node.values():
                make_objects_strict(value)
        elif isinstance(node, list):
            for value in node:
                make_objects_strict(value)

    make_objects_strict(result)
    return result
