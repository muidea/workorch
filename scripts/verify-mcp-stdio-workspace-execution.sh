#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
BASE_URL="${WORKORCH_MCP_STDIO_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
MOCK_ADDR="${WORKORCH_MCP_STDIO_VERIFY_MOCK_ADDR:-127.0.0.1:18185}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_MCP_STDIO_VERIFY_SUFFIX:-$$}"
MCP_SERVER_ID="mcp-stdio-verify-$VERIFY_SUFFIX"
MCP_CAPABILITY_ID="$MCP_SERVER_ID:tool:repo.summary"
MCP_ENDPOINT_ROOT="${WORKORCH_MCP_ENDPOINT_DIR:-.workorch-data/system/mcp-endpoints}"
MCP_ENDPOINT_DIR="$MCP_ENDPOINT_ROOT/$MCP_SERVER_ID"
MOCK_PID=""
MOCK_LOG="$TMP_DIR/llm-mock.log"
MCP_HELPER="$MCP_ENDPOINT_DIR/mcp_stdio_helper.py"

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "workorchd MCP stdio verification failed for deployed service $BASE_URL" >&2
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

post_json() {
  local url="$1"
  local body="$2"
  local output="$3"
  curl -fsS -H "Content-Type: application/json" -X POST "$url" -d "$body" >"$output"
}

upsert_json() {
  local collection_url="$1"
  local resource_url="$2"
  local body="$3"
  local output="$4"
  local tmp="$output.tmp"
  local status
  status="$(curl -sS -H "Content-Type: application/json" -X POST "$collection_url" -d "$body" -o "$tmp" -w "%{http_code}" || true)"
  case "$status" in
    200|201)
      mv "$tmp" "$output"
      ;;
    409)
      rm -f "$tmp"
      curl -fsS -H "Content-Type: application/json" -X PUT "$resource_url" -d "$body" >"$output"
      ;;
    *)
      cat "$tmp" >&2 || true
      rm -f "$tmp"
      return 1
      ;;
  esac
}

start_mock_provider() {
  cat >"$TMP_DIR/llm_mock_provider.py" <<'PY'
import json
import os
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

def decision_payload(user_content):
    if '"runId": "probe"' in user_content:
        return {"completionState": "failed", "thoughtSummary": "probe"}
    if '"graphFollowUp": true' in user_content or '"previousStepOutput"' in user_content:
        previous = re.search(r'"previousStepId": "([^"]+)"', user_content)
        evidence = previous.group(1) if previous else "mcp-stdio-call"
        return {
            "thoughtSummary": "MCP stdio tool call completed; DoD is satisfied.",
            "completionState": "completed",
            "dodAssessment": {
                "satisfied": True,
                "summary": "MCP stdio invocation returned structured tool evidence.",
                "items": [{
                    "item": "stdio MCP tool call completes",
                    "state": "satisfied",
                    "evidenceRefs": [evidence]
                }]
            }
        }
    return {
        "thoughtSummary": "Plan one governed MCP stdio tool call.",
        "completionState": "continue",
        "executionGraph": {
            "nodes": [{
                "id": "mcp-stdio-call",
                "kind": "mcp/tool_call",
                "input": {
                    "serverId": os.environ.get("WORKORCH_MCP_STDIO_VERIFY_SERVER_ID", "mcp-stdio-verify"),
                    "tool": "repo.summary",
                    "input": {"path": "."}
                }
            }]
        }
    }

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path != "/v1/chat/completions":
            self.send_response(404)
            self.end_headers()
            return
        size = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(size))
        user_content = payload["messages"][-1]["content"]
        decision = decision_payload(user_content)
        response = {
            "choices": [{
                "message": {
                    "content": json.dumps(decision, separators=(",", ":"))
                }
            }]
        }
        encoded = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)

host, port_text = os.environ.get("WORKORCH_MCP_STDIO_VERIFY_MOCK_ADDR", "127.0.0.1:18185").split(":", 1)
ThreadingHTTPServer((host, int(port_text)), Handler).serve_forever()
PY
  WORKORCH_MCP_STDIO_VERIFY_MOCK_ADDR="$MOCK_ADDR" WORKORCH_MCP_STDIO_VERIFY_SERVER_ID="$MCP_SERVER_ID" python3 "$TMP_DIR/llm_mock_provider.py" >>"$MOCK_LOG" 2>&1 &
  MOCK_PID=$!
}

