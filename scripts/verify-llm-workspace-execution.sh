#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_LLM_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-}}"
source "$ROOT_DIR/scripts/lib/verify-isolated-runtime.sh"
workorch_verify_require_isolated_runtime "$BASE_URL" "verify-llm-workspace-execution"
TMP_DIR="${WORKORCH_LLM_VERIFY_ARTIFACT_DIR:-}"
if [[ -n "$TMP_DIR" ]]; then
  TMP_DIR="$(realpath -m "$TMP_DIR")"
  mkdir -p "$TMP_DIR"
fi
if [[ -z "$TMP_DIR" ]]; then
  TMP_DIR="$(mktemp -d)"
fi
KEEP_TMP="${WORKORCH_VERIFY_KEEP_TMP:-0}"
MOCK_ADDR="${WORKORCH_LLM_VERIFY_MOCK_ADDR:-127.0.0.1:18181}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_LLM_VERIFY_SUFFIX:-$$}"
CURL_CONNECT_TIMEOUT="${WORKORCH_VERIFY_CURL_CONNECT_TIMEOUT_SECONDS:-5}"
CURL_MAX_TIME="${WORKORCH_VERIFY_CURL_MAX_TIME_SECONDS:-30}"
SCRIPT_TIMEOUT_SECONDS="${WORKORCH_LLM_VERIFY_TIMEOUT_SECONDS:-900}"
RUN_WAIT_SECONDS="${WORKORCH_LLM_VERIFY_RUN_WAIT_SECONDS:-45}"
ACTIVATION_RUN_WAIT_SECONDS="${WORKORCH_LLM_VERIFY_ACTIVATION_RUN_WAIT_SECONDS:-${RUN_WAIT_SECONDS}}"
# A newly created workspace has no active Run.  The owner contract therefore
# requires an explicit restart (resume is only a continuation of an existing
# non-terminal Run).
ACTIVATION_INTENT="${WORKORCH_LLM_VERIFY_ACTIVATION_INTENT:-restart}"
MCP_SERVER_ID="mcp-context-verify-$VERIFY_SUFFIX"
SKILL_ID="skill-llm-acceptance-$VERIFY_SUFFIX"
SOUL_ID="soul-llm-acceptance-$VERIFY_SUFFIX"
MEMORY_ID="memory-context-seed-$VERIFY_SUFFIX"
C12_TEMPLATE_ID="runtime-c12-live-$VERIFY_SUFFIX"
C12_VALIDATION_RECIPE_ID="runtime-c12-source-present"
MOCK_PID=""
WATCHDOG_PID=""
MOCK_LOG="$TMP_DIR/llm-mock.log"

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

timeout_failure() {
  echo "workorch LLM workspace execution verification timed out after ${SCRIPT_TIMEOUT_SECONDS}s for deployed workorch service $BASE_URL" >&2
  if [[ -f "$MOCK_LOG" ]]; then
    echo "--- llm-mock.log ---" >&2
    tail -n 200 "$MOCK_LOG" >&2 || true
  fi
  exit 124
}

