import asyncio
import logging
import time

from fastapi import APIRouter, Request

from schemas import AnswerRequest, AnswerResponse, AnswerResult
from validation import check_non_empty, check_prompt_version, check_tx_hash_coverage

logger = logging.getLogger(__name__)
router = APIRouter()


@router.post("/answer", response_model=AnswerResponse)
async def answer(body: AnswerRequest, request: Request) -> AnswerResponse:
    state = request.app.state
    prompt = state.prompts["answerer_v1"]

    check_prompt_version(body.prompt_version, prompt.version)

    logger.info("answer_batch_start num_transactions=%d", len(body.transactions))
    request_start = time.perf_counter()

    # Cap concurrent Ollama calls to match OLLAMA_NUM_PARALLEL on the server side.
    semaphore = asyncio.Semaphore(state.config.answer_max_concurrency)

    async def process_one(tx) -> AnswerResult:
        async with semaphore:
            t0 = time.perf_counter()
            logger.info("llm_answer_start tx=%s", tx.tx_hash[:8])
            result: AnswerResult = await state.provider.structured_chat(
                system_prompt=prompt.content,
                user_payload={
                    "tx_hash": tx.tx_hash,
                    "prompt": tx.prompt,
                    "subdomains": tx.subdomains,
                },
                response_schema=AnswerResult,
                timeout_seconds=state.config.llm_timeout_seconds,
                operation="answer",
            )
            elapsed = time.perf_counter() - t0
            logger.info(
                "llm_answer_done tx=%s elapsed_s=%.3f answer_preview=%r",
                tx.tx_hash[:8],
                elapsed,
                result.answer[:80] if result.answer else "",
            )

        check_tx_hash_coverage(result.tx_hash, tx.tx_hash)
        check_non_empty(result.answer, "answer")
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
