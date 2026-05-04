#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
BASE_URL="${WORKORCH_LLM_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
MOCK_ADDR="${WORKORCH_LLM_VERIFY_MOCK_ADDR:-127.0.0.1:18181}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_LLM_VERIFY_SUFFIX:-$$}"
MCP_SERVER_ID="mcp-context-verify-$VERIFY_SUFFIX"
SKILL_ID="skill-llm-acceptance-$VERIFY_SUFFIX"
SOUL_ID="soul-llm-acceptance-$VERIFY_SUFFIX"
MEMORY_ID="memory-context-seed-$VERIFY_SUFFIX"
MOCK_PID=""
MOCK_LOG="$TMP_DIR/llm-mock.log"

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
  if [[ -f "$TMP_DIR/original-llm-update.json" ]]; then
    curl -fsS -H "Content-Type: application/json" -X PUT \
      "$BASE_URL/api/v1/workorch/configuration/llm" \
      -d @"$TMP_DIR/original-llm-update.json" >/dev/null 2>&1 || true
  fi
  if [[ -n "$MOCK_PID" ]]; then
    kill "$MOCK_PID" >/dev/null 2>&1 || true
    wait "$MOCK_PID" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR"
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

url_encode() {
  python3 - "$1" <<'PY'
import sys
from urllib.parse import quote

print(quote(sys.argv[1], safe=""))
PY
}

