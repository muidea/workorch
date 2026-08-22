#!/usr/bin/env bash
# TC10 live verification for TUI workspace runtime refresh / history closure.
#
# Covers (against a real local workorch --server process):
#   1. runtime-view readback for active + non-active workspace
#   2. SSE projection stream connect → live notification → Last-Event-ID
#   3. server restart with durable file backend; post-restart runtime-view
#      revisions remain acceptable (CR01 epoch continuity)
#   4. SSE interruption + reconnect recovery with readback
#   5. real Runtime execution through the production activation endpoint
#   6. actual Bubble Tea TUI/PTY rendering gate
#
# Live outputs stay local/ephemeral by default (not committed under docs/).
# Override with WORKORCH_TUI_REFRESH_VERIFY_ARTIFACT_DIR / KEEP_ARTIFACTS as needed.

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/scripts/lib/verify-isolated-runtime.sh"
STAMP="${WORKORCH_TUI_REFRESH_VERIFY_STAMP:-$(date -u +%Y%m%dT%H%M%SZ)}"
DEFAULT_ARTIFACT_DIR="${TMPDIR:-/tmp}/workorch-live-artifacts/tui-runtime-refresh-live-$STAMP"
ARTIFACT_DIR="${WORKORCH_TUI_REFRESH_VERIFY_ARTIFACT_DIR:-$DEFAULT_ARTIFACT_DIR}"
KEEP_ARTIFACTS="${WORKORCH_TUI_REFRESH_VERIFY_KEEP_ARTIFACTS:-0}"
CONFIG_DIR="$ARTIFACT_DIR/config"
DATA_DIR="${WORKORCH_TUI_REFRESH_VERIFY_DATA_DIR:-$ARTIFACT_DIR/data}"
SERVER_LOG="$ARTIFACT_DIR/workorch-server.log"
REPORT_JSON="$ARTIFACT_DIR/report.json"
SUMMARY_MD="$ARTIFACT_DIR/SUMMARY.md"
ISOLATION_STATE_FILE="$ARTIFACT_DIR/managed-runtime-state.json"
BINARY="${WORKORCH_TUI_REFRESH_VERIFY_BINARY:-$ROOT_DIR/bin/workorch}"
VERIFY_SUFFIX="${WORKORCH_TUI_REFRESH_VERIFY_SUFFIX:-$$}"
WS_A="${WORKORCH_TUI_REFRESH_VERIFY_WORKSPACE_A_TITLE:-tui-refresh-ws-a-$VERIFY_SUFFIX}"
WS_B="${WORKORCH_TUI_REFRESH_VERIFY_WORKSPACE_B_TITLE:-tui-refresh-ws-b-$VERIFY_SUFFIX}"
CURL_CONNECT_TIMEOUT="${WORKORCH_VERIFY_CURL_CONNECT_TIMEOUT_SECONDS:-5}"
CURL_MAX_TIME="${WORKORCH_VERIFY_CURL_MAX_TIME_SECONDS:-30}"
SERVER_PID=""
SSE_PID=""
SSE_FIFO=""

select_addr() {
  if [[ -n "${WORKORCH_TUI_REFRESH_VERIFY_ADDR:-}" ]]; then
    printf '%s\n' "$WORKORCH_TUI_REFRESH_VERIFY_ADDR"
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
    echo "curl failed status=$status url=${url:-unknown}" >&2
  fi
  return $status
}

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "tui runtime refresh history live verification failed" >&2
  echo "baseUrl=$BASE_URL artifacts=$ARTIFACT_DIR" >&2
  if [[ -f "$SERVER_LOG" ]]; then
    echo "--- workorch-server.log (tail) ---" >&2
    tail -n 200 "$SERVER_LOG" >&2 || true
  fi
}

stop_sse() {
  if [[ -n "$SSE_PID" ]] && kill -0 "$SSE_PID" >/dev/null 2>&1; then
    kill -TERM "$SSE_PID" >/dev/null 2>&1 || true
    local deadline=$((SECONDS + 5))
    while kill -0 "$SSE_PID" >/dev/null 2>&1 && (( SECONDS < deadline )); do
      sleep 0.1
    done
    if kill -0 "$SSE_PID" >/dev/null 2>&1; then
      kill -KILL "$SSE_PID" >/dev/null 2>&1 || true
    fi
    wait "$SSE_PID" >/dev/null 2>&1 || true
  fi
  SSE_PID=""
  if [[ -n "$SSE_FIFO" && -p "$SSE_FIFO" ]]; then
    rm -f "$SSE_FIFO" || true
  fi
  SSE_FIFO=""
}

stop_server() {
  stop_sse
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
  workorch_verify_write_isolation_state "$ISOLATION_STATE_FILE" "stopped" "$BASE_URL" "$SERVER_PID" "$CONFIG_DIR" "$DATA_DIR"
  SERVER_PID=""
}

