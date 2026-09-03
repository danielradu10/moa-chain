import asyncio
import logging
import time
from datetime import datetime, timezone

from fastapi import APIRouter, Request

from experiment.recorder import _parse_int_header, run_with_token_capture
from schemas import LabelEntry, LabelRequest, LabelResponse, LabelResult
from validation import (
    check_confidence_range,
    check_label_list,
    check_prompt_version,
    check_subdomain_membership,
    check_tx_hash_coverage,
)

router = APIRouter()
logger = logging.getLogger(__name__)


@router.post("/label", response_model=LabelResponse)
async def label(body: LabelRequest, request: Request) -> LabelResponse:
    state = request.app.state
    prompt = state.prompts["labeler_v3"]

    check_prompt_version(body.prompt_version, prompt.version)

    allowed = set(body.allowed_subdomains)
    recorder = state.recorder
    run_id = request.headers.get("X-Run-ID")
    round_num = _parse_int_header(request.headers.get("X-Round"))
    mini_round = _parse_int_header(request.headers.get("X-Mini-Round"))

    # Cap concurrent LLM calls to match OLLAMA_NUM_PARALLEL on the server side.
    semaphore = asyncio.Semaphore(state.config.label_max_concurrency)
    request_start = time.perf_counter()

    async def process_one(tx) -> LabelResult:
        request_payload = {
            "tx_hash": tx.tx_hash,
            "prompt": tx.prompt,
            "allowed_subdomains": body.allowed_subdomains,
        }
        start_ts = datetime.now(timezone.utc)

        if state.config.mock_preprocessing_label:
            result = LabelResult(
                tx_hash=tx.tx_hash,
                labels=[LabelEntry(
                    subdomain=state.config.mock_preprocessing_label,
                    confidence=1.0,
                )],
            )
            end_ts = datetime.now(timezone.utc)
            check_tx_hash_coverage(result.tx_hash, tx.tx_hash)
            check_label_list(result.labels, tx.tx_hash, allowed)
            check_subdomain_membership(result.labels[0].subdomain, allowed)
            if recorder.is_active:
                recorder.append(recorder.build_record(
                    run_id=run_id, operation="label", tx_hash=tx.tx_hash,
                    round_num=round_num, mini_round=mini_round,
                    start_ts=start_ts, end_ts=end_ts,
                    request_payload=request_payload,
                    parsed_response=result.model_dump(), input_tokens=0,
                    output_tokens=0, total_tokens=0, success=True, error=None,
                    mocked=True, provider_called=False,
                ))
            logger.info("mocked_label_done tx=%s label=%s", tx.tx_hash[:8], result.labels[0].subdomain)
            return result

        async with semaphore:
            logger.info("llm_label_start tx=%s", tx.tx_hash[:8])
            try:
                result, in_tok, out_tok, total_tok, _err = await run_with_token_capture(
                    state.provider.structured_chat(
                        system_prompt=prompt.content,
                        user_payload=request_payload,
                        response_schema=LabelResult,
                        timeout_seconds=state.config.llm_timeout_seconds,
                        operation="label",
                    )
                )
            except Exception as exc:
                end_ts = datetime.now(timezone.utc)
                if recorder.is_active:
                    recorder.append(recorder.build_record(
                        run_id=run_id,
                        operation="label",
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
                "llm_label_done tx=%s elapsed_s=%.3f",
                tx.tx_hash[:8],
                (end_ts - start_ts).total_seconds(),
            )

        # The model returns all relevant labels with confidence scores.
        # Select the top 3 real labels by confidence, or keep non_related if it
        # dominates (higher confidence than any real label the model found).
        non_related_entry = next((e for e in result.labels if e.subdomain == "non_related"), None)
        real_entries = [e for e in result.labels if e.subdomain != "non_related"]

        if real_entries:
            max_real_confidence = max(e.confidence for e in real_entries)
            if non_related_entry and non_related_entry.confidence > max_real_confidence:
                result = LabelResult(tx_hash=result.tx_hash, labels=[non_related_entry])
            else:
                top3 = sorted(real_entries, key=lambda e: e.confidence, reverse=True)[:3]
                result = LabelResult(tx_hash=result.tx_hash, labels=top3)

        check_tx_hash_coverage(result.tx_hash, tx.tx_hash)
        check_label_list(result.labels, tx.tx_hash, allowed)
        for entry in result.labels:
            check_subdomain_membership(entry.subdomain, allowed)
            check_confidence_range(entry.confidence)

        if recorder.is_active:
            recorder.append(recorder.build_record(
                run_id=run_id,
                operation="label",
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
        "label_batch_done batch=%d total_s=%.3f",
        len(body.transactions),
        time.perf_counter() - request_start,
    )

    return LabelResponse(
        prompt_version=prompt.version,
        prompt_hash=prompt.sha256_hash,
        results=list(results),
    )
