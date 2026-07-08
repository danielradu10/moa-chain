import asyncio

from fastapi import APIRouter, Request

from schemas import LabelRequest, LabelResponse, LabelResult
from validation import (
    check_confidence_range,
    check_prompt_version,
    check_subdomain_membership,
    check_tx_hash_coverage,
)

router = APIRouter()


@router.post("/label", response_model=LabelResponse)
async def label(body: LabelRequest, request: Request) -> LabelResponse:
    state = request.app.state
    prompt = state.prompts["labeler_v1"]

    check_prompt_version(body.prompt_version, prompt.version)

    allowed = set(body.allowed_subdomains)

    # Cap concurrent Ollama calls to match OLLAMA_NUM_PARALLEL on the server side.
    # Without this, all transactions would be fired at once regardless of batch size.
    semaphore = asyncio.Semaphore(state.config.label_max_concurrency)

    async def process_one(tx) -> LabelResult:
        # Semaphore only wraps the LLM call — validation runs after the slot is released.
        async with semaphore:
            result: LabelResult = await state.provider.structured_chat(
                system_prompt=prompt.content,
                user_payload={
                    "tx_hash": tx.tx_hash,
                    "prompt": tx.prompt,
                    "allowed_subdomains": body.allowed_subdomains,
                },
                response_schema=LabelResult,
                timeout_seconds=state.config.llm_timeout_seconds,
            )

        check_tx_hash_coverage(result.tx_hash, tx.tx_hash)
        for entry in result.labels:
            check_subdomain_membership(entry.subdomain, allowed)
            check_confidence_range(entry.confidence)
        return result

    # gather preserves submission order regardless of completion order.
    results = await asyncio.gather(*[process_one(tx) for tx in body.transactions])

    return LabelResponse(
        prompt_version=prompt.version,
        prompt_hash=prompt.sha256_hash,
        results=list(results),
    )