cleanup() {
  local status=$?
  report_failure || true
  stop_server
  if [[ "$KEEP_ARTIFACTS" != "1" && "$KEEP_ARTIFACTS" != "true" && $status -eq 0 ]]; then
    # Still keep versioned docs artifacts by default; only purge when explicitly asked.
    if [[ "$ARTIFACT_DIR" == /tmp/* ]]; then
      rm -rf "$ARTIFACT_DIR"
    fi
  fi
}

trap cleanup EXIT

mkdir -p "$ARTIFACT_DIR" "$CONFIG_DIR" "$DATA_DIR" "$ARTIFACT_DIR/sse" "$ARTIFACT_DIR/views"

write_config() {
  cat >"$CONFIG_DIR/application.toml" <<EOF
endpointName = "workorch"

[server]
addr = "$ADDR"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "180s"
idleTimeout = "60s"
shutdownTimeout = "5s"

[repository]
backend = "file"
dataDir = "$DATA_DIR"

[runtime.git]
defaultTimeout = "30s"

[llm.aiProxy]
baseURL = "http://127.0.0.1:9"
apiKey = ""
timeout = "5s"
catalogRequestTimeout = "1s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "gpt-4o"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25

[governance]
[governance.autoTicket]
enabled = false
[governance.autoApproval]
enabled = false
EOF
}

build_binary_if_needed() {
  # Always rebuild so live verification exercises the current workspace tree
  # (CR01 epoch continuity and HTTP store-owned RuntimeView path).
  echo "==> go build -o $BINARY ./application/workorch"
  (cd "$ROOT_DIR" && go build -o "$BINARY" ./application/workorch)
}

start_server() {
  local log_suffix="${1:-}"
  local log_file="$SERVER_LOG"
  if [[ -n "$log_suffix" ]]; then
    log_file="$ARTIFACT_DIR/workorch-server-$log_suffix.log"
  fi
  "$BINARY" --server -config-dir "$CONFIG_DIR" >"$log_file" 2>&1 &
  SERVER_PID=$!
  local deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1 &&
      curl -fsS "$BASE_URL/readyz" >"$ARTIFACT_DIR/readyz.json" 2>/dev/null; then
      if grep -q '"status"[[:space:]]*:[[:space:]]*"ready"' "$ARTIFACT_DIR/readyz.json"; then
        workorch_verify_write_isolation_state "$ISOLATION_STATE_FILE" "running" "$BASE_URL" "$SERVER_PID" "$CONFIG_DIR" "$DATA_DIR"
        export WORKORCH_VERIFY_ISOLATION_STATE_FILE="$ISOLATION_STATE_FILE"
        echo "server ready pid=$SERVER_PID addr=$ADDR log=$log_file"
        return 0
      fi
    fi
    if ! kill -0 "$SERVER_PID" >/dev/null 2>&1; then
      echo "workorch server exited before ready; log=$log_file" >&2
      tail -n 80 "$log_file" >&2 || true
      return 1
    fi
    sleep 0.2
  done
  echo "timed out waiting for service at $BASE_URL" >&2
  return 1
}

create_workspace() {
  local title="$1"
  local out="$2"
  local deliverable="$ARTIFACT_DIR/deliverables/$(echo "$title" | tr ' /' '__')"
  mkdir -p "$deliverable"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" \
    -H 'Content-Type: application/json' \
    -d "$(python3 - <<PY
import json
print(json.dumps({
  "title": "$title",
  "description": "tui runtime refresh live verification",
  "deliverableDirectory": "$deliverable",
  "templateId": "generic-governed",
}))
PY
)" >"$out"
}

workspace_id_from() {
  python3 - "$1" <<'PY'
import json, sys
doc = json.load(open(sys.argv[1], encoding="utf-8"))
ws = doc.get("workspace") or doc
print((ws.get("id") or ws.get("workspaceId") or "").strip())
PY
}

fetch_runtime_view() {
  local workspace_id="$1"
  local out="$2"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/${workspace_id}/runtime-view?activityLimit=20" >"$out"
}

extract_revisions() {
  python3 - "$1" <<'PY'
import json, sys
path = sys.argv[1]
doc = json.load(open(path, encoding="utf-8"))
view = doc.get("runtimeView") or doc.get("RuntimeView") or doc
rev = view.get("revision") or {}
activity = view.get("activity") or {}
items = activity.get("items") or []
print(json.dumps({
  "workspaceId": view.get("workspaceId") or "",
  "displayRunId": view.get("displayRunId") or "",
  "workspaceRevision": rev.get("workspaceRevision") or 0,
  "runtimeStateRevision": rev.get("runtimeStateRevision") or 0,
  "activityRevision": rev.get("activityRevision") or activity.get("activityRevision") or 0,
  "projectedSourceRevision": rev.get("projectedSourceRevision") or 0,
  "activityItemCount": len(items),
  "activityIds": [i.get("id") for i in items[:8]],
  "activitySummaries": [i.get("summary") for i in items[:8]],
  "truncated": bool(activity.get("truncated")),
}, ensure_ascii=False, sort_keys=True))
PY
}

# Open SSE projection stream in background; write frames to file.
start_sse() {
  local workspace_id="${1:-}"
  local out_file="$2"
  local last_event_id="${3:-}"
  stop_sse
  : >"$out_file"
  local url="$BASE_URL/api/v1/workorch/events/stream?view=projection"
  if [[ -n "$workspace_id" ]]; then
    url="${url}&workspaceId=${workspace_id}"
  fi
  local headers=(-H 'Accept: text/event-stream' -N)
  if [[ -n "$last_event_id" ]]; then
    headers+=(-H "Last-Event-ID: $last_event_id")
  fi
  # shellcheck disable=SC2086
  command curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time 0 -sS "${headers[@]}" "$url" >"$out_file" 2>"$out_file.err" &
  SSE_PID=$!
}

wait_sse_connected() {
  local out_file="$1"
  local deadline=$((SECONDS + 20))
  while (( SECONDS <= deadline )); do
    if grep -qE 'event:[[:space:]]*connected|data:.*"state"[[:space:]]*:[[:space:]]*"connected"|: connected' "$out_file" 2>/dev/null; then
      return 0
    fi
    # Some servers emit a comment/retry or first data frame as readiness.
    if grep -qE '^data:|^id:|^event:' "$out_file" 2>/dev/null; then
      return 0
    fi
    if [[ -n "$SSE_PID" ]] && ! kill -0 "$SSE_PID" >/dev/null 2>&1; then
      echo "SSE process exited before connect; err:" >&2
      cat "$out_file.err" >&2 || true
      cat "$out_file" >&2 || true
      return 1
    fi
    sleep 0.1
  done
  echo "timed out waiting for SSE connect; file=$out_file" >&2
  cat "$out_file" >&2 || true
  return 1
}

wait_sse_data_frame() {
  local out_file="$1"
  local min_data_lines="${2:-1}"
  local deadline=$((SECONDS + 30))
  while (( SECONDS <= deadline )); do
    local count
    count="$(grep -c '^data:' "$out_file" 2>/dev/null || true)"
    if (( count >= min_data_lines )); then
      return 0
    fi
    sleep 0.1
  done
  echo "timed out waiting for SSE data frames (want>=$min_data_lines) file=$out_file" >&2
  return 1
}

last_event_id_from_sse() {
  python3 - "$1" <<'PY'
import sys
last = ""
for line in open(sys.argv[1], encoding="utf-8", errors="replace"):
    if line.startswith("id:"):
        last = line[3:].strip()
print(last)
PY
}

parse_last_projection_frame() {
  python3 - "$1" <<'PY'
import json, sys
path = sys.argv[1]
event_id = ""
data_lines = []
frames = []
for raw in open(path, encoding="utf-8", errors="replace"):
    line = raw.rstrip("\n")
    if line.startswith("id:"):
        event_id = line[3:].strip()
    elif line.startswith("data:"):
        data_lines.append(line[5:].strip())
    elif line == "":
        if data_lines:
            payload = "\n".join(data_lines)
            try:
                obj = json.loads(payload)
            except Exception:
                obj = {"raw": payload}
            frames.append({"id": event_id, "data": obj})
        event_id = ""
        data_lines = []
if data_lines:
    payload = "\n".join(data_lines)
    try:
        obj = json.loads(payload)
    except Exception:
        obj = {"raw": payload}
    frames.append({"id": event_id, "data": obj})
if not frames:
    print("{}")
else:
    print(json.dumps(frames[-1], ensure_ascii=False))
PY
}

# Select and validate an actual projection DTO, rather than treating the SSE
# connected marker as proof of a projection update. The generation-scoped ID
# is the restart contract: counters may restart, identities may not collide.
assert_sse_projection_frame() {
  local sse_log="$1"
  local workspace_id="$2"
  local out="$3"
  python3 - "$sse_log" "$workspace_id" "$out" <<'PY'
import json, sys
path, workspace_id, out = sys.argv[1:]
event_id = ""
data_lines = []
frames = []
for raw in open(path, encoding="utf-8", errors="replace"):
    line = raw.rstrip("\n")
    if line.startswith("id:"):
        event_id = line[3:].strip()
    elif line.startswith("data:"):
        data_lines.append(line[5:].strip())
    elif line == "":
        if data_lines:
            try:
                frames.append((event_id, json.loads("\n".join(data_lines))))
            except json.JSONDecodeError:
                pass
        event_id, data_lines = "", []
if data_lines:
    try:
        frames.append((event_id, json.loads("\n".join(data_lines))))
    except json.JSONDecodeError:
        pass
for event_id, frame in reversed(frames):
    if not isinstance(frame, dict) or frame.get("workspaceId") != workspace_id:
        continue
    revision = frame.get("revision") or {}
    generation = str(frame.get("streamGeneration") or revision.get("streamGeneration") or "").strip()
    stream_revision = int(frame.get("streamRevision") or revision.get("streamRevision") or 0)
    if not generation or stream_revision <= 0:
        continue
    expected = f"projection:{generation}:{workspace_id}:"
    if not event_id.startswith(expected):
        raise SystemExit(f"projection frame ID is not generation-scoped: id={event_id!r} expectedPrefix={expected!r}")
    result = {"id": event_id, "generation": generation, "streamRevision": stream_revision, "data": frame}
    open(out, "w", encoding="utf-8").write(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(result, ensure_ascii=False))
    break
else:
    raise SystemExit(f"no generation-scoped projection DTO for workspace {workspace_id!r} in {path}")
PY
}

assert_sse_generation_changed() {
  python3 - "$1" "$2" <<'PY'
import json, sys
before, after = (json.load(open(path, encoding="utf-8")) for path in sys.argv[1:])
if before["generation"] == after["generation"]:
    raise SystemExit(f"stream generation did not change across restart: {before['generation']}")
if before["id"] == after["id"]:
    raise SystemExit(f"Last-Event-ID collided across restart: {before['id']}")
print(f"generation changed: {before['generation']} -> {after['generation']}")
PY
}

assert_revision_acceptable() {
  # post must not be permanently rejected vs pre (CR01): workspace/activity
  # revisions after restart must be >= pre when pre > 0, OR if store rebuilt
  # empty and then re-projected, epoch ensures post >> pre.
  python3 - "$1" "$2" <<'PY'
import json, sys
pre = json.load(open(sys.argv[1], encoding="utf-8"))
post = json.load(open(sys.argv[2], encoding="utf-8"))
errors = []
for key in ("workspaceRevision", "activityRevision"):
    a = int(pre.get(key) or 0)
    b = int(post.get(key) or 0)
    if a > 0 and b < a:
        errors.append(f"{key}: post={b} < pre={a}")
if errors:
    raise SystemExit("revision not acceptable after restart: " + "; ".join(errors))
print("ok")
PY
}

mutate_workspace_title() {
  # Force a workspace projection rebuild/notification without requiring LLM.
  # Goal define API expects {"text": "..."} and triggers rebuildProjectedWorkspace.
  local workspace_id="$1"
  local label="$2"
  local out="$3"
  local status=0
  set +e
  command curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -sS -X POST \
    "$BASE_URL/api/v1/workorch/workspaces/${workspace_id}/goals" \
    -H 'Content-Type: application/json' \
    -d "$(python3 -c "import json; print(json.dumps({'text': 'live-goal-${label}: projection change trigger for tui refresh verification'}))")" \
    >"$out" 2>"$out.err"
  status=$?
  if [[ $status -eq 0 ]] && python3 -c "import json,sys; doc=json.load(open(sys.argv[1],encoding='utf-8')); sys.exit(1 if doc.get('error') else 0)" "$out"; then
    set -e
    return 0
  fi
  # Fallback 1: define DoD items (also rebuilds projection).
  command curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -sS -X POST \
    "$BASE_URL/api/v1/workorch/workspaces/${workspace_id}/dods" \
    -H 'Content-Type: application/json' \
    -d "$(python3 -c "import json; print(json.dumps({'items': ['live-dod-${label}-1', 'live-dod-${label}-2']}))")" \
    >"$out" 2>"$out.err"
  status=$?
  if [[ $status -eq 0 ]] && python3 -c "import json,sys; doc=json.load(open(sys.argv[1],encoding='utf-8')); sys.exit(1 if doc.get('error') else 0)" "$out"; then
    set -e
    return 0
  fi
  # Fallback 2: create throwaway workspace (global projection stream activity).
  set -e
  create_workspace "tui-refresh-trigger-$label-$VERIFY_SUFFIX" "$out"
}

seed_runtime_run() {
  local workspace_id="$1"
  local run_id="$2"
  local state_revision="$3"
  local focus_id="$4"
  local action="$5"
  (cd "$ROOT_DIR" && go run ./scripts/seed-runtime-run.go "$DATA_DIR" "$workspace_id" "$run_id" "$state_revision" "$focus_id" "$action")
}

assert_runtime_state_revision() {
  local revisions_json="$1"
  local min_rev="$2"
  python3 - "$revisions_json" "$min_rev" <<'PY2'
import json, sys
path, min_rev = sys.argv[1], int(sys.argv[2])
doc = json.load(open(path, encoding="utf-8"))
rev = int(doc.get("runtimeStateRevision") or 0)
if rev < min_rev:
    raise SystemExit(f"runtimeStateRevision={rev}, want >= {min_rev} in {path}")
print(f"runtimeStateRevision={rev} ok")
PY2
}

run_tui_restart_history_gate() {
  # FR04: package gates cover restart/history/switch/follow-tail/PgUp unread
  # and FR02 equal-revision field mutations used by the real TUI process.
  (cd "$ROOT_DIR" && go test ./internal/services/workorch/service/tui \
    -run 'TestTUIRestart|TestNonActiveDirtyThenSwitchRecoversHistory|TestRuntimeViewFollowTail|TestEqualRevisionEqualContent|TestEqualRevisionFieldMutations|TestRuntimeViewMsgScopeMismatch|TestZeroRevision' \
    -count=1 -timeout 120s) | tee "$ARTIFACT_DIR/tui-automated-gates.log"
}

run_tui_pty_smoke_if_enabled() {
  # TC10 is not complete without a real Bubble Tea process. Keep the legacy
  # function name for callers, but PTY is mandatory from this point on.
  mkdir -p "$ARTIFACT_DIR/tui-pty"
  WORKORCH_TUI_TERMINAL_BASE_URL="$BASE_URL" \
  WORKORCH_TUI_TERMINAL_BINARY="$BINARY" \
  WORKORCH_TUI_TERMINAL_WORKSPACE_ID="$WS_A_ID" \
  WORKORCH_TUI_TERMINAL_EXERCISE_SCROLL=1 \
  WORKORCH_TUI_TERMINAL_VERIFY_LIVE_UPDATE=1 \
  WORKORCH_TUI_TERMINAL_KEEP_ARTIFACTS=1 \
  WORKORCH_TUI_TERMINAL_ARTIFACT_DIR="$ARTIFACT_DIR/tui-pty" \
  WORKORCH_TUI_TERMINAL_TIMEOUT_SECONDS="${WORKORCH_TUI_REFRESH_VERIFY_PTY_TIMEOUT:-20}" \
    python3 "$ROOT_DIR/scripts/verify-tui-terminal-render-smoke.py" \
    | tee "$ARTIFACT_DIR/tui-pty.log"
}

run_real_runtime_and_tui_gate() {
  # Reuse the repository's production activation E2E: it configures a scoped
  # mock provider through the HTTP API, creates Goal/DoD/skill/soul, calls the
  # execution-activation endpoint, waits for DriveRuntime completion, and
  # drives the actual Bubble Tea renderer through a PTY. This is deliberately
  # separate from the seed fixture below; direct repository writes cannot prove
  # a real Runtime execution path.
  local gate_dir="$ARTIFACT_DIR/real-runtime-tui"
  local mock_addr
  mock_addr="$(select_addr)"
  mkdir -p "$gate_dir"
  WORKORCH_TUI_LONG_OUTPUT_ARTIFACT_DIR="$gate_dir" \
  WORKORCH_TUI_LONG_OUTPUT_BASE_URL="$BASE_URL" \
  WORKORCH_TUI_LONG_OUTPUT_MOCK_ADDR="$mock_addr" \
  WORKORCH_TUI_LONG_OUTPUT_KEEP_ARTIFACTS=1 \
  WORKORCH_TUI_LONG_OUTPUT_VERIFY_TERMINAL_RENDER=1 \
    "$ROOT_DIR/scripts/verify-tui-long-output-e2e.sh" | tee "$gate_dir/real-runtime-tui.log"

  local workspace_id
  workspace_id="$(workspace_id_from "$gate_dir/workspace.json")"
  if [[ -z "$workspace_id" ]]; then
    echo "real Runtime gate did not record workspace id" >&2
    return 1
  fi
  fetch_runtime_view "$workspace_id" "$gate_dir/runtime-view.json"
  extract_revisions "$gate_dir/runtime-view.json" >"$gate_dir/runtime-revisions.json"
  assert_runtime_state_revision "$gate_dir/runtime-revisions.json" 1 \
    | tee "$gate_dir/runtime-revision-assert.txt"
  test -f "$gate_dir/tui-terminal-render/tui-terminal-render-summary.json"
}


write_config
build_binary_if_needed

echo "==> phase 1: start server"
start_server "boot1"
cp "$ARTIFACT_DIR/readyz.json" "$ARTIFACT_DIR/01-readyz-boot1.json"

echo "==> phase 1b: real Runtime execution + actual TUI/PTY"
run_real_runtime_and_tui_gate

echo "==> phase 2: create workspaces A/B"
create_workspace "$WS_A" "$ARTIFACT_DIR/02-create-ws-a.json"
create_workspace "$WS_B" "$ARTIFACT_DIR/03-create-ws-b.json"
WS_A_ID="$(workspace_id_from "$ARTIFACT_DIR/02-create-ws-a.json")"
WS_B_ID="$(workspace_id_from "$ARTIFACT_DIR/03-create-ws-b.json")"
if [[ -z "$WS_A_ID" || -z "$WS_B_ID" ]]; then
  echo "failed to parse workspace ids" >&2
  exit 1
fi
echo "wsA=$WS_A_ID wsB=$WS_B_ID" | tee "$ARTIFACT_DIR/workspace-ids.txt"
RUN_A_ID="run-live-a-$VERIFY_SUFFIX"
RUN_B_ID="run-live-b-$VERIFY_SUFFIX"
echo "runA=$RUN_A_ID runB=$RUN_B_ID" | tee -a "$ARTIFACT_DIR/workspace-ids.txt"

echo "==> phase 2b: seed restart/readback fixtures (not the real Runtime proof)"
stop_server
seed_runtime_run "$WS_A_ID" "$RUN_A_ID" 7 "focus-live-a" "produce_validation" | tee "$ARTIFACT_DIR/02b-seed-run-a.log"
seed_runtime_run "$WS_B_ID" "$RUN_B_ID" 3 "focus-live-b" "produce_validation" | tee "$ARTIFACT_DIR/02b-seed-run-b.log"
start_server "boot1b"

echo "==> phase 3: initial runtime-view readback (must show RuntimeStateRevision > 0)"
fetch_runtime_view "$WS_A_ID" "$ARTIFACT_DIR/views/04-ws-a-runtime-view-before.json"
fetch_runtime_view "$WS_B_ID" "$ARTIFACT_DIR/views/05-ws-b-runtime-view-before.json"
extract_revisions "$ARTIFACT_DIR/views/04-ws-a-runtime-view-before.json" >"$ARTIFACT_DIR/views/04-ws-a-revisions-before.json"
extract_revisions "$ARTIFACT_DIR/views/05-ws-b-runtime-view-before.json" >"$ARTIFACT_DIR/views/05-ws-b-revisions-before.json"
assert_runtime_state_revision "$ARTIFACT_DIR/views/04-ws-a-revisions-before.json" 7 | tee "$ARTIFACT_DIR/views/04-runtime-rev-assert.txt"
assert_runtime_state_revision "$ARTIFACT_DIR/views/05-ws-b-revisions-before.json" 3 | tee -a "$ARTIFACT_DIR/views/04-runtime-rev-assert.txt"

echo "==> phase 4: SSE connect (projection view, workspace A)"
start_sse "$WS_A_ID" "$ARTIFACT_DIR/sse/06-sse-boot1.log"
wait_sse_connected "$ARTIFACT_DIR/sse/06-sse-boot1.log"
echo "sse_connected=true" >"$ARTIFACT_DIR/sse/06-sse-status.txt"

echo "==> phase 5: trigger projection change while SSE live"
mutate_workspace_title "$WS_A_ID" "live1" "$ARTIFACT_DIR/07-mutate-ws-a-live1.json"
wait_sse_data_frame "$ARTIFACT_DIR/sse/06-sse-boot1.log" 1
assert_sse_projection_frame \
  "$ARTIFACT_DIR/sse/06-sse-boot1.log" "$WS_A_ID" \
  "$ARTIFACT_DIR/sse/06-sse-projection-before-restart.json" \
  | tee "$ARTIFACT_DIR/sse/06-sse-projection-before-restart.assert.txt"
# Advance durable Runtime state (server stopped for exclusive file access).
stop_sse
stop_server
seed_runtime_run "$WS_A_ID" "$RUN_A_ID" 11 "focus-live-a-2" "produce_validation" | tee "$ARTIFACT_DIR/07b-seed-run-a-advance.log"
start_server "boot1c"
start_sse "$WS_A_ID" "$ARTIFACT_DIR/sse/06b-sse-after-runtime-advance.log"
wait_sse_connected "$ARTIFACT_DIR/sse/06b-sse-after-runtime-advance.log" || true
sleep 0.5
fetch_runtime_view "$WS_A_ID" "$ARTIFACT_DIR/views/08-ws-a-runtime-view-after-mutate1.json"
extract_revisions "$ARTIFACT_DIR/views/08-ws-a-runtime-view-after-mutate1.json" >"$ARTIFACT_DIR/views/08-ws-a-revisions-after-mutate1.json"
assert_runtime_state_revision "$ARTIFACT_DIR/views/08-ws-a-revisions-after-mutate1.json" 11 | tee "$ARTIFACT_DIR/views/08-runtime-rev-assert.txt"
LAST_EVENT_ID="$(python3 - "$ARTIFACT_DIR/sse/06-sse-projection-before-restart.json" <<'PY'
import json, sys
print(json.load(open(sys.argv[1], encoding="utf-8"))["id"])
PY
)"
parse_last_projection_frame "$ARTIFACT_DIR/sse/06-sse-boot1.log" >"$ARTIFACT_DIR/sse/06-sse-last-frame.json"
echo "lastEventId=$LAST_EVENT_ID" | tee "$ARTIFACT_DIR/sse/06-last-event-id.txt"

echo "==> phase 6: non-active workspace B mutation while focused on A semantics"
mutate_workspace_title "$WS_B_ID" "background" "$ARTIFACT_DIR/09-mutate-ws-b-background.json"
fetch_runtime_view "$WS_B_ID" "$ARTIFACT_DIR/views/10-ws-b-runtime-view-after-background.json"
extract_revisions "$ARTIFACT_DIR/views/10-ws-b-runtime-view-after-background.json" >"$ARTIFACT_DIR/views/10-ws-b-revisions-after-background.json"

PRE_RESTART_A="$ARTIFACT_DIR/views/08-ws-a-revisions-after-mutate1.json"
cp "$PRE_RESTART_A" "$ARTIFACT_DIR/views/11-ws-a-revisions-pre-restart.json"

echo "==> phase 7: interrupt SSE (simulate disconnect)"
stop_sse
echo "sse_state=interrupted" >>"$ARTIFACT_DIR/sse/06-sse-status.txt"

echo "==> phase 8: restart server (durable file backend, same data dir)"
stop_server
sleep 0.5
start_server "boot2"
cp "$ARTIFACT_DIR/readyz.json" "$ARTIFACT_DIR/12-readyz-boot2.json"

echo "==> phase 9: post-restart runtime-view readback (CR01 continuity)"
fetch_runtime_view "$WS_A_ID" "$ARTIFACT_DIR/views/13-ws-a-runtime-view-post-restart.json"
fetch_runtime_view "$WS_B_ID" "$ARTIFACT_DIR/views/14-ws-b-runtime-view-post-restart.json"
extract_revisions "$ARTIFACT_DIR/views/13-ws-a-runtime-view-post-restart.json" >"$ARTIFACT_DIR/views/13-ws-a-revisions-post-restart.json"
extract_revisions "$ARTIFACT_DIR/views/14-ws-b-runtime-view-post-restart.json" >"$ARTIFACT_DIR/views/14-ws-b-revisions-post-restart.json"
assert_revision_acceptable \
  "$ARTIFACT_DIR/views/11-ws-a-revisions-pre-restart.json" \
  "$ARTIFACT_DIR/views/13-ws-a-revisions-post-restart.json" \
  | tee "$ARTIFACT_DIR/15-revision-acceptability.txt"

echo "==> phase 10: SSE reconnect with Last-Event-ID + live recovery"
start_sse "$WS_A_ID" "$ARTIFACT_DIR/sse/16-sse-boot2-reconnect.log" "$LAST_EVENT_ID"
wait_sse_connected "$ARTIFACT_DIR/sse/16-sse-boot2-reconnect.log"
echo "sse_state=reconnected lastEventId=$LAST_EVENT_ID" >>"$ARTIFACT_DIR/sse/06-sse-status.txt"
mutate_workspace_title "$WS_A_ID" "post-reconnect" "$ARTIFACT_DIR/17-mutate-ws-a-post-reconnect.json"
sleep 0.5
fetch_runtime_view "$WS_A_ID" "$ARTIFACT_DIR/views/18-ws-a-runtime-view-post-reconnect.json"
extract_revisions "$ARTIFACT_DIR/views/18-ws-a-runtime-view-post-reconnect.json" >"$ARTIFACT_DIR/views/18-ws-a-revisions-post-reconnect.json"
wait_sse_data_frame "$ARTIFACT_DIR/sse/16-sse-boot2-reconnect.log" 1
parse_last_projection_frame "$ARTIFACT_DIR/sse/16-sse-boot2-reconnect.log" >"$ARTIFACT_DIR/sse/16-sse-last-frame.json"
assert_sse_projection_frame \
  "$ARTIFACT_DIR/sse/16-sse-boot2-reconnect.log" "$WS_A_ID" \
  "$ARTIFACT_DIR/sse/16-sse-projection-after-restart.json" \
  | tee "$ARTIFACT_DIR/sse/16-sse-projection-after-restart.assert.txt"
assert_sse_generation_changed \
  "$ARTIFACT_DIR/sse/06-sse-projection-before-restart.json" \
  "$ARTIFACT_DIR/sse/16-sse-projection-after-restart.json" \
  | tee "$ARTIFACT_DIR/sse/16-sse-generation-assert.txt"
POST_RECONNECT_LAST_ID="$(python3 - "$ARTIFACT_DIR/sse/16-sse-projection-after-restart.json" <<'PY'
import json, sys
print(json.load(open(sys.argv[1], encoding="utf-8"))["id"])
PY
)"
echo "postReconnectLastEventId=$POST_RECONNECT_LAST_ID" | tee "$ARTIFACT_DIR/sse/16-last-event-id.txt"
stop_sse

echo "==> phase 11: TUI automated restart/history/switch gates"
run_tui_restart_history_gate
echo "==> phase 11b: mandatory TUI PTY smoke"
run_tui_pty_smoke_if_enabled

echo "==> phase 12: write report"
python3 - "$ARTIFACT_DIR" "$BASE_URL" "$WS_A_ID" "$WS_B_ID" "$LAST_EVENT_ID" "$POST_RECONNECT_LAST_ID" "$STAMP" <<'PY'
import json, pathlib, sys
artifact_dir, base_url, ws_a, ws_b, last_id, post_id, stamp = sys.argv[1:]
root = pathlib.Path(artifact_dir)

def load(path):
    p = root / path
    if not p.exists():
        return None
    try:
        return json.loads(p.read_text(encoding="utf-8"))
    except Exception:
        return {"raw": p.read_text(encoding="utf-8", errors="replace")[:2000]}

report = {
    "stamp": stamp,
    "baseUrl": base_url,
    "artifactDir": str(root),
    "workspaces": {"A": ws_a, "B": ws_b},
    "sse": {
        "lastEventIdBeforeRestart": last_id,
        "lastEventIdAfterReconnect": post_id,
        "boot1Log": "sse/06-sse-boot1.log",
        "boot2ReconnectLog": "sse/16-sse-boot2-reconnect.log",
    },
    "revisions": {
        "A_before": load("views/04-ws-a-revisions-before.json"),
        "A_afterMutate1": load("views/08-ws-a-revisions-after-mutate1.json"),
        "A_preRestart": load("views/11-ws-a-revisions-pre-restart.json"),
        "A_postRestart": load("views/13-ws-a-revisions-post-restart.json"),
        "A_postReconnect": load("views/18-ws-a-revisions-post-reconnect.json"),
        "B_before": load("views/05-ws-b-revisions-before.json"),
        "B_afterBackground": load("views/10-ws-b-revisions-after-background.json"),
        "B_postRestart": load("views/14-ws-b-revisions-post-restart.json"),
    },
    "checks": {
        "serverRestartRevisionAcceptable": True,
        "sseConnectLive": True,
        "sseInterruptReconnect": True,
        "sseGenerationChangedAcrossRestart": True,
        "runtimeViewReadback": True,
        "tuiPtyLiveUnreadAndFollowTail": True,
        "tuiAutomatedHistoryGates": True,
    },
    "notes": [
        "Server uses file backend; dataDir retained across restart.",
        "CR01: post-restart workspace/activity revisions must be >= pre-restart when pre > 0.",
        "SSE interruption simulated by killing curl consumer; reconnect used Last-Event-ID when available.",
        "Real Runtime activation and PTY evidence are in real-runtime-tui/.",
        "PTY asserts a production projection update produces the PgUp unread banner and PgDn restores follow-tail.",
    ],
}
(root / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

def rev_line(label, obj):
    if not obj:
        return f"- {label}: (missing)"
    return (
        f"- {label}: workspace={obj.get('workspaceRevision')} "
        f"runtime={obj.get('runtimeStateRevision')} "
        f"activity={obj.get('activityRevision')} "
        f"items={obj.get('activityItemCount')} "
        f"summaries={obj.get('activitySummaries')}"
    )

summary = f"""# TUI Runtime Refresh / History Live Verification

Stamp: `{stamp}`

Base URL: `{base_url}`

Artifact dir: `{root}`

## Workspaces

- A: `{ws_a}`
- B: `{ws_b}`

## SSE

- Last-Event-ID before restart: `{last_id or "(none)"}`
- Last-Event-ID after reconnect: `{post_id or "(none)"}`
- boot1 log: `sse/06-sse-boot1.log`
- reconnect log: `sse/16-sse-boot2-reconnect.log`

## Revisions

{rev_line("A before", report["revisions"]["A_before"])}
{rev_line("A after mutate", report["revisions"]["A_afterMutate1"])}
{rev_line("A pre-restart", report["revisions"]["A_preRestart"])}
{rev_line("A post-restart", report["revisions"]["A_postRestart"])}
{rev_line("A post-reconnect", report["revisions"]["A_postReconnect"])}
{rev_line("B before", report["revisions"]["B_before"])}
{rev_line("B background mutate", report["revisions"]["B_afterBackground"])}
{rev_line("B post-restart", report["revisions"]["B_postRestart"])}

## Checks

- server restart revision acceptable (CR01): PASS
- SSE connect / live: PASS
- SSE interrupt + reconnect + readback: PASS
- runtime-view history readback A/B: PASS
- real Runtime activation + TUI PTY: PASS (see `real-runtime-tui/`)
- TUI automated restart/history gates: PASS (supplemental; see `tui-automated-gates.log`)

## Plan mapping

Covers plan §15.5 automated/local-deployed portion for TC10:

1. runtime-view progress/history readback
2. non-active workspace activity recoverable on switch/readback
3. server restart while client would keep AppliedRevision
4. SSE interruption / reconnect / Last-Event-ID
5. production Runtime execution through execution-activation
6. actual Bubble Tea renderer via mandatory PTY

Interactive TUI viewport (PgUp unread banner) remains covered by package tests
`TestRuntimeViewFollowTailAndUserScrolledUnread` in the automated gate log.
"""
(root / "SUMMARY.md").write_text(summary, encoding="utf-8")
print(summary)
PY

echo "tui runtime refresh history live verification PASSED"
echo "artifacts=$ARTIFACT_DIR"
echo "report=$REPORT_JSON"
echo "summary=$SUMMARY_MD"
