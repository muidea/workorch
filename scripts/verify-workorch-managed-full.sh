#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="${WORKORCH_MANAGED_VERIFY_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS:-0}"
VERBOSE_LOGS="${WORKORCH_MANAGED_VERIFY_VERBOSE_LOGS:-0}"
FAILURE_LOG_TAIL_LINES="${WORKORCH_MANAGED_VERIFY_FAILURE_LOG_TAIL_LINES:-400}"
SERVER_PID=""
SERVER_LOG="$TMP_DIR/workorch-server.log"
STEP_LOG_DIR="$TMP_DIR/step-logs"
STEP_LOG_INDEX=0
CONFIG_DIR="$TMP_DIR/config"
DATA_DIR="$TMP_DIR/.workorch-data"
BINARY="${WORKORCH_MANAGED_VERIFY_BINARY:-$ROOT_DIR/bin/workorch}"
GO_TEST_PACKAGES="${WORKORCH_MANAGED_GO_TEST_PACKAGES:-./...}"
GO_TEST_FLAGS="${WORKORCH_MANAGED_GO_TEST_FLAGS:--count=1}"
# The default managed gate is the Runtime M1 -> M3 queue. The historical
# verify-e2e-full aggregate contains retired V1 fixture protocols and remains
# runnable only when explicitly requested; it is not a Runtime acceptance
# substitute.
VERIFY_TARGET="${WORKORCH_MANAGED_VERIFY_TARGET:-runtime-managed}"
MAX_CONCURRENT_RUNS="${WORKORCH_MANAGED_MAX_CONCURRENT_RUNS:-1}"
STALE_DISPATCHED_STEP_MIN_AGE="${WORKORCH_MANAGED_STALE_DISPATCHED_STEP_MIN_AGE:-10m}"
LLM_CONFIG_JSON="${WORKORCH_MANAGED_LLM_CONFIG_JSON:-}"
PRECREATE_WORKSPACE_COUNT="${WORKORCH_MANAGED_PRECREATE_WORKSPACE_COUNT:-0}"
SKIP_PREFLIGHT="${WORKORCH_MANAGED_SKIP_PREFLIGHT:-0}"
# The managed verifier owns this TOML value before WorkOrch starts. Child
# verification scripts may select catalog models, but never rewrite this
# endpoint through the configuration HTTP API.
AI_PROXY_BASE_URL="${WORKORCH_MANAGED_AI_PROXY_BASE_URL:-http://127.0.0.1:18181/v1}"

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
    write_runtime_state "stopped"
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
  write_runtime_state "stopped"
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
  local label="$*"
  local log_name
  local log_file
  mkdir -p "$STEP_LOG_DIR"
  log_name="$(printf '%s' "$label" | tr -cs '[:alnum:]_.=-' '-' | sed 's/^-//;s/-$//' | cut -c1-120)"
  if [[ -z "$log_name" ]]; then
    log_name="step"
  fi
  STEP_LOG_INDEX=$((STEP_LOG_INDEX + 1))
  log_file="$STEP_LOG_DIR/$(printf '%03d' "$STEP_LOG_INDEX")-$log_name.log"
  echo "==> $label"
  if [[ "$VERBOSE_LOGS" == "1" || "$VERBOSE_LOGS" == "true" ]]; then
    "$@" 2>&1 | tee "$log_file"
    return "${PIPESTATUS[0]}"
  fi
  if "$@" >"$log_file" 2>&1; then
    return 0
  else
    local status=$?
    echo "command failed: $label" >&2
    echo "command log: $log_file" >&2
    if [[ -f "$log_file" ]]; then
      echo "--- command output tail ---" >&2
      tail -n "$FAILURE_LOG_TAIL_LINES" "$log_file" >&2 || true
    fi
    return "$status"
  fi
}

write_runtime_state() {
  local state="$1"
  python3 - "$TMP_DIR/managed-runtime-state.json" "$state" "$BASE_URL" "${SERVER_PID:-}" "$CONFIG_DIR" "$DATA_DIR" "$SERVER_LOG" <<'PY'
import json
import sys

path, state, base_url, pid, config_dir, data_dir, server_log = sys.argv[1:8]
payload = {
    "state": state,
    "baseUrl": base_url,
    "pid": int(pid) if pid.isdigit() else None,
    "configDir": config_dir,
    "dataDir": data_dir,
    "serverLog": server_log,
}
with open(path, "w", encoding="utf-8") as handle:
    json.dump(payload, handle, ensure_ascii=False, indent=2, sort_keys=True)
    handle.write("\n")
PY
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

[llm.aiProxy]
baseURL = "$AI_PROXY_BASE_URL"

[llm.defaultChat]
providerID = "ai-proxy"
# The managed mock catalog exposes verify-model. LLM configuration writes in
# child verifiers are restart-staged and must not be used as a hot-switch.
modelID = "verify-model"
EOF
}

