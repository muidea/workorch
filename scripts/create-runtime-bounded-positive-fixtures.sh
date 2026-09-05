#!/usr/bin/env bash
# Create two fresh, bounded Runtime terminal-gate fixtures. The script is
# intentionally separate from the observer: it owns only the workspaces and
# deliverable directories it creates, while the live verifier owns activation
# and terminal assertions. Both fixtures exercise the same neutral
# state-change -> validation -> assessment path; Git-specific sequencing stays
# covered by the file-backed owner matrix rather than becoming a second live
# success contract.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BASE_URL="${WORKORCH_BASE_URL:-http://127.0.0.1:8080}"
# An explicit template may be supplied by a deployment, but the default is a
# fresh, script-owned minimal template that seals the required durable state
# change without inheriting the large software-dev profile surface.
TEMPLATE_ID="${WORKORCH_E2E_BOUNDED_TEMPLATE_ID:-}"
RUN_TERMINAL="${WORKORCH_E2E_RUN_TERMINAL:-0}"
CLEANUP_FIXTURES="${WORKORCH_E2E_FIXTURE_CLEANUP:-0}"
PROVIDER_ID="${WORKORCH_E2E_PROVIDER_ID:-}"
MODEL_ID="${WORKORCH_E2E_MODEL_ID:-}"
ARTIFACT_DIR="$(realpath -m "${WORKORCH_E2E_ARTIFACT_DIR:-${TMPDIR:-/tmp}/workorch-e2e/runtime-bounded-fixtures/$(date -u +%Y%m%dT%H%M%SZ)}")"
REPOSITORY_E2E_ARTIFACT_DIR="$(realpath -m "$ROOT_DIR/.workorch-e2e")"
if [[ "$ARTIFACT_DIR" == "$REPOSITORY_E2E_ARTIFACT_DIR" || "$ARTIFACT_DIR" == "$REPOSITORY_E2E_ARTIFACT_DIR/"* ]]; then
  echo "WORKORCH_E2E_ARTIFACT_DIR must not point to the repository .workorch-e2e directory" >&2
  exit 2
fi
mkdir -p "$ARTIFACT_DIR"

created_workspace_ids=()
created_template_ids=()
cleanup() {
  local status=$?
  if [[ "$CLEANUP_FIXTURES" == "1" ]]; then
    local workspace_id
    for workspace_id in "${created_workspace_ids[@]}"; do
      curl -fsS -X DELETE "$BASE_URL/api/v1/workorch/workspaces/$workspace_id?purge=true" >/dev/null || true
    done
    local template_id
    for template_id in "${created_template_ids[@]}"; do
      curl -fsS -X DELETE "$BASE_URL/api/v1/workorch/workspace-templates/$template_id" >/dev/null || true
    done
  fi
  exit "$status"
}
trap cleanup EXIT

curl -fsS --connect-timeout 5 --max-time 15 "$BASE_URL/healthz" >"$ARTIFACT_DIR/healthz.json"
curl -fsS --connect-timeout 5 --max-time 15 "$BASE_URL/readyz" >"$ARTIFACT_DIR/readyz.json"

# Select exactly the target that terminal activation will seal, before creating
# any server-side fixture state.  A bad inherited system default is a deployment
# configuration error, not a Runtime terminal-gate failure.
python3 - "$BASE_URL" "$ARTIFACT_DIR" "$PROVIDER_ID" "$MODEL_ID" <<'PY'
import json
import os
import sys
import urllib.request

base, artifact_dir, requested_provider, requested_model = sys.argv[1:]
if bool(requested_provider) != bool(requested_model):
    raise SystemExit("WORKORCH_E2E_PROVIDER_ID and WORKORCH_E2E_MODEL_ID must be set together")

req = urllib.request.Request(base.rstrip("/") + "/api/v1/workorch/configuration", headers={"Accept": "application/json"})
with urllib.request.urlopen(req, timeout=45) as response:
    configuration = json.load(response)
