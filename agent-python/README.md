# Agent Python Service

A protocol-safe LLM adapter for the moa-chain Go validator. This service provides
the three LLM-backed semantic operations required by the consensus protocol.
It is not consensus logic. Signatures, quorum rules, deterministic aggregation,
validator membership, and protocol finalization remain entirely in Go.

## Role in the Protocol

```
Go Validator
  │
  ├─ Mini-Round One ──────────► POST /label   (batch)
  │    block executor labels       subdomain classification per transaction
  │    each transaction
  │
  ├─ Mini-Round Two (answers) ─► POST /answer  (batch)
  │    each committee member       one LLM answer per transaction
  │    executes all prompts
  │
  └─ Mini-Round Two (judge) ──► POST /judge
       each validator judges        raw system + user prompt strings,
       all candidate answers        returns raw JSON string for Go to parse
```

The Go side calls this service through the `BatchAgent` interface
(see `agent/httpclient/`). The service never sees validator identities,
signatures, round keys, or quorum state.

## Operations

### 1. Labeling (`POST /label`)

Classifies each transaction prompt into one or more protocol-defined
coding subdomains. Called once per block during Mini-Round One.

- Input: batch of `{tx_hash, prompt}`, list of allowed subdomains
- Output: per-transaction label list with confidence scores
- Prompt: versioned `labeler_v1.txt`, hash returned in every response
- Validation: every returned subdomain must be in the allowed set; every
  input `tx_hash` must appear exactly once in the response; confidence
  must be in `[0, 1]`

### 2. Answering (`POST /answer`)

Produces one LLM answer per transaction prompt. Called once per block
per committee member during Mini-Round Two answer collection.

- Input: batch of `{tx_hash, prompt, subdomains}`
- Output: one non-empty answer string per transaction
- Prompt: versioned `answerer_v1.txt`, hash returned in every response
- Validation: one answer per input transaction; answer non-empty after
  trimming; no missing or extra `tx_hash`

### 3. Judging (`POST /judge`)

Forwards Go-prepared prompt strings to the LLM and returns the raw
structured JSON response. The Go side owns the judge prompt and parses
the response. This endpoint does the minimum validation needed to surface
provider errors early.

- Input: `{system_prompt, user_prompt}` — both built by Go
- Output: `{response}` — raw model output string
- Validation: response is valid JSON; every `category` in the JSON is
  one of `Correct`, `Hallucination`, `Malicious`, `Wrong`; every
  `reason` is non-empty after trimming
- No versioned prompt file on the Python side; the Go side owns
  `AnswerJudgePromptVersion` and `AnswerJudgePromptHash`

## Deployment Model

In production each physical validator node runs its own Python service and its
own Ollama instance. The Go validator connects to `localhost:8081` (or the
configured `AGENT_BASE_URL`). Validators operate independently: each calls
its own LLM to produce labels, answers, and classification votes.

In the goroutine-based integration tests (N nodes in one process), nodes can
share a single stub HTTP server because the responses are deterministic fakes
and no independence guarantee needs to be tested.

> **Benchmarking heads-up (under debate):** sharing one real Python service
> and one Ollama instance across all goroutine-nodes produces misleading
> latency numbers. In production validators run in parallel and round time
> ≈ single validator time. With a shared service they serialize through one
> Ollama, so measured round time ≈ N × single validator time — the wrong
> scaling curve. The right benchmark topology (N separate services, one or N
> Ollama instances) will be decided when benchmarking work begins.

## Architecture

```
agent-python/
├── app.py                   # FastAPI application, lifespan, router wiring
├── config.py                # Settings via environment variables (pydantic-settings)
├── schemas.py               # Pydantic v2 request / response models
├── validation.py            # Pure validation helpers (coverage, categories, etc.)
├── prompts/
│   ├── loader.py            # load_protocol_prompt() → ProtocolPrompt
│   ├── labeler_v1.txt       # Versioned labeler system prompt
│   └── answerer_v1.txt      # Versioned answerer system prompt
├── providers/
│   ├── base.py              # LLMProvider Protocol
│   ├── ollama_provider.py   # OllamaProvider — /api/chat, structured output
│   └── fake_provider.py     # FakeProvider for tests
├── routers/
│   ├── health.py            # GET /health
│   ├── label.py             # POST /label
│   ├── answer.py            # POST /answer
│   └── judge.py             # POST /judge
├── tests/
│   ├── test_label.py
│   ├── test_answer.py
│   ├── test_judge.py
│   ├── test_prompt_hash.py
│   └── test_validation.py
├── pyproject.toml
└── README.md
```

## Configuration

