from typing import Protocol, TypeVar, runtime_checkable

from pydantic import BaseModel

T = TypeVar("T", bound=BaseModel)


@runtime_checkable
class LLMProvider(Protocol):
    async def structured_chat(
        self,
        system_prompt: str,
        user_payload: dict,
        response_schema: type[T],
        timeout_seconds: float,
    ) -> T:
        """Call the LLM and parse the response into response_schema.

        Raises AgentServiceError on timeout, provider error, invalid JSON,
        or schema mismatch.
        """
        ...

    async def raw_chat(
        self,
        system_prompt: str,
        user_message: str,
        timeout_seconds: float,
    ) -> str:
        """Call the LLM and return the raw response string.

        Used by /judge where Go owns the schema and parses the output itself.
        Raises AgentServiceError on timeout or provider error.
        """
        ...
