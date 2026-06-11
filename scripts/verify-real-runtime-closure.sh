#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_REAL_RUNTIME_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
TMP_DIR="${WORKORCH_REAL_RUNTIME_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_REAL_RUNTIME_KEEP_ARTIFACTS:-1}"
LOAD_TOTAL="${WORKORCH_REAL_RUNTIME_LOAD_TOTAL:-${WORKORCH_LOAD_TOTAL:-2}}"
LOAD_CONCURRENCY="${WORKORCH_REAL_RUNTIME_LOAD_CONCURRENCY:-${WORKORCH_LOAD_CONCURRENCY:-2}}"
LOAD_MIN_SUCCESS_RATE="${WORKORCH_REAL_RUNTIME_LOAD_MIN_SUCCESS_RATE:-${WORKORCH_LOAD_MIN_SUCCESS_RATE:-1.0}}"
LOAD_REAL_RUN_TIMEOUT_SECONDS="${WORKORCH_REAL_RUNTIME_LOAD_RUN_TIMEOUT_SECONDS:-${WORKORCH_LOAD_REAL_RUN_TIMEOUT_SECONDS:-900}}"
LOAD_REAL_PROVIDER_TIMEOUT="${WORKORCH_REAL_RUNTIME_PROVIDER_TIMEOUT:-${WORKORCH_LOAD_REAL_PROVIDER_TIMEOUT:-180s}}"
OBSERVE_DURATION_SECONDS="${WORKORCH_REAL_RUNTIME_OBSERVE_DURATION_SECONDS:-20}"
OBSERVE_REQUIRE_CACHE_SAMPLES="${WORKORCH_REAL_RUNTIME_REQUIRE_CACHE_SAMPLES:-1}"
OBSERVE_MIN_CACHE_SAMPLE_COUNT="${WORKORCH_REAL_RUNTIME_MIN_CACHE_SAMPLE_COUNT:-1}"
OBSERVE_CACHE_PROVIDER="${WORKORCH_REAL_RUNTIME_CACHE_PROVIDER:-${WORKORCH_OBSERVE_CACHE_PROVIDER:-}}"
OBSERVE_CACHE_MODEL="${WORKORCH_REAL_RUNTIME_CACHE_MODEL:-${WORKORCH_OBSERVE_CACHE_MODEL:-}}"
OBSERVE_CACHE_BASELINE_FILE="${WORKORCH_REAL_RUNTIME_CACHE_BASELINE_FILE:-${WORKORCH_OBSERVE_CACHE_BASELINE_FILE:-}}"
OBSERVE_MIN_CACHE_AVERAGE_DELTA="${WORKORCH_REAL_RUNTIME_MIN_CACHE_AVERAGE_DELTA:-${WORKORCH_OBSERVE_MIN_CACHE_AVERAGE_DELTA:-}}"
OBSERVE_MIN_CACHE_LATEST_DELTA="${WORKORCH_REAL_RUNTIME_MIN_CACHE_LATEST_DELTA:-${WORKORCH_OBSERVE_MIN_CACHE_LATEST_DELTA:-}}"
OBSERVE_STALE_DISPATCHED_STEP_MIN_AGE_SECONDS="${WORKORCH_REAL_RUNTIME_STALE_DISPATCHED_STEP_MIN_AGE_SECONDS:-${WORKORCH_OBSERVE_STALE_DISPATCHED_STEP_MIN_AGE_SECONDS:-}}"
AI_PROXY_LOG="${WORKORCH_REAL_RUNTIME_AI_PROXY_LOG:-${WORKORCH_OBSERVE_AI_PROXY_LOG:-}}"
AI_PROXY_INTERACTIONS_DIR="${WORKORCH_REAL_RUNTIME_AI_PROXY_INTERACTIONS_DIR:-${WORKORCH_OBSERVE_AI_PROXY_INTERACTIONS_DIR:-}}"
PREFLIGHT_ONLY="${WORKORCH_REAL_RUNTIME_PREFLIGHT_ONLY:-0}"

mkdir -p "$TMP_DIR"

