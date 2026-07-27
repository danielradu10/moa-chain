import json

from fastapi import APIRouter, Request

from errors import AgentServiceError, ErrorCode
from schemas import JudgeRequest, JudgeResponse
from validation import check_judge_response

router = APIRouter()


@router.post("/judge", response_model=JudgeResponse)
async def judge(body: JudgeRequest, request: Request) -> JudgeResponse:
    state = request.app.state

    if not body.system_prompt.strip():
        raise AgentServiceError(ErrorCode.INVALID_REQUEST, "system_prompt is empty")
    if not body.user_prompt.strip():
        raise AgentServiceError(ErrorCode.INVALID_REQUEST, "user_prompt is empty")

    try:
        payload = json.loads(body.user_prompt)
        candidates = payload["candidates"]
    except (json.JSONDecodeError, KeyError, TypeError):
        raise AgentServiceError(
            ErrorCode.INVALID_REQUEST,
            "user_prompt must be valid JSON with a 'candidates' array",
        )

    if not candidates:
        raise AgentServiceError(ErrorCode.INVALID_REQUEST, "user_prompt contains no candidates")

    # Send one candidate per LLM call to avoid canonical-preference bias.
    # When all candidates are batched together, the 7B model treats one phrasing
    # as authoritative and either misclassifies or silently drops the others.
    # Evaluating each candidate in isolation removes the cross-candidate influence.
    classifications = []
    for candidate in candidates:
        single_prompt = json.dumps({
            "transactionHash": payload.get("transactionHash", ""),
            "prompt": payload.get("prompt", ""),
            "candidates": [candidate],
        })

        response = await state.provider.raw_chat(
            system_prompt=body.system_prompt,
            user_message=single_prompt,
            timeout_seconds=state.config.llm_timeout_seconds,
        )

        check_judge_response(response)
        classifications.extend(json.loads(response)["classifications"])

    return JudgeResponse(response=json.dumps({"classifications": classifications}))