| Variable | Default | Description |
|---|---|---|
| `LLM_PROVIDER` | `ollama` | Provider name (only `ollama` supported) |
| `OLLAMA_BASE_URL` | `http://127.0.0.1:11434` | Ollama base URL |
| `OLLAMA_MODEL` | `qwen2.5-coder:7b` | Model name |
| `LLM_TEMPERATURE` | `0` | Sampling temperature (0 = deterministic) |
| `LLM_TIMEOUT_SECONDS` | `60` | Per-request timeout |
| `LABEL_MAX_CONCURRENCY` | `4` | Max parallel label LLM calls |
| `ANSWER_MAX_CONCURRENCY` | `4` | Max parallel answer LLM calls |
| `JUDGE_MAX_CONCURRENCY` | `4` | Max parallel judge LLM calls |

Set `OLLAMA_NUM_PARALLEL` on the Ollama process to match the highest
`MAX_CONCURRENCY` value for full parallelism benefit.

The Ollama `/api/chat` endpoint is used (not `/api/generate`) so that the
shared system prompt prefix is eligible for KV cache reuse across concurrent
per-transaction calls.

## Provider Interface

```python
class LLMProvider(Protocol):
    async def structured_chat(
        self,
        system_prompt: str,
        user_payload: dict,
        response_schema: type[BaseModel],
        timeout_seconds: float,
    ) -> BaseModel: ...

    async def raw_chat(
        self,
        system_prompt: str,
        user_message: str,
        timeout_seconds: float,
    ) -> str: ...
```

`structured_chat` is used by `/label` and `/answer` (structured Pydantic output).
`raw_chat` is used by `/judge` (Go owns the schema; Python returns the raw string).

## Protocol Prompt Handling

At startup, `load_protocol_prompt(name)` reads the prompt file, computes its
SHA-256 hash, and returns a `ProtocolPrompt`:

```python
@dataclass
class ProtocolPrompt:
    name: str
    version: str       # e.g. "labeler_v1"
    content: str
    sha256_hash: str   # hex-encoded, computed once at startup
```

Prompt files are never mutated at runtime. Every `/label` and `/answer` response
includes `prompt_version` and `prompt_hash` so the Go client can verify
the service is running the expected prompt version.

The `/judge` endpoint does not return a prompt version — the judge prompt
is owned and versioned by the Go side.

## Error Model

All errors return structured JSON. Stack traces are never exposed.

| Code | Meaning |
|---|---|
| `INVALID_REQUEST` | Malformed or missing request fields |
| `PROMPT_VERSION_MISMATCH` | Request `prompt_version` does not match loaded version |
| `PROVIDER_ERROR` | Ollama returned a non-200 or unexpected response |
| `PROVIDER_TIMEOUT` | Ollama did not respond within `LLM_TIMEOUT_SECONDS` |
| `INVALID_MODEL_OUTPUT` | Model output failed JSON parse or schema validation |
| `COVERAGE_MISMATCH` | Response tx_hash set does not match request tx_hash set |
| `UNKNOWN_CATEGORY` | Judge response contains a category outside the allowed set |
| `UNKNOWN_SUBDOMAIN` | Label response contains a subdomain not in `allowed_subdomains` |
| `EMPTY_ANSWER` | Answer or reason field is empty after trimming |

Partial results are never returned. If any per-transaction LLM call fails,
the whole endpoint returns an error.

## Concurrency Model

Each endpoint processes transactions concurrently up to its configured
`MAX_CONCURRENCY` limit using `asyncio.Semaphore`. Output order is
always the same as input order, regardless of completion order.

```
request: [tx_A, tx_B, tx_C, tx_D]  (MAX_CONCURRENCY=2)

  slot 1: tx_A ──────────► done
  slot 2: tx_B ──► done
                   slot 2: tx_C ──────► done
                                         slot 1: tx_D ──► done

response: [tx_A, tx_B, tx_C, tx_D]  (always input order)
```

## How Ollama Works

Ollama is a local model runtime that exposes a REST API (`http://127.0.0.1:11434`
by default). It is a separate process that owns the model weights and the GPU/CPU
context — the Python service is just an HTTP client to it.

```
Python service  →  POST /api/chat  →  Ollama server  →  model weights (local disk/RAM)
```

The only time Ollama contacts the internet is during `ollama pull`, which downloads
model weights from Ollama's registry to your local disk (similar to `docker pull`).
After that the server runs fully offline — no external calls at runtime.

When the first request arrives Ollama loads the model into memory and keeps it warm
between requests. This is why it runs as a server rather than being embedded directly
in the Python process: a 7B model takes several gigabytes of VRAM and a few seconds
to load. A shared server loads it once and serves any number of clients, instead of
each process loading it independently and exhausting memory.

