#!/usr/bin/env bash

set -euo pipefail

BASE_URL="${WORKORCH_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
TMP_DIR="${WORKORCH_VERIFY_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_VERIFY_KEEP_ARTIFACTS:-0}"

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "workorch HTTP verification failed for deployed workorch service $BASE_URL" >&2
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
grep '当前运行' "$TMP_DIR/workorch-console.js" >/dev/null
grep '补充决策信息' "$TMP_DIR/workorch-console.js" >/dev/null
grep '提交并转入后台' "$TMP_DIR/workorch-console.js" >/dev/null
grep '审批确认' "$TMP_DIR/workorch-console.js" >/dev/null
if grep -E 'workorch management overview|Create run|Run slice|Available slice catalog' "$TMP_DIR/workorch-console.js" >/dev/null; then
  echo "console exposes retired web entries" >&2
  exit 1
fi

curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
grep '"configuration"' "$TMP_DIR/configuration.json" >/dev/null

curl -fsS "$BASE_URL/api/v1/workorch/workspaces" >"$TMP_DIR/workspaces.json"
grep '"workspaces"' "$TMP_DIR/workspaces.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs" >"$TMP_DIR/runs.json"
grep '"runs"' "$TMP_DIR/runs.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/events?limit=20" >"$TMP_DIR/events.json"
grep '"eventRecords"' "$TMP_DIR/events.json" >/dev/null

echo "workorch HTTP verification passed for deployed workorch service $BASE_URL"
