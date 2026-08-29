"""
Harness integration tests using a per-operation fake provider.

These tests run the full run_qualification() pipeline without making any real
LLM calls. The operation-aware FakeProvider returns pre-canned responses keyed
by the `operation` argument, which allows the harness to complete all 9 calls
per repetition without needing a live provider.
"""
from __future__ import annotations

import asyncio
import json
import pytest
from typing import TypeVar

from pydantic import BaseModel

from qualification.fixtures import (
    CANONICAL_TX_HASH,
    JUDGE_FIXTURES,
    SYNTHESIS_EVAL_FIXTURES,
)
from qualification.harness import (
    EvaluateSynthesisResult,
    QualRun,
    SynthesizeResult,
    run_qualification,
)
from schemas import AnswerResult, LabelEntry, LabelResult

T = TypeVar("T", bound=BaseModel)


# ── Operation-aware fake ──────────────────────────────────────────────────────

_CORRECT_JUDGE_RESPONSE = json.dumps({
    "classifications": [{"candidateId": "candidate-1", "category": "CORRECT"}]
})

# One response per judge fixture label, in the same order as JUDGE_FIXTURES.
_JUDGE_RESPONSES = {
    "correct": json.dumps({"classifications": [{"candidateId": "candidate-1", "category": "CORRECT"}]}),
    "wrong": json.dumps({"classifications": [{"candidateId": "candidate-1", "category": "WRONG"}]}),
    "hallucination": json.dumps({"classifications": [{"candidateId": "candidate-1", "category": "HALLUCINATION"}]}),
    "malicious": json.dumps({"classifications": [{"candidateId": "candidate-1", "category": "MALICIOUS"}]}),
}


class _OperationFakeProvider:
    """Returns pre-canned responses keyed by `operation`.

    structured_chat responses must be pre-configured per operation via
    set_structured_responses(). raw_chat cycles through raw_responses_by_op["judge"]
    in order (one response per fixture call).
    """

    def __init__(self) -> None:
        self._structured: dict[str, BaseModel] = {}
        self._raw: dict[str, list[str]] = {}
        self._raw_cursor: dict[str, int] = {}
        self._error: Exception | None = None
        self._error_ops: set[str] = set()

    def set_structured(self, operation: str, response: BaseModel) -> None:
        self._structured[operation] = response

    def set_raw_sequence(self, operation: str, responses: list[str]) -> None:
        self._raw[operation] = responses
        self._raw_cursor[operation] = 0

    def set_error_for(self, operation: str, error: Exception) -> None:
        self._error_ops.add(operation)
        self._error = error

    async def structured_chat(
        self,
        system_prompt: str,
        user_payload: dict,
        response_schema: type[T],
        timeout_seconds: float,
        operation: str = "",
    ) -> T:
        if operation in self._error_ops:
            raise self._error  # type: ignore[misc]
        if operation not in self._structured:
            raise ValueError(f"OperationFakeProvider: no structured response for operation={operation!r}")
        return self._structured[operation]  # type: ignore[return-value]

    async def raw_chat(
        self,
        system_prompt: str,
        user_message: str,
        timeout_seconds: float,
        json_format: bool = False,
        operation: str = "",
    ) -> str:
        if operation in self._error_ops:
            raise self._error  # type: ignore[misc]
        if operation not in self._raw:
            raise ValueError(f"OperationFakeProvider: no raw response for operation={operation!r}")
        seq = self._raw[operation]
        cursor = self._raw_cursor.get(operation, 0)
        response = seq[cursor % len(seq)]
        self._raw_cursor[operation] = cursor + 1
        return response

    async def ping(self) -> bool:
        return True


