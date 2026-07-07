from contextlib import asynccontextmanager

from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

from config import Settings, settings
from errors import AgentServiceError, ErrorCode, ErrorResponse
from routers import health


@asynccontextmanager
async def lifespan(app: FastAPI):
    app.state.config = settings
    # PR 2: initialize and store OllamaProvider here
    # PR 3: load and hash protocol prompt files here
    yield


app = FastAPI(lifespan=lifespan)

app.include_router(health.router)


@app.exception_handler(AgentServiceError)
async def agent_service_error_handler(
    request: Request, exc: AgentServiceError
) -> JSONResponse:
    return JSONResponse(
        status_code=exc.status_code,
        content=ErrorResponse(error=exc.code, detail=exc.detail).model_dump(),
    )


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
