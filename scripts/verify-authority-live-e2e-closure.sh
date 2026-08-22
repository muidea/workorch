#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_AUTHORITY_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-}}"
TMP_DIR="${WORKORCH_AUTHORITY_VERIFY_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_AUTHORITY_VERIFY_KEEP_ARTIFACTS:-1}"
CURL_CONNECT_TIMEOUT="${WORKORCH_VERIFY_CURL_CONNECT_TIMEOUT_SECONDS:-5}"
CURL_MAX_TIME="${WORKORCH_VERIFY_CURL_MAX_TIME_SECONDS:-30}"
MIN_WORKSPACE_COUNT="${WORKORCH_AUTHORITY_VERIFY_MIN_WORKSPACE_COUNT:-1}"
WAIT_SECONDS="${WORKORCH_AUTHORITY_VERIFY_WAIT_SECONDS:-5}"
WAIT_INTERVAL_SECONDS="${WORKORCH_AUTHORITY_VERIFY_WAIT_INTERVAL_SECONDS:-1}"
REQUIRE_PROJECTION_ALIGNED="${WORKORCH_AUTHORITY_VERIFY_REQUIRE_PROJECTION_ALIGNED:-1}"
MAX_METADATA_RECORDS_SCANNED_DELTA="${WORKORCH_AUTHORITY_VERIFY_MAX_METADATA_RECORDS_SCANNED_DELTA:-}"
MAX_PAYLOAD_HYDRATES_DELTA="${WORKORCH_AUTHORITY_VERIFY_MAX_PAYLOAD_HYDRATES_DELTA:-}"
MAX_PAYLOAD_HYDRATE_BYTES_DELTA="${WORKORCH_AUTHORITY_VERIFY_MAX_PAYLOAD_HYDRATE_BYTES_DELTA:-}"
MAX_WORKSPACE_DETAIL_FETCHES="${WORKORCH_AUTHORITY_VERIFY_MAX_WORKSPACE_DETAIL_FETCHES:-20}"

RESUME_WORKSPACE_ID="${WORKORCH_AUTHORITY_VERIFY_RESUME_WORKSPACE_ID:-}"
RESUME_INTENT="${WORKORCH_AUTHORITY_VERIFY_RESUME_INTENT:-resume}"
RESUME_REQUEST_ID="${WORKORCH_AUTHORITY_VERIFY_RESUME_REQUEST_ID:-authority-live-verify-resume}"
RESUME_REASON="${WORKORCH_AUTHORITY_VERIFY_RESUME_REASON:-authority_live_verification}"

DECISION_WORKSPACE_ID="${WORKORCH_AUTHORITY_VERIFY_DECISION_WORKSPACE_ID:-}"
DECISION_TICKET_ID="${WORKORCH_AUTHORITY_VERIFY_DECISION_TICKET_ID:-}"
DECISION_RESOLUTION_TYPE="${WORKORCH_AUTHORITY_VERIFY_DECISION_RESOLUTION_TYPE:-}"
DECISION_PAYLOAD_JSON="${WORKORCH_AUTHORITY_VERIFY_DECISION_PAYLOAD_JSON:-}"

if [[ -n "$RESUME_WORKSPACE_ID" || -n "$DECISION_WORKSPACE_ID" ]]; then
  source "$ROOT_DIR/scripts/lib/verify-isolated-runtime.sh"
  workorch_verify_require_isolated_runtime "$BASE_URL" "verify-authority-live-e2e-closure mutation mode"
fi

mkdir -p "$TMP_DIR"

