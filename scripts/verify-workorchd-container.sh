#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ADDR_PORT="${WORKORCH_VERIFY_PORT:-18081}"
BASE_URL="http://127.0.0.1:$ADDR_PORT"
IMAGE_TAG="${WORKORCH_VERIFY_IMAGE:-workorchd:verify}"
BACKEND="${WORKORCH_VERIFY_BACKEND:-memory}"
TMP_DIR="$(mktemp -d)"
DATA_DIR="$TMP_DIR/workorch-data"
CID=""
LOG_PATH="$TMP_DIR/container.log"

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "workorchd container verification failed for backend=$BACKEND" >&2
  if [[ -n "$CID" ]]; then
    echo "--- docker logs ---" >&2
    docker logs "$CID" >&2 || true
  fi
}

cleanup() {
  if [[ -n "$CID" ]]; then
    docker rm -f "$CID" >/dev/null 2>&1 || true
  fi
  docker image rm "$IMAGE_TAG" >/dev/null 2>&1 || true
  rm -rf "$TMP_DIR"
}

start_container() {
  local docker_args=(run -d -p "$ADDR_PORT:8080")
  if [[ "$BACKEND" == "file" ]]; then
    docker_args+=(
      -e WORKORCH_REPOSITORY_BACKEND=file
      -e WORKORCH_DATA_DIR=/var/lib/workorch
      -v "$DATA_DIR:/var/lib/workorch"
    )
  fi
  docker_args+=("$IMAGE_TAG")
  CID="$(docker "${docker_args[@]}")"
}

restart_container() {
  docker rm -f "$CID" >/dev/null
  CID=""
  start_container
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
docker build -t "$IMAGE_TAG" .

mkdir -p "$DATA_DIR"
chmod 0777 "$DATA_DIR"

start_container
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

  restart_container
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

echo "workorchd container verification passed for backend=$BACKEND"