start_mock_provider() {
  cat >"$TMP_DIR/llm_mock_provider.py" <<'PY'
import json
import os
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

initial_runs = []

def decision_payload(request):
    run_id = request.get("runId", "")
    step_id = request.get("stepId", "")
    if run_id == "probe":
        return {"completionState": "failed", "thoughtSummary": "probe"}
    scenario = initial_runs.index(run_id) if run_id in initial_runs else -1
    if "llm-after-decision" in step_id and scenario == 3:
        return {
            "thoughtSummary": "Decision information was provided; approval is now required before completing.",
            "completionState": "blocked_on_approval",
            "requiredApproval": {
                "reason": "Approve the second governance gate after decision resolution.",
                "action": "complete multi-gate LLM path",
                "context": {"scenario": "decision_then_approval"}
            }
        }
    if "llm-after-decision" in step_id:
        return {
            "thoughtSummary": "Decision information provided; DoD is now satisfied.",
            "completionState": "completed",
            "memoryWrites": [{
                "memoryType": "decision",
                "summary": "Operator provided missing implementation direction.",
                "content": "Decision ticket was resolved through the LLM acceptance script.",
                "tags": ["decision", "acceptance"]
            }],
            "dodAssessment": {
                "satisfied": True,
                "summary": "Decision path completed.",
                "items": [{"item": "decision can be resolved", "state": "satisfied", "evidenceRefs": [step_id]}]
            }
        }
    if "llm-after-approval" in step_id and scenario == 3:
        return {
            "thoughtSummary": "Decision and approval gates were both completed; DoD is now satisfied.",
            "completionState": "completed",
            "dodAssessment": {
                "satisfied": True,
                "summary": "Multi-gate path completed after Decision and Approval.",
                "items": [{
                    "item": "multi gate decision then approval can complete",
                    "state": "satisfied",
                    "evidenceRefs": [step_id]
                }]
            }
        }
    if run_id not in initial_runs:
        initial_runs.append(run_id)
    scenario = initial_runs.index(run_id)
    if scenario == 0:
        return {
            "thoughtSummary": "Happy path completed with evidence.",
            "completionState": "completed",
            "memoryWrites": [{
                "memoryType": "execution_lesson",
                "summary": "LLM happy path acceptance produced reusable workspace memory.",
                "content": "The first LLM-driven run completed and wrote memory for future recall.",
                "tags": ["llm", "happy-path"]
            }],
            "dodAssessment": {
                "satisfied": True,
                "summary": "Happy path DoD satisfied.",
                "items": [{"item": "happy path completes", "state": "satisfied", "evidenceRefs": [step_id]}]
            }
        }
    if scenario == 1:
        return {
            "thoughtSummary": "Need operator direction before continuing.",
            "completionState": "blocked_on_decision",
            "requiredDecision": {
                "reason": "Need missing implementation detail.",
                "decisionKind": "provide_missing_info",
                "question": "What missing detail should the run use?",
                "context": {"scenario": "decision"}
            }
        }
    if scenario == 2:
        return {
            "thoughtSummary": "Risky action requires approval.",
            "completionState": "blocked_on_approval",
            "requiredApproval": {
                "reason": "Approval required before risky action.",
                "action": "continue risky test path",
                "context": {"scenario": "approval"}
            }
        }
    if scenario == 3:
        return {
            "thoughtSummary": "Need operator direction before approval gate can be evaluated.",
            "completionState": "blocked_on_decision",
            "requiredDecision": {
                "reason": "Need missing detail before the approval gate.",
                "decisionKind": "provide_missing_info",
                "question": "What implementation direction should be approved next?",
                "context": {"scenario": "decision_then_approval"}
            }
        }
    if scenario == 4:
        return "not-json"
    return {
        "thoughtSummary": "Memory recall path completed.",
        "completionState": "completed",
        "dodAssessment": {
            "satisfied": True,
            "summary": "Memory recall DoD satisfied.",
            "items": [{"item": "memory recall is visible", "state": "satisfied", "evidenceRefs": [step_id]}]
        }
    }

class Handler(BaseHTTPRequestHandler):
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
            response = {"data": [{"embedding": vector}]}
            encoded = json.dumps(response).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)
            return
        if self.path != "/v1/chat/completions":
            self.send_response(404)
            self.end_headers()
            return
        size = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(size)
        payload = json.loads(body)
        user_content = payload["messages"][-1]["content"]
        run_match = re.search(r'"runId": "([^"]+)"', user_content)
        step_match = re.search(r'"stepId": "([^"]+)"', user_content)
        provider_request = {
            "runId": run_match.group(1) if run_match else "",
            "stepId": step_match.group(1) if step_match else "",
        }
        decision = decision_payload(provider_request)
        content = decision if isinstance(decision, str) else json.dumps(decision, separators=(",", ":"))
        response = {"choices": [{"message": {"content": content}}]}
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
ThreadingHTTPServer((host, port), Handler).serve_forever()
PY
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
    if curl -fsS -X POST "http://$MOCK_ADDR/v1/chat/completions" \
      -H 'Content-Type: application/json' \
      -d '{"messages":[{"role":"user","content":"{\"runId\": \"probe\", \"stepId\": \"probe\"}"}]}' >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl -fsS -X POST "http://$MOCK_ADDR/v1/chat/completions" \
    -H 'Content-Type: application/json' \
    -d '{"messages":[{"role":"user","content":"{\"runId\": \"probe\", \"stepId\": \"probe\"}"}]}' >/dev/null
}

