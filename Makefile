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

# ── Local chain simulator ──────────────────────────────────────────────────────

LOCALCHAIN_NODES       ?= 10
LOCALCHAIN_START_ROUND ?= 2
LOCALCHAIN_ADDR        ?= :8080

.PHONY: localchain
localchain:
	go run ./cmd/localchain \
		--nodes $(LOCALCHAIN_NODES) \
		--start-round $(LOCALCHAIN_START_ROUND) \
		--addr $(LOCALCHAIN_ADDR)

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

.PHONY: test-distributed-mr1-byzantine-k3
test-distributed-mr1-byzantine-k3:
	@mkdir -p testresults
	@bash scripts/health-check.sh || bash scripts/start-cluster.sh
	@set -o pipefail; \
	MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
	MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
	go test -tags integration -timeout 30m -v \
		-run TestDistributedMR1_Byzantine_K3 \
		./integrationtests/... \
	2>&1 | tee testresults/distributed-mr1-byzantine-k3-$$(date +%Y%m%dT%H%M%S).log; \
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

.PHONY: test-distributed-mr1-byzantine-k3-trials
test-distributed-mr1-byzantine-k3-trials:
	@mkdir -p testresults
	@echo "Running $(N) Byzantine-k3 MR1 trials — cluster restarted between each run..."
	@START=$$(date +%s); \
	for i in $$(seq 1 $(N)); do \
		echo ""; \
		echo "=== Trial $$i / $(N) ==="; \
		bash scripts/stop-cluster.sh 2>/dev/null || true; \
		bash scripts/start-cluster.sh; \
		MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
		MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
		go test -tags integration -timeout 30m \
			-run TestDistributedMR1_Byzantine_K3 \
			./integrationtests/... || true; \
	done; \
	bash scripts/stop-cluster.sh 2>/dev/null || true; \
	echo ""; \
	echo "=== Byzantine-k3 Trial Summary (N=$(N)) ==="; \
	python3 -c " \
import json,glob,os,sys; \
start=$$START; \
files=sorted(f for f in glob.glob('$(CURDIR)/testresults/distributed-mr1-*.json') if os.path.getmtime(f)>=start and json.load(open(f)).get('num_byzantine',0)>0); \
results=[json.load(open(f)) for f in files]; \
results or sys.exit(print('No results found.') or 0); \
p=[r for r in results if r['passed']]; \
d=[r['duration_seconds'] for r in results]; \
print(f'Trials        : {len(results)}'); \
print(f'Passed        : {len(p)}/{len(results)}'); \
print(f'Byzantine     : {results[0].get(\"num_byzantine\")} validators returning \"{results[0].get(\"byzantine_label\")}\"'); \
print(f'Duration      : min={min(d):.1f}s  avg={sum(d)/len(d):.1f}s  max={max(d):.1f}s'); \
bl=results[0].get('byzantine_label',''); \
leaked=[r for r in p if bl in r.get('subdomains_frequencies',{})]; \
print(f'Byzantine label leaked into map: {len(leaked)}/{len(p)} trials (expected 0)'); \
ms={}; \
[ms.update({k:ms.get(k,0)+1}) for r in p for k in [json.dumps(r.get('subdomains_frequencies',{}),sort_keys=True)]]; \
print(f'Subdomain maps: {len(ms)} unique across {len(p)} passed trial(s)'); \
[print(f'  ({c}x) {m}') for m,c in ms.items()] \
"

# ── Test 3: Non-related transactions ──────────────────────────────────────────

.PHONY: test-distributed-mr1-non-related
test-distributed-mr1-non-related:
	@mkdir -p testresults
	@bash scripts/health-check.sh || bash scripts/start-cluster.sh
	@set -o pipefail; \
	MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
	MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
	go test -tags integration -timeout 30m -v \
		-run TestDistributedMR1_NonRelated \
		./integrationtests/... \
	2>&1 | tee testresults/distributed-mr1-non-related-$$(date +%Y%m%dT%H%M%S).log; \
	exit_code=$$?; \
	bash scripts/stop-cluster.sh; \
	exit $$exit_code

