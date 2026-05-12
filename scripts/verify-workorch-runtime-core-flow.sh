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
RUN_FILE="$RUN_DIR/run.json"
STEP_FILE="$RUN_DIR/step.json"
RESULT_FILE="$RUN_DIR/result.json"
TICKET_FILE="$RUN_DIR/ticket.json"

if [[ ! -f "$RUN_FILE" ]]; then
  echo "missing run file: $RUN_FILE" >&2
  exit 1
fi
if [[ ! -f "$STEP_FILE" ]]; then
  echo "missing step file: $STEP_FILE" >&2
  exit 1
fi

python3 - "$RUN_FILE" "$STEP_FILE" "$RESULT_FILE" "$TICKET_FILE" "$RUN_ID" <<'PY'
import json
import os
import re
import sys

run_path, step_path, result_path, ticket_path, run_id = sys.argv[1:]

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

def payload(item, name):
    value = field(item, name, name[:1].upper() + name[1:])
    return value if isinstance(value, dict) else {}

def has_key(payload, key):
    return isinstance(payload, dict) and key in payload and payload[key] not in (None, "", [], {})

def graph_has_valid_patch_apply(graph):
    if not isinstance(graph, dict):
        return False
    nodes = graph.get("nodes")
    if not isinstance(nodes, list):
        return False
    for node in nodes:
        if not isinstance(node, dict) or str(node.get("kind") or "").strip() != "patch/apply":
            continue
        node_input = node.get("input")
        if not isinstance(node_input, dict):
            continue
        patch_type = str(node_input.get("patchType") or "").strip()
        target_ref = str(node_input.get("targetRef") or "").strip()
        if not patch_type or not target_ref:
            continue
        if patch_type == "file_write" and str(node_input.get("content") or ""):
            return True
        if patch_type == "unified_diff" and str(node_input.get("diff") or ""):
            return True
    return False

def graph_has_ungated_valid_patch_apply(graph):
    if not isinstance(graph, dict):
        return False
    nodes = graph.get("nodes")
    if not isinstance(nodes, list):
        return False
    by_id = {}
    for node in nodes:
        if isinstance(node, dict):
            node_id = str(node.get("id") or "").strip()
            if node_id:
                by_id[node_id] = node

    def has_llm_ancestor(node, seen):
        depends_on = node.get("dependsOn")
        if not isinstance(depends_on, list):
            return False
        for dep in depends_on:
            dep_id = str(dep or "").strip()
            if not dep_id or dep_id in seen:
                continue
            seen.add(dep_id)
            parent = by_id.get(dep_id)
            if not isinstance(parent, dict):
                continue
            if str(parent.get("kind") or "").strip() == "llm/workspace_execution":
                return True
            if has_llm_ancestor(parent, seen):
                return True
        return False

    for node in nodes:
        if not isinstance(node, dict) or str(node.get("kind") or "").strip() != "patch/apply":
            continue
        node_input = node.get("input")
        if not isinstance(node_input, dict):
            continue
        patch_type = str(node_input.get("patchType") or "").strip()
        target_ref = str(node_input.get("targetRef") or "").strip()
        if not patch_type or not target_ref:
            continue
        if patch_type == "file_write" and not str(node_input.get("content") or ""):
            continue
        if patch_type == "unified_diff" and not str(node_input.get("diff") or ""):
            continue
        if not has_llm_ancestor(node, set()):
            return True
    return False

def graph_patch_apply_errors(graph):
    if not isinstance(graph, dict):
        return []
    nodes = graph.get("nodes")
    if not isinstance(nodes, list):
        return []
    ret = []
    for node in nodes:
        if not isinstance(node, dict) or str(node.get("kind") or "").strip() != "patch/apply":
            continue
        node_id = str(node.get("id") or "").strip() or "<unknown>"
        node_input = node.get("input")
        if not isinstance(node_input, dict):
            ret.append(f"patch/apply node {node_id} input is required")
            continue
        patch_type = str(node_input.get("patchType") or node_input.get("operation") or "file_write").strip()
        if patch_type in ("create", "write", "replace", "file"):
            patch_type = "file_write"
        target_ref = str(node_input.get("targetRef") or "").strip()
        if not target_ref:
            ret.append(f"patch/apply node {node_id} targetRef is required")
        if patch_type not in ("file_write", "unified_diff"):
            ret.append(f"patch/apply node {node_id} patchType is invalid")
        if patch_type == "file_write" and not str(node_input.get("content") or "").strip():
            ret.append(f"patch/apply node {node_id} content is required")
        if patch_type == "unified_diff" and not str(node_input.get("diff") or "").strip():
            ret.append(f"patch/apply node {node_id} diff is required")
    return ret

def value_list(item, *names):
    value = field(item, *names)
    return value if isinstance(value, list) else []