write_llm_update_payloads() {
  python3 - "$TMP_DIR/configuration.json" "$TMP_DIR/original-llm-update.json" "$TMP_DIR/verify-llm-update.json" "$MOCK_BASE_URL" <<'PY'
import json
import sys

config_path, original_path, update_path, mock_base_url = sys.argv[1:]
data = json.load(open(config_path, encoding="utf-8"))
llm = data.get("configuration", {}).get("llm", {})
providers = llm.get("providers") or []
default_provider = llm.get("defaultProviderId") or "verify"
default_embedding_provider = llm.get("defaultEmbeddingProviderId") or ""

def clean_provider(item):
    return {
        "id": item.get("id", ""),
        "provider": item.get("provider", ""),
        "baseUrl": item.get("baseUrl", ""),
        "model": item.get("model", ""),
        "timeout": item.get("timeout", ""),
        "disabled": bool(item.get("disabled", False)),
        "chatCompletions": bool(item.get("chatCompletions", True)),
        "streaming": bool(item.get("streaming", False)),
        "nonChatOperations": item.get("nonChatOperations") or [],
    }

original = {
    "defaultProviderId": default_provider,
    "defaultEmbeddingProviderId": default_embedding_provider,
    "providers": [clean_provider(item) for item in providers if item.get("id")],
}
if not original["providers"]:
    original["providers"].append({
        "id": "verify",
        "provider": "compatible",
        "baseUrl": mock_base_url,
        "model": "verify-model",
        "timeout": "60s",
        "disabled": False,
        "chatCompletions": True,
        "streaming": False,
        "nonChatOperations": ["embeddings"],
    })
    original["defaultProviderId"] = "verify"
    original["defaultEmbeddingProviderId"] = "verify"
with open(original_path, "w", encoding="utf-8") as handle:
    json.dump(original, handle, separators=(",", ":"))

updated_providers = [item for item in original["providers"] if item["id"] != "verify"]
updated_providers.append({
    "id": "verify",
    "provider": "compatible",
    "baseUrl": mock_base_url,
    "apiKey": "verify-secret",
    "model": "verify-model",
    "timeout": "60s",
    "disabled": False,
    "chatCompletions": True,
    "streaming": False,
    "nonChatOperations": ["embeddings"],
})
update = {
    "defaultProviderId": "verify",
    "defaultEmbeddingProviderId": "verify",
    "providers": updated_providers,
}
with open(update_path, "w", encoding="utf-8") as handle:
    json.dump(update, handle, separators=(",", ":"))
PY
}

activate_workspace_execution() {
  local output_file="$1"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
    -H 'Content-Type: application/json' \
    -d '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"verify","soulId":"'"$SOUL_ID"'","skillIds":["'"$SKILL_ID"'"],"mcpServerIds":["'"$MCP_SERVER_ID"'"],"memoryIds":["'"$MEMORY_ID"'"],"priority":2}' >"$output_file"
}

activate_workspace_execution_with_auto_memory() {
  local output_file="$1"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
    -H 'Content-Type: application/json' \
    -d '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"verify","soulId":"'"$SOUL_ID"'","skillIds":["'"$SKILL_ID"'"],"mcpServerIds":["'"$MCP_SERVER_ID"'"],"priority":2}' >"$output_file"
}