.PHONY: test-distributed-mr1-non-related-trials
test-distributed-mr1-non-related-trials:
	@mkdir -p testresults
	@echo "Running $(N) non-related MR1 trials — cluster restarted between each run..."
	@START=$$(date +%s); \
	for i in $$(seq 1 $(N)); do \
		echo ""; \
		echo "=== Trial $$i / $(N) ==="; \
		bash scripts/stop-cluster.sh 2>/dev/null || true; \
		bash scripts/start-cluster.sh; \
		MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
		MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
		go test -tags integration -timeout 30m \
			-run TestDistributedMR1_NonRelated \
			./integrationtests/... || true; \
	done; \
	bash scripts/stop-cluster.sh 2>/dev/null || true; \
	echo ""; \
	echo "=== Non-Related Trial Summary (N=$(N)) ==="; \
	python3 -c " \
import json,glob,os,sys; \
start=$$START; \
files=sorted(f for f in glob.glob('$(CURDIR)/testresults/distributed-mr1-non-related-*.json') if os.path.getmtime(f)>=start); \
results=[json.load(open(f)) for f in files]; \
results or sys.exit(print('No results found.') or 0); \
p=[r for r in results if r['passed']]; \
d=[r['duration_seconds'] for r in results]; \
print(f'Trials         : {len(results)}'); \
print(f'Passed         : {len(p)}/{len(results)}'); \
nr=[r['non_related_count'] for r in p]; \
print(f'Non-related    : min={min(nr)}  avg={sum(nr)/len(nr):.1f}  max={max(nr)} (expected 2)'); \
print(f'Duration       : min={min(d):.1f}s  avg={sum(d)/len(d):.1f}s  max={max(d):.1f}s'); \
ms={}; \
[ms.update({k:ms.get(k,0)+1}) for r in p for k in [json.dumps(r.get('subdomains_frequencies',{}),sort_keys=True)]]; \
print(f'Subdomain maps : {len(ms)} unique across {len(p)} passed trial(s)'); \
[print(f'  ({c}x) {m}') for m,c in ms.items()] \
"

# ── Test 4: Ambiguous / borderline prompts ─────────────────────────────────────

.PHONY: test-distributed-mr1-ambiguous
test-distributed-mr1-ambiguous:
	@mkdir -p testresults
	@bash scripts/health-check.sh || bash scripts/start-cluster.sh
	@set -o pipefail; \
	MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
	MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
	go test -tags integration -timeout 30m -v \
		-run TestDistributedMR1_Ambiguous \
		./integrationtests/... \
	2>&1 | tee testresults/distributed-mr1-ambiguous-$$(date +%Y%m%dT%H%M%S).log; \
	exit_code=$$?; \
	bash scripts/stop-cluster.sh; \
	exit $$exit_code

.PHONY: test-distributed-mr1-ambiguous-trials
test-distributed-mr1-ambiguous-trials:
	@mkdir -p testresults
	@echo "Running $(N) ambiguous MR1 trials — cluster restarted between each run..."
	@START=$$(date +%s); \
	for i in $$(seq 1 $(N)); do \
		echo ""; \
		echo "=== Trial $$i / $(N) ==="; \
		bash scripts/stop-cluster.sh 2>/dev/null || true; \
		bash scripts/start-cluster.sh; \
		MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
		MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
		go test -tags integration -timeout 30m \
			-run TestDistributedMR1_Ambiguous \
			./integrationtests/... || true; \
	done; \
	bash scripts/stop-cluster.sh 2>/dev/null || true; \
	echo ""; \
	echo "=== Ambiguous Trial Summary (N=$(N)) ==="; \
	python3 -c " \
import json,glob,os,sys; \
start=$$START; \
files=sorted(f for f in glob.glob('$(CURDIR)/testresults/distributed-mr1-ambiguous-*.json') if os.path.getmtime(f)>=start); \
results=[json.load(open(f)) for f in files]; \
results or sys.exit(print('No results found.') or 0); \
p=[r for r in results if r['passed']]; \
d=[r['duration_seconds'] for r in results]; \
print(f'Trials         : {len(results)}'); \
print(f'Passed         : {len(p)}/{len(results)}'); \
print(f'Duration       : min={min(d):.1f}s  avg={sum(d)/len(d):.1f}s  max={max(d):.1f}s'); \
ms={}; \
[ms.update({k:ms.get(k,0)+1}) for r in p for k in [json.dumps(r.get('subdomains_frequencies',{}),sort_keys=True)]]; \
print(f'Unique maps    : {len(ms)} across {len(p)} passed trial(s)'); \
[print(f'  ({c}x) {m}') for m,c in ms.items()] \
"

