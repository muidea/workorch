#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_REAL_MCP_STDIO_BASE_URL:-${WORKORCH_BASE_URL:-}}"
source "$ROOT_DIR/scripts/lib/verify-isolated-runtime.sh"
workorch_verify_require_isolated_runtime "$BASE_URL" "verify-real-mcp-stdio-workspace-execution"
MOCK_ADDR="${WORKORCH_REAL_MCP_STDIO_MOCK_ADDR:-127.0.0.1:18188}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
PROVIDER_ID="${WORKORCH_REAL_MCP_STDIO_PROVIDER_ID:-verify}"
MCP_SERVER_ID="${WORKORCH_REAL_MCP_STDIO_SERVER_ID:-mcp-stdio-real-verify-$$}"
TMP_DIR="${WORKORCH_REAL_MCP_STDIO_ARTIFACT_DIR:-$(mktemp -d)}"
ACTIVATION_RUN_WAIT_SECONDS="${WORKORCH_REAL_MCP_STDIO_ACTIVATION_RUN_WAIT_SECONDS:-60}"
MCP_ENDPOINT_ROOT="${WORKORCH_MCP_ENDPOINT_DIR:-.workorch-data/system/mcp-endpoints}"
MCP_ENDPOINT_DIR="$MCP_ENDPOINT_ROOT/$MCP_SERVER_ID"
KEEP_ARTIFACTS="${WORKORCH_REAL_MCP_STDIO_KEEP_ARTIFACTS:-1}"
MOCK_PID=""
MOCK_LOG="$TMP_DIR/llm-mock.log"
MCP_HELPER="$MCP_ENDPOINT_DIR/mcp_stdio_helper.py"

