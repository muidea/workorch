#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="${WORKORCH_GOVERNANCE_RESOLUTION_VERIFY_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_TMP="${WORKORCH_GOVERNANCE_RESOLUTION_VERIFY_KEEP_ARTIFACTS:-0}"
BASE_URL="${WORKORCH_GOVERNANCE_RESOLUTION_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
VERIFY_SUFFIX="${WORKORCH_GOVERNANCE_RESOLUTION_VERIFY_SUFFIX:-$$}"
SCRIPT_TIMEOUT_SECONDS="${WORKORCH_GOVERNANCE_RESOLUTION_VERIFY_TIMEOUT_SECONDS:-180}"
RUN_WAIT_SECONDS="${WORKORCH_GOVERNANCE_RESOLUTION_VERIFY_RUN_WAIT_SECONDS:-90}"
CURL_CONNECT_TIMEOUT="${WORKORCH_VERIFY_CURL_CONNECT_TIMEOUT_SECONDS:-5}"
CURL_MAX_TIME="${WORKORCH_VERIFY_CURL_MAX_TIME_SECONDS:-30}"
PROVIDER_ID="verify-governance-resolution-$VERIFY_SUFFIX"
MOCK_ADDR="${WORKORCH_GOVERNANCE_RESOLUTION_VERIFY_MOCK_ADDR:-}"
MOCK_BASE_URL=""
MOCK_PID=""
WATCHDOG_PID=""
WORKSPACE_ID=""
WORKSPACE_IDS=()
MOCK_LOG="$TMP_DIR/llm-mock.log"

select_addr() {
  if [[ -n "$MOCK_ADDR" ]]; then
    printf '%s\n' "$MOCK_ADDR"
    return
  fi
  python3 - <<'PY'
import socket

with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
    sock.bind(("127.0.0.1", 0))
    print(f"127.0.0.1:{sock.getsockname()[1]}")
PY
}

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

