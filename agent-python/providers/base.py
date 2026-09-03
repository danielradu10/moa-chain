from typing import Protocol, TypeVar, runtime_checkable

from pydantic import BaseModel

T = TypeVar("T", bound=BaseModel)


# LLMProvider is a structural Protocol — any class that implements these three
# methods is a valid provider, without needing to inherit from this class.
# This allows OllamaProvider and FakeProvider to be swapped transparently.
@runtime_checkable
class LLMProvider(Protocol):

    async def structured_chat(
        self,
        system_prompt: str,
        user_payload: dict,
        response_schema: type[T],
        timeout_seconds: float,
        operation: str = "",
    ) -> T:
        """Call the LLM and parse the response into response_schema.

        Used by /label and /answer where the Python side owns the output schema.
        Raises AgentServiceError on timeout, provider error, invalid JSON,
        or schema mismatch.
        `operation` is included in the llm_call structured log line.
        """
        ...

    async def raw_chat(
        self,
        system_prompt: str,
        user_message: str,
        timeout_seconds: float,
        json_format: bool = False,
        operation: str = "",
    ) -> str:
        """Call the LLM and return the raw response string.

        Used by /judge where Go owns the schema and parses the output itself.
        Raises AgentServiceError on timeout or provider error.
        `operation` is included in the llm_call structured log line.
        """
        ...

    async def ping(self) -> bool:
        """Return True if the provider is reachable and the model can serve a request, False otherwise.

        Must never raise — used by the health endpoint.
        """
        ...