In production each physical validator node runs its own Ollama instance alongside its
own Python service. Validators call their local Ollama independently, which is the
whole point: each validator generates labels, answers, and classification votes without
sharing a model or inference state with any other validator.

## Running Locally

```bash
# Pull the model
ollama pull qwen2.5-coder:7b

# Install dependencies
cd agent-python
python -m venv .venv
source .venv/bin/activate
pip install -e ".[dev]"

# Start the service
uvicorn app:app --host 127.0.0.1 --port 8081

# Health check
curl http://127.0.0.1:8081/health
```

## Running Tests

Tests use `FakeProvider` and do not require a running Ollama instance.

```bash
pytest tests/ -v
```

## Example Requests

```bash
# Label
curl -X POST http://127.0.0.1:8081/label \
  -H 'Content-Type: application/json' \
  -d '{
    "prompt_version": "labeler_v1",
    "allowed_subdomains": ["databases", "back_end_with_apis", "security"],
    "transactions": [
      {"tx_hash": "0xabc", "prompt": "Design a rate-limited API with a PostgreSQL backend."}
    ]
  }'

# Answer
curl -X POST http://127.0.0.1:8081/answer \
  -H 'Content-Type: application/json' \
  -d '{
    "prompt_version": "answerer_v1",
    "transactions": [
      {
        "tx_hash": "0xabc",
        "prompt": "Design a rate-limited API with a PostgreSQL backend.",
        "subdomains": ["databases", "back_end_with_apis"]
      }
    ]
  }'

# Judge (Go builds both prompt strings before calling)
curl -X POST http://127.0.0.1:8081/judge \
  -H 'Content-Type: application/json' \
  -d '{
    "system_prompt": "<versioned judge system prompt built by Go>",
    "user_prompt": "<anonymized candidates payload built by Go>"
  }'
```

## Go Integration

The Go side integrates through two new components:

### 1. `BatchAgent` interface (`agent/interface.go`)

```go
type BatchAgent interface {
    AnswersJudge  // JudgeTransactionAnswers(AnswerJudgeRequest) (string, error)
    LabelBatch(txs []data.Transaction) ([]LabelResult, error)
    AnswerBatch(txs []data.Transaction) ([]AnswerResult, error)
}

type LabelResult struct {
    TxHash []byte
    Labels []string
}

type AnswerResult struct {
    TxHash []byte
    Answer string
}
```

`bodyExecutor.go` and the block processor use `BatchAgent` instead of
calling `Label` / `Answer` per transaction in a loop.

### 2. HTTP client (`agent/httpclient/`)

Implements `BatchAgent`. Configuration:

```go
type Config struct {
    BaseURL        string
    TimeoutSeconds int
}
```

- `LabelBatch` → `POST /label` with all transactions; parses label list per tx
- `AnswerBatch` → `POST /answer` with all transactions; parses answer string per tx
- `JudgeTransactionAnswers` → `POST /judge` with Go-built prompt strings; returns raw response string for Go's existing `classification.ExecuteRequests` to parse

The client verifies that the returned `prompt_version` and `prompt_hash`
match the locally known values before accepting a `/label` or `/answer` response.

---

## Incremental PR Plan

Each PR compiles independently and does not break existing tests.

### PR 1 — Python project scaffold

Scope:
- `pyproject.toml` with FastAPI, Pydantic v2, httpx, pytest, uvicorn
- `config.py`: all env var settings via `pydantic-settings`
- `app.py`: FastAPI app skeleton with lifespan hook and router registration
- `GET /health`: returns provider name, model, and service reachable flag
- Structured error model and exception handler (no stack traces in responses)
- `providers/base.py`: `LLMProvider` Protocol with `structured_chat` and `raw_chat`
- `providers/fake_provider.py`: configurable fake for tests

Tests:
- Health endpoint returns 200 with expected fields
- Unknown route returns structured error, not FastAPI default

### PR 2 — Ollama provider

Scope:
- `providers/ollama_provider.py`: calls `/api/chat` with `format: "json"` and
  `temperature: 0`; raises typed exceptions for timeout, invalid JSON,
  schema mismatch, and provider error
- Wire `OllamaProvider` into the app lifespan via `LLM_PROVIDER` config
- Health endpoint updated to reflect Ollama reachability (live ping)

Tests (all using `FakeProvider` or `httpx` mock):
- Successful structured response parsed and validated
- Invalid JSON raises `INVALID_MODEL_OUTPUT`
- Timeout raises `PROVIDER_TIMEOUT`
- Non-200 from Ollama raises `PROVIDER_ERROR`
- Schema mismatch raises `INVALID_MODEL_OUTPUT`