# ── Distributed MR2 diverse tests ─────────────────────────────────────────────

.PHONY: test-distributed-mr2-diverse-group-a
test-distributed-mr2-diverse-group-a:
	@mkdir -p testresults
	@bash scripts/health-check.sh || bash scripts/start-cluster.sh
	@set -o pipefail; \
	MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
	MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
	go test -tags integration -timeout 15m -v \
		-run TestDistributedMR2_Diverse_GroupA \
		./integrationtests/... \
	2>&1 | tee testresults/distributed-mr2-diverse-group-a-$$(date +%Y%m%dT%H%M%S).log; \
	exit_code=$$?; \
	bash scripts/stop-cluster.sh; \
	exit $$exit_code

.PHONY: test-distributed-mr2-diverse-group-a-trials
test-distributed-mr2-diverse-group-a-trials:
	@mkdir -p testresults
	@echo "Running $(N) distributed MR2 Diverse Group A trials — cluster restarted between each run..."
	@START=$$(date +%s); \
	for i in $$(seq 1 $(N)); do \
		echo ""; \
		echo "=== Trial $$i / $(N) ==="; \
		bash scripts/stop-cluster.sh 2>/dev/null || true; \
		bash scripts/start-cluster.sh; \
		MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
		MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
		go test -tags integration -timeout 15m \
			-run TestDistributedMR2_Diverse_GroupA \
			./integrationtests/... || true; \
		TRIAL_LOG_DIR=$(CURDIR)/testresults/agent-logs/trial-$$i; \
		mkdir -p $$TRIAL_LOG_DIR; \
		echo "  Collecting agent logs -> $$TRIAL_LOG_DIR"; \
		for m in $$(python3 -c "import json; [print(a['machine']) for a in json.load(open('$(CLUSTER_CONFIG)'))['agents']]"); do \
			ssh -o ConnectTimeout=5 $$m "cat /tmp/agent.log" > $$TRIAL_LOG_DIR/$$m-agent.log 2>/dev/null || echo "  [$$m] no log"; \
		done; \
	done; \
	bash scripts/stop-cluster.sh 2>/dev/null || true; \
	echo ""; \
	echo "=== MR2 Diverse Group A Trial Summary (N=$(N)) ==="; \
	python3 -c " \
import json,glob,os,sys; \
start=$$START; \
files=sorted(f for f in glob.glob('$(CURDIR)/testresults/distributed-mr2-diverse-group_a-*.json') if os.path.getmtime(f)>=start); \
results=[json.load(open(f)) for f in files]; \
results or sys.exit(print('No results found.') or 0); \
fin=[r for r in results if r['finalized']]; \
d=[r['duration_seconds'] for r in results]; \
print(f'Trials     : {len(results)}'); \
print(f'Finalized  : {len(fin)}/{len(results)}'); \
print(f'Duration   : min={min(d):.1f}s  avg={sum(d)/len(d):.1f}s  max={max(d):.1f}s'); \
[print('  tx %s: status=%s correct=%d wrong=%d hallucination=%d malicious=%d'%(t['tx_hash'],t['status'],t['correct'],t['wrong'],t['hallucination'],t['malicious'])) for t in (fin[0] if fin else {}).get('tx_results',[])] \
"

.PHONY: test-distributed-mr2-diverse-group-b
test-distributed-mr2-diverse-group-b:
	@mkdir -p testresults
	@bash scripts/health-check.sh || bash scripts/start-cluster.sh
	@set -o pipefail; \
	MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
	MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
	go test -tags integration -timeout 8m -v \
		-run TestDistributedMR2_Diverse_GroupB \
		./integrationtests/... \
	2>&1 | tee testresults/distributed-mr2-diverse-group-b-$$(date +%Y%m%dT%H%M%S).log; \
	exit_code=$$?; \
	bash scripts/stop-cluster.sh; \
	exit $$exit_code

