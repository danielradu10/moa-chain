import asyncio
import json
import logging
import time

from fastapi import APIRouter, Request

from errors import AgentServiceError, ErrorCode
from schemas import JudgeRequest, JudgeResponse
from validation import check_judge_response

logger = logging.getLogger(__name__)
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

    tx_hash = payload.get("transactionHash", "?")[:16]
    logger.info(
        "judge_request_start tx=%s num_candidates=%d",
        tx_hash,
        len(candidates),
    )
    request_start = time.perf_counter()

    # One LLM call per candidate, all fired concurrently, to eliminate both
    # canonical-preference bias (cross-candidate comparison) and sequential latency.
    async def judge_candidate(candidate: dict) -> list:
        candidate_id = candidate.get("candidateId", "?")
        single_prompt = json.dumps({
            "transactionHash": payload.get("transactionHash", ""),
            "prompt": payload.get("prompt", ""),
            "candidates": [candidate],
        })
        t0 = time.perf_counter()
        logger.info("judge_candidate_start tx=%s candidate=%s", tx_hash, candidate_id)
        async with state.judge_semaphore:
            response = await state.provider.raw_chat(
                system_prompt=body.system_prompt,
                user_message=single_prompt,
                timeout_seconds=state.config.llm_timeout_seconds,
                json_format=True,
                operation="judge",
            )
        elapsed = time.perf_counter() - t0
        logger.info(
            "judge_raw_response tx=%s candidate=%s elapsed_s=%.3f response=%r",
            tx_hash,
            candidate_id,
            elapsed,
            response,
        )
        try:
            check_judge_response(response)
        except AgentServiceError as exc:
            logger.error(
                "judge_validation_failed tx=%s candidate=%s error=%s response=%r",
                tx_hash,
                candidate_id,
                exc,
                response,
            )
            raise
        classifications = json.loads(response)["classifications"]
        for item in classifications:
            logger.info(
                "judge_classification tx=%s candidate=%s category=%s",
                tx_hash,
                item.get("candidateId", "?"),
                item.get("category", "?"),
            )
        return classifications

    results = await asyncio.gather(*[judge_candidate(c) for c in candidates])
    classifications = [item for sublist in results for item in sublist]

    logger.info(
        "judge_request_done tx=%s num_candidates=%d total_s=%.3f",
        tx_hash,
        len(candidates),
        time.perf_counter() - request_start,
    )

    return JudgeResponse(response=json.dumps({"classifications": classifications}))
