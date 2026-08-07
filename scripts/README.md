# Scripts

This directory contains two categories of scripts:

- **Local-only** (`sync-to-master.sh`, this file): run from your local machine. Never synced to the cluster.
- **Cluster scripts** (everything else): synced to moa-chain-0 and run from there.

---

## Local scripts

### `sync-to-master.sh`

Pushes the repository from your local machine to moa-chain-0 using `rsync` over SSH.

**How it works:**

`rsync` is a file synchronisation tool. It compares source and destination and transfers
only the diff — so the second run is much faster than the first.

Key flags used:
- `-a` — archive mode: preserves permissions, timestamps, symlinks, recurses subdirectories
- `-v` — verbose: prints each file being transferred
- `-z` — compresses data in transit
- `--delete` — removes files on the remote that no longer exist locally, keeping both sides in exact sync
- `-e "ssh -i $KEY -l ubuntu"` — uses SSH as the transport with the cluster identity file

The `--exclude` list prevents sensitive, large, or machine-specific files from being
transferred: git history, Python virtualenvs, bytecode caches, IDE files, PDFs, and
this file itself.

**Usage:** run from your local machine workspace root:
```bash
bash work/moa-chain/scripts/sync-to-master.sh
```

### `collect-benchmark-results.sh`

Pulls standalone judge benchmark outputs from `benchmark_results/` on moa-chain-0 into
the local repository. It never deletes remote or local files. With no argument it pulls
all runs; pass a run directory name to collect only that run.

**Usage:**
```bash
bash scripts/collect-benchmark-results.sh
bash scripts/collect-benchmark-results.sh qualification_20260806_run01
```

---

## Cluster scripts (run on moa-chain-0)

### Qualified MR2 experiment

The qualified cluster mapping is resolved in validator order from
`configs/cluster.json`: array entry 1 is `validator-1` on `moa-chain-0`, entry 2
is `validator-2` on `moa-chain-1`, and so on. The checked configuration contains
six `qwen3.5:9b` workers and four `gemma4:12b` workers, all at temperature zero.

Prepare or update the workers from moa-chain-0:

```bash
make install-qualified-workers
```

This deploys the current `agent-python`, installs Ollama only when absent, and
pulls only the model assigned to each worker when it is not already present.

Validate the complete ten-configuration schedule without contacting workers:

```bash
make test-distributed-mr2-qualified-dry-run TRIALS=1
```

Run the real experiment inside tmux:

```bash
make test-distributed-mr2-qualified-all TRIALS=10
```

The internal MR2 observation window, outer Go test timeout, judge request
timeout, and classification grace period remain overrideable, for example:

```bash
make test-distributed-mr2-qualified-all TRIALS=10 \
  QUALIFIED_ROUND_TIMEOUT=30m QUALIFIED_TEST_TIMEOUT=45m \
  QUALIFIED_JUDGE_TIMEOUT_SECONDS=1200 \
  LLM_TIMEOUT_SECONDS=300 CLASSIFICATION_GRACE_PERIOD=180s
```

Every invocation creates a unique directory under `experiment-results/`. Each
trial uses a fresh Go test process and restarts the stateless Python agent.
Ollama and its loaded model cache intentionally remain alive between trials.
Agent logs and consensus validator logs are copied before the next trial starts.
The runner continues after protocol non-finalization and semantic assertion
failures, but exits non-zero if infrastructure failure, timeout, or an execution
failure prevents a trial result from being persisted.

### `install-workers.sh`

One-time setup script. Run once (or again if machines are reset). Idempotent — safe to re-run.

Does three things across all 10 machines in parallel:

1. **Deploy `agent-python`**: rsyncs the Python agent server source to `~/agent-python/`
   on each machine, creates a virtualenv, and installs dependencies via pip.

2. **Install Ollama**: checks if the `ollama` binary exists; if not, runs the official
   install script (`curl ... | sh`). On Ubuntu this registers a systemd service.

3. **Pull the model**: starts Ollama temporarily if needed, then runs `ollama pull <model>`.
   This downloads several GB — runs in parallel across all machines, takes 5-10 min first time.

**Usage:**
```bash
cd ~/moa-chain && bash scripts/install-workers.sh
```

**Checkpoints after running:**
```bash
ssh moa-chain-1 'ollama list'
ssh moa-chain-3 '~/agent-python/.venv/bin/python3 -c "import fastapi; print(fastapi.__version__)"'
```

---

### `start-cluster.sh`

Starts the agent server on every machine with the temperature configured in `configs/cluster.json`.

For each machine (in parallel):
1. Starts Ollama if not already running (`nohup ... & disown` survives SSH disconnect)
2. Kills any existing process on port 8081
3. Starts uvicorn with `--host 0.0.0.0` so it is reachable from moa-chain-0

After starting all machines, polls `/health` on each agent until `"reachable":true`
or a 90-second timeout.

**Why `nohup ... & disown`**: a plain background process dies when the SSH session ends.
`nohup` detaches it from the terminal and `disown` removes it from the shell's job table,
so it keeps running after disconnect.

**Usage:**
```bash
cd ~/moa-chain && bash scripts/start-cluster.sh
```

---

### `stop-cluster.sh`

Stops the agent server (uvicorn) on every machine.

By default only kills uvicorn on port 8081 — Ollama is left running because restarting
it requires reloading the model into memory (~30s per machine). Pass `--stop-ollama`
to also kill Ollama when you need a full reset.

**Usage:**
```bash
cd ~/moa-chain && bash scripts/stop-cluster.sh
cd ~/moa-chain && bash scripts/stop-cluster.sh --stop-ollama
```

---

### `health-check.sh`

Polls `/health` on every agent and prints a status table:

```
machine       url                        temp   status   ollama
moa-chain-0   http://moa-chain-0:8081    0.30   OK       reachable
moa-chain-1   http://moa-chain-1:8081    0.35   OK       reachable
...
```

Useful to run before starting integration tests to confirm all agents are up.

**Usage:**
```bash
cd ~/moa-chain && bash scripts/health-check.sh
```
