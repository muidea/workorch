#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKSPACE_ID="${1:-${WORKORCH_RUNTIME_VERIFY_WORKSPACE_ID:-}}"
RUN_ID="${2:-${WORKORCH_RUNTIME_VERIFY_RUN_ID:-}}"
DATA_ROOT="${WORKORCH_RUNTIME_VERIFY_DATA_ROOT:-$ROOT_DIR/.workorch-data}"

if [[ -z "$WORKSPACE_ID" || -z "$RUN_ID" ]]; then
  echo "usage: $0 <workspace-id> <run-id>" >&2
  echo "or set WORKORCH_RUNTIME_VERIFY_WORKSPACE_ID and WORKORCH_RUNTIME_VERIFY_RUN_ID" >&2
  exit 2
fi

RUN_DIR="$DATA_ROOT/workspaces/$WORKSPACE_ID/runs/$RUN_ID"
WORKSPACE_FILE="$DATA_ROOT/workspaces/$WORKSPACE_ID/workspace.json"
RUN_FILE="$RUN_DIR/run.json"
RESULT_FILE="$RUN_DIR/result.json"

if [[ ! -f "$WORKSPACE_FILE" ]]; then
  echo "missing workspace file: $WORKSPACE_FILE" >&2
  exit 1
fi
if [[ ! -f "$RUN_FILE" ]]; then
  echo "missing run file: $RUN_FILE" >&2
  exit 1
fi

python3 - "$WORKSPACE_FILE" "$RUN_FILE" "$RESULT_FILE" "$WORKSPACE_ID" "$RUN_ID" <<'PY'
import json
import os
import sys

workspace_path, run_path, result_path, workspace_id, run_id = sys.argv[1:]

def load_items(path):
    if not os.path.exists(path):
        return []
    with open(path, "r", encoding="utf-8") as handle:
        data = json.load(handle)
    if isinstance(data, dict):
        return list(data.values())
    if isinstance(data, list):
        return data
    raise SystemExit(f"unsupported json root in {path}: {type(data).__name__}")

def field(item, *names):
    if not isinstance(item, dict):
        return None
    for name in names:
        if name in item:
            return item[name]
    return None

def payload(item, *names):
    for name in names:
        value = field(item, name, name[:1].upper() + name[1:])
        if isinstance(value, dict):
            return value
    return {}

def value_list(item, *names):
    for name in names:
        value = field(item, name, name[:1].upper() + name[1:])
        if isinstance(value, list):
            return value
    return []

def profile_contract(run):
    context = payload(run, "executionContext")
    contract = payload(context, "executionContract")
    profiles = payload(contract, "profileContracts")
    return payload(profiles, "software-dev")

def is_software_dev_run(workspace, run, profile):
    if profile:
        return True
    hints = payload(workspace, "contextHints")
    contract = payload(hints, "executionContract")
    profiles = payload(contract, "profileContracts")
    if payload(profiles, "software-dev"):
        return True
    for key in ("templateId", "TemplateID", "profile", "Profile"):
        if str(workspace.get(key, "")).strip() == "software-dev":
            return True
        if str(run.get(key, "")).strip() == "software-dev":
            return True
    return False

def assessment_from_result(result):
    assessment = payload(result, "doDAssessment")
    if assessment:
        return assessment
    summary = payload(result, "summaryDetail")
    if summary:
        assessment = payload(summary, "doDAssessment")
        if assessment:
            return assessment
    return {}

def coverage_items(assessment, key):
    coverage = payload(assessment, key)
    return coverage, value_list(coverage, "items")

def required_ids(profile, key):
    ret = []
    for item in value_list(profile, key):
        if not isinstance(item, dict):
            continue
        item_id = str(item.get("id") or "").strip()
        if item_id and item.get("required") is True:
            ret.append(item_id)
    return ret

def item_by_id(items):
    ret = {}
    for item in items:
        if not isinstance(item, dict):
            continue
        item_id = str(item.get("id") or "").strip()
        if item_id:
            ret[item_id] = item
    return ret

