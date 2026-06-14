#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="${WORKORCH_MANAGED_VERIFY_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS:-0}"
SERVER_PID=""
SERVER_LOG="$TMP_DIR/workorch-server.log"
CONFIG_DIR="$TMP_DIR/config"
DATA_DIR="$TMP_DIR/.workorch-data"
BINARY="${WORKORCH_MANAGED_VERIFY_BINARY:-$ROOT_DIR/bin/workorch}"
GO_TEST_PACKAGES="${WORKORCH_MANAGED_GO_TEST_PACKAGES:-./...}"
GO_TEST_FLAGS="${WORKORCH_MANAGED_GO_TEST_FLAGS:--count=1}"
VERIFY_TARGET="${WORKORCH_MANAGED_VERIFY_TARGET:-verify-e2e-full}"
MAX_CONCURRENT_RUNS="${WORKORCH_MANAGED_MAX_CONCURRENT_RUNS:-1}"
STALE_DISPATCHED_STEP_MIN_AGE="${WORKORCH_MANAGED_STALE_DISPATCHED_STEP_MIN_AGE:-10m}"
LLM_CONFIG_JSON="${WORKORCH_MANAGED_LLM_CONFIG_JSON:-}"

select_addr() {
  if [[ -n "${WORKORCH_MANAGED_ADDR:-}" ]]; then
    printf '%s\n' "$WORKORCH_MANAGED_ADDR"
    return
  fi
  python3 - <<'PY'
import socket

with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
    sock.bind(("127.0.0.1", 0))
    print(f"127.0.0.1:{sock.getsockname()[1]}")
PY
}

ADDR="$(select_addr)"
BASE_URL="http://$ADDR"

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "managed WorkOrch verification failed" >&2
  echo "baseUrl=$BASE_URL" >&2
  echo "artifacts=$TMP_DIR" >&2
  if [[ -f "$SERVER_LOG" ]]; then
    echo "--- workorch server log ---" >&2
    tail -n 240 "$SERVER_LOG" >&2 || true
  fi
}

stop_server() {
  if [[ -z "$SERVER_PID" ]]; then
    return
  fi
  if ! kill -0 "$SERVER_PID" >/dev/null 2>&1; then
    wait "$SERVER_PID" >/dev/null 2>&1 || true
    SERVER_PID=""
    return
  fi
  kill -INT "$SERVER_PID" >/dev/null 2>&1 || true
  local deadline=$((SECONDS + 15))
  while kill -0 "$SERVER_PID" >/dev/null 2>&1; do
    if (( SECONDS >= deadline )); then
      kill -KILL "$SERVER_PID" >/dev/null 2>&1 || true
      break
    fi
    sleep 0.2
  done
  wait "$SERVER_PID" >/dev/null 2>&1 || true
  SERVER_PID=""
}

cleanup() {
  local status=$?
  stop_server
  if [[ "$KEEP_ARTIFACTS" != "1" && $status -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  else
    echo "managed verification artifacts retained at $TMP_DIR" >&2
  fi
}

run_step() {
  echo "==> $*"
  "$@"
}

write_config() {
  mkdir -p "$CONFIG_DIR" "$DATA_DIR"
  cat >"$CONFIG_DIR/application.toml" <<EOF
endpointName = "workorch"

[server]
addr = "$ADDR"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "180s"
idleTimeout = "60s"
shutdownTimeout = "5s"
maxConcurrentRuns = $MAX_CONCURRENT_RUNS
staleDispatchedStepMinAge = "$STALE_DISPATCHED_STEP_MIN_AGE"

[repository]
backend = "file"
dataDir = "$DATA_DIR"

[runtime.git]
defaultTimeout = "30s"

[llm]
timeout = "60s"
disabled = false
EOF
}

start_server() {
  write_config
  "$BINARY" --server -config-dir "$CONFIG_DIR" >"$SERVER_LOG" 2>&1 &
  SERVER_PID=$!
  local deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1 &&
      curl -fsS "$BASE_URL/readyz" >/dev/null 2>&1; then
      echo "managed workorch service ready at $BASE_URL"
      return
    fi
    if ! kill -0 "$SERVER_PID" >/dev/null 2>&1; then
      echo "workorch server exited before becoming ready" >&2
      return 1
    fi
    sleep 0.2
  done
  echo "timed out waiting for workorch server at $BASE_URL" >&2
  return 1
}

apply_managed_llm_config() {
  if [[ -z "$LLM_CONFIG_JSON" ]]; then
    return
  fi
  if [[ ! -f "$LLM_CONFIG_JSON" ]]; then
    echo "WORKORCH_MANAGED_LLM_CONFIG_JSON file not found: $LLM_CONFIG_JSON" >&2
    return 1
  fi
  echo "==> apply managed LLM configuration from $LLM_CONFIG_JSON"
  local response_file="$TMP_DIR/configuration-llm-managed.json"
  local status_file="$TMP_DIR/configuration-llm-managed.status"
  local http_status
  http_status="$(
    curl -sS --max-time 30 \
      -X PUT \
      -H "Content-Type: application/json" \
      -d @"$LLM_CONFIG_JSON" \
      -o "$response_file" \
      -w "%{http_code}" \
      "$BASE_URL/api/v1/workorch/configuration/llm"
  )"
  printf '%s\n' "$http_status" >"$status_file"
  if [[ "$http_status" -lt 200 || "$http_status" -ge 300 ]]; then
    echo "managed LLM configuration update failed with HTTP $http_status" >&2
    if [[ -f "$response_file" ]]; then
      cat "$response_file" >&2 || true
    fi
    return 1
  fi
}

trap report_failure ERR
trap cleanup EXIT

cd "$ROOT_DIR"

mkdir -p "$TMP_DIR" "$(dirname "$BINARY")"

run_step make fmt-check
run_step make console-build
run_step make vet
run_step go test $GO_TEST_FLAGS $GO_TEST_PACKAGES
run_step go build -o "$BINARY" ./application/workorch

start_server
apply_managed_llm_config

export WORKORCH_BASE_URL="$BASE_URL"
export WORKORCH_VERIFY_BASE_URL="${WORKORCH_VERIFY_BASE_URL:-$BASE_URL}"
export WORKORCH_OBSERVE_ARTIFACT_DIR="${WORKORCH_OBSERVE_ARTIFACT_DIR:-$TMP_DIR/observe-runtime-closure}"
export WORKORCH_OBSERVE_KEEP_ARTIFACTS="${WORKORCH_OBSERVE_KEEP_ARTIFACTS:-$KEEP_ARTIFACTS}"
export WORKORCH_REAL_RUNTIME_ARTIFACT_DIR="${WORKORCH_REAL_RUNTIME_ARTIFACT_DIR:-$TMP_DIR/verify-real-runtime-closure}"
export WORKORCH_REAL_RUNTIME_KEEP_ARTIFACTS="${WORKORCH_REAL_RUNTIME_KEEP_ARTIFACTS:-$KEEP_ARTIFACTS}"
export WORKORCH_TUI_LONG_OUTPUT_ARTIFACT_DIR="${WORKORCH_TUI_LONG_OUTPUT_ARTIFACT_DIR:-$TMP_DIR/verify-tui-long-output}"
export WORKORCH_TUI_LONG_OUTPUT_KEEP_ARTIFACTS="${WORKORCH_TUI_LONG_OUTPUT_KEEP_ARTIFACTS:-$KEEP_ARTIFACTS}"

run_step make "$VERIFY_TARGET"

echo "managed WorkOrch verification passed for $BASE_URL target=$VERIFY_TARGET"
