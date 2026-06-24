#!/usr/bin/env bash

set -Eeuo pipefail

TMP_DIR="${WORKORCH_DUAL_WORKSPACE_VERIFY_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_TMP="${WORKORCH_DUAL_WORKSPACE_VERIFY_KEEP_ARTIFACTS:-0}"
BASE_URL="${WORKORCH_DUAL_WORKSPACE_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
VERIFY_SUFFIX="${WORKORCH_DUAL_WORKSPACE_VERIFY_SUFFIX:-$$}"
SCRIPT_TIMEOUT_SECONDS="${WORKORCH_DUAL_WORKSPACE_VERIFY_TIMEOUT_SECONDS:-120}"
CURL_CONNECT_TIMEOUT="${WORKORCH_VERIFY_CURL_CONNECT_TIMEOUT_SECONDS:-5}"
CURL_MAX_TIME="${WORKORCH_VERIFY_CURL_MAX_TIME_SECONDS:-30}"
WATCHDOG_PID=""
WORKSPACE_IDS=()

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

start_watchdog() {
  if ! [[ "$SCRIPT_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || [[ "$SCRIPT_TIMEOUT_SECONDS" -le 0 ]]; then
    return
  fi
  local parent_pid="$$"
  (
    sleep "$SCRIPT_TIMEOUT_SECONDS"
    echo "dual workspace resume/replan verification exceeded ${SCRIPT_TIMEOUT_SECONDS}s; terminating" >&2
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
  echo "dual workspace resume/replan verification failed for workorch service $BASE_URL" >&2
  echo "artifacts=$TMP_DIR" >&2
  for file in "$TMP_DIR"/*.json; do
    [[ -f "$file" ]] || continue
    echo "--- $file ---" >&2
    python3 -m json.tool "$file" >&2 || cat "$file" >&2 || true
  done
}

cleanup() {
  local status=$?
  if [[ -n "$WATCHDOG_PID" ]]; then
    kill "$WATCHDOG_PID" >/dev/null 2>&1 || true
    wait "$WATCHDOG_PID" >/dev/null 2>&1 || true
  fi
  for workspace_id in "${WORKSPACE_IDS[@]:-}"; do
    delete_workspace "$workspace_id"
  done
  if [[ "$KEEP_TMP" == "1" || "$KEEP_TMP" == "true" || $status -ne 0 ]]; then
    echo "keeping dual workspace verification temp dir: $TMP_DIR" >&2
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
    if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1 &&
      curl -fsS "$BASE_URL/readyz" >"$TMP_DIR/readyz.json" 2>/dev/null; then
      grep '"status"[[:space:]]*:[[:space:]]*"ready"' "$TMP_DIR/readyz.json" >/dev/null
      return
    fi
    sleep 0.3
  done
  echo "timed out waiting for workorch service at $BASE_URL" >&2
  return 1
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

create_workspace_contract() {
  local idx="$1"
  local payload_file="$TMP_DIR/workspace-$idx-payload.json"
  local workspace_file="$TMP_DIR/workspace-$idx.json"
  local goal_file="$TMP_DIR/goal-$idx.json"
  local dod_file="$TMP_DIR/dod-$idx.json"
  local workspace_dir="$TMP_DIR/deliverable-$idx"
  mkdir -p "$workspace_dir"
  python3 - "$idx" "$workspace_dir" >"$payload_file" <<'PY'
import json
import sys

idx, workspace_dir = sys.argv[1:3]
print(json.dumps({
    "title": f"dual workspace command verification {idx}",
    "description": "verify workspace-scoped concurrent resume and replan commands",
    "deliverableDirectory": workspace_dir,
    "templateId": "generic-governed",
}, ensure_ascii=False))
PY
  post_json "/api/v1/workorch/workspaces" @"$payload_file" "$workspace_file"
  local workspace_id
  workspace_id="$(json_path "$workspace_file" "workspace.id")"
  [[ -n "$workspace_id" ]]
  post_json "/api/v1/workorch/workspaces/$workspace_id/goals" \
    '{"text":"verify workspace-scoped command projection does not cross another workspace"}' \
    "$goal_file"
  post_json "/api/v1/workorch/workspaces/$workspace_id/dods" \
    '{"items":["resume command is accepted for this workspace","replan command is accepted for this workspace"]}' \
    "$dod_file"
  printf '%s %s %s\n' "$workspace_id" "$(json_path "$goal_file" "goal.id")" "$(json_path "$dod_file" "dod.id")"
}

validate_command_response() {
  local path="$1"
  local workspace_id="$2"
  local intent="$3"
  local request_id="$4"
  local reason="$5"
  python3 - "$path" "$workspace_id" "$intent" "$request_id" "$reason" <<'PY'
import json
import sys

path, workspace_id, intent, request_id, reason = sys.argv[1:]
with open(path, "r", encoding="utf-8") as handle:
    data = json.load(handle)
if data.get("accepted") is not True:
    raise SystemExit(f"response accepted=false: {data}")
if data.get("workspaceId") != workspace_id:
    raise SystemExit(f"response workspaceId={data.get('workspaceId')!r}, want {workspace_id!r}: {data}")
if data.get("intent") != intent:
    raise SystemExit(f"response intent={data.get('intent')!r}, want {intent!r}: {data}")
if data.get("requestId") != request_id:
    raise SystemExit(f"response requestId={data.get('requestId')!r}, want {request_id!r}: {data}")
if data.get("reason") != reason:
    raise SystemExit(f"response reason={data.get('reason')!r}, want {reason!r}: {data}")
if not data.get("source"):
    raise SystemExit(f"response source missing: {data}")
PY
}

wait_command_projection() {
  local workspace_id="$1"
  local intent="$2"
  local request_id="$3"
  local decision_id="$4"
  local require_gap="$5"
  local output="$6"
  local deadline=$((SECONDS + 45))
  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$output"; then
      if python3 - "$output" "$workspace_id" "$intent" "$request_id" "$decision_id" "$require_gap" <<'PY'
import json
import sys

path, workspace_id, intent, request_id, decision_id, require_gap = sys.argv[1:]
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
if command.get("intent") != intent:
    raise SystemExit(1)
if command.get("requestId") != request_id:
    raise SystemExit(1)
if decision_id and command.get("decisionId") != decision_id:
    raise SystemExit(1)
if int(command.get("revision") or 0) <= 0:
    raise SystemExit(1)
if require_gap in {"1", "true"}:
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
  echo "timed out waiting for command projection workspace=$workspace_id intent=$intent request=$request_id" >&2
  return 1
}

run_id_from_snapshot() {
  python3 - "$1" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)
snapshot = data.get("snapshot") or {}
workspace = snapshot.get("workspace") or {}
command = snapshot.get("runCommand") or {}
print(workspace.get("activeRunId") or command.get("runId") or "")
PY
}

wait_for_pids() {
  local status=0
  local pid
  for pid in "$@"; do
    if ! wait "$pid"; then
      status=1
    fi
  done
  return "$status"
}

write_summary() {
  python3 - "$TMP_DIR/summary.json" "$BASE_URL" "$VERIFY_SUFFIX" "$@" <<'PY'
import json
import sys

path, base_url, suffix, *items = sys.argv[1:]
records = []
for raw in items:
    workspace_id, run_id, resume_request_id, replan_request_id, decision_id = raw.split("|", 4)
    records.append({
        "workspaceId": workspace_id,
        "runId": run_id,
        "resumeRequestId": resume_request_id,
        "replanRequestId": replan_request_id,
        "decisionId": decision_id,
    })
payload = {
    "service": "workorch",
    "baseUrl": base_url,
    "suffix": suffix,
    "workspaceTotal": len(records),
    "successTotal": len(records),
    "pass": len(records) == 2,
    "workspaces": records,
}
with open(path, "w", encoding="utf-8") as handle:
    json.dump(payload, handle, ensure_ascii=False, indent=2, sort_keys=True)
    handle.write("\n")
print(json.dumps(payload, ensure_ascii=False, sort_keys=True))
PY
}

trap report_failure ERR
trap cleanup EXIT
start_watchdog
mkdir -p "$TMP_DIR"
wait_ready

read -r WORKSPACE_1 GOAL_1 DOD_1 < <(create_workspace_contract 1)
read -r WORKSPACE_2 GOAL_2 DOD_2 < <(create_workspace_contract 2)
WORKSPACE_IDS+=("$WORKSPACE_1" "$WORKSPACE_2")

RESUME_REQUEST_1="s11-2-resume-$VERIFY_SUFFIX-1"
RESUME_REQUEST_2="s11-2-resume-$VERIFY_SUFFIX-2"
RESUME_REASON="s11_2_dual_workspace_resume"

post_json "/api/v1/workorch/workspaces/$WORKSPACE_1/execution-activations" \
  '{"goalId":"'"$GOAL_1"'","doDId":"'"$DOD_1"'","priority":2,"requestId":"'"$RESUME_REQUEST_1"'","reason":"'"$RESUME_REASON"'","intent":"resume","executionContextPatch":{"latestOperatorInput":{"source":"e2e","content":"dual workspace resume one"}}}' \
  "$TMP_DIR/resume-1.json" &
pid_resume_1=$!
post_json "/api/v1/workorch/workspaces/$WORKSPACE_2/execution-activations" \
  '{"goalId":"'"$GOAL_2"'","doDId":"'"$DOD_2"'","priority":2,"requestId":"'"$RESUME_REQUEST_2"'","reason":"'"$RESUME_REASON"'","intent":"resume","executionContextPatch":{"latestOperatorInput":{"source":"e2e","content":"dual workspace resume two"}}}' \
  "$TMP_DIR/resume-2.json" &
pid_resume_2=$!
wait_for_pids "$pid_resume_1" "$pid_resume_2"

validate_command_response "$TMP_DIR/resume-1.json" "$WORKSPACE_1" "resume" "$RESUME_REQUEST_1" "$RESUME_REASON"
validate_command_response "$TMP_DIR/resume-2.json" "$WORKSPACE_2" "resume" "$RESUME_REQUEST_2" "$RESUME_REASON"

wait_command_projection "$WORKSPACE_1" "resume" "$RESUME_REQUEST_1" "" "1" "$TMP_DIR/resume-snapshot-1.json"
wait_command_projection "$WORKSPACE_2" "resume" "$RESUME_REQUEST_2" "" "1" "$TMP_DIR/resume-snapshot-2.json"

RUN_1="$(run_id_from_snapshot "$TMP_DIR/resume-snapshot-1.json")"
RUN_2="$(run_id_from_snapshot "$TMP_DIR/resume-snapshot-2.json")"
if [[ -z "$RUN_1" || -z "$RUN_2" ]]; then
  echo "resume projection did not expose both run ids: run1=${RUN_1:-none} run2=${RUN_2:-none}" >&2
  exit 1
fi

REPLAN_REQUEST_1="s11-2-replan-$VERIFY_SUFFIX-1"
REPLAN_REQUEST_2="s11-2-replan-$VERIFY_SUFFIX-2"
DECISION_1="s11-2-decision-$VERIFY_SUFFIX-1"
DECISION_2="s11-2-decision-$VERIFY_SUFFIX-2"
REPLAN_REASON="s11_2_dual_workspace_replan"

post_json "/api/v1/workorch/workspaces/$WORKSPACE_1/runs/$RUN_1/resume?priority=2" \
  '{"intent":"replan","requestId":"'"$REPLAN_REQUEST_1"'","decisionId":"'"$DECISION_1"'","reason":"'"$REPLAN_REASON"'","executionContextPatch":{"latestOperatorInput":{"source":"e2e","content":"dual workspace replan one"}}}' \
  "$TMP_DIR/replan-1.json" &
pid_replan_1=$!
post_json "/api/v1/workorch/workspaces/$WORKSPACE_2/runs/$RUN_2/resume?priority=2" \
  '{"intent":"replan","requestId":"'"$REPLAN_REQUEST_2"'","decisionId":"'"$DECISION_2"'","reason":"'"$REPLAN_REASON"'","executionContextPatch":{"latestOperatorInput":{"source":"e2e","content":"dual workspace replan two"}}}' \
  "$TMP_DIR/replan-2.json" &
pid_replan_2=$!
wait_for_pids "$pid_replan_1" "$pid_replan_2"

validate_command_response "$TMP_DIR/replan-1.json" "$WORKSPACE_1" "replan" "$REPLAN_REQUEST_1" "$REPLAN_REASON"
validate_command_response "$TMP_DIR/replan-2.json" "$WORKSPACE_2" "replan" "$REPLAN_REQUEST_2" "$REPLAN_REASON"

wait_command_projection "$WORKSPACE_1" "replan" "$REPLAN_REQUEST_1" "$DECISION_1" "0" "$TMP_DIR/replan-snapshot-1.json"
wait_command_projection "$WORKSPACE_2" "replan" "$REPLAN_REQUEST_2" "$DECISION_2" "0" "$TMP_DIR/replan-snapshot-2.json"

write_summary \
  "$WORKSPACE_1|$RUN_1|$RESUME_REQUEST_1|$REPLAN_REQUEST_1|$DECISION_1" \
  "$WORKSPACE_2|$RUN_2|$RESUME_REQUEST_2|$REPLAN_REQUEST_2|$DECISION_2" >/dev/null

echo "dual workspace resume/replan verification passed for $BASE_URL workspaces=$WORKSPACE_1,$WORKSPACE_2"
