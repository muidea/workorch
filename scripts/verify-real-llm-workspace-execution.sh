#!/usr/bin/env bash

set -euo pipefail

BASE_URL="${WORKORCH_REAL_LLM_BASE_URL:-http://127.0.0.1:8080}"
PROVIDER_ID="${WORKORCH_REAL_LLM_PROVIDER_ID:-}"
TMP_DIR="${WORKORCH_REAL_LLM_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_REAL_LLM_KEEP_ARTIFACTS:-1}"

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
provider_id = requested or llm.get("defaultProviderId", "")
providers = {item.get("id"): item for item in llm.get("providers", [])}
provider = providers.get(provider_id)
if not provider:
    raise SystemExit(f"provider {provider_id!r} is not configured")
if not provider.get("ready"):
    raise SystemExit(f"provider {provider_id!r} is not ready: {provider.get('reason', '')}")
if not provider.get("apiKeyConfigured"):
    raise SystemExit(f"provider {provider_id!r} has no configured API key")
print(provider_id)
PY
)"

post_json "$BASE_URL/api/v1/workorch/workspaces" \
  '{"title":"real llm graph verification","description":"Verify real provider executionGraph, shell block, and LLM DoD assessment closure."}' \
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

post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"'"$PROVIDER_ID"'"}' \
  >"$TMP_DIR/activation.json"
run_id="$(json_path "$TMP_DIR/activation.json" "activation.run.id")"
[[ -n "$run_id" ]]

curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/snapshot" >"$TMP_DIR/snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/observability" >"$TMP_DIR/observability.json"

python3 - "$TMP_DIR/activation.json" "$TMP_DIR/snapshot.json" "$TMP_DIR/observability.json" <<'PY'
import json
import sys

activation = json.load(open(sys.argv[1], encoding="utf-8"))["activation"]
snapshot = json.load(open(sys.argv[2], encoding="utf-8"))["snapshot"]
observability = json.load(open(sys.argv[3], encoding="utf-8"))["observability"]
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
