"""
Integration test: full benchmark run with a mocked Ollama server.

No real models are started. All HTTP calls are intercepted by a custom httpx transport
that handles all Ollama API endpoints used by the benchmark pipeline.

Finding 13 — mocked smoke tests only; no real model calls.
"""
from __future__ import annotations

import json
from unittest.mock import patch

import httpx
import pytest

from benchmark.client import BenchmarkConfig, OllamaBenchmarkClient
from benchmark.fixtures import ALL_FIXTURES, filter_fixtures
from benchmark.metrics import compute_metrics
from benchmark.qualification import VERDICT_QUALIFIED, qualify_model
from benchmark.runner import JSONL_FILENAME, load_completed_keys, run_model


# ── mock Ollama transport ─────────────────────────────────────────────────────

class FullOllamaMockTransport(httpx.BaseTransport):
    """Handles all Ollama API endpoints used by the benchmark pipeline.

    /api/version  → version string
    /api/tags     → model list
    /api/show     → model metadata (digest, parameter_size, quantization_level)
    /api/generate → load/unload (returns empty response)
    /api/chat     → returns the expected category for the fixture's answer
    """

    def __init__(self, model: str, category_by_answer: dict[str, str] | None = None) -> None:
        self._model = model
        self._category_by_answer = category_by_answer or {}

    def handle_request(self, request: httpx.Request) -> httpx.Response:
        path = request.url.path

        if path == "/api/version":
            return httpx.Response(200, json={"version": "0.20.0"})

        if path == "/api/tags":
            return httpx.Response(200, json={"models": [{"name": self._model}]})

        if path == "/api/show":
            return httpx.Response(200, json={
                "digest": "sha256:abc123",
                "details": {
                    "parameter_size": "7B",
                    "quantization_level": "Q4_K_M",
                },
            })

        if path == "/api/generate":
            # load_model / unload_model — return minimal valid response
            return httpx.Response(200, json={"done": True})

        if path == "/api/chat":
            body = json.loads(request.content)
            user_content = body["messages"][1]["content"]
            try:
                user_data = json.loads(user_content)
                answer = user_data["candidates"][0]["answer"]
            except Exception:
                answer = ""
            category = self._category_by_answer.get(answer, "CORRECT")
            resp_body = {
                "model": self._model,
                "message": {
                    "role": "assistant",
                    "content": json.dumps({
                        "classifications": [
                            {"candidateId": "candidate-1", "category": category}
                        ]
                    }),
                },
                "eval_duration": 50_000_000,
                "done": True,
            }
            return httpx.Response(200, json=resp_body)

        return httpx.Response(404, content=b"not found")


def _patched_client(config: BenchmarkConfig, transport: httpx.BaseTransport) -> None:
    """Patch OllamaBenchmarkClient.__init__ to inject our mock transport."""
    def _init(self_inner, cfg):
        self_inner._config = cfg
        self_inner._http = httpx.Client(transport=transport, base_url=cfg.base_url)

    return patch.object(OllamaBenchmarkClient, "__init__", _init)


def _build_category_map(fixtures) -> dict[str, str]:
    """Map answer text → expected category for all fixtures."""
    return {f.answer: f.expected for f in fixtures}


# ── tests ─────────────────────────────────────────────────────────────────────