### PR 3 — Protocol prompt loading

Scope:
- `prompts/loader.py`: `load_protocol_prompt(name)` reads file bytes, computes
  SHA-256, returns `ProtocolPrompt`; called once at startup, result cached
- `prompts/labeler_v1.txt`: first version of the labeler system prompt
- `prompts/answerer_v1.txt`: first version of the answerer system prompt
- Health endpoint updated to include `prompt_versions` and `prompt_hashes`

Tests:
- SHA-256 hash is stable across repeated calls for the same file
- Hash changes if file content changes
- Missing prompt file raises at startup, not at request time
- Health response includes both prompt versions and hashes

### PR 4 — `/label` endpoint

Scope:
- `schemas.py`: `LabelRequest`, `LabelResponse`, `LabelResult` models
- `validation.py`: subdomain membership check, tx_hash coverage check,
  confidence range check
- `routers/label.py`: bounded-concurrency batch dispatch, canonical output order
- `POST /label` full implementation

Tests:
- Valid response with multiple transactions
- Subdomain outside `allowed_subdomains` → `UNKNOWN_SUBDOMAIN`
- Missing tx_hash in response → `COVERAGE_MISMATCH`
- Extra tx_hash in response → `COVERAGE_MISMATCH`
- Confidence outside `[0, 1]` → `INVALID_MODEL_OUTPUT`
- `prompt_version` mismatch in request → `PROMPT_VERSION_MISMATCH`
- Output order matches input order regardless of completion order
- One failed per-transaction call causes whole endpoint to fail

### PR 5 — `/answer` endpoint

Scope:
- `schemas.py`: `AnswerRequest`, `AnswerResponse`, `AnswerResult` models
- `routers/answer.py`: bounded-concurrency batch dispatch, canonical output order
- `POST /answer` full implementation

Tests:
- Valid response with multiple transactions
- Empty answer after trimming → `EMPTY_ANSWER`
- Missing tx_hash → `COVERAGE_MISMATCH`
- Extra tx_hash → `COVERAGE_MISMATCH`
- Output order matches input order
- One failed per-transaction call causes whole endpoint to fail

### PR 6 — `/judge` endpoint

Scope:
- `schemas.py`: `JudgeRequest` (`system_prompt`, `user_prompt`), `JudgeResponse`
  (`response` raw string)
- `validation.py`: parse response JSON; validate every `category` is in
  `{Correct, Hallucination, Malicious, Wrong}`; validate every `reason`
  is non-empty after trimming
- `routers/judge.py`: single call (Go batches candidates into the prompt strings)

Tests:
- Valid response passes through as raw string
- Invalid JSON from model → `INVALID_MODEL_OUTPUT`
- Category outside allowed set → `UNKNOWN_CATEGORY`
- Empty reason → `EMPTY_ANSWER`
- `system_prompt` or `user_prompt` empty → `INVALID_REQUEST`

### PR 7 — Go `BatchAgent` interface

Scope:
- Add `LabelResult`, `AnswerResult`, and `BatchAgent` to `agent/interface.go`
- Update `blockprocessing/bodyExecutor.go` to accept `BatchAgent` and call
  `LabelBatch` / `AnswerBatch` instead of the per-transaction loop
- Update `testscommon/LabelerStub.go` to implement `BatchAgent`
- Keep `Agent` interface unchanged; existing stubs still compile

Tests:
- Existing unit and integration tests pass without modification

### PR 8 — Go HTTP client (`agent/httpclient/`)

Scope:
- `agent/httpclient/client.go`: `Config` struct, `New(Config)`, implements `BatchAgent`
- `LabelBatch`: `POST /label`, parses label list per tx, verifies returned
  `prompt_version` and `prompt_hash` against locally known values
- `AnswerBatch`: `POST /answer`, parses answer string per tx, same version check
- `JudgeTransactionAnswers`: `POST /judge` with `{system_prompt, user_prompt}`,
  returns raw response string for Go's existing parsing
- Typed error mapping from Python error codes to Go errors
- `agent/httpclient/client_test.go`: uses `httptest.NewServer` to mock
  the Python service

Tests:
- `LabelBatch` success and per-field mapping
- `AnswerBatch` success and per-field mapping
- `JudgeTransactionAnswers` success returns raw string unchanged
- Prompt version mismatch → typed Go error
- HTTP timeout → typed Go error
- Non-200 → typed Go error
- Coverage mismatch in response → typed Go error

### PR 9 — Wiring and configuration

