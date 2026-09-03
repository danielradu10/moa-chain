"""
Core qualification harness.

Calls the LLMProvider directly (no HTTP round-trips) to exercise all five
MoA validator operations and collect latency + accuracy data.

Token counts are extracted via a lightweight log interceptor that parses the
llm_call structured log line emitted by each provider — this avoids any
protocol changes.
"""
from __future__ import annotations

import asyncio
import logging
import re
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from pydantic import BaseModel

from benchmark.prompt import (
    ANSWER_JUDGE_SYSTEM_PROMPT,
    build_user_prompt,
    parse_judge_response,
)
from prompts.loader import load_protocol_prompt, ProtocolPrompt
from qualification.fixtures import (
    ALLOWED_SUBDOMAINS,
    CANONICAL_QUESTION,
    CANONICAL_TX_HASH,
    JUDGE_FIXTURES,
    SYNTHESIS_CANDIDATES,
    SYNTHESIS_EVAL_FIXTURES,
)
from schemas import AnswerResult, LabelResult


# ── Inline schemas for not-yet-implemented endpoints ─────────────────────────

class SynthesizeResult(BaseModel):
    tx_hash: str
    synthesized_answer: str


class EvaluateSynthesisResult(BaseModel):
    tx_hash: str
    approved: bool


# ── Token capture via log interception ───────────────────────────────────────

_TOKEN_RE = re.compile(
    r"input_tokens=(\S+)\s+output_tokens=(\S+)\s+total_tokens=(\S+)"
)


class _TokenCapture(logging.Handler):
    """Intercepts a single llm_call log line and extracts token counts."""

    def __init__(self) -> None:
        super().__init__()
        self.input_tokens: int | None = None
        self.output_tokens: int | None = None
        self.total_tokens: int | None = None

    def emit(self, record: logging.LogRecord) -> None:
        msg = record.getMessage()
        if "llm_call" not in msg:
            return
        m = _TOKEN_RE.search(msg)
        if not m:
            return
        try:
            self.input_tokens = int(m.group(1))
            self.output_tokens = int(m.group(2))
            self.total_tokens = int(m.group(3))
        except (ValueError, IndexError):
            pass


# ── Per-call record ──────────────────────────────────────────────────────────

@dataclass
class CallRecord:
    operation: str
    repetition: int
    started_at: float        # Unix timestamp
    duration_ms: float
    success: bool
    error: str | None
    input_tokens: int | None
    output_tokens: int | None
    total_tokens: int | None
    data: dict = field(default_factory=dict)


# ── Aggregated run result ────────────────────────────────────────────────────

@dataclass
class QualRun:
    provider_name: str
    model_name: str
    repetitions: int
    started_at: float
    finished_at: float
    records: list[CallRecord] = field(default_factory=list)


# ── Timed call helper ─────────────────────────────────────────────────────────

async def _run_timed(coro) -> tuple[Any, float, float, int | None, int | None, int | None, str | None]:
    """Execute coro, capture tokens, and return (result, duration_ms, started_at, in_tok, out_tok, total_tok, error).

    Token capture works by intercepting the llm_call log line emitted by each
    provider. Providers log under the "providers" namespace, so we attach the
    handler there and temporarily ensure INFO messages are not filtered out.
    The harness is sequential — no concurrent calls — so mutating the logger
    level is safe.
    """
    capture = _TokenCapture()
    capture.setLevel(logging.INFO)
    providers_logger = logging.getLogger("providers")
    _old_level = providers_logger.level
    providers_logger.setLevel(logging.INFO)
    providers_logger.addHandler(capture)
    started_at = time.time()
    result = None
    error: str | None = None
    try:
        result = await coro
    except Exception as exc:
        error = f"{type(exc).__name__}: {exc}"
    finally:
        duration_ms = (time.time() - started_at) * 1000.0
        providers_logger.removeHandler(capture)
        providers_logger.setLevel(_old_level)
    return result, duration_ms, started_at, capture.input_tokens, capture.output_tokens, capture.total_tokens, error