def _make_provider(error_ops: set[str] | None = None) -> _OperationFakeProvider:
    p = _OperationFakeProvider()
    if error_ops is None:
        error_ops = set()

    p.set_structured("label", LabelResult(
        tx_hash=CANONICAL_TX_HASH,
        labels=[LabelEntry(subdomain="systems_programming", confidence=0.95)],
    ))
    p.set_structured("answer", AnswerResult(
        tx_hash=CANONICAL_TX_HASH,
        answer="A mutex prevents race conditions by serializing access to shared state.",
    ))
    # Judge responses cycle through all four fixture-expected categories.
    judge_seq = [_JUDGE_RESPONSES[f["label"]] for f in JUDGE_FIXTURES]
    p.set_raw_sequence("judge", judge_seq * 10)  # enough for any repetition count

    p.set_structured("synthesize", SynthesizeResult(
        tx_hash=CANONICAL_TX_HASH,
        synthesized_answer="A mutex prevents race conditions by guaranteeing exclusive access.",
    ))
    # Eval responses: first fixture expects approved=True, second expects approved=False.
    eval_seq = [
        EvaluateSynthesisResult(tx_hash=CANONICAL_TX_HASH, approved=fix["expected_approved"])
        for fix in SYNTHESIS_EVAL_FIXTURES
    ]
    # Cycle for arbitrary repetitions.
    eval_idx = [0]

    orig_structured = p.structured_chat.__func__  # type: ignore[attr-defined]

    class _EvalAwareFake(_OperationFakeProvider):
        def __init__(self, inner: _OperationFakeProvider) -> None:
            super().__init__()
            self._inner = inner
            self._eval_cursor = 0

        async def structured_chat(self, system_prompt, user_payload, response_schema, timeout_seconds, operation=""):
            if operation in self._inner._error_ops:
                raise self._inner._error  # type: ignore[misc]
            if operation == "evaluate_synthesis":
                resp = eval_seq[self._eval_cursor % len(eval_seq)]
                self._eval_cursor += 1
                return resp  # type: ignore[return-value]
            return await self._inner.structured_chat(system_prompt, user_payload, response_schema, timeout_seconds, operation)

        async def raw_chat(self, system_prompt, user_message, timeout_seconds, json_format=False, operation=""):
            if operation in self._inner._error_ops:
                raise self._inner._error  # type: ignore[misc]
            return await self._inner.raw_chat(system_prompt, user_message, timeout_seconds, json_format, operation)

        async def ping(self):
            return True

    return _EvalAwareFake(p)


# ── Tests ─────────────────────────────────────────────────────────────────────

def test_run_qualification_returns_qual_run():
    provider = _make_provider()
    run = asyncio.run(run_qualification(provider, "fake", "test-model", repetitions=1))
    assert isinstance(run, QualRun)


def test_run_qualification_record_count_per_rep():
    # 1 label + 1 answer + 4 judge + 1 synthesize + 2 eval = 9 per rep
    provider = _make_provider()
    run = asyncio.run(run_qualification(provider, "fake", "test-model", repetitions=2))
    assert len(run.records) == 18


def test_run_qualification_operations_present():
    provider = _make_provider()
    run = asyncio.run(run_qualification(provider, "fake", "test-model", repetitions=1))
    ops = {r.operation for r in run.records}
    assert ops == {"label", "answer", "judge", "synthesize", "evaluate_synthesis"}


def test_run_qualification_all_success():
    provider = _make_provider()
    run = asyncio.run(run_qualification(provider, "fake", "test-model", repetitions=1))
    for rec in run.records:
        assert rec.success, f"Expected success for {rec.operation} rep {rec.repetition}: {rec.error}"


def test_run_qualification_judge_perfect_accuracy():
    provider = _make_provider()
    run = asyncio.run(run_qualification(provider, "fake", "test-model", repetitions=1))
    judge_records = [r for r in run.records if r.operation == "judge"]
    assert len(judge_records) == 4
    for rec in judge_records:
        assert rec.data.get("matches") is True, (
            f"Judge {rec.data.get('fixture_label')} misclassified: "
            f"expected={rec.data.get('expected')} actual={rec.data.get('actual')}"
        )


def test_run_qualification_eval_perfect_accuracy():
    provider = _make_provider()
    run = asyncio.run(run_qualification(provider, "fake", "test-model", repetitions=1))
    eval_records = [r for r in run.records if r.operation == "evaluate_synthesis"]
    assert len(eval_records) == 2
    for rec in eval_records:
        assert rec.data.get("matches") is True, (
            f"Eval {rec.data.get('fixture_label')} incorrect: "
            f"expected={rec.data.get('expected_approved')} actual={rec.data.get('actual_approved')}"
        )


def test_run_qualification_error_does_not_crash_run():
    """A provider error on one operation should be recorded, not crash the run."""
    provider = _make_provider()
    provider._inner.set_error_for("label", RuntimeError("LLM timeout"))

    run = asyncio.run(run_qualification(provider, "fake", "test-model", repetitions=1))
    label_records = [r for r in run.records if r.operation == "label"]
    assert len(label_records) == 1
    assert not label_records[0].success
    assert "LLM timeout" in (label_records[0].error or "")

    # Other operations should still succeed.
    answer_records = [r for r in run.records if r.operation == "answer"]
    assert answer_records[0].success


def test_run_qualification_metadata():
    provider = _make_provider()
    run = asyncio.run(run_qualification(provider, "openai", "gpt-4o-mini", repetitions=1))
    assert run.provider_name == "openai"
    assert run.model_name == "gpt-4o-mini"
    assert run.repetitions == 1
    assert run.finished_at > run.started_at


def test_run_qualification_repetition_numbering():
    provider = _make_provider()
    run = asyncio.run(run_qualification(provider, "fake", "test-model", repetitions=3))
    label_reps = sorted(r.repetition for r in run.records if r.operation == "label")
    assert label_reps == [1, 2, 3]