llm = (configuration.get("configuration") or {}).get("llm") or {}
target = {"providerId": requested_provider, "modelId": requested_model} if requested_provider else (llm.get("defaultChat") or {})
provider_id = (target.get("providerId") or "").strip()
model_id = (target.get("modelId") or "").strip()
if not provider_id or not model_id:
    raise SystemExit("deployed configuration has no usable defaultChat target; set WORKORCH_E2E_PROVIDER_ID and WORKORCH_E2E_MODEL_ID explicitly")
if provider_id != "ai-proxy":
    raise SystemExit(f"selected terminal target provider must be ai-proxy, got {provider_id!r}")

catalog = llm.get("catalog") or {}
ready = False
source = "ai_proxy_catalog"
if (catalog.get("providerId") or "").strip() != provider_id:
    raise SystemExit(f"ai-proxy catalog owner mismatch: {catalog.get('providerId')!r}")
if catalog.get("state") != "fresh" or catalog.get("stale"):
    raise SystemExit(f"provider catalog for {provider_id!r} is not fresh: state={catalog.get('state')!r} stale={catalog.get('stale')!r}")
for model in catalog.get("models") or []:
    if model.get("providerId") == provider_id and model.get("modelId") == model_id:
        ready = bool(model.get("capabilityReady")) and "responses" in (model.get("operations") or [])
        break
if not ready:
    raise SystemExit(f"selected terminal target {provider_id}/{model_id} is not a ready Responses target ({source})")

payload = {"target": {"providerId": provider_id, "modelId": model_id}, "source": source, "catalog": {"providerId": catalog.get("providerId"), "state": catalog.get("state"), "revision": catalog.get("revision"), "stale": catalog.get("stale")}}
with open(os.path.join(artifact_dir, "provider-target-preflight.json"), "w", encoding="utf-8") as handle:
    json.dump(payload, handle, ensure_ascii=False, indent=2)
    handle.write("\n")
print(json.dumps(payload, ensure_ascii=False))
PY

python3 - "$BASE_URL" "$ARTIFACT_DIR" "$TEMPLATE_ID" <<'PY'
import json
import os
import sys
import urllib.request

base, artifact_dir, template_id = sys.argv[1:]

def request(method, path, body=None):
    data = None
    headers = {"Accept": "application/json"}
    if body is not None:
        data = json.dumps(body, ensure_ascii=False).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(base.rstrip("/") + path, data=data, headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=45) as response:
        return response.status, json.load(response)

created_template_ids = []
if template_id:
    template_ids = {"a": template_id, "b": template_id}