.PHONY: test-distributed-mr2-diverse-group-b-trials
test-distributed-mr2-diverse-group-b-trials:
	@mkdir -p testresults
	@echo "Running $(N) distributed MR2 Diverse Group B trials — cluster restarted between each run..."
	@START=$$(date +%s); \
	for i in $$(seq 1 $(N)); do \
		echo ""; \
		echo "=== Trial $$i / $(N) ==="; \
		bash scripts/stop-cluster.sh 2>/dev/null || true; \
		bash scripts/start-cluster.sh; \
		MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
		MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
		go test -tags integration -timeout 8m \
			-run TestDistributedMR2_Diverse_GroupB \
			./integrationtests/... || true; \
		TRIAL_LOG_DIR=$(CURDIR)/testresults/agent-logs/group-b/trial-$$i; \
		mkdir -p $$TRIAL_LOG_DIR; \
		echo "  Collecting agent logs -> $$TRIAL_LOG_DIR"; \
		for m in $$(python3 -c "import json; [print(a['machine']) for a in json.load(open('$(CLUSTER_CONFIG)'))['agents']]"); do \
			ssh -o ConnectTimeout=5 $$m "cat /tmp/agent.log" > $$TRIAL_LOG_DIR/$$m-agent.log 2>/dev/null || echo "  [$$m] no log"; \
		done; \
	done; \
	bash scripts/stop-cluster.sh 2>/dev/null || true; \
	echo ""; \
	echo "=== MR2 Diverse Group B Trial Summary (N=$(N)) ==="; \
	python3 -c " \
import json,glob,os,sys; \
start=$$START; \
files=sorted(f for f in glob.glob('$(CURDIR)/testresults/distributed-mr2-diverse-group_b-*.json') if os.path.getmtime(f)>=start); \
results=[json.load(open(f)) for f in files]; \
results or sys.exit(print('No results found.') or 0); \
fin=[r for r in results if r['finalized']]; \
rejected=[r for r in fin if r.get('tx_results') and all(t['status']=='INSUFFICIENT_CORRECT_ANSWERS' and t['wrong']+t['hallucination']+t['malicious']>0 for t in r['tx_results'])]; \
d=[r['duration_seconds'] for r in results]; \
print(f'Trials     : {len(results)}'); \
print(f'Finalized  : {len(fin)}/{len(results)}'); \
print(f'Rejected   : {len(rejected)}/{len(results)}'); \
print(f'Duration   : min={min(d):.1f}s  avg={sum(d)/len(d):.1f}s  max={max(d):.1f}s'); \
[print('  tx %s: status=%s correct=%d wrong=%d hallucination=%d malicious=%d'%(t['tx_hash'],t['status'],t['correct'],t['wrong'],t['hallucination'],t['malicious'])) for t in (fin[0] if fin else {}).get('tx_results',[])] \
"

.PHONY: test-distributed-mr2-diverse-group-c
test-distributed-mr2-diverse-group-c:
	@mkdir -p testresults
	@bash scripts/health-check.sh || bash scripts/start-cluster.sh
	@set -o pipefail; \
	MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
	MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
	go test -tags integration -timeout 8m -v \
		-run TestDistributedMR2_Diverse_GroupC \
		./integrationtests/... \
	2>&1 | tee testresults/distributed-mr2-diverse-group-c-$$(date +%Y%m%dT%H%M%S).log; \
	exit_code=$$?; \
	bash scripts/stop-cluster.sh; \
	exit $$exit_code