wait_run_state() {
  local run_id="$1"
  local expected_state="$2"
  local output_file="$3"
  local deadline=$((SECONDS + 30))

  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/status" >"$output_file"
    if grep '"state": "'"$expected_state"'"' "$output_file" >/dev/null; then
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

  curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/snapshot" >"$TMP_DIR/$label-snapshot-read-model.json"
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/observability" >"$TMP_DIR/$label-observability-read-model.json"
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/task-progress" >"$TMP_DIR/$label-task-progress.json"
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/context-bundle" >"$TMP_DIR/$label-context-bundle.json"
  curl -fsS "$BASE_URL/api/v1/workorch/events?runId=$run_id" >"$TMP_DIR/$label-events.json"
  python3 - "$TMP_DIR/$label-snapshot-read-model.json" \
    "$TMP_DIR/$label-observability-read-model.json" \
    "$TMP_DIR/$label-task-progress.json" \
    "$TMP_DIR/$label-context-bundle.json" \
    "$TMP_DIR/$label-events.json" \
    "$run_id" "$workspace_id" "$expected_state" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8"))["snapshot"]
observability = json.load(open(sys.argv[2], encoding="utf-8"))["observability"]
task_progress = json.load(open(sys.argv[3], encoding="utf-8"))["taskProgress"]
bundle = json.load(open(sys.argv[4], encoding="utf-8"))["contextBundle"]
events = json.load(open(sys.argv[5], encoding="utf-8")).get("eventRecords") or []
run_id, workspace_id, expected_state = sys.argv[6:9]

errors = []
run = snapshot.get("run") or {}
if run.get("id") != run_id:
    errors.append(f"snapshot run id is {run.get('id')!r}, want {run_id!r}")
if run.get("workspaceId") != workspace_id:
    errors.append(f"snapshot workspace id is {run.get('workspaceId')!r}, want {workspace_id!r}")
if run.get("state") != expected_state:
    errors.append(f"run state is {run.get('state')!r}, want {expected_state!r}")

compressed = (run.get("executionContext") or {}).get("compressedContext") or {}
if compressed.get("version") != "workorch.compressedContext.v1":
    errors.append(f"executionContext.compressedContext missing or invalid: {compressed}")
if compressed.get("mode") != "deterministic_recovery_summary":
    errors.append(f"compressedContext mode is {compressed.get('mode')!r}")
if compressed.get("runId") != run_id or compressed.get("workspaceId") != workspace_id:
    errors.append(f"compressedContext identity mismatch: {compressed}")
if not (compressed.get("sourceSectionRefs") or []):
    errors.append(f"compressedContext sourceSectionRefs empty: {compressed}")

bundle_compressed = bundle.get("compressedContext") or {}
if bundle.get("runId") != run_id or bundle.get("workspaceId") != workspace_id:
    errors.append(f"context bundle identity mismatch: {bundle.get('workspaceId')}/{bundle.get('runId')}")
if bundle_compressed.get("version") != "workorch.compressedContext.v1":
    errors.append(f"context bundle compressedContext invalid: {bundle_compressed}")
section_ids = {item.get("id") for item in (bundle.get("sections") or [])}
for required in ["workspace", "run", "task_progress", "recovery"]:
    if required not in section_ids:
        errors.append(f"context bundle missing section {required!r}: {section_ids}")
if expected_state == "completed" and "formal_outputs" not in section_ids:
    errors.append(f"context bundle missing completed formal_outputs section: {section_ids}")

if task_progress.get("runId") != run_id or task_progress.get("workspaceId") != workspace_id:
    errors.append(f"taskProgress identity mismatch: {task_progress}")
if not task_progress.get("state"):
    errors.append(f"taskProgress state empty: {task_progress}")
if not (task_progress.get("recovery") or {}).get("mode"):
    errors.append(f"taskProgress recovery mode empty: {task_progress}")
if not (task_progress.get("milestones") or []):
    errors.append(f"taskProgress milestones empty: {task_progress}")

observed_progress = observability.get("taskProgress") or {}
if observed_progress.get("runId") != run_id:
    errors.append(f"observability taskProgress missing run id: {observed_progress}")

topics = {item.get("topic") for item in events}
if "/run/created" not in topics:
    errors.append(f"event stream missing run-created event: {topics}")
if expected_state in {"completed", "failed"} and "/run/state/changed" not in topics:
    errors.append(f"event stream missing terminal run state event: {topics}")
if any(item.get("state") == "accepted" for item in (snapshot.get("results") or [])):
    for topic in ["/result/accepted", "/artifact/created"]:
        if topic not in topics:
            errors.append(f"event stream missing {topic}: {topics}")

if expected_state == "completed":
    if not [item for item in (snapshot.get("results") or []) if item.get("state") == "accepted"]:
        errors.append("completed run missing accepted Result")
    if not snapshot.get("artifacts"):
        errors.append("completed run missing Artifact")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY
}