start_watchdog() {
  if ! [[ "$SCRIPT_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || [[ "$SCRIPT_TIMEOUT_SECONDS" -le 0 ]]; then
    return
  fi
  local parent_pid="$$"
  (
    sleep "$SCRIPT_TIMEOUT_SECONDS"
    echo "approval/decision resolution e2e exceeded ${SCRIPT_TIMEOUT_SECONDS}s; terminating" >&2
    kill -TERM "$parent_pid" >/dev/null 2>&1 || true
  ) &
  WATCHDOG_PID=$!
}

delete_workspace() {
  local workspace_id="$1"
  if [[ -z "$workspace_id" ]]; then
    return
  fi
  curl -fsS -X DELETE "$BASE_URL/api/v1/workorch/workspaces/$workspace_id?purge=true" >/dev/null 2>&1 || true
}

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "approval/decision resolution e2e failed for workorch service $BASE_URL" >&2
  echo "artifacts=$TMP_DIR" >&2
  if [[ -f "$MOCK_LOG" ]]; then
    echo "--- llm-mock.log ---" >&2
    tail -n 240 "$MOCK_LOG" >&2 || true
  fi
  for file in "$TMP_DIR"/*.json; do
    [[ -f "$file" ]] || continue
    echo "--- $file ---" >&2
    python3 -m json.tool "$file" >&2 || cat "$file" >&2 || true
  done
}

cleanup() {
  local status=$?
  if [[ -n "$WATCHDOG_PID" ]]; then
    kill "$WATCHDOG_PID" >/dev/null 2>&1 || true
    wait "$WATCHDOG_PID" >/dev/null 2>&1 || true
  fi
  if [[ -f "$TMP_DIR/original-llm-update.json" ]]; then
    curl -fsS -H "Content-Type: application/json" -X PUT \
      "$BASE_URL/api/v1/workorch/configuration/llm" \
      -d @"$TMP_DIR/original-llm-update.json" >/dev/null 2>&1 || true
  fi
  for workspace_id in "${WORKSPACE_IDS[@]:-}"; do
    delete_workspace "$workspace_id"
  done
  if [[ -n "$MOCK_PID" ]]; then
    kill "$MOCK_PID" >/dev/null 2>&1 || true
    wait "$MOCK_PID" >/dev/null 2>&1 || true
  fi
  if [[ "$KEEP_TMP" == "1" || "$KEEP_TMP" == "true" || $status -ne 0 ]]; then
    echo "keeping approval/decision resolution temp dir: $TMP_DIR" >&2
  else
    rm -rf "$TMP_DIR"
  fi
}

post_json() {
  local path="$1"
  local body="$2"
  local output="$3"
  local status_file="$output.status"
  local http_status
  set +e
  command curl --connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME" \
    -sS -X POST "$BASE_URL$path" \
    -H 'Content-Type: application/json' \
    -d "$body" \
    -w '%{http_code}' \
    -o "$output" >"$status_file"
  local curl_status=$?
  set -e
  http_status="$(tr -d '\r\n' <"$status_file" 2>/dev/null || true)"
  if [[ $curl_status -ne 0 ]]; then
    echo "POST $BASE_URL$path failed curl_status=$curl_status http_status=${http_status:-unknown}" >&2
    return $curl_status
  fi
  if [[ ! "$http_status" =~ ^2[0-9][0-9]$ ]]; then
    echo "POST $BASE_URL$path failed HTTP $http_status" >&2
    if [[ -s "$output" ]]; then
      echo "--- response body: $output ---" >&2
      python3 -m json.tool "$output" >&2 || sed -n '1,200p' "$output" >&2 || true
    fi
    return 1
  fi
}

wait_ready() {
  local deadline=$((SECONDS + 30))
  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1 &&
      curl -fsS "$BASE_URL/readyz" >"$TMP_DIR/readyz.json" 2>/dev/null; then
      grep '"status"[[:space:]]*:[[:space:]]*"ready"' "$TMP_DIR/readyz.json" >/dev/null
      return
    fi
    sleep 0.3
  done
  echo "timed out waiting for workorch service at $BASE_URL" >&2
  return 1
}

start_mock_provider() {
  MOCK_ADDR="$(select_addr)"
  MOCK_BASE_URL="http://$MOCK_ADDR/v1"
  python3 - "$MOCK_ADDR" >"$TMP_DIR/llm_mock_provider.py" <<'PY'
import sys

mock_addr = sys.argv[1]
source = r'''
import json
import os
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

def merged_content(payload):
    return "\n".join(str(item.get("content", "")) for item in (payload.get("messages") or []))

def last_match(pattern, text):
    matches = re.findall(pattern, text)
    return matches[-1] if matches else ""

def provider_request(payload):
    merged = merged_content(payload)
    workspace_id = last_match(r'"workspaceId"\s*:\s*"([^"]+)"', merged)
    run_id = last_match(r'"runId"\s*:\s*"([^"]+)"', merged)
    step_id = last_match(r'"stepId"\s*:\s*"([^"]+)"', merged)
    return {
        "workspaceId": workspace_id,
        "runId": run_id,
        "stepId": step_id,
    }

def completed_payload(step_id, label):
    evidence = [step_id or label]
    labels = [
        "decision resolution continuation completes",
        "approval resolution continuation completes",
        "governance resolution successor completes",
    ]
    return {
        "thoughtSummary": "Governance resolution e2e completed after the scoped owner state changed.",
        "completionState": "completed",
        "dodAssessment": {
            "satisfied": True,
            "summary": "Governance resolution continuation completed.",
            "items": [
                {
                    "item": item,
                    "state": "satisfied",
                    "evidenceRefs": evidence,
                }
                for item in labels
            ],
            "goalCoverage": {
                "satisfied": True,
                "items": [
                    {
                        "requirement": "verify governance resolution continues execution",
                        "state": "satisfied",
                        "evidenceRefs": evidence,
                    }
                ],
            },
        },
    }

def decision_payload(request):
    run_id = request.get("runId", "")
    step_id = request.get("stepId", "") or f"{run_id}/step-1"
    if run_id == "probe":
        return {"completionState": "failed", "thoughtSummary": "probe"}
    if "llm-after-decision" in step_id:
        return completed_payload(step_id, "decision resolution continuation completes")
    if "llm-after-approval" in step_id:
        return completed_payload(step_id, "approval resolution continuation completes")
    if "decision-replan" in run_id:
        return {
            "thoughtSummary": "This run requires a replanning decision.",
            "completionState": "blocked_on_decision",
            "requiredDecision": {
                "reason": "Replan continuation must be verified.",
                "decisionKind": "provide_missing_info",
                "question": "Choose whether to replan the current path.",
                "context": {"runId": run_id},
            },
        }
    if "decision-block" in run_id:
        return {
            "thoughtSummary": "This run requires a decision.",
            "completionState": "blocked_on_decision",
            "requiredDecision": {
                "reason": "Decision continuation must be verified.",
                "decisionKind": "provide_missing_info",
                "question": "Provide the missing decision input.",
                "context": {"runId": run_id},
            },
        }
    if "approval-block" in run_id:
        return {
            "thoughtSummary": "This run requires approval.",
            "completionState": "blocked_on_approval",
            "requiredApproval": {
                "reason": "Approval continuation must be verified.",
                "action": "approve governance resolution path",
                "context": {"runId": run_id},
            },
        }
    return completed_payload(step_id, "governance resolution successor completes")

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
        payload = json.loads(self.rfile.read(size) or b"{}")
        request = provider_request(payload)
        decision = decision_payload(request)
        print("runId=%s stepId=%s state=%s" % (
            request.get("runId", ""),
            request.get("stepId", ""),
            decision.get("completionState", ""),
        ), flush=True)
        content = json.dumps(decision, separators=(",", ":"))
        response = {
            "choices": [{"message": {"content": content}}],
            "usage": {"prompt_tokens": 96, "completion_tokens": 32, "total_tokens": 128},
        }
        encoded = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)

host, port_text = os.environ["WORKORCH_GOVERNANCE_RESOLUTION_VERIFY_MOCK_ADDR"].split(":", 1)
ThreadingHTTPServer((host, int(port_text)), Handler).serve_forever()
'''
open(sys.stdout.fileno(), "w", encoding="utf-8", closefd=False).write(source)
PY
  WORKORCH_GOVERNANCE_RESOLUTION_VERIFY_MOCK_ADDR="$MOCK_ADDR" python3 "$TMP_DIR/llm_mock_provider.py" >>"$MOCK_LOG" 2>&1 &
  MOCK_PID=$!
}

wait_mock() {
  for _ in $(seq 1 50); do
    if curl -fsS -X POST "$MOCK_BASE_URL/chat/completions" \
      -H 'Content-Type: application/json' \
      -d '{"messages":[{"role":"user","content":"{\"runId\":\"probe\",\"stepId\":\"probe\"}"}]}' >/dev/null 2>&1; then
      return
    fi
    sleep 0.2
  done
  curl -fsS -X POST "$MOCK_BASE_URL/chat/completions" \
    -H 'Content-Type: application/json' \
    -d '{"messages":[{"role":"user","content":"{\"runId\":\"probe\",\"stepId\":\"probe\"}"}]}' >/dev/null
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
        "providerID": default_chat.get("providerId") or default_chat.get("providerID") or provider_id,
        "modelID": default_chat.get("modelId") or default_chat.get("modelID") or "verify-model",
    },
    "budget": {
        "contextUtilizationRatio": float(budget.get("contextUtilizationRatio") or 0.75),
        "estimatedTokensPerByte": float(budget.get("estimatedTokensPerByte") or 0.25),
        "defaultMaxOutputTokens": int(budget.get("defaultMaxOutputTokens") or 4096),
        "maxRequestBytes": int(budget.get("maxRequestBytes") or 4194304),
    },
    "customProviders": custom_providers_payload(custom_providers, include_api_key=False, mock_base_url=mock_base_url),
}
if default_embedding.get("modelId") or default_embedding.get("modelID"):
    original["defaultEmbedding"] = {
        "providerID": default_embedding.get("providerId") or default_embedding.get("providerID") or provider_id,
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

configure_mock_llm() {
  curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
  write_llm_update_payloads
  curl -fsS -X PUT "$BASE_URL/api/v1/workorch/configuration/llm" -H 'Content-Type: application/json' \
    -d @"$TMP_DIR/verify-llm-update.json" >"$TMP_DIR/configuration-updated.json"
  grep '"apiKeyConfigured": true' "$TMP_DIR/configuration-updated.json" >/dev/null
}

create_workspace_contract() {
  local label="$1"
  local workspace_dir="$TMP_DIR/deliverable"
  mkdir -p "$workspace_dir"
  post_json "/api/v1/workorch/workspaces" \
    '{"title":"approval decision resolution e2e '"$label"'","description":"verify governance resolution continuation","deliverableDirectory":"'"$workspace_dir/$label"'","templateId":"generic-governed"}' \
    "$TMP_DIR/workspace-$label.json"
  WORKSPACE_ID="$(json_path "$TMP_DIR/workspace-$label.json" "workspace.id")"
  [[ -n "$WORKSPACE_ID" ]]
  WORKSPACE_IDS+=("$WORKSPACE_ID")
  post_json "/api/v1/workorch/workspaces/$WORKSPACE_ID/goals" \
    '{"text":"verify governance resolution continues execution"}' \
    "$TMP_DIR/goal-$label.json"
  post_json "/api/v1/workorch/workspaces/$WORKSPACE_ID/dods" \
    '{"items":["decision resolution continuation completes","approval resolution continuation completes","governance resolution successor completes"]}' \
    "$TMP_DIR/dod-$label.json"
  GOAL_ID="$(json_path "$TMP_DIR/goal-$label.json" "goal.id")"
  DOD_ID="$(json_path "$TMP_DIR/dod-$label.json" "dod.id")"
}

create_run() {
  local run_id="$1"
  local output="$2"
  post_json "/api/v1/workorch/workspaces/$WORKSPACE_ID/runs" \
    '{"runId":"'"$run_id"'","goalId":"'"$GOAL_ID"'","doDId":"'"$DOD_ID"'"}' \
    "$output"
}

start_run() {
  local run_id="$1"
  local output="$2"
  post_json "/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$run_id/start" '{}' "$output"
}

wait_run_state() {
  local run_id="$1"
  local expected_state="$2"
  local output="$3"
  local deadline=$((SECONDS + RUN_WAIT_SECONDS))
  local state=""
  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$run_id/status" >"$output"
    state="$(python3 - "$output" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
print((data.get("status") or {}).get("state") or "")
PY
)"
    if [[ "$state" == "$expected_state" ]]; then
      return
    fi
    sleep 0.5
  done
  echo "timed out waiting for run=$run_id state=$expected_state last=$state" >&2
  cat "$output" >&2 || true
  return 1
}

wait_mock_log_contains() {
  local pattern="$1"
  local deadline=$((SECONDS + RUN_WAIT_SECONDS))
  while (( SECONDS <= deadline )); do
    if [[ -f "$MOCK_LOG" ]] && grep -F "$pattern" "$MOCK_LOG" >/dev/null; then
      return
    fi
    sleep 0.3
  done
  echo "timed out waiting for mock log pattern: $pattern" >&2
  [[ -f "$MOCK_LOG" ]] && cat "$MOCK_LOG" >&2 || true
  return 1
}

wait_run_blocked() {
  local run_id="$1"
  local blocker="$2"
  local output="$3"
  local deadline=$((SECONDS + RUN_WAIT_SECONDS))
  local state=""
  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$run_id/status" >"$output"
    state="$(python3 - "$output" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
print((data.get("status") or {}).get("state") or "")
PY
)"
    if [[ "$state" == "$blocker" ]]; then
      return
    fi
    sleep 0.5
  done
  echo "timed out waiting for run=$run_id blocker=$blocker last=$state" >&2
  cat "$output" >&2 || true
  return 1
}

blocking_id() {
  local path="$1"
  local kind="$2"
  python3 - "$path" "$kind" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
status = data.get("status") or {}
kind = sys.argv[2]
key = "blockingTicketId" if kind == "decision" else "blockingApprovalId"
print(status.get(key) or "")
PY
}

validate_governance_get_and_list() {
  local kind="$1"
  local id="$2"
  local run_id="$3"
  local expected_state="$4"
  local plural path_prefix
  if [[ "$kind" == "decision" ]]; then
    plural="decisions"
  else
    plural="approvals"
  fi
  path_prefix="/api/v1/workorch/$plural"
  curl -fsS "$BASE_URL$path_prefix/$id?workspaceId=$WORKSPACE_ID" >"$TMP_DIR/$kind-$run_id-get.json"
  curl -fsS "$BASE_URL$path_prefix?workspaceId=$WORKSPACE_ID&runId=$run_id" >"$TMP_DIR/$kind-$run_id-list.json"
  python3 - "$TMP_DIR/$kind-$run_id-get.json" "$TMP_DIR/$kind-$run_id-list.json" "$kind" "$id" "$expected_state" <<'PY'
import json
import sys

get_path, list_path, kind, expected_id, expected_state = sys.argv[1:]
single = json.load(open(get_path, encoding="utf-8")).get(kind) or {}
if single.get("id") != expected_id or single.get("state") != expected_state:
    raise SystemExit(f"{kind} get = {single}, want id={expected_id} state={expected_state}")
plural = "decisions" if kind == "decision" else "approvals"
items = json.load(open(list_path, encoding="utf-8")).get(plural) or []
if not any(item.get("id") == expected_id for item in items):
    raise SystemExit(f"{kind} list missing {expected_id}: {items}")
PY
}

resolve_decision() {
  local ticket_id="$1"
  local resolution_type="$2"
  local output="$3"
  post_json "/api/v1/workorch/decisions/$ticket_id/resolve?workspaceId=$WORKSPACE_ID&priority=2" \
    '{"resolutionType":"'"$resolution_type"'","resolutionPayload":{"source":"e2e","content":"verify governance decision resolution"}}' \
    "$output"
}

approve_approval() {
  local approval_id="$1"
  local output="$2"
  post_json "/api/v1/workorch/approvals/$approval_id/approve?workspaceId=$WORKSPACE_ID&priority=2" \
    '{"payload":{"rawInput":"approve governance resolution continuation","source":"e2e"}}' \
    "$output"
}

wait_replan_projection() {
  local decision_id="$1"
  local output="$2"
  local deadline=$((SECONDS + 30))
  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/snapshot" >"$output"; then
      if python3 - "$output" "$decision_id" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
decision_id = sys.argv[2]
snapshot = data.get("snapshot") or {}
command = snapshot.get("runCommand") or {}
if command.get("intent") != "replan":
    raise SystemExit(1)
if command.get("decisionId") != decision_id:
    raise SystemExit(1)
if int(command.get("revision") or 0) <= 0:
    raise SystemExit(1)
PY
      then
        return
      fi
    fi
    sleep 0.3
  done
  echo "timed out waiting for replan projection decision=$decision_id" >&2
  return 1
}

validate_mock_log() {
  python3 - "$MOCK_LOG" <<'PY'
import pathlib
import sys

text = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8", errors="replace")
required = [
    "run-governance-decision-block",
    "run-governance-decision-replan",
    "run-governance-approval-block",
    "llm-after-decision",
    "llm-after-approval",
]
missing = [item for item in required if item not in text]
if missing:
    raise SystemExit(f"mock log missing {missing}: {text}")
PY
}

write_summary() {
  python3 - "$TMP_DIR/summary.json" "$BASE_URL" "$DECISION_WORKSPACE_ID" "$REPLAN_WORKSPACE_ID" "$APPROVAL_WORKSPACE_ID" "$DECISION_RUN_ID" "$DECISION_ID" "$REPLAN_RUN_ID" "$REPLAN_DECISION_ID" "$TMP_DIR/replan-workspace-snapshot.json" "$APPROVAL_RUN_ID" "$APPROVAL_ID" <<'PY'
import json
import sys

path, base_url, decision_workspace_id, replan_workspace_id, approval_workspace_id, decision_run_id, decision_id, replan_run_id, replan_decision_id, replan_snapshot_path, approval_run_id, approval_id = sys.argv[1:]
snapshot = json.load(open(replan_snapshot_path, encoding="utf-8")).get("snapshot") or {}
command = snapshot.get("runCommand") or {}
payload = {
    "service": "workorch",
    "baseUrl": base_url,
    "decision": {
        "workspaceId": decision_workspace_id,
        "runId": decision_run_id,
        "ticketId": decision_id,
        "resolutionType": "provide_missing_info",
        "continuationStepObserved": True,
    },
    "decisionReplan": {
        "workspaceId": replan_workspace_id,
        "runId": replan_run_id,
        "ticketId": replan_decision_id,
        "resolutionType": "replan",
        "commandObserved": True,
        "commandIntent": command.get("intent"),
        "commandRevision": command.get("revision"),
        "commandDecisionId": command.get("decisionId"),
    },
    "approval": {
        "workspaceId": approval_workspace_id,
        "runId": approval_run_id,
        "approvalId": approval_id,
        "action": "approve",
        "continuationStepObserved": True,
    },
    "pass": True,
}
with open(path, "w", encoding="utf-8") as handle:
    json.dump(payload, handle, ensure_ascii=False, indent=2, sort_keys=True)
    handle.write("\n")
PY
}

trap report_failure ERR
trap cleanup EXIT
mkdir -p "$TMP_DIR"
cd "$ROOT_DIR"
start_watchdog
start_mock_provider
wait_mock
wait_ready
configure_mock_llm

create_workspace_contract "decision"
DECISION_WORKSPACE_ID="$WORKSPACE_ID"
DECISION_RUN_ID="run-governance-decision-block-$VERIFY_SUFFIX"
create_run "$DECISION_RUN_ID" "$TMP_DIR/decision-run-create.json"
start_run "$DECISION_RUN_ID" "$TMP_DIR/decision-run-start.json"
wait_run_blocked "$DECISION_RUN_ID" "blocked_on_decision" "$TMP_DIR/decision-run-status-blocked.json"
DECISION_ID="$(blocking_id "$TMP_DIR/decision-run-status-blocked.json" "decision")"
[[ -n "$DECISION_ID" ]]
validate_governance_get_and_list "decision" "$DECISION_ID" "$DECISION_RUN_ID" "open"
resolve_decision "$DECISION_ID" "provide_missing_info" "$TMP_DIR/decision-resolve.json"
validate_governance_get_and_list "decision" "$DECISION_ID" "$DECISION_RUN_ID" "resolved"
wait_mock_log_contains "runId=$DECISION_RUN_ID stepId=$DECISION_RUN_ID/llm-after-decision state=completed"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$DECISION_RUN_ID/status" >"$TMP_DIR/decision-run-status-after-continuation.json"

create_workspace_contract "replan"
REPLAN_WORKSPACE_ID="$WORKSPACE_ID"
REPLAN_RUN_ID="run-governance-decision-replan-$VERIFY_SUFFIX"
create_run "$REPLAN_RUN_ID" "$TMP_DIR/replan-run-create.json"
start_run "$REPLAN_RUN_ID" "$TMP_DIR/replan-run-start.json"
wait_run_blocked "$REPLAN_RUN_ID" "blocked_on_decision" "$TMP_DIR/replan-run-status-blocked.json"
REPLAN_DECISION_ID="$(blocking_id "$TMP_DIR/replan-run-status-blocked.json" "decision")"
[[ -n "$REPLAN_DECISION_ID" ]]
validate_governance_get_and_list "decision" "$REPLAN_DECISION_ID" "$REPLAN_RUN_ID" "open"
resolve_decision "$REPLAN_DECISION_ID" "replan" "$TMP_DIR/replan-resolve.json"
validate_governance_get_and_list "decision" "$REPLAN_DECISION_ID" "$REPLAN_RUN_ID" "resolved"
wait_replan_projection "$REPLAN_DECISION_ID" "$TMP_DIR/replan-workspace-snapshot.json"

create_workspace_contract "approval"
APPROVAL_WORKSPACE_ID="$WORKSPACE_ID"
APPROVAL_RUN_ID="run-governance-approval-block-$VERIFY_SUFFIX"
create_run "$APPROVAL_RUN_ID" "$TMP_DIR/approval-run-create.json"
start_run "$APPROVAL_RUN_ID" "$TMP_DIR/approval-run-start.json"
wait_run_blocked "$APPROVAL_RUN_ID" "blocked_on_approval" "$TMP_DIR/approval-run-status-blocked.json"
APPROVAL_ID="$(blocking_id "$TMP_DIR/approval-run-status-blocked.json" "approval")"
[[ -n "$APPROVAL_ID" ]]
validate_governance_get_and_list "approval" "$APPROVAL_ID" "$APPROVAL_RUN_ID" "pending"
approve_approval "$APPROVAL_ID" "$TMP_DIR/approval-approve.json"
validate_governance_get_and_list "approval" "$APPROVAL_ID" "$APPROVAL_RUN_ID" "approved"
wait_mock_log_contains "runId=$APPROVAL_RUN_ID stepId=$APPROVAL_RUN_ID/llm-after-approval state=completed"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$APPROVAL_RUN_ID/status" >"$TMP_DIR/approval-run-status-after-continuation.json"

validate_mock_log
write_summary

echo "approval/decision resolution e2e passed for $BASE_URL decision=$DECISION_ID replanDecision=$REPLAN_DECISION_ID approval=$APPROVAL_ID"