create_mcp_helper() {
  mkdir -p "$MCP_ENDPOINT_DIR"
  cat >"$MCP_HELPER" <<'PY'
#!/usr/bin/env python3
import json
import sys

for line in sys.stdin:
    if not line.strip():
        continue
    request = json.loads(line)
    method = request.get("method")
    request_id = request.get("id")
    if method == "initialize":
        print(json.dumps({
            "jsonrpc": "2.0",
            "id": request_id,
            "result": {
                "protocolVersion": request.get("params", {}).get("protocolVersion", "2025-11-25"),
                "capabilities": {"tools": {}},
                "serverInfo": {"name": "workorch-mcp-stdio-verify", "version": "test"}
            }
        }), flush=True)
    elif method == "notifications/initialized":
        continue
    elif method == "tools/call":
        params = request.get("params", {})
        print(json.dumps({
            "jsonrpc": "2.0",
            "id": request_id,
            "result": {
                "content": [{"type": "text", "text": "repo summary ready through stdio"}],
                "structuredContent": {
                    "transport": "stdio",
                    "tool": params.get("name"),
                    "arguments": params.get("arguments", {}),
                    "ok": True
                },
                "isError": False
            }
        }), flush=True)
    else:
        print(json.dumps({
            "jsonrpc": "2.0",
            "id": request_id,
            "error": {"code": -32601, "message": "method not found"}
        }), flush=True)
PY
  chmod +x "$MCP_HELPER"
}

wait_ready() {
  for _ in $(seq 1 50); do
    if curl -fsS "$BASE_URL/readyz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl -fsS "$BASE_URL/readyz" >/dev/null
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
        "nonChatOperations": [],
    })
    original["defaultProviderId"] = "verify"
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
    "nonChatOperations": [],
})
update = {
    "defaultProviderId": "verify",
    "defaultEmbeddingProviderId": default_embedding_provider,
    "providers": updated_providers,
}
with open(update_path, "w", encoding="utf-8") as handle:
    json.dump(update, handle, separators=(",", ":"))
PY
}

validate_snapshot() {
  python3 - "$1" "$MCP_CAPABILITY_ID" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8"))["snapshot"]
capability_id = sys.argv[2]
run = snapshot["run"]
steps = snapshot.get("steps") or ([] if not snapshot.get("step") else [snapshot["step"]])
tool_calls = snapshot.get("toolCalls") or []
validations = snapshot.get("validationResults") or []
checkpoints = snapshot.get("runtimeCheckpoints") or []
selected_capabilities = snapshot.get("selectedCapabilities") or []
used_capabilities = snapshot.get("usedCapabilities") or []

errors = []
if run.get("state") != "completed":
    errors.append(f"run state is {run.get('state')!r}, want completed: {run.get('failureReason', '')}")

mcp_steps = [s for s in steps if s.get("kind") == "mcp/tool_call"]
if not mcp_steps:
    errors.append("missing mcp/tool_call step")
else:
    output = mcp_steps[0].get("output") or {}
    result = output.get("result") or {}
    structured = result.get("structuredContent") or {}
    if mcp_steps[0].get("state") != "completed" or output.get("status") != "completed":
        errors.append(f"mcp step did not complete: {mcp_steps[0]}")
    if output.get("transport") != "stdio":
        errors.append(f"mcp transport is {output.get('transport')!r}, want stdio")
    if structured.get("tool") != "repo.summary" or structured.get("transport") != "stdio":
        errors.append(f"unexpected structured MCP result: {structured}")

assessment_steps = [s for s in steps if (s.get("output") or {}).get("stage") == "llm_dod_assessment"]
if not assessment_steps:
    errors.append("missing llm_dod_assessment follow-up step")
else:
    assessment = (assessment_steps[-1].get("output") or {}).get("dodAssessment") or {}
    if assessment.get("satisfied") is not True:
        errors.append(f"dodAssessment.satisfied is {assessment.get('satisfied')!r}, want true")
    if not str(assessment.get("summary") or "").strip():
        errors.append(f"dodAssessment.summary is empty: {assessment!r}")

if not tool_calls:
    errors.append("missing ToolCall evidence")
if not validations:
    errors.append("missing ValidationResult evidence")
if not checkpoints:
    errors.append("missing RuntimeCheckpoint evidence")
if not any(item.get("id") == capability_id and item.get("displayName") == "仓库摘要能力 E2E" for item in selected_capabilities):
    errors.append(f"missing selected MCP capability metadata: {selected_capabilities}")
if not any(item.get("id") == capability_id and item.get("useCount", 0) >= 1 for item in used_capabilities):
    errors.append(f"missing used MCP capability evidence: {used_capabilities}")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)

print(f"runId={run['id']}")
print(f"runState={run['state']}")
print(f"steps={len(steps)}")
print(f"toolCalls={len(tool_calls)}")
print(f"validations={len(validations)}")
print(f"runtimeCheckpoints={len(checkpoints)}")
PY
}

trap report_failure ERR
trap cleanup EXIT

cd "$ROOT_DIR"
create_mcp_helper
start_mock_provider
wait_mock
wait_ready

