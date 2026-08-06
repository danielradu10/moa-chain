"""Tests for benchmark/runner.py (Findings 3, 6, 8)"""
import dataclasses
import json
import pytest
from pathlib import Path
from unittest.mock import MagicMock, patch

from benchmark.client import BenchmarkConfig, CallResult
from benchmark.fixtures import ALL_FIXTURES, Fixture
from benchmark.runner import (
    AttemptDetail,
    PredictionRecord,
    _completed_key,
    _shuffle_fixtures,
    _run_single,
    load_completed_keys,
    _append_record,
    JSONL_FILENAME,
)


def _make_fixture(
    group: str = "A",
    tx_id: str = "scenario-01-control-before",
    perspective: str = "p1",
    expected: str = "CORRECT",
    is_adversarial: bool = False,
) -> Fixture:
    return Fixture(
        group=group,
        scenario_id="scenario-01",
        tx_id=tx_id,
        perspective=perspective,
        prompt="What is the main benefit of unit tests?",
        answer="Unit tests catch regressions early.",
        expected=expected,
        is_adversarial=is_adversarial,
        assumption_basis="canonical",
    )


def _make_client(
    content: str | None = None,
    timed_out: bool = False,
    http_error: bool = False,
    eval_ns: int | None = None,
) -> MagicMock:
    client = MagicMock()
    client.call.return_value = CallResult(
        content=content,
        latency_s=0.5,
        timed_out=timed_out,
        http_error=http_error,
        error_message="err" if (timed_out or http_error) else None,
        ollama_eval_duration_ns=eval_ns,
    )
    return client


VALID_RESPONSE = '{"classifications":[{"candidateId":"candidate-1","category":"CORRECT"}]}'


class TestCompletedKey:
    def test_includes_config_hash(self):
        r = {
            "config_hash": "abc", "model": "m", "group": "A",
            "tx_id": "tx1", "perspective": "p1", "candidate_id": "candidate-1", "trial": 1,
        }
        key = _completed_key(r)
        assert key[0] == "abc"

    def test_full_key_tuple_length(self):
        key = _completed_key({})
        assert len(key) == 5

    def test_candidate_id_in_key(self):
        r = {"config_hash": "", "model": "", "group": "", "tx_id": "",
             "perspective": "", "candidate_id": "candidate-1", "trial": 1}
        key = _completed_key(r)
        assert key[3] == "candidate-1"


class TestFixtureShuffle:
    """Finding 6: deterministic shuffle same across models for same trial seed."""

    def test_same_seed_same_order(self):
        fixtures = ALL_FIXTURES[:10]
        order1 = [f.tx_id + f.perspective for f in _shuffle_fixtures(fixtures, 42001)]
        order2 = [f.tx_id + f.perspective for f in _shuffle_fixtures(fixtures, 42001)]
        assert order1 == order2

    def test_different_seeds_different_orders(self):
        fixtures = ALL_FIXTURES[:10]
        order1 = [f.tx_id + f.perspective for f in _shuffle_fixtures(fixtures, 42001)]
        order2 = [f.tx_id + f.perspective for f in _shuffle_fixtures(fixtures, 42002)]
        assert order1 != order2, "Different trial seeds should produce different orders"

    def test_shuffle_does_not_modify_original(self):
        fixtures = ALL_FIXTURES[:5]
        original_ids = [(f.tx_id, f.perspective) for f in fixtures]
        _shuffle_fixtures(fixtures, 99)
        assert [(f.tx_id, f.perspective) for f in fixtures] == original_ids

    def test_all_fixtures_present_after_shuffle(self):
        fixtures = ALL_FIXTURES[:8]
        shuffled = _shuffle_fixtures(fixtures, 123)
        assert len(shuffled) == len(fixtures)
        assert set(id(f) for f in shuffled) == set(id(f) for f in fixtures)


