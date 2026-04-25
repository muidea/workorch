#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
BIN_PATH="$TMP_DIR/workorchd"
ADDR="${WORKORCH_VERIFY_ADDR:-127.0.0.1:18080}"
BASE_URL="http://$ADDR"
BACKEND="${WORKORCH_VERIFY_BACKEND:-memory}"
DATA_DIR="${WORKORCH_VERIFY_DATA_DIR:-$TMP_DIR/workorch-data}"
PID=""
LOG_PATH="$TMP_DIR/workorchd.log"

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "workorchd HTTP verification failed for backend=$BACKEND" >&2
  if [[ -f "$LOG_PATH" ]]; then
    echo "--- workorchd.log ---" >&2
    tail -n 200 "$LOG_PATH" >&2 || true
  fi
}

cleanup() {
  if [[ -n "$PID" ]]; then
    kill -INT "$PID" >/dev/null 2>&1 || true
    wait "$PID" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR"
}

start_server() {
  WORKORCH_SERVER_ADDR="$ADDR" \
  WORKORCH_REPOSITORY_BACKEND="$BACKEND" \
  WORKORCH_DATA_DIR="$DATA_DIR" \
  "$BIN_PATH" >>"$LOG_PATH" 2>&1 &
  PID=$!
}

stop_server() {
  if [[ -z "$PID" ]]; then
    return
  fi
  kill -INT "$PID" >/dev/null 2>&1 || true
  wait "$PID" >/dev/null 2>&1 || true
  PID=""
}

wait_ready() {
  for _ in $(seq 1 50); do
    if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl -fsS "$BASE_URL/healthz" >/dev/null
}

trap report_failure ERR
trap cleanup EXIT

cd "$ROOT_DIR"
go build -o "$BIN_PATH" ./application/workorchd

mkdir -p "$DATA_DIR"
start_server
wait_ready

curl -fsS "$BASE_URL/healthz" | grep '"status": "ok"' >/dev/null
curl -fsS "$BASE_URL/readyz" | grep '"status": "ready"' >/dev/null
curl -fsS "$BASE_URL/metrics" | grep 'workorch_http_requests_total' >/dev/null
curl -fsS "$BASE_URL/console" | grep 'workorch Console' >/dev/null
curl -fsS "$BASE_URL/console/assets/workorch-console.css" | grep 'workorch-section' >/dev/null
curl -fsS "$BASE_URL/console/assets/workorch-console.js" | grep 'Workspace Board' >/dev/null
curl -fsS "$BASE_URL/console/assets/workorch-console.js" | grep 'Decision Queue' >/dev/null
curl -fsS "$BASE_URL/console/assets/workorch-console.js" | grep 'Workspace Detail' >/dev/null
curl -fsS "$BASE_URL/console/assets/workorch-console.js" | grep 'Start execution' >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
  -d '{"title":"verify workspace","description":"verify workspace description"}' >"$TMP_DIR/workspace.json"
