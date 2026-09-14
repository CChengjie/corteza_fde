#!/usr/bin/env python3
"""Run SWE-agent on one C311 task, then execute that task's evaluator.

Example:
  .venv/bin/python run_swe_agent.py /path/to/C311-FUNC-FND-01
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parent
ENV_FILE = ROOT / ".env"


def load_env() -> dict[str, str]:
    values = dict(os.environ)
    for raw in ENV_FILE.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if line and not line.startswith("#") and "=" in line:
            key, value = line.split("=", 1)
            values.setdefault(key.strip(), value.strip().strip("\"'"))
    for key in ("OPENAI_API_KEY", "OPENAI_BASE_URL", "MODEL"):
        if not values.get(key):
            raise RuntimeError(f"{ENV_FILE} must define {key}")
    values.setdefault("OPENAI_API_BASE", values["OPENAI_BASE_URL"])
    values.setdefault("SWE_AGENT_MODEL", values["MODEL"])
    return values


def task_id(task: Path) -> str:
    for line in (task / "task.yaml").read_text(encoding="utf-8").splitlines():
        if line.startswith("task_id:"):
            return line.split(":", 1)[1].strip()
    raise RuntimeError(f"task_id missing from {task / 'task.yaml'}")


def prepare(task: Path, run_name: str) -> tuple[Path, Path]:
    baseline = task / "B" / "customer-system"
    if not baseline.is_dir():
        raise RuntimeError(f"B/customer-system missing from {task}")
    work = ROOT / "workspaces" / "swe-agent" / run_name
    candidate = work / "candidate"
    if work.exists():
        shutil.rmtree(work)
    shutil.copytree(baseline, candidate)
    materials = candidate / ".benchmark-task"
    materials.mkdir()
    for source, name in (
        (task / "R" / "requirement.md", "requirement.md"),
        (task / "B" / "constraints.md", "constraints.md"),
        (task / "B" / "customer-context.md", "customer-context.md"),
    ):
        if source.is_file():
            shutil.copy2(source, materials / name)
    subprocess.run(["git", "init", "-q"], cwd=candidate, check=True)
    subprocess.run(["git", "config", "user.email", "benchmark-agent@example.test"], cwd=candidate, check=True)
    subprocess.run(["git", "config", "user.name", "Benchmark Agent"], cwd=candidate, check=True)
    subprocess.run(["git", "add", "-A"], cwd=candidate, check=True)
    subprocess.run(["git", "commit", "-qm", "baseline"], cwd=candidate, check=True)
    prompt = """Implement this C311 delivery task in the current repository only.

Read `.benchmark-task/requirement.md`, `.benchmark-task/constraints.md`, and
`.benchmark-task/customer-context.md` before editing. The repository is B, the
customer's runnable baseline. Implement R fully, preserve existing behavior and
customer constraints, and run relevant checks. Do not inspect or copy a
reference answer. Do not modify task metadata or evaluator files. Leave the
candidate Docker-buildable when done.
"""
    (work / "agent-prompt.md").write_text(prompt, encoding="utf-8")
    return work, candidate


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("task", type=Path, help="Task directory containing B, R, evaluation, task.yaml")
    parser.add_argument("--cost-limit", type=float, default=3.0)
    args = parser.parse_args()
    task = args.task.resolve()
    tid = task_id(task)
    env = load_env()
    run_name = f"{tid}-{datetime.now(timezone.utc):%Y%m%dT%H%M%SZ}"
    work, candidate = prepare(task, run_name)
    logs = ROOT / "logs" / "swe-agent" / run_name
    logs.mkdir(parents=True)
    sweagent = ROOT / ".venv" / "bin" / "sweagent"
    command = [
        str(sweagent), "run", "--config", "config/default.yaml",
        "--agent.model.name", f"openai/{env['SWE_AGENT_MODEL']}",
        "--agent.model.api_base", env["OPENAI_API_BASE"],
        "--agent.model.api_key", "$OPENAI_API_KEY",
        "--agent.model.completion_kwargs", '{"reasoning_effort":"none"}',
        "--agent.model.per_instance_cost_limit", str(args.cost_limit),
        "--env.repo.type", "local", "--env.repo.path", str(candidate),
        "--problem_statement.path", str(work / "agent-prompt.md"),
        "--output_dir", str(logs), "--actions.apply_patch_locally", "true",
    ]
    with (logs / "runner.log").open("w", encoding="utf-8") as stream:
        agent = subprocess.run(command, cwd=ROOT / "tools" / "swe-agent", env=env, stdout=stream, stderr=subprocess.STDOUT)
    with (logs / "evaluation.log").open("w", encoding="utf-8") as stream:
        evaluation = subprocess.run([str(task / "evaluation" / "run.sh"), str(candidate)], env=env, stdout=stream, stderr=subprocess.STDOUT)
    result = {"agent": "swe-agent", "task_id": tid, "candidate": str(candidate), "agent_exit": agent.returncode, "evaluation_exit": evaluation.returncode, "status": "pass" if evaluation.returncode == 0 else "fail", "logs": str(logs)}
    print(json.dumps(result, ensure_ascii=False))
    return evaluation.returncode


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except RuntimeError as exc:
        print(json.dumps({"status": "configuration_error", "error": str(exc)}), file=sys.stderr)
        raise SystemExit(2)
