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
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
  -d '{"workspaceId":"ws-verify","title":"verify workspace"}' | grep '"id": "ws-verify"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/slices" | grep '"id": "workorch-runnable"' >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs" | grep '"title": "workorch runnable slice"' >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/slices/workorch-runnable/runs" | grep '"id": "workorch-runnable"' >/dev/null

if [[ "$BACKEND" == "file" ]]; then
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/ws-verify/runs" -H 'Content-Type: application/json' \
    -d '{"runId":"run-verify","goalId":"goal-verify","doDId":"dod-verify"}' | grep '"id": "run-verify"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-verify/start" | grep '"state": "completed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-verify/snapshot" | grep '"id": "run-verify"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-verify/status" | grep '"state": "completed"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/ws-verify/runs" -H 'Content-Type: application/json' \
    -d '{"runId":"run-runtime-decision-block-verify"}' | grep '"id": "run-runtime-decision-block-verify"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-decision-block-verify/start" >"$TMP_DIR/run-runtime-decision-block-verify.json"
  grep '"state": "blocked_on_decision"' "$TMP_DIR/run-runtime-decision-block-verify.json" >/dev/null
  ticket_id="$(sed -n 's/.*"blockingTicketId": "\(.*\)".*/\1/p' "$TMP_DIR/run-runtime-decision-block-verify.json" | head -n1)"
  [[ -n "$ticket_id" ]]
  curl -fsS "$BASE_URL/api/v1/workorch/decisions" | grep "\"id\": \"$ticket_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_id" | grep '"state": "open"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=ws-verify" | grep "\"id\": \"$ticket_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?runId=run-runtime-decision-block-verify" | grep "\"id\": \"$ticket_id\"" >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$ticket_id/resolve" | grep '"resolutionType": "approve_path"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-decision-block-verify/resume" | grep '"state": "completed"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/ws-verify/runs" -H 'Content-Type: application/json' \
    -d '{"runId":"run-runtime-decision-block-rejected-verify"}' | grep '"id": "run-runtime-decision-block-rejected-verify"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-decision-block-rejected-verify/start" >"$TMP_DIR/run-runtime-decision-block-rejected-verify.json"
  reject_ticket_id="$(sed -n 's/.*"blockingTicketId": "\(.*\)".*/\1/p' "$TMP_DIR/run-runtime-decision-block-rejected-verify.json" | head -n1)"
  [[ -n "$reject_ticket_id" ]]
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$reject_ticket_id/reject" | grep '"resolutionType": "reject_path"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-decision-block-rejected-verify/resume" | grep '"state": "failed"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/ws-verify/runs" -H 'Content-Type: application/json' \
    -d '{"runId":"run-runtime-approval-block-verify"}' | grep '"id": "run-runtime-approval-block-verify"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-approval-block-verify/start" >"$TMP_DIR/run-runtime-approval-block-verify.json"
  grep '"state": "blocked_on_approval"' "$TMP_DIR/run-runtime-approval-block-verify.json" >/dev/null
  approval_id="$(sed -n 's/.*"blockingApprovalId": "\(.*\)".*/\1/p' "$TMP_DIR/run-runtime-approval-block-verify.json" | head -n1)"
  [[ -n "$approval_id" ]]
  curl -fsS "$BASE_URL/api/v1/workorch/approvals" | grep "\"id\": \"$approval_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/approvals/$approval_id" | grep '"state": "pending"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/approvals?workspaceId=ws-verify" | grep "\"id\": \"$approval_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/approvals?runId=run-runtime-approval-block-verify" | grep "\"id\": \"$approval_id\"" >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/approvals/$approval_id/approve" | grep '"state": "approved"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-approval-block-verify/resume" | grep '"state": "completed"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/ws-verify/runs" -H 'Content-Type: application/json' \
    -d '{"runId":"run-runtime-approval-block-rejected-verify"}' | grep '"id": "run-runtime-approval-block-rejected-verify"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-approval-block-rejected-verify/start" >"$TMP_DIR/run-runtime-approval-block-rejected-verify.json"
  reject_approval_id="$(sed -n 's/.*"blockingApprovalId": "\(.*\)".*/\1/p' "$TMP_DIR/run-runtime-approval-block-rejected-verify.json" | head -n1)"
  [[ -n "$reject_approval_id" ]]
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/approvals/$reject_approval_id/reject" | grep '"state": "rejected"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/run-runtime-approval-block-rejected-verify/resume" | grep '"state": "failed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces" | grep '"id": "ws-verify"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/ws-verify" | grep '"title": "verify workspace"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/ws-verify/snapshot" | grep '"id": "ws-verify"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs" | grep '"id": "run-verify"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs?workspaceId=ws-verify" | grep '"id": "run-verify"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-verify" | grep '"workspaceId": "ws-verify"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/slices/snapshots" | grep '"id": "workorch-runnable"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/slices/statuses" | grep '"id": "workorch-runnable"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/slices/workorch-runnable/status" | grep '"runState": "completed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/slices/workorch-runnable/snapshot" | grep '"id": "workorch-runnable"' >/dev/null
  for name in workspace.json run.json step.json ticket.json resolution.json approval.json; do
    [[ -f "$DATA_DIR/$name" ]]
  done

  stop_server
  start_server
  wait_ready

  curl -fsS "$BASE_URL/readyz" | grep '"status": "ready"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/ws-verify" | grep '"title": "verify workspace"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/ws-verify/snapshot" | grep '"id": "run-verify"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-verify" | grep '"state": "completed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-verify/snapshot" | grep '"id": "run-verify"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-verify/status" | grep '"state": "completed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-runtime-decision-block-verify" | grep '"state": "completed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/run-runtime-decision-block-rejected-verify" | grep '"state": "failed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_id" | grep '"state": "resolved"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$reject_ticket_id" | grep '"resolutionType": "reject_path"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/approvals/$approval_id" | grep '"state": "approved"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/approvals/$reject_approval_id" | grep '"state": "rejected"' >/dev/null
fi

echo "workorchd HTTP verification passed for backend=$BACKEND"
