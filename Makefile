SHELL := /bin/bash

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
LLM_TIMEOUT_SECONDS   ?= 600
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

# MOA_REPEATED_RUNS controls how many rounds to run (default: 5 set in test code).
# Each run appends to testData/miniround1/results/repeated_runs_results.jsonl.
.PHONY: test-realagent-mr1-repeated-a
test-realagent-mr1-repeated-a: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	MOA_REPEATED_RUNS=$(MOA_REPEATED_RUNS) \
	go test -tags integration -timeout 120m -v \
		-run TestMiniRoundOne_RealAgent_RepeatedRuns_GroupA_NonCodingConvergeToNonRelated \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr1-repeated-b
test-realagent-mr1-repeated-b: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	MOA_REPEATED_RUNS=$(MOA_REPEATED_RUNS) \
	go test -tags integration -timeout 120m -v \
		-run TestMiniRoundOne_RealAgent_RepeatedRuns_GroupB_ClearDomainConvergeToExpectedLabels \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

# k=3 corrupted validators (= f, the BFT bound): consensus must always succeed.
.PHONY: test-realagent-mr1-byzantine-k3
test-realagent-mr1-byzantine-k3: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	MOA_REPEATED_RUNS=$(MOA_REPEATED_RUNS) \
	go test -tags integration -timeout 120m -v \
		-run TestMiniRoundOne_RealAgent_Byzantine_K3_CorrectLabelSurvives \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

# k=6 corrupted validators (= 2f): protocol degrades, some rounds fail to finalize.
.PHONY: test-realagent-mr1-byzantine-k6
test-realagent-mr1-byzantine-k6: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	MOA_REPEATED_RUNS=$(MOA_REPEATED_RUNS) \
	go test -tags integration -timeout 120m -v \
		-run TestMiniRoundOne_RealAgent_Byzantine_K6_ProtocolDegrades \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

# Labeling latency benchmark — calibrates maxBlockConsumption and maxNumTransactions.
# Experiment A: fixed short prompt, vary tx count (1, 2, 4, 8, 16).
# Experiment B: fixed 4 txs, vary prompt length (short, medium, long, very_long).
# Results are appended to integrationtests/testData/miniround1/results/benchmark_labeling_results.jsonl.
.PHONY: test-realagent-mr1-benchmark
test-realagent-mr1-benchmark: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 60m -v \
		-run TestMiniRoundOne_LabelingLatencyBenchmark \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr2-benchmark
test-realagent-mr2-benchmark: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 60m -v \
		-run TestMiniRoundTwo_JudgingLatencyBenchmark \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr2-group-a
test-realagent-mr2-group-a: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 60m -v \
		-run TestMiniRoundTwo_RealAgent_GroupA_AllCorrectAnswersConverge \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr2-group-b
test-realagent-mr2-group-b: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 60m -v \
		-run TestMiniRoundTwo_RealAgent_GroupB_WrongAnswerIsRejected \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr2-group-c
test-realagent-mr2-group-c: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 60m -v \
		-run TestMiniRoundTwo_RealAgent_GroupC_PromptInjectionIsRejected \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr2-group-d
test-realagent-mr2-group-d: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 60m -v \
		-run TestMiniRoundTwo_RealAgent_GroupD_HallucinationIsRejected \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr2-group-e
test-realagent-mr2-group-e: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 60m -v \
		-run TestMiniRoundTwo_RealAgent_GroupE_CrossDomainAnswerDetection \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr2-group-f
test-realagent-mr2-group-f: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 60m -v \
		-run TestMiniRoundTwo_RealAgent_GroupF_SubtleByzantineErrorIsRejected \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr2
test-realagent-mr2: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 180m -v \
		-run 'TestMiniRoundTwo_RealAgent_Group' \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr2-diverse-group-a
test-realagent-mr2-diverse-group-a: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 60m -v \
		-run TestMiniRoundTwo_RealAgent_Diverse_GroupA_AllCorrectAnswersConverge \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr2-diverse-group-b
test-realagent-mr2-diverse-group-b: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 60m -v \
		-run TestMiniRoundTwo_RealAgent_Diverse_GroupB_WrongAnswerIsRejected \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr2-diverse-group-c
test-realagent-mr2-diverse-group-c: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 60m -v \
		-run TestMiniRoundTwo_RealAgent_Diverse_GroupC_PromptInjectionResistance \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr2-diverse-group-d
test-realagent-mr2-diverse-group-d: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 60m -v \
		-run TestMiniRoundTwo_RealAgent_Diverse_GroupD_HallucinationIsRejected \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr2-diverse-group-e
test-realagent-mr2-diverse-group-e: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 60m -v \
		-run TestMiniRoundTwo_RealAgent_Diverse_GroupE_CrossDomainAnswerDetection \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr2-diverse-group-f