class TestAttemptPersistence:
    """Finding 3: all attempts stored, successful retry does not overwrite first attempt."""

    def test_successful_first_attempt_stored(self):
        client = _make_client(content=VALID_RESPONSE)
        fixture = _make_fixture()
        record = _run_single(client, "model", fixture, trial=1, trial_seed=1,
                             config_hash="hash", max_retries=0)
        assert record.attempt_count == 1
        assert len(record.attempts) == 1
        assert record.attempts[0]["attempt_number"] == 1
        assert record.attempts[0]["parse_error"] is False

    def test_first_attempt_error_preserved_on_retry_success(self):
        """First attempt: parse error. Second attempt: success. Both must be in attempts list."""
        bad = '{"bad": true}'
        good = VALID_RESPONSE
        client = MagicMock()
        client.call.side_effect = [
            CallResult(content=bad, latency_s=0.4, timed_out=False, http_error=False,
                       error_message=None, ollama_eval_duration_ns=None),
            CallResult(content=good, latency_s=0.6, timed_out=False, http_error=False,
                       error_message=None, ollama_eval_duration_ns=None),
        ]
        fixture = _make_fixture()
        record = _run_single(client, "model", fixture, trial=1, trial_seed=1,
                             config_hash="hash", max_retries=1)

        assert record.predicted == "CORRECT"
        assert record.attempt_count == 2
        assert len(record.attempts) == 2
        # First attempt: parse error, raw content preserved
        assert record.attempts[0]["attempt_number"] == 1
        assert record.attempts[0]["parse_error"] is True
        assert record.attempts[0]["raw_content"] == bad
        assert record.attempts[0]["latency_s"] == pytest.approx(0.4)
        # Second attempt: success
        assert record.attempts[1]["attempt_number"] == 2
        assert record.attempts[1]["parse_error"] is False
        assert record.attempts[1]["raw_content"] == good

    def test_total_latency_is_sum_of_all_attempts(self):
        client = MagicMock()
        client.call.side_effect = [
            CallResult(content='{"bad":1}', latency_s=1.0, timed_out=False, http_error=False,
                       error_message=None, ollama_eval_duration_ns=None),
            CallResult(content=VALID_RESPONSE, latency_s=2.0, timed_out=False, http_error=False,
                       error_message=None, ollama_eval_duration_ns=None),
        ]
        record = _run_single(client, "model", _make_fixture(), trial=1, trial_seed=1,
                             config_hash="h", max_retries=1)
        assert record.total_latency_s == pytest.approx(3.0)

    def test_timeout_not_retried(self):
        client = _make_client(timed_out=True)
        record = _run_single(client, "m", _make_fixture(), trial=1, trial_seed=1,
                             config_hash="h", max_retries=3)
        assert record.attempt_count == 1
        assert record.timed_out is True
        assert record.predicted is None

    def test_http_error_not_retried(self):
        client = _make_client(http_error=True)
        record = _run_single(client, "m", _make_fixture(), trial=1, trial_seed=1,
                             config_hash="h", max_retries=3)
        assert record.attempt_count == 1
        assert record.http_error is True

    def test_parse_error_exhausted_retries(self):
        client = _make_client(content='{"bad":1}')
        record = _run_single(client, "m", _make_fixture(), trial=1, trial_seed=1,
                             config_hash="h", max_retries=0)
        assert record.predicted is None
        assert record.parse_error is True
        assert record.attempt_count == 1


class TestLoadCompletedKeys:
    def test_empty_dir(self, tmp_path):
        assert load_completed_keys(tmp_path) == set()

    def test_missing_file(self, tmp_path):
        assert load_completed_keys(tmp_path / "nonexistent") == set()

    def test_reads_existing_jsonl(self, tmp_path):
        record = PredictionRecord(
            model="m", group="A", scenario_id="s", tx_id="tx1", candidate_id="candidate-1",
            perspective="p1", prompt="Q", answer="A", expected="CORRECT",
            assumption_basis="canonical", predicted="CORRECT", is_adversarial=False,
            is_correct=True, attempts=[], attempt_count=1, final_latency_s=1.0,
            total_latency_s=1.0, timed_out=False, http_error=False, parse_error=False,
            error_message=None, config_hash="hash123", fixture_hash="fh",
            seed=42, trial=1, trial_seed=420001,
            timestamp="2025-01-01T00:00:00+00:00",
        )
        (tmp_path / JSONL_FILENAME).write_text(
            json.dumps(dataclasses.asdict(record)) + "\n"
        )
        keys = load_completed_keys(tmp_path)
        assert ("hash123", "m", "fh", "candidate-1", 1) in keys

    def test_rejects_malformed_lines(self, tmp_path):
        (tmp_path / JSONL_FILENAME).write_text("not json\n")
        with pytest.raises(RuntimeError, match="Corrupt"):
            load_completed_keys(tmp_path)


class TestAppendRecord:
    def _make_record(self) -> PredictionRecord:
        return PredictionRecord(
            model="m", group="A", scenario_id="s", tx_id="tx1", candidate_id="candidate-1",
            perspective="p1", prompt="Q", answer="A", expected="CORRECT",
            assumption_basis="canonical", predicted="CORRECT", is_adversarial=False,
            is_correct=True, attempts=[], attempt_count=1, final_latency_s=1.0,
            total_latency_s=1.0, timed_out=False, http_error=False, parse_error=False,
            error_message=None, config_hash="hash", fixture_hash="fh",
            seed=42, trial=1, trial_seed=420001,
            timestamp="2025-01-01T00:00:00+00:00",
        )

    def test_creates_and_appends(self, tmp_path):
        _append_record(self._make_record(), tmp_path)
        lines = (tmp_path / JSONL_FILENAME).read_text().strip().split("\n")
        assert len(lines) == 1
        data = json.loads(lines[0])
        assert data["model"] == "m"
        assert data["config_hash"] == "hash"

    def test_appends_multiple(self, tmp_path):
        _append_record(self._make_record(), tmp_path)
        _append_record(self._make_record(), tmp_path)
        lines = (tmp_path / JSONL_FILENAME).read_text().strip().split("\n")
        assert len(lines) == 2

    def test_attempts_list_is_serialized(self, tmp_path):
        r = self._make_record()
        r = dataclasses.replace(
            r,
            attempts=[{"attempt_number": 1, "raw_content": "x", "latency_s": 1.0,
                        "timed_out": False, "http_error": False, "parse_error": False,
                        "error_message": None, "ollama_eval_duration_ns": None}],
            attempt_count=1,
        )
        _append_record(r, tmp_path)
        data = json.loads((tmp_path / JSONL_FILENAME).read_text().strip())
        assert isinstance(data["attempts"], list)
        assert len(data["attempts"]) == 1
        assert data["attempts"][0]["attempt_number"] == 1
