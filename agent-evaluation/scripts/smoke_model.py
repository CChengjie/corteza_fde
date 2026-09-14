#!/usr/bin/env python3
"""Send a minimal streaming Responses API request through the model gateway."""

from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
ENV_FILE = ROOT / ".env"


def load_env(path: Path) -> None:
    if not path.is_file():
        raise RuntimeError(f"missing {path}; copy .env.example to .env first")

    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip().strip("\"'")
        if key:
            os.environ.setdefault(key, value)


def main() -> int:
    parser = argparse.ArgumentParser(description="Send a streaming Responses API request.")
    parser.add_argument("prompt", nargs="?", default="hi")
    args = parser.parse_args()

    load_env(ENV_FILE)
    api_key = os.environ.get("OPENAI_API_KEY")
    base_url = os.environ.get("OPENAI_BASE_URL")
    model = os.environ.get("MODEL")
    if not api_key or not base_url or not model:
        raise RuntimeError(".env must define OPENAI_API_KEY, OPENAI_BASE_URL, and MODEL")
    try:
        from openai import OpenAI
    except ImportError as exc:
        raise RuntimeError(
            "the openai package is required; run with "
            "uv run --with 'openai>=2.0' python scripts/smoke_model.py hi"
        ) from exc

    client = OpenAI(
        api_key=api_key,
        base_url=base_url,
        timeout=60,
        max_retries=0,
    )
    payload = {
        "model": model,
        "instructions": "Reply with JSON only.",
        "input": [
            {
                "role": "user",
                "content": [
                    {
                        "type": "input_text",
                        "text": "Reply helpfully to the following user message. "
                        "Return JSON with exactly one string field named message.\n\n"
                        "User message:\n"
                        + args.prompt,
                    }
                ],
            }
        ],
        "text": {"format": {"type": "json_object"}},
        "stream": True,
    }

    content: list[str] = []
    for event in client.responses.create(**payload):
        if event.type == "response.output_text.delta":
            content.append(event.delta)

    result = "".join(content)
    print(result)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except RuntimeError as exc:
        print(f"model smoke test: {exc}", file=sys.stderr)
        raise SystemExit(2) from exc