def result_covers_step(result, step_id):
    if field(result, "sourceRef", "SourceRef") == step_id:
        return True
    if step_id in value_list(result, "evidenceRefs", "EvidenceRefs"):
        return True
    assessment = payload(result, "doDAssessment")
    items = assessment.get("items") or assessment.get("Items")
    if isinstance(items, list):
        for item in items:
            if isinstance(item, dict) and step_id in value_list(item, "evidenceRefs", "EvidenceRefs"):
                return True
    return False

def ticket_covers_step(ticket, step_id):
    if not isinstance(ticket, dict):
        return False
    if field(ticket, "stepId", "StepID") == step_id:
        return True
    context = payload(ticket, "context")
    for key in ("failedStepId", "repairOfStepId", "repairOfRootStepId", "recoveryRootStepId"):
        if str(context.get(key) or "") == step_id:
            return True
    return False

def handled_violation(step_id, steps, results, tickets):
    for candidate in steps:
        candidate_input = payload(candidate, "input")
        if candidate_input.get("runtimeRepairFollowUp") is True and str(candidate_input.get("repairOfStepId") or "") == step_id:
            return True, "runtime repair"
        if candidate_input.get("assessmentOnlyContractCorrection") is True and str(candidate_input.get("assessmentOnlyContractViolationStepId") or "") == step_id:
            return True, "assessment correction"
    for result in results:
        if field(result, "state", "State") == "rejected" and result_covers_step(result, step_id):
            return True, "rejected result"
    for ticket in tickets:
        if ticket_covers_step(ticket, step_id):
            return True, "decision ticket"
    return False, ""

def missing_file_read_failure(step):
    if not isinstance(step, dict):
        return False
    kind = str(field(step, "kind", "Kind") or "").strip()
    output_payload = payload(step, "output")
    text = "\n".join([
        kind,
        str(output_payload.get("stage") or ""),
        str(output_payload.get("error") or ""),
        str(output_payload.get("reason") or ""),
        str(output_payload.get("failureSignature") or ""),
    ]).lower()
    if "file/read" not in text and "file_read" not in text:
        return False
    return (
        "file_read_invalid_path" in text
        or "no such file" in text
        or "cannot find the file" in text
        or "lstat " in text
    )

def has_patch_execution(steps):
    for step in steps:
        if str(field(step, "kind", "Kind") or "").strip() == "patch/apply":
            return True
        output_payload = payload(step, "output")
        if value_list(output_payload, "patches", "patchRequests"):
            return True
    return False

def is_recovery_step_input(input_payload):
    return (
        input_payload.get("providerRetryFollowUp") is True
        or input_payload.get("runtimeRepairFollowUp") is True
        or input_payload.get("assessmentOnlyContractCorrection") is True
        or str(input_payload.get("recoveryAction") or "").strip() != ""
    )

def text_contains_state_changing_repair_signal(input_payload, output_payload):
    text = "\n".join([
        str(input_payload.get("repairObjective") or ""),
        str(input_payload.get("repairOfFailureClass") or ""),
        str(input_payload.get("repairOfFailureSignature") or ""),
        str(input_payload.get("failedStepOutput") or ""),
        str(output_payload.get("error") or ""),
        str(output_payload.get("reason") or ""),
    ]).lower()
    return (
        "produce_state_changing_implementation" in text
        or "state-changing" in text
        or "patch/apply" in text
        or "implementation planning llm step returned read-only executiongraph" in text
        or "planning executiongraph patch/apply depends on llm/workspace_execution" in text
    )

runs = load_items(run_path)
steps = load_items(step_path)
results = load_items(result_path)
tickets = load_items(ticket_path)
run = next((item for item in runs if field(item, "id", "ID") == run_id), runs[0] if runs else None)
if not run:
    raise SystemExit(f"run {run_id} not found in {run_path}")

errors = []
warnings = []

