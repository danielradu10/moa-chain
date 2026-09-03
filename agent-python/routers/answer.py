import asyncio
import logging
import time
from datetime import datetime, timezone

from fastapi import APIRouter, Request

from experiment.recorder import _parse_int_header, run_with_token_capture
from schemas import AnswerRequest, AnswerResponse, AnswerResult
from validation import check_non_empty, check_prompt_version, check_tx_hash_coverage

logger = logging.getLogger(__name__)
router = APIRouter()


@router.post("/answer", response_model=AnswerResponse)
async def answer(body: AnswerRequest, request: Request) -> AnswerResponse:
    state = request.app.state
    prompt = state.prompts["answerer_v1"]

    check_prompt_version(body.prompt_version, prompt.version)

    recorder = state.recorder
    run_id = request.headers.get("X-Run-ID")
    round_num = _parse_int_header(request.headers.get("X-Round"))
    mini_round = _parse_int_header(request.headers.get("X-Mini-Round"))

    logger.info("answer_batch_start num_transactions=%d", len(body.transactions))
    request_start = time.perf_counter()

    semaphore = asyncio.Semaphore(state.config.answer_max_concurrency)

    async def process_one(tx) -> AnswerResult:
        request_payload = {
            "tx_hash": tx.tx_hash,
            "prompt": tx.prompt,
            "subdomains": tx.subdomains,
        }
        start_ts = datetime.now(timezone.utc)

        if state.config.mock_preprocessing_answer:
            result = AnswerResult(
                tx_hash=tx.tx_hash,
                answer=state.config.mock_preprocessing_answer,
            )
            end_ts = datetime.now(timezone.utc)
            check_tx_hash_coverage(result.tx_hash, tx.tx_hash)
            check_non_empty(result.answer, "answer")
            if recorder.is_active:
                recorder.append(recorder.build_record(
                    run_id=run_id, operation="answer", tx_hash=tx.tx_hash,
                    round_num=round_num, mini_round=mini_round,
                    start_ts=start_ts, end_ts=end_ts,
                    request_payload=request_payload,
                    parsed_response=result.model_dump(), input_tokens=0,
                    output_tokens=0, total_tokens=0, success=True, error=None,
                    mocked=True, provider_called=False,
                ))
            logger.info("mocked_answer_done tx=%s", tx.tx_hash[:8])
            return result

        async with semaphore:
            logger.info("llm_answer_start tx=%s", tx.tx_hash[:8])
            try:
                result, in_tok, out_tok, total_tok, _err = await run_with_token_capture(
                    state.provider.structured_chat(
                        system_prompt=prompt.content,
                        user_payload=request_payload,
                        response_schema=AnswerResult,
                        timeout_seconds=state.config.llm_timeout_seconds,
                        operation="answer",
                    )
                )
            except Exception as exc:
                end_ts = datetime.now(timezone.utc)
                if recorder.is_active:
                    recorder.append(recorder.build_record(
                        run_id=run_id,
                        operation="answer",
                        tx_hash=tx.tx_hash,
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
            logger.info(
                "llm_answer_done tx=%s elapsed_s=%.3f answer_preview=%r",
                tx.tx_hash[:8],
                (end_ts - start_ts).total_seconds(),
                result.answer[:80] if result.answer else "",
            )

        check_tx_hash_coverage(result.tx_hash, tx.tx_hash)
        check_non_empty(result.answer, "answer")

        if recorder.is_active:
            recorder.append(recorder.build_record(
                run_id=run_id,
                operation="answer",
                tx_hash=tx.tx_hash,
                round_num=round_num,
                mini_round=mini_round,
                start_ts=start_ts,
                end_ts=end_ts,
                request_payload=request_payload,
                parsed_response=result.model_dump(),
                input_tokens=in_tok,
                output_tokens=out_tok,
                total_tokens=total_tok,
                success=True,
                error=None,
            ))

        return result

    # gather preserves submission order regardless of completion order.
    results = await asyncio.gather(*[process_one(tx) for tx in body.transactions])

    logger.info(
        "answer_batch_done num_transactions=%d total_s=%.3f",
        len(body.transactions),
        time.perf_counter() - request_start,
    )

    return AnswerResponse(
        prompt_version=prompt.version,
        prompt_hash=prompt.sha256_hash,
        results=list(results),
    )