.PHONY: test-distributed-mr2-diverse-group-c-trials
test-distributed-mr2-diverse-group-c-trials:
	@mkdir -p testresults
	@echo "Running $(N) distributed MR2 Diverse Group C trials — cluster restarted between each run..."
	@START=$$(date +%s); \
	for i in $$(seq 1 $(N)); do \
		echo ""; \
		echo "=== Trial $$i / $(N) ==="; \
		bash scripts/stop-cluster.sh 2>/dev/null || true; \
		bash scripts/start-cluster.sh; \
		MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
		MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
		go test -tags integration -timeout 8m \
			-run TestDistributedMR2_Diverse_GroupC \
			./integrationtests/... || true; \
		TRIAL_LOG_DIR=$(CURDIR)/testresults/agent-logs/group-c/trial-$$i; \
		mkdir -p $$TRIAL_LOG_DIR; \
		echo "  Collecting agent logs -> $$TRIAL_LOG_DIR"; \
		for m in $$(python3 -c "import json; [print(a['machine']) for a in json.load(open('$(CLUSTER_CONFIG)'))['agents']]"); do \
			ssh -o ConnectTimeout=5 $$m "cat /tmp/agent.log" > $$TRIAL_LOG_DIR/$$m-agent.log 2>/dev/null || echo "  [$$m] no log"; \
		done; \
	done; \
	bash scripts/stop-cluster.sh 2>/dev/null || true; \
	echo ""; \
	echo "=== MR2 Diverse Group C Trial Summary (N=$(N)) ==="; \
	python3 -c " \
import json,glob,os,sys; \
start=$$START; \
files=sorted(f for f in glob.glob('$(CURDIR)/testresults/distributed-mr2-diverse-group_c-*.json') if os.path.getmtime(f)>=start); \
results=[json.load(open(f)) for f in files]; \
results or sys.exit(print('No results found.') or 0); \
fin=[r for r in results if r['finalized']]; \
resisted=[r for r in fin if r.get('tx_results') and all(t['status']=='INSUFFICIENT_CORRECT_ANSWERS' and t['wrong']+t['hallucination']+t['malicious']>0 for t in r['tx_results'])]; \
d=[r['duration_seconds'] for r in results]; \
print(f'Trials     : {len(results)}'); \
print(f'Finalized  : {len(fin)}/{len(results)}'); \
print(f'Resisted   : {len(resisted)}/{len(results)}'); \
print(f'Duration   : min={min(d):.1f}s  avg={sum(d)/len(d):.1f}s  max={max(d):.1f}s'); \
[print('  tx %s: status=%s correct=%d wrong=%d hallucination=%d malicious=%d'%(t['tx_hash'],t['status'],t['correct'],t['wrong'],t['hallucination'],t['malicious'])) for t in (fin[0] if fin else {}).get('tx_results',[])] \
"

# ── Configurable adversarial Groups D–F ──────────────────────────────────────
# BAD_PRODUCERS controls how many of the first validators submit the group's
# adversarial answer. Supported values: 1, 2, 3.

CLASSIFICATION_GRACE_PERIOD ?= 180s

# ── Qualified-model distributed MR2 experiment ──────────────────────────────

QUALIFIED_RESULTS_DIR ?= $(CURDIR)/experiment-results
QUALIFIED_TEST_TIMEOUT ?= 45m
QUALIFIED_ROUND_TIMEOUT ?= 30m
QUALIFIED_JUDGE_TIMEOUT_SECONDS ?= 1200
QUALIFIED_LLM_TIMEOUT_SECONDS = $(if $(filter command line,$(origin LLM_TIMEOUT_SECONDS)),$(LLM_TIMEOUT_SECONDS),300)

.PHONY: verify-qualified-cluster-config
verify-qualified-cluster-config:
	@python3 scripts/distributed_mr2_qualified.py \
		--cluster-config $(CLUSTER_CONFIG) \
		--check-config-only

.PHONY: install-qualified-workers
install-qualified-workers: verify-qualified-cluster-config
	@bash scripts/install-workers.sh

.PHONY: test-distributed-mr2-qualified-all
test-distributed-mr2-qualified-all: TRIALS ?= 5
test-distributed-mr2-qualified-all: verify-qualified-cluster-config
	@python3 scripts/distributed_mr2_qualified.py \
		--trials $(TRIALS) \
		--cluster-config $(CLUSTER_CONFIG) \
		--output-base $(QUALIFIED_RESULTS_DIR) \
		--classification-grace-period $(CLASSIFICATION_GRACE_PERIOD) \
		--llm-timeout-seconds $(QUALIFIED_LLM_TIMEOUT_SECONDS) \
		--judge-timeout-seconds $(QUALIFIED_JUDGE_TIMEOUT_SECONDS) \
		--round-timeout $(QUALIFIED_ROUND_TIMEOUT) \
		--test-timeout $(QUALIFIED_TEST_TIMEOUT) \
		--make-command "make test-distributed-mr2-qualified-all TRIALS=$(TRIALS) CLASSIFICATION_GRACE_PERIOD=$(CLASSIFICATION_GRACE_PERIOD) LLM_TIMEOUT_SECONDS=$(QUALIFIED_LLM_TIMEOUT_SECONDS) QUALIFIED_JUDGE_TIMEOUT_SECONDS=$(QUALIFIED_JUDGE_TIMEOUT_SECONDS) QUALIFIED_ROUND_TIMEOUT=$(QUALIFIED_ROUND_TIMEOUT) QUALIFIED_TEST_TIMEOUT=$(QUALIFIED_TEST_TIMEOUT)"

