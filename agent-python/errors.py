from enum import Enum

from pydantic import BaseModel


# All error codes the service can return. Go maps these to typed Go errors
# in the HTTP client (agent/httpclient/).
class ErrorCode(str, Enum):
    INVALID_REQUEST = "INVALID_REQUEST"           # malformed or missing request fields
    PROMPT_VERSION_MISMATCH = "PROMPT_VERSION_MISMATCH"  # Go sent a version we don't have loaded
    PROVIDER_ERROR = "PROVIDER_ERROR"             # Ollama returned a non-200 or unexpected response
    PROVIDER_TIMEOUT = "PROVIDER_TIMEOUT"         # Ollama did not respond within LLM_TIMEOUT_SECONDS
    INVALID_MODEL_OUTPUT = "INVALID_MODEL_OUTPUT" # model output failed JSON parse or schema validation
    COVERAGE_MISMATCH = "COVERAGE_MISMATCH"       # response tx_hash set does not match request set
    UNKNOWN_CATEGORY = "UNKNOWN_CATEGORY"         # judge response contains a category outside the allowed set
    UNKNOWN_SUBDOMAIN = "UNKNOWN_SUBDOMAIN"       # label response contains a subdomain not in allowed_subdomains
    EMPTY_ANSWER = "EMPTY_ANSWER"                 # answer or reason field is empty after trimming


# Shape of every error response body. Stack traces are never included.
class ErrorResponse(BaseModel):
    error: ErrorCode
    detail: str


# Base exception for all service-level errors.
# Routers raise this; app.py catches it and converts it to ErrorResponse JSON.
class AgentServiceError(Exception):
    def __init__(self, code: ErrorCode, detail: str, status_code: int = 400) -> None:
        super().__init__(detail)
        self.code = code
        self.detail = detail
        self.status_code = status_code
