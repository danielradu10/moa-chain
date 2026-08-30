import asyncio
import logging
import time
from datetime import datetime, timezone

from fastapi import APIRouter, Request

from experiment.recorder import _parse_int_header, run_with_token_capture
from schemas import (
    EvaluateSynthesisLLMResult,
    EvaluateSynthesisRequest,
    EvaluateSynthesisResponse,
    EvaluateSynthesisResultItem,
)
from validation import check_prompt_version, check_tx_hash_coverage

logger = logging.getLogger(__name__)
router = APIRouter()


@router.post("/evaluate-synthesis", response_model=EvaluateSynthesisResponse)
async def evaluate_synthesis(body: EvaluateSynthesisRequest, request: Request) -> EvaluateSynthesisResponse:
    state = request.app.state
    prompt = state.prompts["synthesis_evaluator_v1"]

    check_prompt_version(body.prompt_version, prompt.version)

    recorder = state.recorder
    run_id = request.headers.get("X-Run-ID")
    round_num = _parse_int_header(request.headers.get("X-Round"))
    mini_round = _parse_int_header(request.headers.get("X-Mini-Round"))

    logger.info("evaluate_synthesis_batch_start num_transactions=%d", len(body.transactions))
    request_start = time.perf_counter()

    semaphore = asyncio.Semaphore(state.config.answer_max_concurrency)

    async def process_one(tx) -> EvaluateSynthesisResultItem:
        request_payload = {
            "tx_hash": tx.tx_hash,
            "prompt": tx.prompt,
            "correct_answers": tx.correct_answers,
            "proposed_synthesis": tx.proposed_synthesis,
        }
        start_ts = datetime.now(timezone.utc)

        # Byzantine/mock validators must never call an external provider in
        # MR3.  Their evaluation is deterministic and still fully recorded
        # so the trace makes the local decision auditable.
        if state.config.llm_provider.strip().lower() == "mock":
            result = EvaluateSynthesisLLMResult(tx_hash=tx.tx_hash, approved=False)
            end_ts = datetime.now(timezone.utc)
            if recorder.is_active:
                recorder.append(recorder.build_record(
                    run_id=run_id,
                    operation="evaluate_synthesis",
                    tx_hash=tx.tx_hash,
                    round_num=round_num,
                    mini_round=mini_round,
                    start_ts=start_ts,
                    end_ts=end_ts,
                    request_payload=request_payload,
                    parsed_response=result.model_dump(),
                    input_tokens=0,
                    output_tokens=0,
                    total_tokens=0,
                    success=True,
                    error=None,
                    mocked=True,
                    provider_called=False,
                ))
            return EvaluateSynthesisResultItem(tx_hash=result.tx_hash, approved=False)

        async with semaphore:
            logger.info("llm_evaluate_synthesis_start tx=%s", tx.tx_hash[:8])
            try:
                result, in_tok, out_tok, total_tok, _err = await run_with_token_capture(
                    state.provider.structured_chat(
                        system_prompt=prompt.content,
                        user_payload=request_payload,
                        response_schema=EvaluateSynthesisLLMResult,
                        timeout_seconds=state.config.llm_timeout_seconds,
                        operation="evaluate_synthesis",
                    )
                )
            except Exception as exc:
                end_ts = datetime.now(timezone.utc)
                if recorder.is_active:
                    recorder.append(recorder.build_record(
                        run_id=run_id,
                        operation="evaluate_synthesis",
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
                "llm_evaluate_synthesis_done tx=%s elapsed_s=%.3f approved=%s",
                tx.tx_hash[:8],
                (end_ts - start_ts).total_seconds(),
                result.approved,
            )

        check_tx_hash_coverage(result.tx_hash, tx.tx_hash)

        if recorder.is_active:
            recorder.append(recorder.build_record(
                run_id=run_id,
                operation="evaluate_synthesis",
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

        return EvaluateSynthesisResultItem(tx_hash=result.tx_hash, approved=result.approved)

    results = await asyncio.gather(*[process_one(tx) for tx in body.transactions])

    logger.info(
        "evaluate_synthesis_batch_done num_transactions=%d total_s=%.3f",
        len(body.transactions),
        time.perf_counter() - request_start,
    )

    return EvaluateSynthesisResponse(
        prompt_version=prompt.version,
        prompt_hash=prompt.sha256_hash,
        evaluations=list(results),
    )
