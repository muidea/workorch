#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
BASE_URL="${WORKORCH_MCP_STDIO_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
MOCK_ADDR="${WORKORCH_MCP_STDIO_VERIFY_MOCK_ADDR:-127.0.0.1:18185}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_MCP_STDIO_VERIFY_SUFFIX:-$$}"
CURL_CONNECT_TIMEOUT="${WORKORCH_MCP_STDIO_VERIFY_CURL_CONNECT_TIMEOUT:-2}"
CURL_MAX_TIME="${WORKORCH_MCP_STDIO_VERIFY_CURL_MAX_TIME:-300}"
CURL_READY_MAX_TIME="${WORKORCH_MCP_STDIO_VERIFY_READY_MAX_TIME:-2}"
CURL_READY_ATTEMPTS="${WORKORCH_MCP_STDIO_VERIFY_READY_ATTEMPTS:-50}"
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
  echo "workorch MCP stdio verification failed for deployed workorch service $BASE_URL" >&2
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
    curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -fsS -H "Content-Type: application/json" -X PUT \
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

json_first_id() {
  python3 - "$1" "$2" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)
items = data.get(sys.argv[2]) or []
if items:
    print(items[0].get("id", ""))
PY
}

post_json() {
  local url="$1"
  local body="$2"
  local output="$3"
  local status
  status="$(curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -sS -H "Content-Type: application/json" -X POST "$url" -d "$body" -o "$output" -w "%{http_code}" || true)"
  case "$status" in
    200|201)
      ;;
    *)
      echo "POST $url failed with HTTP $status" >&2
      cat "$output" >&2 || true
      return 1
      ;;
  esac
}

post_activation_json() {
  local url="$1"
  local body="$2"
  local output="$3"
  local status
  status="$(curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -sS -H "Content-Type: application/json" -X POST "$url" -d "$body" -o "$output" -w "%{http_code}" || true)"
  case "$status" in
    200|201|202)
      ;;
    *)
      echo "POST $url failed with HTTP $status" >&2
      cat "$output" >&2 || true
      return 1
      ;;
  esac
}

put_json() {
  local url="$1"
  local body="$2"
  local output="$3"
  local status
  status="$(curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -sS -H "Content-Type: application/json" -X PUT "$url" -d "$body" -o "$output" -w "%{http_code}" || true)"
  case "$status" in
    200|201)
      ;;
    *)
      echo "PUT $url failed with HTTP $status" >&2
      cat "$output" >&2 || true
      return 1
      ;;
  esac
}

put_json_file() {
  local url="$1"
  local body_file="$2"
  local output="$3"
  local status
  status="$(curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -sS -H "Content-Type: application/json" -X PUT "$url" -d @"$body_file" -o "$output" -w "%{http_code}" || true)"
  case "$status" in
    200|201)
      ;;
    *)
      echo "PUT $url failed with HTTP $status" >&2
      cat "$output" >&2 || true
      return 1
      ;;
  esac
}

get_json() {
  local url="$1"
  local output="$2"
  local status
  status="$(curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -sS "$url" -o "$output" -w "%{http_code}" || true)"
  case "$status" in
    200)
      ;;
    *)
      echo "GET $url failed with HTTP $status" >&2
      cat "$output" >&2 || true
      return 1
      ;;
  esac
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
  local status_url
  status_url="$(run_api_url "$workspace_id" "$run_id" "status")"
  get_json "$status_url?waitState=$expected_state&waitTimeoutMillis=120000&waitIntervalMillis=500" "$output"
  python3 - "$output" "$expected_state" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
state = (data.get("status") or {}).get("state")
if state != sys.argv[2]:
    raise SystemExit(f"run state is {state!r}, want {sys.argv[2]!r}")
PY
}

wait_activation_run() {
  local workspace_id="$1"
  local activation_file="$2"
  local before_runs_file="$3"
  local runs_file="$4"
  local output_file="$5"
  local run_id=""
  local deadline=$((SECONDS + 120))

  python3 - "$activation_file" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
errors = []
if data.get("accepted") is not True:
    errors.append(f"accepted is {data.get('accepted')!r}, want true")
if data.get("intent") != "resume":
    errors.append(f"intent is {data.get('intent')!r}, want resume")
if not str(data.get("requestId") or "").strip():
    errors.append("requestId is empty")
if data.get("reason") != "e2e_resume":
    errors.append(f"reason is {data.get('reason')!r}, want e2e_resume")
if errors:
    raise SystemExit("\n".join(errors))
PY

  while (( SECONDS <= deadline )); do
    if get_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" "$runs_file"; then
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
    sleep 1
  done

  echo "activation accepted but run did not materialize within 120s for workspace $workspace_id" >&2
  cat "$activation_file" >&2 || true
  [[ -f "$runs_file" ]] && cat "$runs_file" >&2
  return 1
}