Scope:
- `cmd/config.go`: add `AgentBaseURL` and `AgentTimeoutSeconds` fields
- `cmd/node/main.go`: construct `httpclient.New(cfg)` and pass it as the
  `BatchAgent` to the block executor and as `AnswerJudge` to
  `MiniRoundTwoHandlerArgs`
- Document the env vars / config flags in this README

Tests:
- Existing integration tests still pass (stubs remain the default in test builds)

### PR 10 — MR1 integration test with real HTTP agent

These tests require a running Python service and Ollama instance. They are
tagged `//go:build integration` and are never run in normal CI.

Required preconditions:

```bash
ollama pull qwen2.5-coder:7b
cd agent-python && uvicorn app:app --host 127.0.0.1 --port 8081
```

Run with:

```bash
go test -tags integration ./integrationtests/...
```

At test start, ping `GET /health` on the configured agent URL. If unreachable,
skip with a clear message — do not fail.

Scope:
- `integrationtests/fullflow_mr1_test.go`: N validator goroutines, each wired
  to `httpclient.New(cfg)` pointing at the real Python service; runs MR1 to
  completion
- No stubs — the real `/label` endpoint is called with real transactions
- All validators share one Python service instance (benchmarking topology is
  a separate concern; see Deployment Model note above)

Assertions:
- All validators finalized the same `BlockOnChain` (identical block hash)
- `SubdomainsFrequencies` is non-empty and all keys are valid protocol subdomains
- No content assertions on label values (LLM output is non-deterministic)

### PR 11 — Full MR1 → MR2 integration test with real HTTP agent

Same preconditions and skip-if-unreachable pattern as PR 10.

Scope:
- `integrationtests/fullflow_mr2_test.go`: N validator goroutines wired to
  the real Python service; runs MR1 → MR2 to completion
- Real `/label`, `/answer`, and `/judge` endpoints called with real transactions
  and real candidate answers

Assertions:
- All validators finalized the same `BlockOnChain` (identical block hash)
- `AggregatedExecutionResults` has one entry per transaction
- `AnswerClassifications` has one entry per transaction with a valid `Status`
  (`READY_FOR_MINI_ROUND_THREE` or `INSUFFICIENT_CORRECT_ANSWERS`)
- `AnswerEvidence` is non-nil, signer count ≥ quorum
- No content assertions on answers or classifications (LLM output is
  non-deterministic)

### PR 12 — Python service benchmarks

> **Prerequisite for protocol timeout implementation.** Do not implement
> protocol-level timeouts until this data exists.

Benchmark the Python service in isolation — independent of Go and the consensus
protocol. The goal is latency data per endpoint across realistic inputs so that
timeout values in the Go protocol can be derived from evidence rather than
guesswork. A timeout set too tight breaks liveness under normal load; a timeout
set too loose makes the protocol unable to make progress in fault scenarios.

Scope:
- Benchmark script (e.g. `agent-python/benchmarks/bench.py`) using `httpx`
  or `locust`, targeting a locally running Python service + Ollama
- Measure p50, p95, p99 latency per endpoint: `/label`, `/answer`, `/judge`
- Vary batch size (number of transactions per request) and candidate count
  (for `/judge`)
- Vary `MAX_CONCURRENCY` and `OLLAMA_NUM_PARALLEL` to find the concurrency
  ceiling for the target hardware
- Record results in `agent-python/benchmarks/results/` as structured output
  (JSON or CSV) for reproducibility

What to capture per run:
- Hardware specs and Ollama model + quantization
- Batch size and concurrency settings
- p50 / p95 / p99 / max latency per endpoint
- Throughput (requests per second)

### PR 13 — Protocol timeout implementation

> **Depends on PR 12.** Timeout values must be derived from benchmark results,
> not invented.

The round handler already has timeout stubs but no real timer wiring. This PR
activates them using values informed by the benchmark data from PR 12.

Scope:
- Wire real timers into the round handler for each protocol step that now
  depends on agent calls: labeling (MR1), answer collection (MR2), judge
  collection (MR2)
- Define timeout values as named constants derived from benchmark p99 +
  safety margin; document the derivation
- Define explicit protocol behavior on timeout per step: which steps can
  proceed with a partial result, which must fail the round
- Add `AgentLabelTimeoutSeconds`, `AgentAnswerTimeoutSeconds`,
  `AgentJudgeTimeoutSeconds` to `cmd/config.go` so values can be overridden
  per deployment without recompilation
- Update existing MR1 and MR2 integration tests to cover timeout-triggered
  step transitions

Tests:
- Round proceeds correctly when agent responds within timeout
- Round handles timeout-triggered step transition without panic or deadlock
- Configured timeout values are respected by the HTTP client