# ── Per-operation callers ─────────────────────────────────────────────────────

async def _call_label(provider, labeler: ProtocolPrompt, repetition: int) -> CallRecord:
    payload = {
        "prompt_version": labeler.version,
        "allowed_subdomains": ALLOWED_SUBDOMAINS,
        "transactions": [{"tx_hash": CANONICAL_TX_HASH, "prompt": CANONICAL_QUESTION}],
    }
    result, duration_ms, started_at, in_tok, out_tok, total_tok, error = await _run_timed(
        provider.structured_chat(
            system_prompt=labeler.content,
            user_payload=payload,
            response_schema=LabelResult,
            timeout_seconds=60.0,
            operation="label",
        )
    )

    data: dict = {}
    if result is not None:
        labels = result.labels if hasattr(result, "labels") else []
        data = {
            "tx_hash": result.tx_hash,
            "labels": [{"subdomain": e.subdomain, "confidence": e.confidence} for e in labels],
        }

    return CallRecord(
        operation="label",
        repetition=repetition,
        started_at=started_at,
        duration_ms=duration_ms,
        success=error is None,
        error=error,
        input_tokens=in_tok,
        output_tokens=out_tok,
        total_tokens=total_tok,
        data=data,
    )


async def _call_answer(provider, answerer: ProtocolPrompt, repetition: int) -> CallRecord:
    payload = {
        "prompt_version": answerer.version,
        "transactions": [
            {
                "tx_hash": CANONICAL_TX_HASH,
                "prompt": CANONICAL_QUESTION,
                "subdomains": ["systems_programming"],
            }
        ],
    }
    result, duration_ms, started_at, in_tok, out_tok, total_tok, error = await _run_timed(
        provider.structured_chat(
            system_prompt=answerer.content,
            user_payload=payload,
            response_schema=AnswerResult,
            timeout_seconds=90.0,
            operation="answer",
        )
    )

    data: dict = {}
    if result is not None:
        data = {"tx_hash": result.tx_hash, "answer_length": len(result.answer)}

    return CallRecord(
        operation="answer",
        repetition=repetition,
        started_at=started_at,
        duration_ms=duration_ms,
        success=error is None,
        error=error,
        input_tokens=in_tok,
        output_tokens=out_tok,
        total_tokens=total_tok,
        data=data,
    )


async def _call_judge_one(provider, fixture: dict, repetition: int) -> CallRecord:
    user_prompt = build_user_prompt(
        tx_id=CANONICAL_TX_HASH,
        prompt=CANONICAL_QUESTION,
        answer=fixture["answer"],
        candidate_id="candidate-1",
    )
    result, duration_ms, started_at, in_tok, out_tok, total_tok, error = await _run_timed(
        provider.raw_chat(
            system_prompt=ANSWER_JUDGE_SYSTEM_PROMPT,
            user_message=user_prompt,
            timeout_seconds=60.0,
            json_format=True,
            operation="judge",
        )
    )

    actual: str | None = None
    if result is not None and error is None:
        try:
            actual = parse_judge_response(result, candidate_id="candidate-1")
        except (ValueError, Exception) as exc:
            error = f"parse_error: {exc}"

    matches = actual == fixture["expected"] if actual is not None else False
    data = {
        "fixture_label": fixture["label"],
        "expected": fixture["expected"],
        "actual": actual,
        "matches": matches,
    }

    return CallRecord(
        operation="judge",
        repetition=repetition,
        started_at=started_at,
        duration_ms=duration_ms,
        success=error is None,
        error=error,
        input_tokens=in_tok,
        output_tokens=out_tok,
        total_tokens=total_tok,
        data=data,
    )


