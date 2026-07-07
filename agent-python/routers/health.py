from fastapi import APIRouter, Request
from pydantic import BaseModel

router = APIRouter()


class HealthResponse(BaseModel):
    status: str
    provider: str
    model: str
    reachable: bool
    # prompt_versions and prompt_hashes added in PR 3


@router.get("/health", response_model=HealthResponse)
async def health(request: Request) -> HealthResponse:
    cfg = request.app.state.config
    provider = request.app.state.provider
    reachable = await provider.ping()
    return HealthResponse(
        status="ok",
        provider=cfg.llm_provider,
        model=cfg.ollama_model,
        reachable=reachable,
    )
