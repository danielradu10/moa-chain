# Agent Python Service

FastAPI service used by MoA Chain validators for subdomain labeling, answer
generation, and answer classification. The Go protocol communicates with it
through `agent/httpclient`.

## Endpoints

- `GET /health` — reports service status, provider name, model, reachability, and prompt versions.
- `POST /label` — assigns allowed technical subdomains or `non_related` to a transaction batch.
- `POST /answer` — generates one answer per transaction.
- `POST /judge` — classifies candidate answers as `correct`, `wrong`, `hallucination`, or `malicious`.

Requests carry a prompt version. The service loads the corresponding immutable
template from `prompts/`, computes its SHA-256 hash, and returns the hash so the
Go caller can verify that the intended protocol prompt was used.

## Layout

```
app.py                   FastAPI application assembly and lifespan
config.py                Environment-backed settings (pydantic-settings)
schemas.py               Request and response Pydantic models
validation.py            Response validation and normalization helpers
errors.py                Error codes and AgentServiceError
routers/
  health.py              GET /health
  label.py               POST /label
  answer.py              POST /answer
  judge.py               POST /judge
providers/
  base.py                LLMProvider protocol (structural interface)
  factory.py             Selects and instantiates the configured provider
  ollama_provider.py     Ollama backend
  openai_provider.py     OpenAI backend
  deepseek_provider.py   DeepSeek backend (OpenAI-compatible transport)
  fake_provider.py       In-process stub used by tests
prompts/                 Versioned protocol prompt files and loader
tests/                   Endpoint, provider, factory, validation, prompt-hash tests
```

## Provider selection

Set `LLM_PROVIDER` to choose the backend. The factory validates all required
configuration at startup and aborts immediately with a clear error if anything is
missing — no request will fail silently due to bad config.

### Ollama (default)

