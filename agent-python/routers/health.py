from fastapi import APIRouter, Request
from pydantic import BaseModel

router = APIRouter()


class HealthResponse(BaseModel):
    status: str
    provider: str
    model: str
    reachable: bool
    prompt_versions: dict[str, str]
    prompt_hashes: dict[str, str]


@router.get("/health", response_model=HealthResponse)
async def health(request: Request) -> HealthResponse:
    cfg = request.app.state.config
    provider = request.app.state.provider
    reachable = await provider.ping()
    prompts = request.app.state.prompts
    return HealthResponse(
        status="ok",
        provider=cfg.llm_provider,
        model=cfg.ollama_model,
        reachable=reachable,
        prompt_versions={name: p.version for name, p in prompts.items()},
        prompt_hashes={name: p.sha256_hash for name, p in prompts.items()},
    )
