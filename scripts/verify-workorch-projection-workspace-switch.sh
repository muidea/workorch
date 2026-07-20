#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_PROJECTION_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-}}"
source "$ROOT_DIR/scripts/lib/verify-isolated-runtime.sh"
workorch_verify_require_isolated_runtime "$BASE_URL" "verify-workorch-projection-workspace-switch"
TMP_DIR="$(mktemp -d)"
KEEP_TMP="${WORKORCH_PROJECTION_VERIFY_KEEP_TMP:-0}"
VERIFY_SUFFIX="${WORKORCH_PROJECTION_VERIFY_SUFFIX:-$$}"
SCRIPT_TIMEOUT_SECONDS="${WORKORCH_PROJECTION_VERIFY_TIMEOUT_SECONDS:-90}"
CURL_CONNECT_TIMEOUT="${WORKORCH_VERIFY_CURL_CONNECT_TIMEOUT_SECONDS:-5}"
CURL_MAX_TIME="${WORKORCH_VERIFY_CURL_MAX_TIME_SECONDS:-30}"
STREAM_PID=""
WATCHDOG_PID=""
WORKSPACE_A=""
WORKSPACE_B=""
EVENTS_FILE="$TMP_DIR/sse-events.jsonl"
STREAM_READY_FILE="$TMP_DIR/sse-ready"
STREAM_LOG="$TMP_DIR/sse-listener.log"

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
    echo "workorch projection workspace switch verification exceeded ${SCRIPT_TIMEOUT_SECONDS}s; terminating" >&2
    kill -TERM "$parent_pid" >/dev/null 2>&1 || true
  ) &
  WATCHDOG_PID=$!
}

