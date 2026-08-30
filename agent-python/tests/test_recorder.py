import json
from datetime import datetime, timezone
from pathlib import Path

import pytest

from experiment.recorder import AgentCallRecord, CallRecorder


def _make_recorder(tmp_path: Path, **kwargs) -> CallRecorder:
    defaults = dict(
        validator_id="validator-1",
        validator_name="gpt-5.4-mini-1",
        provider="openai",
        model="gpt-5.4-mini",
        agent_endpoint="http://127.0.0.1:8100",
        experiment_dir=str(tmp_path),
    )
    defaults.update(kwargs)
    return CallRecorder(**defaults)


def _make_record(recorder: CallRecorder, **overrides) -> AgentCallRecord:
    now = datetime.now(timezone.utc)
    defaults = dict(
        run_id="run-abc123",
        operation="label",
        tx_hash="txhash-001",
        round_num=5,
        mini_round=1,
        start_ts=now,
        end_ts=now,
        request_payload={"tx_hash": "txhash-001", "prompt": "test"},
        parsed_response={"tx_hash": "txhash-001", "labels": []},
        input_tokens=100,
        output_tokens=20,
        total_tokens=120,
        success=True,
        error=None,
    )
    defaults.update(overrides)
    return recorder.build_record(**defaults)


def test_noop_when_no_experiment_dir(tmp_path: Path):
    recorder = CallRecorder(
        validator_id="v-1",
        validator_name="test",
        provider="openai",
        model="gpt-5.4-mini",
        agent_endpoint="http://127.0.0.1:8100",
        experiment_dir="",
    )
    assert not recorder.is_active
    # Should not raise and should not create any files
    record = _make_record(recorder)
    recorder.append(record)
    assert not any(tmp_path.iterdir())


def test_appends_jsonl_record(tmp_path: Path):
    recorder = _make_recorder(tmp_path)
    assert recorder.is_active

    record = _make_record(recorder)
    recorder.append(record)

    out_file = tmp_path / "agents" / "gpt-5.4-mini-1.jsonl"
    assert out_file.exists()
    lines = out_file.read_text().splitlines()
    assert len(lines) == 1
    data = json.loads(lines[0])
    assert data["operation"] == "label"
    assert data["tx_hash"] == "txhash-001"
    assert data["input_tokens"] == 100
    assert data["success"] is True


def test_mocked_record_has_zero_tokens_and_marker(tmp_path: Path):
    recorder = _make_recorder(tmp_path, validator_name="mocked-agent")
    record = _make_record(
        recorder,
        mocked=True,
        provider_called=False,
        input_tokens=0,
        output_tokens=0,
        total_tokens=0,
    )
    recorder.append(record)

    data = json.loads((tmp_path / "agents" / "mocked-agent.jsonl").read_text())
    assert data["mocked"] is True
    assert data["input_tokens"] == 0
    assert data["output_tokens"] == 0
    assert data["total_tokens"] == 0
    assert data["provider_called"] is False


def test_multiple_records_append(tmp_path: Path):
    recorder = _make_recorder(tmp_path)

    for i in range(3):
        record = _make_record(recorder, tx_hash=f"tx-{i:03d}", round_num=i)
        recorder.append(record)

    out_file = tmp_path / "agents" / "gpt-5.4-mini-1.jsonl"
    lines = out_file.read_text().splitlines()
    assert len(lines) == 3
    for i, line in enumerate(lines):
        data = json.loads(line)
        assert data["tx_hash"] == f"tx-{i:03d}"
        assert data["round"] == i


def test_run_id_propagated(tmp_path: Path):
    recorder = _make_recorder(tmp_path)
    record = _make_record(recorder, run_id="run-xyz-999")
    recorder.append(record)

    out_file = tmp_path / "agents" / "gpt-5.4-mini-1.jsonl"
    data = json.loads(out_file.read_text())
    assert data["run_id"] == "run-xyz-999"


def test_error_sanitized(tmp_path: Path):
    recorder = _make_recorder(tmp_path)
    record = _make_record(
        recorder,
        success=False,
        parsed_response=None,
        error="APIError: sk-secretkey123456 was rejected",
    )
    recorder.append(record)

    out_file = tmp_path / "agents" / "gpt-5.4-mini-1.jsonl"
    data = json.loads(out_file.read_text())
    assert "sk-secretkey123456" not in (data["error"] or "")
    assert "REDACTED" in (data["error"] or "")


def test_validator_fields_present(tmp_path: Path):
    recorder = _make_recorder(tmp_path)
    record = _make_record(recorder)
    recorder.append(record)

    out_file = tmp_path / "agents" / "gpt-5.4-mini-1.jsonl"
    data = json.loads(out_file.read_text())
    assert data["validator_id"] == "validator-1"
    assert data["validator_name"] == "gpt-5.4-mini-1"
    assert data["provider"] == "openai"
    assert data["model"] == "gpt-5.4-mini"
    assert data["agent_endpoint"] == "http://127.0.0.1:8100"


def test_latency_ms_positive(tmp_path: Path):
    recorder = _make_recorder(tmp_path)
    start = datetime(2026, 1, 1, 0, 0, 0, tzinfo=timezone.utc)
    end = datetime(2026, 1, 1, 0, 0, 1, tzinfo=timezone.utc)
    record = recorder.build_record(
        run_id="r1",
        operation="answer",
        tx_hash="tx1",
        round_num=1,
        mini_round=2,
        start_ts=start,
        end_ts=end,
        request_payload={},
        parsed_response=None,
        input_tokens=50,
        output_tokens=10,
        total_tokens=60,
        success=True,
        error=None,
    )
    recorder.append(record)

    out_file = tmp_path / "agents" / "gpt-5.4-mini-1.jsonl"
    data = json.loads(out_file.read_text())
    assert data["latency_ms"] == pytest.approx(1000.0, rel=0.01)
