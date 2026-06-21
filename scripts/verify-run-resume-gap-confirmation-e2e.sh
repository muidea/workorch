#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
KEEP_TMP="${WORKORCH_RUN_RESUME_VERIFY_KEEP_TMP:-0}"
MANAGE_INSTANCE="${WORKORCH_RUN_RESUME_VERIFY_MANAGE_INSTANCE:-0}"
VERIFY_SUFFIX="${WORKORCH_RUN_RESUME_VERIFY_SUFFIX:-$$}"
SCRIPT_TIMEOUT_SECONDS="${WORKORCH_RUN_RESUME_VERIFY_TIMEOUT_SECONDS:-90}"
CURL_CONNECT_TIMEOUT="${WORKORCH_VERIFY_CURL_CONNECT_TIMEOUT_SECONDS:-5}"
CURL_MAX_TIME="${WORKORCH_VERIFY_CURL_MAX_TIME_SECONDS:-30}"
REQUIRE_REPLAN="${WORKORCH_RUN_RESUME_VERIFY_REQUIRE_REPLAN:-0}"
WATCHDOG_PID=""
SERVER_PID=""
SERVER_LOG="$TMP_DIR/workorch-server.log"
CONFIG_DIR="$TMP_DIR/config"
DATA_DIR="${WORKORCH_RUN_RESUME_VERIFY_DATA_DIR:-$TMP_DIR/.workorch-data}"
BINARY="${WORKORCH_RUN_RESUME_VERIFY_BINARY:-$ROOT_DIR/bin/workorch}"
BUILD_BINARY="${WORKORCH_RUN_RESUME_VERIFY_BUILD:-0}"
SERVER_ADDR="${WORKORCH_RUN_RESUME_VERIFY_ADDR:-}"
BASE_URL="${WORKORCH_RUN_RESUME_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-}}"
WORKSPACE_ID=""

select_addr() {
  if [[ -n "$SERVER_ADDR" ]]; then
    printf '%s\n' "$SERVER_ADDR"
    return
  fi
  python3 - <<'PY'
import socket

with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
    sock.bind(("127.0.0.1", 0))
    print(f"127.0.0.1:{sock.getsockname()[1]}")
PY
}

if [[ "$MANAGE_INSTANCE" == "1" || "$MANAGE_INSTANCE" == "true" ]]; then
  SERVER_ADDR="$(select_addr)"
  BASE_URL="http://$SERVER_ADDR"
elif [[ -z "$BASE_URL" ]]; then
  BASE_URL="http://127.0.0.1:8080"
fi

