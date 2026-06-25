#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SNAPSHOT_FILE=""
RESULTS_FILE=""
if [[ "${1:-}" == "--snapshot" ]]; then
  SNAPSHOT_FILE="${2:-}"
  RESULTS_FILE="${3:-}"
  shift 3 || true
fi
WORKSPACE_ID="${1:-${WORKORCH_RUNTIME_VERIFY_WORKSPACE_ID:-}}"
RUN_ID="${2:-${WORKORCH_RUNTIME_VERIFY_RUN_ID:-}}"
DATA_ROOT="${WORKORCH_RUNTIME_VERIFY_DATA_ROOT:-$ROOT_DIR/.workorch-data}"

if [[ -n "$SNAPSHOT_FILE" ]]; then
  if [[ ! -f "$SNAPSHOT_FILE" ]]; then
    echo "missing snapshot file: $SNAPSHOT_FILE" >&2
    exit 1
  fi
  if [[ -n "$RESULTS_FILE" && ! -f "$RESULTS_FILE" ]]; then
    echo "missing results file: $RESULTS_FILE" >&2
    exit 1
  fi
elif [[ -z "$WORKSPACE_ID" || -z "$RUN_ID" ]]; then
  echo "usage: $0 <workspace-id> <run-id>" >&2
  echo "   or: $0 --snapshot <run-snapshot.json> [run-results.json]" >&2
  echo "or set WORKORCH_RUNTIME_VERIFY_WORKSPACE_ID and WORKORCH_RUNTIME_VERIFY_RUN_ID" >&2
  exit 2
fi

if [[ -z "$SNAPSHOT_FILE" ]]; then
  RUN_DIR="$DATA_ROOT/workspaces/$WORKSPACE_ID/runs/$RUN_ID"
  WORKSPACE_FILE="$DATA_ROOT/workspaces/$WORKSPACE_ID/workspace.json"
  RUN_FILE="$RUN_DIR/run.json"
  STEP_FILE="$RUN_DIR/step.json"
  RESULT_FILE="$RUN_DIR/result.json"
  if [[ ! -f "$STEP_FILE" && -d "$RUN_DIR/steps" ]]; then
    STEP_FILE="$RUN_DIR/steps"
  fi
  if [[ ! -f "$RESULT_FILE" && -d "$RUN_DIR/results" ]]; then
    RESULT_FILE="$RUN_DIR/results"
  fi

  if [[ ! -f "$WORKSPACE_FILE" ]]; then
    echo "missing workspace file: $WORKSPACE_FILE" >&2
    exit 1
  fi
  if [[ ! -f "$RUN_FILE" ]]; then
    echo "missing run file: $RUN_FILE" >&2
    exit 1
  fi
fi
python3 - "$SNAPSHOT_FILE" "${RESULTS_FILE:-}" "${WORKSPACE_FILE:-}" "${RUN_FILE:-}" "${STEP_FILE:-}" "${RESULT_FILE:-}" "${WORKSPACE_ID:-}" "${RUN_ID:-}" <<'PY'
import json
import os
import sys

snapshot_path, results_page_path, workspace_path, run_path, step_path, result_path, workspace_id, run_id = sys.argv[1:]

def load_items(path):
    if not path or not os.path.exists(path):
        return []
    if os.path.isdir(path):
        ret = []
        for root, dirs, files in os.walk(path):
            dirs.sort()
            for name in sorted(files):
                if not name.endswith(".json"):
                    continue
                child = os.path.join(root, name)
                try:
                    ret.extend(load_items(child))
                except Exception as exc:
                    raise SystemExit(f"failed to load {child}: {exc}") from exc
        return ret
    with open(path, "r", encoding="utf-8") as handle:
        data = json.load(handle)
    if isinstance(data, dict):
        for key in ("id", "ID", "runId", "RunID", "workspaceId", "WorkspaceID"):
            if key in data:
                return [data]
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

def unwrap_payload(data, key):
    if isinstance(data, dict) and isinstance(data.get(key), dict):
        return data.get(key)
    return data

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