upsert_json() {
  local collection_url="$1"
  local resource_url="$2"
  local body="$3"
  local output="$4"
  local tmp="$output.tmp"
  local status
  status="$(curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -sS -H "Content-Type: application/json" -X POST "$collection_url" -d "$body" -o "$tmp" -w "%{http_code}" || true)"
  case "$status" in
    200|201)
      mv "$tmp" "$output"
      ;;
    409)
      rm -f "$tmp"
      curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -fsS -H "Content-Type: application/json" -X PUT "$resource_url" -d "$body" >"$output"
      ;;
    *)
      echo "POST $collection_url failed with HTTP $status" >&2
      cat "$tmp" >&2 || true
      rm -f "$tmp"
      return 1
      ;;
  esac
}

ensure_goal() {
  local workspace_id="$1"
  local text="$2"
  local list_output="$TMP_DIR/goals-before.json"
  local existing_id
  get_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" "$list_output"
  existing_id="$(json_first_id "$list_output" "goals")"
  if [[ -n "$existing_id" ]]; then
    put_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals/$existing_id" \
      '{"text":"'"$text"'"}' \
      "$TMP_DIR/goal.json"
    return
  fi
  post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" \
    '{"text":"'"$text"'"}' \
    "$TMP_DIR/goal.json"
}

ensure_dod() {
  local workspace_id="$1"
  local item="$2"
  local list_output="$TMP_DIR/dods-before.json"
  local existing_id
  get_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" "$list_output"
  existing_id="$(json_first_id "$list_output" "dods")"
  if [[ -n "$existing_id" ]]; then
    put_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods/$existing_id" \
      '{"items":["'"$item"'"]}' \
      "$TMP_DIR/dod.json"
    return
  fi
  post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" \
    '{"items":["'"$item"'"]}' \
    "$TMP_DIR/dod.json"
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
    if (
        re.search(r'"assessmentOnly"\s*:\s*true', user_content)
        or re.search(r'"graphExpansionLimitFollowUp"\s*:\s*true', user_content)
        or re.search(r'"graphFollowUp"\s*:\s*true', user_content)
        or re.search(r'"assessmentOnlyContractCorrection"\s*:\s*true', user_content)
        or re.search(r'"type"\s*:\s*"assess_only"', user_content)
        or re.search(r'"previousStepOutput"\s*:', user_content)
        or re.search(r'"repairObjective"\s*:\s*"produce_concrete_validation_evidence"', user_content)
        or re.search(r'"completedGraphEvidence"\s*:', user_content)
        or re.search(r'"latestActionEvidence"\s*:', user_content)
        or "mcp_tool_call_completed" in user_content
    ):
        previous = (
            re.search(r'"previousStepId"\s*:\s*"([^"]+)"', user_content)
            or re.search(r'"stepId"\s*:\s*"([^"]*mcp-stdio-call[^"]*)"', user_content)
            or re.search(r'"sourceRef"\s*:\s*"step://([^"]*mcp-stdio-call[^"]*)"', user_content)
        )
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
                }],
                "goalCoverage": {
                    "satisfied": True,
                    "items": [{
                        "requirement": "Plan and execute one MCP stdio repo.summary tool call, then assess the DoD from the tool evidence.",
                        "state": "satisfied",
                        "evidenceRefs": [evidence]
                    }]
                }
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
        if self.path == "/v1/embeddings":
            response = {"data": [{"embedding": [0.1, 0.2, 0.3]}]}
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
        payload = json.loads(self.rfile.read(size))
        user_content = "\n".join(str(item.get("content", "")) for item in payload.get("messages") or [])
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
  for _ in $(seq 1 "$CURL_READY_ATTEMPTS"); do
    if curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_READY_MAX_TIME" -fsS "$BASE_URL/readyz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_READY_MAX_TIME" -fsS "$BASE_URL/readyz" >/dev/null
}