for step in steps:
    step_id = str(field(step, "id", "ID") or "")
    if step_id and not step_id.startswith(run_id + "/"):
        errors.append(f"non-run-scoped step id: {step_id}")
    if len(re.findall(r"provider-retry-\d+", step_id)) > 1:
        errors.append(f"nested provider retry step id: {step_id}")
    input_payload = payload(step, "input")
    if has_key(input_payload, "graphNodeId"):
        if not is_recovery_step_input(input_payload) and "/graph-" not in step_id:
            errors.append(f"graph node step id is not graph-scoped: {step_id}")
        if not is_recovery_step_input(input_payload):
            for key in ("graphSeq", "graphNodeSeq"):
                if not has_key(input_payload, key):
                    errors.append(f"graph node step missing {key}: {step_id}")
    if input_payload.get("providerRetryFollowUp") is True:
        root = str(input_payload.get("providerRetryRootStepId") or "")
        if not root:
            errors.append(f"provider retry step missing providerRetryRootStepId: {step_id}")
        if len(re.findall(r"provider-retry-\d+", root)) > 0:
            errors.append(f"provider retry root points at retry step: {step_id} root={root}")
    output_payload = payload(step, "output")
    violation_messages = []
    if (
        input_payload.get("providerRetryFollowUp") is True
        and has_key(input_payload, "repairOfRootStepId")
        and text_contains_state_changing_repair_signal(input_payload, output_payload)
        and input_payload.get("stateChangingOutputRequired") is not True
    ):
        violation_messages.append("provider retry of state-changing repair lost stateChangingOutputRequired")
    for reason in graph_patch_apply_errors(output_payload.get("executionGraph")):
        violation_messages.append(f"invalid executionGraph patch/apply: {reason}")
    if input_payload.get("stateChangingOutputRequired") is True and has_key(output_payload, "executionGraph"):
        if not graph_has_valid_patch_apply(output_payload.get("executionGraph")):
            violation_messages.append("state-changing step returned non-patch executionGraph")
        elif not graph_has_ungated_valid_patch_apply(output_payload.get("executionGraph")):
            violation_messages.append("state-changing executionGraph patch/apply depends on llm/workspace_execution before patch")
    if (
        str(input_payload.get("soulStage") or "").strip() == "planning"
        and graph_has_valid_patch_apply(output_payload.get("executionGraph"))
        and not graph_has_ungated_valid_patch_apply(output_payload.get("executionGraph"))
    ):
        violation_messages.append("planning executionGraph patch/apply depends on llm/workspace_execution before patch")
    for message in violation_messages:
        handled, by = handled_violation(step_id, steps, results, tickets)
        if handled:
            warnings.append(f"handled violation by {by}: {message}: {step_id}")
        else:
            errors.append(f"unhandled violation: {message}: {step_id}")
    if input_payload.get("runtimeRepairFollowUp") is True and input_payload.get("recoveryAction") != "runtime_repair":
        errors.append(f"runtime repair step missing recoveryAction=runtime_repair: {step_id}")
    if input_payload.get("providerRetryFollowUp") is True and input_payload.get("recoveryAction") != "provider_retry":
        errors.append(f"provider retry step missing recoveryAction=provider_retry: {step_id}")
    if (input_payload.get("runtimeRepairFollowUp") is True or input_payload.get("providerRetryFollowUp") is True):
        for key in ("recoveryRootStepId", "recoveryLineageType", "recoveryBudgetScope"):
            if not has_key(input_payload, key):
                errors.append(f"recovery step missing {key}: {step_id}")

run_state = str(field(run, "state", "State") or "")
run_dod = str(field(run, "doDId", "DoDID", "dodId") or "")
accepted_results = [item for item in results if field(item, "runId", "RunID") == run_id and field(item, "state", "State") == "accepted"]
rejected_results = [item for item in results if field(item, "runId", "RunID") == run_id and field(item, "state", "State") == "rejected"]
missing_file_failures = [step for step in steps if missing_file_read_failure(step)]
rejected_policy_tickets = []
for ticket_item in tickets:
    context = payload(ticket_item, "context")
    if str(context.get("resultPolicy") or "") == "rejected":
        rejected_policy_tickets.append(ticket_item)
if run_state == "completed" and not accepted_results:
    errors.append("completed run has no accepted result")
if run_state == "failed" and run_dod and not rejected_results:
    errors.append("failed DoD run has no rejected result")
if run_state in ("blocked_on_decision", "superseded", "failed") and rejected_policy_tickets and not rejected_results:
    ticket_ids = ", ".join(str(field(item, "id", "ID") or "") for item in rejected_policy_tickets[:3])
    errors.append(f"run has rejected resultPolicy decision without rejected result: {ticket_ids}")
if (
    run_state == "running"
    and run_dod
    and len(missing_file_failures) >= 2
    and not results
    and not tickets
    and not has_patch_execution(steps)
):
    sample_ids = ", ".join(str(field(step, "id", "ID") or "") for step in missing_file_failures[:3])
    errors.append(
        "running DoD run repeats missing file reads without patch/result/ticket: "
        + sample_ids
    )
if run_state == "blocked_on_decision":
    blocking_ticket_id = str(field(run, "blockingTicketId", "BlockingTicketID") or "")
    ticket = next((item for item in tickets if field(item, "id", "ID") == blocking_ticket_id), None)
    context = payload(ticket, "context") if ticket else {}
    result_policy = str(context.get("resultPolicy") or "")
    if result_policy == "rejected" and not rejected_results:
        errors.append(f"blocked decision {blocking_ticket_id} requires rejected result but none exists")
    if result_policy == "none" and rejected_results:
        errors.append(f"blocked provider/config decision {blocking_ticket_id} unexpectedly has rejected result")

if errors:
    raise SystemExit("\n".join(errors))

print("workorch runtime core flow verification passed")
print(f"runId={run_id}")
print(f"runState={run_state}")
print(f"stepCount={len(steps)}")
print(f"acceptedResultCount={len(accepted_results)}")
print(f"rejectedResultCount={len(rejected_results)}")
for warning in warnings:
    print(f"warning: {warning}")
PY
