#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ "${1:-}" == "--self-test" ]]; then
  exec python3 "$ROOT_DIR/scripts/verify_mcp_stdio_mock.py" --self-test
fi
if [[ $# -gt 0 ]]; then
  echo "usage: $0 [--self-test]" >&2
  exit 2
fi
BASE_URL="${WORKORCH_MCP_STDIO_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-}}"
source "$ROOT_DIR/scripts/lib/verify-isolated-runtime.sh"
workorch_verify_require_isolated_runtime "$BASE_URL" "verify-mcp-stdio-workspace-execution"
if [[ -n "${WORKORCH_VERIFY_TMP_ROOT:-}" ]]; then
  mkdir -p "$WORKORCH_VERIFY_TMP_ROOT"
  TMP_DIR="$(mktemp -d "$WORKORCH_VERIFY_TMP_ROOT/mcp-stdio.XXXXXX")"
  TMP_DIR="$(realpath "$TMP_DIR")"
else
  TMP_DIR="$(mktemp -d)"
fi
KEEP_TMP="${WORKORCH_VERIFY_KEEP_TMP:-0}"
MOCK_ADDR="${WORKORCH_MCP_STDIO_VERIFY_MOCK_ADDR:-127.0.0.1:18181}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_MCP_STDIO_VERIFY_SUFFIX:-$$}"
CURL_CONNECT_TIMEOUT="${WORKORCH_MCP_STDIO_VERIFY_CURL_CONNECT_TIMEOUT:-2}"
CURL_MAX_TIME="${WORKORCH_MCP_STDIO_VERIFY_CURL_MAX_TIME:-300}"
CURL_READY_MAX_TIME="${WORKORCH_MCP_STDIO_VERIFY_READY_MAX_TIME:-2}"
CURL_READY_ATTEMPTS="${WORKORCH_MCP_STDIO_VERIFY_READY_ATTEMPTS:-50}"
MCP_SERVER_ID="mcp-stdio-verify-$VERIFY_SUFFIX"
MCP_CAPABILITY_ID="$MCP_SERVER_ID:tool:repo.summary"
MCP_TEMPLATE_ID="mcp-stdio-verify-$VERIFY_SUFFIX"
MCP_VERIFICATION_RESOURCE="workspace://verification-context"
MCP_VERIFICATION_PROMPT="workorch.verification-context"
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
  echo "verification evidence retained at $TMP_DIR (JSON responses and mock log)" >&2
  if [[ -f "$MOCK_LOG" ]]; then
    echo "--- llm-mock.log ---" >&2
    tail -n 200 "$MOCK_LOG" >&2 || true
  fi
}

cleanup() {
  local status=$?
  if [[ -f "$TMP_DIR/original-llm-update.json" ]]; then
    curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" -fsS -H "Content-Type: application/json" -X PUT \
      "$BASE_URL/api/v1/workorch/configuration/llm" \
      -d @"$TMP_DIR/original-llm-update.json" >/dev/null 2>&1 || true
  fi
  if [[ -n "$MOCK_PID" ]]; then
    kill "$MOCK_PID" >/dev/null 2>&1 || true
    wait "$MOCK_PID" >/dev/null 2>&1 || true
  fi
  if [[ $status -ne 0 || "$KEEP_TMP" == "1" || "$KEEP_TMP" == "true" ]]; then
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

  python3 "$ROOT_DIR/scripts/verify_execution_activation.py" "$activation_file" \
    --intent restart --workspace-id "$workspace_id" --request-reason e2e_restart || return 1

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

wait_pending_approval() {
  local workspace_id="$1"
  local run_id="$2"
  local output="$3"
  local approval_id=""
  local run_status_output="${output%.json}-run-status.json"
  local status_url
  local deadline=$((SECONDS + 120))
  status_url="$(run_api_url "$workspace_id" "$run_id" "status")"

  while (( SECONDS <= deadline )); do
    get_json "$BASE_URL/api/v1/workorch/approvals?workspaceId=$workspace_id&runId=$run_id" "$output"
    approval_id="$(python3 - "$output" "$workspace_id" "$run_id" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
workspace_id, run_id = sys.argv[2:]
matches = [
    item for item in (data.get("approvals") or [])
    if item.get("workspaceId") == workspace_id
    and item.get("runId") == run_id
    and item.get("state") == "pending"
    and item.get("id")
]
if len(matches) == 1:
    print(matches[0]["id"])
    raise SystemExit(0)
raise SystemExit(1)
PY
    )" && {
      echo "$approval_id"
      return 0
    }
    if get_json "$status_url" "$run_status_output"; then
      if python3 - "$run_status_output" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
status = data.get("status") or {}
state = status.get("state")
if state not in {"completed", "failed", "cancelled", "superseded"}:
    raise SystemExit(1)
failure = status.get("failureReason") or ""
convergence = status.get("convergenceStatus") or {}
print(
    "Run became terminal before pending approval: "
    f"state={state!r} failure={failure!r} "
    f"runtimeFailureCode={convergence.get('failureCode')!r} "
    f"runtimeFailureSignature={convergence.get('failureSignature')!r}",
    file=sys.stderr,
)
raise SystemExit(0)
PY
      then
        cat "$run_status_output" >&2 || true
        return 1
      fi
    fi
    sleep 0.5
  done

  echo "MCP operation did not materialize one pending approval within 120s for workspace $workspace_id run $run_id" >&2
  cat "$output" >&2 || true
  return 1
}