workspace_id="$(sed -n 's/.*"id": "\([^"]*\)".*/\1/p' "$TMP_DIR/workspace.json" | head -n1)"
[[ -n "$workspace_id" ]]
grep '"title": "verify workspace"' "$TMP_DIR/workspace.json" >/dev/null
grep '"description": "verify workspace description"' "$TMP_DIR/workspace.json" >/dev/null
grep "\"workingDirectory\": \".*/workspaces/$workspace_id\"" "$TMP_DIR/workspace.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces" | grep "\"id\": \"$workspace_id\"" >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"verify workorch service delivery"}' >"$TMP_DIR/goal.json"
goal_id="$(sed -n 's/.*"id": "\([^"]*\)".*/\1/p' "$TMP_DIR/goal.json" | head -n1)"
[[ -n "$goal_id" ]]
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["workspace can be created","run can be completed"]}' >"$TMP_DIR/dod.json"
dod_id="$(sed -n 's/.*"id": "\([^"]*\)".*/\1/p' "$TMP_DIR/dod.json" | head -n1)"
[[ -n "$dod_id" ]]
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" | grep "\"id\": \"$goal_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/goals/$goal_id" | grep '"text": "verify workorch service delivery"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" | grep "\"id\": \"$dod_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/dods/$dod_id" | grep '"state": "ready"' >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" -H 'Content-Type: application/json' \
  -d '{"runId":"run-shared-verify","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'"}' | grep '"id": "run-shared-verify"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs" | grep '"id": "run-shared-verify"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs?workspaceId=$workspace_id" | grep '"id": "run-shared-verify"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/run-shared-verify" | grep "\"workspaceId\": \"$workspace_id\"" >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-shared-verify/start" | grep '"state": "completed"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/run-shared-verify/status" | grep '"state": "completed"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" | grep '"id": "run-shared-verify"' >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" -H 'Content-Type: application/json' \
  -d '{}' >"$TMP_DIR/workspace-activation.json"
grep '"activation"' "$TMP_DIR/workspace-activation.json" >/dev/null
grep "\"goalId\": \"$goal_id\"" "$TMP_DIR/workspace-activation.json" >/dev/null
grep "\"doDId\": \"$dod_id\"" "$TMP_DIR/workspace-activation.json" >/dev/null
grep '"state": "completed"' "$TMP_DIR/workspace-activation.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/slices" | grep '"id": "workorch-runnable"' >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs" | grep '"title": "workorch runnable slice"' >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/slices/workorch-runnable/runs" | grep '"id": "workorch-runnable"' >/dev/null

if [[ "$BACKEND" == "file" ]]; then
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" -H 'Content-Type: application/json' \
    -d '{"runId":"run-verify","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'"}' | grep '"id": "run-verify"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-verify/start" | grep '"state": "completed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-verify/snapshot" | grep '"id": "run-verify"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-verify/status" | grep '"state": "completed"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" -H 'Content-Type: application/json' \
    -d '{"runId":"run-runtime-decision-block-verify","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'"}' | grep '"id": "run-runtime-decision-block-verify"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-decision-block-verify/start" >"$TMP_DIR/run-runtime-decision-block-verify.json"
  grep '"state": "blocked_on_decision"' "$TMP_DIR/run-runtime-decision-block-verify.json" >/dev/null
  ticket_id="$(sed -n 's/.*"blockingTicketId": "\(.*\)".*/\1/p' "$TMP_DIR/run-runtime-decision-block-verify.json" | head -n1)"
  [[ -n "$ticket_id" ]]
  curl -fsS "$BASE_URL/api/v1/workorch/decisions" | grep "\"id\": \"$ticket_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_id" | grep '"state": "open"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id" | grep "\"id\": \"$ticket_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?runId=run-runtime-decision-block-verify" | grep "\"id\": \"$ticket_id\"" >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$ticket_id/resolve" -H 'Content-Type: application/json' \
    -d '{"resolutionType":"provide_missing_info"}' | grep '"resolutionType": "provide_missing_info"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-decision-block-verify/resume" | grep '"state": "completed"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" -H 'Content-Type: application/json' \
    -d '{"runId":"run-runtime-decision-block-rejected-verify","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'"}' | grep '"id": "run-runtime-decision-block-rejected-verify"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-decision-block-rejected-verify/start" >"$TMP_DIR/run-runtime-decision-block-rejected-verify.json"
  reject_ticket_id="$(sed -n 's/.*"blockingTicketId": "\(.*\)".*/\1/p' "$TMP_DIR/run-runtime-decision-block-rejected-verify.json" | head -n1)"
  [[ -n "$reject_ticket_id" ]]
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$reject_ticket_id/resolve" -H 'Content-Type: application/json' \
    -d '{"resolutionType":"reject_current_path","resolutionPayload":{"operator":"verify"}}' | grep '"resolutionType": "reject_current_path"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-decision-block-rejected-verify/resume" | grep '"state": "failed"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" -H 'Content-Type: application/json' \
    -d '{"runId":"run-runtime-approval-block-verify","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'"}' | grep '"id": "run-runtime-approval-block-verify"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-approval-block-verify/start" >"$TMP_DIR/run-runtime-approval-block-verify.json"
  grep '"state": "blocked_on_approval"' "$TMP_DIR/run-runtime-approval-block-verify.json" >/dev/null
  approval_id="$(sed -n 's/.*"blockingApprovalId": "\(.*\)".*/\1/p' "$TMP_DIR/run-runtime-approval-block-verify.json" | head -n1)"
  [[ -n "$approval_id" ]]
  curl -fsS "$BASE_URL/api/v1/workorch/approvals" | grep "\"id\": \"$approval_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/approvals/$approval_id" | grep '"state": "pending"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/approvals?workspaceId=$workspace_id" | grep "\"id\": \"$approval_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/approvals?runId=run-runtime-approval-block-verify" | grep "\"id\": \"$approval_id\"" >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/approvals/$approval_id/approve" | grep '"state": "approved"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-approval-block-verify/resume" | grep '"state": "completed"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" -H 'Content-Type: application/json' \
    -d '{"runId":"run-runtime-approval-block-rejected-verify","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'"}' | grep '"id": "run-runtime-approval-block-rejected-verify"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-approval-block-rejected-verify/start" >"$TMP_DIR/run-runtime-approval-block-rejected-verify.json"
  reject_approval_id="$(sed -n 's/.*"blockingApprovalId": "\(.*\)".*/\1/p' "$TMP_DIR/run-runtime-approval-block-rejected-verify.json" | head -n1)"
  [[ -n "$reject_approval_id" ]]
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/approvals/$reject_approval_id/reject" | grep '"state": "rejected"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-approval-block-rejected-verify/resume" | grep '"state": "failed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces" | grep "\"id\": \"$workspace_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id" | grep '"title": "verify workspace"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" | grep "\"id\": \"$workspace_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs" | grep '"id": "run-verify"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs?workspaceId=$workspace_id" | grep '"id": "run-verify"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-verify" | grep "\"workspaceId\": \"$workspace_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/slices/snapshots" | grep '"id": "workorch-runnable"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/slices/statuses" | grep '"id": "workorch-runnable"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/slices/workorch-runnable/status" | grep '"runState": "completed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/slices/workorch-runnable/snapshot" | grep '"id": "workorch-runnable"' >/dev/null
  for name in workspace.json goal.json dod.json run.json step.json ticket.json resolution.json approval.json; do
    [[ -f "$DATA_DIR/$name" ]]
  done

  stop_server
  start_server
  wait_ready

  curl -fsS "$BASE_URL/readyz" | grep '"status": "ready"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id" | grep '"title": "verify workspace"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/goals/$goal_id" | grep '"text": "verify workorch service delivery"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/dods/$dod_id" | grep '"state": "ready"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" | grep '"id": "run-verify"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-verify" | grep '"state": "completed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-verify/snapshot" | grep '"id": "run-verify"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-verify/status" | grep '"state": "completed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-runtime-decision-block-verify" | grep '"state": "completed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-runtime-decision-block-rejected-verify" | grep '"state": "failed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_id" | grep '"state": "resolved"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$reject_ticket_id" | grep '"resolutionType": "reject_current_path"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/approvals/$approval_id" | grep '"state": "approved"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/approvals/$reject_approval_id" | grep '"state": "rejected"' >/dev/null
fi

echo "workorchd HTTP verification passed for backend=$BACKEND"
