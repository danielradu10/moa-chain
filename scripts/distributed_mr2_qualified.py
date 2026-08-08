#!/usr/bin/env python3
"""Unattended runner for the qualified distributed MR2 experiment matrix."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import time
from collections import defaultdict
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable


EXPECTED_MODELS = [
    "qwen3.5:9b", "gemma4:12b", "qwen3.5:9b", "gemma4:12b",
    "qwen3.5:9b", "gemma4:12b", "qwen3.5:9b", "gemma4:12b",
    "qwen3.5:9b", "qwen3.5:9b",
]
PROMPT_VERSION = "answer-judge-v4"
PROMPT_HASH = "768d4c9632e1098d94475e1cf04ec4922aed193e70a53742ca137b3d3725b5b2"


@dataclass(frozen=True)
class MatrixEntry:
    group: str
    q: int
    test_name: str


MATRIX = (
    MatrixEntry("a", 0, "TestDistributedMR2_Diverse_GroupA_CanonicalPreferenceBias"),
    MatrixEntry("b", 0, "TestDistributedMR2_Diverse_GroupB_WrongAnswerIsRejected"),
    MatrixEntry("c", 0, "TestDistributedMR2_Diverse_GroupC_PromptInjectionResistance"),
    MatrixEntry("d", 1, "TestDistributedMR2_Diverse_ConfigurableAdversarial_GroupD"),
    MatrixEntry("d", 2, "TestDistributedMR2_Diverse_ConfigurableAdversarial_GroupD"),
    MatrixEntry("d", 3, "TestDistributedMR2_Diverse_ConfigurableAdversarial_GroupD"),
    MatrixEntry("e", 1, "TestDistributedMR2_Diverse_ConfigurableAdversarial_GroupE"),
    MatrixEntry("e", 2, "TestDistributedMR2_Diverse_ConfigurableAdversarial_GroupE"),
    MatrixEntry("f", 1, "TestDistributedMR2_Diverse_ConfigurableAdversarial_GroupF"),
    MatrixEntry("f", 2, "TestDistributedMR2_Diverse_ConfigurableAdversarial_GroupF"),
)


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def load_config(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text())


def validate_qualified_config(config: dict[str, Any]) -> None:
    agents = config.get("agents", [])
    if len(agents) != 10:
        raise ValueError(f"qualified experiment requires 10 agents, found {len(agents)}")
    if config.get("committeeStrategy") != "full":
        raise ValueError("qualified experiment requires committeeStrategy=full")
    models = [agent.get("model") for agent in agents]
    if models != EXPECTED_MODELS:
        raise ValueError(f"qualified model assignment mismatch: {models}")
    for index, agent in enumerate(agents):
        expected_machine = f"moa-chain-{index}"
        if agent.get("machine") != expected_machine:
            raise ValueError(f"validator-{index + 1} must map to {expected_machine}")
        if float(agent.get("temperature", -1)) != 0.0:
            raise ValueError(f"validator-{index + 1} temperature must be 0")


def build_schedule(trials: int) -> list[tuple[MatrixEntry, int]]:
    if trials < 1:
        raise ValueError("TRIALS must be at least 1")
    return [(entry, trial) for entry in MATRIX for trial in range(1, trials + 1)]


def unique_experiment_root(base: Path, now: datetime | None = None) -> Path:
    stamp = (now or datetime.now()).strftime("%Y%m%d-%H%M%S")
    candidate = base / f"distributed-mr2-qualified-{stamp}"
    suffix = 1
    while candidate.exists():
        candidate = base / f"distributed-mr2-qualified-{stamp}-{suffix:02d}"
        suffix += 1
    return candidate


class ExperimentLog:
    def __init__(self, path: Path) -> None:
        self.path = path
        self._file = path.open("a", buffering=1)

    def close(self) -> None:
        self._file.close()

    def write(self, message: str) -> None:
        print(message, flush=True)
        self._file.write(message + "\n")

    def run(self, command: list[str], *, env: dict[str, str] | None = None,
            cwd: Path | None = None, output_file: Path | None = None) -> tuple[int, str]:
        self.write("[Command] " + " ".join(command))
        captured: list[str] = []
        trial_handle = output_file.open("a", buffering=1) if output_file else None
        try:
            process = subprocess.Popen(
                command, cwd=cwd, env=env, text=True,
                stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
            )
            assert process.stdout is not None
            for line in process.stdout:
                line = line.rstrip("\n")
                captured.append(line)
                print(line, flush=True)
                self._file.write(line + "\n")
                if trial_handle:
                    trial_handle.write(line + "\n")
            return process.wait(), "\n".join(captured)
        finally:
            if trial_handle:
                trial_handle.close()


def git_metadata(repo: Path) -> tuple[str, bool | None]:
    try:
        commit = subprocess.check_output(
            ["git", "rev-parse", "HEAD"], cwd=repo, text=True,
            stderr=subprocess.DEVNULL,
        ).strip()
        dirty = bool(subprocess.check_output(
            ["git", "status", "--porcelain"], cwd=repo, text=True,
            stderr=subprocess.DEVNULL,
        ).strip())
        return commit, dirty
    except (OSError, subprocess.CalledProcessError):
        return "unavailable", None


def expected_bad_producers(group: str, q: int) -> list[str]:
    count = 4 if group in {"b", "c"} else q
    return [f"validator-{index}" for index in range(1, count + 1)]


def copy_result_json(trial_dir: Path) -> dict[str, Any] | None:
    candidates = sorted(
        path for path in trial_dir.glob("distributed-mr2-*.json")
        if path.name not in {"metadata.json", "trial-status.json", "result.json"}
    )
    if len(candidates) != 1:
        return None
    result = json.loads(candidates[0].read_text())
    (trial_dir / "result.json").write_text(json.dumps(result, indent=2) + "\n")
    return result


def classify_trial(result: dict[str, Any] | None, test_rc: int, output: str,
                   collection_ok: bool, setup_ok: bool = True) -> tuple[str, str]:
    if not setup_ok or not collection_ok:
        return "INFRASTRUCTURE_ERROR", "cluster setup or required log collection failed"
    if "test timed out after" in output.lower() or "panic: test timed out" in output.lower():
        return "TIMEOUT", "go test timeout"
    if result is None:
        if test_rc != 0:
            return "TEST_FAILED", f"go test exited {test_rc} without a result JSON"
        return "INFRASTRUCTURE_ERROR", "test produced no unique result JSON"
    if not result.get("finalized", False):
        return "NON_FINALIZED", "protocol round did not finalize"
    if test_rc != 0:
        return "FINALIZED", f"protocol finalized; scenario assertion exited {test_rc}"
    return "FINALIZED", "protocol round finalized"


def canonical_metrics(result: dict[str, Any]) -> dict[str, int]:
    metrics = dict(adversarial_rejected=0, adversarial_accepted=0,
                   legitimate_retained=0, legitimate_rejected=0)
    if not result.get("finalized"):
        return metrics
    bad = set(result.get("bad_producer_ids") or [])
    for tx in result.get("tx_results") or []:
        for field, accepted in (
            ("correct_candidates", True), ("wrong_candidates", False),
            ("hallucination_candidates", False), ("malicious_candidates", False),
        ):
            for candidate in tx.get(field) or []:
                adversarial = candidate.get("producer_id") in bad
                key = (
                    "adversarial_accepted" if accepted else "adversarial_rejected"
                ) if adversarial else (
                    "legitimate_retained" if accepted else "legitimate_rejected"
                )
                metrics[key] += 1
    return metrics


def semantic_errors(result: dict[str, Any]) -> tuple[int, dict[str, dict[str, int]]]:
    total = 0
    by_model: dict[str, dict[str, int]] = defaultdict(lambda: {
        "decisions": 0, "errors": 0, "legitimate_total": 0,
        "legitimate_retained": 0, "adversarial_total": 0,
        "adversarial_rejected": 0,
    })
    for vote in result.get("classification_votes") or []:
        if vote.get("mocked_byzantine"):
            continue
        model = vote.get("model", "unknown")
        for classification in vote.get("classifications") or []:
            by_model[model]["decisions"] += 1
            if classification.get("is_adversarial"):
                by_model[model]["adversarial_total"] += 1
                if classification.get("actual") != "CORRECT":
                    by_model[model]["adversarial_rejected"] += 1
            else:
                by_model[model]["legitimate_total"] += 1
                if classification.get("actual") == "CORRECT":
                    by_model[model]["legitimate_retained"] += 1
            if classification.get("semantic_error"):
                total += 1
                by_model[model]["errors"] += 1
    return total, dict(by_model)


def summarize(root: Path, trials_requested: int) -> dict[str, Any]:
    configurations: dict[str, Any] = {}
    model_totals: dict[str, dict[str, int]] = defaultdict(lambda: {
        "decisions": 0, "errors": 0, "legitimate_total": 0,
        "legitimate_retained": 0, "adversarial_total": 0,
        "adversarial_rejected": 0,
    })
    for entry in MATRIX:
        key = f"group-{entry.group}/q{entry.q}"
        data: dict[str, Any] = {
            "group": entry.group.upper(), "q": entry.q,
            "trials_requested": trials_requested, "trials_attempted": 0,
            "trials_completed": 0, "trials_finalized": 0,
            "protocol_non_finalizations": 0, "test_failures": 0,
            "infrastructure_failures": 0, "timeouts": 0,
            "adversarial_rejected": 0, "adversarial_accepted": 0,
            "legitimate_retained": 0, "legitimate_rejected": 0,
            "error_free_canonical_rounds": 0, "honest_semantic_errors": 0,
            "total_duration_seconds": 0.0,
        }
        config_dir = root / f"group-{entry.group}" / f"q{entry.q}"
        for status_path in sorted(config_dir.glob("trial-*/trial-status.json")):
            data["trials_attempted"] += 1
            status = json.loads(status_path.read_text())
            status_name = status["status"]
            data["total_duration_seconds"] += float(status.get("duration_seconds", 0))
            result_path = status_path.parent / "result.json"
            result = json.loads(result_path.read_text()) if result_path.exists() else None
            if result is not None:
                data["trials_completed"] += 1
                metrics = canonical_metrics(result)
                for metric, value in metrics.items():
                    data[metric] += value
                errors, by_model = semantic_errors(result)
                data["honest_semantic_errors"] += errors
                for model, values in by_model.items():
                    for metric, value in values.items():
                        model_totals[model][metric] += value
                if result.get("finalized"):
                    data["trials_finalized"] += 1
                    if (metrics["adversarial_accepted"] == 0 and
                            metrics["legitimate_rejected"] == 0):
                        data["error_free_canonical_rounds"] += 1
            if status_name == "NON_FINALIZED":
                data["protocol_non_finalizations"] += 1
            elif status_name == "TEST_FAILED":
                data["test_failures"] += 1
            elif status_name == "INFRASTRUCTURE_ERROR":
                data["infrastructure_failures"] += 1
            elif status_name == "TIMEOUT":
                data["timeouts"] += 1
        configurations[key] = data
    return {
        "generated_at": utc_now(), "trials_per_configuration": trials_requested,
        "configurations": configurations, "by_model": dict(model_totals),
    }


def write_summary(root: Path, trials: int) -> dict[str, Any]:
    summary = summarize(root, trials)
    (root / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    lines = ["# Distributed MR2 qualified-cluster summary", "",
             f"Generated: {summary['generated_at']}", "",
             "| Group | q | Attempted | Completed | Finalized | Non-finalized | Infra | Timeout | Adv rejected | Adv accepted | Legit retained | Legit rejected | Error-free | Honest errors | Duration |",
             "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|"]
    for data in summary["configurations"].values():
        lines.append(
            f"| {data['group']} | {data['q']} | {data['trials_attempted']} | "
            f"{data['trials_completed']} | {data['trials_finalized']} | "
            f"{data['protocol_non_finalizations']} | {data['infrastructure_failures']} | "
            f"{data['timeouts']} | {data['adversarial_rejected']} | "
            f"{data['adversarial_accepted']} | {data['legitimate_retained']} | "
            f"{data['legitimate_rejected']} | {data['error_free_canonical_rounds']} | "
            f"{data['honest_semantic_errors']} | {data['total_duration_seconds']:.1f}s |"
        )
    lines.extend(["", "## Honest semantic decisions by model", "",
                  "| Model | Decisions | Exact errors | Legitimate retained | Adversarial rejected |",
                  "|---|---:|---:|---:|---:|"])
    for model, values in sorted(summary["by_model"].items()):
        lines.append(
            f"| `{model}` | {values['decisions']} | {values['errors']} | "
            f"{values['legitimate_retained']}/{values['legitimate_total']} | "
            f"{values['adversarial_rejected']}/{values['adversarial_total']} |"
        )
    (root / "summary.md").write_text("\n".join(lines) + "\n")
    return summary


class Runner:
    def __init__(self, args: argparse.Namespace, repo: Path) -> None:
        self.args = args
        self.repo = repo
        self.config_path = Path(args.cluster_config).resolve()
        self.config = load_config(self.config_path)
        validate_qualified_config(self.config)
        self.root = unique_experiment_root(Path(args.output_base).resolve())
        self.root.mkdir(parents=True)
        self.log = ExperimentLog(self.root / "experiment.log")
        self.interrupted = False

    def _command_env(self, entry: MatrixEntry) -> dict[str, str]:
        env = os.environ.copy()
        env.update({
            "MOA_CLUSTER_CONFIG": str(self.config_path),
            "MOA_BAD_PRODUCERS": str(entry.q),
            "MOA_CLASSIFICATION_GRACE_PERIOD": self.args.classification_grace_period,
            "MOA_MR2_ROUND_TIMEOUT": self.args.round_timeout,
            "MOA_JUDGE_TIMEOUT_SECONDS": str(self.args.judge_timeout_seconds),
            "LLM_TIMEOUT_SECONDS": str(self.args.llm_timeout_seconds),
            "LLM_NUM_CTX": "4096", "LLM_NUM_PREDICT": "256", "LLM_THINK": "false",
        })
        return env

    def write_metadata(self) -> None:
        commit, dirty = git_metadata(self.repo)
        agents = []
        for index, agent in enumerate(self.config["agents"], start=1):
            agents.append({
                "validator_id": f"validator-{index}", "machine": agent["machine"],
                "url": agent["url"], "model": agent["model"],
                "temperature": agent["temperature"],
            })
        metadata = {
            "experiment_id": self.root.name, "start_timestamp": utc_now(),
            "trials": self.args.trials, "git_commit": commit, "git_dirty": dirty,
            "make_command": self.args.make_command,
            "matrix": [asdict(entry) for entry in MATRIX], "agents": agents,
            "judge": {"prompt_version": PROMPT_VERSION, "prompt_hash": PROMPT_HASH,
                      "temperature": 0.0, "think": False, "num_ctx": 4096,
                      "num_predict": 256, "stream": False},
            "timeouts": {"llm_seconds": self.args.llm_timeout_seconds,
                         "judge_http_seconds": self.args.judge_timeout_seconds,
                         "mr2_round": self.args.round_timeout,
                         "go_test": self.args.test_timeout,
                         "classification_grace_period": self.args.classification_grace_period},
            "byzantine_behavior": "first q validators use in-process stub; no LLM call",
            "output_directory": str(self.root),
            "isolation": "fresh Go process and agent process per trial; Ollama/model cache retained",
        }
        (self.root / "run-metadata.json").write_text(json.dumps(metadata, indent=2) + "\n")

    def verify_prerequisites(self) -> bool:
        for command in ("go", "python3", "ssh", "curl"):
            if shutil.which(command) is None:
                self.log.write(f"[Prerequisite] missing command: {command}")
                return False
        ok = True
        for agent in self.config["agents"]:
            rc, output = self.log.run([
                "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10",
                "-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=2",
                agent["machine"], "ollama", "list",
            ], cwd=self.repo)
            installed_models = {
                line.split()[0] for line in output.splitlines()[1:] if line.split()
            }
            if rc != 0 or agent["model"] not in installed_models:
                self.log.write(f"[Prerequisite] {agent['machine']} missing {agent['model']}")
                ok = False
        return ok

    def collect_logs(self, entry: MatrixEntry, trial_dir: Path) -> bool:
        nodes_dir = trial_dir / "nodes"
        nodes_dir.mkdir(exist_ok=True)
        ok = True
        for index, agent in enumerate(self.config["agents"], start=1):
            for remote_name, suffix, required in (
                ("/tmp/agent.log", "agent", True), ("/tmp/ollama.log", "ollama", False),
            ):
                destination = nodes_dir / f"validator-{index}-{suffix}.log"
                with destination.open("w") as handle:
                    process = subprocess.run(
                        ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10",
                         "-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=2",
                         agent["machine"], "cat", remote_name],
                        stdout=handle, stderr=subprocess.PIPE, text=True,
                    )
                if process.returncode != 0 and required:
                    self.log.write(f"[LogCollection] FAILED {agent['machine']}:{remote_name}: {process.stderr.strip()}")
                    ok = False
        source = self.repo / "integrationtests" / "logs" / entry.test_name
        validator_dir = nodes_dir / "validator-consensus"
        if source.is_dir():
            shutil.copytree(source, validator_dir)
        else:
            self.log.write(f"[LogCollection] FAILED missing consensus logs: {source}")
            ok = False
        if ok:
            self.log.write(f"[LogCollection] complete: {nodes_dir}")
        return ok

    def run_trial(self, entry: MatrixEntry, trial: int) -> str:
        trial_dir = self.root / f"group-{entry.group}" / f"q{entry.q}" / f"trial-{trial:03d}"
        trial_dir.mkdir(parents=True, exist_ok=False)
        mocked = [f"validator-{i}" for i in range(1, entry.q + 1)]
        trial_metadata = {
            "group": entry.group.upper(), "q": entry.q, "trial": trial,
            "test_name": entry.test_name, "start_timestamp": utc_now(),
            "bad_producer_ids": expected_bad_producers(entry.group, entry.q),
            "mocked_byzantine_judges": mocked,
            "validators": [
                {"validator_id": f"validator-{i}", "machine": a["machine"],
                 "model": a["model"], "mocked_byzantine": f"validator-{i}" in mocked}
                for i, a in enumerate(self.config["agents"], start=1)
            ],
        }
        (trial_dir / "metadata.json").write_text(json.dumps(trial_metadata, indent=2) + "\n")
        label = f"[Group {entry.group.upper()}][q={entry.q}][Trial {trial}/{self.args.trials}]"
        self.log.write(f"{label} START")
        started = time.monotonic()
        env = self._command_env(entry)

        stop_rc, _ = self.log.run(["bash", "scripts/stop-cluster.sh"], env=env, cwd=self.repo)
        log_source = self.repo / "integrationtests" / "logs" / entry.test_name
        if log_source.exists():
            shutil.rmtree(log_source)
        start_rc, start_output = self.log.run(["bash", "scripts/start-cluster.sh"], env=env, cwd=self.repo)
        setup_ok = stop_rc == 0 and start_rc == 0
        test_rc, test_output = 1, start_output
        if setup_ok:
            test_env = env.copy()
            test_env["MOA_TEST_RESULTS_DIR"] = str(trial_dir)
            command = [
                "go", "test", "-count=1", "-tags", "integration",
                "-timeout", self.args.test_timeout, "-v", "-run", f"^{entry.test_name}$",
                "./integrationtests/...",
            ]
            test_rc, test_output = self.log.run(
                command, env=test_env, cwd=self.repo, output_file=trial_dir / "trial.log",
            )
        result = copy_result_json(trial_dir)
        # Collect best-effort diagnostics even after a partial startup failure.
        # The status still remains an infrastructure error, but reachable-node
        # logs are not lost when they are most useful.
        collection_ok = self.collect_logs(entry, trial_dir)
        status, detail = classify_trial(result, test_rc, test_output, collection_ok, setup_ok)
        duration = time.monotonic() - started
        trial_status = {
            "status": status, "detail": detail, "test_exit_code": test_rc,
            "cluster_stop_exit_code": stop_rc, "cluster_start_exit_code": start_rc,
            "log_collection_ok": collection_ok, "duration_seconds": duration,
            "finish_timestamp": utc_now(),
        }
        (trial_dir / "trial-status.json").write_text(json.dumps(trial_status, indent=2) + "\n")
        write_summary(self.root, self.args.trials)
        self.log.write(f"{label} FINISHED - {status} duration={duration:.1f}s")
        return status

    def run(self) -> int:
        experiment_started = time.monotonic()
        self.write_metadata()
        metadata = json.loads((self.root / "run-metadata.json").read_text())
        self.log.write(f"[Experiment] root: {self.root}")
        self.log.write(f"[Experiment] start: {utc_now()}")
        self.log.write(f"[Experiment] trials/configuration: {self.args.trials}")
        self.log.write(f"[Experiment] git: {metadata['git_commit']} dirty={metadata['git_dirty']}")
        self.log.write(f"[Experiment] models: {EXPECTED_MODELS}")
        self.log.write(f"[Experiment] timeouts: {metadata['timeouts']}")
        if self.args.dry_run:
            for entry, trial in build_schedule(self.args.trials):
                self.log.write(f"[DryRun] Group {entry.group.upper()} q={entry.q} trial={trial} test={entry.test_name}")
            write_summary(self.root, self.args.trials)
            self.log.write(f"[Experiment] dry-run schedule entries: {len(build_schedule(self.args.trials))}")
            self.log.write(f"[Experiment] total duration: {time.monotonic() - experiment_started:.1f}s")
            self.log.close()
            return 0
        if not self.verify_prerequisites():
            self.log.write("[Experiment] fatal prerequisite failure")
            write_summary(self.root, self.args.trials)
            self.log.write(f"[Experiment] total duration: {time.monotonic() - experiment_started:.1f}s")
            self.log.close()
            return 2
        statuses = []
        try:
            for entry, trial in build_schedule(self.args.trials):
                statuses.append(self.run_trial(entry, trial))
        finally:
            self.log.run(["bash", "scripts/stop-cluster.sh"], cwd=self.repo)
            summary = write_summary(self.root, self.args.trials)
            self.log.write(f"[Experiment] finish: {utc_now()}")
            self.log.write(f"[Experiment] total duration: {time.monotonic() - experiment_started:.1f}s")
            self.log.write(f"[Experiment] summary: {self.root / 'summary.md'}")
            self.log.close()
        return 1 if any(status in {"INFRASTRUCTURE_ERROR", "TIMEOUT", "TEST_FAILED"} for status in statuses) else 0


def parse_args(argv: Iterable[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--trials", type=int, default=5)
    parser.add_argument("--cluster-config", default="configs/cluster.json")
    parser.add_argument("--output-base", default="experiment-results")
    parser.add_argument("--classification-grace-period", default="180s")
    parser.add_argument("--llm-timeout-seconds", type=int, default=300)
    parser.add_argument("--judge-timeout-seconds", type=int, default=1200)
    parser.add_argument("--round-timeout", default="30m")
    parser.add_argument("--test-timeout", default="45m")
    parser.add_argument("--make-command", default="make test-distributed-mr2-qualified-all")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--check-config-only", action="store_true")
    args = parser.parse_args(argv)
    if args.trials < 1:
        parser.error("--trials must be at least 1")
    return args


def main(argv: Iterable[str] | None = None) -> int:
    args = parse_args(argv)
    repo = Path(__file__).resolve().parent.parent
    try:
        if args.check_config_only:
            config = load_config(Path(args.cluster_config).resolve())
            validate_qualified_config(config)
            for index, agent in enumerate(config["agents"], start=1):
                print(f"validator-{index} -> {agent['machine']} -> {agent['model']}")
            return 0
        return Runner(args, repo).run()
    except (ValueError, OSError, json.JSONDecodeError) as exc:
        print(f"qualified runner error: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
