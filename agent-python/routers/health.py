from fastapi import APIRouter, Request
from pydantic import BaseModel

router = APIRouter()


class HealthResponse(BaseModel):
    status: str
    provider: str
    model: str

    # False if the provider is not reachable or the model is unavailable.
    # Does not cause a non-200 response — signals to the caller that LLM
    # calls will fail until the provider is ready.
    reachable: bool

    # Prompt version strings returned so Go can verify the service is running
    # the expected prompt versions before sending real requests.
    prompt_versions: dict[str, str]
    prompt_hashes: dict[str, str]


@router.get("/health", response_model=HealthResponse)
async def health(request: Request) -> HealthResponse:
    cfg = request.app.state.config
    provider = request.app.state.provider
    prompts = request.app.state.prompts

    reachable = await provider.ping()

    return HealthResponse(
        status="ok",
        provider=cfg.reported_provider,
        model=cfg.reported_model,
        reachable=reachable,
        prompt_versions={name: p.version for name, p in prompts.items()},
        prompt_hashes={name: p.sha256_hash for name, p in prompts.items()},
    )
