from enum import Enum

from pydantic import BaseModel


class ErrorCode(str, Enum):
    INVALID_REQUEST = "INVALID_REQUEST"
    PROMPT_VERSION_MISMATCH = "PROMPT_VERSION_MISMATCH"
    PROVIDER_ERROR = "PROVIDER_ERROR"
    PROVIDER_TIMEOUT = "PROVIDER_TIMEOUT"
    INVALID_MODEL_OUTPUT = "INVALID_MODEL_OUTPUT"
    COVERAGE_MISMATCH = "COVERAGE_MISMATCH"
    UNKNOWN_CATEGORY = "UNKNOWN_CATEGORY"
    UNKNOWN_SUBDOMAIN = "UNKNOWN_SUBDOMAIN"
    EMPTY_ANSWER = "EMPTY_ANSWER"


class ErrorResponse(BaseModel):
    error: ErrorCode
    detail: str


class AgentServiceError(Exception):
    def __init__(self, code: ErrorCode, detail: str, status_code: int = 400) -> None:
        super().__init__(detail)
        self.code = code
        self.detail = detail
        self.status_code = status_code
