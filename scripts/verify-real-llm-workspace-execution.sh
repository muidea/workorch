#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_REAL_LLM_BASE_URL:-${WORKORCH_BASE_URL:-}}"
source "$ROOT_DIR/scripts/lib/verify-isolated-runtime.sh"
workorch_verify_require_isolated_runtime "$BASE_URL" "verify-real-llm-workspace-execution"
PROVIDER_ID="${WORKORCH_REAL_LLM_PROVIDER_ID:-}"
TMP_DIR="${WORKORCH_REAL_LLM_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_REAL_LLM_KEEP_ARTIFACTS:-1}"
RUN_WAIT_SECONDS="${WORKORCH_REAL_LLM_RUN_WAIT_SECONDS:-900}"
ACTIVATION_RUN_WAIT_SECONDS="${WORKORCH_REAL_LLM_ACTIVATION_RUN_WAIT_SECONDS:-60}"

cleanup() {
  local status=$?
  if [[ "$KEEP_ARTIFACTS" != "1" && $status -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  fi
}

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "real LLM workspace execution verification failed" >&2
  echo "artifacts=$TMP_DIR" >&2
  for file in "$TMP_DIR"/*.json; do
    [[ -f "$file" ]] || continue
    echo "--- $file ---" >&2
    python3 -m json.tool "$file" >&2 || cat "$file" >&2
  done
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

post_json() {
  local url="$1"
  local body="$2"
  curl -fsS -H "Content-Type: application/json" -X POST "$url" -d "$body"
}

capture_runs_before_activation() {
  local output_file="$1"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" >"$output_file"
}

validate_activation_command_response() {
  local response_file="$1"
  local expected_request_id="$2"
  local expected_reason="$3"
  python3 - "$response_file" "$expected_request_id" "$expected_reason" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
expected_request_id = sys.argv[2]
expected_reason = sys.argv[3]
errors = []
if data.get("accepted") is not True:
    errors.append(f"accepted is {data.get('accepted')!r}, want true")
if data.get("intent") != "restart":
    errors.append(f"intent is {data.get('intent')!r}, want restart")
if data.get("requestId") != expected_request_id:
    errors.append(f"requestId is {data.get('requestId')!r}, want {expected_request_id!r}")
if data.get("reason") != expected_reason:
    errors.append(f"reason is {data.get('reason')!r}, want {expected_reason!r}")
if errors:
    raise SystemExit("\n".join(errors))
PY
}

wait_activation_run() {
  local before_runs_file="$1"
  local runs_file="$2"
  local output_file="$3"
  local run_id=""
  local deadline=$((SECONDS + ACTIVATION_RUN_WAIT_SECONDS))
  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" >"$runs_file"; then
      run_id="$(python3 - "$runs_file" "$before_runs_file" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
before_data = json.load(open(sys.argv[2], encoding="utf-8"))
before = {str(item.get("id") or "") for item in before_data.get("runs", []) if item.get("id")}
new_runs = [
    item for item in data.get("runs", [])
    if item.get("id") and str(item.get("id")) not in before
]
if not new_runs:
    raise SystemExit(1)
print(new_runs[-1]["id"])
PY
)" && {
        python3 - "$runs_file" "$run_id" "$output_file" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
run_id = sys.argv[2]
run = next((item for item in data.get("runs", []) if item.get("id") == run_id), {"id": run_id})
with open(sys.argv[3], "w", encoding="utf-8") as handle:
    json.dump({"run": run}, handle, ensure_ascii=False, indent=2)
    handle.write("\n")
PY
        echo "$run_id"
        return 0
      }
    fi
    sleep 1
  done
  echo "activation accepted but run did not materialize within ${ACTIVATION_RUN_WAIT_SECONDS}s for workspace $workspace_id" >&2
  return 1
}

wait_run_state() {
  local run_id="$1"
  local expected_state="$2"
  local output_file="$3"
  local deadline=$((SECONDS + RUN_WAIT_SECONDS))
  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status?waitState=$expected_state&waitTimeoutMillis=1000&waitIntervalMillis=100" >"$output_file"
    if grep '"state": "'"$expected_state"'"' "$output_file" >/dev/null; then
      return 0
    fi
    sleep 1
  done
  cat "$output_file" >&2
  return 1
}

trap report_failure ERR
trap cleanup EXIT

mkdir -p "$TMP_DIR"

curl -fsS "$BASE_URL/readyz" >"$TMP_DIR/readyz.json"
python3 - "$TMP_DIR/readyz.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
if data.get("status") != "ready":
    raise SystemExit(f"readyz status is {data.get('status')!r}, want ready")
PY

curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
PROVIDER_ID="$(
  python3 - "$TMP_DIR/configuration.json" "$PROVIDER_ID" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
requested = sys.argv[2].strip()
llm = data.get("configuration", {}).get("llm", {})
default_chat = llm.get("defaultChat") or {}
provider_id = requested or default_chat.get("providerId") or default_chat.get("providerID") or ""
model_id = default_chat.get("modelId") or default_chat.get("modelID") or ""
if not provider_id:
    raise SystemExit("no defaultChat provider configured")
if provider_id == "ai-proxy":
    ai_proxy = llm.get("aiProxy") or {}
    if not ai_proxy.get("baseUrl") and not ai_proxy.get("baseURL"):
        raise SystemExit("ai-proxy baseURL is not configured")
    # API key may be optional depending on deployment; catalog readiness is checked at activation.
    print(provider_id)
else:
    customs = {item.get("id"): item for item in (llm.get("customProviders") or [])}
    provider = customs.get(provider_id)
    if not provider:
        raise SystemExit(f"custom provider {provider_id!r} is not configured")
    if not provider.get("apiKeyConfigured"):
        raise SystemExit(f"custom provider {provider_id!r} has no configured API key")
    models = {item.get("id"): item for item in (provider.get("models") or [])}
    if model_id and model_id not in models and models:
        # tolerate activation-time explicit model selection
        pass
    print(provider_id)
PY
)"

post_json "$BASE_URL/api/v1/workorch/workspaces" \
  '{"title":"real llm graph verification","description":"Verify real provider executionGraph, shell block, and LLM DoD assessment closure.","templateId":"generic-governed"}' \
  >"$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]

post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" \
  '{"text":"无需人工决策、无需审批。第一轮 LLM 必须返回 executionGraph，包含一个 shell/command 节点执行 pwd。不要返回 requiredDecision 或 requiredApproval。图节点执行完成后的后续 LLM 步骤必须基于 previousStepOutput 返回 completionState completed 和 dodAssessment，evidenceRefs 使用 previousStepId，dodAssessment.summary 必须用自然语言总结最终结果。"}' \
  >"$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
[[ -n "$goal_id" ]]

post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" \
  '{"items":["第一轮 LLM 输出 executionGraph","shell 图节点由 Runtime 执行成功","后续 LLM 步骤返回 completed 的 dodAssessment，包含 summary 和 evidenceRefs"]}' \
  >"$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"
[[ -n "$dod_id" ]]

activation_request_id="real-llm-e2e-$$"
activation_reason="e2e_restart"
capture_runs_before_activation "$TMP_DIR/activation-before-runs.json"
post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/llm-target" \
  '{"providerId":"'"$PROVIDER_ID"'","modelId":"verify-model"}' \
  "$TMP_DIR/llm-target.json"
post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  '{"intent":"restart","requestId":"'"$activation_request_id"'","reason":"'"$activation_reason"'","source":"e2e"}' \
  >"$TMP_DIR/activation.json"
validate_activation_command_response "$TMP_DIR/activation.json" "$activation_request_id" "$activation_reason"
run_id="$(wait_activation_run "$TMP_DIR/activation-before-runs.json" "$TMP_DIR/activation-runs.json" "$TMP_DIR/activation-run.json")"
[[ -n "$run_id" ]]

wait_run_state "$run_id" "completed" "$TMP_DIR/run-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/snapshot" >"$TMP_DIR/snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/observability" >"$TMP_DIR/observability.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/task-progress" >"$TMP_DIR/task-progress.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/context-bundle" >"$TMP_DIR/context-bundle.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/task-decompositions" >"$TMP_DIR/task-decompositions.json"
curl -fsS "$BASE_URL/api/v1/workorch/events?workspaceId=$workspace_id&runId=$run_id" >"$TMP_DIR/events.json"

python3 - "$TMP_DIR/activation.json" "$TMP_DIR/snapshot.json" "$TMP_DIR/observability.json" "$TMP_DIR/task-progress.json" "$TMP_DIR/context-bundle.json" "$TMP_DIR/task-decompositions.json" "$TMP_DIR/events.json" <<'PY'
import json
import sys

activation = json.load(open(sys.argv[1], encoding="utf-8"))["activation"]
snapshot = json.load(open(sys.argv[2], encoding="utf-8"))["snapshot"]
observability = json.load(open(sys.argv[3], encoding="utf-8"))["observability"]
task_progress = json.load(open(sys.argv[4], encoding="utf-8"))["taskProgress"]
context_bundle = json.load(open(sys.argv[5], encoding="utf-8"))["contextBundle"]
task_decompositions = json.load(open(sys.argv[6], encoding="utf-8")).get("taskDecompositions") or []
event_records = json.load(open(sys.argv[7], encoding="utf-8")).get("eventRecords") or []
run = snapshot["run"]
steps = snapshot.get("steps") or ([] if not snapshot.get("step") else [snapshot["step"]])
tool_calls = snapshot.get("toolCalls") or []
validations = snapshot.get("validationResults") or []
checkpoints = snapshot.get("runtimeCheckpoints") or []
results = snapshot.get("results") or []
artifacts = snapshot.get("artifacts") or []

errors = []
if run.get("state") != "completed":
    errors.append(f"run state is {run.get('state')!r}, want completed: {run.get('failureReason', '')}")
routing = (run.get("executionContext") or {}).get("soulRouting") or {}
bindings = routing.get("bindings") or {}
execution_binding = bindings.get("execution") or {}
if routing.get("status") != "resolved":
    errors.append(f"soulRouting.status is {routing.get('status')!r}, want resolved")
if execution_binding.get("soulId") != "soul-generic-governed-execution":
    errors.append(f"execution soul binding is {execution_binding!r}, want soul-generic-governed-execution")

compressed = (run.get("executionContext") or {}).get("compressedContext") or {}
if compressed.get("version") != "workorch.compressedContext.v1":
    errors.append(f"executionContext.compressedContext missing or invalid: {compressed}")
if compressed.get("mode") != "deterministic_recovery_summary":
    errors.append(f"compressedContext mode is {compressed.get('mode')!r}")
if compressed.get("runId") != run.get("id") or compressed.get("workspaceId") != run.get("workspaceId"):
    errors.append(f"compressedContext identity mismatch: {compressed}")

graph_steps = [s for s in steps if (s.get("output") or {}).get("stage") == "llm_execution_graph_plan"]
if not graph_steps:
    errors.append("missing llm_execution_graph_plan step")
elif not (graph_steps[0].get("output") or {}).get("executionGraph", {}).get("nodes"):
    errors.append("executionGraph has no nodes")

shell_steps = [s for s in steps if s.get("kind") == "shell/command"]
if not shell_steps:
    errors.append("missing shell/command graph node step")
elif shell_steps[0].get("state") != "completed" or (shell_steps[0].get("output") or {}).get("status") != "completed":
    errors.append(f"shell step did not complete: {shell_steps[0]}")

assessment_steps = [s for s in steps if (s.get("output") or {}).get("stage") == "llm_dod_assessment"]
if not assessment_steps:
    errors.append("missing llm_dod_assessment follow-up step")
else:
    assessment = (assessment_steps[-1].get("output") or {}).get("dodAssessment") or {}
    if assessment.get("satisfied") is not True:
        errors.append(f"dodAssessment.satisfied is {assessment.get('satisfied')!r}, want true")
    if not str(assessment.get("summary") or "").strip():
        errors.append(f"dodAssessment.summary is empty: {assessment!r}")
    for item in assessment.get("items") or []:
        if not item.get("evidenceRefs"):
            errors.append(f"dodAssessment item has no evidenceRefs: {item}")

progress = observability.get("progress") or {}
if progress.get("terminal") is not True:
    errors.append(f"observability terminal is {progress.get('terminal')!r}, want true")

if task_progress.get("runId") != run.get("id") or task_progress.get("state") != "completed":
    errors.append(f"taskProgress mismatch: {task_progress}")
if not (task_progress.get("recovery") or {}).get("mode"):
    errors.append(f"taskProgress recovery mode empty: {task_progress}")

bundle_compressed = context_bundle.get("compressedContext") or {}
if context_bundle.get("runId") != run.get("id") or context_bundle.get("workspaceId") != run.get("workspaceId"):
    errors.append(f"context bundle identity mismatch: {context_bundle.get('workspaceId')}/{context_bundle.get('runId')}")
if bundle_compressed.get("version") != "workorch.compressedContext.v1":
    errors.append(f"context bundle compressedContext invalid: {bundle_compressed}")
section_ids = {item.get("id") for item in (context_bundle.get("sections") or [])}
for required in ["workspace", "run", "task_progress", "task_decomposition", "formal_outputs", "recovery"]:
    if required not in section_ids:
        errors.append(f"context bundle missing section {required!r}: {section_ids}")

if not task_decompositions:
    errors.append("missing taskDecompositions for executionGraph run")
else:
    latest = task_decompositions[-1]
    if latest.get("state") != "completed":
        errors.append(f"taskDecomposition state is {latest.get('state')!r}, want completed: {latest}")
    if latest.get("nodeTotal", 0) < 1 or latest.get("nodeCompleted") != latest.get("nodeTotal"):
        errors.append(f"taskDecomposition node completion mismatch: {latest}")
    nodes = latest.get("nodes") or []
    if not nodes or not any(item.get("kind") == "shell/command" and item.get("state") == "completed" for item in nodes):
        errors.append(f"taskDecomposition missing completed shell node: {nodes}")

if not tool_calls:
    errors.append("missing ToolCall evidence")
if not validations:
    errors.append("missing ValidationResult evidence")
if not checkpoints:
    errors.append("missing RuntimeCheckpoint evidence")
accepted_results = [item for item in results if item.get("state") == "accepted"]
if not accepted_results:
    errors.append("missing accepted Result final outcome")
else:
    result = accepted_results[-1]
    if not str(result.get("summary") or "").strip():
        errors.append(f"Result summary is empty: {result}")
    artifact_refs = set(result.get("artifactRefs") or [])
    linked_artifacts = [item for item in artifacts if item.get("resultId") == result.get("id")]
    if not linked_artifacts:
        errors.append(f"missing Artifact linked to accepted Result {result.get('id')}")
    elif not any(item.get("id") in artifact_refs and item.get("artifactType") == "text" for item in linked_artifacts):
        errors.append(f"missing text Artifact referenced by Result: result={result} artifacts={linked_artifacts}")

topics = {item.get("topic") for item in event_records}
for topic in ["/run/created", "/run/state/changed", "/step/dispatched", "/step/state/changed", "/result/accepted", "/artifact/created"]:
    if topic not in topics:
        errors.append(f"event stream missing {topic}: {topics}")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)

print(f"workspaceId={activation['workspace']['id']}")
print(f"runId={run['id']}")
print(f"runState={run['state']}")
print(f"steps={len(steps)}")
print(f"toolCalls={len(tool_calls)}")
print(f"validations={len(validations)}")
print(f"runtimeCheckpoints={len(checkpoints)}")
print(f"results={len(results)}")
print(f"artifacts={len(artifacts)}")
PY

echo "real LLM workspace execution verification passed for provider=$PROVIDER_ID"
echo "artifacts=$TMP_DIR"