stop_stream() {
  if [[ -n "$STREAM_PID" ]]; then
    kill "$STREAM_PID" >/dev/null 2>&1 || true
    wait "$STREAM_PID" >/dev/null 2>&1 || true
    STREAM_PID=""
  fi
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
  echo "workorch projection workspace switch verification failed for deployed workorch service $BASE_URL" >&2
  echo "artifacts=$TMP_DIR" >&2
  if [[ -f "$STREAM_LOG" ]]; then
    echo "--- sse-listener.log ---" >&2
    cat "$STREAM_LOG" >&2 || true
  fi
  if [[ -f "$EVENTS_FILE" ]]; then
    echo "--- captured projection events ---" >&2
    tail -n 80 "$EVENTS_FILE" >&2 || true
  fi
  for file in "$TMP_DIR"/*.json; do
    [[ -f "$file" ]] || continue
    echo "--- $file ---" >&2
    python3 -m json.tool "$file" >&2 || cat "$file" >&2
  done
}

cleanup() {
  local status=$?
  stop_stream
  if [[ -n "$WATCHDOG_PID" ]]; then
    kill "$WATCHDOG_PID" >/dev/null 2>&1 || true
    wait "$WATCHDOG_PID" >/dev/null 2>&1 || true
  fi
  delete_workspace "$WORKSPACE_A"
  delete_workspace "$WORKSPACE_B"
  if [[ "$KEEP_TMP" == "1" || "$KEEP_TMP" == "true" || $status -ne 0 ]]; then
    echo "keeping projection verification temp dir: $TMP_DIR" >&2
  else
    rm -rf "$TMP_DIR"
  fi
}

wait_ready() {
  local deadline=$((SECONDS + 30))
  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/healthz" >/dev/null && curl -fsS "$BASE_URL/readyz" >"$TMP_DIR/readyz.json"; then
      grep '"status"[[:space:]]*:[[:space:]]*"ready"' "$TMP_DIR/readyz.json" >/dev/null
      return
    fi
    sleep 0.2
  done
  echo "timed out waiting for workorch ready at $BASE_URL" >&2
  return 1
}

start_sse_listener() {
  : >"$EVENTS_FILE"
  python3 - "$BASE_URL/api/v1/workorch/events/stream" "$EVENTS_FILE" "$STREAM_READY_FILE" >"$STREAM_LOG" 2>&1 <<'PY' &
import json
import pathlib
import sys
import urllib.request

stream_url, events_path, ready_path = sys.argv[1:]
request = urllib.request.Request(stream_url, headers={"Accept": "text/event-stream"})
with urllib.request.urlopen(request, timeout=60) as response:
    pathlib.Path(ready_path).write_text("ready\n", encoding="utf-8")
    for raw_line in response:
        line = raw_line.decode("utf-8", errors="replace").strip()
        if not line.startswith("data:"):
            continue
        payload = line[5:].strip()
        if not payload:
            continue
        try:
            item = json.loads(payload)
        except json.JSONDecodeError:
            continue
        with open(events_path, "a", encoding="utf-8") as handle:
            handle.write(json.dumps(item, ensure_ascii=False, sort_keys=True) + "\n")
            handle.flush()
PY
  STREAM_PID=$!
  local deadline=$((SECONDS + 10))
  while (( SECONDS <= deadline )); do
    if [[ -f "$STREAM_READY_FILE" ]]; then
      return
    fi
    if ! kill -0 "$STREAM_PID" >/dev/null 2>&1; then
      echo "SSE listener exited before becoming ready" >&2
      return 1
    fi
    sleep 0.1
  done
  echo "timed out waiting for SSE listener" >&2
  return 1
}

post_json() {
  local path="$1"
  local body="$2"
  local output="$3"
  curl -fsS -X POST "$BASE_URL$path" -H 'Content-Type: application/json' -d "$body" >"$output"
}

create_workspace() {
  local name="$1"
  local output="$2"
  post_json "/api/v1/workorch/workspaces" \
    '{"title":"projection switch '"$name"' '"$VERIFY_SUFFIX"'","description":"verify projection scoped events and workspace switch refresh contract","templateId":"generic-governed"}' \
    "$output"
  json_path "$output" "workspace.id"
}

define_goal_dod() {
  local workspace_id="$1"
  local label="$2"
  post_json "/api/v1/workorch/workspaces/$workspace_id/goals" \
    '{"text":"verify projection scoped refresh for '"$label"' workspace"}' \
    "$TMP_DIR/goal-$label.json"
  post_json "/api/v1/workorch/workspaces/$workspace_id/dods" \
    '{"items":["workspace projection detail refreshes for '"$label"'"]}' \
    "$TMP_DIR/dod-$label.json"
}

wait_workspace_snapshot_projection() {
  local workspace_id="$1"
  local label="$2"
  local output="$TMP_DIR/snapshot-$label.json"
  local deadline=$((SECONDS + 30))
  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$output"; then
      if python3 - "$output" "$workspace_id" <<'PY'
import json
import sys

path, workspace_id = sys.argv[1:]
with open(path, "r", encoding="utf-8") as handle:
    data = json.load(handle)
projection = data.get("projection") or {}
snapshot = data.get("snapshot") or {}
workspace = snapshot.get("workspace") or {}
state = projection.get("state")
if workspace.get("id") != workspace_id:
    raise SystemExit(1)
if state not in {"ready", "stale", "degraded"}:
    raise SystemExit(1)
workspace_revision = int(projection.get("workspaceRevision") or 0)
source_revision = int(projection.get("sourceEventRevision") or 0)
projected_revision = int(projection.get("projectedSourceRevision") or 0)
if workspace_revision <= 0:
    raise SystemExit(1)
if source_revision and projected_revision < source_revision:
    raise SystemExit(f"projected source revision is behind source revision: {projection}")
if not projection.get("updatedAt") or not projection.get("rebuiltAt"):
    raise SystemExit(f"projection timing metadata missing: {projection}")
PY
      then
        return
      fi
    fi
    sleep 0.3
  done
  echo "timed out waiting for workspace projection snapshot workspace=$workspace_id label=$label" >&2
  return 1
}

validate_workspace_list_projection() {
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces" >"$TMP_DIR/workspaces.json"
  python3 - "$TMP_DIR/workspaces.json" "$WORKSPACE_A" "$WORKSPACE_B" <<'PY'
import json
import sys

path, ws_a, ws_b = sys.argv[1:]
with open(path, "r", encoding="utf-8") as handle:
    data = json.load(handle)
projection = data.get("projection") or {}
workspaces = data.get("workspaces") or []
ids = {item.get("id") for item in workspaces}
missing = [item for item in (ws_a, ws_b) if item not in ids]
if missing:
    raise SystemExit(f"workspace list missing {missing}; ids={sorted(ids)}")
if projection.get("state") not in {"ready", "stale", "degraded"}:
    raise SystemExit(f"unexpected projection state: {projection}")
if int(projection.get("workspaceTotal") or 0) < 2:
    raise SystemExit(f"workspaceTotal too small: {projection}")
if "projectedSourceRevision" not in projection:
    raise SystemExit(f"projectedSourceRevision missing: {projection}")
if not projection.get("updatedAt") or not projection.get("rebuiltAt"):
    raise SystemExit(f"list projection timing metadata missing: {projection}")
PY
}

wait_projection_events() {
  local deadline=$((SECONDS + 30))
  while (( SECONDS <= deadline )); do
    if python3 - "$EVENTS_FILE" "$WORKSPACE_A" "$WORKSPACE_B" <<'PY'
import json
import pathlib
import sys

path, ws_a, ws_b = sys.argv[1:]
seen = {ws_a: False, ws_b: False}
topic = "/workorchd/workspace/projection/changed"
if not pathlib.Path(path).exists():
    raise SystemExit(1)
with open(path, "r", encoding="utf-8") as handle:
    for line in handle:
        if not line.strip():
            continue
        item = json.loads(line)
        if item.get("topic") != topic:
            continue
        payload = item.get("payload") or {}
        workspace_id = item.get("workspaceId") or payload.get("workspaceId") or ""
        if workspace_id in seen:
            if payload.get("changeKind") != "detail":
                continue
            state = payload.get("projectionState")
            if state not in {"ready", "stale", "degraded"}:
                raise SystemExit(f"unexpected projectionState for {workspace_id}: {payload}")
            required = ["globalRevision", "workspaceRevision", "sourceEventRevision", "projectedSourceRevision", "updatedAt", "rebuiltAt"]
            missing = [key for key in required if key not in payload]
            if missing:
                raise SystemExit(f"projection event missing {missing} for {workspace_id}: {payload}")
            if int(payload.get("globalRevision") or 0) <= 0:
                raise SystemExit(f"globalRevision not positive for {workspace_id}: {payload}")
            if int(payload.get("workspaceRevision") or 0) <= 0:
                raise SystemExit(f"workspaceRevision not positive for {workspace_id}: {payload}")
            source_revision = int(payload.get("sourceEventRevision") or 0)
            projected_revision = int(payload.get("projectedSourceRevision") or 0)
            if projected_revision < source_revision:
                raise SystemExit(f"projected revision behind source revision for {workspace_id}: {payload}")
            if not str(payload.get("updatedAt") or "").strip() or not str(payload.get("rebuiltAt") or "").strip():
                raise SystemExit(f"projection event timing metadata missing for {workspace_id}: {payload}")
            seen[workspace_id] = True
if not all(seen.values()):
    raise SystemExit(1)
PY
    then
      return
    fi
    sleep 0.2
  done
  echo "timed out waiting for workspace-scoped projection events for $WORKSPACE_A and $WORKSPACE_B" >&2
  return 1
}

validate_unscoped_run_action_fails() {
  local status
  status="$(command curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -sS -o "$TMP_DIR/unscoped-run-status.out" -w "%{http_code}" "$BASE_URL/api/v1/workorch/runs/run-001/status")"
  case "$status" in
    2*)
      echo "unscoped run status unexpectedly succeeded with HTTP $status" >&2
      return 1
      ;;
  esac
}

validate_projection_metrics() {
  curl -fsS "$BASE_URL/metrics" >"$TMP_DIR/metrics.txt"
  python3 - "$TMP_DIR/metrics.txt" "$WORKSPACE_A" "$WORKSPACE_B" <<'PY'
import re
import sys

path, ws_a, ws_b = sys.argv[1:]
with open(path, "r", encoding="utf-8") as handle:
    text = handle.read()

required_names = [
    "workorch_workspace_projection_rebuild_total",
    "workorch_workspace_projection_rebuild_failed_total",
    "workorch_workspace_projection_rebuild_duration_ms_total",
    "workorch_workspace_projection_pending_rebuilds",
    "workorch_workspace_projection_debounced_rebuild_total",
    "workorch_workspace_projection_schedule_dropped_total",
    "workorch_workspace_projection_active_run_heartbeats",
    "workorch_workspace_projection_active_run_heartbeat_age_seconds_max",
    "workorch_workspace_projection_stalled_runs",
    "workorch_workspace_projection_ineffective_retry_runs",
]
missing = [name for name in required_names if name not in text]
if missing:
    raise SystemExit(f"missing projection metrics: {missing}")

full_match = re.search(r'^workorch_workspace_projection_rebuild_total\{scope="full"\}\s+([0-9]+(?:\.[0-9]+)?)$', text, re.MULTILINE)
if not full_match or float(full_match.group(1)) < 1:
    raise SystemExit("full projection rebuild metric did not record startup baseline")

for workspace_id in (ws_a, ws_b):
    pattern = r'^workorch_workspace_projection_rebuild_total\{scope="workspace",workspace="' + re.escape(workspace_id) + r'"\}\s+([0-9]+(?:\.[0-9]+)?)$'
    match = re.search(pattern, text, re.MULTILINE)
    if not match or float(match.group(1)) < 1:
        raise SystemExit(f"workspace projection rebuild metric missing or zero for {workspace_id}")
PY
}

validate_console_projection_contract() {
  python3 - "$BASE_URL" "$TMP_DIR/console-index.html" "$TMP_DIR/console-assets.txt" <<'PY'
import html.parser
import pathlib
import re
import sys
import urllib.parse
import urllib.request

base_url, index_path, assets_path = sys.argv[1:]

class AssetParser(html.parser.HTMLParser):
    def __init__(self):
        super().__init__()
        self.scripts = []

    def handle_starttag(self, tag, attrs):
        if tag != "script":
            return
        values = dict(attrs)
        src = values.get("src", "")
        if src:
            self.scripts.append(src)

def fetch_url(url):
    with urllib.request.urlopen(url, timeout=30) as response:
        return response.read().decode("utf-8", errors="replace")

def resolve_console_url(path):
    return urllib.parse.urljoin(base_url.rstrip("/") + "/", path.lstrip("/"))

def fetch(path):
    return fetch_url(resolve_console_url(path))

index = fetch("/console")
pathlib.Path(index_path).write_text(index, encoding="utf-8")
parser = AssetParser()
parser.feed(index)
scripts = [src for src in parser.scripts if src.endswith(".js")]
if not scripts:
    raise SystemExit("console index did not include any JavaScript asset")

seen = set()
queue = [resolve_console_url(src if src.startswith("/") else "/console/" + src.lstrip("./")) for src in scripts]
combined = []
asset_refs = []
while queue:
    url = queue.pop(0)
    if url in seen:
        continue
    seen.add(url)
    body = fetch_url(url)
    combined.append(body)
    asset_refs.append(url)
    for match in re.findall(r'(?:"|\')((?:\./)?(?:assets/)?[^"\']+\.js)(?:"|\')', body):
        if not match.startswith("./") and not match.startswith("assets/"):
            continue
        if match.startswith("assets/"):
            next_url = resolve_console_url("/console/" + match)
        else:
            next_url = urllib.parse.urljoin(url, match)
        if next_url not in seen:
            queue.append(next_url)
text = "\n".join(combined)
pathlib.Path(assets_path).write_text("\n".join(asset_refs), encoding="utf-8")
required = [
    "/workorchd/workspace/projection/changed",
    "workspace_projection_changed",
    "投影状态",
    "运行活动",
    "最近有效进展",
    "stalled",
    "ineffective retry",
]
missing = [item for item in required if item not in text]
if missing:
    raise SystemExit(f"console bundle missing projection/heartbeat display contract markers: {missing}")
PY
}

trap report_failure ERR
trap cleanup EXIT

start_watchdog
wait_ready
start_sse_listener

WORKSPACE_A="$(create_workspace "A" "$TMP_DIR/workspace-a.json")"
WORKSPACE_B="$(create_workspace "B" "$TMP_DIR/workspace-b.json")"
[[ -n "$WORKSPACE_A" && -n "$WORKSPACE_B" && "$WORKSPACE_A" != "$WORKSPACE_B" ]]

define_goal_dod "$WORKSPACE_A" "a"
define_goal_dod "$WORKSPACE_B" "b"

wait_workspace_snapshot_projection "$WORKSPACE_A" "a"
wait_workspace_snapshot_projection "$WORKSPACE_B" "b"
validate_workspace_list_projection
wait_projection_events
validate_unscoped_run_action_fails
validate_projection_metrics
validate_console_projection_contract

echo "workorch projection workspace switch verification passed for deployed workorch service $BASE_URL workspaces=$WORKSPACE_A,$WORKSPACE_B"
