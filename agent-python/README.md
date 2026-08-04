# Agent Python Service

FastAPI service used by MoA Chain validators for subdomain labeling, answer
generation, and answer classification. The Go protocol communicates with it
through `agent/httpclient`.

## Endpoints

- `GET /health` reports service and provider reachability.
- `POST /label` assigns allowed technical subdomains or `non_related` to a
  transaction batch.
- `POST /answer` generates one answer for each transaction.
- `POST /judge` classifies candidate answers as `correct`, `wrong`,
  `hallucination`, or `malicious`.

Requests carry a prompt version. The service loads the corresponding immutable
template from `prompts/`, computes its SHA-256 hash, and returns the hash so the
Go caller can verify that the intended protocol prompt was used.

## Layout

- `app.py`: FastAPI application assembly
- `config.py`: environment-backed settings
- `routers/`: HTTP handlers
- `providers/`: provider interface, fake provider, and Ollama implementation
- `prompts/`: versioned protocol prompts and loader
- `schemas.py`: request and response models
- `validation.py`: response validation and normalization
- `tests/`: endpoint, provider, validation, and prompt-hash tests

## Configuration

Common environment variables:

- `OLLAMA_BASE_URL` — Ollama server URL
- `OLLAMA_MODEL` — model name used for inference
- `LLM_TIMEOUT_SECONDS` — provider request timeout
- `LABEL_MAX_CONCURRENCY` — maximum concurrent labeling work

See `config.py` for defaults and the complete set.

## Local setup

```sh
ollama pull qwen2.5-coder:7b
make install
OLLAMA_MODEL=qwen2.5-coder:7b .venv/bin/uvicorn app:app \
  --host 127.0.0.1 --port 8081
```

Check the service:

```sh
curl http://127.0.0.1:8081/health
```

The repository-root Makefile can start Ollama and this service automatically
for real-agent integration-test targets.

## Tests

```sh
make test
```

Tests use the fake provider unless a test explicitly targets Ollama behavior.
They cover endpoint schemas, error mapping, prompt hashes, concurrency limits,
response coverage, and category validation.

## Protocol boundary

LLM output is nondeterministic and is never accepted directly as shared chain
state. MR1 treats labels as signed validator evidence. MR2 treats judge results
as signed classification votes and derives its finalized artifact with
deterministic Go aggregation and certificate verification.

Answer generation occurs off-round. Producers sign their answers before MR2;
the timed round verifies that evidence and reaches consensus over classification
votes. This prevents variable model latency from defining the consensus timeout.

The service should therefore remain stateless with respect to consensus. It
executes versioned prompts and validates model responses, while membership,
signatures, quorum rules, aggregation, and finalization remain in Go.