assert_workspace_recovery_read_models() {
  local label="$1"

  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/task-progress" >"$TMP_DIR/$label-workspace-task-progress.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/context-bundle" >"$TMP_DIR/$label-workspace-context-bundle.json"
  python3 - "$TMP_DIR/$label-workspace-task-progress.json" "$TMP_DIR/$label-workspace-context-bundle.json" "$workspace_id" <<'PY'
import json
import sys

progress = json.load(open(sys.argv[1], encoding="utf-8"))["taskProgress"]
bundle = json.load(open(sys.argv[2], encoding="utf-8"))["contextBundle"]
workspace_id = sys.argv[3]
errors = []
if progress.get("workspaceId") != workspace_id:
    errors.append(f"workspace taskProgress identity mismatch: {progress}")
if not progress.get("state"):
    errors.append(f"workspace taskProgress state empty: {progress}")
if bundle.get("workspaceId") != workspace_id:
    errors.append(f"workspace context bundle identity mismatch: {bundle}")
if (bundle.get("compressedContext") or {}).get("version") != "workorch.compressedContext.v1":
    errors.append(f"workspace context bundle missing compressedContext: {bundle}")
if not (bundle.get("sections") or []):
    errors.append(f"workspace context bundle sections empty: {bundle}")
if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY
}

trap report_failure ERR
trap cleanup EXIT

cd "$ROOT_DIR"
start_mock_provider
wait_mock
wait_ready

curl -fsS "$BASE_URL/readyz" | grep '"status": "ready"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
write_llm_update_payloads
curl -fsS -X PUT "$BASE_URL/api/v1/workorch/configuration/llm" -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/verify-llm-update.json" >"$TMP_DIR/configuration-updated.json"
grep '"apiKeyConfigured": true' "$TMP_DIR/configuration-updated.json" >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
  -d '{"title":"llm acceptance workspace","description":"verify LLM driven workspace execution"}' >"$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]
curl -fsS "$BASE_URL/api/v1/workorch/workspaces" | grep "\"id\": \"$workspace_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id" | grep '"description": "verify LLM driven workspace execution"' >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"verify LLM driven workspace execution"}' >"$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
[[ -n "$goal_id" ]]
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" | grep "\"id\": \"$goal_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/goals/$goal_id" | grep '"verify LLM driven workspace execution"' >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["happy path completes","decision can be resolved","approval rejection fails safely","multi gate decision then approval can complete","schema failure is observable","memory recall is visible"]}' >"$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"
[[ -n "$dod_id" ]]
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" | grep "\"id\": \"$dod_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/dods/$dod_id" | grep '"multi gate decision then approval can complete"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$TMP_DIR/workspace-snapshot-defined.json"
grep "\"id\": \"$workspace_id\"" "$TMP_DIR/workspace-snapshot-defined.json" >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/mcp/servers" -H 'Content-Type: application/json' \
  -d '{"id":"'"$MCP_SERVER_ID"'","name":"MCP Context Verify","transport":"stdio","endpoint":"context-helper","enabled":true,"allowedTools":["repo.summary"],"allowedResources":["workspace://acceptance"],"allowedPrompts":["workorch.acceptance"],"defaultApprovalPolicy":"operator","sessionPolicy":"long_lived","timeoutMillis":60000}' >"$TMP_DIR/mcp-server.json"
grep "\"id\": \"$MCP_SERVER_ID\"" "$TMP_DIR/mcp-server.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/servers" | grep "\"id\": \"$MCP_SERVER_ID\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID" | grep '"sessionPolicy": "long_lived"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID/tools" | grep '"name": "repo.summary"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID/resources" | grep '"uri": "workspace://acceptance"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID/prompts" | grep '"name": "workorch.acceptance"' >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/skills" -H 'Content-Type: application/json' \
  -d '{"id":"'"$SKILL_ID"'","name":"LLM acceptance skill","version":"v1","description":"acceptance skill for governed execution","triggerRules":["workspace acceptance"],"instruction":"Use the provided Goal, DoD, MCP and Memory context to complete the run.","requiredToolKinds":["llm/chat"],"requiredMCPServerRefs":["'"$MCP_SERVER_ID"'"],"constraints":["must preserve governance gates"]}' >"$TMP_DIR/skill.json"