curl -fsS "$BASE_URL/readyz" >"$TMP_DIR/readyz.json"
grep '"status": "ready"' "$TMP_DIR/readyz.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
write_llm_update_payloads
curl -fsS -H "Content-Type: application/json" -X PUT \
  "$BASE_URL/api/v1/workorch/configuration/llm" \
  -d @"$TMP_DIR/verify-llm-update.json" >"$TMP_DIR/configuration-updated.json"
grep '"apiKeyConfigured": true' "$TMP_DIR/configuration-updated.json" >/dev/null

upsert_json "$BASE_URL/api/v1/workorch/mcp/servers" \
  "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID" \
  '{"id":"'"$MCP_SERVER_ID"'","name":"stdio verification","transport":"stdio","endpoint":"'"$MCP_HELPER"'","enabled":true,"allowedTools":["repo.summary"],"allowedResources":["workspace://acceptance"],"allowedPrompts":["workorch.acceptance"],"timeoutMillis":5000}' \
  "$TMP_DIR/mcp-server.json"
grep '"transport": "stdio"' "$TMP_DIR/mcp-server.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/capabilities?serverId=$MCP_SERVER_ID&kind=tool&q=repo.summary" >"$TMP_DIR/mcp-capabilities-derived.json"
grep "\"id\": \"$MCP_CAPABILITY_ID\"" "$TMP_DIR/mcp-capabilities-derived.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/capabilities?serverId=$MCP_SERVER_ID&kind=resource&q=workspace" >"$TMP_DIR/mcp-resource-capabilities-derived.json"
grep '工作空间验收资料' "$TMP_DIR/mcp-resource-capabilities-derived.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/capabilities?serverId=$MCP_SERVER_ID&kind=prompt&q=acceptance" >"$TMP_DIR/mcp-prompt-capabilities-derived.json"
grep '验收提示词' "$TMP_DIR/mcp-prompt-capabilities-derived.json" >/dev/null
curl -fsS -H "Content-Type: application/json" -X PUT \
  "$BASE_URL/api/v1/workorch/mcp/capabilities/$MCP_CAPABILITY_ID" \
  -d '{"id":"'"$MCP_CAPABILITY_ID"'","serverId":"'"$MCP_SERVER_ID"'","kind":"tool","name":"repo.summary","displayName":"仓库摘要能力 E2E","description":"通过 MCP stdio 读取仓库摘要，用于能力目录、Workspace 绑定和 Run 证据验收。","category":"代码仓库","tags":["e2e","stdio","repo"],"usageHint":"当 Goal 需要仓库上下文摘要时使用。","inputSummary":"workspaceId、runId、stepId 和工具输入。","outputSummary":"返回仓库摘要 structuredContent。","stageHints":["execution","validation"],"riskLevel":"read_only","enabled":true,"approvalPolicy":"operator","status":"active"}' \
  >"$TMP_DIR/mcp-capability-updated.json"
grep '"displayName": "仓库摘要能力 E2E"' "$TMP_DIR/mcp-capability-updated.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/capabilities?category=%E4%BB%A3%E7%A0%81%E4%BB%93%E5%BA%93&stage=execution&riskLevel=read_only&enabled=true&q=stdio" >"$TMP_DIR/mcp-capabilities-filtered.json"
grep "\"id\": \"$MCP_CAPABILITY_ID\"" "$TMP_DIR/mcp-capabilities-filtered.json" >/dev/null

post_json "$BASE_URL/api/v1/workorch/workspaces" \
  '{"title":"mcp stdio verification","description":"verify MCP stdio transport through LLM execution graph"}' \
  "$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]

post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" \
  '{"text":"Plan and execute one approved MCP stdio repo.summary tool call, then assess the DoD from the tool evidence."}' \
  "$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
[[ -n "$goal_id" ]]

post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" \
  '{"items":["stdio MCP tool call completes"]}' \
  "$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"
[[ -n "$dod_id" ]]

post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"verify","mcpServerIds":["'"$MCP_SERVER_ID"'"],"capabilityIds":["'"$MCP_CAPABILITY_ID"'"]}' \
  "$TMP_DIR/activation.json"
run_id="$(json_path "$TMP_DIR/activation.json" "activation.run.id")"
[[ -n "$run_id" ]]

curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/snapshot" >"$TMP_DIR/snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/observability" >"$TMP_DIR/observability.json"
grep '"selectedCapabilities"' "$TMP_DIR/observability.json" >/dev/null
grep '"usedCapabilities"' "$TMP_DIR/observability.json" >/dev/null
echo "workspaceId=$workspace_id"
echo "runId=$run_id"
validate_snapshot "$TMP_DIR/snapshot.json"

echo "workorchd MCP stdio verification passed for deployed service $BASE_URL"
