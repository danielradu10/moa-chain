import asyncio

from fastapi import APIRouter, Request

from schemas import AnswerRequest, AnswerResponse, AnswerResult
from validation import check_non_empty, check_prompt_version, check_tx_hash_coverage

router = APIRouter()


@router.post("/answer", response_model=AnswerResponse)
async def answer(body: AnswerRequest, request: Request) -> AnswerResponse:
    state = request.app.state
    prompt = state.prompts["answerer_v1"]

    check_prompt_version(body.prompt_version, prompt.version)

    # Cap concurrent Ollama calls to match OLLAMA_NUM_PARALLEL on the server side.
    semaphore = asyncio.Semaphore(state.config.answer_max_concurrency)

    async def process_one(tx) -> AnswerResult:
        # Semaphore only wraps the LLM call — validation runs after the slot is released.
        async with semaphore:
            result: AnswerResult = await state.provider.structured_chat(
                system_prompt=prompt.content,
                user_payload={
                    "tx_hash": tx.tx_hash,
                    "prompt": tx.prompt,
                    "subdomains": tx.subdomains,
                },
                response_schema=AnswerResult,
                timeout_seconds=state.config.llm_timeout_seconds,
            )

        check_tx_hash_coverage(result.tx_hash, tx.tx_hash)
        check_non_empty(result.answer, "answer")
        return result

    # gather preserves submission order regardless of completion order.
    results = await asyncio.gather(*[process_one(tx) for tx in body.transactions])

    return AnswerResponse(
        prompt_version=prompt.version,
        prompt_hash=prompt.sha256_hash,
        results=list(results),
    )
