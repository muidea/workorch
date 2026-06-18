#!/usr/bin/env bash

set -Eeuo pipefail

BASE_URL="${WORKORCH_OBSERVE_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
TMP_DIR="${WORKORCH_OBSERVE_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_OBSERVE_KEEP_ARTIFACTS:-1}"
DURATION_SECONDS="${WORKORCH_OBSERVE_DURATION_SECONDS:-20}"
CURL_CONNECT_TIMEOUT="${WORKORCH_VERIFY_CURL_CONNECT_TIMEOUT_SECONDS:-5}"
CURL_MAX_TIME="${WORKORCH_VERIFY_CURL_MAX_TIME_SECONDS:-30}"
WORKSPACE_IDS="${WORKORCH_OBSERVE_WORKSPACE_IDS:-}"
AUTO_SNAPSHOT_LIMIT="${WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT:-20}"
AI_PROXY_LOG="${WORKORCH_OBSERVE_AI_PROXY_LOG:-}"
AI_PROXY_INTERACTIONS_DIR="${WORKORCH_OBSERVE_AI_PROXY_INTERACTIONS_DIR:-}"
MIN_CACHE_HIT_RATE="${WORKORCH_OBSERVE_MIN_CACHE_HIT_RATE:-}"
REQUIRE_CACHE_SAMPLES="${WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES:-0}"
MIN_CACHE_SAMPLE_COUNT="${WORKORCH_OBSERVE_MIN_CACHE_SAMPLE_COUNT:-}"
CACHE_PROVIDER="${WORKORCH_OBSERVE_CACHE_PROVIDER:-}"
CACHE_MODEL="${WORKORCH_OBSERVE_CACHE_MODEL:-}"
CACHE_SINCE="${WORKORCH_OBSERVE_CACHE_SINCE:-}"
CACHE_UNTIL="${WORKORCH_OBSERVE_CACHE_UNTIL:-}"
CACHE_BASELINE_FILE="${WORKORCH_OBSERVE_CACHE_BASELINE_FILE:-}"
MIN_CACHE_AVERAGE_DELTA="${WORKORCH_OBSERVE_MIN_CACHE_AVERAGE_DELTA:-}"
MIN_CACHE_LATEST_DELTA="${WORKORCH_OBSERVE_MIN_CACHE_LATEST_DELTA:-}"
EXPECTED_STALE_DISPATCHED_STEP_MIN_AGE_SECONDS="${WORKORCH_OBSERVE_STALE_DISPATCHED_STEP_MIN_AGE_SECONDS:-}"
SYNTHETIC_WORKSPACE_COUNT="${WORKORCH_OBSERVE_SYNTHETIC_WORKSPACE_COUNT:-0}"
ENABLE_PPROF_CHECK="${WORKORCH_OBSERVE_ENABLE_PPROF_CHECK:-1}"
REQUIRE_PPROF="${WORKORCH_OBSERVE_REQUIRE_PPROF:-0}"
PPROF_GOROUTINE_PATH="${WORKORCH_OBSERVE_PPROF_GOROUTINE_PATH:-/debug/pprof/goroutine?debug=2}"
ENABLE_CPU_PROFILE_CHECK="${WORKORCH_OBSERVE_ENABLE_CPU_PROFILE_CHECK:-1}"
REQUIRE_CPU_PROFILE="${WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE:-0}"
CPU_PROFILE_SECONDS="${WORKORCH_OBSERVE_CPU_PROFILE_SECONDS:-5}"
MAX_PENDING_REBUILDS="${WORKORCH_OBSERVE_MAX_PENDING_REBUILDS:-10}"
MAX_PROJECTION_SCHEDULE_DROPPED_DELTA="${WORKORCH_OBSERVE_MAX_PROJECTION_SCHEDULE_DROPPED_DELTA:-0}"
MAX_SUSPICIOUS_CPU_FLAT_PERCENT="${WORKORCH_OBSERVE_MAX_SUSPICIOUS_CPU_FLAT_PERCENT:-50}"
MAX_SUSPICIOUS_CPU_CUM_PERCENT="${WORKORCH_OBSERVE_MAX_SUSPICIOUS_CPU_CUM_PERCENT:-90}"
REQUIRE_GOVERNANCE_CONTINUATION="${WORKORCH_OBSERVE_REQUIRE_GOVERNANCE_CONTINUATION:-0}"

mkdir -p "$TMP_DIR"

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

cleanup() {
  local status=$?
  if [[ "$KEEP_ARTIFACTS" != "1" && "$KEEP_ARTIFACTS" != "true" && $status -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  else
    echo "runtime observation artifacts: $TMP_DIR" >&2
  fi
}

trap cleanup EXIT

curl -fsS "$BASE_URL/readyz" >"$TMP_DIR/readyz.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces" >"$TMP_DIR/workspaces.json"
curl -fsS "$BASE_URL/metrics" >"$TMP_DIR/metrics-before.txt"

python3 - "$BASE_URL/api/v1/workorch/events/stream" "$TMP_DIR/sse-events.jsonl" "$DURATION_SECONDS" >"$TMP_DIR/sse-listener.log" 2>&1 <<'PY' &
import json
import sys
import time
import urllib.request

stream_url, output_path, duration = sys.argv[1], sys.argv[2], float(sys.argv[3])
deadline = time.monotonic() + max(0.0, duration)
request = urllib.request.Request(stream_url, headers={"Accept": "text/event-stream"})
with urllib.request.urlopen(request, timeout=max(duration + 10.0, 15.0)) as response:
    for raw_line in response:
        if time.monotonic() >= deadline:
            break
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
        with open(output_path, "a", encoding="utf-8") as handle:
            handle.write(json.dumps(item, ensure_ascii=False, sort_keys=True) + "\n")
            handle.flush()
PY
sse_pid=$!

sleep 1

created_workspace_ids=()
if [[ "$SYNTHETIC_WORKSPACE_COUNT" =~ ^[0-9]+$ ]] && (( SYNTHETIC_WORKSPACE_COUNT > 0 )); then
  for ((idx = 1; idx <= SYNTHETIC_WORKSPACE_COUNT; idx++)); do
    suffix="$(date +%s)-$$-$idx"
    workspace_dir="$TMP_DIR/synthetic-workspace-$idx"
    mkdir -p "$workspace_dir"
    payload="$(python3 - "$suffix" "$workspace_dir" <<'PY'
import json
import sys

suffix, workspace_dir = sys.argv[1], sys.argv[2]
print(json.dumps({
    "title": f"runtime observation synthetic {suffix}",
    "description": "synthetic workspace used to stimulate projection/SSE observation",
    "deliverableDirectory": workspace_dir,
    "templateId": "generic-governed",
}, ensure_ascii=False))
PY
)"
    curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
      -d "$payload" >"$TMP_DIR/synthetic-workspace-$idx.json"
    workspace_id="$(python3 - "$TMP_DIR/synthetic-workspace-$idx.json" <<'PY'
import json
import sys

body = json.load(open(sys.argv[1], encoding="utf-8"))
print((body.get("workspace") or {}).get("id") or "")
PY
)"
    if [[ -n "$workspace_id" ]]; then
      created_workspace_ids+=("$workspace_id")
      echo "$workspace_id" >>"$TMP_DIR/synthetic-workspace-ids.txt"
    fi
  done
