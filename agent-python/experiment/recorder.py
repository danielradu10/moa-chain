"""Per-call structured recorder for experiment runs.

When EXPERIMENT_DIR is set, each LLM call is appended as one JSON line to
{experiment_dir}/agents/{validator_name}.jsonl. Token counts are captured from
provider log lines using a ContextVar-based handler that is safe under
asyncio.gather() concurrency (each task gets its own context copy).
"""
from __future__ import annotations

import contextvars
import json
import logging
import re
import threading
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from experiment.sanitizer import sanitize_error

_TOKEN_RE = re.compile(
    r"input_tokens=(\S+)\s+output_tokens=(\S+)\s+total_tokens=(\S+)"
)

# Per-task sink: each asyncio.gather task gets its own copy via ContextVar.
_token_sink: contextvars.ContextVar[dict | None] = contextvars.ContextVar(
    "experiment_token_sink", default=None
)


class _ContextVarTokenCapture(logging.Handler):
    """Write token counts from llm_call log lines into the current task's ContextVar."""

    def emit(self, record: logging.LogRecord) -> None:
        sink = _token_sink.get()
        if sink is None:
            return
        msg = record.getMessage()
        if "llm_call" not in msg:
            return
        m = _TOKEN_RE.search(msg)
        if not m:
            return
        try:
            sink["input"] = int(m.group(1))
            sink["output"] = int(m.group(2))
            sink["total"] = int(m.group(3))
        except (ValueError, IndexError):
            pass


_capture_handler = _ContextVarTokenCapture()
_capture_handler.setLevel(logging.INFO)
_providers_logger = logging.getLogger("providers")


def _install_capture_handler() -> None:
    if _capture_handler not in _providers_logger.handlers:
        _providers_logger.addHandler(_capture_handler)
        _providers_logger.setLevel(logging.INFO)


@dataclass
class AgentCallRecord:
    run_id: str | None
    validator_id: str
    validator_name: str
    provider: str
    model: str
    agent_endpoint: str
    operation: str
    tx_hash: str | None
    round: int | None
    mini_round: int | None
    start_ts: str
    end_ts: str
    latency_ms: float
    request_payload: Any
    parsed_response: Any
    input_tokens: int | None
    output_tokens: int | None
    total_tokens: int | None
    success: bool
    error: str | None = None
    mocked: bool = False
    provider_called: bool = True


class CallRecorder:
    """Appends one JSONL line per LLM call to {experiment_dir}/agents/{validator_name}.jsonl.

    Safe for concurrent use: file writes are protected by a threading.Lock.
    No-op when experiment_dir is empty.
    """

    def __init__(
        self,
        validator_id: str,
        validator_name: str,
        provider: str,
        model: str,
        agent_endpoint: str,
        experiment_dir: str,
    ) -> None:
        self._validator_id = validator_id
        self._validator_name = validator_name
        self._provider = provider
        self._model = model
        self._agent_endpoint = agent_endpoint
        self._path: Path | None = None
        self._lock = threading.Lock()

        if experiment_dir:
            self._path = Path(experiment_dir) / "agents" / f"{validator_name}.jsonl"
            self._path.parent.mkdir(parents=True, exist_ok=True)
            _install_capture_handler()

    @property
    def is_active(self) -> bool:
        return self._path is not None

    def append(self, record: AgentCallRecord) -> None:
        if not self._path:
            return
        line = json.dumps(asdict(record), default=str) + "\n"
        with self._lock:
            with self._path.open("a", encoding="utf-8") as f:
                f.write(line)

    def build_record(
        self,
        *,
        run_id: str | None,
        operation: str,
        tx_hash: str | None,
        round_num: int | None,
        mini_round: int | None,
        start_ts: datetime,
        end_ts: datetime,
        request_payload: Any,
        parsed_response: Any,
        input_tokens: int | None,
        output_tokens: int | None,
        total_tokens: int | None,
        success: bool,
        error: str | None,
        mocked: bool = False,
        provider_called: bool = True,
    ) -> AgentCallRecord:
        latency_ms = (end_ts - start_ts).total_seconds() * 1000
        return AgentCallRecord(
            run_id=run_id,
            validator_id=self._validator_id,
            validator_name=self._validator_name,
            provider=self._provider,
            model=self._model,
            agent_endpoint=self._agent_endpoint,
            operation=operation,
            tx_hash=tx_hash,
            round=round_num,
            mini_round=mini_round,
            start_ts=start_ts.isoformat(),
            end_ts=end_ts.isoformat(),
            latency_ms=round(latency_ms, 3),
            request_payload=request_payload,
            parsed_response=parsed_response,
            input_tokens=input_tokens,
            output_tokens=output_tokens,
            total_tokens=total_tokens,
            success=success,
            error=sanitize_error(error),
            mocked=mocked,
            provider_called=provider_called,
        )


async def run_with_token_capture(
    coro,
) -> tuple[Any, int | None, int | None, int | None, str | None]:
    """Run coro; return (result, input_tok, output_tok, total_tok, error_str).

    Uses a per-task ContextVar so concurrent asyncio.gather() calls do not
    interfere with each other's token counts.
    """
    sink: dict = {}
    token = _token_sink.set(sink)
    result = None
    error: str | None = None
    try:
        result = await coro
    except Exception as exc:
        error = f"{type(exc).__name__}: {exc}"
        raise
    finally:
        _token_sink.reset(token)
    return result, sink.get("input"), sink.get("output"), sink.get("total"), error


def _parse_int_header(v: str | None) -> int | None:
    """Parse an integer HTTP header value; return None if absent or non-numeric."""
    if v is None:
        return None
    try:
        return int(v)
    except ValueError:
        return None
