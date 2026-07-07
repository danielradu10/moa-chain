from typing import TypeVar

from pydantic import BaseModel

T = TypeVar("T", bound=BaseModel)


class FakeProvider:
    """Configurable fake LLM provider for tests.

    Configure before each test via set_* methods. The provider raises
    the configured error if one is set, otherwise returns the configured
    response. Both structured_chat and raw_chat share the same error slot.
    """

    def __init__(self) -> None:
        self._structured_response: BaseModel | None = None
        self._raw_response: str | None = None
        self._error: Exception | None = None

    def set_structured_response(self, response: BaseModel) -> None:
        self._structured_response = response
        self._error = None

    def set_raw_response(self, response: str) -> None:
        self._raw_response = response
        self._error = None

    def set_error(self, error: Exception) -> None:
        self._error = error

    async def structured_chat(
        self,
        system_prompt: str,
        user_payload: dict,
        response_schema: type[T],
        timeout_seconds: float,
    ) -> T:
        if self._error is not None:
            raise self._error
        if self._structured_response is None:
            raise ValueError("FakeProvider: no structured response configured")
        return self._structured_response  # type: ignore[return-value]

    async def raw_chat(
        self,
        system_prompt: str,
        user_message: str,
        timeout_seconds: float,
    ) -> str:
        if self._error is not None:
            raise self._error
        if self._raw_response is None:
            raise ValueError("FakeProvider: no raw response configured")
        return self._raw_response