fi

wait "$sse_pid" || true

curl -fsS "$BASE_URL/metrics" >"$TMP_DIR/metrics-after.txt"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces" >"$TMP_DIR/workspaces.json"
if [[ "$ENABLE_PPROF_CHECK" == "1" || "$ENABLE_PPROF_CHECK" == "true" || "$ENABLE_PPROF_CHECK" == "yes" ]]; then
  if ! curl -fsS "$BASE_URL$PPROF_GOROUTINE_PATH" >"$TMP_DIR/pprof-goroutine.txt"; then
    echo "pprof goroutine unavailable at $BASE_URL$PPROF_GOROUTINE_PATH" >"$TMP_DIR/pprof-goroutine-error.txt"
  fi
fi
if [[ "$ENABLE_CPU_PROFILE_CHECK" == "1" || "$ENABLE_CPU_PROFILE_CHECK" == "true" || "$ENABLE_CPU_PROFILE_CHECK" == "yes" ]]; then
  if curl -fsS "$BASE_URL/debug/pprof/profile?seconds=$CPU_PROFILE_SECONDS" >"$TMP_DIR/pprof-cpu.pb.gz"; then
    if command -v go >/dev/null 2>&1; then
      if ! go tool pprof -top -nodecount=40 "$TMP_DIR/pprof-cpu.pb.gz" >"$TMP_DIR/pprof-cpu-top.txt" 2>"$TMP_DIR/pprof-cpu-top-error.txt"; then
        echo "go tool pprof failed for CPU profile" >>"$TMP_DIR/pprof-cpu-top-error.txt"
      fi
    else
      echo "go tool not found; CPU profile captured without top summary" >"$TMP_DIR/pprof-cpu-top-error.txt"
    fi
  else
    echo "pprof CPU profile unavailable at $BASE_URL/debug/pprof/profile?seconds=$CPU_PROFILE_SECONDS" >"$TMP_DIR/pprof-cpu-error.txt"
  fi
fi

