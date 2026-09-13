#!/usr/bin/env bash

# Terminate a runner-owned process and every child it spawned.
terminate_process_tree () {
  local pid="${1:-}"
  [[ "$pid" =~ ^[0-9]+$ ]] || return 0

  local child
  while read -r child; do
    terminate_process_tree "$child"
  done < <(pgrep -P "$pid" 2>/dev/null || true)

  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}