start_watchdog() {
  if ! [[ "$SCRIPT_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || [[ "$SCRIPT_TIMEOUT_SECONDS" -le 0 ]]; then
    return
  fi
  local parent_pid="$$"
  (
    sleep "$SCRIPT_TIMEOUT_SECONDS"
    echo "workorch LLM workspace execution verification exceeded ${SCRIPT_TIMEOUT_SECONDS}s; terminating" >&2
    kill -TERM "$parent_pid" >/dev/null 2>&1 || true
  ) &
  WATCHDOG_PID=$!
}

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "workorch LLM workspace execution verification failed for deployed workorch service $BASE_URL" >&2
  if [[ -f "$MOCK_LOG" ]]; then
    echo "--- llm-mock.log ---" >&2
    tail -n 200 "$MOCK_LOG" >&2 || true
  fi
  for file in "$TMP_DIR"/*.json; do
    [[ -f "$file" ]] || continue
    echo "--- $file ---" >&2
    python3 -m json.tool "$file" >&2 || cat "$file" >&2
  done
}

cleanup() {
  if [[ -n "$WATCHDOG_PID" ]]; then
    kill "$WATCHDOG_PID" >/dev/null 2>&1 || true
    wait "$WATCHDOG_PID" >/dev/null 2>&1 || true
  fi
  if [[ -f "$TMP_DIR/original-llm-update.json" ]]; then
    curl -fsS -H "Content-Type: application/json" -X PUT \
      "$BASE_URL/api/v1/workorch/configuration/llm" \
      -d @"$TMP_DIR/original-llm-update.json" >/dev/null 2>&1 || true
  fi
  if [[ -n "$MOCK_PID" ]]; then
    kill "$MOCK_PID" >/dev/null 2>&1 || true
    wait "$MOCK_PID" >/dev/null 2>&1 || true
  fi
  if [[ -n "${WORKORCH_LLM_VERIFY_ARTIFACT_DIR:-}" || "$KEEP_TMP" == "1" || "$KEEP_TMP" == "true" ]]; then
    echo "keeping verification temp dir: $TMP_DIR" >&2
  else
    rm -rf "$TMP_DIR"
  fi
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

json_path_equals() {
  python3 - "$1" "$2" "$3" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)
current = data
for part in sys.argv[2].split("."):
    if not isinstance(current, dict) or part not in current:
        raise SystemExit(1)
    current = current[part]
raise SystemExit(0 if str(current) == sys.argv[3] else 1)
PY
}

url_encode() {
  python3 - "$1" <<'PY'
import sys
from urllib.parse import quote

print(quote(sys.argv[1], safe=""))
PY
}

json_grep_string_value() {
  local key="$1"
  local value="$2"
  local file="${3:-}"
  local pattern="\"$key\"[[:space:]]*:[[:space:]]*\"$value\""
  if [[ -n "$file" ]]; then
    grep -E "$pattern" "$file"
  else
    grep -E "$pattern"
  fi
}

json_grep_bool_value() {
  local key="$1"
  local value="$2"
  local file="${3:-}"
  local pattern="\"$key\"[[:space:]]*:[[:space:]]*$value"
  if [[ -n "$file" ]]; then
    grep -E "$pattern" "$file"
  else
    grep -E "$pattern"
  fi
}

resolve_decision_ticket_if_needed() {
  local ticket_path="$1"
  local payload="$2"
  local detail_file="$3"
  local resolve_file="$4"

  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_path?workspaceId=$workspace_id" >"$detail_file"
  if json_grep_string_value state resolved "$detail_file" >/dev/null; then
    return 0
  fi
  if curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$ticket_path/resolve?workspaceId=$workspace_id" -H 'Content-Type: application/json' \
    -d "$payload" >"$resolve_file"; then
    json_grep_string_value resolutionType provide_missing_info "$resolve_file" >/dev/null
    return 0
  fi

  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_path?workspaceId=$workspace_id" >"$detail_file"
  json_grep_string_value state resolved "$detail_file" >/dev/null
}

start_mock_provider() {
  cat >"$TMP_DIR/llm_mock_provider.py" <<'PY'
import json
import os
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

# Runtime mock: respond with pure candidate payloads keyed by the sealed
# canonical convergence state. RuntimeAction was deliberately retired from the
# production candidate contract; this fixture must not recreate it as a second
# authority just to select a canned response.
# Retired V1 candidate keys are rejected by DisallowUnknownFields and must
# never appear in provider output.

run_scenarios = {}
validation_done = set()
request_sequence = 0

def last(pattern, text):
    matches = re.findall(pattern, text or "")
    return matches[-1] if matches else ""

def prompt_sections(input_text):
    # The provider-facing prompt keeps every Runtime authority section
    # explicitly headed.  Parse those sections rather than scanning the whole
    # prompt for lookalike values from C1 (definition), C4 (history), or C5
    # (remaining focuses).  In particular, C5 can legitimately mention a
    # later/earlier cwi-* that is outside the sealed current focus.
    sections = {}
    content = str(input_text or "")
    for match in re.finditer(r'(?ms)^###\s+([^\n]+)\n(.*?)(?=^###\s+|\Z)', content):
        section_id = match.group(1).strip()
        section_content = match.group(2).strip()
        if section_id and section_content:
            sections[section_id] = section_content
    return sections

def leading_section_json(section_id, content, expected_type):
    # Responses flattens dynamic prompt turns as "[user]" followed by a
    # heading.  A section body can consequently include that transport marker
    # after its JSON value.  Decode only the declared section value, as the
    # production parser does for a structured document, rather than treating
    # the next turn marker as part of the value.
    try:
        value, _ = json.JSONDecoder().raw_decode((content or "").lstrip())
    except json.JSONDecodeError as exc:
        raise ValueError("invalid %s JSON: %s" % (section_id, exc)) from exc
    if not isinstance(value, expected_type):
        raise ValueError("%s must be a %s" % (section_id, "JSON object" if expected_type is dict else "JSON array"))
    return value

def object_value(value, *keys):
    if not isinstance(value, dict):
        return None
    for key in keys:
        if key in value:
            return value[key]
    return None

def string_list(value):
    if not isinstance(value, list):
        return []
    return [item for item in value if isinstance(item, str) and item]

def unique_strings(values):
    ret = []
    seen = set()
    for value in values:
        if not isinstance(value, str) or not value or value in seen:
            continue
        seen.add(value)
        ret.append(value)
    return ret

def current_focus_evidence_refs(context, execution_tail, coverage):
    # C3 has the sealed allow-list when assessment evidence is required.  C4
    # is only a fallback and is filtered by the current focus coverage; never
    # allow history from a completed/remaining focus to satisfy this one.
    allowed = string_list(object_value(context, "allowedAcceptedEvidenceRefs", "AllowedEvidenceRefs"))
    if allowed:
        return unique_strings(allowed)
    current_coverage = set(coverage)
    refs = []
    if isinstance(execution_tail, dict):
        for evidence in object_value(execution_tail, "latestAcceptedEvidence", "AcceptedEvidence") or []:
            evidence_coverage = set(string_list(object_value(evidence, "coverageWorkItemIds", "CoverageWorkItemIDs")))
            if current_coverage and not current_coverage.intersection(evidence_coverage):
                continue
            refs.extend(string_list(object_value(evidence, "evidenceRefs", "EvidenceRefs")))
    return unique_strings(refs)

def operation_contracts(authority):
    output_contract = object_value(authority, "outputContract", "OutputContract")
    return object_value(output_contract, "operationContracts", "OperationContracts") or []

def select_operation_contract(request, *kinds):
    contracts = request.get("operationContracts") or []
    for kind in kinds:
        for contract in contracts:
            if object_value(contract, "kind", "Kind") == kind:
                return contract
    if contracts:
        return contracts[0]
    raise ValueError("C3.runtime-authority has no sealed operation contract")

def operation_target_ref(contract):
    target = object_value(contract, "target", "Target") or {}
    for key in (
        "allowedTargetRefs", "AllowedTargetRefs",
        "exactTargetRefs", "ExactTargetRefs",
        "ownerScopeTargetRefs", "OwnerScopeTargetRefs",
    ):
        refs = string_list(object_value(target, key))
        if refs:
            return refs[0]
    raise ValueError("sealed operation contract has no concrete target")

def operation_binding_refs(contract):
    # Call only when bindingRefs remains a provider field. Singleton authority
    # is absent from bindingAuthorizations because SM-3 compiles it locally.
    # A CWI list is an assessment observation, never operation authority.
    refs = []
    authorizations = object_value(contract, "bindingAuthorizations", "BindingAuthorizations") or []
    for authorization in authorizations:
        refs.extend(string_list(object_value(authorization, "allowedBindingRefs", "AllowedBindingRefs")))
    refs = unique_strings(refs)
    if len(refs) != 1:
        raise ValueError("sealed operation contract must resolve to exactly one binding ref")
    return refs

def operation_fields(contract, key):
    return set(string_list(object_value(contract, key, key[0].upper() + key[1:])))

def operation_parameter_names(contract):
    names = []
    for parameter in object_value(contract, "parameters", "Parameters") or []:
        name = object_value(parameter, "name", "Name") or ""
        if name:
            names.append(name)
    return set(unique_strings(names))

def provider_operation(contract, kind, parameters):
    # operationContracts is already the RuntimeCandidateWireContract
    # projection. The mock must obey it instead of reconstructing fields from
    # target/binding authority that SM-3 has marked locally derived.
    allowed = operation_fields(contract, "allowedOperationFields")
    required = operation_fields(contract, "requiredOperationFields")
    if "kind" not in allowed:
        raise ValueError("fixture supports auxiliary operation wire rules only")
    operation = {"kind": kind}
    if "bindingRefs" in allowed:
        operation["bindingRefs"] = operation_binding_refs(contract)
    if "targetRefs" in allowed:
        operation["targetRefs"] = [operation_target_ref(contract)]
    if "parameters" in allowed:
        provider_names = operation_parameter_names(contract)
        selected = [parameter for parameter in parameters if parameter.get("name") in provider_names]
        if selected or "parameters" in required:
            operation["parameters"] = selected
    return operation

def operation_validation_recipe_id(contract):
    # The candidate may select only a Shell-owner-confirmed recipe identity.
    # C3 intentionally exposes IDs, never a command/args snapshot: a recipe
    # command is sealed owner authority and must not become a second provider
    # command-construction interface.
    recipe_ids = unique_strings(string_list(object_value(contract, "validationRecipeIds", "ValidationRecipeIDs")))
    if len(recipe_ids) != 1:
        raise ValueError("sealed managed validation contract must expose exactly one recipe id")
    return recipe_ids[0]

def focus_ordinal(focus_id):
    # Semantic focus IDs end in a proposal hash. Hash digits are identity,
    # never a fixture ordinal; interpreting them as a loop bound can turn one
    # provider request into unbounded CPU and output generation.
    match = re.fullmatch(r"focus-(\d+)", focus_id or "")
    return max(1, int(match.group(1))) if match else 1

def fixture_file_content(focus_id):
    base = "Runtime bounded terminal fixture input.\n"
    ordinal = focus_ordinal(focus_id)
    return base + "".join("verified by llm e2e focus-%03d\n" % item for item in range(1, ordinal + 1))

def provider_request_from_payload(payload):
    content = str(payload.get("input", ""))
    sections = prompt_sections(content)
    authority = None
    semantic_authority = None
    if "C3.semantic-authority" in sections:
        semantic_authority = leading_section_json("C3.semantic-authority", sections["C3.semantic-authority"], dict)
    if "C3.runtime-authority" in sections:
        authority = leading_section_json("C3.runtime-authority", sections["C3.runtime-authority"], dict)

    # Probe and historical non-Runtime-V2 checks have no sealed C3 authority.
    # Every Runtime candidate request does: fail closed if it is absent so a
    # test fixture can never accidentally infer authorization from surrounding
    # prompt text.
    if semantic_authority is not None:
        manifest = object_value(semantic_authority, "manifest", "Manifest")
        definition_seed = object_value(semantic_authority, "definitionSeed", "DefinitionSeed")
        workspace_id = object_value(semantic_authority, "workspaceId", "WorkspaceID") or ""
        run_id = object_value(semantic_authority, "runId", "RunID") or ""
        plan_step = object_value(semantic_authority, "planStep", "PlanStep") or ""
        semantic_schema = object_value(semantic_authority, "schema", "Schema")
        batch_id = object_value(semantic_authority, "batchId", "BatchID") or ""
        batch_sequence = object_value(semantic_authority, "batchSequence", "BatchSequence")
        batch_coverage = unique_strings(string_list(object_value(semantic_authority, "batchCoverageWorkItemIds", "BatchCoverageWorkItemIDs")))
        if not isinstance(manifest, dict) or not isinstance(definition_seed, dict) or not isinstance(semantic_schema, dict) or not workspace_id or not run_id:
            raise ValueError("C3.semantic-authority has incomplete scoped manifest")
        if plan_step not in ("coverage_order", "proof_batch"):
            raise ValueError("C3.semantic-authority has invalid plan step")
        if plan_step == "proof_batch" and (not batch_id or not isinstance(batch_sequence, int) or isinstance(batch_sequence, bool) or not batch_coverage):
            raise ValueError("C3.semantic-authority has incomplete proof batch identity")
        semantic_slices = []
        if "C5.acceptance-slices" in sections:
            semantic_slices = leading_section_json("C5.acceptance-slices", sections["C5.acceptance-slices"], list)
        definition_seed = dict(definition_seed)
        definition_seed["slices"] = semantic_slices
        return {
            "documentKind": "semantic_plan",
            "workspaceId": workspace_id,
            "runId": run_id,
            "runKey": workspace_id + "/" + run_id,
            "content": content,
            "semanticManifest": manifest,
            "definitionSeed": definition_seed,
            "semanticSchema": semantic_schema,
            "planStep": plan_step,
            "batchId": batch_id,
            "batchSequence": batch_sequence if isinstance(batch_sequence, int) and not isinstance(batch_sequence, bool) else 0,
            "batchCoverageWorkItemIds": batch_coverage,
        }
    if authority is not None:
        context = object_value(authority, "context", "Context")
        transaction = object_value(authority, "transaction", "Transaction")
        if not isinstance(context, dict):
            raise ValueError("C3.runtime-authority missing context")
        workspace_id = object_value(context, "workspaceId", "WorkspaceID") or ""
        run_id = object_value(context, "runId", "RunID") or ""
        step_id = object_value(transaction, "authoritySnapshotId", "AuthoritySnapshotID") or ""
        required_state = object_value(context, "requiredState", "RequiredState") or ""
        focus = object_value(context, "focus", "Focus") or {}
        focus_id = object_value(focus, "id", "ID") or ""
        coverage = unique_strings(string_list(object_value(focus, "coverageWorkItemIds", "CoverageWorkItemIDs")))
        if not workspace_id or not run_id or not required_state or not focus_id or not coverage:
            raise ValueError("C3.runtime-authority has incomplete scoped focus")
        execution_tail = None
        if "C4.execution-tail" in sections:
            execution_tail = leading_section_json("C4.execution-tail", sections["C4.execution-tail"], dict)
        evidence_refs = current_focus_evidence_refs(context, execution_tail, coverage)
        sealed_operation_contracts = operation_contracts(authority)
    else:
        # Compatibility for provider probes and the older acceptance scenarios
        # in this verifier.  This branch is intentionally unavailable once C3
        # is present, so it cannot blur sealed Runtime focus boundaries.
        workspace_id = last(r'"workspaceId"\s*:\s*"([^"]+)"', content) or last(r'workspaceId\s*[:=]\s*([A-Za-z0-9._/-]+)', content)
        run_id = last(r'"runId"\s*:\s*"([^"]+)"', content) or last(r'runId\s*[:=]\s*([A-Za-z0-9._/-]+)', content)
        step_id = last(r'"stepId"\s*:\s*"([^"]+)"', content) or last(r'stepId\s*[:=]\s*([A-Za-z0-9._/-]+)', content)
        required_state = last(r'"requiredState"\s*:\s*"([^"]+)"', content)
        focus_id = last(r'"Focus"\s*:\s*\{\s*"ID"\s*:\s*"([^"]+)"', content) or last(r'"focusId"\s*:\s*"([^"]+)"', content) or "focus-001"
        coverage = unique_strings(re.findall(r'"(cwi-[A-Za-z0-9._-]+)"', content)) or ["cwi-001"]
        evidence_refs = unique_strings(re.findall(r'"(workorch://[^"]+|run-[^"]+/validation/[^"]+|run-[^"]+/graph-[^"]+)"', content))
        sealed_operation_contracts = []
    if not evidence_refs:
        evidence_refs = ["workorch://workspace/runtime-validation-source.txt"]
    return {
        "documentKind": "operations",
        "workspaceId": workspace_id,
        "runId": run_id,
        "runKey": (workspace_id + "/" + run_id) if workspace_id and run_id else run_id,
        "stepId": step_id,
        "content": content,
        "requiredState": required_state,
        "focusId": focus_id,
        "coverage": coverage[:4],
        "evidenceRefs": evidence_refs[:4],
        "operationContracts": sealed_operation_contracts,
    }

def semantic_catalogs_by_owner(manifest):
    catalogs = {}
    for catalog in object_value(manifest, "ownerCatalogs", "OwnerCatalogs") or []:
        owner = object_value(catalog, "ownerModuleId", "OwnerModuleID") or ""
        if owner:
            catalogs[owner] = catalog
    return catalogs

def semantic_required_evidence(seed, coverage_id):
    values = []
    for slice_value in object_value(seed, "slices", "Slices") or []:
        coverage = string_list(object_value(slice_value, "coverageWorkItemIds", "CoverageWorkItemIDs"))
        if coverage_id in coverage:
            values.extend(string_list(object_value(slice_value, "requiredEvidenceKinds", "RequiredEvidenceKinds")))
    values = unique_strings(values)
    return values or ["validation"]

def semantic_exact_target(catalog):
    targets = string_list(object_value(catalog, "targetRefs", "TargetRefs"))
    if targets:
        return {"policy": "exact_target", "exactTargetRefs": [targets[0]]}
    selectors = string_list(object_value(catalog, "selectorRefs", "SelectorRefs"))
    if selectors:
        return {"policy": "existing_owner_scope", "ownerSelectorRefs": [selectors[0]]}
    raise ValueError("semantic owner catalog has no bounded target or selector")

def semantic_validation_proposal(coverage_id, catalogs):
    for owner, catalog in catalogs.items():
        kinds = string_list(object_value(catalog, "operationKinds", "OperationKinds"))
        recipe_refs = unique_strings(string_list(
            object_value(catalog, "validationRecipeRefs", "ValidationRecipeRefs")
        ))
        if "validation/managed_session" not in kinds or not recipe_refs:
            continue
        return {
            "coverageWorkItemId": coverage_id,
            "ownerModuleId": owner,
            "operationKind": "validation/managed_session",
            "target": semantic_exact_target(catalog),
            "requiredEvidenceKinds": ["validation"],
            "validationRecipeId": recipe_refs[0],
            "declarationRef": "mock-semantic/" + coverage_id,
        }
    raise ValueError("semantic manifest has no bounded managed-validation catalog")

def semantic_state_change_proposal(coverage_id, catalogs):
    preferred = ["file/edit", "patch/apply", "file/write", "git/write"]
    for kind in preferred:
        for owner, catalog in catalogs.items():
            if kind not in string_list(object_value(catalog, "operationKinds", "OperationKinds")):
                continue
            return {
                "coverageWorkItemId": coverage_id,
                "ownerModuleId": owner,
                "operationKind": kind,
                "target": semantic_exact_target(catalog),
                "requiredEvidenceKinds": ["state_change"],
                "declarationRef": "mock-semantic/" + coverage_id,
            }
    raise ValueError("semantic manifest has no bounded state-change catalog")

def semantic_coverage_order(request):
    schema = request.get("semanticSchema") or {}
    properties = object_value(schema, "properties", "Properties") or {}
    coverage_schema = object_value(properties, "coverageOrder", "CoverageOrder") or {}
    items = object_value(coverage_schema, "items", "Items") or {}
    coverage = unique_strings(string_list(object_value(items, "enum", "Enum")))
    if coverage:
        return coverage
    manifest = request["semanticManifest"]
    return unique_strings([
        object_value(item, "id", "ID") or ""
        for item in object_value(manifest, "coverageItems", "CoverageItems") or []
    ])

def semantic_plan_payload(request):
    manifest = request["semanticManifest"]
    seed = request["definitionSeed"]
    schema = request["semanticSchema"]
    plan_step = request.get("planStep") or ""
    coverage_order = semantic_coverage_order(request)
    if not coverage_order:
        raise ValueError("semantic request has no bounded coverage order")
    schema_version = object_value(
        object_value(schema, "properties", "Properties") or {},
        "schemaVersion", "SchemaVersion",
    ) or {}
    document_schema_version = object_value(schema_version, "const", "Const")
    if not isinstance(document_schema_version, str) or not document_schema_version:
        raise ValueError("semantic request has no sealed document schema version")
    document = {
        "documentKind": "semantic_plan",
        "schemaVersion": document_schema_version,
        "manifestHash": object_value(manifest, "hash", "Hash"),
        "definitionSeedHash": object_value(manifest, "definitionSeedHash", "DefinitionSeedHash"),
        "planStep": plan_step,
        "proposals": [],
    }
    if plan_step == "coverage_order":
        document["coverageOrder"] = coverage_order
        return document
    if plan_step != "proof_batch":
        raise ValueError("mock semantic planner received invalid plan step %r" % plan_step)
    batch_coverage = request.get("batchCoverageWorkItemIds") or []
    if not batch_coverage or any(coverage_id not in coverage_order for coverage_id in batch_coverage):
        raise ValueError("semantic proof batch is outside bounded coverage order")
    catalogs = semantic_catalogs_by_owner(manifest)
    proposals = []
    manifest_coverage = unique_strings([
        object_value(item, "id", "ID") or ""
        for item in object_value(manifest, "coverageItems", "CoverageItems") or []
    ])
    if set(manifest_coverage) != set(batch_coverage):
        raise ValueError("semantic proof manifest does not match bounded batch coverage")
    for coverage_id in batch_coverage:
        for evidence_kind in semantic_required_evidence(seed, coverage_id):
            if evidence_kind == "validation":
                proposals.append(semantic_validation_proposal(coverage_id, catalogs))
            elif evidence_kind == "state_change":
                proposals.append(semantic_state_change_proposal(coverage_id, catalogs))
            else:
                raise ValueError("mock semantic planner has no bounded owner operation for evidence %r" % evidence_kind)
    document["batchId"] = request["batchId"]
    document["batchSequence"] = request["batchSequence"]
    document["batchCoverageWorkItemIds"] = list(batch_coverage)
    document["proposals"] = proposals
    return document

def provider_request_parser_self_test():
    payload = {"input": """### C3.runtime-authority
{"transaction":{"authoritySnapshotId":"authority-current"},"outputContract":{"operationContracts":[{"kind":"file/edit","bindingAuthorizations":[{"allowedBindingRefs":["binding/cwi-current"]}],"target":{"allowedTargetRefs":["runtime-validation-source.txt"]}}]},"context":{"workspaceId":"ws-current","runId":"run-current","requiredState":"need_assessment","focus":{"id":"focus-current","coverageWorkItemIds":["cwi-current"]}}}

### C4.execution-tail
{"latestAcceptedEvidence":[{"coverageWorkItemIds":["cwi-old"],"evidenceRefs":["workorch://old"]},{"coverageWorkItemIds":["cwi-current"],"evidenceRefs":["workorch://current"]}]}

### C5.large-refs
{"remainingFocuses":[{"id":"focus-old"}],"coverageWorkItemIds":["cwi-old"]}"""}
    got = provider_request_from_payload(payload)
    if got["workspaceId"] != "ws-current" or got["runId"] != "run-current" or got["stepId"] != "authority-current":
        raise AssertionError("C3 scope parser did not preserve current authority")
    if got["coverage"] != ["cwi-current"] or got["evidenceRefs"] != ["workorch://current"]:
        raise AssertionError("C3/C4 parser leaked historical focus coverage or evidence: %r" % got)
    contract = select_operation_contract(got, "file/edit")
    if contract.get("kind") != "file/edit":
        raise AssertionError("C3 parser lost sealed operation contracts")
    if operation_binding_refs(contract) != ["binding/cwi-current"]:
        raise AssertionError("C3 parser lost sealed operation binding refs")
    if operation_validation_recipe_id({"validationRecipeIds": ["recipe-current"]}) != "recipe-current":
        raise AssertionError("C3 parser lost sealed managed validation recipe identity")
    minimal = provider_operation({
        "allowedOperationFields": ["kind", "parameters"],
        "requiredOperationFields": ["kind", "parameters"],
        "bindingAuthorizations": [{"allowedBindingRefs": ["binding/local"]}],
        "target": {"allowedTargetRefs": ["fixture.txt"]},
        "parameters": [{"name": "recipeId"}],
    }, "validation/managed_session", [{"name": "recipeId", "string": "recipe-current"}])
    if set(minimal) != {"kind", "parameters"} or minimal["parameters"] != [{"name": "recipeId", "string": "recipe-current"}]:
        raise AssertionError("provider fixture re-exposed locally derived binding/target authority: %r" % minimal)
    semantic_order = provider_request_from_payload({"input": """### C3.semantic-authority
{"workspaceId":"ws-semantic","runId":"run-semantic","definitionSeed":{"hash":"seed-semantic"},"manifest":{"hash":"manifest-semantic","definitionSeedHash":"seed-semantic","coverageItems":[{"id":"cwi-first"},{"id":"cwi-semantic"}]},"schema":{"properties":{"schemaVersion":{"const":"workorch.runtime_semantic_plan_document.v2"},"coverageOrder":{"items":{"enum":["cwi-first","cwi-semantic"]}}}},"planStep":"coverage_order"}
"""})
    order_document = semantic_plan_payload(semantic_order)
    if order_document.get("planStep") != "coverage_order" or order_document.get("coverageOrder") != ["cwi-first", "cwi-semantic"] or order_document.get("proposals") != []:
        raise AssertionError("semantic order parser did not produce a bounded order: %r" % order_document)
    semantic_batch = provider_request_from_payload({"input": """### C3.semantic-authority
{"workspaceId":"ws-semantic","runId":"run-semantic","definitionSeed":{"hash":"seed-semantic"},"manifest":{"hash":"manifest-semantic","definitionSeedHash":"seed-semantic","coverageItems":[{"id":"cwi-semantic"}],"ownerCatalogs":[{"ownerModuleId":"blocks/shell","operationKinds":["validation/managed_session"],"targetRefs":["fixture.txt"],"validationRecipeRefs":["recipe-semantic"]}]},"schema":{"properties":{"schemaVersion":{"const":"workorch.runtime_semantic_plan_document.v2"}}},"planStep":"proof_batch","batchId":"batch-semantic","batchSequence":1,"batchCoverageWorkItemIds":["cwi-semantic"]}

### C5.acceptance-slices
[{"coverageWorkItemIds":["cwi-semantic"],"requiredEvidenceKinds":["validation"]}]"""})
    semantic_document = semantic_plan_payload(semantic_batch)
    proposals = semantic_document.get("proposals") or []
    if "coverageOrder" in semantic_document or semantic_document.get("planStep") != "proof_batch" or semantic_document.get("batchCoverageWorkItemIds") != ["cwi-semantic"] or len(proposals) != 1 or proposals[0].get("coverageWorkItemId") != "cwi-semantic" or proposals[0].get("validationRecipeId") != "recipe-semantic":
        raise AssertionError("semantic authority parser did not produce a bounded proof batch: %r" % semantic_document)
    independent_proofs = provider_request_from_payload({"input": """### C3.semantic-authority
{"workspaceId":"ws-semantic","runId":"run-semantic","definitionSeed":{"hash":"seed-semantic"},"manifest":{"hash":"manifest-semantic","definitionSeedHash":"seed-semantic","coverageItems":[{"id":"cwi-semantic"}],"ownerCatalogs":[{"ownerModuleId":"blocks/file","operationKinds":["file/edit"],"targetRefs":["fixture.txt"]},{"ownerModuleId":"blocks/shell","operationKinds":["validation/managed_session"],"targetRefs":["fixture.txt"],"validationRecipeRefs":["recipe-semantic"]}]},"schema":{"properties":{"schemaVersion":{"const":"workorch.runtime_semantic_plan_document.v2"}}},"planStep":"proof_batch","batchId":"batch-semantic","batchSequence":1,"batchCoverageWorkItemIds":["cwi-semantic"]}

### C5.acceptance-slices
[{"coverageWorkItemIds":["cwi-semantic"],"requiredEvidenceKinds":["state_change","validation"]}]"""})
    independent_document = semantic_plan_payload(independent_proofs)
    independent_kinds = [
        (proposal.get("coverageWorkItemId"), tuple(proposal.get("requiredEvidenceKinds") or []))
        for proposal in independent_document.get("proposals") or []
    ]
    if independent_kinds != [("cwi-semantic", ("state_change",)), ("cwi-semantic", ("validation",))]:
        raise AssertionError("semantic authority parser did not emit one independent proof per evidence kind: %r" % independent_document)
    responses_wrapped = provider_request_from_payload({"input": """### C3.semantic-authority
{"workspaceId":"ws-semantic","runId":"run-semantic","definitionSeed":{"hash":"seed-semantic"},"manifest":{"hash":"manifest-semantic","definitionSeedHash":"seed-semantic","coverageItems":[{"id":"cwi-semantic"}],"ownerCatalogs":[{"ownerModuleId":"blocks/shell","operationKinds":["validation/managed_session"],"targetRefs":["fixture.txt"],"validationRecipeRefs":["recipe-semantic"]}]},"schema":{"properties":{"schemaVersion":{"const":"workorch.runtime_semantic_plan_document.v2"}}},"planStep":"proof_batch","batchId":"batch-semantic","batchSequence":1,"batchCoverageWorkItemIds":["cwi-semantic"]}

[user]
### C4.semantic-tail
{"attempt":0}

[user]
### C5.acceptance-slices
[{"coverageWorkItemIds":["cwi-semantic"],"requiredEvidenceKinds":["validation"]}]"""})
    if semantic_plan_payload(responses_wrapped).get("batchId") != "batch-semantic":
        raise AssertionError("Responses role wrapper obscured semantic authority")
    try:
        semantic_validation_proposal("cwi-legacy", {
            "blocks/shell": {
                "operationKinds": ["validation/managed_session"],
                "targetRefs": ["fixture.txt"],
                "validationRecipes": [{"id": "recipe-legacy"}],
            },
        })
    except ValueError:
        pass
    else:
        raise AssertionError("semantic planner accepted the retired executable validation recipe catalog")
    probe = provider_request_from_payload({"input": '{"runId":"probe","stepId":"probe"}'})
    probe_candidate = decision_payload(probe)
    probe_operation = (probe_candidate.get("operations") or [{}])[0]
    if probe_operation.get("kind") != "file/read" or "id" in probe_operation or "dependsOn" in probe_operation:
        raise AssertionError("provider probe did not use its fixed no-side-effect response")

def scenario_from_content(content):
    lowered = (content or "").lower()
    for marker, scenario in [
        ("multi gate decision then approval can complete", 4),
        ("approval rejection fails safely", 3),
        ("decision can be resolved", 1),
        ("schema failure is observable", 7),
        ("memory recall is visible", 8),
        ("stream usage is recorded", 9),
        ("happy path completes", 0),
    ]:
        if marker in lowered:
            return scenario
    return 0

def next_scenario(request):
    global request_sequence
    run_key = request.get("runKey") or request.get("runId") or ""
    if run_key and run_key in run_scenarios:
        request["scenario"] = run_scenarios[run_key]
        return run_scenarios[run_key]
    scenario = scenario_from_content(request.get("content"))
    if run_key:
        run_scenarios[run_key] = scenario
    request["scenario"] = scenario
    if request_sequence <= scenario:
        request_sequence = scenario + 1
    return scenario

def validation_ops(request):
    # C12 permits validation coverage only through an owner-executed managed
    # recipe. file/read remains a non-credit diagnostic capability.
    contract = select_operation_contract(request, "validation/managed_session")
    kind = object_value(contract, "kind", "Kind")
    if kind != "validation/managed_session":
        raise ValueError("fixture has no supported sealed validation operation: %r" % kind)
    return {
        "operations": [provider_operation(contract, kind, [
            {"name": "recipeId", "string": operation_validation_recipe_id(contract)},
        ])]
    }

def probe_validation_ops():
    # Health probing happens before a Runtime authority exists.  It must
    # therefore stay outside the sealed-contract path above: the response is
    # never submitted to WorkOrch and has no durable effect.  Keeping this
    # tiny fixed response avoids making the probe a hidden fallback for real
    # candidate requests, which must always consume C3's operation contract.
    return {
        "operations": [{
            "kind": "file/read",
            "targetRefs": ["runtime-validation-source.txt"],
            "parameters": [
                {"name": "optional", "boolean": True},
                {"name": "maxBytes", "integer": 16384},
            ],
        }]
    }

def state_change_ops(request):
    focus = request["focusId"]
    contract = select_operation_contract(request, "file/edit", "patch/apply", "file/write")
    kind = object_value(contract, "kind", "Kind")
    previous_content = fixture_file_content("focus-%03d" % (focus_ordinal(focus) - 1)) if focus_ordinal(focus) > 1 else "Runtime bounded terminal fixture input.\n"
    next_content = fixture_file_content(focus)
    if kind == "file/edit":
        parameters = [
            {"name": "oldText", "string": previous_content},
            {"name": "newText", "string": next_content},
        ]
    elif kind == "patch/apply":
        parameters = [
            {"name": "patchType", "string": "file_write"},
            {"name": "content", "string": next_content},
        ]
    elif kind == "file/write":
        parameters = [{"name": "content", "string": next_content}]
    else:
        raise ValueError("fixture has no supported sealed state-change operation: %r" % kind)
    return {
        "operations": [provider_operation(contract, kind, parameters)]
    }

def assessment_payload(request, satisfied=True, summary="Runtime assessment completed."):
    refs = list(request.get("evidenceRefs") or [])
    coverage = list(request.get("coverage") or [])
    items = [{
        "coverageWorkItemId": coverage_work_item_id,
        "satisfied": bool(satisfied),
        "evidenceRefs": refs if satisfied else [],
    } for coverage_work_item_id in coverage]
    return {
        "focusId": request["focusId"],
        "coverageWorkItemIds": coverage,
        "disposition": "satisfied" if satisfied else "repairable_gap",
        "items": items,
        "summary": summary,
        "evidenceRefs": refs,
    }

def governance_payload(request, kind, reason):
    return {
        "externalDependency": {
            "kind": kind,
            "subjectRef": request.get("runId") or request.get("focusId") or "runtime",
            "reason": reason,
        }
    }

def decision_payload(request):
    run_id = request.get("runId") or ""
    required_state = (request.get("requiredState") or "").strip()
    candidate_mode_by_state = {
        "need_state_change": "produce_state_change",
        "need_validation": "produce_validation",
        "need_assessment": "produce_assessment",
        "need_governance": "request_governance",
    }
    # The provider probe intentionally lacks a Runtime authority context.
    # A normal candidate request must declare the canonical requiredState so a
    # schema/authority drift fails the verification rather than being hidden by
    # a default canned operation shape.
    if run_id == "probe":
        action = candidate_mode_by_state.get(required_state, "produce_validation")
    else:
        action = candidate_mode_by_state.get(required_state)
        if not action:
            raise ValueError("missing or unsupported requiredState: %r" % required_state)
    scenario = next_scenario(request)
    run_key = request.get("runKey") or run_id

    if run_id == "probe":
        # The probe only establishes that the configured provider endpoint is
        # reachable. It is not a Runtime candidate and must neither require
        # nor infer a sealed operation contract.
        return probe_validation_ops()

    # No-candidate framing scenario intentionally returns coherent non-JSON for assessment.
    if scenario == 7 and action == "produce_assessment":
        return "not-json"

    if action == "produce_validation":
        validation_done.add(run_key)
        return validation_ops(request)

    if action == "produce_state_change":
        return state_change_ops(request)

    if action == "produce_assessment":
        # After validation evidence exists in context, close the focus.
        return assessment_payload(
            request,
            True,
            {
                0: "Happy path DoD satisfied.",
                1: "Decision path completed.",
                2: "Decision path completed.",
                3: "Approval path assessment.",
                4: "Multi-gate path completed.",
                6: "Multi-gate path completed.",
                8: "Memory recall DoD satisfied.",
                9: "Stream usage path completed.",
            }.get(scenario, "Runtime assessment completed."),
        )

    if action == "request_governance":
        # V2 opens Decision tickets only; kind/reason still drive ticket text.
        if scenario in {3}:
            return governance_payload(
                request,
                "approval_required",
                "Approval required before risky action. action=continue risky test path scenario=approval",
            )
        if scenario in {4, 5, 6}:
            # multi-gate: first governance is missing info; later ones may re-enter.
            return governance_payload(
                request,
                "missing_info",
                "Need missing detail before the approval gate. scenario=decision_then_approval",
            )
        if scenario in {1, 2}:
            return governance_payload(
                request,
                "missing_info",
                "Need missing implementation detail. scenario=decision",
            )
        return governance_payload(request, "missing_info", "Need operator direction before continuing.")

    # Unknown action: fail closed with validation ops rather than legacy fields.
    return validation_ops(request)

def usage_payload(request):
    run_id = request.get("runId", "")
    if run_id == "probe":
        return None
    scenario = request.get("scenario", 0)
    # Keep scenario-0 without usage so e2e can assert estimated fallback path.
    if scenario == 0:
        return None
    return {"prompt_tokens": 64, "completion_tokens": 16, "total_tokens": 80}

def stream_usage_payload(model):
    model = (model or "").strip()
    if "stream-native" in model:
        return {"prompt_tokens": 72, "completion_tokens": 18, "total_tokens": 90}
    return None

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        path = self.path.split("?", 1)[0]
        if path in ("/v1/models", "/models"):
            models = [
                {"id": "verify-model", "object": "model", "owned_by": "verify", "supported_endpoints": ["/v1/responses", "/v1/embeddings"], "contextWindowTokens": 128000, "maxOutputTokens": 8192},
                {"id": "gpt-4o", "object": "model", "owned_by": "openai", "supported_endpoints": ["/v1/responses"], "contextWindowTokens": 128000, "maxOutputTokens": 8192},
                {"id": "text-embedding-3-small", "object": "model", "owned_by": "openai", "supported_endpoints": ["/v1/embeddings"], "contextWindowTokens": 8192, "maxOutputTokens": 1024},
                {"id": "verify-stream-native-model", "object": "model", "owned_by": "verify", "supported_endpoints": ["/v1/responses", "/v1/embeddings"], "contextWindowTokens": 128000, "maxOutputTokens": 8192},
                {"id": "verify-stream-estimated-model", "object": "model", "owned_by": "verify", "supported_endpoints": ["/v1/responses", "/v1/embeddings"], "contextWindowTokens": 128000, "maxOutputTokens": 8192},
                {"id": "web-search-e2e-model", "object": "model", "owned_by": "verify", "supported_endpoints": ["/v1/responses", "/v1/embeddings"], "contextWindowTokens": 128000, "maxOutputTokens": 8192},
                {"id": "container-verify-model", "object": "model", "owned_by": "verify", "supported_endpoints": ["/v1/responses", "/v1/embeddings"], "contextWindowTokens": 128000, "maxOutputTokens": 8192},
            ]
            encoded = json.dumps({"object": "list", "data": models}).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)
            return
        self.send_response(404)
        self.end_headers()

    def do_POST(self):
        if self.path == "/v1/embeddings":
            size = int(self.headers.get("Content-Length", "0"))
            body = self.rfile.read(size)
            try:
                payload = json.loads(body)
            except Exception:
                payload = {}
            input_text = payload.get("input", "")
            seed = sum(ord(ch) for ch in str(input_text)) or 1
            vector = [round(((seed % 17) + 1) / 17, 6), round(((seed % 23) + 1) / 23, 6), round(((seed % 29) + 1) / 29, 6)]
            encoded = json.dumps({"data": [{"embedding": vector}]}).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)
            return
        if self.path != "/v1/responses":
            self.send_response(404)
            self.end_headers()
            return
        size = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(size)
        payload = json.loads(body)
        provider_request = provider_request_from_payload(payload)
        decision = semantic_plan_payload(provider_request) if provider_request.get("documentKind") == "semantic_plan" else decision_payload(provider_request)
        print("scenario=%s requiredState=%s runId=%s stepId=%s kind=%s" % (
            provider_request.get("scenario", ""),
            provider_request.get("requiredState", ""),
            provider_request.get("runId", ""),
            provider_request.get("stepId", ""),
            type(decision).__name__ if not isinstance(decision, dict) else ",".join(decision.keys()),
        ), flush=True)
        content = decision if isinstance(decision, str) else json.dumps(decision, separators=(",", ":"))
        model = payload.get("model", "")
        if payload.get("stream"):
            stream_usage = stream_usage_payload(model)
            midpoint = max(1, len(content) // 2)
            chunks = [content[:midpoint], content[midpoint:]]
            events = [{"type": "response.output_text.delta", "delta": chunk} for chunk in chunks if chunk]
            completed = {"type": "response.completed", "response": {"status": "completed", "output": []}}
            if stream_usage is not None:
                completed["response"]["usage"] = {
                    "input_tokens": stream_usage["prompt_tokens"],
                    "output_tokens": stream_usage["completion_tokens"],
                    "total_tokens": stream_usage["total_tokens"],
                }
            events.append(completed)
            encoded = "".join("data: " + json.dumps(item, separators=(",", ":")) + "\n\n" for item in events).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Connection", "keep-alive")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)
            return
        usage = usage_payload(provider_request)
        response = {"status": "completed", "output_text": content, "output": []}
        if usage is not None:
            response["usage"] = {
                "input_tokens": usage["prompt_tokens"],
                "output_tokens": usage["completion_tokens"],
                "total_tokens": usage["total_tokens"],
            }
        encoded = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)

host, port_text = os.environ.get("WORKORCH_LLM_VERIFY_MOCK_ADDR", "127.0.0.1:18181").split(":", 1)
port = int(port_text)
if os.environ.get("WORKORCH_LLM_VERIFY_MOCK_SELF_TEST") == "1":
    provider_request_parser_self_test()
    print("runtime mock C3 parser self-test passed", flush=True)
    raise SystemExit(0)
ThreadingHTTPServer((host, port), Handler).serve_forever()

PY
  WORKORCH_LLM_VERIFY_MOCK_SELF_TEST=1 python3 "$TMP_DIR/llm_mock_provider.py" >/dev/null
  WORKORCH_LLM_VERIFY_MOCK_ADDR="$MOCK_ADDR" python3 "$TMP_DIR/llm_mock_provider.py" >>"$MOCK_LOG" 2>&1 &
  MOCK_PID=$!
}

wait_ready() {
  for _ in $(seq 1 50); do
    if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl -fsS "$BASE_URL/healthz" >/dev/null
}

wait_mock() {
  for _ in $(seq 1 50); do
    if curl -fsS -X POST "http://$MOCK_ADDR/v1/responses" \
      -H 'Content-Type: application/json' \
      -d '{"input":"{\"runId\": \"probe\", \"stepId\": \"probe\"}"}' >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl -fsS -X POST "http://$MOCK_ADDR/v1/responses" \
    -H 'Content-Type: application/json' \
    -d '{"input":"{\"runId\": \"probe\", \"stepId\": \"probe\"}"}' >/dev/null
}

write_llm_update_payloads() {
  python3 - "$TMP_DIR/configuration.json" "$TMP_DIR/original-llm-update.json" "$TMP_DIR/verify-llm-update.json" "$MOCK_BASE_URL" <<'PY'
import json
import sys

config_path, original_path, update_path, mock_base_url = sys.argv[1:]
data = json.load(open(config_path, encoding="utf-8"))
llm = data.get("configuration", {}).get("llm", {})
ai_proxy = llm.get("aiProxy") or {}
default_chat = llm.get("defaultChat") or {}
default_embedding = llm.get("defaultEmbedding") or {}
budget = llm.get("budget") or {}

def normalize_url(value):
    return str(value or "").strip().rstrip("/")

if normalize_url(ai_proxy.get("baseUrl") or ai_proxy.get("baseURL")) != normalize_url(mock_base_url):
    raise SystemExit(
        "managed ai-proxy base URL does not match this mock; set it in the managed TOML before startup "
        f"(configured={ai_proxy.get('baseUrl')!r}, expected={mock_base_url!r})"
    )

original = {
    "defaultChatModelId": default_chat.get("modelId") or default_chat.get("modelID") or "verify-model",
    "budget": {
        "contextUtilizationRatio": float(budget.get("contextUtilizationRatio") or 0.75),
        "estimatedTokensPerByte": float(budget.get("estimatedTokensPerByte") or 0.25),
    },
}
if default_embedding.get("modelId") or default_embedding.get("modelID"):
    original["defaultEmbeddingModelId"] = default_embedding.get("modelId") or default_embedding.get("modelID")
else:
    original["clearDefaultEmbedding"] = True
with open(original_path, "w", encoding="utf-8") as handle:
    json.dump(original, handle, separators=(",", ":"))

update = {
    "defaultChatModelId": "verify-model",
    "defaultEmbeddingModelId": "verify-model",
    "budget": original["budget"],
}
with open(update_path, "w", encoding="utf-8") as handle:
    json.dump(update, handle, separators=(",", ":"))
PY
}

activate_workspace_execution() {
  local output_file="$1"
  local activation_dod_id="${2:-$dod_id}"
  local request_id="llm-e2e-$(basename "${output_file%.json}")"
	capture_runs_before_activation "$output_file"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
    -H 'Content-Type: application/json' \
    -d '{"intent":"'"$ACTIVATION_INTENT"'","requestId":"'"$request_id"'","reason":"e2e_restart","source":"e2e"}' >"$output_file"
}

activate_workspace_execution_without_memory() {
  local output_file="$1"
  local activation_dod_id="${2:-$dod_id}"
  local request_id="llm-e2e-$(basename "${output_file%.json}")"
  capture_runs_before_activation "$output_file"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
    -H 'Content-Type: application/json' \
    -d '{"intent":"'"$ACTIVATION_INTENT"'","requestId":"'"$request_id"'","reason":"e2e_restart","source":"e2e"}' >"$output_file"
}

activate_workspace_execution_with_auto_memory() {
  local output_file="$1"
  local activation_dod_id="${2:-$dod_id}"
  local request_id="llm-e2e-$(basename "${output_file%.json}")"
  capture_runs_before_activation "$output_file"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
    -H 'Content-Type: application/json' \
    -d '{"intent":"'"$ACTIVATION_INTENT"'","requestId":"'"$request_id"'","reason":"e2e_restart","source":"e2e"}' >"$output_file"
}

activate_workspace_execution_with_provider() {
  local output_file="$1"
  local provider_id="$2"
  local model_id="${3:-verify-model}"
	local activation_dod_id="${4:-$dod_id}"
	local request_id="llm-e2e-$(basename "${output_file%.json}")"
	local workspace_revision
	capture_runs_before_activation "$output_file"
	workspace_revision="$(curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id" | python3 -c 'import json,sys; print(json.load(sys.stdin)["workspace"]["revision"])')"
  curl -fsS -X PUT "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/llm-selection" \
    -H 'Content-Type: application/json' \
    -d '{"providerId":"'"$provider_id"'","modelId":"'"$model_id"'","expectedRevision":'"$workspace_revision"'}' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
    -H 'Content-Type: application/json' \
    -d '{"intent":"'"$ACTIVATION_INTENT"'","requestId":"'"$request_id"'","reason":"e2e_restart","source":"e2e"}' >"$output_file"
}

capture_runs_before_activation() {
  local output_file="$1"
  local before_file="${output_file%.json}-before-runs.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" >"$before_file"
}

wait_activation_run() {
  local activation_file="$1"
  local output_file="$2"
  local snapshot_file="${output_file%.json}-snapshot.json"
  local runs_file="${output_file%.json}-runs.json"
  local before_runs_file="${activation_file%.json}-before-runs.json"
  local run_id=""
  local deadline=$((SECONDS + ACTIVATION_RUN_WAIT_SECONDS))

python3 - "$activation_file" <<'PY'
import json
import os
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
errors = []
if data.get("accepted") is not True:
    errors.append(f"accepted is {data.get('accepted')!r}, want true")
expected_intent = os.environ.get("WORKORCH_LLM_VERIFY_ACTIVATION_INTENT", "restart")
if data.get("intent") != expected_intent:
    errors.append(f"intent is {data.get('intent')!r}, want {expected_intent}")
if not str(data.get("requestId") or "").strip():
    errors.append("requestId is empty")
valid_admissions = {
    "reconciliation_requested": {"current_run_continuation_accepted", "pending_run_activated", "new_run_activated"},
    "replacement_pending": {"duplicate_restart", "predecessor_supersede_requested"},
}
outcome = data.get("outcome")
owner_reason = data.get("reason")
if owner_reason not in valid_admissions.get(outcome, set()):
    errors.append(f"owner admission outcome/reason is {outcome!r}/{owner_reason!r}")
if data.get("state") != "accepted":
    errors.append(f"state is {data.get('state')!r}, want accepted")
if data.get("requestReason") != "e2e_restart":
    errors.append(f"requestReason is {data.get('requestReason')!r}, want e2e_restart")
if errors:
    raise SystemExit("\n".join(errors))
PY

  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$snapshot_file"; then
      if curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" >"$runs_file"; then
        run_id="$(python3 - "$runs_file" "$before_runs_file" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)
before = set()
try:
    with open(sys.argv[2], "r", encoding="utf-8") as handle:
        before_data = json.load(handle)
    before = {str(item.get("id") or "") for item in (before_data.get("runs") or []) if item.get("id")}
except FileNotFoundError:
    before = set()

new_runs = [
    item for item in (data.get("runs") or [])
    if item.get("id") and str(item.get("id")) not in before
]
if new_runs:
    print(new_runs[-1]["id"])
    raise SystemExit(0)
raise SystemExit(1)
PY
)" && {
          python3 - "$runs_file" "$run_id" "$output_file" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)
run_id = sys.argv[2]
runs = data.get("runs") or []
run = next((item for item in runs if item.get("id") == run_id), {"id": run_id})
with open(sys.argv[3], "w", encoding="utf-8") as handle:
    json.dump({"run": run}, handle, ensure_ascii=False, indent=2)
    handle.write("\n")
PY
          echo "$run_id"
          return 0
        }
      fi
    fi
    sleep 1
  done

  echo "activation accepted but run did not materialize within ${ACTIVATION_RUN_WAIT_SECONDS}s for workspace $workspace_id" >&2
  cat "$activation_file" >&2 || true
  [[ -f "$snapshot_file" ]] && cat "$snapshot_file" >&2
  [[ -f "$runs_file" ]] && cat "$runs_file" >&2
  return 1
}

create_acceptance_workspace() {
  local prefix="$1"
  local title="$2"
  local description="$3"
  local dod_items_json="$4"

  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
    -d '{"title":"'"$title"'","description":"'"$description"'","templateId":"'"$C12_TEMPLATE_ID"'"}' >"$TMP_DIR/$prefix-workspace.json"
  workspace_id="$(json_path "$TMP_DIR/$prefix-workspace.json" "workspace.id")"
  [[ -n "$workspace_id" ]]
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" -H 'Content-Type: application/json' \
    -d '{"text":"verify LLM driven workspace execution"}' >"$TMP_DIR/$prefix-goal.json"
  goal_id="$(json_path "$TMP_DIR/$prefix-goal.json" "goal.id")"
  [[ -n "$goal_id" ]]
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" -H 'Content-Type: application/json' \
    -d '{"items":'"$dod_items_json"'}' >"$TMP_DIR/$prefix-dod.json"
  dod_id="$(json_path "$TMP_DIR/$prefix-dod.json" "dod.id")"
  [[ -n "$dod_id" ]]
  # Seed a non-mutating validation target under the workspace deliverable root.
  deliverable_dir="$(python3 -c 'import json,sys; ws=json.load(open(sys.argv[1],encoding="utf-8")).get("workspace") or {}; print(ws.get("deliverableDirectory") or ws.get("workingDirectory") or "")' "$TMP_DIR/$prefix-workspace.json")"
  if [[ -n "$deliverable_dir" ]]; then
    mkdir -p "$deliverable_dir"
    printf 'verified by llm e2e\n' >"$deliverable_dir/runtime-validation-source.txt"
  fi
}

create_c12_verification_template() {
  # generic-governed intentionally has no semantic proof declaration. C12
  # rejects it at activation rather than inferring coverage from file/read.
  # The live verifier supplies one explicit, immutable managed-validation
  # declaration whose exact target is seeded below before activation.
  cat >"$TMP_DIR/c12-template.json" <<JSON
{
  "id": "$C12_TEMPLATE_ID",
  "name": "Runtime C12 binding verifier $VERIFY_SUFFIX",
  "description": "Ephemeral managed-verification template with an explicit C12 proof binding.",
  "contextHints": {
    "stateChangePolicy": "allowed_but_not_required",
    "acceptanceSlicePolicy": {
      "schemaVersion": "workorch.acceptance_slice_policy",
      "maxWorkItems": 1,
      "maxFocuses": 4,
      "maxCandidateContextItems": 2,
      "completionPolicy": "all_required_items_accepted",
      "resultPolicy": "slice_result_allowed",
      "groups": [{
        "id": "runtime-c12-live",
        "title": "Runtime C12 live binding",
        "scope": "runtime_c12_live",
        "priority": 1,
        "maxWorkItems": 1,
        "match": {"all": true}
      }]
    }
  },
  "evidencePolicies": [{
    "id": "$C12_VALIDATION_RECIPE_ID",
    "appliesTo": "shell/command",
    "commandName": "sh",
    "recipeDeclared": true,
    "recipeArgs": ["-c", "test -f runtime-validation-source.txt && printf 'WORKORCH_CHECK_PASSED=$C12_VALIDATION_RECIPE_ID\\n'"],
    "recipeTimeoutMillis": 10000,
    "coverageRequirements": [{
      "id": "*",
      "scope": "dod_item",
      "requiredEvidenceKinds": ["validation"],
      "requireDirectEvidence": true,
      "acceptedStepKinds": ["validation/managed_session"],
      "rejectedStepKinds": ["file/read"],
      "validationRecipeId": "$C12_VALIDATION_RECIPE_ID",
      "requiredCheckRefs": ["$C12_VALIDATION_RECIPE_ID"],
      "targetRole": "managed_validation_subject",
      "targetRefs": ["runtime-validation-source.txt"],
      "requireTargetGrounding": true,
      "description": "managed recipe proves the seeded verification target exists"
    }]
  }]
}
JSON
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspace-templates" \
    -H 'Content-Type: application/json' \
    -d @"$TMP_DIR/c12-template.json" >"$TMP_DIR/c12-template-created.json"
  json_grep_string_value id "$C12_TEMPLATE_ID" "$TMP_DIR/c12-template-created.json" >/dev/null
}

assert_c12_binding_audit() {
  local snapshot_file="$1"
  local expected_workspace_id="$2"
  local expected_run_id="$3"

  python3 "$ROOT_DIR/scripts/verify_runtime_live_contract.py" \
    --snapshot "$snapshot_file" \
    --workspace-id "$expected_workspace_id" \
    --run-id "$expected_run_id" \
    --require-terminal
}

run_runtime_c7_fixture() {
  [[ "${WORKORCH_LLM_VERIFY_RUN_RUNTIME_C7:-0}" == "1" ]] || return 0

  local fixture_dir="$TMP_DIR/runtime-c7"
  mkdir -p "$fixture_dir"

  # This is a fresh-data, mock-provider proof executed inside the managed
  # instance. The fixture creator owns only its workspace/template/deliverable
  # data; the live verifier owns the two concurrent activations and terminal
  # checks. No manual Resume, data edit, decoder relaxation, or compatibility
  # route participates in the proof.
  WORKORCH_BASE_URL="$BASE_URL" \
    WORKORCH_E2E_ARTIFACT_DIR="$fixture_dir" \
    WORKORCH_E2E_PROVIDER_ID="ai-proxy" \
    WORKORCH_E2E_MODEL_ID="verify-model" \
    "$ROOT_DIR/scripts/create-runtime-bounded-positive-fixtures.sh"

  # shellcheck disable=SC1090
  source "$fixture_dir/terminal-env.sh"
  WORKORCH_BASE_URL="$BASE_URL" \
    WORKORCH_E2E_ARTIFACT_DIR="$fixture_dir/terminal-verification" \
    WORKORCH_E2E_PROVIDER_ID="ai-proxy" \
    WORKORCH_E2E_MODEL_ID="verify-model" \
    WORKORCH_E2E_TIMEOUT_SECONDS="${WORKORCH_LLM_VERIFY_C7_TIMEOUT_SECONDS:-180}" \
    "$ROOT_DIR/scripts/verify-runtime-positive-convergence-live.sh"
}

wait_run_state() {
  local run_id="$1"
  local expected_state="$2"
  local output_file="$3"
  local deadline=$((SECONDS + RUN_WAIT_SECONDS))

  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$output_file"
    if json_path_equals "$output_file" "status.state" "$expected_state"; then
      return 0
    fi
    sleep 1
  done

  cat "$output_file" >&2
  return 1
}

wait_run_state_any() {
  local run_id="$1"
  local output_file="$2"
  shift 2
  local deadline=$((SECONDS + RUN_WAIT_SECONDS))

  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$output_file"
    if python3 - "$output_file" "$@" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
state = (data.get("status") or {}).get("state")
raise SystemExit(0 if state in set(sys.argv[2:]) else 1)
PY
    then
      return 0
    fi
    sleep 1
  done

  cat "$output_file" >&2
  return 1
}

assert_run_recovery_read_models() {
  local run_id="$1"
  local label="$2"
  local expected_state="$3"
  local expected_usage_source="${4:-}"

  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/snapshot" >"$TMP_DIR/$label-snapshot-read-model.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/observability" >"$TMP_DIR/$label-observability-read-model.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/task-progress" >"$TMP_DIR/$label-task-progress.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/context-bundle" >"$TMP_DIR/$label-context-bundle.json"
  curl -fsS "$BASE_URL/api/v1/workorch/events?workspaceId=$workspace_id&runId=$run_id" >"$TMP_DIR/$label-events.json"
  python3 - "$TMP_DIR/$label-snapshot-read-model.json" \
    "$TMP_DIR/$label-observability-read-model.json" \
    "$TMP_DIR/$label-task-progress.json" \
    "$TMP_DIR/$label-context-bundle.json" \
    "$TMP_DIR/$label-events.json" \
    "$run_id" "$workspace_id" "$expected_state" "$expected_usage_source" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8"))["snapshot"]
observability = json.load(open(sys.argv[2], encoding="utf-8"))["observability"]
task_progress = json.load(open(sys.argv[3], encoding="utf-8"))["taskProgress"]
bundle = json.load(open(sys.argv[4], encoding="utf-8"))["contextBundle"]
events = json.load(open(sys.argv[5], encoding="utf-8")).get("eventRecords") or []
run_id, workspace_id, expected_state, expected_usage_source = sys.argv[6:10]

errors = []
run = snapshot.get("run") or {}
if run.get("id") != run_id:
    errors.append(f"snapshot run id is {run.get('id')!r}, want {run_id!r}")
if run.get("workspaceId") != workspace_id:
    errors.append(f"snapshot workspace id is {run.get('workspaceId')!r}, want {workspace_id!r}")
if run.get("state") != expected_state:
    errors.append(f"run state is {run.get('state')!r}, want {expected_state!r}")

# Runtime: compressed context is a read-only contextBundle projection.
bundle_compressed = bundle.get("compressedContext") or {}
if bundle.get("runId") not in ("", None, run_id) and bundle.get("runId") != run_id:
    errors.append(f"context bundle run id mismatch: {bundle.get('runId')!r} want {run_id!r}")
if bundle.get("workspaceId") not in ("", None, workspace_id) and bundle.get("workspaceId") != workspace_id:
    errors.append(f"context bundle workspace id mismatch: {bundle.get('workspaceId')!r}")
if bundle_compressed and bundle_compressed.get("version") not in (None, "", "workorch.compressedContext"):
    errors.append(f"context bundle compressedContext invalid: {bundle_compressed}")
section_ids = {item.get("id") for item in (bundle.get("sections") or [])}
if section_ids:
    for required in ["workspace", "run"]:
        if required not in section_ids:
            errors.append(f"context bundle missing section {required!r}: {section_ids}")

if task_progress.get("runId") not in ("", None, run_id) and task_progress.get("runId") != run_id:
    errors.append(f"taskProgress run id mismatch: {task_progress}")
if task_progress.get("workspaceId") not in ("", None, workspace_id) and task_progress.get("workspaceId") != workspace_id:
    errors.append(f"taskProgress workspace id mismatch: {task_progress}")
if not task_progress.get("state"):
    errors.append(f"taskProgress state empty: {task_progress}")
if not (task_progress.get("milestones") or []):
    errors.append(f"taskProgress milestones empty: {task_progress}")

observed_progress = observability.get("taskProgress") or {}
if observed_progress and observed_progress.get("runId") not in ("", None, run_id) and observed_progress.get("runId") != run_id:
    errors.append(f"observability taskProgress missing run id: {observed_progress}")

usage_traces = observability.get("llmUsageTraces") or []
if not usage_traces:
    if expected_usage_source:
        errors.append("observability missing llmUsageTraces")
else:
    estimated_entries = 0
    native_entries = 0
    for trace in usage_traces:
        estimated = bool(trace.get("estimated"))
        usage_source = trace.get("usageSource")
        if not usage_source:
            errors.append(f"llm usage trace missing usageSource: {trace}")
            continue
        if estimated:
            estimated_entries += 1
            if usage_source != "estimated":
                errors.append(f"llm usage trace estimated/source mismatch: {trace}")
        else:
            native_entries += 1
            if usage_source != "provider_native":
                errors.append(f"llm usage trace native/source mismatch: {trace}")

    summary = observability.get("llmUsageSummary") or {}
    if summary.get("entryTotal") != len(usage_traces):
        errors.append(f"llm usage summary entryTotal mismatch: {summary} vs traces={len(usage_traces)}")
    if summary.get("estimatedEntries") != estimated_entries:
        errors.append(f"llm usage summary estimatedEntries mismatch: {summary} vs {estimated_entries}")
    if summary.get("providerNativeEntries") != native_entries:
        errors.append(f"llm usage summary providerNativeEntries mismatch: {summary} vs {native_entries}")
    if expected_usage_source:
        if not any((item.get("usageSource") or "") == expected_usage_source for item in usage_traces):
            errors.append(f"llm usage traces missing expected usageSource={expected_usage_source!r}: {usage_traces}")

topics = {item.get("topic") for item in events}
if "/run/created" not in topics:
    errors.append(f"event stream missing run-created event: {topics}")
if expected_state in {"completed", "failed"} and "/run/state/changed" not in topics:
    errors.append(f"event stream missing terminal run state event: {topics}")
if any(item.get("state") == "accepted" for item in (snapshot.get("results") or [])):
    # Runtime always emits accepted Result; Artifact materialization is optional.
    if "/result/accepted" not in topics:
        errors.append(f"event stream missing /result/accepted: {topics}")

if expected_state == "completed":
    if not [item for item in (snapshot.get("results") or []) if item.get("state") == "accepted"]:
        # Some completed paths may only expose accepted outcome via resultQuality/acceptance.
        has_accepted = any(
            (item.get("state") == "accepted") for item in (snapshot.get("results") or [])
        ) or ((snapshot.get("outcome") or {}).get("accepted") is True)
        if not has_accepted:
            errors.append("completed run missing accepted Result")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY
}

assert_workspace_recovery_read_models() {
  local label="$1"

  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$TMP_DIR/$label-workspace-snapshot.json"
  python3 - "$TMP_DIR/$label-workspace-snapshot.json" "$workspace_id" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8"))["snapshot"]
progress = snapshot["taskProgress"]
bundle = snapshot["contextBundle"]
workspace_id = sys.argv[2]
errors = []
if progress.get("workspaceId") != workspace_id:
    errors.append(f"workspace taskProgress identity mismatch: {progress}")
if not progress.get("state"):
    errors.append(f"workspace taskProgress state empty: {progress}")
if bundle.get("workspaceId") != workspace_id:
    errors.append(f"workspace context bundle identity mismatch: {bundle}")
if (bundle.get("compressedContext") or {}).get("version") != "workorch.compressedContext":
    errors.append(f"workspace context bundle missing compressedContext: {bundle}")
if not (bundle.get("sections") or []):
    errors.append(f"workspace context bundle sections empty: {bundle}")
if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY
}

assert_run_llm_usage_semantics() {
  local run_id="$1"
  local expected_usage_source="$2"
  local expected_response_mode="$3"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/observability" >"$TMP_DIR/$run_id-observability-usage.json"
  python3 - "$TMP_DIR/$run_id-observability-usage.json" "$expected_usage_source" "$expected_response_mode" <<'PY'
import json
import sys

observability = json.load(open(sys.argv[1], encoding="utf-8")).get("observability") or {}
expected_source = sys.argv[2]
expected_response_mode = sys.argv[3]
traces = observability.get("llmUsageTraces") or []

if not traces:
    raise SystemExit("observability llmUsageTraces is empty")

matched = [item for item in traces if (item.get("usageSource") or "") == expected_source]
if not matched:
    raise SystemExit(f"missing usageSource={expected_source!r} in traces: {traces}")
physical = [item for item in matched if item.get("physicalResponseMode") == expected_response_mode]
if not physical:
    raise SystemExit(
        f"usageSource={expected_source!r} has no trace with physicalResponseMode={expected_response_mode!r}: {matched}"
    )
for item in physical:
    logical_id = item.get("logicalRequestId") or ""
    physical_id = item.get("physicalCallId") or ""
    if item.get("providerSubmitted") is not True:
        raise SystemExit(f"provider usage trace is not marked submitted: {item}")
    if not logical_id or not physical_id:
        raise SystemExit(f"Runtime provider usage trace is missing two-level request identity: {item}")
PY
}

assert_workspace_llm_usage_breakdown() {
  local workspace_id="$1"
  local deadline=$((SECONDS + 15))
  local error_file="$TMP_DIR/workspace-llm-usage-error.txt"
  while true; do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot?view=observability" >"$TMP_DIR/workspace-llm-usage-snapshot.json"
    if python3 - "$TMP_DIR/workspace-llm-usage-snapshot.json" >"$error_file" 2>&1 <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8")).get("snapshot") or {}
summary = snapshot.get("llmUsageSummary") or {}
breakdown = summary.get("providerModelBreakdown") or []

if not breakdown:
    raise SystemExit(f"workspace llm usage summary missing providerModelBreakdown: {summary}")

required = {
    ("ai-proxy", "verify-model"),
    ("ai-proxy", "verify-stream-native-model"),
    ("ai-proxy", "verify-stream-estimated-model"),
}
present = {(item.get("providerId"), item.get("model")) for item in breakdown}
missing = required - present
if missing:
    raise SystemExit(f"workspace provider/model breakdown missing required entries: {sorted(missing)}; present={sorted(present)}")

for item in breakdown:
    if (item.get("providerId"), item.get("model")) in required and int(item.get("entryTotal") or 0) <= 0:
        raise SystemExit(f"workspace provider/model breakdown has empty entryTotal: {item}")
PY
    then
      return 0
    fi
    if [[ "$SECONDS" -ge "$deadline" ]]; then
      cat "$error_file" >&2 || true
      return 1
    fi
    sleep 1
  done
}

trap report_failure ERR
trap timeout_failure TERM
trap cleanup EXIT

cd "$ROOT_DIR"
start_watchdog
start_mock_provider
wait_mock
wait_ready

curl -fsS "$BASE_URL/readyz" | json_grep_string_value status ready >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
write_llm_update_payloads
curl -fsS -X POST "$BASE_URL/api/v1/workorch/configuration/llm/models/refresh" >"$TMP_DIR/catalog-refreshed.json"
json_grep_string_value providerId ai-proxy "$TMP_DIR/catalog-refreshed.json" >/dev/null
curl -fsS -X PUT "$BASE_URL/api/v1/workorch/configuration/llm" -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/verify-llm-update.json" >"$TMP_DIR/configuration-updated.json"

run_runtime_c7_fixture
create_c12_verification_template

create_acceptance_workspace "happy" "llm acceptance workspace" "verify LLM driven workspace execution" '["happy path completes"]'
curl -fsS "$BASE_URL/api/v1/workorch/workspaces" | json_grep_string_value id "$workspace_id" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id" | json_grep_string_value description "verify LLM driven workspace execution" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" | json_grep_string_value id "$goal_id" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals/$goal_id" >"$TMP_DIR/goal-read.json"
goal_text="$(json_path "$TMP_DIR/goal-read.json" "goal.text")"
[[ "$goal_text" == *"verify LLM driven workspace execution"* ]]
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" | json_grep_string_value id "$dod_id" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods/$dod_id" | grep '"happy path completes"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$TMP_DIR/workspace-snapshot-defined.json"
json_grep_string_value id "$workspace_id" "$TMP_DIR/workspace-snapshot-defined.json" >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/mcp/servers" -H 'Content-Type: application/json' \
  -d '{"id":"'"$MCP_SERVER_ID"'","name":"MCP Context Verify","transport":"stdio","endpoint":"context-helper","enabled":true,"allowedTools":["repo.summary"],"allowedResources":["workspace://acceptance"],"allowedPrompts":["workorch.acceptance"],"defaultApprovalPolicy":"operator","sessionPolicy":"long_lived","timeoutMillis":60000}' >"$TMP_DIR/mcp-server.json"
json_grep_string_value id "$MCP_SERVER_ID" "$TMP_DIR/mcp-server.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/servers" | json_grep_string_value id "$MCP_SERVER_ID" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID" | json_grep_string_value sessionPolicy long_lived >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID/tools" | json_grep_string_value name repo.summary >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID/resources" | json_grep_string_value uri "workspace://acceptance" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID/prompts" | json_grep_string_value name workorch.acceptance >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/skills" -H 'Content-Type: application/json' \
  -d '{"id":"'"$SKILL_ID"'","name":"LLM acceptance skill","version":"v1","description":"acceptance skill for governed execution","triggerRules":["workspace acceptance"],"instruction":"Use the provided Goal, DoD, MCP and Memory context to complete the run.","requiredToolKinds":["llm/chat","shell/command"],"requiredMCPServerRefs":["'"$MCP_SERVER_ID"'"],"constraints":["must preserve governance gates"]}' >"$TMP_DIR/skill.json"
json_grep_string_value id "$SKILL_ID" "$TMP_DIR/skill.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/skills" | json_grep_string_value id "$SKILL_ID" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/skills/$SKILL_ID" | grep '"requiredMCPServerRefs"' >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/souls" -H 'Content-Type: application/json' \
  -d '{"id":"'"$SOUL_ID"'","name":"LLM Acceptance Soul","description":"acceptance soul for governed execution","businessDomains":["workorch"],"soulMd":"# LLM Acceptance Soul\n\nDrive the workspace according to Goal and DoD while respecting governance gates.\n\n## Planning\n\ngoal-first\n\n## Decision\n\nask when missing info is required\n\n## Approval\n\nrequire operator approval for risky action\n\n## Validation\n\nproduce DoD evidence\n\n## Output Contract\n\nworkorch llm step contract","allowedSkillRefs":["'"$SKILL_ID"'"],"allowedMCPServerRefs":["'"$MCP_SERVER_ID"'"],"allowedToolKinds":["llm/chat","shell/command"]}' >"$TMP_DIR/soul.json"
json_grep_string_value id "$SOUL_ID" "$TMP_DIR/soul.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/souls" | json_grep_string_value id "$SOUL_ID" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/souls/$SOUL_ID" | grep '"soulMd"' >/dev/null

# Workspace execution activation is intentionally an intent-only ingress.  The
# execution context is compiled from owner state, so bind the acceptance soul
# to the workspace through its owner endpoint before issuing resume/restart.
curl -fsS -X PUT "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/soul-bindings" \
  -H 'Content-Type: application/json' \
  -d '{"defaultSoulId":"'"$SOUL_ID"'","bindings":{"planning":"'"$SOUL_ID"'","execution":"'"$SOUL_ID"'","decision":"'"$SOUL_ID"'","approval":"'"$SOUL_ID"'","validation":"'"$SOUL_ID"'","artifact":"'"$SOUL_ID"'"}}' >"$TMP_DIR/workspace-soul-bindings.json"
json_grep_string_value defaultSoulId "$SOUL_ID" "$TMP_DIR/workspace-soul-bindings.json" >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/memory" -H 'Content-Type: application/json' \
  -d '{"memoryId":"'"$MEMORY_ID"'","sourceType":"user","memoryType":"project_context","content":"Seed memory for acceptance execution.","summary":"Seeded acceptance memory is available to runs.","tags":["acceptance","seed"],"importance":8,"confidence":9,"visibility":"workspace","embeddingProviderId":"verify","embeddingModel":"verify-embedding","embeddingText":"seeded acceptance memory","vector":[0.1,0.2,0.3]}' >"$TMP_DIR/memory-seed.json"
json_grep_string_value id "$MEMORY_ID" "$TMP_DIR/memory-seed.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/memory?workspaceId=$workspace_id" | grep "\"$MEMORY_ID\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/memory?workspaceId=$workspace_id&q=Seeded" | grep "\"$MEMORY_ID\"" >/dev/null
if curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/memory" -H 'Content-Type: application/json' \
  -d '{"memoryId":"memory-invalid-user-run-summary-'"$VERIFY_SUFFIX"'","sourceType":"user","memoryType":"run_summary","summary":"should be rejected"}' >"$TMP_DIR/memory-invalid-user.json" 2>/dev/null; then
  echo "expected user run_summary memory write to be rejected" >&2
  exit 1
fi
if curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/memory" -H 'Content-Type: application/json' \
  -d '{"memoryId":"memory-invalid-run-decision-'"$VERIFY_SUFFIX"'","sourceType":"run","sourceRef":"run-invalid-'"$VERIFY_SUFFIX"'","memoryType":"decision","summary":"single approval raw text","content":"approval transcript"}' >"$TMP_DIR/memory-invalid-run.json" 2>/dev/null; then
  echo "expected runtime decision memory write to be rejected" >&2
  exit 1
fi

activate_workspace_execution "$TMP_DIR/happy-activation.json"
happy_run_id="$(wait_activation_run "$TMP_DIR/happy-activation.json" "$TMP_DIR/happy-activation-run.json")"
wait_run_state "$happy_run_id" "completed" "$TMP_DIR/happy-completed-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs?workspaceId=$workspace_id" | json_grep_string_value id "$happy_run_id" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$happy_run_id" | json_grep_string_value state completed >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$happy_run_id/status" >"$TMP_DIR/happy-status-confirm.json"
json_path_equals "$TMP_DIR/happy-status-confirm.json" "status.state" "completed"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$happy_run_id/snapshot" >"$TMP_DIR/happy-snapshot.json"
# Runtime: accepted Result carries doDAssessment; V1 memoryRefs/soulRouting/skillIds are gone.
grep -E '"doDAssessment"|"dodAssessment"' "$TMP_DIR/happy-snapshot.json" >/dev/null
grep '"state": "accepted"' "$TMP_DIR/happy-snapshot.json" >/dev/null
grep '"state": "completed"' "$TMP_DIR/happy-snapshot.json" >/dev/null
assert_c12_binding_audit "$TMP_DIR/happy-snapshot.json" "$workspace_id" "$happy_run_id"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$happy_run_id/observability" >"$TMP_DIR/happy-observability.json"
json_grep_bool_value terminal true "$TMP_DIR/happy-observability.json" >/dev/null
assert_run_recovery_read_models "$happy_run_id" "happy" "completed" "estimated"

create_acceptance_workspace "decision" "llm acceptance decision workspace" "verify LLM decision governance" '["decision can be resolved"]'
activate_workspace_execution_without_memory "$TMP_DIR/decision-activation.json"
decision_run_id="$(wait_activation_run "$TMP_DIR/decision-activation.json" "$TMP_DIR/decision-activation-run.json")"
wait_run_state_any "$decision_run_id" "$TMP_DIR/decision-current-status.json" "blocked_on_decision" "completed"
decision_state="$(json_path "$TMP_DIR/decision-current-status.json" "status.state")"
if [[ "$decision_state" == "blocked_on_decision" ]]; then
  ticket_id="$(json_path "$TMP_DIR/decision-current-status.json" "status.blockingTicketId")"
  ticket_path="$(url_encode "$ticket_id")"
  python3 - "$TMP_DIR/decision-current-status.json" <<'PY'
import json
import sys

state = (json.load(open(sys.argv[1], encoding="utf-8")).get("status") or {}).get("state")
raise SystemExit(0 if state in {"blocked_on_decision", "running", "completed"} else 1)
PY
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id" | json_grep_string_value id "$ticket_id" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id&runId=$decision_run_id" | json_grep_string_value id "$ticket_id" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_path?workspaceId=$workspace_id" >"$TMP_DIR/decision-detail-before-resolve.json"
  json_grep_string_value decisionKind provide_missing_info "$TMP_DIR/decision-detail-before-resolve.json" >/dev/null
  resolve_decision_ticket_if_needed \
    "$ticket_path" \
    '{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"llm-acceptance"}}' \
    "$TMP_DIR/decision-detail-before-resolve.json" \
    "$TMP_DIR/decision-resolve.json"
elif [[ "$decision_state" == "completed" ]]; then
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id&runId=$decision_run_id" >"$TMP_DIR/decision-auto-resolved-list.json"
  ticket_id="$(python3 - "$TMP_DIR/decision-auto-resolved-list.json" <<'PY'
import json
import sys
items = json.load(open(sys.argv[1], encoding="utf-8")).get("decisions") or []
print(items[0].get("id", "") if items else "")
PY
)"
  if [[ -n "$ticket_id" ]]; then
    ticket_path="$(url_encode "$ticket_id")"
    curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_path?workspaceId=$workspace_id" >"$TMP_DIR/decision-auto-resolved-detail.json"
    json_grep_string_value decisionKind provide_missing_info "$TMP_DIR/decision-auto-resolved-detail.json" >/dev/null
    json_grep_string_value state resolved "$TMP_DIR/decision-auto-resolved-detail.json" >/dev/null
  fi
else
  echo "decision scenario state=$decision_state, want blocked_on_decision or completed" >&2
  exit 1
fi
wait_run_state "$decision_run_id" "completed" "$TMP_DIR/decision-after-resolve-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$decision_run_id/observability" | json_grep_bool_value terminal true >/dev/null
assert_run_recovery_read_models "$decision_run_id" "decision" "completed" ""

# Runtime governance opens Decision tickets only (not Approval items).
create_acceptance_workspace "approval" "llm acceptance approval workspace" "verify LLM approval governance" '["approval rejection fails safely"]'
activate_workspace_execution_without_memory "$TMP_DIR/approval-activation.json"
approval_run_id="$(wait_activation_run "$TMP_DIR/approval-activation.json" "$TMP_DIR/approval-activation-run.json")"
wait_run_state_any "$approval_run_id" "$TMP_DIR/approval-blocked-status.json" "blocked_on_decision" "failed" "completed"
approval_state="$(json_path "$TMP_DIR/approval-blocked-status.json" "status.state")"
if [[ "$approval_state" == "blocked_on_decision" ]]; then
  approval_ticket_id="$(json_path "$TMP_DIR/approval-blocked-status.json" "status.blockingTicketId")"
  approval_ticket_path="$(url_encode "$approval_ticket_id")"
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$approval_ticket_path?workspaceId=$workspace_id" >"$TMP_DIR/approval-decision-detail.json"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$approval_ticket_path/resolve?workspaceId=$workspace_id"     -H 'Content-Type: application/json'     -d '{"resolutionType":"reject_current_path","resolutionPayload":{"operator":"llm-acceptance","reason":"reject risky path"}}'     >"$TMP_DIR/approval-decision-reject.json" || true
  wait_run_state_any "$approval_run_id" "$TMP_DIR/approval-rejected-status.json" "failed" "completed"
  final_approval_state="$(json_path "$TMP_DIR/approval-rejected-status.json" "status.state")"
else
  final_approval_state="$approval_state"
fi
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$approval_run_id/observability" | json_grep_bool_value terminal true >/dev/null
assert_run_recovery_read_models "$approval_run_id" "approval-rejected" "$final_approval_state" ""

create_acceptance_workspace "multi-gate" "llm acceptance multi-gate workspace" "verify LLM decision then approval governance" '["multi gate decision then approval can complete"]'
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods/$dod_id" | grep '"multi gate decision then approval can complete"' >/dev/null
activate_workspace_execution_without_memory "$TMP_DIR/multi-gate-decision-activation.json"
multi_gate_run_id="$(wait_activation_run "$TMP_DIR/multi-gate-decision-activation.json" "$TMP_DIR/multi-gate-decision-activation-run.json")"
wait_run_state_any "$multi_gate_run_id" "$TMP_DIR/multi-gate-decision-status.json" "blocked_on_decision" "completed"
multi_gate_state="$(json_path "$TMP_DIR/multi-gate-decision-status.json" "status.state")"
multi_gate_ticket_id=""
multi_gate_approval_id=""
if [[ "$multi_gate_state" == "blocked_on_decision" ]]; then
  multi_gate_ticket_id="$(json_path "$TMP_DIR/multi-gate-decision-status.json" "status.blockingTicketId")"
  multi_gate_ticket_path="$(url_encode "$multi_gate_ticket_id")"
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$multi_gate_ticket_path?workspaceId=$workspace_id" | json_grep_string_value decisionKind provide_missing_info >/dev/null
  resolve_decision_ticket_if_needed \
    "$multi_gate_ticket_path" \
    '{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"llm-acceptance","nextGate":"approval"}}' \
    "$TMP_DIR/multi-gate-decision-detail-before-resolve.json" \
    "$TMP_DIR/multi-gate-decision-resolve.json"
  # V2: second governance gate is still a Decision ticket (not Approval).
  wait_run_state_any "$multi_gate_run_id" "$TMP_DIR/multi-gate-approval-status.json" "blocked_on_decision" "completed" "failed"
  multi_gate_second_state="$(json_path "$TMP_DIR/multi-gate-approval-status.json" "status.state")"
  if [[ "$multi_gate_second_state" == "blocked_on_decision" ]]; then
    multi_gate_approval_id="$(json_path "$TMP_DIR/multi-gate-approval-status.json" "status.blockingTicketId")"
    multi_gate_approval_path="$(url_encode "$multi_gate_approval_id")"
    curl -fsS "$BASE_URL/api/v1/workorch/decisions/$multi_gate_approval_path?workspaceId=$workspace_id" >"$TMP_DIR/multi-gate-approval-detail.json"
    json_grep_string_value state open "$TMP_DIR/multi-gate-approval-detail.json" >/dev/null || true
  fi
  if [[ -n "${multi_gate_approval_path:-}" ]]; then
    curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$multi_gate_approval_path/resolve?workspaceId=$workspace_id"       -H 'Content-Type: application/json'       -d '{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"llm-acceptance","gate":"approval-as-decision"}}'       >"$TMP_DIR/multi-gate-second-resolve.json" || true
  fi
  wait_run_state_any "$multi_gate_run_id" "$TMP_DIR/multi-gate-completed-status.json" "completed" "failed"
else
  cp "$TMP_DIR/multi-gate-decision-status.json" "$TMP_DIR/multi-gate-completed-status.json"
fi
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$multi_gate_run_id/snapshot" >"$TMP_DIR/multi-gate-snapshot.json"
if [[ -n "$multi_gate_ticket_id" ]]; then
  json_grep_string_value id "$multi_gate_ticket_id" "$TMP_DIR/multi-gate-snapshot.json" >/dev/null
fi
if [[ -n "$multi_gate_approval_id" ]]; then
  json_grep_string_value id "$multi_gate_approval_id" "$TMP_DIR/multi-gate-snapshot.json" >/dev/null
fi
grep 'multi gate decision then approval can complete' "$TMP_DIR/multi-gate-snapshot.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$multi_gate_run_id/status" >"$TMP_DIR/multi-gate-status-confirm.json"
json_path_equals "$TMP_DIR/multi-gate-status-confirm.json" "status.state" "completed"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$multi_gate_run_id/observability" >"$TMP_DIR/multi-gate-observability.json"
json_grep_bool_value terminal true "$TMP_DIR/multi-gate-observability.json" >/dev/null
assert_run_recovery_read_models "$multi_gate_run_id" "multi-gate" "completed" ""
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$TMP_DIR/workspace-snapshot-after-governance.json"
json_grep_string_value id "$multi_gate_run_id" "$TMP_DIR/workspace-snapshot-after-governance.json" >/dev/null
if [[ -n "$multi_gate_ticket_id" ]]; then
  json_grep_string_value id "$multi_gate_ticket_id" "$TMP_DIR/workspace-snapshot-after-governance.json" >/dev/null
fi
if [[ -n "$multi_gate_approval_id" ]]; then
  json_grep_string_value id "$multi_gate_approval_id" "$TMP_DIR/workspace-snapshot-after-governance.json" >/dev/null
fi

create_acceptance_workspace "schema" "llm acceptance schema workspace" "verify LLM schema failure observability" '["schema failure is observable"]'
activate_workspace_execution_without_memory "$TMP_DIR/schema-failure-activation.json"
schema_run_id="$(wait_activation_run "$TMP_DIR/schema-failure-activation.json" "$TMP_DIR/schema-failure-activation-run.json")"
wait_run_state_any "$schema_run_id" "$TMP_DIR/schema-failure-status.json" "failed" "superseded" "blocked_on_decision" "completed"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$schema_run_id/snapshot" >"$TMP_DIR/schema-failure-snapshot.json"
# The non-JSON response never opens a candidate attempt; bounded whole-document
# recovery eventually surfaces document-recovery exhaustion or a failed status.
grep -EEi 'candidate_document_recovery_exhausted|semantic_document_recovery_exhausted|document.recovery|not-json|failed' "$TMP_DIR/schema-failure-snapshot.json" "$TMP_DIR/schema-failure-status.json" >/dev/null
schema_run_state="$(json_path "$TMP_DIR/schema-failure-status.json" "status.state")"
if [[ "$schema_run_state" == "blocked_on_decision" ]]; then
  schema_ticket_id="$(json_path "$TMP_DIR/schema-failure-status.json" "status.blockingTicketId")"
  schema_ticket_path="$(url_encode "$schema_ticket_id")"
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$schema_ticket_path?workspaceId=$workspace_id" >"$TMP_DIR/schema-failure-decision.json"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$schema_ticket_path/resolve?workspaceId=$workspace_id" -H 'Content-Type: application/json' \
    -d '{"resolutionType":"reject_current_path","resolutionPayload":{"operator":"llm-acceptance","recoveryAction":"terminate"}}' \
    >"$TMP_DIR/schema-failure-resolve.json" || true
  wait_run_state_any "$schema_run_id" "$TMP_DIR/schema-failure-terminated-status.json" "failed" "completed" "superseded"
  schema_run_state="$(json_path "$TMP_DIR/schema-failure-terminated-status.json" "status.state")"
fi
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$schema_run_id/observability" | json_grep_bool_value terminal true >/dev/null || true
# V2 does not auto-create replan successors; accept terminal failed/completed/superseded.
assert_run_recovery_read_models "$schema_run_id" "schema-failure" "$schema_run_state" ""

create_acceptance_workspace "memory" "llm acceptance memory workspace" "verify LLM memory recall" '["memory recall is visible"]'
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/memory" -H 'Content-Type: application/json' \
  -d '{"memoryId":"'"$MEMORY_ID"'","sourceType":"user","memoryType":"project_context","content":"Seed memory for acceptance execution.","summary":"Seeded acceptance memory is available to runs.","tags":["acceptance","seed"],"importance":8,"confidence":9,"visibility":"workspace","embeddingProviderId":"verify","embeddingModel":"verify-embedding","embeddingText":"seeded acceptance memory","vector":[0.1,0.2,0.3]}' >"$TMP_DIR/memory-recall-seed.json"
json_grep_string_value id "$MEMORY_ID" "$TMP_DIR/memory-recall-seed.json" >/dev/null
activate_workspace_execution_with_auto_memory "$TMP_DIR/memory-recall-activation.json"
memory_run_id="$(wait_activation_run "$TMP_DIR/memory-recall-activation.json" "$TMP_DIR/memory-recall-activation-run.json")"
wait_run_state "$memory_run_id" "completed" "$TMP_DIR/memory-recall-completed-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$memory_run_id/snapshot" >"$TMP_DIR/memory-recall-snapshot.json"
# V2 surfaces recall via memoryRecallTraces (not V1 memories/memoryRetrieval maps).
grep -E '"memoryRecallTraces"|"memories"|"memoryRetrieval"' "$TMP_DIR/memory-recall-snapshot.json" >/dev/null
grep -F "$MEMORY_ID" "$TMP_DIR/memory-recall-snapshot.json" >/dev/null || true
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$memory_run_id/observability" >"$TMP_DIR/memory-recall-observability.json"
json_grep_bool_value terminal true "$TMP_DIR/memory-recall-observability.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$memory_run_id/context-bundle" >"$TMP_DIR/memory-recall-context-bundle.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$memory_run_id/memory-recalls" >"$TMP_DIR/memory-recall-run-api.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/memory-recalls" >"$TMP_DIR/memory-recall-workspace-api.json"
curl -fsS "$BASE_URL/api/v1/workorch/events?workspaceId=$workspace_id&runId=$memory_run_id" >"$TMP_DIR/memory-recall-events.json"
python3 - "$TMP_DIR/memory-recall-snapshot.json" \
  "$TMP_DIR/memory-recall-observability.json" \
  "$TMP_DIR/memory-recall-context-bundle.json" \
  "$TMP_DIR/memory-recall-run-api.json" \
  "$TMP_DIR/memory-recall-workspace-api.json" \
  "$TMP_DIR/memory-recall-events.json" \
  "$MEMORY_ID" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8"))["snapshot"]
observability = json.load(open(sys.argv[2], encoding="utf-8"))["observability"]
bundle = json.load(open(sys.argv[3], encoding="utf-8"))["contextBundle"]
run_api = json.load(open(sys.argv[4], encoding="utf-8")).get("memoryRecallTraces") or []
workspace_api = json.load(open(sys.argv[5], encoding="utf-8")).get("memoryRecallTraces") or []
events = json.load(open(sys.argv[6], encoding="utf-8")).get("eventRecords") or []
memory_id = sys.argv[7]

errors = []
snapshot_traces = snapshot.get("memoryRecallTraces") or []
observability_traces = observability.get("memoryRecallTraces") or []
if not snapshot_traces:
    errors.append("snapshot missing memoryRecallTraces")
if not observability_traces:
    errors.append("observability missing memoryRecallTraces")
if not run_api:
    errors.append("run memory-recalls API returned no traces")
if not workspace_api:
    errors.append("workspace memory-recalls API returned no traces")

def trace_ids(items):
    return {item.get("id") for item in items if item.get("id")}

run_trace_ids = trace_ids(run_api)
for label, items in [
    ("snapshot", snapshot_traces),
    ("observability", observability_traces),
    ("workspace_api", workspace_api),
]:
    missing = run_trace_ids - trace_ids(items)
    if missing:
        errors.append(f"{label} missing trace ids from run API: {sorted(missing)}")

sources = {item.get("source") for item in run_api}
if run_api and not sources:
    errors.append(f"run memory-recalls API missing sources: {sources}")

if run_api and not any(memory_id in (item.get("retrievedMemoryIds") or []) for item in run_api):
    blob = json.dumps(run_api)
    if memory_id not in blob:
        errors.append(f"run memory-recalls API missing recalled memory {memory_id}")

section = next((item for item in (bundle.get("sections") or []) if item.get("id") == "memory_recall"), None)
if section:
    section_refs = set(section.get("evidenceRefs") or [])
    section_traces = ((section.get("data") or {}).get("memoryRecallTraces") or [])
    if section_refs and memory_id not in section_refs and memory_id not in json.dumps(section_traces):
        errors.append(f"context bundle memory_recall section missing memory id {memory_id}: {section_refs}")

timeline_sources = {item.get("source") for item in (observability.get("timeline") or [])}
event_topics = {item.get("topic") for item in events}
if run_api and "/memory/recall/recorded" not in event_topics and "memory_recall" not in timeline_sources:
    if not snapshot_traces and not observability_traces:
        errors.append("memory recall traces present in API but not in snapshot/observability")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY
assert_run_recovery_read_models "$memory_run_id" "memory-recall" "completed" ""

create_acceptance_workspace "stream" "llm acceptance stream workspace" "verify LLM streaming usage" '["stream usage is recorded"]'
activate_workspace_execution_without_memory "$TMP_DIR/stream-baseline-activation.json"
stream_baseline_run_id="$(wait_activation_run "$TMP_DIR/stream-baseline-activation.json" "$TMP_DIR/stream-baseline-activation-run.json")"
wait_run_state "$stream_baseline_run_id" "completed" "$TMP_DIR/stream-baseline-completed-status.json"
assert_run_recovery_read_models "$stream_baseline_run_id" "stream-baseline" "completed" ""

activate_workspace_execution_with_provider "$TMP_DIR/stream-native-activation.json" "ai-proxy" "verify-stream-native-model"
stream_native_run_id="$(wait_activation_run "$TMP_DIR/stream-native-activation.json" "$TMP_DIR/stream-native-activation-run.json")"
wait_run_state "$stream_native_run_id" "completed" "$TMP_DIR/stream-native-completed-status.json"
assert_run_recovery_read_models "$stream_native_run_id" "stream-native" "completed" ""
# Runtime candidate dispatch uses the target's sealed streaming capability.
# Assert the canonical bounded mode and exact submitted/two-level identity
# facts rather than the compatibility-only streaming boolean.
assert_run_llm_usage_semantics "$stream_native_run_id" "provider_native" "streaming"

activate_workspace_execution_with_provider "$TMP_DIR/stream-estimated-activation.json" "ai-proxy" "verify-stream-estimated-model"
stream_estimated_run_id="$(wait_activation_run "$TMP_DIR/stream-estimated-activation.json" "$TMP_DIR/stream-estimated-activation-run.json")"
wait_run_state "$stream_estimated_run_id" "completed" "$TMP_DIR/stream-estimated-completed-status.json"
# ai-proxy uses one Responses transport for every catalog model. This mock's
# estimated model deliberately omits usage from the completed event, which
# must leave an auditable estimated fallback rather than inventing native use.
assert_run_recovery_read_models "$stream_estimated_run_id" "stream-estimated" "completed" "estimated"
assert_run_llm_usage_semantics "$stream_estimated_run_id" "estimated" "streaming"

assert_workspace_llm_usage_breakdown "$workspace_id"
assert_workspace_recovery_read_models "final"
curl -fsS "$BASE_URL/metrics" >"$TMP_DIR/metrics.txt"
grep 'workorch_http_requests_total{endpoint="configuration_update"}' "$TMP_DIR/metrics.txt" >/dev/null
grep 'workorch_http_requests_total{endpoint="workspace_execution_activation"}' "$TMP_DIR/metrics.txt" >/dev/null
grep 'workorch_http_requests_total{endpoint="run_observability"}' "$TMP_DIR/metrics.txt" >/dev/null
# Approval HTTP endpoints may be unused under Runtime decision-only governance.
grep -E 'workorch_http_requests_total\{endpoint="(decision_resolve|approval_approve|approval_reject)"\}' "$TMP_DIR/metrics.txt" >/dev/null || true

echo "workorch LLM workspace execution verification passed for deployed workorch service $BASE_URL"
