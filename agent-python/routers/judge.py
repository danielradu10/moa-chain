from fastapi import APIRouter, Request

from errors import AgentServiceError, ErrorCode
from schemas import JudgeRequest, JudgeResponse
from validation import check_judge_response

router = APIRouter()


@router.post("/judge", response_model=JudgeResponse)
async def judge(body: JudgeRequest, request: Request) -> JudgeResponse:
    state = request.app.state

    # Go builds both prompt strings — reject empty values before hitting the LLM.
    if not body.system_prompt.strip():
        raise AgentServiceError(ErrorCode.INVALID_REQUEST, "system_prompt is empty")
    if not body.user_prompt.strip():
        raise AgentServiceError(ErrorCode.INVALID_REQUEST, "user_prompt is empty")

    # Single raw_chat call — Go batches all candidate answers into the prompt strings,
    # so there is no per-transaction loop here. No semaphore needed.
    response = await state.provider.raw_chat(
        system_prompt=body.system_prompt,
        user_message=body.user_prompt,
        timeout_seconds=state.config.llm_timeout_seconds,
    )

    # Validate categories and reasons before returning — surface model errors early
    # so Go doesn't receive garbage it then fails to parse during consensus.
    check_judge_response(response)

    return JudgeResponse(response=response)