grep "\"id\": \"$SKILL_ID\"" "$TMP_DIR/skill.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/skills" | grep "\"id\": \"$SKILL_ID\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/skills/$SKILL_ID" | grep '"requiredMCPServerRefs"' >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/souls" -H 'Content-Type: application/json' \
  -d '{"id":"'"$SOUL_ID"'","name":"LLM Acceptance Soul","description":"acceptance soul for governed execution","businessDomains":["workorch"],"soulMd":"# LLM Acceptance Soul\n\nDrive the workspace according to Goal and DoD while respecting governance gates.\n\n## Planning\n\ngoal-first\n\n## Decision\n\nask when missing info is required\n\n## Approval\n\nrequire operator approval for risky action\n\n## Validation\n\nproduce DoD evidence\n\n## Output Contract\n\nworkorch llm step contract","allowedSkillRefs":["'"$SKILL_ID"'"],"allowedMCPServerRefs":["'"$MCP_SERVER_ID"'"],"allowedToolKinds":["llm/chat"]}' >"$TMP_DIR/soul.json"
grep "\"id\": \"$SOUL_ID\"" "$TMP_DIR/soul.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/souls" | grep "\"id\": \"$SOUL_ID\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/souls/$SOUL_ID" | grep '"soulMd"' >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/memory" -H 'Content-Type: application/json' \
  -d '{"memoryId":"'"$MEMORY_ID"'","sourceType":"user","memoryType":"project_context","content":"Seed memory for acceptance execution.","summary":"Seeded acceptance memory is available to runs.","tags":["acceptance","seed"],"importance":8,"confidence":9,"visibility":"workspace","embeddingProviderId":"verify","embeddingModel":"verify-embedding","embeddingText":"seeded acceptance memory","vector":[0.1,0.2,0.3]}' >"$TMP_DIR/memory-seed.json"
grep "\"id\": \"$MEMORY_ID\"" "$TMP_DIR/memory-seed.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/memory?workspaceId=$workspace_id" | grep "\"$MEMORY_ID\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/memory?workspaceId=$workspace_id&q=Seeded" | grep "\"$MEMORY_ID\"" >/dev/null

activate_workspace_execution "$TMP_DIR/happy-activation.json"
grep '"state": "completed"' "$TMP_DIR/happy-activation.json" >/dev/null
happy_run_id="$(json_path "$TMP_DIR/happy-activation.json" "activation.run.id")"
curl -fsS "$BASE_URL/api/v1/workorch/runs?workspaceId=$workspace_id" | grep "\"id\": \"$happy_run_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$happy_run_id" | grep '"state": "completed"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$happy_run_id/status" | grep '"state": "completed"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$happy_run_id/snapshot" >"$TMP_DIR/happy-snapshot.json"
grep '"dodAssessment"' "$TMP_DIR/happy-snapshot.json" >/dev/null
grep '"memoryRefs"' "$TMP_DIR/happy-snapshot.json" >/dev/null
grep "\"soulId\": \"$SOUL_ID\"" "$TMP_DIR/happy-snapshot.json" >/dev/null
grep '"skillIds"' "$TMP_DIR/happy-snapshot.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$happy_run_id/observability" >"$TMP_DIR/happy-observability.json"
grep '"terminal": true' "$TMP_DIR/happy-observability.json" >/dev/null
assert_run_recovery_read_models "$happy_run_id" "happy" "completed"
curl -fsS "$BASE_URL/api/v1/workorch/memory?workspaceId=$workspace_id" >"$TMP_DIR/memory-after-happy.json"
grep 'LLM happy path acceptance produced reusable workspace memory' "$TMP_DIR/memory-after-happy.json" >/dev/null

activate_workspace_execution "$TMP_DIR/decision-activation.json"
grep '"state": "blocked_on_decision"' "$TMP_DIR/decision-activation.json" >/dev/null
decision_run_id="$(json_path "$TMP_DIR/decision-activation.json" "activation.run.id")"
ticket_id="$(json_path "$TMP_DIR/decision-activation.json" "activation.run.blockingTicketId")"
ticket_path="$(url_encode "$ticket_id")"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$decision_run_id/status" | grep '"state": "blocked_on_decision"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id" | grep "\"id\": \"$ticket_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/decisions?runId=$decision_run_id" | grep "\"id\": \"$ticket_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_path" | grep '"decisionKind": "provide_missing_info"' >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$ticket_path/resolve" -H 'Content-Type: application/json' \
  -d '{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"llm-acceptance"}}' | grep '"resolutionType": "provide_missing_info"' >/dev/null