test-realagent-mr2-diverse-group-f: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 60m -v \
		-run TestMiniRoundTwo_RealAgent_Diverse_GroupF_SubtleByzantineErrorIsRejected \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

.PHONY: test-realagent-mr2-diverse
test-realagent-mr2-diverse: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 180m -v \
		-run 'TestMiniRoundTwo_RealAgent_Diverse_Group' \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

# Ablation experiment 1: single-candidate judging
# Submits each diverse correct-answer perspective as the sole candidate in an
# independent /judge call, to determine whether canonical-preference bias is
# context-driven (comparison with other candidates) or parametric (model's
# internal knowledge prefers one phrasing regardless of context).
# 3 txs × 7 candidates × 3 runs = 63 judge calls. Expected runtime: ~5 min.
.PHONY: test-realagent-mr2-ablation1
test-realagent-mr2-ablation1: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 30m -v \
		-run TestMiniRoundTwo_Ablation1_SingleCandidateJudging \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

# Ablation experiment 2: single-candidate judging with one bad answer (Group B)
# Proves that single-candidate mode retains correct discriminative behaviour —
# it does not simply approve everything; it still rejects genuinely wrong answers.
# 3 txs × 7 candidates (6 correct + 1 bad) × 3 runs = 63 judge calls. ~5 min.
.PHONY: test-realagent-mr2-ablation2
test-realagent-mr2-ablation2: _start-agent
	@MOA_AGENT_BASE_URL=$(MOA_AGENT_BASE_URL) \
	go test -tags integration -timeout 30m -v \
		-run TestMiniRoundTwo_Ablation2_SingleCandidateJudging_GroupB \
		./integrationtests/... ; \
	EXIT=$$?; $(MAKE) _stop-agent; exit $$EXIT

# ── Distributed cluster integration tests ─────────────────────────────────────
#
# Ensures all cluster agents are healthy (starting them if needed), runs the
# distributed MR1 test, then stops all agents — regardless of test outcome.
#
# Override cluster config path:
#   make test-distributed-mr1 CLUSTER_CONFIG=configs/cluster.json
#
# ─────────────────────────────────────────────────────────────────────────────

CLUSTER_CONFIG ?= $(CURDIR)/configs/cluster.json
N              ?= 5

.PHONY: test-distributed-mr1
test-distributed-mr1:
	@mkdir -p testresults
	@bash scripts/health-check.sh || bash scripts/start-cluster.sh
	@set -o pipefail; \
	MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
	MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
	go test -tags integration -timeout 30m -v \
		-run TestDistributedMR1 \
		./integrationtests/... \
	2>&1 | tee testresults/distributed-mr1-$$(date +%Y%m%dT%H%M%S).log; \
	exit_code=$$?; \
	bash scripts/stop-cluster.sh; \
	exit $$exit_code

.PHONY: test-distributed-mr1-trials
test-distributed-mr1-trials:
	@mkdir -p testresults
	@echo "Running $(N) MR1 trials — cluster restarted between each run..."
	@START=$$(date +%s); \
	for i in $$(seq 1 $(N)); do \
		echo ""; \
		echo "=== Trial $$i / $(N) ==="; \
		bash scripts/stop-cluster.sh 2>/dev/null || true; \
		bash scripts/start-cluster.sh; \
		MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
		MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
		go test -tags integration -timeout 30m \
			-run TestDistributedMR1 \
			./integrationtests/... || true; \
	done; \
	bash scripts/stop-cluster.sh 2>/dev/null || true; \
	echo ""; \
	echo "=== Trial Summary (N=$(N)) ==="; \
	python3 -c " \
import json,glob,os,sys; \
start=$$START; \
files=sorted(f for f in glob.glob('$(CURDIR)/testresults/distributed-mr1-*.json') if os.path.getmtime(f)>=start); \
results=[json.load(open(f)) for f in files]; \
results or sys.exit(print('No results found.') or 0); \
p=[r for r in results if r['passed']]; \
d=[r['duration_seconds'] for r in results]; \
print(f'Trials  : {len(results)}'); \
print(f'Passed  : {len(p)}/{len(results)}'); \
print(f'Duration: min={min(d):.1f}s  avg={sum(d)/len(d):.1f}s  max={max(d):.1f}s'); \
ms={}; \
[ms.update({k:ms.get(k,0)+1}) for r in p for k in [json.dumps(r.get('subdomains_frequencies',{}),sort_keys=True)]]; \
print(f'Subdomain maps: {len(ms)} unique across {len(p)} passed trial(s)'); \
[print(f'  ({c}x) {m}') for m,c in ms.items()] \
"

