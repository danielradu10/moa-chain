from contextlib import asynccontextmanager

from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

from config import settings
from errors import AgentServiceError, ErrorCode, ErrorResponse
from prompts.loader import load_protocol_prompt
from providers.ollama_provider import OllamaProvider
from routers import answer, health, label


@asynccontextmanager
async def lifespan(app: FastAPI):
    # Store config so routers can access it via request.app.state.config.
    app.state.config = settings

    # Single LLM provider instance shared across all requests.
    # In production this points at the local Ollama server.
    # Tests replace this with FakeProvider after startup.
    app.state.provider = OllamaProvider(
        base_url=settings.ollama_base_url,
        model=settings.ollama_model,
        temperature=settings.llm_temperature,
    )

    # Load and hash versioned prompt files once at startup.
    # If a file is missing this raises immediately — fail fast rather than
    # serving requests with a broken prompt.
    app.state.prompts = {
        "labeler_v1": load_protocol_prompt("labeler_v1"),
        "answerer_v1": load_protocol_prompt("answerer_v1"),
    }

    yield


app = FastAPI(lifespan=lifespan)

app.include_router(health.router)
app.include_router(label.router)
app.include_router(answer.router)


# All AgentServiceError exceptions are caught here and returned as structured JSON.
# Stack traces are never exposed — only the error code and a human-readable detail.
@app.exception_handler(AgentServiceError)
async def agent_service_error_handler(
    request: Request, exc: AgentServiceError
) -> JSONResponse:
    return JSONResponse(
        status_code=exc.status_code,
        content=ErrorResponse(error=exc.code, detail=exc.detail).model_dump(),
    )


# FastAPI raises RequestValidationError when the request body fails Pydantic validation.
# We map it to INVALID_REQUEST so the client gets a consistent error shape.
@app.exception_handler(RequestValidationError)
async def validation_error_handler(
    request: Request, exc: RequestValidationError
) -> JSONResponse:
    return JSONResponse(
        status_code=422,
        content=ErrorResponse(
            error=ErrorCode.INVALID_REQUEST,
            detail=str(exc),
        ).model_dump(),
    )


# Catches 404 (unknown route) and other Starlette HTTP errors.
# Returns the same structured error shape as everything else.
@app.exception_handler(StarletteHTTPException)
async def http_exception_handler(
    request: Request, exc: StarletteHTTPException
) -> JSONResponse:
    return JSONResponse(
        status_code=exc.status_code,
        content=ErrorResponse(
            error=ErrorCode.INVALID_REQUEST,
            detail=exc.detail,
        ).model_dump(),
    )
