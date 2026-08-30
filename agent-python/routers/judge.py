import asyncio
import json
import logging
import time
from datetime import datetime, timezone

from fastapi import APIRouter, Request

from errors import AgentServiceError, ErrorCode
from experiment.recorder import _parse_int_header, run_with_token_capture
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

    tx_hash_full = payload.get("transactionHash", "")
    tx_hash_short = tx_hash_full[:16]
    logger.info(
        "judge_request_start tx=%s num_candidates=%d",
        tx_hash_short,
        len(candidates),
    )
    request_start = time.perf_counter()

    recorder = state.recorder
    run_id = request.headers.get("X-Run-ID")
    round_num = _parse_int_header(request.headers.get("X-Round"))
    mini_round = _parse_int_header(request.headers.get("X-Mini-Round"))

    # One LLM call per candidate, all fired concurrently, to eliminate both
    # canonical-preference bias (cross-candidate comparison) and sequential latency.
    async def judge_candidate(candidate: dict) -> list:
        candidate_id = candidate.get("candidateId", "?")
        single_prompt = json.dumps({
            "transactionHash": tx_hash_full,
            "prompt": payload.get("prompt", ""),
            "candidates": [candidate],
        })
        request_payload = {
            "transactionHash": tx_hash_full,
            "prompt": payload.get("prompt", ""),
            "candidateId": candidate_id,
        }
        start_ts = datetime.now(timezone.utc)
        logger.info("judge_candidate_start tx=%s candidate=%s", tx_hash_short, candidate_id)

        if state.config.llm_provider.strip().lower() == "mock":
            category = (
                "CORRECT"
                if candidate.get("answer") == state.config.mock_preprocessing_answer
                else "WRONG"
            )
            classifications = [{"candidateId": candidate_id, "category": category}]
            end_ts = datetime.now(timezone.utc)
            if recorder.is_active:
                recorder.append(recorder.build_record(
                    run_id=run_id, operation="judge",
                    tx_hash=tx_hash_full or None, round_num=round_num,
                    mini_round=mini_round, start_ts=start_ts, end_ts=end_ts,
                    request_payload=request_payload,
                    parsed_response=classifications, input_tokens=0,
                    output_tokens=0, total_tokens=0, success=True, error=None,
                    mocked=True, provider_called=False,
                ))
            logger.info(
                "mocked_judge_classification tx=%s candidate=%s category=%s",
                tx_hash_short, candidate_id, category,
            )
            return classifications

        async with state.judge_semaphore:
            try:
                response, in_tok, out_tok, total_tok, _err = await run_with_token_capture(
                    state.provider.raw_chat(
                        system_prompt=body.system_prompt,
                        user_message=single_prompt,
                        timeout_seconds=state.config.llm_timeout_seconds,
                        json_format=True,
                        operation="judge",
                    )
                )
            except Exception as exc:
                end_ts = datetime.now(timezone.utc)
                if recorder.is_active:
                    recorder.append(recorder.build_record(
                        run_id=run_id,
                        operation="judge",
                        tx_hash=tx_hash_full or None,
                        round_num=round_num,
                        mini_round=mini_round,
                        start_ts=start_ts,
                        end_ts=end_ts,
                        request_payload=request_payload,
                        parsed_response=None,
                        input_tokens=None,
                        output_tokens=None,
                        total_tokens=None,
                        success=False,
                        error=f"{type(exc).__name__}: {exc}",
                    ))
                raise

        end_ts = datetime.now(timezone.utc)
        elapsed = (end_ts - start_ts).total_seconds()
        logger.info(
            "judge_raw_response tx=%s candidate=%s elapsed_s=%.3f",
            tx_hash_short,
            candidate_id,
            elapsed,
        )
        try:
            check_judge_response(response)
        except AgentServiceError as exc:
            logger.error(
                "judge_validation_failed tx=%s candidate=%s error=%s",
                tx_hash_short,
                candidate_id,
                exc,
            )
            if recorder.is_active:
                recorder.append(recorder.build_record(
                    run_id=run_id,
                    operation="judge",
                    tx_hash=tx_hash_full or None,
                    round_num=round_num,
                    mini_round=mini_round,
                    start_ts=start_ts,
                    end_ts=end_ts,
                    request_payload=request_payload,
                    parsed_response=None,
                    input_tokens=in_tok,
                    output_tokens=out_tok,
                    total_tokens=total_tok,
                    success=False,
                    error=f"{type(exc).__name__}: {exc}",
                ))
            raise
        classifications = json.loads(response)["classifications"]
        for item in classifications:
            logger.info(
                "judge_classification tx=%s candidate=%s category=%s",
                tx_hash_short,
                item.get("candidateId", "?"),
                item.get("category", "?"),
            )

        if recorder.is_active:
            recorder.append(recorder.build_record(
                run_id=run_id,
                operation="judge",
                tx_hash=tx_hash_full or None,
                round_num=round_num,
                mini_round=mini_round,
                start_ts=start_ts,
                end_ts=end_ts,
                request_payload=request_payload,
                parsed_response=classifications,
                input_tokens=in_tok,
                output_tokens=out_tok,
                total_tokens=total_tok,
                success=True,
                error=None,
            ))

        return classifications

    results = await asyncio.gather(*[judge_candidate(c) for c in candidates])
    classifications = [item for sublist in results for item in sublist]

    logger.info(
        "judge_request_done tx=%s num_candidates=%d total_s=%.3f",
        tx_hash_short,
        len(candidates),
        time.perf_counter() - request_start,
    )

    return JudgeResponse(response=json.dumps({"classifications": classifications}))