else:
    # New Runtime activations seal an explicit acceptance-slice policy.  Keep
    # every generated work item isolated so this fixture exercises exactly two
    # bounded focuses instead of relying on an implicit compiler default.
    acceptance_slice_policy = {
        "schemaVersion": "workorch.acceptance_slice_policy",
        "maxWorkItems": 1,
        "maxFocuses": 2,
        "maxCandidateContextItems": 4,
        "completionPolicy": "all_required_items_accepted",
        "resultPolicy": "slice_result_allowed",
        "groups": [{
            "id": "bounded-all",
            "title": "Bounded fixture acceptance",
            "scope": "bounded_fixture",
            "priority": 10,
            "maxWorkItems": 1,
            "match": {"all": True},
        }],
    }
    suffix = os.path.basename(artifact_dir).lower().replace("_", "-")
    template_ids = {
        "a": "runtime-bounded-general-a-" + suffix,
        "b": "runtime-bounded-general-b-" + suffix,
    }
    def bounded_template(label, name):
        profile_id = "runtime-bounded-native-proof"
        return {
            "id": template_ids[label],
            "name": name,
            "description": "Script-owned two-focus fixture with File preparation and owner-native Go validation.",
            "goalText": "Produce and validate a bounded, evidence-backed workspace result.",
            "doDItems": ["A bounded state change is produced", "The state change is validated"],
            "contextHints": {
                "stateChangePolicy": "must_produce_state_change",
                "acceptanceSlicePolicy": acceptance_slice_policy,
                "profileContracts": {
                    profile_id: {
                        "evidenceRequirements": [
                            {
                                "id": "dod-001",
                                "scope": "dodItem",
                                "requiredEvidenceKinds": ["state_change"],
                                "requireDirectEvidence": True,
                                "acceptedStepKinds": ["file/edit"],
                                "targetRole": "bounded_fixture_subject",
                                "targetRefs": ["runtime-validation-source.txt"],
                                "requireTargetGrounding": True,
                                "description": "The first CWI requires one binding-matched durable File effect.",
                            },
                            {
                                "id": "dod-002",
                                "scope": "dodItem",
                                "requiredEvidenceKinds": ["validation"],
                                "requireDirectEvidence": True,
                                "acceptedStepKinds": ["validation/managed_session", "file/write"],
                                "rejectedStepKinds": ["file/read"],
                                "targetRole": "bounded_fixture_subject",
                                "targetDiscoverySources": ["workspace_manifest", "project_files"],
                                "targetSelectorRef": "file/workspace_deliverable",
                                "targetSelectorOwnerRef": profile_id,
                                "targetSelectorSubjectRef": "dodItem",
                                "prospectiveTargetAllOfConstraintRefs": ["workspace-relative-path", "new-target-requires-owner-selector"],
                                "requireTargetGrounding": True,
                                "requiredOwnerFacts": [{
                                    "ownerModuleId": "blocks/shell",
                                    "factKind": "selectable_validation_surface",
                                    "operationKind": "validation/managed_session",
                                }],
                                "ownerFactConvergences": [{
                                    "ownerModuleId": "blocks/shell",
                                    "factKind": "selectable_validation_surface",
                                    "operationKind": "validation/managed_session",
                                    "missingTrigger": "owner_fact_missing",
                                    "acquisitionOwnerModuleId": "blocks/file",
                                    "acquisitionOperationKind": "file/write",
                                    "acquisitionTargetPolicy": "prospective_owner_target",
                                    "acquisitionTargetSelectorRef": "workspace-relative-new-file-target",
                                    "prospectiveSatisfactionContractRef": "shell/selectable-validation-surface/prospective-workspace-manifest/v1",
                                    "targetSelectorRef": "file/workspace_deliverable",
                                    "targetSelectorOwnerRef": profile_id,
                                    "targetSelectorSubjectRef": "dodItem",
                                    "reobserveOwnerModuleId": "blocks/shell",
                                    "reobserveFactKind": "selectable_validation_surface",
                                    "maxAcquisitionAttempts": 1,
                                }],
                                "description": "The second CWI closes only after its exact Shell-attested native Go test passes.",
                            },
                        ],
                    },
                },
            },
            "evidencePolicies": [{
                "id": "bounded-native-go-validation",
                "appliesTo": "shell/command",
                "commandName": "go",
                "recipeDeclared": True,
                "recipeArgs": ["test", "-v", "./..."],
                "recipeTimeoutMillis": 10000,
            }],
        }

    templates = {
        "a": bounded_template("a", "Runtime bounded general terminal fixture A"),
        "b": bounded_template("b", "Runtime bounded general terminal fixture B"),
    }
    for label, template in templates.items():
        _, template_response = request("POST", "/api/v1/workorch/workspace-templates", template)
        if (template_response.get("workspaceTemplate") or {}).get("id") != template["id"]:
            raise SystemExit(f"bounded fixture template {label} creation failed: {template_response}")
        created_template_ids.append(template["id"])