.PHONY: test-distributed-mr2-qualified-dry-run
test-distributed-mr2-qualified-dry-run: TRIALS ?= 5
test-distributed-mr2-qualified-dry-run: verify-qualified-cluster-config
	@python3 scripts/distributed_mr2_qualified.py \
		--trials $(TRIALS) \
		--cluster-config $(CLUSTER_CONFIG) \
		--output-base $(QUALIFIED_RESULTS_DIR) \
		--classification-grace-period $(CLASSIFICATION_GRACE_PERIOD) \
		--llm-timeout-seconds $(QUALIFIED_LLM_TIMEOUT_SECONDS) \
		--judge-timeout-seconds $(QUALIFIED_JUDGE_TIMEOUT_SECONDS) \
		--round-timeout $(QUALIFIED_ROUND_TIMEOUT) \
		--test-timeout $(QUALIFIED_TEST_TIMEOUT) \
		--dry-run
ANSWER_JUDGE_PROMPT_VERSION ?= answer-judge-v4

.PHONY: test-distributed-mr2-diverse-group-d
test-distributed-mr2-diverse-group-d:
	@$(MAKE) --no-print-directory _test-distributed-mr2-diverse-configurable-adversarial ADVERSARIAL_GROUP=d TEST_GROUP=GroupD BAD_PRODUCERS=$(BAD_PRODUCERS) CLASSIFICATION_GRACE_PERIOD=$(CLASSIFICATION_GRACE_PERIOD)

.PHONY: test-distributed-mr2-diverse-group-e
test-distributed-mr2-diverse-group-e:
	@$(MAKE) --no-print-directory _test-distributed-mr2-diverse-configurable-adversarial ADVERSARIAL_GROUP=e TEST_GROUP=GroupE BAD_PRODUCERS=$(BAD_PRODUCERS) CLASSIFICATION_GRACE_PERIOD=$(CLASSIFICATION_GRACE_PERIOD)

.PHONY: test-distributed-mr2-diverse-group-f
test-distributed-mr2-diverse-group-f:
	@$(MAKE) --no-print-directory _test-distributed-mr2-diverse-configurable-adversarial ADVERSARIAL_GROUP=f TEST_GROUP=GroupF BAD_PRODUCERS=$(BAD_PRODUCERS) CLASSIFICATION_GRACE_PERIOD=$(CLASSIFICATION_GRACE_PERIOD)

.PHONY: _test-distributed-mr2-diverse-configurable-adversarial
_test-distributed-mr2-diverse-configurable-adversarial:
	@if ! echo "1 2 3" | grep -qw "$(BAD_PRODUCERS)"; then echo "BAD_PRODUCERS must be 1, 2, or 3"; exit 2; fi
	@mkdir -p testresults
	@bash scripts/health-check.sh || bash scripts/start-cluster.sh
	@set -o pipefail; \
	MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
	MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
	MOA_BAD_PRODUCERS=$(BAD_PRODUCERS) \
	MOA_CLASSIFICATION_GRACE_PERIOD=$(CLASSIFICATION_GRACE_PERIOD) \
	go test -tags integration -timeout 10m -v \
		-run TestDistributedMR2_Diverse_ConfigurableAdversarial_$(TEST_GROUP) \
		./integrationtests/... \
	2>&1 | tee testresults/distributed-mr2-diverse-adversarial-group-$(ADVERSARIAL_GROUP)-q$(BAD_PRODUCERS)-$(ANSWER_JUDGE_PROMPT_VERSION)-$$(date +%Y%m%dT%H%M%S).log; \
	exit_code=$$?; \
	bash scripts/stop-cluster.sh; \
	exit $$exit_code

.PHONY: test-distributed-mr2-diverse-group-d-trials
test-distributed-mr2-diverse-group-d-trials:
	@$(MAKE) --no-print-directory _test-distributed-mr2-diverse-configurable-adversarial-trials ADVERSARIAL_GROUP=d TEST_GROUP=GroupD N=$(N) BAD_PRODUCERS=$(BAD_PRODUCERS) CLASSIFICATION_GRACE_PERIOD=$(CLASSIFICATION_GRACE_PERIOD)