cleanup() {
  local status=$?
  if [[ "$KEEP_ARTIFACTS" != "1" && "$KEEP_ARTIFACTS" != "true" && $status -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  else
    echo "real runtime closure artifacts: $TMP_DIR" >&2
  fi
}

trap cleanup EXIT

cd "$ROOT_DIR"

preflight_artifact_dir="$TMP_DIR/preflight"
mkdir -p "$preflight_artifact_dir"

echo "real runtime closure preflight starting baseUrl=$BASE_URL"
curl -fsS "$BASE_URL/readyz" >"$preflight_artifact_dir/readyz.json"
curl -fsS "$BASE_URL/metrics" >"$preflight_artifact_dir/metrics.txt"
python3 - "$preflight_artifact_dir" "$LOAD_REAL_RUN_TIMEOUT_SECONDS" <<'PY'
import json
import re
import sys
from pathlib import Path

artifact_dir = Path(sys.argv[1])
run_timeout_value = sys.argv[2].strip()

def parse_duration_seconds(value):
    value = str(value or "").strip()
    if not value:
        raise ValueError("empty duration")
    if re.fullmatch(r"[0-9]+(?:\.[0-9]+)?", value):
        return float(value)
    match = re.fullmatch(r"([0-9]+(?:\.[0-9]+)?)(ns|us|µs|ms|s|m|h)", value)
    if not match:
        raise ValueError(f"invalid duration {value!r}; use seconds or Go-style ns/us/ms/s/m/h")
    number = float(match.group(1))
    unit = match.group(2)
    factors = {
        "ns": 1 / 1_000_000_000,
        "us": 1 / 1_000_000,
        "µs": 1 / 1_000_000,
        "ms": 1 / 1_000,
        "s": 1,
        "m": 60,
        "h": 3600,
    }
    return number * factors[unit]

def parse_metrics(path):
    values = {}
    text = path.read_text(encoding="utf-8")
    for raw in text.splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split()
        if len(parts) != 2:
            continue
        try:
            values[parts[0]] = float(parts[1])
        except ValueError:
            continue
    return values

errors = []
readyz = json.loads((artifact_dir / "readyz.json").read_text(encoding="utf-8"))
if readyz.get("status") != "ready":
    errors.append(f"readyz status = {readyz.get('status')!r}, want ready")

metrics = parse_metrics(artifact_dir / "metrics.txt")
stale_age = metrics.get("workorch_stale_dispatched_step_min_age_seconds")
if stale_age is None:
    errors.append("metrics missing workorch_stale_dispatched_step_min_age_seconds")
run_timeout_seconds = parse_duration_seconds(run_timeout_value)
if stale_age is not None and stale_age >= run_timeout_seconds:
    errors.append(
        "stale dispatched watchdog threshold is not shorter than real runtime load wait window: "
        f"workorch_stale_dispatched_step_min_age_seconds={stale_age:.0f}, "
        f"WORKORCH_LOAD_REAL_RUN_TIMEOUT_SECONDS={run_timeout_seconds:.0f}. "
        "Set server.staleDispatchedStepMinAge below the load wait window, or use "
        "WORKORCH_MANAGED_STALE_DISPATCHED_STEP_MIN_AGE for managed verification."
    )

summary = {
    "serviceReady": readyz.get("status") == "ready",
    "runTimeoutSeconds": run_timeout_seconds,
    "staleDispatchedStepMinAgeSeconds": stale_age,
    "pass": not errors,
    "errors": errors,
}
(artifact_dir / "preflight-summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps(summary, ensure_ascii=False, indent=2))
if errors:
    raise SystemExit(1)
PY

if [[ "$PREFLIGHT_ONLY" == "1" || "$PREFLIGHT_ONLY" == "true" || "$PREFLIGHT_ONLY" == "yes" ]]; then
  echo "real runtime closure preflight passed baseUrl=$BASE_URL artifacts=$preflight_artifact_dir"
  exit 0
fi

cache_since="$(python3 - <<'PY'
from datetime import datetime, timezone
print(datetime.now(timezone.utc).isoformat())
PY
)"

load_artifact_dir="$TMP_DIR/loadtest"
observe_artifact_dir="$TMP_DIR/observe-after"

echo "real runtime closure loadtest starting baseUrl=$BASE_URL total=$LOAD_TOTAL concurrency=$LOAD_CONCURRENCY"
WORKORCH_BASE_URL="$BASE_URL" \
WORKORCH_LOAD_BASE_URL="$BASE_URL" \
WORKORCH_LOAD_LLM_MODE=real \
WORKORCH_LOAD_TOTAL="$LOAD_TOTAL" \
WORKORCH_LOAD_CONCURRENCY="$LOAD_CONCURRENCY" \
WORKORCH_LOAD_REAL_RUN_TIMEOUT_SECONDS="$LOAD_REAL_RUN_TIMEOUT_SECONDS" \
WORKORCH_LOAD_REAL_PROVIDER_TIMEOUT="$LOAD_REAL_PROVIDER_TIMEOUT" \
WORKORCH_LOAD_EXPECT_ACTIVE_OVERLAP="${WORKORCH_REAL_RUNTIME_EXPECT_ACTIVE_OVERLAP:-${WORKORCH_LOAD_EXPECT_ACTIVE_OVERLAP:-1}}" \
WORKORCH_LOAD_MIN_SUCCESS_RATE="$LOAD_MIN_SUCCESS_RATE" \
WORKORCH_LOAD_KEEP_ARTIFACTS=1 \
WORKORCH_LOAD_ARTIFACT_DIR="$load_artifact_dir" \
./scripts/loadtest-governed-workspace-concurrency.sh

workspace_ids="$(python3 - "$load_artifact_dir/summary.json" <<'PY'
import json
import sys

summary = json.load(open(sys.argv[1], encoding="utf-8"))
ids = []
for item in summary.get("results") or []:
    workspace_id = item.get("workspaceId") or ""
    if workspace_id:
        ids.append(workspace_id)
print(",".join(ids))
PY
)"

echo "real runtime closure observation starting workspaces=$workspace_ids cacheSince=$cache_since"
WORKORCH_BASE_URL="$BASE_URL" \
WORKORCH_OBSERVE_BASE_URL="$BASE_URL" \
WORKORCH_OBSERVE_ARTIFACT_DIR="$observe_artifact_dir" \
WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 \
WORKORCH_OBSERVE_DURATION_SECONDS="$OBSERVE_DURATION_SECONDS" \
WORKORCH_OBSERVE_WORKSPACE_IDS="$workspace_ids" \
WORKORCH_OBSERVE_AI_PROXY_LOG="$AI_PROXY_LOG" \
WORKORCH_OBSERVE_AI_PROXY_INTERACTIONS_DIR="$AI_PROXY_INTERACTIONS_DIR" \
WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES="$OBSERVE_REQUIRE_CACHE_SAMPLES" \
WORKORCH_OBSERVE_MIN_CACHE_SAMPLE_COUNT="$OBSERVE_MIN_CACHE_SAMPLE_COUNT" \
WORKORCH_OBSERVE_CACHE_PROVIDER="$OBSERVE_CACHE_PROVIDER" \
WORKORCH_OBSERVE_CACHE_MODEL="$OBSERVE_CACHE_MODEL" \
WORKORCH_OBSERVE_CACHE_SINCE="${WORKORCH_REAL_RUNTIME_CACHE_SINCE:-$cache_since}" \
WORKORCH_OBSERVE_CACHE_BASELINE_FILE="$OBSERVE_CACHE_BASELINE_FILE" \
WORKORCH_OBSERVE_MIN_CACHE_AVERAGE_DELTA="$OBSERVE_MIN_CACHE_AVERAGE_DELTA" \
WORKORCH_OBSERVE_MIN_CACHE_LATEST_DELTA="$OBSERVE_MIN_CACHE_LATEST_DELTA" \
WORKORCH_OBSERVE_STALE_DISPATCHED_STEP_MIN_AGE_SECONDS="$OBSERVE_STALE_DISPATCHED_STEP_MIN_AGE_SECONDS" \
./scripts/observe-workorch-runtime-closure.sh

python3 - "$TMP_DIR" "$load_artifact_dir/summary.json" "$observe_artifact_dir/runtime-observation-summary.json" <<'PY'
import json
import sys
from pathlib import Path

artifact_dir = Path(sys.argv[1])
load_summary = json.load(open(sys.argv[2], encoding="utf-8"))
observe_summary = json.load(open(sys.argv[3], encoding="utf-8"))
summary = {
    "baseUrl": load_summary.get("baseUrl"),
    "load": {
        "batchId": load_summary.get("batchId"),
        "llmMode": load_summary.get("llmMode"),
        "total": load_summary.get("total"),
        "concurrency": load_summary.get("concurrency"),
        "workspaceSuccessTotal": load_summary.get("workspaceSuccessTotal"),
        "workspaceFailedTotal": load_summary.get("workspaceFailedTotal"),
        "successRate": load_summary.get("successRate"),
        "activeRunOverlapPass": load_summary.get("activeRunOverlapPass"),
        "pass": load_summary.get("pass"),
    },
    "observation": {
        "projectionState": observe_summary.get("projectionState"),
        "workspaceTotal": observe_summary.get("workspaceTotal"),
        "observedWorkspaceIds": observe_summary.get("observedWorkspaceIds"),
        "metrics": observe_summary.get("metrics"),
        "cacheHitRate": observe_summary.get("cacheHitRate"),
        "warnings": observe_summary.get("warnings"),
        "errors": observe_summary.get("errors"),
    },
}
(artifact_dir / "real-runtime-closure-summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps(summary, ensure_ascii=False, indent=2))
if not load_summary.get("pass"):
    raise SystemExit("real runtime loadtest did not pass")
if observe_summary.get("errors"):
    raise SystemExit("real runtime observation reported errors")
PY

echo "real runtime closure verification passed baseUrl=$BASE_URL artifacts=$TMP_DIR"
