#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="$ROOT_DIR/web/workorch-console/src"
DIST_ASSETS="$ROOT_DIR/internal/modules/application/workorchd/service/server/consoleassets/dist/assets"
BASE_URL="${WORKORCH_WEB_CONSOLE_VISUAL_BASE_URL:-${WORKORCH_BASE_URL:-}}"
TMP_DIR="${WORKORCH_WEB_CONSOLE_VISUAL_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_WEB_CONSOLE_VISUAL_KEEP_ARTIFACTS:-1}"

cleanup() {
  local status=$?
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
  python3 - "$TMP_DIR" "$base_url" <<'PY'
import json
import sys
from pathlib import Path

artifact_dir = Path(sys.argv[1])
base_url = sys.argv[2]
readyz = json.loads((artifact_dir / "readyz.json").read_text(encoding="utf-8"))
workspaces = json.loads((artifact_dir / "workspaces.json").read_text(encoding="utf-8"))
chunks = [
    line.strip()
    for line in (artifact_dir / "workorch-console-chunks.txt").read_text(encoding="utf-8").splitlines()
    if line.strip()
]
projection = workspaces.get("projection") or {}
summary = {
    "baseUrl": base_url,
    "serviceReady": readyz.get("status") == "ready",
    "projectionState": projection.get("state"),
    "workspaceTotal": len(workspaces.get("workspaces") or []),
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
if summary["workspaceTotal"] <= 0:
    raise SystemExit("workspace list is empty")
PY
fi

echo "web console projection visual contract verification passed"