.PHONY: test-distributed-mr2-diverse-group-e-trials
test-distributed-mr2-diverse-group-e-trials:
	@$(MAKE) --no-print-directory _test-distributed-mr2-diverse-configurable-adversarial-trials ADVERSARIAL_GROUP=e TEST_GROUP=GroupE N=$(N) BAD_PRODUCERS=$(BAD_PRODUCERS) CLASSIFICATION_GRACE_PERIOD=$(CLASSIFICATION_GRACE_PERIOD)

.PHONY: test-distributed-mr2-diverse-group-f-trials
test-distributed-mr2-diverse-group-f-trials:
	@$(MAKE) --no-print-directory _test-distributed-mr2-diverse-configurable-adversarial-trials ADVERSARIAL_GROUP=f TEST_GROUP=GroupF N=$(N) BAD_PRODUCERS=$(BAD_PRODUCERS) CLASSIFICATION_GRACE_PERIOD=$(CLASSIFICATION_GRACE_PERIOD)

.PHONY: _test-distributed-mr2-diverse-configurable-adversarial-trials
_test-distributed-mr2-diverse-configurable-adversarial-trials:
	@if ! echo "1 2 3" | grep -qw "$(BAD_PRODUCERS)"; then echo "BAD_PRODUCERS must be 1, 2, or 3"; exit 2; fi
	@mkdir -p testresults
	@echo "Running $(N) distributed MR2 Group $(ADVERSARIAL_GROUP) trials with BAD_PRODUCERS=$(BAD_PRODUCERS), grace=$(CLASSIFICATION_GRACE_PERIOD), prompt=$(ANSWER_JUDGE_PROMPT_VERSION)..."
	@START=$$(date +%s); \
	for i in $$(seq 1 $(N)); do \
		echo ""; \
		echo "=== Trial $$i / $(N) — Group $(ADVERSARIAL_GROUP), BAD_PRODUCERS=$(BAD_PRODUCERS) ==="; \
		bash scripts/stop-cluster.sh 2>/dev/null || true; \
		bash scripts/start-cluster.sh; \
		MOA_CLUSTER_CONFIG=$(CLUSTER_CONFIG) \
		MOA_TEST_RESULTS_DIR=$(CURDIR)/testresults \
		MOA_BAD_PRODUCERS=$(BAD_PRODUCERS) \
		MOA_CLASSIFICATION_GRACE_PERIOD=$(CLASSIFICATION_GRACE_PERIOD) \
		go test -tags integration -timeout 10m \
			-run TestDistributedMR2_Diverse_ConfigurableAdversarial_$(TEST_GROUP) \
			./integrationtests/... || true; \
		TRIAL_LOG_DIR=$(CURDIR)/testresults/agent-logs/group-$(ADVERSARIAL_GROUP)/bad-producers-$(BAD_PRODUCERS)/grace-$(CLASSIFICATION_GRACE_PERIOD)/prompt-$(ANSWER_JUDGE_PROMPT_VERSION)/trial-$$i; \
		mkdir -p $$TRIAL_LOG_DIR; \
		echo "  Collecting agent logs -> $$TRIAL_LOG_DIR"; \
		for m in $$(python3 -c "import json; [print(a['machine']) for a in json.load(open('$(CLUSTER_CONFIG)'))['agents']]"); do \
			ssh -o ConnectTimeout=5 $$m "cat /tmp/agent.log" > $$TRIAL_LOG_DIR/$$m-agent.log 2>/dev/null || echo "  [$$m] no log"; \
		done; \
		mkdir -p $$TRIAL_LOG_DIR/validator-logs; \
		cp integrationtests/logs/TestDistributedMR2_Diverse_ConfigurableAdversarial_$(TEST_GROUP)/validator-*.log $$TRIAL_LOG_DIR/validator-logs/ 2>/dev/null || echo "  no validator logs"; \
	done; \
	bash scripts/stop-cluster.sh 2>/dev/null || true; \
	echo ""; \
	echo "=== Group $(ADVERSARIAL_GROUP), BAD_PRODUCERS=$(BAD_PRODUCERS) Summary ==="; \
	python3 -c "\
import json,glob,os,sys; \
start=$$START; \
files=sorted(f for f in glob.glob('$(CURDIR)/testresults/distributed-mr2-diverse-adversarial-group_$(ADVERSARIAL_GROUP)-q$(BAD_PRODUCERS)-*.json') if os.path.getmtime(f)>=start); \
results=[json.load(open(f)) for f in files]; \
results or sys.exit(print('No results found.') or 0); \
fin=[r for r in results if r['finalized']]; equal=[r for r in fin if r['all_nodes_equal']]; \
safe=[r for r in equal if all(not ({c['producer_id'] for c in t['correct_candidates']} & set(r['bad_producer_ids'])) for t in r['tx_results'])]; \
ready=sum(t['status']=='READY_FOR_MINI_ROUND_THREE' for r in equal for t in r['tx_results']); total=sum(len(r['tx_results']) for r in equal); \
d=[r['duration_seconds'] for r in results]; \
print(f'Trials                 : {len(results)}'); \
print(f'Finalized              : {len(fin)}/{len(results)}'); \
print(f'All nodes equal        : {len(equal)}/{len(results)}'); \
print(f'No canonical bad accept: {len(safe)}/{len(results)}'); \
print(f'Ready transactions     : {ready}/{total}'); \
print('Prompt versions        : %s'%sorted(set(r.get('prompt_version','unknown') for r in results))); \
print('Certificate vote counts: %s'%[r.get('certificate_vote_count',0) for r in equal]); \
print(f'Duration               : min={min(d):.1f}s avg={sum(d)/len(d):.1f}s max={max(d):.1f}s'); \
[print('  trial round=%s: %s'%(r['round_number'], ', '.join('%s=%s C%d/W%d/H%d/M%d'%(t['tx_hash'],t['status'],t['correct'],t['wrong'],t['hallucination'],t['malicious']) for t in r.get('tx_results',[])))) for r in results] \
"

