#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="$ROOT_DIR/web/workorch-console/src"
DIST_ASSETS="$ROOT_DIR/web/consoleassets/dist/assets"
BASE_URL="${WORKORCH_WEB_CONSOLE_VISUAL_BASE_URL:-${WORKORCH_BASE_URL:-}}"
TMP_DIR="${WORKORCH_WEB_CONSOLE_VISUAL_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_WEB_CONSOLE_VISUAL_KEEP_ARTIFACTS:-1}"
RUNTIME_BASE_URL=""
TMP_WORKSPACE_ID=""

cleanup() {
  local status=$?
  if [[ -n "$RUNTIME_BASE_URL" && -n "$TMP_WORKSPACE_ID" ]]; then
    curl -fsS -X DELETE "$RUNTIME_BASE_URL/api/v1/workorch/workspaces/$TMP_WORKSPACE_ID?purge=true" >/dev/null 2>&1 || true
  fi
  if [[ "$KEEP_ARTIFACTS" != "1" && "$KEEP_ARTIFACTS" != "true" && $status -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  else
    echo "web console projection visual artifacts: $TMP_DIR" >&2
  fi
}

trap cleanup EXIT

mkdir -p "$TMP_DIR"

require_file_contains() {
  local file="$1"
  local pattern="$2"
  local label="$3"
  if ! grep -Fq "$pattern" "$file"; then
    echo "web console projection visual contract missing $label in $file" >&2
    echo "expected pattern: $pattern" >&2
    exit 1
  fi
}

forbid_file_contains() {
  local file="$1"
  local pattern="$2"
  local label="$3"
  if grep -Fq "$pattern" "$file"; then
    echo "web console projection visual contract forbids $label in $file" >&2
    echo "forbidden pattern: $pattern" >&2
    exit 1
  fi
}

require_dist_contains() {
  local pattern="$1"
  local label="$2"
  if ! grep -RFq "$pattern" "$DIST_ASSETS"; then
    echo "web console projection visual contract missing $label in built assets" >&2
    echo "expected pattern: $pattern" >&2
    exit 1
  fi
}

require_file_contains "$SRC_DIR/helpers.tsx" "export function projectionStatusDescriptions" "projection status descriptions helper"
require_file_contains "$SRC_DIR/helpers.tsx" "label: '投影状态'" "projection status label"
require_file_contains "$SRC_DIR/helpers.tsx" "label: '最近有效进展'" "latest effective progress label"
require_file_contains "$SRC_DIR/helpers.tsx" "stalled" "stalled heartbeat tag"
require_file_contains "$SRC_DIR/helpers.tsx" "ineffective retry" "ineffective retry heartbeat tag"
require_file_contains "$SRC_DIR/pages/WorkspaceDetailPage.tsx" "ProCard title=\"投影状态\"" "workspace detail projection card"
require_file_contains "$SRC_DIR/pages/WorkspaceDetailPage.tsx" "projectionStatusDescriptions(workspaceProjection)" "workspace detail projection descriptions"
require_file_contains "$SRC_DIR/pages/WorkspaceDetailPage.tsx" "runHeartbeatDescriptions(currentRunHeartbeat)" "workspace detail run heartbeat descriptions"
require_file_contains "$SRC_DIR/pages/WorkspaceDetailPage.tsx" "label: '投影摘要'" "workspace detail projection summary"
require_file_contains "$SRC_DIR/pages/WorkspaceDetailPage.tsx" "workorchApi.readWorkspaceSnapshotProjection(targetWorkspaceId)" "workspace detail reads server workspace projection"
require_file_contains "$SRC_DIR/pages/WorkspaceDetailPage.tsx" "workorchApi.readRunSnapshot(runId, targetWorkspaceId)" "workspace detail reads server run snapshot"
require_file_contains "$SRC_DIR/pages/WorkspaceDetailPage.tsx" "workorchApi.readRunObservability(runId, targetWorkspaceId)" "workspace detail reads server run observability"
require_file_contains "$SRC_DIR/pages/WorkspaceDetailPage.tsx" "expectedWorkspaceContextSeq !== workspaceContextRequestSeq.current" "workspace detail ignores stale run detail response"
require_file_contains "$SRC_DIR/pages/WorkspaceDetailPage.tsx" "targetWorkspaceId !== currentWorkspaceIdRef.current" "workspace detail binds run detail to active workspace"
require_file_contains "$SRC_DIR/pages/WorkspaceBoardPage.tsx" "projectionStatusSummary(projection)" "workspace board projection summary"
require_file_contains "$SRC_DIR/pages/WorkspaceBoardPage.tsx" "/workorchd/workspace/projection/changed" "workspace board projection SSE refresh"
require_file_contains "$SRC_DIR/pages/WorkspaceBoardPage.tsx" "workorchApi.readWorkspaceSnapshotProjection(workspace.id)" "workspace board uses server workspace projection for activation hints"
require_file_contains "$SRC_DIR/api.ts" "readRunSnapshot: async (runId: string, workspaceId: string)" "run snapshot API requires workspace scope"
require_file_contains "$SRC_DIR/api.ts" "runPath(workspaceId, runId, 'snapshot')" "run snapshot API uses workspace-scoped path"
require_file_contains "$SRC_DIR/api.ts" "runCommandBody('resume', workspaceId, runId" "web resume sends canonical resume intent"
require_file_contains "$SRC_DIR/api.ts" "source: 'web'" "web run command carries source"
require_file_contains "$SRC_DIR/api.ts" "reason: 'user_requested'" "web run command carries reason"

forbid_file_contains "$SRC_DIR/pages/WorkspaceDetailPage.tsx" "projectedRunSnapshot" "client-side projected run snapshot state"
forbid_file_contains "$SRC_DIR/pages/WorkspaceDetailPage.tsx" "buildProjectedRunSnapshot" "client-side projected run snapshot builder"
forbid_file_contains "$SRC_DIR/helpers.tsx" "buildProjectedRunSnapshot" "client-side projected run snapshot helper"

if [[ ! -d "$DIST_ASSETS" ]]; then
  echo "web console dist assets not found: $DIST_ASSETS" >&2
  exit 1
fi

require_dist_contains "投影状态" "projection status label"
require_dist_contains "运行活动" "run heartbeat label"
require_dist_contains "最近有效进展" "latest effective progress label"
require_dist_contains "stalled" "stalled heartbeat marker"
require_dist_contains "ineffective retry" "ineffective retry heartbeat marker"
require_dist_contains "/workorchd/workspace/projection/changed" "projection SSE topic"
require_dist_contains "workspace_projection_changed" "projection changed payload marker"

if [[ -n "$BASE_URL" ]]; then
  base_url="${BASE_URL%/}"
  RUNTIME_BASE_URL="$base_url"
  runtime_suffix="${WORKORCH_WEB_CONSOLE_VISUAL_SUFFIX:-$$}"
  curl -fsS -H "Content-Type: application/json" -X POST \
    "$base_url/api/v1/workorch/workspaces" \
    -d '{"title":"Web console projection visual '"$runtime_suffix"'","description":"verify Web Console runtime projection visual contract","templateId":"generic-governed"}' \
    >"$TMP_DIR/web-console-workspace-create.json"
  TMP_WORKSPACE_ID="$(python3 - "$TMP_DIR/web-console-workspace-create.json" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)
workspace_id = ((data.get("workspace") or {}).get("id") or "").strip()
if not workspace_id:
    raise SystemExit(f"workspace id missing from create response: {data}")
print(workspace_id)
PY
)"
  curl -fsS "$base_url/readyz" >"$TMP_DIR/readyz.json"
  curl -fsS "$base_url/api/v1/workorch/workspaces" >"$TMP_DIR/workspaces.json"
  curl -fsS "$base_url/console" >"$TMP_DIR/console.html"
  grep -F "workorch Console" "$TMP_DIR/console.html" >/dev/null
  curl -fsS "$base_url/console/assets/workorch-console.js" >"$TMP_DIR/workorch-console.js"
  cp "$TMP_DIR/workorch-console.js" "$TMP_DIR/workorch-console-all.js"
  grep -oE '[A-Za-z0-9_.-]+-[A-Za-z0-9_-]+\.js' "$TMP_DIR/workorch-console.js" | sort -u >"$TMP_DIR/workorch-console-chunks.txt" || true
  while IFS= read -r chunk; do
    [[ -z "$chunk" ]] && continue
    curl -fsS "$base_url/console/assets/$chunk" >>"$TMP_DIR/workorch-console-all.js"
    printf '\n' >>"$TMP_DIR/workorch-console-all.js"
  done <"$TMP_DIR/workorch-console-chunks.txt"
  for marker in \
    "投影状态" \
    "运行活动" \
    "最近有效进展" \
    "stalled" \
    "ineffective retry" \
    "/workorchd/workspace/projection/changed" \
    "workspace_projection_changed"; do
    grep -F "$marker" "$TMP_DIR/workorch-console-all.js" >/dev/null
  done
  python3 - "$TMP_DIR" "$base_url" "$TMP_WORKSPACE_ID" <<'PY'
import json
import sys
from pathlib import Path

artifact_dir = Path(sys.argv[1])
base_url = sys.argv[2]
created_workspace_id = sys.argv[3]
readyz = json.loads((artifact_dir / "readyz.json").read_text(encoding="utf-8"))
workspaces = json.loads((artifact_dir / "workspaces.json").read_text(encoding="utf-8"))
chunks = [
    line.strip()
    for line in (artifact_dir / "workorch-console-chunks.txt").read_text(encoding="utf-8").splitlines()
    if line.strip()
]
projection = workspaces.get("projection") or {}
workspace_ids = [item.get("id") for item in workspaces.get("workspaces") or []]
summary = {
    "baseUrl": base_url,
    "workspaceId": created_workspace_id,
    "serviceReady": readyz.get("status") == "ready",
    "projectionState": projection.get("state"),
    "workspaceTotal": len(workspace_ids),
    "consoleHTMLBytes": (artifact_dir / "console.html").stat().st_size,
    "consoleBundleBytes": (artifact_dir / "workorch-console-all.js").stat().st_size,
    "chunkCount": len(chunks),
    "markers": [
        "投影状态",
        "运行活动",
        "最近有效进展",
        "stalled",
        "ineffective retry",
        "/workorchd/workspace/projection/changed",
        "workspace_projection_changed",
    ],
}
(artifact_dir / "web-console-runtime-visual-summary.json").write_text(
    json.dumps(summary, ensure_ascii=False, indent=2) + "\n",
    encoding="utf-8",
)
print(json.dumps(summary, ensure_ascii=False, indent=2))
if not summary["serviceReady"]:
    raise SystemExit("service is not ready")
if summary["projectionState"] != "ready":
    raise SystemExit(f"projection state = {summary['projectionState']!r}, want ready")
if created_workspace_id not in workspace_ids:
    raise SystemExit(f"workspace list missing created workspace {created_workspace_id}; ids={workspace_ids}")
PY
fi

echo "web console projection visual contract verification passed"