class TestOllamaBenchmarkClientMocked:
    """Unit-level client tests using the mock transport directly."""

    def _make_client(self, transport: httpx.BaseTransport) -> OllamaBenchmarkClient:
        config = BenchmarkConfig(base_url="http://localhost:11434", timeout_s=10.0)
        instance = OllamaBenchmarkClient.__new__(OllamaBenchmarkClient)
        instance._config = config
        instance._http = httpx.Client(transport=transport, base_url="http://localhost:11434")
        return instance

    def test_model_available_true(self):
        transport = FullOllamaMockTransport("mymodel")
        client = self._make_client(transport)
        assert client.model_available("mymodel") is True

    def test_model_available_false(self):
        transport = FullOllamaMockTransport("mymodel")
        client = self._make_client(transport)
        assert client.model_available("other-model") is False

    def test_get_version_returns_string(self):
        transport = FullOllamaMockTransport("mymodel")
        client = self._make_client(transport)
        assert client.get_version() == "0.20.0"

    def test_show_model_returns_digest(self):
        transport = FullOllamaMockTransport("mymodel")
        client = self._make_client(transport)
        info = client.show_model("mymodel")
        assert info.get("digest") == "sha256:abc123"

    def test_call_returns_valid_json_content(self):
        transport = FullOllamaMockTransport("mymodel", {"some answer": "CORRECT"})
        client = self._make_client(transport)
        result = client.call("mymodel", "system", "user")
        assert result.content is not None
        assert not result.timed_out
        assert not result.http_error
        data = json.loads(result.content)
        assert "classifications" in data

    def test_load_model_returns_float(self):
        transport = FullOllamaMockTransport("mymodel")
        client = self._make_client(transport)
        elapsed = client.load_model("mymodel")
        assert isinstance(elapsed, float)
        assert elapsed >= 0.0

    def test_load_model_uses_benchmark_context_limit(self):
        seen = {}

        def handler(request):
            seen.update(json.loads(request.content))
            return httpx.Response(200, json={"done": True})

        client = self._make_client(httpx.MockTransport(handler))
        client.load_model("mymodel")
        assert seen["options"]["num_ctx"] == 4096

    def test_call_eval_duration_captured(self):
        transport = FullOllamaMockTransport("mymodel")
        client = self._make_client(transport)
        result = client.call("mymodel", "system", "user")
        assert result.ollama_eval_duration_ns == 50_000_000


class TestFullRunGroupA:
    """End-to-end: run group A fixtures with a perfect judge, verify records."""

    def test_all_group_a_records_written(self, tmp_path):
        model = "perfect-judge"
        fixtures = filter_fixtures(["A"])
        config = BenchmarkConfig(base_url="http://localhost:11434", timeout_s=10.0)
        transport = FullOllamaMockTransport(model, _build_category_map(fixtures))

        with _patched_client(config, transport):
            records = run_model(
                model=model, fixtures=fixtures, config=config,
                output_dir=tmp_path, trials=1, max_retries=0,
            )

        assert len(records) == len(fixtures)
        assert all(r.predicted == "CORRECT" for r in records)
        assert all(r.attempt_count == 1 for r in records)

    def test_jsonl_has_all_fields(self, tmp_path):
        model = "perfect-judge"
        fixtures = filter_fixtures(["A"])
        config = BenchmarkConfig(base_url="http://localhost:11434", timeout_s=10.0)
        transport = FullOllamaMockTransport(model, _build_category_map(fixtures))

        with _patched_client(config, transport):
            run_model(model=model, fixtures=fixtures, config=config,
                      output_dir=tmp_path, trials=1, max_retries=0)

        lines = (tmp_path / JSONL_FILENAME).read_text().strip().split("\n")
        for line in lines:
            data = json.loads(line)
            for key in ("config_hash", "attempts", "attempt_count", "fixture_hash",
                        "assumption_basis", "trial_seed", "total_latency_s"):
                assert key in data, f"Missing field: {key}"

    def test_metrics_perfect_on_group_a(self, tmp_path):
        model = "perfect-judge"
        fixtures = filter_fixtures(["A"])
        config = BenchmarkConfig(base_url="http://localhost:11434", timeout_s=10.0)
        transport = FullOllamaMockTransport(model, _build_category_map(fixtures))

        with _patched_client(config, transport):
            run_model(model=model, fixtures=fixtures, config=config,
                      output_dir=tmp_path, trials=1, max_retries=0)

        raw = [json.loads(line) for line in (tmp_path / JSONL_FILENAME).read_text().strip().split("\n")]
        m = compute_metrics(raw)
        assert m.all_candidate.legitimate_retention == pytest.approx(1.0)
        assert m.accuracy == pytest.approx(1.0)

    def test_manifest_created(self, tmp_path):
        model = "perfect-judge"
        fixtures = filter_fixtures(["A"])
        config = BenchmarkConfig(base_url="http://localhost:11434", timeout_s=10.0)
        transport = FullOllamaMockTransport(model)

        with _patched_client(config, transport):
            run_model(model=model, fixtures=fixtures, config=config,
                      output_dir=tmp_path, trials=1, max_retries=0)

        from benchmark.manifest import load_manifest
        manifest = load_manifest(tmp_path)
        assert manifest is not None
        assert manifest.model_name == model
        assert manifest.model_digest == "sha256:abc123"
        assert manifest.ollama_version == "0.20.0"
        assert manifest.cold_load_s is not None