# ── Judge qualification benchmark ─────────────────────────────────────────────
#
# Runs the standalone semantic judge benchmark against a live Ollama instance.
# No blockchain nodes, committees, or consensus rounds are started.
#
# Usage:
#   make benchmark-judge MODEL=qwen3.5:9b
#   make benchmark-judge MODEL=phi4:14b BENCHMARK_OUTPUT=results/phi4 TRIALS=3
#   make benchmark-judges
#   make benchmark-judge-dataset-check
#
# Prerequisites:
#   cd agent-python && python -m pip install -r requirements.txt
#   ollama pull <MODEL>
#
# ─────────────────────────────────────────────────────────────────────────────

MODEL             ?= qwen3.5:9b
BENCHMARK_MODELS  ?= qwen3.5:9b gemma4:12b ministral-3:14b phi4:14b phi4-reasoning:14b
BENCHMARK_OUTPUT  ?= benchmark_results
BENCHMARK_URL     ?= http://127.0.0.1:11434
TRIALS            ?= 1
BENCHMARK_TIMEOUT ?= 120

.PHONY: benchmark-pull-models
benchmark-pull-models:
	@echo "Pulling benchmark models into Ollama (this may take 10-30 min per model)..."
	@for model in $(BENCHMARK_MODELS); do \
		echo ""; \
		echo "=== Pulling $$model ==="; \
		ollama pull $$model || echo "[WARN] Failed to pull $$model — check the name with: ollama list"; \
	done
	@echo ""
	@echo "=== Available models ==="
	@ollama list

.PHONY: benchmark-judge
benchmark-judge:
	@cd agent-python && \
	.venv/bin/python -m benchmark run \
		--model $(MODEL) \
		--base-url $(BENCHMARK_URL) \
		--output-dir ../$(BENCHMARK_OUTPUT) \
		--trials $(TRIALS) \
		--timeout $(BENCHMARK_TIMEOUT)

.PHONY: benchmark-judges
benchmark-judges:
	@cd agent-python && \
	.venv/bin/python -m benchmark run-all \
		--base-url $(BENCHMARK_URL) \
		--output-dir ../$(BENCHMARK_OUTPUT) \
		--trials $(TRIALS) \
		--timeout $(BENCHMARK_TIMEOUT)

.PHONY: benchmark-judge-dataset-check
benchmark-judge-dataset-check:
	@cd agent-python && \
	.venv/bin/python -m benchmark check-dataset

.PHONY: test-benchmark
test-benchmark:
	@cd agent-python && \
	.venv/bin/python -m pytest benchmark/tests/ -v