start_server() {
  write_config
  "$BINARY" --server -config-dir "$CONFIG_DIR" >"$SERVER_LOG" 2>&1 &
  SERVER_PID=$!
  write_runtime_state "starting"
  local deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1 &&
      curl -fsS "$BASE_URL/readyz" >/dev/null 2>&1; then
      echo "managed workorch service ready at $BASE_URL"
      write_runtime_state "running"
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

wait_projection_drained() {
  local deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/readyz" >"$TMP_DIR/readyz-projection-wait.json"
    curl -fsS "$BASE_URL/metrics" >"$TMP_DIR/metrics-projection-wait.txt"
    if python3 - "$TMP_DIR/readyz-projection-wait.json" "$TMP_DIR/metrics-projection-wait.txt" <<'PY'
import json
import sys

readyz = json.load(open(sys.argv[1], encoding="utf-8"))
metrics_text = open(sys.argv[2], encoding="utf-8").read()
projection = readyz.get("projection") or {}
state = projection.get("state")
pending = None
for raw in metrics_text.splitlines():
    if raw.startswith("workorch_workspace_projection_pending_rebuilds "):
        try:
            pending = float(raw.split()[1])
        except Exception:
            pending = None
        break
if state in {"ready", "stale", "degraded"} and (pending is None or pending <= 0):
    raise SystemExit(0)
raise SystemExit(1)
PY
    then
      return
    fi
    sleep 0.2
  done
  echo "timed out waiting for workspace projection to drain" >&2
  return 1
}

precreate_observation_workspaces() {
  if ! [[ "$PRECREATE_WORKSPACE_COUNT" =~ ^[0-9]+$ ]] || (( PRECREATE_WORKSPACE_COUNT <= 0 )); then
    return
  fi
  local ids_file="$TMP_DIR/precreated-workspace-ids.txt"
  : >"$ids_file"
  for ((idx = 1; idx <= PRECREATE_WORKSPACE_COUNT; idx++)); do
    local workspace_dir="$TMP_DIR/precreated-workspace-$idx"
    local payload_file="$TMP_DIR/precreated-workspace-$idx-payload.json"
    local response_file="$TMP_DIR/precreated-workspace-$idx.json"
    mkdir -p "$workspace_dir"
    python3 - "$idx" "$workspace_dir" >"$payload_file" <<'PY'
import json
import sys

idx, workspace_dir = sys.argv[1], sys.argv[2]
print(json.dumps({
    "title": f"runtime idle baseline workspace {idx}",
    "description": "workspace precreated before idle baseline observation",
    "deliverableDirectory": workspace_dir,
    "templateId": "generic-governed",
}, ensure_ascii=False))
PY
    curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" \
      -H 'Content-Type: application/json' \
      -d @"$payload_file" >"$response_file"
    python3 - "$response_file" >>"$ids_file" <<'PY'
import json
import sys

body = json.load(open(sys.argv[1], encoding="utf-8"))
workspace_id = ((body.get("workspace") or {}).get("id") or "").strip()
if workspace_id:
    print(workspace_id)
PY
  done
  wait_projection_drained
  local created_csv
  created_csv="$(paste -sd, "$ids_file")"
  if [[ -n "$created_csv" ]]; then
    if [[ -n "${WORKORCH_OBSERVE_WORKSPACE_IDS:-}" ]]; then
      export WORKORCH_OBSERVE_WORKSPACE_IDS="${WORKORCH_OBSERVE_WORKSPACE_IDS},$created_csv"
    else
      export WORKORCH_OBSERVE_WORKSPACE_IDS="$created_csv"
    fi
  fi
}

trap report_failure ERR
trap cleanup EXIT

cd "$ROOT_DIR"

mkdir -p "$TMP_DIR" "$(dirname "$BINARY")"

if [[ "$SKIP_PREFLIGHT" != "1" && "$SKIP_PREFLIGHT" != "true" ]]; then
  run_step make fmt-check
  run_step make console-build
  run_step make vet
  run_step go test $GO_TEST_FLAGS $GO_TEST_PACKAGES
else
  echo "==> skip managed preflight checks"
fi
run_step go build -o "$BINARY" ./application/workorch

start_server
apply_managed_llm_config
precreate_observation_workspaces

export WORKORCH_BASE_URL="$BASE_URL"
export WORKORCH_VERIFY_ISOLATION_STATE_FILE="$TMP_DIR/managed-runtime-state.json"
export WORKORCH_VERIFY_BASE_URL="${WORKORCH_VERIFY_BASE_URL:-$BASE_URL}"
export WORKORCH_OBSERVE_ARTIFACT_DIR="${WORKORCH_OBSERVE_ARTIFACT_DIR:-$TMP_DIR/observe-runtime-closure}"
export WORKORCH_OBSERVE_KEEP_ARTIFACTS="${WORKORCH_OBSERVE_KEEP_ARTIFACTS:-$KEEP_ARTIFACTS}"
export WORKORCH_REAL_RUNTIME_ARTIFACT_DIR="${WORKORCH_REAL_RUNTIME_ARTIFACT_DIR:-$TMP_DIR/verify-real-runtime-closure}"
export WORKORCH_REAL_RUNTIME_KEEP_ARTIFACTS="${WORKORCH_REAL_RUNTIME_KEEP_ARTIFACTS:-$KEEP_ARTIFACTS}"
export WORKORCH_TUI_LONG_OUTPUT_ARTIFACT_DIR="${WORKORCH_TUI_LONG_OUTPUT_ARTIFACT_DIR:-$TMP_DIR/verify-tui-long-output}"
export WORKORCH_TUI_LONG_OUTPUT_KEEP_ARTIFACTS="${WORKORCH_TUI_LONG_OUTPUT_KEEP_ARTIFACTS:-$KEEP_ARTIFACTS}"

run_runtime_managed_targets() {
  run_step make verify-llm
  run_step make verify-mcp-stdio
  run_step make verify-ci-mvp-file
}

if [[ "$VERIFY_TARGET" == "runtime-managed" ]]; then
  run_runtime_managed_targets
else
  run_step make "$VERIFY_TARGET"
fi

echo "managed WorkOrch verification passed for $BASE_URL target=$VERIFY_TARGET"