if [[ ${#created_workspace_ids[@]} -gt 0 ]]; then
  created_csv="$(IFS=','; echo "${created_workspace_ids[*]}")"
  if [[ -n "$WORKSPACE_IDS" ]]; then
    WORKSPACE_IDS="$WORKSPACE_IDS,$created_csv"
  else
    WORKSPACE_IDS="$created_csv"
  fi
fi

if [[ -z "$WORKSPACE_IDS" && "$AUTO_SNAPSHOT_LIMIT" =~ ^[0-9]+$ && "$AUTO_SNAPSHOT_LIMIT" -gt 0 ]]; then
  WORKSPACE_IDS="$(python3 - "$TMP_DIR/workspaces.json" "$AUTO_SNAPSHOT_LIMIT" <<'PY'
import json
import sys

path, limit_text = sys.argv[1], sys.argv[2]
limit = int(limit_text or "0")
body = json.load(open(path, encoding="utf-8"))
ids = []
for item in body.get("workspaces") or []:
    workspace_id = str(item.get("id") or "").strip()
    if not workspace_id:
        continue
    ids.append(workspace_id)
    if len(ids) >= limit:
        break
print(",".join(ids))
PY
)"
  if [[ -n "$WORKSPACE_IDS" ]]; then
    echo "$WORKSPACE_IDS" >"$TMP_DIR/auto-workspace-ids.txt"
  fi
fi

if [[ -n "$WORKSPACE_IDS" ]]; then
  IFS=',' read -ra IDS <<<"$WORKSPACE_IDS"
  for workspace_id in "${IDS[@]}"; do
    workspace_id="$(echo "$workspace_id" | xargs)"
    [[ -n "$workspace_id" ]] || continue
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$TMP_DIR/snapshot-$workspace_id.json"
  done
fi

python3 - "$TMP_DIR" "$WORKSPACE_IDS" "$AI_PROXY_LOG" "$AI_PROXY_INTERACTIONS_DIR" "$MIN_CACHE_HIT_RATE" "$REQUIRE_CACHE_SAMPLES" "$MIN_CACHE_SAMPLE_COUNT" "$CACHE_PROVIDER" "$CACHE_MODEL" "$CACHE_SINCE" "$CACHE_UNTIL" "$CACHE_BASELINE_FILE" "$MIN_CACHE_AVERAGE_DELTA" "$MIN_CACHE_LATEST_DELTA" "$EXPECTED_STALE_DISPATCHED_STEP_MIN_AGE_SECONDS" "$SYNTHETIC_WORKSPACE_COUNT" "$TMP_DIR/synthetic-workspace-ids.txt" "$ENABLE_PPROF_CHECK" "$REQUIRE_PPROF" "$ENABLE_CPU_PROFILE_CHECK" "$REQUIRE_CPU_PROFILE" "$MAX_PENDING_REBUILDS" "$MAX_PROJECTION_SCHEDULE_DROPPED_DELTA" "$MAX_SUSPICIOUS_CPU_FLAT_PERCENT" "$MAX_SUSPICIOUS_CPU_CUM_PERCENT" "$REQUIRE_GOVERNANCE_CONTINUATION" <<'PY'
import json
import os
import re
import sys
from datetime import datetime, timezone
from pathlib import Path

artifact_dir = Path(sys.argv[1])
workspace_ids = [item.strip() for item in sys.argv[2].split(",") if item.strip()]
ai_proxy_log = sys.argv[3].strip()
ai_proxy_interactions_dir = sys.argv[4].strip()
min_cache_hit_rate = sys.argv[5].strip()
require_cache_samples = sys.argv[6].strip() in {"1", "true", "yes"}
min_cache_sample_count = sys.argv[7].strip()
cache_provider = sys.argv[8].strip()
cache_model = sys.argv[9].strip()
cache_since = sys.argv[10].strip()
cache_until = sys.argv[11].strip()
cache_baseline_file = sys.argv[12].strip()
min_cache_average_delta = sys.argv[13].strip()
min_cache_latest_delta = sys.argv[14].strip()
expected_stale_dispatched_step_min_age_seconds = sys.argv[15].strip()
synthetic_workspace_count = int(sys.argv[16].strip() or "0")
synthetic_workspace_ids_path = Path(sys.argv[17])
enable_pprof_check = sys.argv[18].strip() in {"1", "true", "yes"}
require_pprof = sys.argv[19].strip() in {"1", "true", "yes"}
enable_cpu_profile_check = sys.argv[20].strip() in {"1", "true", "yes"}
require_cpu_profile = sys.argv[21].strip() in {"1", "true", "yes"}
max_pending_rebuilds = int(sys.argv[22].strip() or "10")
max_projection_schedule_dropped_delta = int(sys.argv[23].strip() or "0")
max_suspicious_cpu_flat_percent = float(sys.argv[24].strip() or "50")
max_suspicious_cpu_cum_percent = float(sys.argv[25].strip() or "90")
require_governance_continuation = sys.argv[26].strip() in {"1", "true", "yes"}

def read_json(path):
    with open(path, "r", encoding="utf-8") as handle:
        return json.load(handle)

def parse_time(value):
    value = str(value or "").strip()
    if not value:
        return None
    if re.fullmatch(r"[0-9]+(?:\.[0-9]+)?", value):
        return datetime.fromtimestamp(float(value), tz=timezone.utc)
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    try:
        parsed = datetime.fromisoformat(value)
    except ValueError as exc:
        raise ValueError(f"invalid cache time filter {value!r}; use ISO-8601 or epoch seconds") from exc
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)

cache_since_dt = parse_time(cache_since)
cache_until_dt = parse_time(cache_until)

def metadata_time(value):
    try:
        return parse_time(value)
    except ValueError:
        return None

def cache_sample_allowed(sample):
    if sample.get("source") != "interactions":
        return not (cache_provider or cache_model or cache_since_dt or cache_until_dt)
    if cache_provider and str(sample.get("provider") or "") != cache_provider:
        return False
    if cache_model and str(sample.get("model") or "") != cache_model:
        return False
    sample_time = metadata_time(sample.get("startedAt"))
    if cache_since_dt and (sample_time is None or sample_time < cache_since_dt):
        return False
    if cache_until_dt and (sample_time is None or sample_time > cache_until_dt):
        return False
    return True

def cache_summary_from_file(path):
    if not path:
        return None
    body = read_json(path)
    cache = body.get("cacheHitRate") or body.get("cache_hit_rate") or {}
    latest = cache.get("latest")
    average = cache.get("average")
    if latest is None and "latestPercent" in cache:
        latest = cache.get("latestPercent")
    if average is None and "averagePercent" in cache:
        average = cache.get("averagePercent")
    ret = {
        "path": str(path),
        "latest": float(latest) if latest is not None else None,
        "average": float(average) if average is not None else None,
        "sampleCount": int(cache.get("sampleCount") or cache.get("sample_count") or 0),
    }
    return ret

def read_metrics(path):
    values = {}
    text = Path(path).read_text(encoding="utf-8")
    for line in text.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split()
        if len(parts) != 2:
            continue
        try:
            values[parts[0]] = float(parts[1])
        except ValueError:
            continue
    return values, text

def parse_metric_samples(text):
    samples = []
    for raw in text.splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split()
        if len(parts) != 2:
            continue
        metric = parts[0]
        try:
            value = float(parts[1])
        except ValueError:
            continue
        labels = {}
        name = metric
        if "{" in metric and metric.endswith("}"):
            name, raw_labels = metric.split("{", 1)
            raw_labels = raw_labels[:-1]
            for match in re.finditer(r'([A-Za-z_][A-Za-z0-9_]*)="((?:\\.|[^"])*)"', raw_labels):
                labels[match.group(1)] = bytes(match.group(2), "utf-8").decode("unicode_escape")
        samples.append({"name": name, "labels": labels, "value": value})
    return samples

def metric_sample_sum(samples, name, labels=None):
    labels = labels or {}
    total = 0.0
    for sample in samples:
        if sample["name"] != name:
            continue
        sample_labels = sample.get("labels") or {}
        if all(sample_labels.get(key) == value for key, value in labels.items()):
            total += float(sample.get("value") or 0)
    return total

def scheduler_diagnostics_from_payload(payload, scope):
    if not payload:
        return []
    ret = []
    latest = payload.get("latestContinuation")
    if isinstance(latest, dict):
        item = dict(latest)
        item["scope"] = f"{scope}.latestContinuation"
        ret.append(item)
    for idx, raw in enumerate(payload.get("recentContinuations") or []):
        if not isinstance(raw, dict):
            continue
        item = dict(raw)
        item["scope"] = f"{scope}.recentContinuations[{idx}]"
        ret.append(item)
    return ret

def validate_projection_display_contract(projection, scope, require_workspace_revision):
    required = ["state", "globalRevision", "sourceEventRevision", "projectedSourceRevision", "updatedAt", "rebuiltAt"]
    if require_workspace_revision:
        required.append("workspaceRevision")
    missing = [key for key in required if key not in projection or projection.get(key) in (None, "")]
    issues = []
    if missing:
        issues.append(f"{scope}: projection display fields missing {missing}: {projection}")
    state = projection.get("state")
    if state not in {"ready", "stale", "degraded", "failed"}:
        issues.append(f"{scope}: projection state {state!r} is not displayable")
    source_revision = int(projection.get("sourceEventRevision") or 0)
    projected_revision = int(projection.get("projectedSourceRevision") or 0)
    if source_revision and projected_revision < source_revision:
        issues.append(f"{scope}: projectedSourceRevision behind sourceEventRevision: {projection}")
    return {
        "scope": scope,
        "state": state,
        "globalRevision": projection.get("globalRevision"),
        "workspaceRevision": projection.get("workspaceRevision"),
        "sourceEventRevision": projection.get("sourceEventRevision"),
        "projectedSourceRevision": projection.get("projectedSourceRevision"),
        "updatedAt": projection.get("updatedAt"),
        "rebuiltAt": projection.get("rebuiltAt"),
        "staleReason": projection.get("staleReason"),
        "error": projection.get("error"),
        "pass": not issues,
        "issues": issues,
    }

def validate_workspace_display_contract(workspace, scope):
    required = ["id", "title", "state"]
    missing = [key for key in required if not str(workspace.get(key) or "").strip()]
    issues = []
    if missing:
        issues.append(f"{scope}: workspace display fields missing {missing}: {workspace}")
    return {
        "scope": scope,
        "id": workspace.get("id"),
        "title": workspace.get("title"),
        "state": workspace.get("state"),
        "activeRunId": workspace.get("activeRunId"),
        "pass": not issues,
        "issues": issues,
    }

def validate_heartbeat_display_contract(heartbeat, scope):
    if not heartbeat:
        return {
            "scope": scope,
            "present": False,
            "displayFallback": "运行活动=-",
            "pass": True,
            "issues": [],
        }
    issues = []
    activity = str(heartbeat.get("activityState") or "").strip()
    summary_text = str(heartbeat.get("summary") or "").strip()
    run_id = str(heartbeat.get("runId") or "").strip()
    if not (run_id or activity or summary_text):
        issues.append(f"{scope}: runHeartbeat present but lacks runId/activityState/summary: {heartbeat}")
    return {
        "scope": scope,
        "present": True,
        "workspaceId": heartbeat.get("workspaceId"),
        "runId": heartbeat.get("runId"),
        "activityState": heartbeat.get("activityState"),
        "summary": heartbeat.get("summary"),
        "latestEffectiveProgressAt": heartbeat.get("latestEffectiveProgressAt"),
        "stalled": bool(heartbeat.get("stalled")),
        "ineffectiveRetry": bool(heartbeat.get("ineffectiveRetry")),
        "pass": not issues,
        "issues": issues,
    }

errors = []
warnings = []
readyz = read_json(artifact_dir / "readyz.json")
scheduler_diagnostics_observed = scheduler_diagnostics_from_payload(readyz.get("diagnostics") or {}, "readyz")
if readyz.get("status") != "ready":
    errors.append(f"readyz status = {readyz.get('status')!r}, want ready")
projection = readyz.get("projection") or {}
if projection.get("state") not in {"ready", "stale", "degraded"}:
    errors.append(f"readyz projection state = {projection!r}, want ready/stale/degraded")

workspaces = read_json(artifact_dir / "workspaces.json")
workspace_items = workspaces.get("workspaces") or []
workspace_set = {item.get("id") for item in workspace_items}
workspace_list_projection = workspaces.get("projection") or {}
display_contract = {
    "listProjection": validate_projection_display_contract(workspace_list_projection, "workspace-list", False),
    "workspaceListItems": [
        validate_workspace_display_contract(item, f"workspace-list[{idx}]")
        for idx, item in enumerate(workspace_items)
    ],
    "snapshots": [],
    "webConsumes": ["projectionStatusDescriptions", "projectionStatusSummary", "runHeartbeatDescriptions"],
    "tuiConsumes": ["WorkspaceSnapshotInfo", "RunHeartbeatInfo", "/status activity/latest progress"],
}
if workspace_list_projection.get("state") not in {"ready", "stale", "degraded"}:
    errors.append(f"workspace list projection state = {workspace_list_projection!r}, want ready/stale/degraded")
if "projectedSourceRevision" not in workspace_list_projection:
    errors.append("workspace list projection missing projectedSourceRevision")
if not workspace_list_projection.get("updatedAt") or not workspace_list_projection.get("rebuiltAt"):
    errors.append(f"workspace list projection timing metadata missing: {workspace_list_projection}")
if int(workspace_list_projection.get("workspaceTotal") or 0) < len(workspace_items):
    errors.append(f"workspace list projection workspaceTotal behind returned list: projection={workspace_list_projection} actual={len(workspace_items)}")

synthetic_workspace_ids = []
if synthetic_workspace_ids_path.exists():
    synthetic_workspace_ids = [
        line.strip()
        for line in synthetic_workspace_ids_path.read_text(encoding="utf-8").splitlines()
        if line.strip()
    ]
if synthetic_workspace_count > 0 and len(synthetic_workspace_ids) != synthetic_workspace_count:
    errors.append(
        f"synthetic workspace creation count mismatch: created={len(synthetic_workspace_ids)} expected={synthetic_workspace_count}"
    )

for workspace_id in workspace_ids:
    if workspace_id not in workspace_set:
        errors.append(f"workspace {workspace_id} missing from workspace projection list")
    snapshot_path = artifact_dir / f"snapshot-{workspace_id}.json"
    if snapshot_path.exists():
        snapshot_body = read_json(snapshot_path)
        snapshot_projection = snapshot_body.get("projection") or {}
        snapshot = snapshot_body.get("snapshot") or {}
        scheduler_diagnostics_observed.extend(
            scheduler_diagnostics_from_payload(snapshot.get("schedulerDiagnostics") or {}, f"workspace-snapshot[{workspace_id}]")
        )
        snapshot_display = {
            "workspaceId": workspace_id,
            "projection": validate_projection_display_contract(snapshot_projection, f"workspace-snapshot[{workspace_id}]", True),
            "workspace": validate_workspace_display_contract(snapshot.get("workspace") or {}, f"workspace-snapshot[{workspace_id}].workspace"),
            "runHeartbeat": validate_heartbeat_display_contract(snapshot.get("runHeartbeat") or {}, f"workspace-snapshot[{workspace_id}].runHeartbeat"),
        }
        display_contract["snapshots"].append(snapshot_display)
        heartbeat = snapshot.get("runHeartbeat") or {}
        if snapshot_projection.get("state") not in {"ready", "stale", "degraded"}:
            errors.append(f"{workspace_id}: snapshot projection state = {snapshot_projection!r}")
        workspace_revision = int(snapshot_projection.get("workspaceRevision") or 0)
        source_revision = int(snapshot_projection.get("sourceEventRevision") or 0)
        projected_revision = int(snapshot_projection.get("projectedSourceRevision") or 0)
        if workspace_revision <= 0:
            errors.append(f"{workspace_id}: snapshot projection workspaceRevision not positive: {snapshot_projection}")
        if "projectedSourceRevision" not in snapshot_projection:
            errors.append(f"{workspace_id}: snapshot projection missing projectedSourceRevision")
        if source_revision and projected_revision < source_revision:
            errors.append(f"{workspace_id}: snapshot projectedSourceRevision behind sourceEventRevision: {snapshot_projection}")
        if not snapshot_projection.get("updatedAt") or not snapshot_projection.get("rebuiltAt"):
            errors.append(f"{workspace_id}: snapshot projection timing metadata missing: {snapshot_projection}")
        snapshot_workspace = snapshot.get("workspace") or {}
        active_run_id = str(snapshot_workspace.get("activeRunId") or "").strip()
        if (heartbeat or active_run_id) and not heartbeat.get("activityState"):
            warnings.append(f"{workspace_id}: runHeartbeat.activityState missing or empty")

display_contract_issues = []
if not display_contract["listProjection"]["pass"]:
    display_contract_issues.extend(display_contract["listProjection"]["issues"])
for item in display_contract["workspaceListItems"]:
    if not item["pass"]:
        display_contract_issues.extend(item["issues"])
for item in display_contract["snapshots"]:
    for key in ("projection", "workspace", "runHeartbeat"):
        if not item[key]["pass"]:
            display_contract_issues.extend(item[key]["issues"])
display_contract["pass"] = not display_contract_issues
display_contract["issues"] = display_contract_issues
if display_contract_issues:
    errors.extend(display_contract_issues)
(artifact_dir / "projection-display-contract.json").write_text(json.dumps(display_contract, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

metrics_before, metrics_before_text = read_metrics(artifact_dir / "metrics-before.txt")
metrics, metrics_text = read_metrics(artifact_dir / "metrics-after.txt")
metric_samples_before = parse_metric_samples(metrics_before_text)
metric_samples = parse_metric_samples(metrics_text)
required_metric_markers = [
    "workorch_stale_dispatched_step_min_age_seconds",
    "workorch_workspace_projection_rebuild_total",
    "workorch_workspace_projection_active_run_heartbeats",
    "workorch_workspace_projection_active_run_heartbeat_age_seconds_max",
    "workorch_workspace_projection_stalled_runs",
    "workorch_workspace_projection_ineffective_retry_runs",
    "workorch_scheduler_commands_total",
    "workorch_scheduler_command_enqueue_failures_total",
    "workorch_scheduler_continuation_failures_total",
    "workorch_scheduler_continuation_no_action_total",
    "workorch_watchdog_interval_seconds",
    "workorch_watchdog_ticks_total",
    "workorch_watchdog_last_error_timestamp_seconds",
    "workorch_watchdog_stale_runtime_gap_recoveries_total",
]
for marker in required_metric_markers:
    if marker not in metrics_text:
        errors.append(f"metrics missing {marker}")
stale_dispatched_step_min_age_seconds = metrics.get("workorch_stale_dispatched_step_min_age_seconds")
if expected_stale_dispatched_step_min_age_seconds:
    expected = float(expected_stale_dispatched_step_min_age_seconds)
    if stale_dispatched_step_min_age_seconds is None:
        errors.append("metrics missing workorch_stale_dispatched_step_min_age_seconds value")
    elif abs(float(stale_dispatched_step_min_age_seconds) - expected) > 0.001:
        errors.append(
            "stale dispatched step min age metric mismatch: "
            f"got={stale_dispatched_step_min_age_seconds} expected={expected}"
        )

def metric_delta(name):
    return float(metrics.get(name) or 0) - float(metrics_before.get(name) or 0)

def labeled_metric_delta(name, labels=None):
    return metric_sample_sum(metric_samples, name, labels) - metric_sample_sum(metric_samples_before, name, labels)

pending_rebuilds = int(metrics.get("workorch_workspace_projection_pending_rebuilds") or 0)
schedule_dropped_delta = int(metric_delta("workorch_workspace_projection_schedule_dropped_total"))
if pending_rebuilds > max_pending_rebuilds:
    errors.append(
        "workspace projection pending rebuilds above threshold: "
        f"pending={pending_rebuilds} threshold={max_pending_rebuilds}"
    )
if schedule_dropped_delta > max_projection_schedule_dropped_delta:
    errors.append(
        "workspace projection schedule dropped delta above threshold: "
        f"delta={schedule_dropped_delta} threshold={max_projection_schedule_dropped_delta}"
    )

governance_resolution_delta = (
    metric_delta('workorch_http_requests_total{endpoint="decision_resolve"}')
    + metric_delta('workorch_http_requests_total{endpoint="approval_approve"}')
    + metric_delta('workorch_http_requests_total{endpoint="approval_reject"}')
)
governance_command_accepted_delta = labeled_metric_delta(
    "workorch_scheduler_commands_total",
    {"kind": "governance_continuation", "result": "accepted"},
)
replan_command_accepted_delta = labeled_metric_delta(
    "workorch_scheduler_commands_total",
    {"kind": "replan_continuation", "result": "accepted"},
)
governance_command_error_delta = sum(
    labeled_metric_delta("workorch_scheduler_commands_total", {"kind": "governance_continuation", "result": result})
    for result in ("failed", "not_accepted", "missing_response")
)
replan_command_error_delta = sum(
    labeled_metric_delta("workorch_scheduler_commands_total", {"kind": "replan_continuation", "result": result})
    for result in ("failed", "not_accepted", "missing_response")
)
governance_enqueue_failure_delta = labeled_metric_delta(
    "workorch_scheduler_command_enqueue_failures_total",
    {"kind": "governance_continuation", "reason": "scheduler_enqueue_failed"},
)
replan_enqueue_failure_delta = labeled_metric_delta(
    "workorch_scheduler_command_enqueue_failures_total",
    {"kind": "replan_continuation", "reason": "scheduler_enqueue_failed"},
)
governance_continuation_failure_delta = labeled_metric_delta(
    "workorch_scheduler_continuation_failures_total",
    {"kind": "governance_continuation"},
)
replan_continuation_failure_delta = labeled_metric_delta(
    "workorch_scheduler_continuation_failures_total",
    {"kind": "replan_continuation"},
)
governance_continuation_no_action_delta = labeled_metric_delta(
    "workorch_scheduler_continuation_no_action_total",
    {"kind": "governance_continuation"},
)
replan_continuation_no_action_delta = labeled_metric_delta(
    "workorch_scheduler_continuation_no_action_total",
    {"kind": "replan_continuation"},
)
watchdog_ticks_delta = metric_delta("workorch_watchdog_ticks_total")
watchdog_stale_recovery_delta = labeled_metric_delta("workorch_watchdog_stale_runtime_gap_recoveries_total")
governance_scheduler_diagnostics = [
    item for item in scheduler_diagnostics_observed
    if item.get("kind") in {"governance_continuation", "replan_continuation"}
]
governance_scheduler_evidence_delta = (
    governance_command_accepted_delta
    + replan_command_accepted_delta
    + governance_command_error_delta
    + replan_command_error_delta
    + governance_enqueue_failure_delta
    + replan_enqueue_failure_delta
    + governance_continuation_failure_delta
    + replan_continuation_failure_delta
    + governance_continuation_no_action_delta
    + replan_continuation_no_action_delta
)
governance_continuation_main_path = {
    "required": require_governance_continuation,
    "governanceResolutionDelta": governance_resolution_delta,
    "governanceCommandAcceptedDelta": governance_command_accepted_delta,
    "replanCommandAcceptedDelta": replan_command_accepted_delta,
    "governanceCommandErrorDelta": governance_command_error_delta,
    "replanCommandErrorDelta": replan_command_error_delta,
    "governanceEnqueueFailureDelta": governance_enqueue_failure_delta,
    "replanEnqueueFailureDelta": replan_enqueue_failure_delta,
    "governanceContinuationFailureDelta": governance_continuation_failure_delta,
    "replanContinuationFailureDelta": replan_continuation_failure_delta,
    "governanceContinuationNoActionDelta": governance_continuation_no_action_delta,
    "replanContinuationNoActionDelta": replan_continuation_no_action_delta,
    "watchdogStaleRecoveryDelta": watchdog_stale_recovery_delta,
    "diagnosticItems": governance_scheduler_diagnostics,
    "explicitSchedulerEvidence": governance_scheduler_evidence_delta > 0 or len(governance_scheduler_diagnostics) > 0,
    "hiddenWatchdogCompensationSuspected": False,
}
if governance_resolution_delta > 0 and not governance_continuation_main_path["explicitSchedulerEvidence"]:
    errors.append(
        "governance resolution observed but no scheduler accepted/error metric or continuation diagnostic was observed"
    )
if require_governance_continuation and governance_resolution_delta <= 0 and not governance_continuation_main_path["explicitSchedulerEvidence"]:
    errors.append(
        "WORKORCH_OBSERVE_REQUIRE_GOVERNANCE_CONTINUATION is enabled but no governance continuation evidence was observed"
    )
if governance_resolution_delta > 0 and watchdog_stale_recovery_delta > 0 and not governance_continuation_main_path["explicitSchedulerEvidence"]:
    governance_continuation_main_path["hiddenWatchdogCompensationSuspected"] = True
    errors.append(
        "governance continuation appears to rely on watchdog stale-runtime recovery without scheduler evidence"
    )

events_path = artifact_dir / "sse-events.jsonl"
topic_counts = {}
workspace_event_counts = {}
workspace_topic_counts = {}
if events_path.exists():
    with open(events_path, "r", encoding="utf-8") as handle:
        for line in handle:
            if not line.strip():
                continue
            item = json.loads(line)
            topic = item.get("topic") or ""
            workspace_id = item.get("workspaceId") or (item.get("payload") or {}).get("workspaceId") or ""
            topic_counts[topic] = topic_counts.get(topic, 0) + 1
            if workspace_id:
                workspace_event_counts[workspace_id] = workspace_event_counts.get(workspace_id, 0) + 1
                topic_key = f"{workspace_id}|{topic}"
                workspace_topic_counts[topic_key] = workspace_topic_counts.get(topic_key, 0) + 1
else:
    warnings.append("no SSE events captured during observation window")

if synthetic_workspace_ids:
    for workspace_id in synthetic_workspace_ids:
        created_count = workspace_topic_counts.get(f"{workspace_id}|/workspace/created", 0)
        projection_count = workspace_topic_counts.get(f"{workspace_id}|/workorchd/workspace/projection/changed", 0)
        if created_count <= 0:
            errors.append(f"{workspace_id}: synthetic observation missing /workspace/created SSE event")
        if projection_count <= 0:
            errors.append(f"{workspace_id}: synthetic observation missing scoped projection changed SSE event")

cache_samples = []
if ai_proxy_log:
    log_path = Path(ai_proxy_log)
    if not log_path.exists():
        errors.append(f"ai-proxy log not found: {ai_proxy_log}")
    else:
        text = log_path.read_text(encoding="utf-8", errors="replace")
        for match in re.finditer(r"cache_hit_rate=([0-9]+(?:\.[0-9]+)?)%", text):
            sample = {
                "source": "log",
                "cacheHitRatePercent": float(match.group(1)),
            }
            if cache_sample_allowed(sample):
                cache_samples.append(sample)

if ai_proxy_interactions_dir:
    interactions_path = Path(ai_proxy_interactions_dir)
    if not interactions_path.exists():
        errors.append(f"ai-proxy interactions dir not found: {ai_proxy_interactions_dir}")
    else:
        for metadata_path in sorted(interactions_path.glob("*/metadata.json")):
            try:
                metadata = read_json(metadata_path)
            except Exception as exc:
                warnings.append(f"skip invalid ai-proxy metadata {metadata_path}: {exc}")
                continue
            if metadata.get("error"):
                continue
            if int(metadata.get("total_tokens") or 0) <= 0:
                continue
            if "cache_hit_rate" not in metadata:
                continue
            try:
                rate = float(metadata.get("cache_hit_rate") or 0)
            except (TypeError, ValueError):
                continue
            if rate <= 1:
                rate *= 100
            sample = {
                "source": "interactions",
                "id": metadata.get("id"),
                "startedAt": metadata.get("started_at"),
                "provider": metadata.get("provider"),
                "model": metadata.get("model"),
                "stream": bool(metadata.get("stream")),
                "estimated": bool(metadata.get("estimated")),
                "inputTokens": int(metadata.get("input_tokens") or 0),
                "outputTokens": int(metadata.get("output_tokens") or 0),
                "cachedInputTokens": int(metadata.get("cached_input_tokens") or 0),
                "cacheHitRatePercent": rate,
            }
            if cache_sample_allowed(sample):
                cache_samples.append(sample)

if require_cache_samples and not cache_samples:
    errors.append(
        "WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES is enabled but no cache_hit_rate samples were found; "
        "set WORKORCH_OBSERVE_AI_PROXY_LOG or WORKORCH_OBSERVE_AI_PROXY_INTERACTIONS_DIR"
    )

cache_rates = [float(item.get("cacheHitRatePercent") or 0) for item in cache_samples]
if min_cache_sample_count:
    threshold = int(min_cache_sample_count)
    if len(cache_rates) < threshold:
        errors.append(f"cache sample count below threshold {threshold}: samples={len(cache_rates)}")
if min_cache_hit_rate and cache_rates:
    threshold = float(min_cache_hit_rate)
    latest = cache_rates[-1]
    average = sum(cache_rates) / len(cache_rates)
    if latest < threshold and average < threshold:
        errors.append(
            f"cache hit rate below threshold {threshold:.2f}%: latest={latest:.2f}% average={average:.2f}% samples={len(cache_rates)}"
        )

cache_baseline = None
cache_delta = None
if cache_baseline_file:
    baseline_path = Path(cache_baseline_file)
    if not baseline_path.exists():
        errors.append(f"cache baseline summary not found: {cache_baseline_file}")
    else:
        try:
            cache_baseline = cache_summary_from_file(baseline_path)
            current_latest = cache_rates[-1] if cache_rates else None
            current_average = (sum(cache_rates) / len(cache_rates)) if cache_rates else None
            cache_delta = {
                "latest": (current_latest - cache_baseline["latest"]) if current_latest is not None and cache_baseline.get("latest") is not None else None,
                "average": (current_average - cache_baseline["average"]) if current_average is not None and cache_baseline.get("average") is not None else None,
            }
            if min_cache_average_delta:
                threshold = float(min_cache_average_delta)
                if cache_delta.get("average") is None or cache_delta["average"] < threshold:
                    errors.append(f"cache average delta below threshold {threshold:.2f}: delta={cache_delta.get('average')}")
            if min_cache_latest_delta:
                threshold = float(min_cache_latest_delta)
                if cache_delta.get("latest") is None or cache_delta["latest"] < threshold:
                    errors.append(f"cache latest delta below threshold {threshold:.2f}: delta={cache_delta.get('latest')}")
        except Exception as exc:
            errors.append(f"failed to read cache baseline summary {cache_baseline_file}: {exc}")

pprof_summary = {
    "enabled": enable_pprof_check,
    "available": False,
    "artifact": None,
    "errorArtifact": None,
    "runtimeAdapterInObserverLaneFound": False,
    "synchronousRuntimeSendStackFound": False,
    "recursiveDispatchStackFound": False,
    "runtimeAdapterObserverLaneStacks": 0,
    "synchronousRuntimeSendStacks": 0,
    "dispatchGraphStepBatchFrames": 0,
    "dispatchRuntimeRepairFollowUpFrames": 0,
    "eventHubSendFrames": 0,
    "executeRuntimeStepFrames": 0,
    "issues": [],
}
if enable_pprof_check:
    pprof_path = artifact_dir / "pprof-goroutine.txt"
    pprof_error_path = artifact_dir / "pprof-goroutine-error.txt"
    if pprof_path.exists() and pprof_path.stat().st_size > 0:
        pprof_text = pprof_path.read_text(encoding="utf-8", errors="replace")
        goroutine_stacks = [item for item in re.split(r"\n\s*\n", pprof_text) if item.strip()]
        graph_count = pprof_text.count("dispatchGraphStepBatchWithWorkspace")
        repair_count = pprof_text.count("dispatchRuntimeRepairFollowUp")
        send_count = len(re.findall(r"\.(?:Hub\.)?Send\(", pprof_text)) + pprof_text.count("event.Hub.Send")
        execute_count = pprof_text.count("ExecuteRuntimeStep")
        adapter_observer_lane_stacks = 0
        sync_runtime_send_stacks = 0
        for stack in goroutine_stacks:
            has_runtimeevent_observer = (
                "runtimeevent.(*Executor).handle" in stack
                or "runtimeevent.(*Executor).Handle" in stack
            )
            has_runtime_adapter = "ExecuteRuntimeStep" in stack
            has_runtimeevent_worker = "runtimeevent.(*Executor).execute" in stack
            if has_runtimeevent_observer and has_runtime_adapter and not has_runtimeevent_worker:
                adapter_observer_lane_stacks += 1
            has_sync_runtime_bridge = (
                "eventRuntimeAdapter.ExecuteRuntimeStep" in stack
                or "RuntimeAdapterRegistry.ExecuteRuntimeStep" in stack
            )
            has_eventhub_send = (
                "MagicHub.Publish" in stack
                or "event.Hub.Send" in stack
                or re.search(r"\.(?:Hub\.)?Send\(", stack) is not None
            )
            if has_sync_runtime_bridge and has_eventhub_send:
                sync_runtime_send_stacks += 1
        recursive_found = (
            graph_count >= 2
            or repair_count >= 2
            or ((graph_count > 0 or repair_count > 0) and send_count > 0 and execute_count > 0)
        )
        pprof_summary.update({
            "available": True,
            "artifact": "pprof-goroutine.txt",
            "runtimeAdapterInObserverLaneFound": adapter_observer_lane_stacks > 0,
            "synchronousRuntimeSendStackFound": sync_runtime_send_stacks > 0,
            "runtimeAdapterObserverLaneStacks": adapter_observer_lane_stacks,
            "synchronousRuntimeSendStacks": sync_runtime_send_stacks,
            "dispatchGraphStepBatchFrames": graph_count,
            "dispatchRuntimeRepairFollowUpFrames": repair_count,
            "eventHubSendFrames": send_count,
            "executeRuntimeStepFrames": execute_count,
            "recursiveDispatchStackFound": recursive_found,
        })
        if adapter_observer_lane_stacks > 0:
            issue = (
                "pprof goroutine shows runtime adapter executing inside runtimeevent observer lane: "
                f"stacks={adapter_observer_lane_stacks}"
            )
            pprof_summary["issues"].append(issue)
            errors.append(issue)
        if sync_runtime_send_stacks > 0:
            issue = (
                "pprof goroutine shows synchronous runtime EventHub Send bridge stack: "
                f"stacks={sync_runtime_send_stacks}"
            )
            pprof_summary["issues"].append(issue)
            errors.append(issue)
        if recursive_found:
            issue = (
                "pprof goroutine shows recursive runtime dispatch stack: "
                f"dispatchGraphStepBatchWithWorkspace={graph_count}, "
                f"dispatchRuntimeRepairFollowUp={repair_count}, "
                f"eventHubSend={send_count}, ExecuteRuntimeStep={execute_count}"
            )
            pprof_summary["issues"].append(issue)
            errors.append(issue)
    else:
        pprof_summary["errorArtifact"] = "pprof-goroutine-error.txt" if pprof_error_path.exists() else None
        message = "pprof goroutine profile unavailable; runtime recursive dispatch stack was not observed"
        pprof_summary["issues"].append(message)
        if require_pprof:
            errors.append(message)
        else:
            warnings.append(message)

cpu_profile_summary = {
    "enabled": enable_cpu_profile_check,
    "available": False,
    "profileArtifact": None,
    "topArtifact": None,
    "errorArtifact": None,
    "suspiciousHotspots": [],
    "thresholds": {
        "maxFlatPercent": max_suspicious_cpu_flat_percent,
        "maxCumPercent": max_suspicious_cpu_cum_percent,
    },
    "issues": [],
}
if enable_cpu_profile_check:
    cpu_profile_path = artifact_dir / "pprof-cpu.pb.gz"
    cpu_top_path = artifact_dir / "pprof-cpu-top.txt"
    cpu_error_path = artifact_dir / "pprof-cpu-error.txt"
    cpu_top_error_path = artifact_dir / "pprof-cpu-top-error.txt"
    if cpu_profile_path.exists() and cpu_profile_path.stat().st_size > 0:
        cpu_profile_summary["available"] = True
        cpu_profile_summary["profileArtifact"] = "pprof-cpu.pb.gz"
        if cpu_top_path.exists() and cpu_top_path.stat().st_size > 0:
            cpu_profile_summary["topArtifact"] = "pprof-cpu-top.txt"
            suspicious_patterns = [
                "readEventPayload",
                "recoverStaleDispatched",
                "RecoverStale",
                "workspaceviewcore.(*Manager).Rebuild",
                "workspaceviewcore.(*Manager).rebuild",
                "workspaceviewcore.(*Store)",
                "dispatchGraphStepBatchWithWorkspace",
                "dispatchRuntimeRepairFollowUp",
            ]
            for raw_line in cpu_top_path.read_text(encoding="utf-8", errors="replace").splitlines():
                line = raw_line.strip()
                if not line or not any(pattern in line for pattern in suspicious_patterns):
                    continue
                percents = [float(value) for value in re.findall(r"([0-9]+(?:\.[0-9]+)?)%", line)]
                flat_percent = percents[0] if percents else 0.0
                cum_percent = percents[-1] if len(percents) >= 2 else flat_percent
                hotspot = {
                    "line": line,
                    "flatPercent": flat_percent,
                    "cumPercent": cum_percent,
                }
                cpu_profile_summary["suspiciousHotspots"].append(hotspot)
                if flat_percent > max_suspicious_cpu_flat_percent or cum_percent > max_suspicious_cpu_cum_percent:
                    issue = (
                        "CPU profile hotspot above threshold: "
                        f"flat={flat_percent:.2f}% cum={cum_percent:.2f}% line={line}"
                    )
                    cpu_profile_summary["issues"].append(issue)
                    errors.append(issue)
        else:
            cpu_profile_summary["errorArtifact"] = "pprof-cpu-top-error.txt" if cpu_top_error_path.exists() else None
            message = "CPU profile captured but top summary unavailable"
            cpu_profile_summary["issues"].append(message)
            if require_cpu_profile:
                errors.append(message)
            else:
                warnings.append(message)
    else:
        cpu_profile_summary["errorArtifact"] = "pprof-cpu-error.txt" if cpu_error_path.exists() else None
        message = "pprof CPU profile unavailable; runtime CPU hotspot gate was not observed"
        cpu_profile_summary["issues"].append(message)
        if require_cpu_profile:
            errors.append(message)
        else:
            warnings.append(message)

summary = {
    "serviceReady": readyz.get("status") == "ready",
    "projectionState": projection.get("state"),
    "workspaceTotal": len(workspace_items),
    "observedWorkspaceIds": workspace_ids,
    "syntheticWorkspaceIds": synthetic_workspace_ids,
    "sseTopicCounts": topic_counts,
    "sseWorkspaceEventCounts": workspace_event_counts,
    "sseWorkspaceTopicCounts": workspace_topic_counts,
    "metrics": {
        "activeRunHeartbeats": metrics.get("workorch_workspace_projection_active_run_heartbeats", 0),
        "activeRunHeartbeatAgeSecondsMax": metrics.get("workorch_workspace_projection_active_run_heartbeat_age_seconds_max", 0),
        "stalledRuns": metrics.get("workorch_workspace_projection_stalled_runs", 0),
        "ineffectiveRetryRuns": metrics.get("workorch_workspace_projection_ineffective_retry_runs", 0),
        "staleDispatchedStepMinAgeSeconds": stale_dispatched_step_min_age_seconds,
        "serverMaxConcurrentRuns": metrics.get("workorch_server_max_concurrent_runs", 0),
        "runRequestsInFlight": metrics.get("workorch_run_requests_in_flight", 0),
        "runQueueWaiting": metrics.get("workorch_run_queue_waiting", 0),
        "watchdogIntervalSeconds": metrics.get("workorch_watchdog_interval_seconds", 0),
        "projectionPendingRebuilds": pending_rebuilds,
        "projectionScheduleDroppedDelta": schedule_dropped_delta,
        "projectionRebuildDelta": {
            "full": metric_delta("workorch_workspace_projection_rebuild_total{scope=\"full\"}"),
            "debounced": metric_delta("workorch_workspace_projection_debounced_rebuild_total"),
        },
        "scheduler": {
            "governanceResolutionDelta": governance_resolution_delta,
            "governanceCommandAcceptedDelta": governance_command_accepted_delta,
            "replanCommandAcceptedDelta": replan_command_accepted_delta,
            "governanceCommandErrorDelta": governance_command_error_delta,
            "replanCommandErrorDelta": replan_command_error_delta,
            "governanceEnqueueFailureDelta": governance_enqueue_failure_delta,
            "replanEnqueueFailureDelta": replan_enqueue_failure_delta,
            "governanceContinuationFailureDelta": governance_continuation_failure_delta,
            "replanContinuationFailureDelta": replan_continuation_failure_delta,
            "governanceContinuationNoActionDelta": governance_continuation_no_action_delta,
            "replanContinuationNoActionDelta": replan_continuation_no_action_delta,
            "watchdogTicksDelta": watchdog_ticks_delta,
            "watchdogStaleRecoveryDelta": watchdog_stale_recovery_delta,
        },
    },
    "governanceContinuationMainPath": governance_continuation_main_path,
    "displayContract": {
        "pass": display_contract.get("pass"),
        "snapshotCount": len(display_contract.get("snapshots") or []),
        "issues": display_contract.get("issues") or [],
        "artifact": "projection-display-contract.json",
    },
    "cacheHitRate": {
        "sampleCount": len(cache_rates),
        "latest": cache_rates[-1] if cache_rates else None,
        "average": (sum(cache_rates) / len(cache_rates)) if cache_rates else None,
        "minRequired": float(min_cache_hit_rate) if min_cache_hit_rate else None,
        "minSampleCount": int(min_cache_sample_count) if min_cache_sample_count else None,
        "filters": {
            "provider": cache_provider or None,
            "model": cache_model or None,
            "since": cache_since or None,
            "until": cache_until or None,
        },
        "baseline": cache_baseline,
        "delta": cache_delta,
        "minAverageDelta": float(min_cache_average_delta) if min_cache_average_delta else None,
        "minLatestDelta": float(min_cache_latest_delta) if min_cache_latest_delta else None,
        "latestSample": cache_samples[-1] if cache_samples else None,
        "sources": sorted({str(item.get("source") or "") for item in cache_samples if item.get("source")}),
    },
    "pprof": pprof_summary,
    "cpuProfile": cpu_profile_summary,
    "warnings": warnings,
    "errors": errors,
}
(artifact_dir / "runtime-observation-summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps(summary, ensure_ascii=False, indent=2))

if errors:
    raise SystemExit(1)
PY

echo "workorch runtime observation completed for $BASE_URL artifacts=$TMP_DIR"
