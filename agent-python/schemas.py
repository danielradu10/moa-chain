from pydantic import BaseModel


# --- /label schemas ---


class LabelEntry(BaseModel):
    subdomain: str    # one of the allowed_subdomains from the request
    confidence: float # model's certainty in [0.0, 1.0]


class LabelTransactionInput(BaseModel):
    tx_hash: str  # protocol transaction hash, echoed back in the response for matching
    prompt: str   # the coding question to classify


# LabelResult is used both as the per-item in LabelResponse.results AND as the
# response_schema passed to structured_chat — the model must output this exact shape.
class LabelResult(BaseModel):
    tx_hash: str
    labels: list[LabelEntry]


class LabelRequest(BaseModel):
    prompt_version: str           # must match the loaded labeler prompt version
    allowed_subdomains: list[str] # complete set of valid subdomains for this block
    transactions: list[LabelTransactionInput]


class LabelResponse(BaseModel):
    prompt_version: str  # loaded version, returned so Go can verify
    prompt_hash: str     # SHA-256 hex of the prompt file, returned so Go can verify
    results: list[LabelResult]


# --- /answer schemas ---

class AnswerTransactionInput(BaseModel):
    tx_hash: str           # protocol transaction hash, echoed back in the response for matching
    prompt: str            # the coding question to answer
    subdomains: list[str]  # relevant subdomains assigned during MR1, used to focus the answer


# AnswerResult is used both as the per-item in AnswerResponse.results AND as the
# response_schema passed to structured_chat — the model must output this exact shape.
class AnswerResult(BaseModel):
    tx_hash: str
    answer: str  # non-empty answer string after trimming


class AnswerRequest(BaseModel):
    prompt_version: str  # must match the loaded answerer prompt version
    transactions: list[AnswerTransactionInput]


class AnswerResponse(BaseModel):
    prompt_version: str  # loaded version, returned so Go can verify
    prompt_hash: str     # SHA-256 hex of the prompt file, returned so Go can verify
    results: list[AnswerResult]
