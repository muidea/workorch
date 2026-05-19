#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKSPACE_ID="${1:-${WORKORCH_LCI_VERIFY_WORKSPACE_ID:-}}"
RUN_ID="${2:-${WORKORCH_LCI_VERIFY_RUN_ID:-}}"
DATA_ROOT="${WORKORCH_LCI_VERIFY_DATA_ROOT:-$ROOT_DIR/.workorch-data/internal/workspaces}"
REQUIRE_REPAIR="${WORKORCH_LCI_REQUIRE_REPAIR:-0}"

if [[ -z "$WORKSPACE_ID" ]]; then
  echo "usage: $0 <workspace-id> [run-id]" >&2
  echo "or set WORKORCH_LCI_VERIFY_WORKSPACE_ID and optional WORKORCH_LCI_VERIFY_RUN_ID" >&2
  exit 2
fi

WORKSPACE_DIR="$DATA_ROOT/$WORKSPACE_ID"
if [[ ! -d "$WORKSPACE_DIR" && -d "$ROOT_DIR/.workorch-data/internal/workspaces/$WORKSPACE_ID" ]]; then
  WORKSPACE_DIR="$ROOT_DIR/.workorch-data/internal/workspaces/$WORKSPACE_ID"
fi
if [[ ! -d "$WORKSPACE_DIR" ]]; then
  echo "missing workspace internal directory: $WORKSPACE_DIR" >&2
  exit 1
fi

python3 - "$WORKSPACE_DIR" "$WORKSPACE_ID" "$RUN_ID" "$REQUIRE_REPAIR" <<'PY'
import json
import sys
from pathlib import Path

workspace_dir = Path(sys.argv[1])
workspace_id = sys.argv[2]
run_id = sys.argv[3]
require_repair = sys.argv[4] == "1"

run_root = workspace_dir / "runs"
if not run_root.exists():
    raise SystemExit(f"missing runs directory: {run_root}")

if run_id:
    run_dirs = [run_root / run_id]
else:
    run_dirs = sorted([item for item in run_root.iterdir() if item.is_dir()])

request_paths = []
for run_dir in run_dirs:
    llm_root = run_dir / "llm-calls"
    if llm_root.exists():
        request_paths.extend(sorted(llm_root.glob("**/request.json")))

if not request_paths:
    raise SystemExit(f"no llm request.json found for workspace={workspace_id} run={run_id or '*'}")

errors = []
warnings = []
repair_requests = 0
truncated_requests = 0
protected_audits = 0

def walk(value, path=""):
    if isinstance(value, dict):
        for key, child in value.items():
            child_path = f"{path}.{key}" if path else str(key)
            yield child_path, child
            yield from walk(child, child_path)
    elif isinstance(value, list):
        for idx, child in enumerate(value):
            child_path = f"{path}[{idx}]"
            yield child_path, child
            yield from walk(child, child_path)

def as_dict(value):
    return value if isinstance(value, dict) else {}

def as_list(value):
    return value if isinstance(value, list) else []

for request_path in request_paths:
    rel = request_path.relative_to(workspace_dir)
    try:
        request = json.loads(request_path.read_text(encoding="utf-8"))
    except Exception as exc:
        errors.append(f"{rel}: invalid json: {exc}")
        continue

    input_payload = as_dict(request.get("input"))
    budget = as_dict(request.get("contextBudget"))
    if budget:
        protected = as_list(budget.get("protectedFields"))
        warnings_list = as_list(budget.get("integrityWarnings"))
        if protected:
            protected_audits += 1
        if budget.get("truncated") is True:
            truncated_requests += 1
            for required in ("protectedFields", "integrityWarnings", "sourceRefs", "rules"):
                if required not in budget:
                    errors.append(f"{rel}: truncated contextBudget missing {required}")
        if budget.get("goalDoDCompressed") is True:
            errors.append(f"{rel}: contextBudget reports compressed Goal/DoD")
        if budget.get("overBudget") is True and budget.get("p0Overflow") is not True:
            warnings.append(f"{rel}: request is over budget after compaction and should fail fast before provider call")
        for item in warnings_list:
            if str(item).strip() in {
                "coverage_repair_required_without_coverage_target_set",
                "must_declare_coverage_targets_without_target_set",
                "protected_goal_dod_changed_after_context_budget",
            }:
                errors.append(f"{rel}: integrity warning remains unresolved: {item}")

    pkg = as_dict(input_payload.get("repairActionPackage"))
    coverage_required = bool(input_payload.get("coverageRepairRequired")) or bool(pkg.get("coverageRepairRequired"))
    if coverage_required:
        repair_requests += 1
        targets = as_list(input_payload.get("coverageTargetSet")) or as_list(pkg.get("coverageTargetSet"))
        if not targets:
            errors.append(f"{rel}: coverage repair required but coverageTargetSet is missing")
        if not as_dict(pkg.get("currentExecutionProgress")):
            errors.append(f"{rel}: coverage repair missing currentExecutionProgress")
        required_output = as_dict(pkg.get("requiredOutput"))
        if required_output.get("mustDeclareCoverageTargets") is True and not targets:
            errors.append(f"{rel}: mustDeclareCoverageTargets without target tokens")

    for path, value in walk(input_payload):
        key = path.rsplit(".", 1)[-1]
        if key in {"rawContentPreview", "repairRawContentPreview", "streamEvents"}:
            errors.append(f"{rel}: rejected or stream raw field leaked into request input at {path}")
        if isinstance(value, str) and ("response.sse" in value or "[REDACTED]" in value):
            errors.append(f"{rel}: non-reusable raw/redacted marker leaked into request input at {path}")

print(f"workspace: {workspace_id}")
print(f"runs checked: {len(run_dirs)}")
print(f"request.json checked: {len(request_paths)}")
print(f"coverage repair requests: {repair_requests}")
print(f"truncated requests: {truncated_requests}")
print(f"context integrity audits: {protected_audits}")
for warning in warnings:
    print(f"warning: {warning}", file=sys.stderr)

if require_repair and repair_requests == 0:
    errors.append("no coverage repair request found while WORKORCH_LCI_REQUIRE_REPAIR=1")

if errors:
    print("context integrity verification failed:", file=sys.stderr)
    for item in errors:
        print(f"- {item}", file=sys.stderr)
    raise SystemExit(1)

print("workorch llm context integrity verification passed")
PY