cleanup() {
  local status=$?
  if [[ -f "$TMP_DIR/original-llm-update.json" ]]; then
    curl -fsS -H "Content-Type: application/json" -X PUT \
      "$BASE_URL/api/v1/workorch/configuration/llm" \
      -d @"$TMP_DIR/original-llm-update.json" >/dev/null 2>&1 || true
  fi
  if [[ -n "$MOCK_PID" ]]; then
    kill "$MOCK_PID" >/dev/null 2>&1 || true
    wait "$MOCK_PID" >/dev/null 2>&1 || true
  fi
  if [[ "$KEEP_ARTIFACTS" != "1" && $status -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  fi
}

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "real service MCP stdio verification failed" >&2
  echo "artifacts=$TMP_DIR" >&2
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

run_api_url() {
  local workspace_id="$1"
  local run_id="$2"
  local action="${3:-}"
  local suffix=""
  if [[ -n "$action" ]]; then
    suffix="/$action"
  fi
  printf '%s/api/v1/workorch/workspaces/%s/runs/%s%s' "$BASE_URL" "$workspace_id" "$run_id" "$suffix"
}

wait_run_state() {
  local workspace_id="$1"
  local run_id="$2"
  local expected_state="$3"
  local output="$4"
  curl -fsS "$(run_api_url "$workspace_id" "$run_id" "status")?waitState=$expected_state&waitTimeoutMillis=300000&waitIntervalMillis=1000" >"$output"
  python3 - "$output" "$expected_state" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
state = (data.get("status") or {}).get("state")
if state != sys.argv[2]:
    raise SystemExit(f"run state is {state!r}, want {sys.argv[2]!r}")
PY
}

post_json() {
  local url="$1"
  local body="$2"
  local output="$3"
  curl -fsS -H "Content-Type: application/json" -X POST "$url" -d "$body" >"$output"
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
if data.get("intent") != "resume":
    errors.append(f"intent is {data.get('intent')!r}, want resume")
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
            "thoughtSummary": "MCP stdio tool call completed on the deployed service.",
            "completionState": "completed",
            "dodAssessment": {
                "satisfied": True,
                "summary": "The deployed service executed the governed MCP stdio tool call.",
                "items": [{
                    "item": "deployed service executes stdio MCP tool call",
                    "state": "satisfied",
                    "evidenceRefs": [evidence]
                }]
            }
        }
    return {
        "thoughtSummary": "Plan one MCP stdio tool call for deployed service verification.",
        "completionState": "continue",
        "executionGraph": {
            "nodes": [{
                "id": "mcp-stdio-call",
                "kind": "mcp/tool_call",
                "input": {
                    "serverId": os.environ.get("WORKORCH_REAL_MCP_STDIO_SERVER_ID", "mcp-stdio-real-verify"),
                    "tool": "repo.summary",
                    "input": {"path": "."}
                }
            }]
        }
    }

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        path = self.path.split("?", 1)[0]
        if path in ("/v1/models", "/models"):
            # WorkOrch catalog expects explicit capacity + operations; without these models stay not-ready.
            models = [
                {
                    "id": "verify-model",
                    "object": "model",
                    "owned_by": "verify",
                    "operations": ["chat_completions", "embeddings"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
                {
                    "id": "gpt-4o",
                    "object": "model",
                    "owned_by": "openai",
                    "operations": ["chat_completions"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
                {
                    "id": "text-embedding-3-small",
                    "object": "model",
                    "owned_by": "openai",
                    "operations": ["embeddings"],
                    "contextWindowTokens": 8192,
                    "maxOutputTokens": 1024,
                },

                {
                    "id": "verify-stream-native-model",
                    "object": "model",
                    "owned_by": "verify",
                    "operations": ["chat_completions", "embeddings"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
                {
                    "id": "verify-stream-estimated-model",
                    "object": "model",
                    "owned_by": "verify",
                    "operations": ["chat_completions", "embeddings"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
                {
                    "id": "web-search-e2e-model",
                    "object": "model",
                    "owned_by": "verify",
                    "operations": ["chat_completions", "embeddings"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
                {
                    "id": "container-verify-model",
                    "object": "model",
                    "owned_by": "verify",
                    "operations": ["chat_completions", "embeddings"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
            ]
            response = {"object": "list", "data": models}
            encoded = json.dumps(response).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)
            return
        self.send_response(404)
        self.end_headers()

    def do_POST(self):
        if self.path != "/v1/chat/completions":
            self.send_response(404)
            self.end_headers()
            return
        size = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(size))
        user_content = payload["messages"][-1]["content"]
        response = {
            "choices": [{
                "message": {
                    "content": json.dumps(decision_payload(user_content), separators=(",", ":"))
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

host, port_text = os.environ.get("WORKORCH_REAL_MCP_STDIO_MOCK_ADDR", "127.0.0.1:18188").split(":", 1)
ThreadingHTTPServer((host, int(port_text)), Handler).serve_forever()
PY
  WORKORCH_REAL_MCP_STDIO_MOCK_ADDR="$MOCK_ADDR" WORKORCH_REAL_MCP_STDIO_SERVER_ID="$MCP_SERVER_ID" python3 "$TMP_DIR/llm_mock_provider.py" >>"$MOCK_LOG" 2>&1 &
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
                "serverInfo": {"name": "workorch-real-mcp-stdio-verify", "version": "test"}
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
                "content": [{"type": "text", "text": "deployed service stdio MCP result"}],
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
  python3 - "$TMP_DIR/configuration.json" "$TMP_DIR/original-llm-update.json" "$TMP_DIR/verify-llm-update.json" "$MOCK_BASE_URL" "$PROVIDER_ID" <<'PY'
import json
import sys

config_path, original_path, update_path, mock_base_url, provider_id = (sys.argv[1:] + [""])[:5]
provider_id = (provider_id or "verify").strip() or "verify"
data = json.load(open(config_path, encoding="utf-8"))
llm = data.get("configuration", {}).get("llm", {})
ai_proxy = llm.get("aiProxy") or {}
default_chat = llm.get("defaultChat") or {}
default_embedding = llm.get("defaultEmbedding") or {}
budget = llm.get("budget") or {}
custom_providers = llm.get("customProviders") or []

def custom_providers_payload(items, include_api_key=False, mock_base_url=""):
    payload = {}
    for item in items:
        provider_id = (item.get("id") or "").strip()
        if not provider_id:
            continue
        models = {}
        for model in item.get("models") or []:
            model_id = (model.get("id") or "").strip()
            if not model_id:
                continue
            models[model_id] = {
                "contextWindowTokens": int(model.get("contextWindowTokens") or 128000),
                "maxOutputTokens": int(model.get("maxOutputTokens") or 8192),
                "operations": list(model.get("operations") or ["chat_completions"]),
            }
        if not models:
            models["verify-model"] = {
                "contextWindowTokens": 128000,
                "maxOutputTokens": 8192,
                "operations": ["chat_completions", "embeddings"],
            }
        entry = {
            "provider": item.get("provider") or "compatible",
            "apiSurface": item.get("apiSurface") or "openai_compatible_http",
            "baseURL": item.get("baseUrl") or item.get("baseURL") or mock_base_url,
            "timeout": item.get("timeout") or "60s",
            "models": models,
        }
        if include_api_key:
            entry["apiKey"] = "verify-secret"
        payload[provider_id] = entry
    return payload

original = {
    "aiProxy": {
        "baseURL": ai_proxy.get("baseUrl") or ai_proxy.get("baseURL") or mock_base_url,
        "timeout": ai_proxy.get("timeout") or "180s",
        "catalogRequestTimeout": ai_proxy.get("catalogRequestTimeout") or "5s",
        "catalogTTL": ai_proxy.get("catalogTTL") or "5m",
        "catalogMaxStaleAge": ai_proxy.get("catalogMaxStaleAge") or "24h",
        "maxCatalogResponseBytes": int(ai_proxy.get("maxCatalogResponseBytes") or 1048576),
        "maxCatalogModels": int(ai_proxy.get("maxCatalogModels") or 1000),
    },
    "defaultChat": {
        "providerID": default_chat.get("providerId") or default_chat.get("providerID") or "verify",
        "modelID": default_chat.get("modelId") or default_chat.get("modelID") or "verify-model",
    },
    "budget": {
        "contextUtilizationRatio": float(budget.get("contextUtilizationRatio") or 0.75),
        "estimatedTokensPerByte": float(budget.get("estimatedTokensPerByte") or 0.25),
    },
    "customProviders": custom_providers_payload(custom_providers, include_api_key=False, mock_base_url=mock_base_url),
}
if default_embedding.get("modelId") or default_embedding.get("modelID"):
    original["defaultEmbedding"] = {
        "providerID": default_embedding.get("providerId") or default_embedding.get("providerID") or "verify",
        "modelID": default_embedding.get("modelId") or default_embedding.get("modelID"),
    }
if not original["customProviders"]:
    original["customProviders"] = {
        provider_id: {
            "provider": "compatible",
            "apiSurface": "openai_compatible_http",
            "baseURL": mock_base_url,
            "timeout": "60s",
            "models": {
                "verify-model": {
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                    "operations": ["chat_completions", "embeddings"],
                }
            },
        }
    }
    original["defaultChat"] = {"providerID": provider_id, "modelID": "verify-model"}
with open(original_path, "w", encoding="utf-8") as handle:
    json.dump(original, handle, separators=(",", ":"))

update = {
    "aiProxy": {
        "baseURL": mock_base_url,
        "apiKey": "verify-secret",
        "timeout": "180s",
        "catalogRequestTimeout": "5s",
        "catalogTTL": "5m",
        "catalogMaxStaleAge": "24h",
        "maxCatalogResponseBytes": 1048576,
        "maxCatalogModels": 1000,
        "modelOverrides": {
            "verify-model": {"contextWindowTokens": 128000, "maxOutputTokens": 8192, "operations": ["chat_completions", "embeddings"]},
            "gpt-4o": {"contextWindowTokens": 128000, "maxOutputTokens": 8192, "operations": ["chat_completions", "embeddings"]},
        },
    },
    "defaultChat": {"providerID": provider_id, "modelID": "verify-model"},
    "defaultEmbedding": {"providerID": provider_id, "modelID": "verify-model"},
    "budget": original["budget"],
    "customProviders": {
        provider_id: {
            "provider": "compatible",
            "apiSurface": "openai_compatible_http",
            "baseURL": mock_base_url,
            "apiKey": "verify-secret",
            "timeout": "60s",
            "models": {
                "verify-model": {
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                    "operations": ["chat_completions", "embeddings"],
                },
                "verify-stream-native-model": {
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                    "operations": ["chat_completions", "embeddings"],
                },
                "verify-stream-estimated-model": {
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                    "operations": ["chat_completions", "embeddings"],
                },
            },
        }
    },
}
with open(update_path, "w", encoding="utf-8") as handle:
    json.dump(update, handle, separators=(",", ":"))
PY
}

validate_snapshot() {
  python3 - "$1" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8"))["snapshot"]
run = snapshot["run"]
steps = snapshot.get("steps") or ([] if not snapshot.get("step") else [snapshot["step"]])
tool_calls = snapshot.get("toolCalls") or []
validations = snapshot.get("validationResults") or []
checkpoints = snapshot.get("runtimeCheckpoints") or []
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
mcp_steps = [s for s in steps if s.get("kind") == "mcp/tool_call"]
if not mcp_steps:
    errors.append("missing mcp/tool_call step")
else:
    output = mcp_steps[0].get("output") or {}
    result = output.get("result") or {}
    structured = result.get("structuredContent") or {}
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

mkdir -p "$TMP_DIR"
cd "$ROOT_DIR"
create_mcp_helper
start_mock_provider
wait_mock

curl -fsS "$BASE_URL/readyz" >"$TMP_DIR/readyz.json"
python3 - "$TMP_DIR/readyz.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
if data.get("status") != "ready":
    raise SystemExit(f"readyz status is {data.get('status')!r}, want ready")
PY

curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
write_llm_update_payloads
curl -fsS -H "Content-Type: application/json" -X PUT \
  "$BASE_URL/api/v1/workorch/configuration/llm" \
  -d @"$TMP_DIR/verify-llm-update.json" >"$TMP_DIR/configuration-updated.json"

upsert_json "$BASE_URL/api/v1/workorch/mcp/servers" \
  "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID" \
  '{"id":"'"$MCP_SERVER_ID"'","name":"real service stdio verification","transport":"stdio","endpoint":"'"$MCP_HELPER"'","enabled":true,"allowedTools":["repo.summary"],"timeoutMillis":5000}' \
  "$TMP_DIR/mcp-server.json"

post_json "$BASE_URL/api/v1/workorch/workspaces" \
  '{"title":"real service mcp stdio verification","description":"verify deployed service MCP stdio transport through LLM execution graph","templateId":"generic-governed"}' \
  "$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]

post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" \
  '{"text":"Use the selected MCP server '"$MCP_SERVER_ID"' to execute repo.summary through stdio. Return an executionGraph first, then after tool evidence return completed dodAssessment with a non-empty summary and evidenceRefs."}' \
  "$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
[[ -n "$goal_id" ]]

post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" \
  '{"items":["deployed service executes stdio MCP tool call"]}' \
  "$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"
[[ -n "$dod_id" ]]

activation_request_id="real-mcp-stdio-e2e-$$"
activation_reason="e2e_resume"
capture_runs_before_activation "$TMP_DIR/activation-before-runs.json"
post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  '{"intent":"resume","requestId":"'"$activation_request_id"'","reason":"'"$activation_reason"'","source":"e2e","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmTarget":{"providerId":"'"$PROVIDER_ID"'","modelId":"verify-model"}","mcpServerIds":["'"$MCP_SERVER_ID"'"]}' \
  "$TMP_DIR/activation.json"
validate_activation_command_response "$TMP_DIR/activation.json" "$activation_request_id" "$activation_reason"
run_id="$(wait_activation_run "$TMP_DIR/activation-before-runs.json" "$TMP_DIR/activation-runs.json" "$TMP_DIR/activation-run.json")"
[[ -n "$run_id" ]]

wait_run_state "$workspace_id" "$run_id" "completed" "$TMP_DIR/run-status.json"
curl -fsS "$(run_api_url "$workspace_id" "$run_id" "snapshot")" >"$TMP_DIR/snapshot.json"
echo "workspaceId=$workspace_id"
validate_snapshot "$TMP_DIR/snapshot.json"

echo "real service MCP stdio verification passed for provider=$PROVIDER_ID"
echo "artifacts=$TMP_DIR"