wait_canonical_approval_blocker() {
  local workspace_id="$1"
  local run_id="$2"
  local approval_id="$3"
  local output="$4"
  local status_url
  status_url="$(run_api_url "$workspace_id" "$run_id" "status")"

  # Approval owns request materialization, while Run owns the lifecycle
  # blocker. They converge through separate typed owner events, so observing a
  # pending Approval is not yet mutation authority. Wait for the exact Run
  # blocker before exercising Application's fail-closed admission gate.
  get_json "$status_url?waitState=blocked_on_approval&waitTimeoutMillis=120000&waitIntervalMillis=500" "$output"
  python3 - "$output" "$workspace_id" "$run_id" "$approval_id" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
status = data.get("status") or {}
workspace_id, run_id, approval_id = sys.argv[2:]
if data.get("waitMatched") is not True:
    raise SystemExit(f"Run blocker wait did not match: {data.get('waitMatched')!r}")
if status.get("id") != run_id or status.get("workspaceId") != workspace_id:
    raise SystemExit(
        f"Run blocker scope is {status.get('workspaceId')!r}/{status.get('id')!r}, "
        f"want {workspace_id!r}/{run_id!r}"
    )
if status.get("state") != "blocked_on_approval":
    raise SystemExit(f"run state is {status.get('state')!r}, want 'blocked_on_approval'")
if status.get("blockingApprovalId") != approval_id:
    raise SystemExit(
        f"Run blocker is {status.get('blockingApprovalId')!r}, want pending approval {approval_id!r}"
    )
if status.get("blockingTicketId"):
    raise SystemExit(f"Run has an unexpected simultaneous ticket blocker: {status.get('blockingTicketId')!r}")
PY
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
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from verify_mcp_stdio_mock import MockContractError, decision_payload, semantic_plan_payload

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        path = self.path.split("?", 1)[0]
        if path in ("/v1/models", "/models"):
            # WorkOrch derives supported operations from ai-proxy endpoints.
            models = [
                {
                    "id": "verify-model",
                    "object": "model",
                    "owned_by": "verify",
                    "supported_endpoints": ["/v1/responses", "/v1/embeddings"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
                {
                    "id": "gpt-4o",
                    "object": "model",
                    "owned_by": "openai",
                    "supported_endpoints": ["/v1/responses"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
                {
                    "id": "text-embedding-3-small",
                    "object": "model",
                    "owned_by": "openai",
                    "supported_endpoints": ["/v1/embeddings"],
                    "contextWindowTokens": 8192,
                    "maxOutputTokens": 1024,
                },

                {
                    "id": "verify-stream-native-model",
                    "object": "model",
                    "owned_by": "verify",
                    "supported_endpoints": ["/v1/responses", "/v1/embeddings"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
                {
                    "id": "verify-stream-estimated-model",
                    "object": "model",
                    "owned_by": "verify",
                    "supported_endpoints": ["/v1/responses", "/v1/embeddings"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
                {
                    "id": "web-search-e2e-model",
                    "object": "model",
                    "owned_by": "verify",
                    "supported_endpoints": ["/v1/responses", "/v1/embeddings"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
                {
                    "id": "container-verify-model",
                    "object": "model",
                    "owned_by": "verify",
                    "supported_endpoints": ["/v1/responses", "/v1/embeddings"],
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
        if self.path == "/v1/embeddings":
            response = {"data": [{"embedding": [0.1, 0.2, 0.3]}]}
            encoded = json.dumps(response).encode("utf-8")
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
        payload = json.loads(self.rfile.read(size))
        user_content = str(payload.get("input", ""))
        try:
            server_id = os.environ.get("WORKORCH_MCP_STDIO_VERIFY_SERVER_ID", "mcp-stdio-verify")
            if "### C3.semantic-authority" in user_content:
                decision = semantic_plan_payload(user_content, server_id)
            else:
                decision = decision_payload(user_content, server_id)
        except MockContractError as error:
            print(f"mock contract rejected request: {error}", flush=True)
            encoded = json.dumps({"error": {"message": f"mock contract: {error}"}}).encode("utf-8")
            self.send_response(422)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)
            return
        content = json.dumps(decision, separators=(",", ":"))
        if payload.get("stream"):
            events = [
                {"type": "response.output_text.delta", "delta": content},
                {
                    "type": "response.completed",
                    "response": {
                        "status": "completed",
                        "output": [],
                        "usage": {"input_tokens": 256, "output_tokens": 128, "total_tokens": 384},
                    },
                },
            ]
            encoded = "".join("data: " + json.dumps(item, separators=(",", ":")) + "\n\n" for item in events).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)
            return
        response = {
            "status": "completed",
            "output_text": content,
            "output": [],
            "usage": {"input_tokens": 256, "output_tokens": 128, "total_tokens": 384},
        }
        encoded = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)

host, port_text = os.environ.get("WORKORCH_MCP_STDIO_VERIFY_MOCK_ADDR", "127.0.0.1:18181").split(":", 1)
ThreadingHTTPServer((host, int(port_text)), Handler).serve_forever()
PY
  PYTHONPATH="$ROOT_DIR/scripts${PYTHONPATH:+:$PYTHONPATH}" WORKORCH_MCP_STDIO_VERIFY_MOCK_ADDR="$MOCK_ADDR" WORKORCH_MCP_STDIO_VERIFY_SERVER_ID="$MCP_SERVER_ID" python3 "$TMP_DIR/llm_mock_provider.py" >>"$MOCK_LOG" 2>&1 &
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
    if curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_READY_MAX_TIME" -fsS "http://$MOCK_ADDR/v1/models" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_READY_MAX_TIME" -fsS "http://$MOCK_ADDR/v1/models" >/dev/null
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

validate_snapshot() {
  python3 - "$1" "$MCP_CAPABILITY_ID" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8"))["snapshot"]
capability_id = sys.argv[2]
run = snapshot["run"]
task_progress = snapshot.get("taskProgress") or {}
runtime = task_progress.get("runtime") or {}
dod_evidence = snapshot.get("doDEvidence") or []
selected_capabilities = snapshot.get("selectedCapabilities") or []
used_capabilities = snapshot.get("usedCapabilities") or []
audit = (snapshot.get("acceptance") or {}).get("audit") or {}

mcp_operation_evidence_refs = set()
for dispatch in audit.get("dispatches") or []:
    dispatch_id = str(dispatch.get("id") or "").strip()
    if not dispatch_id:
        continue
    for operation in dispatch.get("operations") or []:
        operation_id = str(operation.get("id") or "").strip()
        if operation.get("kind") == "mcp/tool_call" and operation_id:
            mcp_operation_evidence_refs.add(
                f"workorch://runtime-operations/{dispatch_id}/{operation_id}"
            )

task_progress_evidence_refs = set(task_progress.get("evidenceRefs") or [])
used_capability_evidence_refs = {
    str(ref)
    for item in used_capabilities
    if item.get("id") == capability_id
    for ref in (item.get("evidenceRefs") or [])
}

errors = []
if run.get("state") != "completed":
    errors.append(f"run state is {run.get('state')!r}, want completed: {run.get('failureReason', '')}")
if not mcp_operation_evidence_refs:
    errors.append(f"Runtime audit has no sealed MCP operation identity: {audit.get('dispatches')}")
elif not task_progress_evidence_refs.intersection(mcp_operation_evidence_refs):
    errors.append(
        "task progress missing dynamically owner-derived MCP operation evidence: "
        f"progress={sorted(task_progress_evidence_refs)} operations={sorted(mcp_operation_evidence_refs)}"
    )
if not any(item.get("satisfied") is True and any((entry.get("evidenceStatus") == "linked") for entry in (item.get("items") or [])) for item in dod_evidence):
    errors.append(f"missing linked Runtime DoD assessment evidence: {dod_evidence}")
if not any(item.get("id") == capability_id and item.get("displayName") == "仓库摘要能力 E2E" for item in selected_capabilities):
    errors.append(f"missing selected MCP capability metadata: {selected_capabilities}")
if not any(item.get("id") == capability_id and item.get("useCount", 0) >= 1 for item in used_capabilities) or not used_capability_evidence_refs.intersection(mcp_operation_evidence_refs):
    errors.append(f"missing used MCP capability evidence: {used_capabilities}")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)

print(f"runId={run['id']}")
print(f"runState={run['state']}")
print(f"completedDispatches={runtime.get('completedDispatchCount', 0)}")
print(f"evidenceSequence={runtime.get('evidenceSequence', 0)}")
print(f"usedCapabilities={len(used_capabilities)}")
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
grep -E '"status"[[:space:]]*:[[:space:]]*"ready"' "$TMP_DIR/readyz.json" >/dev/null
get_json "$BASE_URL/api/v1/workorch/configuration" "$TMP_DIR/configuration.json"
write_llm_update_payloads
post_json "$BASE_URL/api/v1/workorch/configuration/llm/models/refresh" '{}' "$TMP_DIR/catalog-refreshed.json"
put_json_file "$BASE_URL/api/v1/workorch/configuration/llm" \
  "$TMP_DIR/verify-llm-update.json" \
  "$TMP_DIR/configuration-updated.json"

upsert_json "$BASE_URL/api/v1/workorch/mcp/servers" \
  "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID" \
  '{"id":"'"$MCP_SERVER_ID"'","name":"stdio verification","transport":"stdio","endpoint":"'"$MCP_HELPER"'","enabled":true,"allowedTools":["repo.summary"],"allowedResources":["'"$MCP_VERIFICATION_RESOURCE"'"],"allowedPrompts":["'"$MCP_VERIFICATION_PROMPT"'"],"timeoutMillis":5000}' \
  "$TMP_DIR/mcp-server.json"
grep '"transport": "stdio"' "$TMP_DIR/mcp-server.json" >/dev/null
get_json "$BASE_URL/api/v1/workorch/mcp/capabilities?serverId=$MCP_SERVER_ID&kind=tool&q=repo.summary" "$TMP_DIR/mcp-capabilities-derived.json"
grep "\"id\": \"$MCP_CAPABILITY_ID\"" "$TMP_DIR/mcp-capabilities-derived.json" >/dev/null
get_json "$BASE_URL/api/v1/workorch/mcp/capabilities?serverId=$MCP_SERVER_ID&kind=resource&q=verification" "$TMP_DIR/mcp-resource-capabilities-derived.json"
grep '"displayName": "资源 workspace://verification-context"' "$TMP_DIR/mcp-resource-capabilities-derived.json" >/dev/null
get_json "$BASE_URL/api/v1/workorch/mcp/capabilities?serverId=$MCP_SERVER_ID&kind=prompt&q=verification" "$TMP_DIR/mcp-prompt-capabilities-derived.json"
grep '"displayName": "提示词 workorch.verification-context"' "$TMP_DIR/mcp-prompt-capabilities-derived.json" >/dev/null
put_json "$BASE_URL/api/v1/workorch/mcp/capabilities/$MCP_CAPABILITY_ID" \
  '{"id":"'"$MCP_CAPABILITY_ID"'","serverId":"'"$MCP_SERVER_ID"'","kind":"tool","name":"repo.summary","displayName":"仓库摘要能力 E2E","description":"通过 MCP stdio 读取仓库摘要，用于能力目录、Workspace 绑定和 Run 证据验收。","category":"代码仓库","tags":["e2e","stdio","repo"],"usageHint":"当 Goal 需要仓库上下文摘要时使用。","inputSummary":"workspaceId、runId、stepId 和工具输入。","outputSummary":"返回仓库摘要 structuredContent。","stageHints":["execution","validation"],"riskLevel":"read_only","effectScope":"none","idempotencyClass":"safe_to_replay","validationProofApplicability":"direct","schema":{"type":"object","properties":{},"required":[],"additionalProperties":false},"enabled":true,"approvalPolicy":"operator"}' \
  "$TMP_DIR/mcp-capability-updated.json"
grep '"displayName": "仓库摘要能力 E2E"' "$TMP_DIR/mcp-capability-updated.json" >/dev/null
grep '"effectScope": "none"' "$TMP_DIR/mcp-capability-updated.json" >/dev/null
grep '"idempotencyClass": "safe_to_replay"' "$TMP_DIR/mcp-capability-updated.json" >/dev/null
grep '"validationProofApplicability": "direct"' "$TMP_DIR/mcp-capability-updated.json" >/dev/null
get_json "$BASE_URL/api/v1/workorch/mcp/capabilities?category=%E4%BB%A3%E7%A0%81%E4%BB%93%E5%BA%93&stage=execution&riskLevel=read_only&enabled=true&q=stdio" "$TMP_DIR/mcp-capabilities-filtered.json"
grep "\"id\": \"$MCP_CAPABILITY_ID\"" "$TMP_DIR/mcp-capabilities-filtered.json" >/dev/null

post_json "$BASE_URL/api/v1/workorch/workspace-templates" \
  '{"id":"'"$MCP_TEMPLATE_ID"'","name":"MCP stdio verification","description":"Runtime MCP stdio validation profile","goalText":"Plan and execute one MCP stdio repo.summary tool call, then assess the DoD from the tool evidence.","doDItems":["stdio MCP tool call completes"],"defaultSoulId":"soul-generic-governed-execution","soulBindings":{"planning":"soul-generic-governed-planning","execution":"soul-generic-governed-execution","validation":"soul-generic-governed-validation","decision":"soul-generic-governed-decision","approval":"soul-generic-governed-approval","artifact":"soul-generic-governed-artifact"},"contextHints":{"semanticCodeNavigation":{"defaultMCPServerIds":["'"$MCP_SERVER_ID"'"],"defaultCapabilityIds":["'"$MCP_CAPABILITY_ID"'"]},"acceptanceSlicePolicy":{"schemaVersion":"workorch.acceptance_slice_policy","maxWorkItems":1,"maxFocuses":2,"maxCandidateContextItems":16,"completionPolicy":"all_required_items_accepted","resultPolicy":"slice_result_allowed","groups":[{"id":"mcp-stdio-e2e","title":"MCP stdio acceptance","scope":"mcp_stdio","priority":10,"maxWorkItems":1,"match":{"all":true}}]},"profileContracts":{"mcp-stdio-e2e":{"evidenceRequirements":[{"id":"*","scope":"dodItem","evidenceRole":"supplemental_evidence","targetRole":"mcp_capability","targetSelectorRef":"mcp-capability-selector","targetSelectorOwnerRef":"blocks/mcp","targetSelectorSubjectRef":"'"$MCP_CAPABILITY_ID"'","targetDiscoverySources":["available_capabilities"],"requireTargetGrounding":false,"requiredEvidenceKinds":["validation"],"requireDirectEvidence":true,"requireExecutableBehaviorEvidence":true,"acceptedStepKinds":["mcp/tool_call"],"validationRecipeId":"mcp/tool_call/'"$MCP_SERVER_ID"'/repo.summary","requiredCheckRefs":["mcp/tool_call/'"$MCP_SERVER_ID"'/repo.summary"]}]}}}}' \
  "$TMP_DIR/mcp-template.json"

post_json "$BASE_URL/api/v1/workorch/workspaces" \
  '{"title":"mcp stdio verification","description":"verify MCP stdio transport through Runtime candidate execution","templateId":"'"$MCP_TEMPLATE_ID"'"}' \
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
  '{"intent":"restart","requestId":"mcp-stdio-e2e-'"$VERIFY_SUFFIX"'","reason":"e2e_restart","source":"e2e"}' \
  "$TMP_DIR/activation.json"
run_id="$(wait_activation_run "$workspace_id" "$TMP_DIR/activation.json" "$TMP_DIR/activation-before-runs.json" "$TMP_DIR/activation-runs.json" "$TMP_DIR/activation-run.json")"
[[ -n "$run_id" ]]

approval_id="$(wait_pending_approval "$workspace_id" "$run_id" "$TMP_DIR/approvals-pending.json")"
[[ -n "$approval_id" ]]
wait_canonical_approval_blocker "$workspace_id" "$run_id" "$approval_id" "$TMP_DIR/run-blocked-on-approval.json"
post_json "$BASE_URL/api/v1/workorch/approvals/$approval_id/approve?workspaceId=$workspace_id" \
  '{}' \
  "$TMP_DIR/approval-approved.json"
python3 - "$TMP_DIR/approval-approved.json" "$approval_id" "$workspace_id" "$run_id" <<'PY'
import json
import sys

approval = json.load(open(sys.argv[1], encoding="utf-8")).get("approval") or {}
expected_id, workspace_id, run_id = sys.argv[2:]
if (
    approval.get("id") != expected_id
    or approval.get("workspaceId") != workspace_id
    or approval.get("runId") != run_id
    or approval.get("state") != "approved"
):
    raise SystemExit(f"approval response does not match the pending MCP dispatch: {approval}")
PY

wait_run_state "$workspace_id" "$run_id" "completed" "$TMP_DIR/run-status.json"
get_json "$(run_api_url "$workspace_id" "$run_id" "snapshot")" "$TMP_DIR/snapshot.json"
get_json "$(run_api_url "$workspace_id" "$run_id" "observability")" "$TMP_DIR/observability.json"
grep '"selectedCapabilities"' "$TMP_DIR/observability.json" >/dev/null
grep '"usedCapabilities"' "$TMP_DIR/observability.json" >/dev/null
echo "workspaceId=$workspace_id"
echo "runId=$run_id"
validate_snapshot "$TMP_DIR/snapshot.json"
# The C12 verifier is the semantic completion gate.  Dispatch/evidence counts
# are intentionally not used as a proxy: an MCP validation profile can require
# one binding-matched tool receipt plus one assessment dispatch, while another
# profile may legitimately require a different number of sealed dispatches.
# The verifier proves the immutable CWI -> binding -> sealed operation -> owner
# receipt -> SM-2 evidence chain instead.
python3 "$ROOT_DIR/scripts/verify_runtime_live_contract.py" \
  --snapshot "$TMP_DIR/snapshot.json" \
  --workspace-id "$workspace_id" \
  --run-id "$run_id" \
  --require-terminal

echo "workorch MCP stdio verification passed for deployed workorch service $BASE_URL"