def load_snapshot_fixture(snapshot_path, results_page_path):
    if not snapshot_path:
        return None
    with open(snapshot_path, "r", encoding="utf-8") as handle:
        data = json.load(handle)
    snapshot = unwrap_payload(data, "snapshot")
    if not isinstance(snapshot, dict):
        raise SystemExit(f"unsupported snapshot json root in {snapshot_path}: {type(data).__name__}")
    workspace = payload(snapshot, "workspace")
    run = payload(snapshot, "run")
    steps = value_list(snapshot, "steps")
    results = value_list(snapshot, "results")
    if results_page_path:
        with open(results_page_path, "r", encoding="utf-8") as handle:
            page = json.load(handle)
        page_results = value_list(page, "results")
        if page_results:
            results = page_results
    return workspace, run, steps, results

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

def state_value(item):
    return str(field(item, "state", "State") or "").strip()

def bool_field(item, *names):
    value = field(item, *names)
    return value is True

def required_ids(profile, key):
    ret = []
    for item in value_list(profile, key):
        if not isinstance(item, dict):
            continue
        item_id = str(field(item, "id", "ID") or "").strip()
        if item_id and field(item, "required", "Required") is True:
            ret.append(item_id)
    return ret

def item_by_id(items):
    ret = {}
    for item in items:
        if not isinstance(item, dict):
            continue
        item_id = str(field(item, "id", "ID") or "").strip()
        if item_id:
            ret[item_id] = item
    return ret

def evidence_refs(item):
    return value_list(item, "evidenceRefs", "artifactRefs")

def string_list(value):
    if not isinstance(value, list):
        return []
    ret = []
    for item in value:
        text = str(item or "").strip()
        if text:
            ret.append(text)
    return ret

def step_kind_index(steps):
    ret = {}
    for step in steps:
        if not isinstance(step, dict):
            continue
        step_id = str(field(step, "id", "ID") or "").strip()
        kind = str(field(step, "kind", "Kind") or "").strip()
        if step_id and kind:
            ret[step_id] = kind
    return ret

def evidence_ref_step_kind(ref, steps_by_id):
    ref = str(ref or "").strip()
    if not ref:
        return "", False
    kind = steps_by_id.get(ref)
    if kind:
        return kind, True
    for marker in ("/validation/", "/tool-call/"):
        if marker in ref:
            prefix = ref.split(marker, 1)[0].strip()
            kind = steps_by_id.get(prefix)
            if kind:
                return kind, True
    return "", False

def validate_required_coverage(errors, assessment, profile, profile_key, assessment_key, evidence_kind):
    expected = required_ids(profile, profile_key)
    if not expected:
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
        state = state_value(item)
        if state != "satisfied":
            errors.append(f"accepted software-dev {assessment_key}.{item_id} is not satisfied")
        refs = value_list(item, evidence_kind)
        if not refs and evidence_kind == "artifactRefs":
            refs = value_list(item, "evidenceRefs")
        if not refs:
            label = evidence_kind
            if evidence_kind == "artifactRefs":
                label = "artifactRefs or evidenceRefs"
            errors.append(f"accepted software-dev {assessment_key}.{item_id} missing {label}")

def required_direct_evidence_ids(profile):
    ret = []
    for item in value_list(profile, "evidenceRequirements"):
        if not isinstance(item, dict):
            continue
        item_id = str(field(item, "id", "ID") or "").strip()
        if item_id and field(item, "requireDirectEvidence", "RequireDirectEvidence") is True:
            ret.append(item_id)
    return ret

def direct_evidence_requirements(profile):
    ret = {}
    for item in value_list(profile, "evidenceRequirements"):
        if not isinstance(item, dict):
            continue
        item_id = str(field(item, "id", "ID") or "").strip()
        if not item_id or field(item, "requireDirectEvidence", "RequireDirectEvidence") is not True:
            continue
        ret[item_id] = {
            "acceptedStepKinds": set(string_list(field(item, "acceptedStepKinds", "AcceptedStepKinds"))),
            "rejectedStepKinds": set(string_list(field(item, "rejectedStepKinds", "RejectedStepKinds"))),
        }
    return ret

