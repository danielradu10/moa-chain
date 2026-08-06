"""
Synchronous Ollama HTTP client for the judge benchmark.

Reuses the same /api/chat endpoint and httpx pattern as agent-python/providers/ollama_provider.py
but is synchronous (CLI context), adds benchmark-specific options (num_ctx, num_predict, think,
seed, keep_alive, structured-output schema), and is isolated from the production service.
"""
from __future__ import annotations

import json
import logging
import time
from dataclasses import dataclass, field

import httpx

from benchmark.prompt import RESPONSE_JSON_SCHEMA

logger = logging.getLogger(__name__)

DEFAULT_SEED = 42
DEFAULT_KEEP_ALIVE = "30m"


class OllamaUnavailableError(RuntimeError):
    """The Ollama daemon cannot be reached or disconnected unexpectedly."""


@dataclass
class CallResult:
    content: str | None      # raw model output, None on error
    latency_s: float
    timed_out: bool
    http_error: bool
    error_message: str | None
    ollama_eval_duration_ns: int | None = None   # from Ollama response metadata


@dataclass
class BenchmarkConfig:
    base_url: str = "http://127.0.0.1:11434"
    temperature: float = 0.0
    num_ctx: int = 4096
    num_predict: int = 256
    think: bool = False
    timeout_s: float = 120.0
    seed: int = DEFAULT_SEED
    keep_alive: str = DEFAULT_KEEP_ALIVE