wait_run_state "$decision_run_id" "completed" "$TMP_DIR/decision-after-resolve-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$decision_run_id/observability" | grep '"terminal": true' >/dev/null
assert_run_recovery_read_models "$decision_run_id" "decision" "completed"

activate_workspace_execution "$TMP_DIR/approval-activation.json"
grep '"state": "blocked_on_approval"' "$TMP_DIR/approval-activation.json" >/dev/null
approval_run_id="$(json_path "$TMP_DIR/approval-activation.json" "activation.run.id")"
approval_id="$(json_path "$TMP_DIR/approval-activation.json" "activation.run.blockingApprovalId")"
approval_path="$(url_encode "$approval_id")"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$approval_run_id/status" | grep '"state": "blocked_on_approval"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/approvals?workspaceId=$workspace_id" | grep "\"id\": \"$approval_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/approvals?runId=$approval_run_id" | grep "\"id\": \"$approval_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/approvals/$approval_path" >"$TMP_DIR/approval-detail.json"
[[ "$(json_path "$TMP_DIR/approval-detail.json" "approval.action")" == "continue risky test path" ]]
[[ "$(json_path "$TMP_DIR/approval-detail.json" "approval.context.scenario")" == "approval" ]]
curl -fsS -X POST "$BASE_URL/api/v1/workorch/approvals/$approval_path/reject" | grep '"state": "rejected"' >/dev/null
wait_run_state "$approval_run_id" "failed" "$TMP_DIR/approval-rejected-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$approval_run_id/observability" | grep '"terminal": true' >/dev/null
assert_run_recovery_read_models "$approval_run_id" "approval-rejected" "failed"

activate_workspace_execution "$TMP_DIR/multi-gate-decision-activation.json"
grep '"state": "blocked_on_decision"' "$TMP_DIR/multi-gate-decision-activation.json" >/dev/null
multi_gate_run_id="$(json_path "$TMP_DIR/multi-gate-decision-activation.json" "activation.run.id")"
multi_gate_ticket_id="$(json_path "$TMP_DIR/multi-gate-decision-activation.json" "activation.run.blockingTicketId")"
multi_gate_ticket_path="$(url_encode "$multi_gate_ticket_id")"
curl -fsS "$BASE_URL/api/v1/workorch/decisions/$multi_gate_ticket_path" | grep '"decisionKind": "provide_missing_info"' >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$multi_gate_ticket_path/resolve" -H 'Content-Type: application/json' \
  -d '{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"llm-acceptance","nextGate":"approval"}}' | grep '"resolutionType": "provide_missing_info"' >/dev/null