class TestResumeMode:
    """Resume: second run skips already-completed records."""

    def test_second_run_skips_all_completed(self, tmp_path):
        model = "perfect-judge"
        fixtures = filter_fixtures(["A"])
        config = BenchmarkConfig(base_url="http://localhost:11434", timeout_s=10.0)
        transport = FullOllamaMockTransport(model, _build_category_map(fixtures))

        with _patched_client(config, transport):
            run_model(model=model, fixtures=fixtures, config=config,
                      output_dir=tmp_path, trials=1, max_retries=0)
            from benchmark.manifest import load_manifest
            original_run_id = load_manifest(tmp_path).run_id
            records_second = run_model(model=model, fixtures=fixtures, config=config,
                                       output_dir=tmp_path, trials=1, max_retries=0)

        assert records_second == []
        lines = (tmp_path / JSONL_FILENAME).read_text().strip().split("\n")
        assert len(lines) == len(fixtures)
        assert load_manifest(tmp_path).run_id == original_run_id

    def test_completed_keys_loaded_correctly(self, tmp_path):
        model = "perfect-judge"
        fixtures = filter_fixtures(["A"])
        config = BenchmarkConfig(base_url="http://localhost:11434", timeout_s=10.0)
        transport = FullOllamaMockTransport(model)

        with _patched_client(config, transport):
            run_model(model=model, fixtures=fixtures, config=config,
                      output_dir=tmp_path, trials=1, max_retries=0)

        keys = load_completed_keys(tmp_path)
        assert len(keys) == len(fixtures)


class TestForceRestart:
    """force_restart=True discards prior results and starts fresh."""

    def test_force_restart_produces_fresh_records(self, tmp_path):
        model = "perfect-judge"
        fixtures = filter_fixtures(["A"])
        config = BenchmarkConfig(base_url="http://localhost:11434", timeout_s=10.0)
        transport = FullOllamaMockTransport(model, _build_category_map(fixtures))

        with _patched_client(config, transport):
            run_model(model=model, fixtures=fixtures, config=config,
                      output_dir=tmp_path, trials=1, max_retries=0)
            records_new = run_model(model=model, fixtures=fixtures, config=config,
                                    output_dir=tmp_path, trials=1, max_retries=0,
                                    force_restart=True)

        assert len(records_new) == len(fixtures)
        lines = (tmp_path / JSONL_FILENAME).read_text().strip().split("\n")
        assert len(lines) == len(fixtures)


class TestLifecycleFailureHandling:
    def test_invalid_warmup_aborts_and_unloads(self, tmp_path):
        model = "broken-warmup"

        class BrokenWarmupTransport(FullOllamaMockTransport):
            def __init__(self):
                super().__init__(model)
                self.generate_calls = 0

            def handle_request(self, request):
                if request.url.path == "/api/generate":
                    self.generate_calls += 1
                if request.url.path == "/api/chat":
                    return httpx.Response(200, json={
                        "message": {"content": "not-json"}, "done": True,
                    })
                return super().handle_request(request)

        transport = BrokenWarmupTransport()
        config = BenchmarkConfig(base_url="http://localhost:11434", timeout_s=10.0)
        with _patched_client(config, transport):
            with pytest.raises(ValueError):
                run_model(
                    model=model, fixtures=filter_fixtures(["A"]), config=config,
                    output_dir=tmp_path, trials=1, max_retries=0,
                )
        assert transport.generate_calls == 2  # load and finally-unload
