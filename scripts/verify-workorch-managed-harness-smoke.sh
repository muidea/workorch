#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="${WORKORCH_MANAGED_SMOKE_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_MANAGED_SMOKE_KEEP_ARTIFACTS:-0}"
ITERATIONS="${WORKORCH_MANAGED_SMOKE_ITERATIONS:-2}"
VERIFY_TARGET="${WORKORCH_MANAGED_SMOKE_VERIFY_TARGET:-verify-service-file}"

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "managed WorkOrch harness smoke failed" >&2
  echo "artifacts=$TMP_DIR" >&2
  find "$TMP_DIR" -maxdepth 2 -type f -name 'managed-runtime-state.json' -print -exec cat {} \; >&2 || true
  find "$TMP_DIR" -maxdepth 2 -type f -name 'workorch-server.log' -print -exec tail -n 120 {} \; >&2 || true
}

cleanup() {
  local status=$?
  if [[ "$KEEP_ARTIFACTS" != "1" && $status -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  else
    echo "managed harness smoke artifacts retained at $TMP_DIR" >&2
  fi
}

assert_runtime_stopped() {
  local state_file="$1"
  python3 - "$state_file" <<'PY'
import json
import os
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    state = json.load(handle)
errors = []
if state.get("state") != "stopped":
    errors.append(f"state={state.get('state')!r}, want stopped")
pid = state.get("pid")
if isinstance(pid, int) and pid > 0 and os.path.exists(f"/proc/{pid}"):
    errors.append(f"managed server pid {pid} is still running")
if errors:
    for error in errors:
        print(error, file=sys.stderr)
    raise SystemExit(1)
PY
}

trap report_failure ERR
trap cleanup EXIT

if ! [[ "$ITERATIONS" =~ ^[0-9]+$ ]] || (( ITERATIONS <= 0 )); then
  echo "WORKORCH_MANAGED_SMOKE_ITERATIONS must be a positive integer" >&2
  exit 1
fi

mkdir -p "$TMP_DIR"
cd "$ROOT_DIR"

for ((idx = 1; idx <= ITERATIONS; idx++)); do
  run_dir="$TMP_DIR/run-$idx"
  echo "==> managed harness smoke iteration $idx/$ITERATIONS target=$VERIFY_TARGET"
  WORKORCH_MANAGED_VERIFY_ARTIFACT_DIR="$run_dir" \
    WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 \
    WORKORCH_MANAGED_SKIP_PREFLIGHT=1 \
    WORKORCH_MANAGED_VERIFY_TARGET="$VERIFY_TARGET" \
    ./scripts/verify-workorch-managed-full.sh
  assert_runtime_stopped "$run_dir/managed-runtime-state.json"
done

echo "managed WorkOrch harness smoke passed iterations=$ITERATIONS target=$VERIFY_TARGET"