wait_run_state "$multi_gate_run_id" "blocked_on_approval" "$TMP_DIR/multi-gate-approval-status.json"
multi_gate_approval_id="$(json_path "$TMP_DIR/multi-gate-approval-status.json" "status.blockingApprovalId")"
multi_gate_approval_path="$(url_encode "$multi_gate_approval_id")"
curl -fsS "$BASE_URL/api/v1/workorch/approvals/$multi_gate_approval_path" >"$TMP_DIR/multi-gate-approval-detail.json"
grep '"state": "pending"' "$TMP_DIR/multi-gate-approval-detail.json" >/dev/null
[[ "$(json_path "$TMP_DIR/multi-gate-approval-detail.json" "approval.action")" == "complete multi-gate LLM path" ]]
[[ "$(json_path "$TMP_DIR/multi-gate-approval-detail.json" "approval.context.scenario")" == "decision_then_approval" ]]
curl -fsS -X POST "$BASE_URL/api/v1/workorch/approvals/$multi_gate_approval_path/approve" | grep '"state": "approved"' >/dev/null
wait_run_state "$multi_gate_run_id" "completed" "$TMP_DIR/multi-gate-completed-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$multi_gate_run_id/snapshot" >"$TMP_DIR/multi-gate-snapshot.json"
grep "\"id\": \"$multi_gate_ticket_id\"" "$TMP_DIR/multi-gate-snapshot.json" >/dev/null
grep "\"id\": \"$multi_gate_approval_id\"" "$TMP_DIR/multi-gate-snapshot.json" >/dev/null
grep 'multi gate decision then approval can complete' "$TMP_DIR/multi-gate-snapshot.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$multi_gate_run_id/status" | grep '"state": "completed"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$multi_gate_run_id/observability" >"$TMP_DIR/multi-gate-observability.json"
grep '"terminal": true' "$TMP_DIR/multi-gate-observability.json" >/dev/null
assert_run_recovery_read_models "$multi_gate_run_id" "multi-gate" "completed"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$TMP_DIR/workspace-snapshot-after-governance.json"
grep "\"id\": \"$multi_gate_run_id\"" "$TMP_DIR/workspace-snapshot-after-governance.json" >/dev/null
grep "\"id\": \"$multi_gate_ticket_id\"" "$TMP_DIR/workspace-snapshot-after-governance.json" >/dev/null
grep "\"id\": \"$multi_gate_approval_id\"" "$TMP_DIR/workspace-snapshot-after-governance.json" >/dev/null

activate_workspace_execution "$TMP_DIR/schema-failure-activation.json"
grep '"state": "failed"' "$TMP_DIR/schema-failure-activation.json" >/dev/null
schema_run_id="$(json_path "$TMP_DIR/schema-failure-activation.json" "activation.run.id")"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$schema_run_id/snapshot" >"$TMP_DIR/schema-failure-snapshot.json"
grep '"llm_schema_invalid"' "$TMP_DIR/schema-failure-snapshot.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$schema_run_id/observability" | grep '"terminal": true' >/dev/null
assert_run_recovery_read_models "$schema_run_id" "schema-failure" "failed"

activate_workspace_execution_with_auto_memory "$TMP_DIR/memory-recall-activation.json"
grep '"state": "completed"' "$TMP_DIR/memory-recall-activation.json" >/dev/null
memory_run_id="$(json_path "$TMP_DIR/memory-recall-activation.json" "activation.run.id")"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$memory_run_id/snapshot" >"$TMP_DIR/memory-recall-snapshot.json"
grep '"memories"' "$TMP_DIR/memory-recall-snapshot.json" >/dev/null
grep '"memoryRetrieval"' "$TMP_DIR/memory-recall-snapshot.json" >/dev/null
grep "\"$MEMORY_ID\"" "$TMP_DIR/memory-recall-snapshot.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$memory_run_id/observability" | grep '"terminal": true' >/dev/null
assert_run_recovery_read_models "$memory_run_id" "memory-recall" "completed"
assert_workspace_recovery_read_models "final"
curl -fsS "$BASE_URL/metrics" >"$TMP_DIR/metrics.txt"
grep 'workorch_http_requests_total{endpoint="configuration_update"}' "$TMP_DIR/metrics.txt" >/dev/null
grep 'workorch_http_requests_total{endpoint="workspace_execution_activation"}' "$TMP_DIR/metrics.txt" >/dev/null
grep 'workorch_http_requests_total{endpoint="run_observability"}' "$TMP_DIR/metrics.txt" >/dev/null
grep 'workorch_http_requests_total{endpoint="approval_approve"}' "$TMP_DIR/metrics.txt" >/dev/null
grep 'workorch_http_requests_total{endpoint="approval_reject"}' "$TMP_DIR/metrics.txt" >/dev/null

echo "workorch LLM workspace execution verification passed for deployed workorch service $BASE_URL"