cleanup() {
  local status=$?
  if [[ "$KEEP_ARTIFACTS" != "1" && "$KEEP_ARTIFACTS" != "true" && $status -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  else
    echo "authority live verification artifacts: $TMP_DIR" >&2
  fi
}

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "authority live verification failed for deployed workorch service $BASE_URL" >&2
  echo "artifacts=$TMP_DIR" >&2
  for file in "$TMP_DIR"/report.json "$TMP_DIR"/readyz-before.json "$TMP_DIR"/workspaces-before.json "$TMP_DIR"/readyz-after.json "$TMP_DIR"/workspaces-after.json; do
    [[ -f "$file" ]] || continue
    echo "--- $file ---" >&2
    python3 -m json.tool "$file" >&2 || cat "$file" >&2
  done
}

trap report_failure ERR
trap cleanup EXIT

cd "$ROOT_DIR"

curl_fetch() {
  local output="$1"
  local url="$2"
  shift 2
  command curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -fsS "$url" "$@" >"$output"
}

curl_post_json() {
  local output="$1"
  local url="$2"
  local body_file="$3"
  command curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -fsS \
    -X POST "$url" \
    -H 'Content-Type: application/json' \
    -d @"$body_file" >"$output"
}

capture_baseline() {
  local suffix="$1"
  curl_fetch "$TMP_DIR/readyz-$suffix.json" "$BASE_URL/readyz"
  curl_fetch "$TMP_DIR/workspaces-$suffix.json" "$BASE_URL/api/v1/workorch/workspaces"
  curl_fetch "$TMP_DIR/metrics-$suffix.txt" "$BASE_URL/metrics"
}

write_resume_body() {
  python3 - "$TMP_DIR/resume-body.json" "$RESUME_INTENT" "$RESUME_REQUEST_ID" "$RESUME_REASON" <<'PY'
import json
import sys

output, intent, request_id, reason = sys.argv[1:]
body = {
    "intent": intent or "resume",
    "requestId": request_id,
    "reason": reason,
    "source": "authority-live-verification",
}
with open(output, "w", encoding="utf-8") as handle:
    json.dump(body, handle, ensure_ascii=False, indent=2)
    handle.write("\n")
PY
}

write_decision_body() {
  python3 - "$TMP_DIR/decision-body.json" "$DECISION_RESOLUTION_TYPE" "$DECISION_PAYLOAD_JSON" <<'PY'
import json
import sys

output, resolution_type, payload_json = sys.argv[1:]
payload = {}
if payload_json.strip():
    payload = json.loads(payload_json)
body = {
    "resolutionType": resolution_type,
    "resolutionPayload": payload,
}
with open(output, "w", encoding="utf-8") as handle:
    json.dump(body, handle, ensure_ascii=False, indent=2)
    handle.write("\n")
PY
}

capture_baseline before

COMMAND_KIND=""
COMMAND_STATUS="not_requested"
COMMAND_ERROR=""

if [[ -n "$RESUME_WORKSPACE_ID" ]]; then
  COMMAND_KIND="resume"
  if [[ -z "$RESUME_WORKSPACE_ID" ]]; then
    COMMAND_STATUS="invalid_request"
    COMMAND_ERROR="resume verification requires WORKORCH_AUTHORITY_VERIFY_RESUME_WORKSPACE_ID"
  else
    write_resume_body
    curl_post_json "$TMP_DIR/resume-response.json" \
      "$BASE_URL/api/v1/workorch/workspaces/$RESUME_WORKSPACE_ID/execution-activations" \
      "$TMP_DIR/resume-body.json"
    COMMAND_STATUS="submitted"
  fi
fi

if [[ -n "$DECISION_WORKSPACE_ID" || -n "$DECISION_TICKET_ID" || -n "$DECISION_RESOLUTION_TYPE" ]]; then
  if [[ "$COMMAND_KIND" != "" ]]; then
    COMMAND_STATUS="invalid_request"
    COMMAND_ERROR="specify either resume command verification or decision command verification, not both"
  else
    COMMAND_KIND="decision"
    if [[ -z "$DECISION_WORKSPACE_ID" || -z "$DECISION_TICKET_ID" || -z "$DECISION_RESOLUTION_TYPE" ]]; then
      COMMAND_STATUS="invalid_request"
      COMMAND_ERROR="decision verification requires WORKORCH_AUTHORITY_VERIFY_DECISION_WORKSPACE_ID, WORKORCH_AUTHORITY_VERIFY_DECISION_TICKET_ID, and WORKORCH_AUTHORITY_VERIFY_DECISION_RESOLUTION_TYPE"
    else
      write_decision_body
      curl_post_json "$TMP_DIR/decision-response.json" \
        "$BASE_URL/api/v1/workorch/decisions/$DECISION_TICKET_ID/resolve?workspaceId=$DECISION_WORKSPACE_ID" \
        "$TMP_DIR/decision-body.json"
      COMMAND_STATUS="submitted"
    fi
  fi
fi

if [[ "$COMMAND_STATUS" == "submitted" ]]; then
  python3 - "$WAIT_SECONDS" "$WAIT_INTERVAL_SECONDS" <<'PY'
import sys
import time

deadline = time.monotonic() + max(0.0, float(sys.argv[1]))
interval = max(0.1, float(sys.argv[2]))
while time.monotonic() < deadline:
    time.sleep(interval)
PY
fi

capture_baseline after

python3 - \
  "$TMP_DIR" \
  "$BASE_URL" \
  "$MIN_WORKSPACE_COUNT" \
  "$REQUIRE_PROJECTION_ALIGNED" \
  "$COMMAND_KIND" \
  "$COMMAND_STATUS" \
  "$COMMAND_ERROR" \
  "$RESUME_WORKSPACE_ID" \
  "$RESUME_REQUEST_ID" \
  "$RESUME_INTENT" \
  "$DECISION_WORKSPACE_ID" \
  "$DECISION_TICKET_ID" \
  "$MAX_METADATA_RECORDS_SCANNED_DELTA" \
  "$MAX_PAYLOAD_HYDRATES_DELTA" \
  "$MAX_PAYLOAD_HYDRATE_BYTES_DELTA" \
  "$MAX_WORKSPACE_DETAIL_FETCHES" <<'PY'
import json
import re
import sys
import urllib.parse
import urllib.request
from pathlib import Path

(
    artifact_dir,
    base_url,
    min_workspace_count,
    require_projection_aligned,
    command_kind,
    command_status,
    command_error,
    resume_workspace_id,
    resume_request_id,
    resume_intent,
    decision_workspace_id,
    decision_ticket_id,
    max_metadata_records_delta,
    max_payload_hydrates_delta,
    max_payload_hydrate_bytes_delta,
    max_workspace_detail_fetches,
) = sys.argv[1:]

root = Path(artifact_dir)
min_workspace_count = int(min_workspace_count or "0")
require_projection_aligned = str(require_projection_aligned).lower() not in {"0", "false", "no"}
max_workspace_detail_fetches = max(0, int(max_workspace_detail_fetches or "0"))

def load_json(name):
    return json.loads((root / name).read_text(encoding="utf-8"))

def fetch_json_path(path, artifact_name):
    url = base_url.rstrip("/") + path
    try:
        with urllib.request.urlopen(url, timeout=30) as response:
            data = response.read()
    except Exception as exc:
        return None, str(exc)
    if artifact_name:
        (root / artifact_name).write_bytes(data)
    try:
        return json.loads(data.decode("utf-8")), ""
    except Exception as exc:
        return None, f"decode {path}: {exc}"

def parse_metrics(name):
    text = (root / name).read_text(encoding="utf-8")
    values = {}
    labeled = {}
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
        if "{" in metric and metric.endswith("}"):
            base = metric[:metric.index("{")]
            labeled[metric] = value
            values.setdefault(base, 0.0)
            values[base] += value
        else:
            values[metric] = value
    return values, labeled, text

def metric_delta(before, after, name):
    return float(after.get(name, 0.0) or 0.0) - float(before.get(name, 0.0) or 0.0)

def optional_limit(value):
    value = str(value or "").strip()
    if not value:
        return None
    return float(value)

def projection_summary(data):
    projection = data.get("projection") or {}
    return {
        "state": projection.get("state"),
        "globalRevision": projection.get("globalRevision"),
        "sourceEventRevision": projection.get("sourceEventRevision"),
        "projectedSourceRevision": projection.get("projectedSourceRevision"),
        "workspaceTotal": projection.get("workspaceTotal"),
        "updatedAt": projection.get("updatedAt"),
    }

def workspace_index(data):
    return {str(item.get("id") or ""): item for item in data.get("workspaces") or [] if item.get("id")}

def compact_workspace(item):
    if not item:
        return None
    view = item.get("viewModel") or {}
    return {
        "id": item.get("id"),
        "state": item.get("state"),
        "activeRunId": item.get("activeRunId"),
        "blockingTicketId": item.get("blockingTicketId"),
        "blockingApprovalId": item.get("blockingApprovalId"),
        "promptMode": view.get("promptMode"),
        "viewModelRevision": view.get("revision"),
        "sourceEventRevision": view.get("sourceEventRevision"),
        "projectedSourceRevision": view.get("projectedSourceRevision"),
    }

def compact_run(item):
    if not item:
        return None
    return {
        "id": item.get("id"),
        "workspaceId": item.get("workspaceId"),
        "state": item.get("state"),
        "blockingTicketId": item.get("blockingTicketId"),
        "blockingApprovalId": item.get("blockingApprovalId"),
        "failureReason": item.get("failureReason"),
    }

def compact_decision(item):
    if not item:
        return None
    return {
        "id": item.get("id"),
        "workspaceId": item.get("workspaceId"),
        "runId": item.get("runId"),
        "state": item.get("state"),
        "resolutionType": item.get("resolutionType"),
    }

def compact_run_command(item):
    if not item:
        return None
    return {
        "workspaceId": item.get("workspaceId"),
        "runId": item.get("runId"),
        "state": item.get("state"),
        "intent": item.get("intent"),
        "disposition": item.get("disposition"),
        "requestId": item.get("requestId"),
        "definitionRevision": item.get("definitionRevision"),
        "decisionId": item.get("decisionId"),
        "acceptedAt": item.get("acceptedAt"),
        "rejectedAt": item.get("rejectedAt"),
        "revision": item.get("revision"),
    }

def command_target_id():
    if command_kind == "resume":
        return resume_workspace_id
    if command_kind == "decision":
        return decision_workspace_id
    return ""

def command_progress(before_item, after_item, authority_readback):
    if command_status != "submitted":
        return {"required": False, "advanced": None, "reason": command_status}
    if not before_item or not after_item:
        return {"required": True, "advanced": False, "reason": "target_workspace_missing_after_command"}
    target_id = command_target_id()
    owner_readback = authority_readback.get(target_id) or {}
    if command_kind == "decision":
        decision = decision_by_id(owner_readback.get("decisions") or []).get(decision_ticket_id)
        resolved = bool(decision and decision.get("state") == "resolved")
        return {
            "required": True,
            "advanced": resolved,
            "reason": "owner_decision_resolved" if resolved else "owner_decision_not_resolved",
            "decision": decision,
        }
    if command_kind == "resume":
        run_command = owner_readback.get("runCommand") or {}
        state = str(run_command.get("state") or "")
        accepted_at = str(run_command.get("acceptedAt") or "")
        rejected_at = str(run_command.get("rejectedAt") or "")
        mismatch = ""
        if not run_command:
            mismatch = "matching_run_command_missing"
        elif run_command.get("workspaceId") != resume_workspace_id:
            mismatch = "run_command_workspace_mismatch"
        elif run_command.get("requestId") != resume_request_id:
            mismatch = "run_command_request_mismatch"
        elif run_command.get("intent") != resume_intent:
            mismatch = "run_command_intent_mismatch"
        elif state not in {"accepted", "rejected"}:
            mismatch = "run_command_outcome_invalid"
        elif state == "accepted" and (not accepted_at or rejected_at):
            mismatch = "run_command_accepted_timestamp_invalid"
        elif state == "rejected" and (not rejected_at or accepted_at):
            mismatch = "run_command_rejected_timestamp_invalid"
        return {
            "required": True,
            "advanced": not mismatch,
            "reason": mismatch or "workspace_owner_admission_outcome_observed",
            "runCommand": run_command,
        }
    before = compact_workspace(before_item)
    after = compact_workspace(after_item)
    advanced = False
    reasons = []
    for key in ("state", "activeRunId", "blockingTicketId", "blockingApprovalId", "promptMode", "viewModelRevision"):
        if before.get(key) != after.get(key):
            advanced = True
            reasons.append(f"{key}_changed")
    after_state = str(after.get("state") or "")
    before_blocker = str(before.get("blockingTicketId") or before.get("blockingApprovalId") or "")
    after_blocker = str(after.get("blockingTicketId") or after.get("blockingApprovalId") or "")
    if after_state in {"running", "completed", "failed", "waiting_approval"}:
        advanced = True
        reasons.append(f"state_{after_state}")
    if after_state == "waiting_decision" and after_blocker and after_blocker != before_blocker:
        advanced = True
        reasons.append("new_governance_blocker")
    return {
        "required": True,
        "advanced": advanced,
        "reason": ",".join(reasons) if reasons else "owner_state_not_advanced",
        "before": before,
        "after": after,
    }

def workspace_needs_authority_fetch(item):
    view = item.get("viewModel") or {}
    return bool(
        item.get("state") == "waiting_decision" or
        view.get("promptMode") == "decision" or
        item.get("blockingTicketId") or
        item.get("activeRunId")
    )

def selected_authority_workspaces(items):
    ordered = []
    seen = set()
    target_id = command_target_id() if command_status == "submitted" else ""
    if target_id:
        for item in items:
            workspace_id = str(item.get("id") or "")
            if workspace_id == target_id:
                ordered.append(item)
                seen.add(workspace_id)
                break
    for item in items:
        if workspace_needs_authority_fetch(item):
            workspace_id = item.get("id")
            if workspace_id and workspace_id not in seen:
                ordered.append(item)
                seen.add(workspace_id)
    for item in items:
        workspace_id = item.get("id")
        if workspace_id and workspace_id not in seen:
            ordered.append(item)
            seen.add(workspace_id)
    limit = max_workspace_detail_fetches
    if target_id:
        limit = max(1, limit)
    return ordered[:limit]

def build_authority_readback(workspaces):
    readback = {}
    for item in selected_authority_workspaces(workspaces):
        workspace_id = str(item.get("id") or "")
        if not workspace_id:
            continue
        encoded_workspace = urllib.parse.quote(workspace_id, safe="")
        decisions, decision_err = fetch_json_path(
            f"/api/v1/workorch/decisions?workspaceId={encoded_workspace}",
            f"decisions-{encoded_workspace}.json",
        )
        runs, run_err = fetch_json_path(
            f"/api/v1/workorch/runs?workspaceId={encoded_workspace}",
            f"runs-{encoded_workspace}.json",
        )
        snapshot, snapshot_err = fetch_json_path(
            f"/api/v1/workorch/workspaces/{encoded_workspace}/snapshot",
            f"snapshot-{encoded_workspace}.json",
        )
        snapshot_value = (snapshot or {}).get("snapshot") or {}
        readback[workspace_id] = {
            "workspace": compact_workspace(item),
            "decisions": [compact_decision(decision) for decision in (decisions or {}).get("decisions", [])],
            "runs": [compact_run(run) for run in (runs or {}).get("runs", [])],
            "runCommand": compact_run_command(snapshot_value.get("runCommand")),
            "errors": [err for err in (decision_err, run_err, snapshot_err) if err],
        }
    return readback

def decision_by_id(items):
    return {str(item.get("id") or ""): item for item in items if item and item.get("id")}

def run_by_id(items):
    return {str(item.get("id") or ""): item for item in items if item and item.get("id")}

def open_decision(decision, workspace_id, run_id, ticket_id):
    return bool(
        decision and
        decision.get("id") == ticket_id and
        decision.get("workspaceId") == workspace_id and
        decision.get("runId") == run_id and
        decision.get("state") == "open"
    )

def authority_consistency_checks(readback):
    checks = []
    errors = []
    for workspace_id, data in sorted(readback.items()):
        workspace = data.get("workspace") or {}
        decisions = data.get("decisions") or []
        runs = data.get("runs") or []
        decisions_by_id = decision_by_id(decisions)
        runs_by_id = run_by_id(runs)
        active_run_id = str(workspace.get("activeRunId") or "")
        ticket_id = str(workspace.get("blockingTicketId") or "")
        prompt_mode = str(workspace.get("promptMode") or "")
        state = str(workspace.get("state") or "")
        active_run = runs_by_id.get(active_run_id)
        decision = decisions_by_id.get(ticket_id)
        run_command = data.get("runCommand")

        if data.get("errors"):
            for err in data["errors"]:
                errors.append(f"authority readback failed for {workspace_id}: {err}")

        if state == "waiting_decision" or prompt_mode == "decision":
            if not ticket_id:
                reason = "workspace_waiting_decision_without_ticket"
                errors.append(f"{workspace_id}: {reason}")
            elif not open_decision(decision, workspace_id, active_run_id, ticket_id):
                decision_state = (decision or {}).get("state") or "missing"
                reason = f"workspace_decision_prompt_without_open_ticket:{decision_state}"
                errors.append(f"{workspace_id}/{active_run_id}/{ticket_id}: {reason}")
            else:
                reason = "open_decision"
            checks.append({
                "kind": "workspace_decision_prompt",
                "workspaceId": workspace_id,
                "runId": active_run_id,
                "ticketId": ticket_id,
                "ticketState": (decision or {}).get("state"),
                "promptMode": prompt_mode,
                "state": state,
                "result": reason,
            })

        if active_run and active_run.get("state") == "blocked_on_decision":
            run_ticket_id = str(active_run.get("blockingTicketId") or "")
            run_decision = decisions_by_id.get(run_ticket_id)
            if not run_ticket_id:
                reason = "run_blocked_on_decision_without_ticket"
                errors.append(f"{workspace_id}/{active_run_id}: {reason}")
            elif not open_decision(run_decision, workspace_id, active_run_id, run_ticket_id):
                decision_state = (run_decision or {}).get("state") or "missing"
                reason = f"run_blocked_on_non_open_decision:{decision_state}"
                errors.append(f"{workspace_id}/{active_run_id}/{run_ticket_id}: {reason}")
            else:
                reason = "open_decision"
            checks.append({
                "kind": "run_decision_blocker",
                "workspaceId": workspace_id,
                "runId": active_run_id,
                "ticketId": run_ticket_id,
                "ticketState": (run_decision or {}).get("state"),
                "runState": active_run.get("state"),
                "result": reason,
            })

        if run_command:
            command_state = str(run_command.get("state") or "")
            accepted_at = str(run_command.get("acceptedAt") or "")
            rejected_at = str(run_command.get("rejectedAt") or "")
            result = "immutable_admission_outcome"
            if run_command.get("workspaceId") != workspace_id:
                result = "admission_outcome_scope_mismatch"
            elif command_state not in {"accepted", "rejected"}:
                result = "invalid_admission_outcome"
            elif command_state == "accepted" and (not accepted_at or rejected_at):
                result = "invalid_accepted_admission_timestamp"
            elif command_state == "rejected" and (not rejected_at or accepted_at):
                result = "invalid_rejected_admission_timestamp"
            if result != "immutable_admission_outcome":
                errors.append(f"{workspace_id}: {result}")
            checks.append({
                "kind": "execution_admission_outcome",
                "workspaceId": workspace_id,
                "runId": run_command.get("runId"),
                "requestId": run_command.get("requestId"),
                "intent": run_command.get("intent"),
                "state": command_state,
                "disposition": run_command.get("disposition"),
                "result": result,
            })
    return checks, errors

ready_before = load_json("readyz-before.json")
ready_after = load_json("readyz-after.json")
workspaces_before = load_json("workspaces-before.json")
workspaces_after = load_json("workspaces-after.json")
metrics_before, _, metrics_text_before = parse_metrics("metrics-before.txt")
metrics_after, _, metrics_text_after = parse_metrics("metrics-after.txt")

errors = []
warnings = []

if ready_before.get("status") != "ready":
    errors.append(f"readyz before status={ready_before.get('status')!r}, want ready")
if ready_after.get("status") != "ready":
    errors.append(f"readyz after status={ready_after.get('status')!r}, want ready")

before_projection = projection_summary(workspaces_before)
after_projection = projection_summary(workspaces_after)
if before_projection.get("state") != "ready":
    errors.append(f"workspace projection before state={before_projection.get('state')!r}, want ready")
if after_projection.get("state") != "ready":
    errors.append(f"workspace projection after state={after_projection.get('state')!r}, want ready")
if require_projection_aligned:
    for label, projection in (("before", before_projection), ("after", after_projection)):
        if projection.get("sourceEventRevision") != projection.get("projectedSourceRevision"):
            errors.append(
                f"projection {label} source/projected mismatch: "
                f"{projection.get('sourceEventRevision')} != {projection.get('projectedSourceRevision')}"
            )

workspace_count = len(workspaces_after.get("workspaces") or [])
if workspace_count < min_workspace_count:
    errors.append(f"workspace count={workspace_count}, want >= {min_workspace_count}")

required_metric_markers = [
    "workorch_workspace_projection_rebuild_total",
    "workorch_event_record_metadata_scans_total",
    "workorch_event_record_payload_hydrates_total",
    "workorch_event_record_payload_hydrate_bytes_total",
    "workorch_execution_admission_commands_total",
    "workorch_execution_admission_failures_total",
]
for marker in required_metric_markers:
    if marker not in metrics_text_after:
        errors.append(f"metrics missing marker {marker}")

metric_deltas = {
    "eventMetadataScans": metric_delta(metrics_before, metrics_after, "workorch_event_record_metadata_scans_total"),
    "eventMetadataRecordsScanned": metric_delta(metrics_before, metrics_after, "workorch_event_record_metadata_records_scanned_total"),
    "eventPayloadHydrates": metric_delta(metrics_before, metrics_after, "workorch_event_record_payload_hydrates_total"),
    "eventPayloadHydrateBytes": metric_delta(metrics_before, metrics_after, "workorch_event_record_payload_hydrate_bytes_total"),
    "projectionScheduleDropped": metric_delta(metrics_before, metrics_after, "workorch_workspace_projection_schedule_dropped_total"),
    "ineffectiveRetryRuns": metric_delta(metrics_before, metrics_after, "workorch_workspace_projection_ineffective_retry_runs"),
}
limits = {
    "eventMetadataRecordsScanned": optional_limit(max_metadata_records_delta),
    "eventPayloadHydrates": optional_limit(max_payload_hydrates_delta),
    "eventPayloadHydrateBytes": optional_limit(max_payload_hydrate_bytes_delta),
}
for key, limit in limits.items():
    if limit is not None and metric_deltas.get(key, 0.0) > limit:
        errors.append(f"metric delta {key}={metric_deltas.get(key)}, max {limit}")

before_by_id = workspace_index(workspaces_before)
after_by_id = workspace_index(workspaces_after)
target_workspace_id = command_target_id()
authority_readback = build_authority_readback(workspaces_after.get("workspaces") or [])
(root / "authority-readback-after.json").write_text(
    json.dumps(authority_readback, ensure_ascii=False, indent=2) + "\n",
    encoding="utf-8",
)
progress = command_progress(
    before_by_id.get(target_workspace_id),
    after_by_id.get(target_workspace_id),
    authority_readback,
) if target_workspace_id else {
    "required": False,
    "advanced": None,
    "reason": "no_command_requested",
}
if command_error:
    errors.append(command_error)
if progress.get("required") and not progress.get("advanced"):
    errors.append(f"command {command_kind} accepted but owner progress was not observed: {progress.get('reason')}")

authority_checks, authority_errors = authority_consistency_checks(authority_readback)
errors.extend(authority_errors)
if len(authority_readback) < len(workspaces_after.get("workspaces") or []):
    warnings.append(
        f"authority readback fetched {len(authority_readback)} workspace(s); "
        f"set WORKORCH_AUTHORITY_VERIFY_MAX_WORKSPACE_DETAIL_FETCHES to scan more"
    )

waiting_decisions = []
for item in workspaces_after.get("workspaces") or []:
    if item.get("state") == "waiting_decision":
        blocker = str(item.get("blockingTicketId") or "")
        readback = authority_readback.get(str(item.get("id") or "")) or {}
        decision = decision_by_id(readback.get("decisions") or []).get(blocker)
        cause = "decision_required"
        if "contract" in blocker or "violation" in blocker or "retry" in blocker:
            cause = "contract_retry_or_violation"
        waiting_decisions.append({
            "workspaceId": item.get("id"),
            "runId": item.get("activeRunId"),
            "blockingTicketId": blocker,
            "ticketState": (decision or {}).get("state"),
            "resolutionType": (decision or {}).get("resolutionType"),
            "cause": cause,
            "promptMode": (item.get("viewModel") or {}).get("promptMode"),
        })

if waiting_decisions and command_status != "submitted":
    warnings.append("waiting_decision workspaces observed; run with explicit decision or resume command env vars to verify owner progress")

report = {
    "baseUrl": base_url,
    "artifactDir": artifact_dir,
    "pass": not errors,
    "errors": errors,
    "warnings": warnings,
    "command": {
        "kind": command_kind or "none",
        "status": command_status,
        "targetWorkspaceId": target_workspace_id,
        "decisionTicketId": decision_ticket_id if command_kind == "decision" else "",
        "progress": progress,
    },
    "projection": {
        "before": before_projection,
        "after": after_projection,
    },
    "workspaceCount": workspace_count,
    "waitingDecisions": waiting_decisions,
    "executionAdmissionOutcomes": [
        item.get("runCommand")
        for _, item in sorted(authority_readback.items())
        if item.get("runCommand")
    ],
    "authorityChecks": authority_checks,
    "authorityReadbackWorkspaceCount": len(authority_readback),
    "metrics": {
        "before": {key: metrics_before.get(key) for key in sorted(metrics_before) if key.startswith("workorch_")},
        "after": {key: metrics_after.get(key) for key in sorted(metrics_after) if key.startswith("workorch_")},
        "delta": metric_deltas,
    },
}
(root / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps(report, ensure_ascii=False, indent=2))
if errors:
    raise SystemExit(1)
PY

echo "authority live verification passed baseUrl=$BASE_URL artifacts=$TMP_DIR"