async def _call_synthesize(provider, synthesizer_prompt: str, repetition: int) -> CallRecord:
    payload = {
        "tx_hash": CANONICAL_TX_HASH,
        "prompt": CANONICAL_QUESTION,
        "correct_answers": SYNTHESIS_CANDIDATES,
    }
    result, duration_ms, started_at, in_tok, out_tok, total_tok, error = await _run_timed(
        provider.structured_chat(
            system_prompt=synthesizer_prompt,
            user_payload=payload,
            response_schema=SynthesizeResult,
            timeout_seconds=90.0,
            operation="synthesize",
        )
    )

    data: dict = {}
    if result is not None:
        data = {
            "tx_hash": result.tx_hash,
            "synthesis_length": len(result.synthesized_answer),
        }

    return CallRecord(
        operation="synthesize",
        repetition=repetition,
        started_at=started_at,
        duration_ms=duration_ms,
        success=error is None,
        error=error,
        input_tokens=in_tok,
        output_tokens=out_tok,
        total_tokens=total_tok,
        data=data,
    )


async def _call_evaluate_synthesis_one(
    provider, evaluator_prompt: str, fixture: dict, repetition: int
) -> CallRecord:
    payload = {
        "tx_hash": CANONICAL_TX_HASH,
        "prompt": CANONICAL_QUESTION,
        "correct_answers": SYNTHESIS_CANDIDATES,
        "proposed_synthesis": fixture["proposed"],
    }
    result, duration_ms, started_at, in_tok, out_tok, total_tok, error = await _run_timed(
        provider.structured_chat(
            system_prompt=evaluator_prompt,
            user_payload=payload,
            response_schema=EvaluateSynthesisResult,
            timeout_seconds=60.0,
            operation="evaluate_synthesis",
        )
    )

    actual_approved: bool | None = None
    if result is not None:
        actual_approved = result.approved

    matches = actual_approved == fixture["expected_approved"] if actual_approved is not None else False
    data = {
        "fixture_label": fixture["label"],
        "expected_approved": fixture["expected_approved"],
        "actual_approved": actual_approved,
        "matches": matches,
    }

    return CallRecord(
        operation="evaluate_synthesis",
        repetition=repetition,
        started_at=started_at,
        duration_ms=duration_ms,
        success=error is None,
        error=error,
        input_tokens=in_tok,
        output_tokens=out_tok,
        total_tokens=total_tok,
        data=data,
    )


# ── Main entry point ──────────────────────────────────────────────────────────

async def run_qualification(
    provider,
    provider_name: str,
    model_name: str,
    repetitions: int = 3,
    prompts_dir: Path | None = None,
) -> QualRun:
    """Run all qualification operations and return a QualRun with all CallRecords.

    For each repetition, runs in sequence:
      label × 1, answer × 1, judge × 4 (one per fixture), synthesize × 1,
      evaluate_synthesis × 2 (one per eval fixture).
    Total calls per repetition: 9.
    """
    if prompts_dir is None:
        prompts_dir = Path(__file__).parent.parent / "prompts"

    qual_prompts_dir = Path(__file__).parent / "prompts"

    labeler = load_protocol_prompt("labeler_v3", prompts_dir=prompts_dir)
    answerer = load_protocol_prompt("answerer_v1", prompts_dir=prompts_dir)
    synthesizer_prompt = (qual_prompts_dir / "synthesizer_v1.txt").read_text(encoding="utf-8")
    evaluator_prompt = (qual_prompts_dir / "synthesis_evaluator_v1.txt").read_text(encoding="utf-8")

    run = QualRun(
        provider_name=provider_name,
        model_name=model_name,
        repetitions=repetitions,
        started_at=time.time(),
        finished_at=0.0,
    )

    for rep in range(1, repetitions + 1):
        run.records.append(await _call_label(provider, labeler, rep))
        run.records.append(await _call_answer(provider, answerer, rep))

        for fixture in JUDGE_FIXTURES:
            run.records.append(await _call_judge_one(provider, fixture, rep))

        run.records.append(await _call_synthesize(provider, synthesizer_prompt, rep))

        for eval_fixture in SYNTHESIS_EVAL_FIXTURES:
            run.records.append(await _call_evaluate_synthesis_one(provider, evaluator_prompt, eval_fixture, rep))

    run.finished_at = time.time()
    return run