curl() {
  local status url arg
  url=""
  for arg in "$@"; do
    if [[ "$arg" == http://* || "$arg" == https://* ]]; then
      url="$arg"
    fi
  done
  set +e
  command curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" "$@"
  status=$?
  set -e
  if [[ $status -ne 0 ]]; then
    echo "curl failed status=$status url=${url:-unknown} connect-timeout=${CURL_CONNECT_TIMEOUT}s max-time=${CURL_MAX_TIME}s" >&2
  fi
  return $status
}

json_path() {
  python3 - "$1" "$2" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)
current = data
for part in sys.argv[2].split("."):
    current = current[part]
print(current)
PY
}

start_watchdog() {
  if ! [[ "$SCRIPT_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || [[ "$SCRIPT_TIMEOUT_SECONDS" -le 0 ]]; then
    return
  fi
  local parent_pid="$$"
  (
    sleep "$SCRIPT_TIMEOUT_SECONDS"
    echo "workorch run resume gap confirmation verification exceeded ${SCRIPT_TIMEOUT_SECONDS}s; terminating" >&2
    kill -TERM "$parent_pid" >/dev/null 2>&1 || true
  ) &
  WATCHDOG_PID=$!
}

delete_workspace() {
  local workspace_id="$1"
  if [[ -z "$workspace_id" ]]; then
    return
  fi
  curl -fsS -X DELETE "$BASE_URL/api/v1/workorch/workspaces/$workspace_id?purge=true" >/dev/null 2>&1 || true
}

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "workorch run resume gap confirmation verification failed for deployed workorch service $BASE_URL" >&2
  echo "artifacts=$TMP_DIR" >&2
  if [[ -f "$SERVER_LOG" ]]; then
    echo "--- workorch-server.log ---" >&2
    tail -n 240 "$SERVER_LOG" >&2 || true
  fi
  for file in "$TMP_DIR"/*.json; do
    [[ -f "$file" ]] || continue
    echo "--- $file ---" >&2
    python3 -m json.tool "$file" >&2 || cat "$file" >&2
  done
}

write_managed_config() {
  mkdir -p "$CONFIG_DIR" "$DATA_DIR"
  cat >"$CONFIG_DIR/application.toml" <<EOF
endpointName = "workorch"

[server]
addr = "$SERVER_ADDR"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "180s"
idleTimeout = "60s"
shutdownTimeout = "5s"
maxConcurrentRuns = 2
staleDispatchedStepMinAge = "10m"

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

start_server() {
  if [[ "$BUILD_BINARY" == "1" || "$BUILD_BINARY" == "true" ]]; then
    mkdir -p "$(dirname "$BINARY")"
    (cd "$ROOT_DIR" && go build -o "$BINARY" ./application/workorch)
  fi
  if [[ ! -x "$BINARY" ]]; then
    echo "workorch binary not found or not executable: $BINARY" >&2
    echo "build it first with: go build -o $BINARY ./application/workorch" >&2
    echo "or set WORKORCH_RUN_RESUME_VERIFY_BUILD=1" >&2
    return 1
  fi
  write_managed_config
  "$BINARY" --server -config-dir "$CONFIG_DIR" >>"$SERVER_LOG" 2>&1 &
  SERVER_PID=$!
  wait_ready
  echo "managed workorch service ready at $BASE_URL"
}

restart_server() {
  stop_server
  start_server
}

cleanup() {
  local status=$?
  if [[ -n "$WATCHDOG_PID" ]]; then
    kill "$WATCHDOG_PID" >/dev/null 2>&1 || true
    wait "$WATCHDOG_PID" >/dev/null 2>&1 || true
  fi
  delete_workspace "$WORKSPACE_ID"
  stop_server
  if [[ "$KEEP_TMP" == "1" || "$KEEP_TMP" == "true" || $status -ne 0 ]]; then
    echo "keeping run resume verification temp dir: $TMP_DIR" >&2
  else
    rm -rf "$TMP_DIR"
  fi
}

post_json() {
  local path="$1"
  local body="$2"
  local output="$3"
  local status_file="$output.status"
  local http_status
  set +e
  command curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" \
    -sS -X POST "$BASE_URL$path" \
    -H 'Content-Type: application/json' \
    -d "$body" \
    -w '%{http_code}' \
    -o "$output" >"$status_file"
  local curl_status=$?
  set -e
  http_status="$(tr -d '\r\n' <"$status_file" 2>/dev/null || true)"
  if [[ $curl_status -ne 0 ]]; then
    echo "POST $BASE_URL$path failed curl_status=$curl_status http_status=${http_status:-unknown}" >&2
    return $curl_status
  fi
  if [[ ! "$http_status" =~ ^2[0-9][0-9]$ ]]; then
    echo "POST $BASE_URL$path failed HTTP $http_status" >&2
    if [[ -s "$output" ]]; then
      echo "--- response body: $output ---" >&2
      python3 -m json.tool "$output" >&2 || sed -n '1,200p' "$output" >&2 || true
    fi
    return 1
  fi
}

wait_ready() {
  local deadline=$((SECONDS + 30))
  while (( SECONDS <= deadline )); do
    if command curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -fsS "$BASE_URL/healthz" >/dev/null 2>&1 &&
      command curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -fsS "$BASE_URL/readyz" >"$TMP_DIR/readyz.json" 2>/dev/null; then
      grep '"status"[[:space:]]*:[[:space:]]*"ready"' "$TMP_DIR/readyz.json" >/dev/null
      return
    fi
    sleep 0.2
  done
  echo "timed out waiting for workorch ready at $BASE_URL" >&2
  return 1
}

wait_resume_projection() {
  local workspace_id="$1"
  local request_id="$2"
  local output="$3"
  local deadline=$((SECONDS + 30))
  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$output"; then
      if python3 - "$output" "$workspace_id" "$request_id" <<'PY'
import json
import sys

path, workspace_id, request_id = sys.argv[1:]
with open(path, "r", encoding="utf-8") as handle:
    data = json.load(handle)
projection = data.get("projection") or {}
snapshot = data.get("snapshot") or {}
workspace = snapshot.get("workspace") or {}
command = snapshot.get("runCommand") or {}
gap = snapshot.get("gapBaseline") or {}
if workspace.get("id") != workspace_id:
    raise SystemExit(1)
if projection.get("state") not in {"ready", "stale", "degraded"}:
    raise SystemExit(1)
if command.get("intent") != "resume":
    raise SystemExit(1)
if request_id and command.get("requestId") != request_id:
    raise SystemExit(1)
if int(command.get("revision") or 0) <= 0:
    raise SystemExit(1)
if not gap:
    raise SystemExit(1)
if gap.get("workspaceId") != workspace_id:
    raise SystemExit(f"gap workspace mismatch: {gap}")
if not gap.get("baselineRevision"):
    raise SystemExit(f"gap baseline revision missing: {gap}")
if not (gap.get("todoItems") or gap.get("openCoverageWorkItems") or (gap.get("nextAction") or {}).get("kind")):
    raise SystemExit(f"gap baseline lacks actionable content: {gap}")
PY
      then
        return
      fi
    fi
    sleep 0.3
  done
  echo "timed out waiting for resume projection workspace=$workspace_id request=$request_id" >&2
  return 1
}

wait_replan_projection() {
  local workspace_id="$1"
  local decision_id="$2"
  local output="$3"
  local deadline=$((SECONDS + 30))
  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$output"; then
      if python3 - "$output" "$workspace_id" "$decision_id" <<'PY'
import json
import sys

path, workspace_id, decision_id = sys.argv[1:]
with open(path, "r", encoding="utf-8") as handle:
    data = json.load(handle)
projection = data.get("projection") or {}
snapshot = data.get("snapshot") or {}
workspace = snapshot.get("workspace") or {}
command = snapshot.get("runCommand") or {}
if workspace.get("id") != workspace_id:
    raise SystemExit(1)
if projection.get("state") not in {"ready", "stale", "degraded"}:
    raise SystemExit(1)
if command.get("intent") != "replan":
    raise SystemExit(1)
if decision_id and command.get("decisionId") != decision_id:
    raise SystemExit(1)
if int(command.get("revision") or 0) <= 0:
    raise SystemExit(1)
PY
      then
        return
      fi
    fi
    sleep 0.3
  done
  echo "timed out waiting for replan projection workspace=$workspace_id decision=$decision_id" >&2
  return 1
}

validate_command_response() {
  local path="$1"
  local intent="$2"
  local request_id="$3"
  local reason="$4"
  python3 - "$path" "$intent" "$request_id" "$reason" <<'PY'
import json
import sys

path, intent, request_id, reason = sys.argv[1:]
with open(path, "r", encoding="utf-8") as handle:
    data = json.load(handle)
if data.get("accepted") is not True:
    raise SystemExit(f"response accepted=false: {data}")
if data.get("intent") != intent:
    raise SystemExit(f"response intent={data.get('intent')!r}, want {intent!r}: {data}")
if request_id and data.get("requestId") != request_id:
    raise SystemExit(f"response requestId={data.get('requestId')!r}, want {request_id!r}: {data}")
if reason and data.get("reason") != reason:
    raise SystemExit(f"response reason={data.get('reason')!r}, want {reason!r}: {data}")
if not data.get("source"):
    raise SystemExit(f"response source missing: {data}")
PY
}

open_decision_id_for_workspace() {
  local workspace_id="$1"
  local run_id="$2"
  local output="$3"
  local url="$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id"
  if [[ -n "$run_id" ]]; then
    url="${url}&runId=$run_id"
  fi
  if ! curl -fsS "$url" >"$output"; then
    return 0
  fi
  python3 - "$output" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)
for item in data.get("decisions") or []:
    if item.get("state") == "open" and item.get("id"):
        print(item["id"])
        break
PY
}

validate_no_continue_projection() {
  local path="$1"
  python3 - "$path" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)
snapshot = data.get("snapshot") or {}
command = snapshot.get("runCommand") or {}
progress = snapshot.get("taskProgress") or {}
recovery = progress.get("recovery") or {}
if command.get("intent") == "continue":
    raise SystemExit(f"unexpected continue command projection: {command}")
if recovery.get("mode") == "continue":
    raise SystemExit(f"unexpected continue recovery mode: {recovery}")
if recovery.get("nextAction") in {"continue_run", "continue_validation"}:
    raise SystemExit(f"unexpected continue recovery action: {recovery}")
PY
}

trap report_failure ERR
trap cleanup EXIT
start_watchdog
if [[ "$MANAGE_INSTANCE" == "1" || "$MANAGE_INSTANCE" == "true" ]]; then
  start_server
fi
wait_ready

post_json "/api/v1/workorch/workspaces" \
  '{"title":"run resume gap verification '"$VERIFY_SUFFIX"'","description":"verify resume command projection and gap baseline contract","templateId":"generic-governed"}' \
  "$TMP_DIR/workspace.json"
WORKSPACE_ID="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$WORKSPACE_ID" ]]

post_json "/api/v1/workorch/workspaces/$WORKSPACE_ID/goals" \
  '{"text":"verify resume command creates a server-side gap baseline before execution"}' \
  "$TMP_DIR/goal.json"
GOAL_ID="$(json_path "$TMP_DIR/goal.json" "goal.id")"

post_json "/api/v1/workorch/workspaces/$WORKSPACE_ID/dods" \
  '{"items":["resume command is accepted by workorchd projection","gap baseline is available before execution scheduling"]}' \
  "$TMP_DIR/dod.json"
DOD_ID="$(json_path "$TMP_DIR/dod.json" "dod.id")"

REQUEST_ID="verify-resume-$VERIFY_SUFFIX"
post_json "/api/v1/workorch/workspaces/$WORKSPACE_ID/execution-activations" \
  '{"goalId":"'"$GOAL_ID"'","doDId":"'"$DOD_ID"'","priority":2,"requestId":"'"$REQUEST_ID"'","reason":"e2e_resume","executionContextPatch":{"latestOperatorInput":{"source":"e2e","content":"resume verification"}}}' \
  "$TMP_DIR/resume-activation.json"
grep '"accepted"[[:space:]]*:[[:space:]]*true' "$TMP_DIR/resume-activation.json" >/dev/null
validate_command_response "$TMP_DIR/resume-activation.json" "resume" "$REQUEST_ID" "e2e_resume"

wait_resume_projection "$WORKSPACE_ID" "$REQUEST_ID" "$TMP_DIR/resume-snapshot.json"
validate_no_continue_projection "$TMP_DIR/resume-snapshot.json"

RUN_ID="$(python3 - "$TMP_DIR/resume-snapshot.json" <<'PY'
import json
import sys
with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)
snapshot = data.get("snapshot") or {}
workspace = snapshot.get("workspace") or {}
command = snapshot.get("runCommand") or {}
print(workspace.get("activeRunId") or command.get("runId") or "")
PY
)"

if [[ -n "$RUN_ID" ]]; then
  REPEAT_REQUEST_ID="verify-repeat-resume-$VERIFY_SUFFIX"
  post_json "/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$RUN_ID/resume?priority=2" \
    '{"requestId":"'"$REPEAT_REQUEST_ID"'","reason":"e2e_repeat_resume","executionContextPatch":{"latestOperatorInput":{"source":"e2e","content":"repeat resume verification"}}}' \
    "$TMP_DIR/repeat-resume.json"
  grep '"accepted"[[:space:]]*:[[:space:]]*true' "$TMP_DIR/repeat-resume.json" >/dev/null
  validate_command_response "$TMP_DIR/repeat-resume.json" "resume" "$REPEAT_REQUEST_ID" "e2e_repeat_resume"
  wait_resume_projection "$WORKSPACE_ID" "$REPEAT_REQUEST_ID" "$TMP_DIR/repeat-resume-snapshot.json"
  validate_no_continue_projection "$TMP_DIR/repeat-resume-snapshot.json"
fi

if [[ "$MANAGE_INSTANCE" == "1" || "$MANAGE_INSTANCE" == "true" ]]; then
  restart_server
  RESTART_REQUEST_ID="verify-restart-resume-$VERIFY_SUFFIX"
  post_json "/api/v1/workorch/workspaces/$WORKSPACE_ID/execution-activations" \
    '{"goalId":"'"$GOAL_ID"'","doDId":"'"$DOD_ID"'","priority":2,"requestId":"'"$RESTART_REQUEST_ID"'","reason":"e2e_service_restart_resume","executionContextPatch":{"latestOperatorInput":{"source":"e2e","content":"service restart resume verification"}}}' \
    "$TMP_DIR/restart-resume.json"
  grep '"accepted"[[:space:]]*:[[:space:]]*true' "$TMP_DIR/restart-resume.json" >/dev/null
  validate_command_response "$TMP_DIR/restart-resume.json" "resume" "$RESTART_REQUEST_ID" "e2e_service_restart_resume"
  wait_resume_projection "$WORKSPACE_ID" "$RESTART_REQUEST_ID" "$TMP_DIR/restart-resume-snapshot.json"
  validate_no_continue_projection "$TMP_DIR/restart-resume-snapshot.json"
fi

OPEN_DECISION_ID="$(open_decision_id_for_workspace "$WORKSPACE_ID" "$RUN_ID" "$TMP_DIR/decisions-before-replan.json")"
if [[ -n "$OPEN_DECISION_ID" ]]; then
  post_json "/api/v1/workorch/decisions/$OPEN_DECISION_ID/resolve?priority=2" \
    '{"resolutionType":"replan","resolutionPayload":{"source":"e2e","content":"verify decision replan uses unified workspace run command"}}' \
    "$TMP_DIR/replan-decision.json"
  grep '"resolutionType"[[:space:]]*:[[:space:]]*"replan"' "$TMP_DIR/replan-decision.json" >/dev/null
  wait_replan_projection "$WORKSPACE_ID" "$OPEN_DECISION_ID" "$TMP_DIR/replan-snapshot.json"
  validate_no_continue_projection "$TMP_DIR/replan-snapshot.json"
elif [[ "$REQUIRE_REPLAN" == "1" || "$REQUIRE_REPLAN" == "true" ]]; then
  echo "no open decision found for workspace=$WORKSPACE_ID run=${RUN_ID:-none}; cannot verify required replan path" >&2
  exit 1
else
  echo "no open decision found for workspace=$WORKSPACE_ID run=${RUN_ID:-none}; skipping optional replan path" >&2
fi

echo "workorch run resume gap confirmation verification passed for deployed service $BASE_URL workspace=$WORKSPACE_ID run=${RUN_ID:-none}"