def evidence_refs(item):
    return value_list(item, "evidenceRefs", "artifactRefs")

def validate_required_coverage(errors, assessment, profile, profile_key, assessment_key, evidence_kind):
    expected = required_ids(profile, profile_key)
    if not expected:
        errors.append(f"software-dev profileContract.{profile_key} has no required ids")
        return
    coverage, items = coverage_items(assessment, assessment_key)
    if not isinstance(coverage, dict):
        errors.append(f"accepted software-dev result missing dodAssessment.{assessment_key}")
        return
    by_id = item_by_id(items)
    missing = [item_id for item_id in expected if item_id not in by_id]
    if missing:
        errors.append(f"accepted software-dev result missing {assessment_key} ids: {', '.join(missing)}")
    for item_id in expected:
        item = by_id.get(item_id)
        if not isinstance(item, dict):
            continue
        state = str(item.get("state") or "").strip()
        if state != "satisfied":
            errors.append(f"accepted software-dev {assessment_key}.{item_id} is not satisfied")
        refs = value_list(item, evidence_kind)
        if not refs:
            errors.append(f"accepted software-dev {assessment_key}.{item_id} missing {evidence_kind}")

workspaces = load_items(workspace_path)
runs = load_items(run_path)
results = load_items(result_path)
workspace = next((item for item in workspaces if field(item, "id", "ID") == workspace_id), workspaces[0] if workspaces else None)
run = next((item for item in runs if field(item, "id", "ID") == run_id), runs[0] if runs else None)
if not workspace:
    raise SystemExit(f"workspace {workspace_id} not found in {workspace_path}")
if not run:
    raise SystemExit(f"run {run_id} not found in {run_path}")

profile = profile_contract(run)
if not is_software_dev_run(workspace, run, profile):
    print(f"workorch software-dev profile verification skipped for non-software-dev run {run_id}")
    raise SystemExit(0)

errors = []
if not profile:
    errors.append("software-dev run missing executionContract.profileContracts.software-dev")
else:
    for key in ("acceptanceDimensions", "deliverableExpectations", "evidenceRequirements"):
        if not value_list(profile, key):
            errors.append(f"software-dev profileContract.{key} is empty")

accepted_results = [
    item for item in results
    if field(item, "runId", "RunID") == run_id and field(item, "state", "State") == "accepted"
]
if str(field(run, "state", "State") or "") == "completed" and not accepted_results:
    errors.append("completed software-dev run has no accepted result")

for result in accepted_results:
    result_id = str(field(result, "id", "ID") or "<unknown>")
    assessment = assessment_from_result(result)
    if not assessment:
        errors.append(f"accepted software-dev result missing dodAssessment: {result_id}")
        continue
    goal_coverage, goal_items = coverage_items(assessment, "goalCoverage")
    if not isinstance(goal_coverage, dict):
        errors.append(f"accepted software-dev result missing dodAssessment.goalCoverage: {result_id}")
    elif goal_coverage.get("satisfied") is not True:
        errors.append(f"accepted software-dev result goalCoverage is not satisfied: {result_id}")
    elif not goal_items:
        errors.append(f"accepted software-dev result goalCoverage has no items: {result_id}")
    else:
        for item in goal_items:
            if not isinstance(item, dict):
                continue
            if str(item.get("state") or "").strip() == "satisfied" and not value_list(item, "evidenceRefs"):
                errors.append(f"accepted software-dev result goalCoverage item missing evidenceRefs: {result_id}")
    if profile:
        validate_required_coverage(errors, assessment, profile, "acceptanceDimensions", "acceptanceDimensions", "evidenceRefs")
        validate_required_coverage(errors, assessment, profile, "deliverableExpectations", "deliverableCoverage", "artifactRefs")

if errors:
    for message in errors:
        print(f"error: {message}", file=sys.stderr)
    raise SystemExit(1)

print(f"workorch software-dev profile verification passed for workspace={workspace_id} run={run_id}")
PY
