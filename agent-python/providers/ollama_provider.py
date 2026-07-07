import json
from typing import TypeVar

import httpx
from pydantic import BaseModel, ValidationError

from errors import AgentServiceError, ErrorCode

T = TypeVar("T", bound=BaseModel)


class OllamaProvider:
    def __init__(
        self,
        base_url: str,
        model: str,
        temperature: float = 0.0,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        self.model = model
        self.temperature = temperature
        self._client = client or httpx.AsyncClient(base_url=base_url)

    async def structured_chat(
        self,
        system_prompt: str,
        user_payload: dict,
        response_schema: type[T],
        timeout_seconds: float,
    ) -> T:
        content = await self._chat(
            system_prompt=system_prompt,
            user_message=json.dumps(user_payload),
            timeout_seconds=timeout_seconds,
            json_format=True,
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
    ) -> str:
        return await self._chat(
            system_prompt=system_prompt,
            user_message=user_message,
            timeout_seconds=timeout_seconds,
            json_format=False,
        )

    async def ping(self) -> bool:
        try:
            response = await self._client.get("/api/version", timeout=5.0)
            return response.status_code == 200
        except Exception:
            return False

    async def _chat(
        self,
        system_prompt: str,
        user_message: str,
        timeout_seconds: float,
        json_format: bool,
    ) -> str:
        payload: dict = {
            "model": self.model,
            "messages": [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_message},
            ],
            "options": {"temperature": self.temperature},
            "stream": False,
        }
        if json_format:
            payload["format"] = "json"

        try:
            response = await self._client.post(
                "/api/chat",
                json=payload,
                timeout=timeout_seconds,
            )

        except httpx.TimeoutException as exc:
            raise AgentServiceError(
                ErrorCode.PROVIDER_TIMEOUT,
                f"ollama request timed out after {timeout_seconds}s",
                status_code=504,
            ) from exc

        except httpx.RequestError as exc:
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                f"ollama request failed: {exc}",
                status_code=502,
            ) from exc

        if response.status_code != 200:
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                f"ollama returned HTTP {response.status_code}",
                status_code=502,
            )

        try:
            data = response.json()
        except json.JSONDecodeError as exc:
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                f"ollama response is not valid JSON: {exc}",
                status_code=502,
            ) from exc

        content: str = data.get("message", {}).get("content", "")
        if not content:
            raise AgentServiceError(
                ErrorCode.PROVIDER_ERROR,
                "ollama returned an empty message content",
                status_code=502,
            )

        return content