Requires a locally running [Ollama](https://ollama.com) server with the target
model already pulled.

**Required env vars:**

| Variable | Default | Description |
|---|---|---|
| `LLM_PROVIDER` | `ollama` | Select the Ollama backend (or omit) |
| `OLLAMA_BASE_URL` | `http://127.0.0.1:11434` | Ollama server address |
| `OLLAMA_MODEL` | `qwen2.5-coder:7b` | Model name; must be pulled first |
| `LLM_TEMPERATURE` | `0.5` | Sampling temperature |
| `LLM_TIMEOUT_SECONDS` | `60.0` | Per-request timeout |
| `LLM_NUM_CTX` | `4096` | Context window size |
| `LLM_NUM_PREDICT` | `256` | Max output tokens |
| `LLM_THINK` | `false` | Enable Ollama thinking mode |
| `LABEL_MAX_CONCURRENCY` | `4` | Max concurrent label calls |
| `ANSWER_MAX_CONCURRENCY` | `4` | Max concurrent answer calls |
| `JUDGE_MAX_CONCURRENCY` | `4` | Max concurrent judge calls |

**Start:**

```sh
ollama pull qwen2.5-coder:7b
make install
OLLAMA_MODEL=qwen2.5-coder:7b .venv/bin/uvicorn app:app --host 127.0.0.1 --port 8081
```

### OpenAI

`OPENAI_API_KEY` must be set — startup fails immediately if it is missing.

**Required env vars:**

| Variable | Default | Description |
|---|---|---|
| `LLM_PROVIDER` | — | Must be set to `openai` |
| `OPENAI_API_KEY` | — | **Required.** Your OpenAI API key |
| `OPENAI_MODEL` | `gpt-4o-mini` | Any OpenAI chat model (`gpt-4o`, `o3-mini`, …) |
| `LLM_TEMPERATURE` | `0.5` | Sampling temperature |
| `LLM_TIMEOUT_SECONDS` | `60.0` | Per-request timeout |
| `LABEL_MAX_CONCURRENCY` | `4` | Max concurrent label calls |
| `ANSWER_MAX_CONCURRENCY` | `4` | Max concurrent answer calls |
| `JUDGE_MAX_CONCURRENCY` | `4` | Max concurrent judge calls |

Note: `LLM_NUM_CTX`, `LLM_NUM_PREDICT`, and `LLM_THINK` are Ollama-specific
and are ignored when `LLM_PROVIDER=openai`.

**Start:**

```sh
make install
LLM_PROVIDER=openai OPENAI_API_KEY=sk-... .venv/bin/uvicorn app:app --host 127.0.0.1 --port 8081
```

### DeepSeek

DeepSeek uses its official OpenAI-compatible Chat Completions endpoint. The
existing `openai` package is reused; no additional SDK is required.

| Variable | Default | Description |
|---|---|---|
| `LLM_PROVIDER` | — | Must be set to `deepseek` |
| `DEEPSEEK_API_KEY` | — | **Required.** Your DeepSeek API key |
| `DEEPSEEK_MODEL` | `deepseek-v4-flash` | DeepSeek chat model |
| `DEEPSEEK_BASE_URL` | `https://api.deepseek.com` | Official API endpoint |
| `LLM_TEMPERATURE` | `0.5` | Sampling temperature |
| `LLM_TIMEOUT_SECONDS` | `60.0` | Per-request timeout |

**Start:**

```sh
make install
LLM_PROVIDER=deepseek DEEPSEEK_API_KEY=... DEEPSEEK_MODEL=deepseek-v4-flash \
  .venv/bin/uvicorn app:app --host 127.0.0.1 --port 8081
```

**Qualification:**

```sh
LLM_PROVIDER=deepseek DEEPSEEK_API_KEY=... DEEPSEEK_MODEL=deepseek-v4-flash \
  QUALIFICATION_REPETITIONS=3 make qualify-agent
```

Or copy `.env.example` to `.env`, fill in your values, then just run:

```sh
.venv/bin/uvicorn app:app --host 127.0.0.1 --port 8081
```

**Check the service (works for both providers):**

```sh
curl http://127.0.0.1:8081/health
```

## Switching providers at runtime

The provider is instantiated once at startup. To switch, stop the service,
update `LLM_PROVIDER` (and any provider-specific variables), and restart.

## Instrumentation

Every LLM call emits a structured `llm_call` log line immediately after the
response is received:

```
llm_call provider=ollama model=qwen2.5-coder:7b operation=label latency_ms=1243 input_tokens=312 output_tokens=48 total_tokens=360
llm_call provider=openai model=gpt-4o-mini operation=judge latency_ms=876 input_tokens=450 output_tokens=91 total_tokens=541
llm_call provider=deepseek model=deepseek-v4-flash operation=answer latency_ms=932 input_tokens=310 output_tokens=144 total_tokens=454
```

| Field | Description |
|---|---|
| `provider` | Configured provider (`ollama`, `openai`, `anthropic`, `gemini`, or `deepseek`) |
| `model` | Model name as configured |
| `operation` | `label`, `answer`, or `judge` |
| `latency_ms` | Wall-clock time from request sent to response parsed |
| `input_tokens` | Prompt token count (`None` if not returned by the provider) |
| `output_tokens` | Completion token count |
| `total_tokens` | Sum of input and output |

Ollama returns token counts in non-streaming responses via `prompt_eval_count`
and `eval_count`. OpenAI returns them via `usage.prompt_tokens` and
`usage.completion_tokens`. Both are extracted and logged in the same format.

## Tests

```sh
make test
```

Tests use `FakeProvider` unless a test explicitly targets provider behavior.
They cover endpoint schemas, error mapping, prompt hashes, concurrency ordering,
response validation, provider factory selection, and per-provider response parsing.

## Protocol boundary

LLM output is nondeterministic and is never accepted directly as shared chain
state. MR1 treats labels as signed validator evidence. MR2 treats judge results
as signed classification votes and derives its finalized artifact through
deterministic Go aggregation and certificate verification.

Answer generation occurs off-round. Producers sign their answers before MR2;
the timed round verifies the evidence and reaches consensus over classification
votes. This prevents variable model latency from defining the consensus timeout.

The service remains stateless with respect to consensus. It executes versioned
prompts and validates model responses, while membership, signatures, quorum
rules, aggregation, and finalization remain in Go.
