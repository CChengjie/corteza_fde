#!/usr/bin/env python3
"""Run local OpenHands on one C311 task, then execute the task evaluator."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid
from datetime import datetime, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parent
ENV_FILE = ROOT / ".env"
SERVER = "http://127.0.0.1:18000"
SESSION_KEY = "c311-local-agent-server"


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
    values.setdefault("OPENHANDS_MODEL", values["MODEL"])
    return values


def api(path: str, method: str = "GET", payload: dict | None = None) -> dict:
    body = None if payload is None else json.dumps(payload).encode()
    request = urllib.request.Request(SERVER + path, data=body, method=method, headers={"Authorization": f"Bearer {SESSION_KEY}", "Content-Type": "application/json"})
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.loads(response.read())


def ensure_server(env: dict[str, str]) -> None:
    try:
        api("/server_info")
    except urllib.error.URLError:
        command = ["docker", "run", "-d", "--name", "c311-openhands", "--restart", "unless-stopped", "-p", "127.0.0.1:18000:8000", "-e", "DO_NOT_TRACK=1", "-e", f"LOCAL_BACKEND_API_KEY={SESSION_KEY}", "-e", f"OH_SECRET_KEY={SESSION_KEY}", "-v", f"{ROOT / '.openhands-state'}:/home/openhands/.openhands", "-v", f"{ROOT}:/projects/agent-evaluation", "ghcr.io/openhands/agent-canvas:1.17.0"]
        subprocess.run(["docker", "rm", "-f", "c311-openhands"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        subprocess.run(command, check=True, env=env, stdout=subprocess.DEVNULL)
        for _ in range(20):
            try:
                api("/server_info")
                break
            except urllib.error.URLError:
                time.sleep(2)
        else:
            raise RuntimeError("OpenHands did not become ready")
    # The host bind mount is owned by the local user, whereas the container has
    # its own UID. This isolated container trusts only the mounted benchmark tree.
    subprocess.run(["docker", "exec", "c311-openhands", "git", "config", "--global", "--add", "safe.directory", "/projects/agent-evaluation/workspaces/openhands/*"], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def task_id(task: Path) -> str:
    for line in (task / "task.yaml").read_text(encoding="utf-8").splitlines():
        if line.startswith("task_id:"):
            return line.split(":", 1)[1].strip()
    raise RuntimeError("task.yaml has no task_id")


def prepare(task: Path, run_name: str) -> Path:
    work = ROOT / "workspaces" / "openhands" / run_name
    candidate = work / "candidate"
    shutil.copytree(task / "B" / "customer-system", candidate)
    materials = candidate / ".benchmark-task"
    materials.mkdir()
    for source, name in ((task / "R" / "requirement.md", "requirement.md"), (task / "B" / "constraints.md", "constraints.md"), (task / "B" / "customer-context.md", "customer-context.md")):
        if source.is_file():
            shutil.copy2(source, materials / name)
    subprocess.run(["git", "init", "-q"], cwd=candidate, check=True)
    subprocess.run(["git", "config", "user.email", "benchmark-agent@example.test"], cwd=candidate, check=True)
    subprocess.run(["git", "config", "user.name", "Benchmark Agent"], cwd=candidate, check=True)
    subprocess.run(["git", "add", "-A"], cwd=candidate, check=True)
    subprocess.run(["git", "commit", "-qm", "baseline"], cwd=candidate, check=True)
    return candidate


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("task", type=Path)
    parser.add_argument("--max-iterations", type=int, default=40)
    parser.add_argument("--timeout", type=int, default=1800)
    args = parser.parse_args()
    task, env = args.task.resolve(), load_env()
    tid = task_id(task)
    run_name = f"{tid}-{datetime.now(timezone.utc):%Y%m%dT%H%M%SZ}"
    candidate = prepare(task, run_name)
    ensure_server(env)
    mounted_candidate = f"/projects/agent-evaluation/workspaces/openhands/{run_name}/candidate"
    message = "Read .benchmark-task/requirement.md, constraints.md, and customer-context.md. Implement R in the current repository, preserve B behavior and constraints, do not access reference answers or evaluator files, and run checks before finishing."
    conversation = api("/api/conversations", "POST", {"workspace": {"kind": "LocalWorkspace", "working_dir": mounted_candidate}, "initial_message": {"role": "user", "content": [{"type": "text", "text": message}]}, "max_iterations": args.max_iterations, "autotitle": False, "agent": {"kind": "Agent", "llm": {"model": env["OPENHANDS_MODEL"], "api_key": env["OPENAI_API_KEY"], "base_url": env["OPENAI_BASE_URL"], "api_mode": "chat", "reasoning_effort": "none", "drop_params": True, "temperature": None, "top_p": None}, "tools": [{"name": "terminal", "params": {}}, {"name": "file_editor", "params": {}}, {"name": "task_tracker", "params": {}}]}})
    cid = conversation["id"]
    api(f"/api/conversations/{cid}/run", "POST")
    deadline, last = time.time() + args.timeout, None
    while time.time() < deadline:
        response = api(f"/api/conversations/{cid}/agent_final_response").get("response")
        if response and response == last:
            break
        last = response
        time.sleep(5)
    logs = ROOT / "logs" / "openhands" / run_name
    logs.mkdir(parents=True)
    (logs / "final-response.txt").write_text(last or "", encoding="utf-8")
    with (logs / "evaluation.log").open("w", encoding="utf-8") as stream:
        evaluation = subprocess.run([str(task / "evaluation" / "run.sh"), str(candidate)], env=env, stdout=stream, stderr=subprocess.STDOUT)
    result = {"agent": "openhands", "task_id": tid, "conversation_id": cid, "candidate": str(candidate), "evaluation_exit": evaluation.returncode, "status": "pass" if evaluation.returncode == 0 else "fail", "logs": str(logs)}
    print(json.dumps(result, ensure_ascii=False))
    return evaluation.returncode


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except RuntimeError as exc:
        print(json.dumps({"status": "configuration_error", "error": str(exc)}), file=sys.stderr)
        raise SystemExit(2)