class OllamaBenchmarkClient:
    """Thin synchronous Ollama client for benchmark use."""

    def __init__(self, config: BenchmarkConfig) -> None:
        self._config = config
        self._http = httpx.Client(base_url=config.base_url, timeout=config.timeout_s)

    def close(self) -> None:
        self._http.close()

    def __enter__(self) -> "OllamaBenchmarkClient":
        return self

    def __exit__(self, *_: object) -> None:
        self.close()

    # ── introspection ─────────────────────────────────────────────────────────

    def get_version(self) -> str | None:
        """Return Ollama version string, or None on error."""
        try:
            resp = self._http.get("/api/version", timeout=10.0)
            if resp.status_code == 200:
                return resp.json().get("version")
        except Exception as exc:
            logger.warning("Could not get Ollama version: %s", exc)
        return None

    def show_model(self, model: str) -> dict:
        """Return model metadata from /api/show. Returns {} on error."""
        try:
            resp = self._http.post("/api/show", json={"name": model}, timeout=30.0)
            if resp.status_code == 200:
                data = resp.json()
                # /api/show does not consistently expose digest; /api/tags does.
                for entry in self.list_model_entries():
                    if entry.get("name") in {model, f"{model}:latest"}:
                        if entry.get("digest"):
                            data["digest"] = entry["digest"]
                        break
                return data
        except Exception as exc:
            logger.warning("Could not show model %s: %s", model, exc)
        return {}

    def list_models(self) -> list[str]:
        """Return installed model names; daemon failures are never "not found"."""
        try:
            resp = self._http.get("/api/tags", timeout=10.0)
            if resp.status_code == 200:
                return [m["name"] for m in resp.json().get("models", [])]
            raise OllamaUnavailableError(f"Ollama /api/tags returned HTTP {resp.status_code}")
        except httpx.RequestError as exc:
            raise OllamaUnavailableError(f"Cannot reach Ollama at {self._config.base_url}: {exc}") from exc

    def list_model_entries(self) -> list[dict]:
        """Return complete /api/tags model entries, including immutable digests."""
        try:
            resp = self._http.get("/api/tags", timeout=10.0)
            if resp.status_code == 200:
                return [m for m in resp.json().get("models", []) if isinstance(m, dict)]
        except httpx.RequestError as exc:
            raise OllamaUnavailableError(f"Cannot reach Ollama at {self._config.base_url}: {exc}") from exc
        raise OllamaUnavailableError("Ollama /api/tags did not return model metadata")

    def verify_capabilities(self) -> dict[str, bool]:
        """Verify required API capabilities from the Ollama server version.

        This benchmark requires Ollama >= 0.20.0. Earlier releases include
        known combinations where Qwen 3.5 ignores `format` with `think=false`.
        Required options are
        sent on every probe/inference call; an HTTP rejection is surfaced as an
        error rather than treated as support.
        """
        version = self.get_version()
        try:
            parts = tuple(int(p) for p in (version or "").split(".")[:3])
        except ValueError:
            parts = ()
        supported = bool(parts and parts >= (0, 20, 0))
        return {
            "structured_output_schema": supported,
            "seed_option": supported,
            "think_option": supported,
            "format_with_think_false": supported,
        }

    def model_available(self, model: str) -> bool:
        """Return True if `model` is present in Ollama (exact match or :latest suffix)."""
        available = self.list_models()
        if model in available:
            return True
        if not model.endswith(":latest") and f"{model}:latest" in available:
            return True
        return False

    # ── model lifecycle ───────────────────────────────────────────────────────

    def load_model(self, model: str) -> float:
        """Pre-load model into memory. Returns cold-load wall-clock seconds.

        Sends an empty generate request with keep_alive so Ollama loads the
        model without generating tokens. Times the full round-trip.
        """
        payload = {
            "model": model,
            "prompt": "",
            "stream": False,
            "keep_alive": self._config.keep_alive,
            # Bound preload memory exactly as benchmark inference does. Without
            # this, large-context models may allocate their native context during
            # preload and crash an otherwise adequately sized CPU-only host.
            "options": {"num_ctx": self._config.num_ctx},
        }
        t0 = time.perf_counter()
        try:
            resp = self._http.post(
                "/api/generate", json=payload,
                timeout=max(self._config.timeout_s, 300.0),
            )
            elapsed = time.perf_counter() - t0
            if resp.status_code != 200:
                raise RuntimeError(
                    f"load_model: HTTP {resp.status_code} for model {model}: {resp.text[:200]}"
                )
            return elapsed
        except httpx.RequestError as exc:
            raise OllamaUnavailableError(
                f"Ollama disconnected while loading {model}: {exc}"
            ) from exc
        except Exception as exc:
            raise RuntimeError(f"load_model failed for {model}: {exc}") from exc

    def unload_model(self, model: str) -> None:
        """Unload model from memory (keep_alive=0)."""
        payload = {"model": model, "prompt": "", "stream": False, "keep_alive": 0}
        try:
            resp = self._http.post("/api/generate", json=payload, timeout=30.0)
            if resp.status_code != 200:
                raise RuntimeError(f"HTTP {resp.status_code}: {resp.text[:200]}")
        except Exception as exc:
            raise RuntimeError(f"unload_model failed for {model}: {exc}") from exc

    # ── inference ─────────────────────────────────────────────────────────────

    def call(self, model: str, system_prompt: str, user_prompt: str) -> CallResult:
        """Send a single chat request and return the raw content string.

        Sends structured-output JSON schema via the `format` field (Ollama >= 0.3.9).
        Required server capabilities are checked before the lifecycle begins;
        HTTP rejection of an option is returned as an explicit failed attempt.

        Options sent (all recorded in the run manifest):
          temperature, num_ctx, num_predict, seed — standard inference params
          think=false — suppresses chain-of-thought in thinking models (top-level field)
          keep_alive — keeps the model in memory between calls
        """
        options: dict = {
            "temperature": self._config.temperature,
            "num_ctx": self._config.num_ctx,
            "num_predict": self._config.num_predict,
            "seed": self._config.seed,
        }

        payload: dict = {
            "model": model,
            "messages": [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_prompt},
            ],
            "stream": False,
            "options": options,
            "keep_alive": self._config.keep_alive,
            "format": RESPONSE_JSON_SCHEMA,
        }

        # think=false suppresses chain-of-thought (phi4-reasoning and similar models).
        # This is a top-level Ollama parameter, not in options. Models that do not
        # support the `think` field will receive it and log a warning server-side;
        # we log our intent here so the benchmark log is self-documenting.
        if not self._config.think:
            payload["think"] = False
            logger.debug("Sending think=false to suppress chain-of-thought for model %s", model)

        t0 = time.perf_counter()
        try:
            resp = self._http.post(
                "/api/chat", json=payload, timeout=self._config.timeout_s
            )
        except httpx.TimeoutException as exc:
            return CallResult(
                content=None,
                latency_s=time.perf_counter() - t0,
                timed_out=True,
                http_error=False,
                error_message=str(exc),
            )
        except httpx.RequestError as exc:
            return CallResult(
                content=None,
                latency_s=time.perf_counter() - t0,
                timed_out=False,
                http_error=True,
                error_message=str(exc),
            )

        latency = time.perf_counter() - t0

        if resp.status_code != 200:
            return CallResult(
                content=None,
                latency_s=latency,
                timed_out=False,
                http_error=True,
                error_message=f"HTTP {resp.status_code}: {resp.text[:200]}",
            )

        try:
            data = resp.json()
        except json.JSONDecodeError as exc:
            return CallResult(
                content=None,
                latency_s=latency,
                timed_out=False,
                http_error=True,
                error_message=f"invalid JSON from Ollama: {exc}",
            )

        content: str = data.get("message", {}).get("content", "")
        if not content:
            return CallResult(
                content=None,
                latency_s=latency,
                timed_out=False,
                http_error=True,
                error_message="Ollama returned empty message content",
            )

        eval_duration_ns: int | None = data.get("eval_duration")

        return CallResult(
            content=content,
            latency_s=latency,
            timed_out=False,
            http_error=False,
            error_message=None,
            ollama_eval_duration_ns=eval_duration_ns,
        )
