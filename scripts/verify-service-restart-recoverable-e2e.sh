#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="${WORKORCH_SERVICE_RESTART_VERIFY_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_SERVICE_RESTART_VERIFY_KEEP_ARTIFACTS:-0}"
CONFIG_DIR="$TMP_DIR/config"
DATA_DIR="${WORKORCH_SERVICE_RESTART_VERIFY_DATA_DIR:-$TMP_DIR/.workorch-data}"
SERVER_LOG="$TMP_DIR/workorch-server.log"
BINARY="${WORKORCH_SERVICE_RESTART_VERIFY_BINARY:-$ROOT_DIR/bin/workorch}"
VERIFY_SUFFIX="${WORKORCH_SERVICE_RESTART_VERIFY_SUFFIX:-$$}"
WORKSPACE_ID="${WORKORCH_SERVICE_RESTART_VERIFY_WORKSPACE_ID:-ws-service-restart-$VERIFY_SUFFIX}"
RUN_ID="${WORKORCH_SERVICE_RESTART_VERIFY_RUN_ID:-run-service-restart-$VERIFY_SUFFIX}"
CURL_CONNECT_TIMEOUT="${WORKORCH_VERIFY_CURL_CONNECT_TIMEOUT_SECONDS:-5}"
CURL_MAX_TIME="${WORKORCH_VERIFY_CURL_MAX_TIME_SECONDS:-30}"
SERVER_PID=""
SEEDER_FILE=""

select_addr() {
  if [[ -n "${WORKORCH_SERVICE_RESTART_VERIFY_ADDR:-}" ]]; then
    printf '%s\n' "$WORKORCH_SERVICE_RESTART_VERIFY_ADDR"
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

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "service restart recoverable e2e failed" >&2
  echo "baseUrl=$BASE_URL" >&2
  echo "artifacts=$TMP_DIR" >&2
  if [[ -f "$SERVER_LOG" ]]; then
    echo "--- workorch-server.log ---" >&2
    tail -n 240 "$SERVER_LOG" >&2 || true
  fi
  for file in "$TMP_DIR"/*.json; do
    [[ -f "$file" ]] || continue
    echo "--- $file ---" >&2
    python3 -m json.tool "$file" >&2 || cat "$file" >&2 || true
  done
}

stop_server() {
  if [[ -z "$SERVER_PID" ]]; then
    return
  fi
  if kill -0 "$SERVER_PID" >/dev/null 2>&1; then
    kill -INT "$SERVER_PID" >/dev/null 2>&1 || true
    local deadline=$((SECONDS + 15))
    while kill -0 "$SERVER_PID" >/dev/null 2>&1; do
      if (( SECONDS >= deadline )); then
        kill -KILL "$SERVER_PID" >/dev/null 2>&1 || true
        break
      fi
      sleep 0.2
    done
  fi
  wait "$SERVER_PID" >/dev/null 2>&1 || true
  SERVER_PID=""
}

cleanup() {
  local status=$?
  stop_server
  if [[ -n "$SEEDER_FILE" && -f "$SEEDER_FILE" ]]; then
    rm -f "$SEEDER_FILE"
  fi
  if [[ "$KEEP_ARTIFACTS" != "1" && $status -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  else
    echo "service restart recoverable artifacts retained at $TMP_DIR" >&2
  fi
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
maxConcurrentRuns = 1
staleDispatchedStepMinAge = "10m"

[repository]
backend = "file"
dataDir = "$DATA_DIR"

[runtime.git]
defaultTimeout = "30s"

[llm]
timeout = "60s"
disabled = true
EOF
}

build_binary_if_needed() {
  if [[ -x "$BINARY" ]]; then
    return
  fi
  echo "==> go build -o $BINARY ./application/workorch"
  (cd "$ROOT_DIR" && go build -o "$BINARY" ./application/workorch)
}

seed_startup_inactive_running_state() {
  mkdir -p "$TMP_DIR/deliverable"
  SEEDER_FILE="$(mktemp "$ROOT_DIR/service-restart-seed-XXXXXX.go")"
  cat >"$SEEDER_FILE" <<'GO'
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: seed <dataRoot> <workspaceID> <runID> <deliverableDir>")
		os.Exit(2)
	}
	dataRoot, workspaceID, runID, deliverableDir := os.Args[1], os.Args[2], os.Args[3], os.Args[4]
	ctx := context.Background()
	session, err := appservice.NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = session.Close(ctx)
	}()
	exports, err := session.Exports()
	if err != nil {
		panic(err)
	}
	workspace := &workspacemodel.Workspace{
		ID:                   common.WorkspaceID(workspaceID),
		Title:                "service restart recoverable e2e",
		Description:          "verify startup inactive running state resets to idle manual activation",
		DeliverableDirectory: deliverableDir,
		State:                common.WorkspaceStateRunning,
		ActiveRunID:          common.RunID(runID),
		TemplateID:           "generic-governed",
	}
	if err := exports.Repositories.Workspaces.Create(ctx, workspace); err != nil {
		panic(err)
	}
	run := &runmodel.Run{
		ID:          common.RunID(runID),
		WorkspaceID: common.WorkspaceID(workspaceID),
		State:       common.RunStateRunning,
	}
	runCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspace.ID)
	if err := exports.Repositories.Runs.Create(runCtx, run); err != nil {
		panic(err)
	}
}
GO
  (cd "$ROOT_DIR" && go run "$SEEDER_FILE" "$DATA_DIR" "$WORKSPACE_ID" "$RUN_ID" "$TMP_DIR/deliverable")
  rm -f "$SEEDER_FILE"
  SEEDER_FILE=""
}

start_server() {
  "$BINARY" --server -config-dir "$CONFIG_DIR" >"$SERVER_LOG" 2>&1 &
  SERVER_PID=$!
  local deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1 &&
      curl -fsS "$BASE_URL/readyz" >"$TMP_DIR/readyz.json" 2>/dev/null; then
      grep '"status"[[:space:]]*:[[:space:]]*"ready"' "$TMP_DIR/readyz.json" >/dev/null
      return
    fi
    if ! kill -0 "$SERVER_PID" >/dev/null 2>&1; then
      echo "workorch server exited before ready" >&2
      return 1
    fi
    sleep 0.2
  done
  echo "timed out waiting for service at $BASE_URL" >&2
  return 1
}

wait_recovered_projection() {
  local deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/snapshot" >"$TMP_DIR/snapshot.json" &&
      curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$RUN_ID/status" >"$TMP_DIR/run-status.json"; then
      if python3 - "$TMP_DIR/snapshot.json" "$TMP_DIR/run-status.json" "$WORKSPACE_ID" "$RUN_ID" <<'PY'
import json
import sys

snapshot_path, status_path, workspace_id, run_id = sys.argv[1:]
snapshot = json.load(open(snapshot_path, encoding="utf-8")).get("snapshot") or {}
status = json.load(open(status_path, encoding="utf-8")).get("status") or {}
workspace = snapshot.get("workspace") or {}
recovery = ((snapshot.get("taskProgress") or {}).get("recovery") or {})
diagnostic = (((status.get("schedulerDiagnostics") or {}).get("latestContinuation")) or {})
errors = []
if workspace.get("id") != workspace_id:
    errors.append(f"workspace id = {workspace.get('id')!r}, want {workspace_id!r}")
if workspace.get("state") != "active":
    errors.append(f"workspace state = {workspace.get('state')!r}, want active")
if workspace.get("activeRunId"):
    errors.append(f"workspace activeRunId = {workspace.get('activeRunId')!r}, want empty")
if recovery.get("mode") != "idle":
    errors.append(f"recovery mode = {recovery.get('mode')!r}, want idle")
if recovery.get("nextAction") != "start_workspace_execution":
    errors.append(f"recovery nextAction = {recovery.get('nextAction')!r}, want start_workspace_execution")
status_id = status.get("id") or status.get("runId")
if status_id != run_id:
    errors.append(f"run status id = {status_id!r}, want {run_id!r}")
if status.get("state") != "cancelled":
    errors.append(f"run state = {status.get('state')!r}, want cancelled")
if diagnostic.get("state") != "not_scheduled":
    errors.append(f"scheduler diagnostic state = {diagnostic.get('state')!r}, want not_scheduled")
if diagnostic.get("recoverability") != "manual_retry_required":
    errors.append(f"scheduler diagnostic recoverability = {diagnostic.get('recoverability')!r}, want manual_retry_required")
recommended = diagnostic.get("recommendedActions") or []
for action in ("run_resume", "workspace_restart"):
    if action not in recommended:
        errors.append(f"scheduler recommendedActions missing {action!r}: {recommended!r}")
if errors:
    raise SystemExit(1)
PY
      then
        return
      fi
    fi
    sleep 0.3
  done
  echo "timed out waiting for startup inactive running state to become idle/manual activation" >&2
  return 1
}

trap report_failure ERR
trap cleanup EXIT

mkdir -p "$TMP_DIR"
cd "$ROOT_DIR"
write_config
build_binary_if_needed
seed_startup_inactive_running_state
start_server
wait_recovered_projection

echo "service restart recoverable e2e passed baseUrl=$BASE_URL workspace=$WORKSPACE_ID run=$RUN_ID"