fixtures = []
for label, goal, dod_items in (
    (
        "a",
        "Produce one concise, traceable workspace result and independently verify it.",
        [
            "One bounded workspace result is produced with durable evidence.",
            "An independent validation and assessment confirm the result.",
        ],
    ),
    (
        "b",
        "Produce one concise, traceable workspace result and independently verify it.",
        [
            "One bounded workspace result is produced with durable evidence.",
            "An independent validation and assessment confirm the result.",
        ],
    ),
):
    deliverable = os.path.join(artifact_dir, "deliverables", label)
    os.makedirs(deliverable, exist_ok=False)
    # The deterministic Runtime provider fixture uses this owner-grounded
    # existing file. It is intentionally created before activation so target
    # selection is an owner fact, never an inferred prompt-only path.
    with open(os.path.join(deliverable, "runtime-validation-source.txt"), "w", encoding="utf-8") as handle:
        handle.write("Runtime bounded terminal fixture input.\n")
    _, workspace_response = request("POST", "/api/v1/workorch/workspaces", {
        "title": f"Runtime bounded positive fixture {label}",
        "description": "Fresh dual-focus fixture for the Runtime terminal acceptance gate.",
        "deliverableDirectory": deliverable,
        "templateId": template_ids[label],
    })
    workspace = workspace_response.get("workspace") or {}
    workspace_id = workspace.get("id")
    if not workspace_id:
        raise SystemExit(f"workspace creation returned no id: {workspace_response}")
    _, goal_response = request("POST", f"/api/v1/workorch/workspaces/{workspace_id}/goals", {"text": goal})
    _, dod_response = request("POST", f"/api/v1/workorch/workspaces/{workspace_id}/dods", {"items": dod_items})
    goal_id = (goal_response.get("goal") or {}).get("id")
    dod_id = (dod_response.get("dod") or dod_response.get("doD") or {}).get("id")
    if not goal_id or not dod_id:
        raise SystemExit(f"fixture definition creation failed: goal={goal_response} dod={dod_response}")
    fixtures.append({
        "label": label,
        "workspaceId": workspace_id,
        "goalId": goal_id,
        "doDId": dod_id,
        "deliverableDirectory": deliverable,
        "templateId": template_ids[label],
        "expectedFocusCount": 2,
        "maxCoverageItemsPerFocus": 4,
    })

payload = {
    "baseUrl": base,
    "templateIds": template_ids,
    "createdTemplateIds": created_template_ids,
    "fixtures": fixtures,
    "terminalGate": {
        "expectedFocusCount": 2,
        "maxCoverageItemsPerFocus": 4,
        "timeoutSeconds": 180,
    },
}
with open(os.path.join(artifact_dir, "fixtures.json"), "w", encoding="utf-8") as handle:
    json.dump(payload, handle, ensure_ascii=False, indent=2)
    handle.write("\n")
print(json.dumps(payload, ensure_ascii=False))
PY

mapfile -t created_workspace_ids < <(python3 - "$ARTIFACT_DIR/fixtures.json" <<'PY'
import json
import sys

payload = json.load(open(sys.argv[1], encoding="utf-8"))
for fixture in payload["fixtures"]:
    print(fixture["workspaceId"])
PY
)
mapfile -t created_template_ids < <(python3 - "$ARTIFACT_DIR/fixtures.json" <<'PY'
import json
import sys

payload = json.load(open(sys.argv[1], encoding="utf-8"))
for template_id in payload.get("createdTemplateIds") or []:
    print(template_id)
PY
)

if [[ "${#created_workspace_ids[@]}" != "2" ]]; then
  echo "fixture creator did not produce two workspace IDs" >&2
  exit 1
fi

python3 - "$ARTIFACT_DIR/fixtures.json" >"$ARTIFACT_DIR/terminal-env.sh" <<'PY'
import json
import shlex
import sys

payload = json.load(open(sys.argv[1], encoding="utf-8"))
fixtures = payload["fixtures"]
print(f"export WORKORCH_E2E_WORKSPACE_ID_A={shlex.quote(fixtures[0]['workspaceId'])}")
print(f"export WORKORCH_E2E_WORKSPACE_ID_B={shlex.quote(fixtures[1]['workspaceId'])}")
print("export WORKORCH_E2E_MODE=bounded_terminal")
print("export WORKORCH_E2E_ACTIVATE=1")
PY

echo "created fresh bounded fixtures: ${created_workspace_ids[*]}"
echo "artifacts=$ARTIFACT_DIR"
echo "source $ARTIFACT_DIR/terminal-env.sh"
echo "WORKORCH_E2E_ARTIFACT_DIR=$ARTIFACT_DIR scripts/verify-runtime-positive-convergence-live.sh"

if [[ "$RUN_TERMINAL" == "1" ]]; then
  # shellcheck disable=SC1090
  source "$ARTIFACT_DIR/terminal-env.sh"
  export WORKORCH_E2E_ARTIFACT_DIR="$ARTIFACT_DIR/terminal-verification"
  "$ROOT_DIR/scripts/verify-runtime-positive-convergence-live.sh"
fi
