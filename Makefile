lint-install:
ifeq (,$(wildcard test -f bin/golangci-lint))
	@echo "Installing golint"
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s
endif

run-lint:
	@echo "Running golint"
	bin/golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 --timeout=2m

lint: lint-install run-lint

.PHONY: test
test:
	go test ./...

# ── Real-agent integration tests ──────────────────────────────────────────────
#
# Each target starts the Python agent service, waits for it to be healthy,
# runs the test, then stops the service — no manual setup required.
#
# Prerequisites (one-time):
#   ollama pull qwen2.5-coder:7b
#
# Override defaults:
#   make <target> AGENT_PORT=8082 OLLAMA_MODEL=llama3
#
# ─────────────────────────────────────────────────────────────────────────────

AGENT_PORT          ?= 8081
AGENT_HOST          ?= 127.0.0.1
MOA_AGENT_BASE_URL  ?= http://$(AGENT_HOST):$(AGENT_PORT)
OLLAMA_MODEL        ?= qwen2.5-coder:7b
OLLAMA_NUM_PARALLEL   ?= 7
LABEL_MAX_CONCURRENCY ?= 7
LLM_TIMEOUT_SECONDS   ?= 300
AGENT_STARTUP_WAIT    ?= 60

# Start uvicorn in the background, write its PID to .agent.pid, then poll
# /health until the service is ready or the timeout expires.
.PHONY: _start-ollama
_start-ollama:
	@if ! command -v ollama > /dev/null 2>&1; then \
		echo ""; \
		echo "Ollama is not installed. Install it with:"; \
		echo "  brew install ollama"; \
		echo "Then pull the model:"; \
		echo "  ollama pull $(OLLAMA_MODEL)"; \
		echo ""; \
		exit 1; \
	fi
	@if ! curl -sf http://127.0.0.1:11434 > /dev/null 2>&1; then \
		echo "Starting Ollama..."; \
		OLLAMA_NUM_PARALLEL=$(OLLAMA_NUM_PARALLEL) ollama serve > /tmp/moa-ollama.log 2>&1 & echo $$! > .ollama.pid; \
		seconds=0; \
		until curl -sf http://127.0.0.1:11434 > /dev/null 2>&1; do \
			seconds=$$((seconds + 1)); \
			if [ $$seconds -ge 30 ]; then \
				echo "Ollama did not start within 30s — logs:"; \
				cat /tmp/moa-ollama.log; \
				exit 1; \
			fi; \
			sleep 1; \
		done; \
		echo "Ollama is running."; \
	else \
		echo "Ollama is already running."; \
	fi

.PHONY: _stop-ollama
_stop-ollama:
	@if [ -f .ollama.pid ]; then \
		PID=$$(cat .ollama.pid); \
		echo "Stopping Ollama (PID $$PID)..."; \
		kill $$PID 2>/dev/null || true; \
		rm -f .ollama.pid; \
	fi

.PHONY: _start-agent
_start-agent: _start-ollama
	@echo "Killing any existing process on port $(AGENT_PORT)..."
	@lsof -ti tcp:$(AGENT_PORT) | xargs kill -9 2>/dev/null || true
	@sleep 1
	@echo "Starting agent service on $(MOA_AGENT_BASE_URL)..."
	@cd agent-python && \
		OLLAMA_MODEL=$(OLLAMA_MODEL) \
		LABEL_MAX_CONCURRENCY=$(LABEL_MAX_CONCURRENCY) \
		LLM_TIMEOUT_SECONDS=$(LLM_TIMEOUT_SECONDS) \
		.venv/bin/uvicorn app:app --host $(AGENT_HOST) --port $(AGENT_PORT) \
		> /tmp/moa-agent.log 2>&1 & echo $$! > ../.agent.pid
	@echo "Waiting for agent service and Ollama to become healthy (timeout $(AGENT_STARTUP_WAIT)s)..."
	@seconds=0; \
	until [ "$$(curl -sf $(MOA_AGENT_BASE_URL)/health | grep -o '"reachable":true')" = '"reachable":true' ]; do \
		seconds=$$((seconds + 1)); \
		if [ $$seconds -ge $(AGENT_STARTUP_WAIT) ]; then \
			echo ""; \
			echo "Timed out after $(AGENT_STARTUP_WAIT)s. Last health response:"; \
			curl -s $(MOA_AGENT_BASE_URL)/health 2>/dev/null || echo "(service not responding)"; \
			echo ""; \
			echo "Agent logs:"; \
			cat /tmp/moa-agent.log; \
			echo ""; \
			echo "Is Ollama running? Try: ollama serve"; \
			exit 1; \
		fi; \
		sleep 1; \
	done
	@echo "Agent service is healthy."
	@echo "Warming up model $(OLLAMA_MODEL) — loading into memory..."
	@curl -sf $(MOA_AGENT_BASE_URL)/label \
		-H 'Content-Type: application/json' \
		-d '{"prompt_version":"labeler_v2","allowed_subdomains":["non_related"],"transactions":[{"tx_hash":"warmup","prompt":"warmup"}]}' \
		> /dev/null
	@echo "Model is loaded and ready."

# Stop the background uvicorn process started by _start-agent.
.PHONY: _stop-agent
_stop-agent: _stop-ollama
	@if [ -f .agent.pid ]; then \
		PID=$$(cat .agent.pid); \
		echo "Stopping agent service (PID $$PID)..."; \
		kill $$PID 2>/dev/null || true; \
		rm -f .agent.pid; \
	fi

.PHONY: test-realagent-mr1-group-a
test-realagent-mr1-group-a: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 30m -v \
		-run TestMiniRoundOne_RealAgent_GroupA_NonCodingTransactionsConvergeToNonRelated \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr1-group-b
test-realagent-mr1-group-b: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 60m -v \
		-run TestMiniRoundOne_RealAgent_GroupB_ClearDomainTransactionsConvergeToExpectedLabels \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT
