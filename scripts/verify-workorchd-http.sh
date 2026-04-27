#!/usr/bin/env bash

set -euo pipefail

BASE_URL="${WORKORCH_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
TMP_DIR="${WORKORCH_VERIFY_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_VERIFY_KEEP_ARTIFACTS:-0}"
RUN_SUFFIX="${WORKORCH_VERIFY_RUN_SUFFIX:-$$}"

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "workorchd HTTP verification failed for deployed service $BASE_URL" >&2
  echo "artifacts=$TMP_DIR" >&2
  for file in "$TMP_DIR"/*.json; do
    [[ -f "$file" ]] || continue
    echo "--- $file ---" >&2
    python3 -m json.tool "$file" >&2 || cat "$file" >&2
  done
}

cleanup() {
  local status=$?
  if [[ "$KEEP_ARTIFACTS" != "1" && $status -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  fi
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

post_json() {
  local url="$1"
  local body="$2"
  local output="$3"
  curl -fsS -H "Content-Type: application/json" -X POST "$url" -d "$body" >"$output"
}

trap report_failure ERR
trap cleanup EXIT

mkdir -p "$TMP_DIR"

curl -fsS "$BASE_URL/healthz" >"$TMP_DIR/healthz.json"
grep '"status": "ok"' "$TMP_DIR/healthz.json" >/dev/null
curl -fsS "$BASE_URL/readyz" >"$TMP_DIR/readyz.json"
grep '"status": "ready"' "$TMP_DIR/readyz.json" >/dev/null
curl -fsS "$BASE_URL/metrics" >"$TMP_DIR/metrics.txt"
grep 'workorch_http_requests_total' "$TMP_DIR/metrics.txt" >/dev/null

curl -fsS "$BASE_URL/console" | grep 'workorch Console' >/dev/null
curl -fsS "$BASE_URL/console/assets/workorch-console.css" | grep 'workorch-section' >/dev/null
curl -fsS "$BASE_URL/console/assets/workorch-console.js" >"$TMP_DIR/workorch-console.js"
grep 'workorch 控制台' "$TMP_DIR/workorch-console.js" >/dev/null
grep '业务总览' "$TMP_DIR/workorch-console.js" >/dev/null
grep '治理队列' "$TMP_DIR/workorch-console.js" >/dev/null
grep '工作空间详情' "$TMP_DIR/workorch-console.js" >/dev/null
grep '运行控制' "$TMP_DIR/workorch-console.js" >/dev/null
grep '系统配置' "$TMP_DIR/workorch-console.js" >/dev/null
grep '服务状态' "$TMP_DIR/workorch-console.js" >/dev/null
grep '创建工作空间' "$TMP_DIR/workorch-console.js" >/dev/null
grep '定义目标' "$TMP_DIR/workorch-console.js" >/dev/null
grep '定义验收标准' "$TMP_DIR/workorch-console.js" >/dev/null
grep '启动执行' "$TMP_DIR/workorch-console.js" >/dev/null
grep '运行摘要' "$TMP_DIR/workorch-console.js" >/dev/null
grep '最近运行' "$TMP_DIR/workorch-console.js" >/dev/null
grep '补充决策信息' "$TMP_DIR/workorch-console.js" >/dev/null
grep '提交并转入后台' "$TMP_DIR/workorch-console.js" >/dev/null
grep '审批确认' "$TMP_DIR/workorch-console.js" >/dev/null
if grep -E 'workorch management overview|Create run|Run slice|Available slice catalog' "$TMP_DIR/workorch-console.js" >/dev/null; then
  echo "console exposes retired web entries" >&2
  exit 1
fi

curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
grep '"configuration"' "$TMP_DIR/configuration.json" >/dev/null

post_json "$BASE_URL/api/v1/workorch/workspaces" \
  '{"title":"verify workspace '"$RUN_SUFFIX"'","description":"verify workspace description"}' \
  "$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]
grep "\"id\": \"$workspace_id\"" "$TMP_DIR/workspace.json" >/dev/null
grep '"description": "verify workspace description"' "$TMP_DIR/workspace.json" >/dev/null
grep "\"workingDirectory\": \".*/workspaces/$workspace_id\"" "$TMP_DIR/workspace.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces" | grep "\"id\": \"$workspace_id\"" >/dev/null

post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" \
  '{"text":"verify workorch service delivery"}' \
  "$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
[[ -n "$goal_id" ]]

post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" \
  '{"items":["workspace can be created","run can be completed"]}' \
  "$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"
[[ -n "$dod_id" ]]

curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" | grep "\"id\": \"$goal_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/goals/$goal_id" | grep '"text": "verify workorch service delivery"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" | grep "\"id\": \"$dod_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/dods/$dod_id" | grep '"state": "ready"' >/dev/null

run_id="run-shared-verify-$RUN_SUFFIX"
post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" \
  '{"runId":"'"$run_id"'","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'"}' \
  "$TMP_DIR/run.json"
grep "\"id\": \"$run_id\"" "$TMP_DIR/run.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs" | grep "\"id\": \"$run_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs?workspaceId=$workspace_id" | grep "\"id\": \"$run_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id" | grep "\"workspaceId\": \"$workspace_id\"" >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/$run_id/start" >"$TMP_DIR/run-start.json"
grep '"state": "completed"' "$TMP_DIR/run-start.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/status" | grep '"state": "completed"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/observability" >"$TMP_DIR/run-observability.json"
grep '"runtimeCheckpointTotal"' "$TMP_DIR/run-observability.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/snapshot" >"$TMP_DIR/run-snapshot.json"
grep "\"id\": \"$run_id\"" "$TMP_DIR/run-snapshot.json" >/dev/null
grep '"runtimeCheckpoints"' "$TMP_DIR/run-snapshot.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" | grep "\"id\": \"$run_id\"" >/dev/null

patch_work_dir="$TMP_DIR/runtime-patch-workspace"
mkdir -p "$patch_work_dir"
post_json "$BASE_URL/api/v1/workorch/workspaces" \
  '{"title":"verify runtime patch workspace '"$RUN_SUFFIX"'","workingDirectory":"'"$patch_work_dir"'"}' \
  "$TMP_DIR/patch-workspace.json"
patch_workspace_id="$(json_path "$TMP_DIR/patch-workspace.json" "workspace.id")"
[[ -n "$patch_workspace_id" ]]
post_json "$BASE_URL/api/v1/workorch/workspaces/$patch_workspace_id/goals" \
  '{"text":"verify runtime patch"}' \
  "$TMP_DIR/patch-goal.json"
patch_goal_id="$(json_path "$TMP_DIR/patch-goal.json" "goal.id")"
[[ -n "$patch_goal_id" ]]
post_json "$BASE_URL/api/v1/workorch/workspaces/$patch_workspace_id/dods" \
  '{"items":["runtime patch is applied"]}' \
  "$TMP_DIR/patch-dod.json"
patch_dod_id="$(json_path "$TMP_DIR/patch-dod.json" "dod.id")"
[[ -n "$patch_dod_id" ]]
patch_run_id="run-runtime-patch-verify-$RUN_SUFFIX"
post_json "$BASE_URL/api/v1/workorch/workspaces/$patch_workspace_id/runs" \
  '{"runId":"'"$patch_run_id"'","goalId":"'"$patch_goal_id"'","doDId":"'"$patch_dod_id"'"}' \
  "$TMP_DIR/patch-run.json"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/$patch_run_id/start" >"$TMP_DIR/patch-run-start.json"
grep '"state": "completed"' "$TMP_DIR/patch-run-start.json" >/dev/null
[[ -f "$patch_work_dir/workorch-runtime-output.txt" ]]
grep "run=$patch_run_id" "$patch_work_dir/workorch-runtime-output.txt" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$patch_run_id/snapshot" >"$TMP_DIR/patch-run-snapshot.json"
grep '"targetRef": "workorch-runtime-output.txt"' "$TMP_DIR/patch-run-snapshot.json" >/dev/null
grep '"state": "applied"' "$TMP_DIR/patch-run-snapshot.json" >/dev/null

echo "workorchd HTTP verification passed for deployed service $BASE_URL"