wait_mock() {
  for _ in $(seq 1 "$CURL_READY_ATTEMPTS"); do
    if curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_READY_MAX_TIME" -fsS -X POST "http://$MOCK_ADDR/v1/chat/completions" \
      -H 'Content-Type: application/json' \
      -d '{"messages":[{"role":"user","content":"{\"runId\": \"probe\", \"stepId\": \"probe\"}"}]}' >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_READY_MAX_TIME" -fsS -X POST "http://$MOCK_ADDR/v1/chat/completions" \
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
    "nonChatOperations": ["embeddings"] if default_embedding_provider == "verify" else [],
})
update = {
    "defaultProviderId": "verify",
    "defaultEmbeddingProviderId": "verify" if default_embedding_provider == "verify" else default_embedding_provider,
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

get_json "$BASE_URL/readyz" "$TMP_DIR/readyz.json"
grep '"status": "ready"' "$TMP_DIR/readyz.json" >/dev/null
get_json "$BASE_URL/api/v1/workorch/configuration" "$TMP_DIR/configuration.json"
write_llm_update_payloads
put_json_file "$BASE_URL/api/v1/workorch/configuration/llm" \
  "$TMP_DIR/verify-llm-update.json" \
  "$TMP_DIR/configuration-updated.json"
grep '"apiKeyConfigured": true' "$TMP_DIR/configuration-updated.json" >/dev/null

upsert_json "$BASE_URL/api/v1/workorch/mcp/servers" \
  "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID" \
  '{"id":"'"$MCP_SERVER_ID"'","name":"stdio verification","transport":"stdio","endpoint":"'"$MCP_HELPER"'","enabled":true,"allowedTools":["repo.summary"],"allowedResources":["workspace://acceptance"],"allowedPrompts":["workorch.acceptance"],"timeoutMillis":5000}' \
  "$TMP_DIR/mcp-server.json"
grep '"transport": "stdio"' "$TMP_DIR/mcp-server.json" >/dev/null
get_json "$BASE_URL/api/v1/workorch/mcp/capabilities?serverId=$MCP_SERVER_ID&kind=tool&q=repo.summary" "$TMP_DIR/mcp-capabilities-derived.json"
grep "\"id\": \"$MCP_CAPABILITY_ID\"" "$TMP_DIR/mcp-capabilities-derived.json" >/dev/null
get_json "$BASE_URL/api/v1/workorch/mcp/capabilities?serverId=$MCP_SERVER_ID&kind=resource&q=workspace" "$TMP_DIR/mcp-resource-capabilities-derived.json"
grep '工作空间验收资料' "$TMP_DIR/mcp-resource-capabilities-derived.json" >/dev/null
get_json "$BASE_URL/api/v1/workorch/mcp/capabilities?serverId=$MCP_SERVER_ID&kind=prompt&q=acceptance" "$TMP_DIR/mcp-prompt-capabilities-derived.json"
grep '验收提示词' "$TMP_DIR/mcp-prompt-capabilities-derived.json" >/dev/null
put_json "$BASE_URL/api/v1/workorch/mcp/capabilities/$MCP_CAPABILITY_ID" \
  '{"id":"'"$MCP_CAPABILITY_ID"'","serverId":"'"$MCP_SERVER_ID"'","kind":"tool","name":"repo.summary","displayName":"仓库摘要能力 E2E","description":"通过 MCP stdio 读取仓库摘要，用于能力目录、Workspace 绑定和 Run 证据验收。","category":"代码仓库","tags":["e2e","stdio","repo"],"usageHint":"当 Goal 需要仓库上下文摘要时使用。","inputSummary":"workspaceId、runId、stepId 和工具输入。","outputSummary":"返回仓库摘要 structuredContent。","stageHints":["execution","validation"],"riskLevel":"read_only","enabled":true,"approvalPolicy":"operator","status":"active"}' \
  "$TMP_DIR/mcp-capability-updated.json"
grep '"displayName": "仓库摘要能力 E2E"' "$TMP_DIR/mcp-capability-updated.json" >/dev/null
get_json "$BASE_URL/api/v1/workorch/mcp/capabilities?category=%E4%BB%A3%E7%A0%81%E4%BB%93%E5%BA%93&stage=execution&riskLevel=read_only&enabled=true&q=stdio" "$TMP_DIR/mcp-capabilities-filtered.json"
grep "\"id\": \"$MCP_CAPABILITY_ID\"" "$TMP_DIR/mcp-capabilities-filtered.json" >/dev/null

post_json "$BASE_URL/api/v1/workorch/workspaces" \
  '{"title":"mcp stdio verification","description":"verify MCP stdio transport through LLM execution graph","templateId":"generic-governed"}' \
  "$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]

ensure_goal "$workspace_id" \
  "Plan and execute one MCP stdio repo.summary tool call, then assess the DoD from the tool evidence."
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
[[ -n "$goal_id" ]]

ensure_dod "$workspace_id" "stdio MCP tool call completes"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"
[[ -n "$dod_id" ]]

get_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" "$TMP_DIR/activation-before-runs.json"
post_activation_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  '{"intent":"resume","requestId":"mcp-stdio-e2e-'"$VERIFY_SUFFIX"'","reason":"e2e_resume","source":"e2e","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"verify","mcpServerIds":["'"$MCP_SERVER_ID"'"],"capabilityIds":["'"$MCP_CAPABILITY_ID"'"]}' \
  "$TMP_DIR/activation.json"
run_id="$(wait_activation_run "$workspace_id" "$TMP_DIR/activation.json" "$TMP_DIR/activation-before-runs.json" "$TMP_DIR/activation-runs.json" "$TMP_DIR/activation-run.json")"
[[ -n "$run_id" ]]

wait_run_state "$workspace_id" "$run_id" "completed" "$TMP_DIR/run-status.json"
get_json "$(run_api_url "$workspace_id" "$run_id" "snapshot")" "$TMP_DIR/snapshot.json"
get_json "$(run_api_url "$workspace_id" "$run_id" "observability")" "$TMP_DIR/observability.json"
grep '"selectedCapabilities"' "$TMP_DIR/observability.json" >/dev/null
grep '"usedCapabilities"' "$TMP_DIR/observability.json" >/dev/null
echo "workspaceId=$workspace_id"
echo "runId=$run_id"
validate_snapshot "$TMP_DIR/snapshot.json"

echo "workorch MCP stdio verification passed for deployed workorch service $BASE_URL"
