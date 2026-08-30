"""Local provider sentinel for deterministic Byzantine experiment agents."""

from errors import AgentServiceError, ErrorCode


class MockProvider:
    """Never performs network or external-provider calls."""

    async def structured_chat(self, **_kwargs):
        raise AgentServiceError(
            ErrorCode.PROVIDER_ERROR,
            "mock operation has no deterministic response configured",
            500,
        )

    async def raw_chat(self, **_kwargs):
        raise AgentServiceError(
            ErrorCode.PROVIDER_ERROR,
            "mock operation has no deterministic response configured",
            500,
        )

    async def ping(self) -> bool:
        return True