def validate_direct_dimension_evidence(errors, item_id, item, requirement, steps_by_id):
    if not isinstance(item, dict):
        return
    if state_value(item) != "satisfied":
        return
    refs = string_list(field(item, "evidenceRefs", "EvidenceRefs"))
    if not refs:
        errors.append(f"accepted software-dev acceptanceDimensions.{item_id} missing direct evidenceRefs")
        return
    accepted_kinds = requirement.get("acceptedStepKinds") or set()
    rejected_kinds = requirement.get("rejectedStepKinds") or set()
    known = 0
    has_accepted = False
    rejected_only = []
    for ref in refs:
        kind, ok = evidence_ref_step_kind(ref, steps_by_id)
        if not ok:
            continue
        known += 1
        if kind in accepted_kinds:
            has_accepted = True
        if kind in rejected_kinds:
            rejected_only.append(f"{ref}({kind})")
    if known == 0:
        errors.append(f"accepted software-dev acceptanceDimensions.{item_id} direct evidenceRefs are not traceable to runtime steps")
        return
    if accepted_kinds and has_accepted:
        return
    if rejected_only:
        errors.append(
            "accepted software-dev acceptanceDimensions."
            + item_id
            + " direct evidenceRefs use read-only or rejected step kinds: "
            + ", ".join(rejected_only)
        )
        return
    if accepted_kinds:
        errors.append(
            "accepted software-dev acceptanceDimensions."
            + item_id
            + " direct evidenceRefs do not match accepted step kinds: "
            + ", ".join(sorted(accepted_kinds))
        )

fixture = load_snapshot_fixture(snapshot_path, results_page_path)
if fixture:
    workspace, run, steps, results = fixture
    workspace_id = str(field(workspace, "id", "ID") or workspace_id)
    run_id = str(field(run, "id", "ID") or run_id)
else:
    workspaces = load_items(workspace_path)
    runs = load_items(run_path)
    steps = load_items(step_path)
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
steps_by_id = step_kind_index(steps)
direct_requirements = direct_evidence_requirements(profile) if profile else {}

final_results = [
    item for item in results
    if field(item, "runId", "RunID") == run_id
    and field(item, "state", "State") == "accepted"
    and str(field(item, "sourceType", "SourceType") or "").strip() != "acceptance_slice"
]
if str(field(run, "state", "State") or "") == "completed" and not final_results:
    errors.append("completed software-dev run has no final dod_assessment result")

for result in final_results:
    result_id = str(field(result, "id", "ID") or "<unknown>")
    assessment = assessment_from_result(result)
    if not assessment:
        errors.append(f"accepted software-dev result missing dodAssessment: {result_id}")
        continue
    goal_coverage, goal_items = coverage_items(assessment, "goalCoverage")
    if not isinstance(goal_coverage, dict):
        errors.append(f"accepted software-dev result missing dodAssessment.goalCoverage: {result_id}")
    elif not bool_field(goal_coverage, "satisfied", "Satisfied"):
        errors.append(f"accepted software-dev result goalCoverage is not satisfied: {result_id}")
    elif not goal_items:
        errors.append(f"accepted software-dev result goalCoverage has no items: {result_id}")
    else:
        for item in goal_items:
            if not isinstance(item, dict):
                continue
            if state_value(item) == "satisfied" and not value_list(item, "evidenceRefs"):
                errors.append(f"accepted software-dev result goalCoverage item missing evidenceRefs: {result_id}")
    if profile:
        validate_required_coverage(errors, assessment, profile, "acceptanceDimensions", "acceptanceDimensions", "evidenceRefs")
        validate_required_coverage(errors, assessment, profile, "deliverableExpectations", "deliverableCoverage", "artifactRefs")
        _, dimension_items = coverage_items(assessment, "acceptanceDimensions")
        dimensions = item_by_id(dimension_items)
        for item_id in required_direct_evidence_ids(profile):
            item = dimensions.get(item_id)
            validate_direct_dimension_evidence(errors, item_id, item, direct_requirements.get(item_id, {}), steps_by_id)

if errors:
    for message in errors:
        print(f"error: {message}", file=sys.stderr)
    raise SystemExit(1)

print(f"workorch software-dev profile verification passed for workspace={workspace_id} run={run_id}")
PY
