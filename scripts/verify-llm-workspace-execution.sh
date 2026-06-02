#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
BASE_URL="${WORKORCH_LLM_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
MOCK_ADDR="${WORKORCH_LLM_VERIFY_MOCK_ADDR:-127.0.0.1:18181}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_LLM_VERIFY_SUFFIX:-$$}"
CURL_CONNECT_TIMEOUT="${WORKORCH_VERIFY_CURL_CONNECT_TIMEOUT_SECONDS:-5}"
CURL_MAX_TIME="${WORKORCH_VERIFY_CURL_MAX_TIME_SECONDS:-30}"
SCRIPT_TIMEOUT_SECONDS="${WORKORCH_LLM_VERIFY_TIMEOUT_SECONDS:-900}"
RUN_WAIT_SECONDS="${WORKORCH_LLM_VERIFY_RUN_WAIT_SECONDS:-45}"
MCP_SERVER_ID="mcp-context-verify-$VERIFY_SUFFIX"
SKILL_ID="skill-llm-acceptance-$VERIFY_SUFFIX"
SOUL_ID="soul-llm-acceptance-$VERIFY_SUFFIX"
MEMORY_ID="memory-context-seed-$VERIFY_SUFFIX"
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
run_scenarios = {}
request_sequence = 0

def full_dod_items(step_id):
    return [
        {"item": "happy path completes", "state": "satisfied", "evidenceRefs": [step_id]},
        {"item": "decision can be resolved", "state": "satisfied", "evidenceRefs": [step_id]},
        {"item": "approval rejection fails safely", "state": "satisfied", "evidenceRefs": [step_id]},
        {"item": "multi gate decision then approval can complete", "state": "satisfied", "evidenceRefs": [step_id]},
        {"item": "schema failure is observable", "state": "satisfied", "evidenceRefs": [step_id]},
        {"item": "memory recall is visible", "state": "satisfied", "evidenceRefs": [step_id]},
    ]

def full_goal_coverage(step_id):
    return {
        "satisfied": True,
        "items": [
            {
                "requirement": "verify LLM driven workspace execution",
                "state": "satisfied",
                "evidenceRefs": [step_id],
            },
        ],
    }

def next_scenario(request):
    global request_sequence
    run_id = request.get("runId", "")
    if run_id and run_id in run_scenarios:
        request["scenario"] = run_scenarios[run_id]
        return request["scenario"]
    scenario = request_sequence
    request_sequence += 1
    if run_id:
        run_scenarios[run_id] = scenario
    request["scenario"] = scenario
    return scenario

def mark_continuation_scenario(request, scenario):
    global request_sequence
    request["scenario"] = scenario
    if request_sequence <= scenario:
        request_sequence = scenario + 1

def decision_payload(request):
    run_id = request.get("runId", "")
    step_id = request.get("stepId", "")
    if run_id == "probe":
        return {"completionState": "failed", "thoughtSummary": "probe"}
    scenario = run_scenarios.get(run_id, -1) if run_id else -1
    if "llm-after-decision" in step_id and scenario == 4:
        mark_continuation_scenario(request, 5)
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
        mark_continuation_scenario(request, 2)
        return {
            "thoughtSummary": "Decision information provided; DoD is now satisfied.",
            "completionState": "completed",
            "memoryWrites": [{
                "memoryType": "project_context",
                "summary": "Confirmed implementation convention after decision.",
                "content": "Adopted implementation convention from resolved decision.",
                "tags": ["confirmed", "acceptance"]
            }],
            "dodAssessment": {
                "satisfied": True,
                "summary": "Decision path completed.",
                "items": full_dod_items(step_id),
                "goalCoverage": full_goal_coverage(step_id)
            }
        }
    if "llm-after-approval" in step_id and scenario in {4, 5}:
        mark_continuation_scenario(request, 6)
        return {
            "thoughtSummary": "Decision and approval gates were both completed; DoD is now satisfied.",
            "completionState": "completed",
            "dodAssessment": {
                "satisfied": True,
                "summary": "Multi-gate path completed after Decision and Approval.",
                "items": full_dod_items(step_id),
                "goalCoverage": full_goal_coverage(step_id)
            }
        }
    scenario = next_scenario(request)
    if scenario == 5:
        return {
            "thoughtSummary": "Decision information was provided; approval is now required before completing.",
            "completionState": "blocked_on_approval",
            "requiredApproval": {
                "reason": "Approve the second governance gate after decision resolution.",
                "action": "complete multi-gate LLM path",
                "context": {"scenario": "decision_then_approval"}
            }
        }
    if scenario == 6:
        return {
            "thoughtSummary": "Decision and approval gates were both completed; DoD is now satisfied.",
            "completionState": "completed",
            "dodAssessment": {
                "satisfied": True,
                "summary": "Multi-gate path completed after Decision and Approval.",
                "items": full_dod_items(step_id),
                "goalCoverage": full_goal_coverage(step_id)
            }
        }
    if scenario == 0:
        return {
            "thoughtSummary": "Happy path completed with evidence.",
            "completionState": "completed",
            "memoryWrites": [{
                "memoryType": "user_preference",
                "summary": "Default verification preference: use go test ./...",
                "content": "Operator preference is to use go test ./... as default verification command.",
                "tags": ["preference", "happy-path"]
            }],
            "dodAssessment": {
                "satisfied": True,
                "summary": "Happy path DoD satisfied.",
                "items": full_dod_items(step_id),
                "goalCoverage": full_goal_coverage(step_id)
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
            "thoughtSummary": "Decision information provided; DoD is now satisfied.",
            "completionState": "completed",
            "memoryWrites": [{
                "memoryType": "project_context",
                "summary": "Confirmed implementation convention after decision.",
                "content": "Adopted implementation convention from resolved decision.",
                "tags": ["confirmed", "acceptance"]
            }],
            "dodAssessment": {
                "satisfied": True,
                "summary": "Decision path completed.",
                "items": full_dod_items(step_id),
                "goalCoverage": full_goal_coverage(step_id)
            }
        }
    if scenario == 3:
        return {
            "thoughtSummary": "Risky action requires approval.",
            "completionState": "blocked_on_approval",
            "requiredApproval": {
                "reason": "Approval required before risky action.",
                "action": "continue risky test path",
                "context": {"scenario": "approval"}
            }
        }
    if scenario == 4:
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
    if scenario == 7:
        return "not-json"
    return {
        "thoughtSummary": "Memory recall path completed.",
        "completionState": "completed",
        "dodAssessment": {
            "satisfied": True,
            "summary": "Memory recall DoD satisfied.",
            "items": full_dod_items(step_id),
            "goalCoverage": full_goal_coverage(step_id)
        }
    }

def usage_payload(request):
    run_id = request.get("runId", "")
    if run_id == "probe":
        return None
    scenario = request.get("scenario", run_scenarios.get(run_id, -1)) if run_id else request.get("scenario", -1)
    # Keep scenario-0 without usage so e2e can assert estimated fallback path.
    if scenario == 0:
        return None
    # Other scenarios return provider-native usage for coverage.
    return {
        "prompt_tokens": 64,
        "completion_tokens": 16,
        "total_tokens": 80
    }

def stream_usage_payload(model):
    model = (model or "").strip()
    if "stream-native" in model:
        return {
            "prompt_tokens": 72,
            "completion_tokens": 18,
            "total_tokens": 90
        }
    return None

def provider_request_from_payload(payload):
    messages = payload.get("messages") or []
    merged_content = "\n".join(str(item.get("content", "")) for item in messages)
    run_matches = re.findall(r'"runId"\s*:\s*"([^"]+)"', merged_content)
    if not run_matches:
        run_matches = re.findall(r'\brunId\b\s*[:=]\s*([A-Za-z0-9._/-]+)', merged_content)
    step_matches = re.findall(r'"stepId"\s*:\s*"([^"]+)"', merged_content)
    if not step_matches:
        step_matches = re.findall(r'\bstepId\b\s*[:=]\s*([A-Za-z0-9._/-]+)', merged_content)
    return {
        "runId": run_matches[-1] if run_matches else "",
        "stepId": step_matches[-1] if step_matches else "",
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
        provider_request = provider_request_from_payload(payload)
        decision = decision_payload(provider_request)
        print("scenario=%s runId=%s stepId=%s state=%s" % (
            provider_request.get("scenario", ""),
            provider_request.get("runId", ""),
            provider_request.get("stepId", ""),
            decision.get("completionState") if isinstance(decision, dict) else "raw"
        ), flush=True)
        content = decision if isinstance(decision, str) else json.dumps(decision, separators=(",", ":"))
        model = payload.get("model", "")
        if payload.get("stream"):
            stream_usage = stream_usage_payload(model)
            midpoint = max(1, len(content) // 2)
            chunks = [content[:midpoint], content[midpoint:]]
            events = []
            for chunk in chunks:
                if chunk:
                    events.append({
                        "choices": [{"delta": {"content": chunk}}]
                    })
            if stream_usage is not None:
                events.append({
                    "choices": [],
                    "usage": stream_usage
                })
            events.append("[DONE]")
            encoded_events = []
            for item in events:
                if item == "[DONE]":
                    encoded_events.append("data: [DONE]\n\n")
                else:
                    encoded_events.append("data: " + json.dumps(item, separators=(",", ":")) + "\n\n")
            encoded = "".join(encoded_events).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Connection", "keep-alive")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)
            return

        response = {"choices": [{"message": {"content": content}}]}
        usage = usage_payload(provider_request)
        if usage is not None:
            response["usage"] = usage
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
model_capabilities = llm.get("modelCapabilities") or []

def clean_inference_profile(item):
    profile = item.get("inferenceProfile") or {}
    allowed = {
        "temperature",
        "maxOutputTokens",
        "reasoningEffort",
        "thinkingBudgetTokens",
    }
    return {key: profile[key] for key in allowed if key in profile}

def clean_context_profile(item):
    profile = item.get("contextProfile") or {}
    allowed = {
        "profileMode",
        "contextWindowTokens",
        "maxInputTokens",
        "reservedOutputTokens",
        "safetyRatio",
        "contextUtilizationRatio",
        "bytesHardLimit",
        "maxRequestBytes",
        "estimatedTokensPerByte",
    }
    return {key: profile[key] for key in allowed if key in profile}

def clean_provider(item):
    ret = {
        "id": item.get("id", ""),
        "provider": item.get("provider", ""),
        "baseUrl": item.get("baseUrl", ""),
        "model": item.get("model", ""),
        "timeout": item.get("timeout", ""),
        "disabled": bool(item.get("disabled", False)),
        "chatCompletions": bool(item.get("chatCompletions", True)),
        "streaming": bool(item.get("streaming", False)),
        "nonChatOperations": item.get("nonChatOperations") or [],
        "failoverEnabled": bool(item.get("failoverEnabled", False)),
        "failoverProviders": item.get("failoverProviders") or [],
    }
    inference_profile = clean_inference_profile(item)
    if inference_profile:
        ret["inferenceProfile"] = inference_profile
    context_profile = clean_context_profile(item)
    if context_profile:
        ret["contextProfile"] = context_profile
    return ret

original = {
    "defaultProviderId": default_provider,
    "defaultEmbeddingProviderId": default_embedding_provider,
    "modelCapabilities": model_capabilities,
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
    "contextProfile": {
        "estimatedTokensPerByte": 0.25,
    },
})
updated_providers.append({
    "id": "verify-stream-native",
    "provider": "compatible",
    "baseUrl": mock_base_url,
    "apiKey": "verify-secret",
    "model": "verify-stream-native-model",
    "timeout": "60s",
    "disabled": False,
    "chatCompletions": True,
    "streaming": True,
    "nonChatOperations": ["embeddings"],
    "contextProfile": {
        "estimatedTokensPerByte": 0.25,
    },
})
updated_providers.append({
    "id": "verify-stream-estimated",
    "provider": "compatible",
    "baseUrl": mock_base_url,
    "apiKey": "verify-secret",
    "model": "verify-stream-estimated-model",
    "timeout": "60s",
    "disabled": False,
    "chatCompletions": True,
    "streaming": True,
    "nonChatOperations": ["embeddings"],
    "contextProfile": {
        "estimatedTokensPerByte": 0.25,
    },
})
update = {
    "defaultProviderId": "verify",
    "defaultEmbeddingProviderId": "verify",
    "modelCapabilities": model_capabilities,
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
    -d '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"verify","soulId":"'"$SOUL_ID"'","soulBindings":{"planning":"'"$SOUL_ID"'","execution":"'"$SOUL_ID"'","decision":"'"$SOUL_ID"'","approval":"'"$SOUL_ID"'","validation":"'"$SOUL_ID"'","artifact":"'"$SOUL_ID"'"},"skillIds":["'"$SKILL_ID"'"],"mcpServerIds":["'"$MCP_SERVER_ID"'"],"memoryIds":["'"$MEMORY_ID"'"],"priority":2}' >"$output_file"
}

activate_workspace_execution_with_auto_memory() {
  local output_file="$1"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
    -H 'Content-Type: application/json' \
    -d '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"verify","soulId":"'"$SOUL_ID"'","soulBindings":{"planning":"'"$SOUL_ID"'","execution":"'"$SOUL_ID"'","decision":"'"$SOUL_ID"'","approval":"'"$SOUL_ID"'","validation":"'"$SOUL_ID"'","artifact":"'"$SOUL_ID"'"},"skillIds":["'"$SKILL_ID"'"],"mcpServerIds":["'"$MCP_SERVER_ID"'"],"priority":2}' >"$output_file"
}

activate_workspace_execution_with_provider() {
  local output_file="$1"
  local provider_id="$2"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
    -H 'Content-Type: application/json' \
    -d '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"'"$provider_id"'","soulId":"'"$SOUL_ID"'","soulBindings":{"planning":"'"$SOUL_ID"'","execution":"'"$SOUL_ID"'","decision":"'"$SOUL_ID"'","approval":"'"$SOUL_ID"'","validation":"'"$SOUL_ID"'","artifact":"'"$SOUL_ID"'"},"skillIds":["'"$SKILL_ID"'"],"mcpServerIds":["'"$MCP_SERVER_ID"'"],"priority":2}' >"$output_file"
}

wait_run_state() {
  local run_id="$1"
  local expected_state="$2"
  local output_file="$3"
  local deadline=$((SECONDS + RUN_WAIT_SECONDS))

  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/status?workspaceId=$workspace_id" >"$output_file"
    if grep '"state": "'"$expected_state"'"' "$output_file" >/dev/null; then
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
    curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/status?workspaceId=$workspace_id" >"$output_file"
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

find_replan_successor_run() {
  local predecessor_run_id="$1"
  local output_file="$2"

  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$output_file"
  python3 - "$output_file" "$predecessor_run_id" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8")).get("snapshot") or {}
predecessor = sys.argv[2]
for run in snapshot.get("runs") or []:
    context = run.get("executionContext") or {}
    replan = context.get("replanContinuation") or {}
    if replan.get("predecessorRunId") == predecessor:
        print(run.get("id", ""))
        raise SystemExit(0)
raise SystemExit(f"missing replan successor for predecessor {predecessor!r}")
PY
}

assert_run_recovery_read_models() {
  local run_id="$1"
  local label="$2"
  local expected_state="$3"
  local expected_usage_source="${4:-}"

  curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/snapshot?workspaceId=$workspace_id" >"$TMP_DIR/$label-snapshot-read-model.json"
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/observability?workspaceId=$workspace_id" >"$TMP_DIR/$label-observability-read-model.json"
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/task-progress?workspaceId=$workspace_id" >"$TMP_DIR/$label-task-progress.json"
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/context-bundle?workspaceId=$workspace_id" >"$TMP_DIR/$label-context-bundle.json"
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

usage_traces = observability.get("llmUsageTraces") or []
if not usage_traces:
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

assert_run_llm_usage_semantics() {
  local run_id="$1"
  local expected_usage_source="$2"
  local expected_streaming="$3"
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/observability?workspaceId=$workspace_id" >"$TMP_DIR/$run_id-observability-usage.json"
  python3 - "$TMP_DIR/$run_id-observability-usage.json" "$expected_usage_source" "$expected_streaming" <<'PY'
import json
import sys

observability = json.load(open(sys.argv[1], encoding="utf-8")).get("observability") or {}
expected_source = sys.argv[2]
expected_streaming = sys.argv[3].lower() == "true"
traces = observability.get("llmUsageTraces") or []

if not traces:
    raise SystemExit("observability llmUsageTraces is empty")

matched = [item for item in traces if (item.get("usageSource") or "") == expected_source]
if not matched:
    raise SystemExit(f"missing usageSource={expected_source!r} in traces: {traces}")
if not any(bool(item.get("streaming")) == expected_streaming for item in matched):
    raise SystemExit(
        f"usageSource={expected_source!r} has no trace with streaming={expected_streaming}: {matched}"
    )
PY
}

assert_workspace_llm_usage_breakdown() {
  local workspace_id="$1"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$TMP_DIR/workspace-llm-usage-snapshot.json"
  python3 - "$TMP_DIR/workspace-llm-usage-snapshot.json" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8")).get("snapshot") or {}
summary = snapshot.get("llmUsageSummary") or {}
breakdown = summary.get("providerModelBreakdown") or []

if not breakdown:
    raise SystemExit(f"workspace llm usage summary missing providerModelBreakdown: {summary}")

required = {
    ("verify", "verify-model"),
    ("verify-stream-native", "verify-stream-native-model"),
    ("verify-stream-estimated", "verify-stream-estimated-model"),
}
present = {(item.get("providerId"), item.get("model")) for item in breakdown}
missing = required - present
if missing:
    raise SystemExit(f"workspace provider/model breakdown missing required entries: {sorted(missing)}; present={sorted(present)}")

for item in breakdown:
    if (item.get("providerId"), item.get("model")) in required and int(item.get("entryTotal") or 0) <= 0:
        raise SystemExit(f"workspace provider/model breakdown has empty entryTotal: {item}")
PY
}

trap report_failure ERR
trap timeout_failure TERM
trap cleanup EXIT

cd "$ROOT_DIR"
start_watchdog
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
  -d '{"title":"llm acceptance workspace","description":"verify LLM driven workspace execution","templateId":"generic-governed"}' >"$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]
curl -fsS "$BASE_URL/api/v1/workorch/workspaces" | grep "\"id\": \"$workspace_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id" | grep '"description": "verify LLM driven workspace execution"' >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"verify LLM driven workspace execution"}' >"$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
[[ -n "$goal_id" ]]
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" | grep "\"id\": \"$goal_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/goals/$goal_id" >"$TMP_DIR/goal-read.json"
goal_text="$(json_path "$TMP_DIR/goal-read.json" "goal.text")"
[[ "$goal_text" == *"verify LLM driven workspace execution"* ]]

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
grep '"state": "completed"' "$TMP_DIR/happy-activation.json" >/dev/null
happy_run_id="$(json_path "$TMP_DIR/happy-activation.json" "activation.run.id")"
curl -fsS "$BASE_URL/api/v1/workorch/runs?workspaceId=$workspace_id" | grep "\"id\": \"$happy_run_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$happy_run_id?workspaceId=$workspace_id" | grep '"state": "completed"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$happy_run_id/status?workspaceId=$workspace_id" | grep '"state": "completed"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$happy_run_id/snapshot?workspaceId=$workspace_id" >"$TMP_DIR/happy-snapshot.json"
grep '"dodAssessment"' "$TMP_DIR/happy-snapshot.json" >/dev/null
grep '"memoryRefs"' "$TMP_DIR/happy-snapshot.json" >/dev/null
grep '"soulRouting"' "$TMP_DIR/happy-snapshot.json" >/dev/null
grep '"status": "resolved"' "$TMP_DIR/happy-snapshot.json" >/dev/null
grep "\"soulId\": \"$SOUL_ID\"" "$TMP_DIR/happy-snapshot.json" >/dev/null
grep '"skillIds"' "$TMP_DIR/happy-snapshot.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$happy_run_id/observability?workspaceId=$workspace_id" >"$TMP_DIR/happy-observability.json"
grep '"terminal": true' "$TMP_DIR/happy-observability.json" >/dev/null
assert_run_recovery_read_models "$happy_run_id" "happy" "completed" "estimated"
curl -fsS "$BASE_URL/api/v1/workorch/memory?workspaceId=$workspace_id" >"$TMP_DIR/memory-after-happy.json"
grep 'Default verification preference: use go test ./...' "$TMP_DIR/memory-after-happy.json" >/dev/null

activate_workspace_execution "$TMP_DIR/decision-activation.json"
decision_run_id="$(json_path "$TMP_DIR/decision-activation.json" "activation.run.id")"
decision_state="$(json_path "$TMP_DIR/decision-activation.json" "activation.run.state")"
if [[ "$decision_state" == "blocked_on_decision" ]]; then
  ticket_id="$(json_path "$TMP_DIR/decision-activation.json" "activation.run.blockingTicketId")"
  ticket_path="$(url_encode "$ticket_id")"
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$decision_run_id/status?workspaceId=$workspace_id" | grep '"state": "blocked_on_decision"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id" | grep "\"id\": \"$ticket_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id&runId=$decision_run_id" | grep "\"id\": \"$ticket_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_path?workspaceId=$workspace_id" | grep '"decisionKind": "provide_missing_info"' >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$ticket_path/resolve?workspaceId=$workspace_id" -H 'Content-Type: application/json' \
    -d '{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"llm-acceptance"}}' | grep '"resolutionType": "provide_missing_info"' >/dev/null
elif [[ "$decision_state" == "completed" ]]; then
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id&runId=$decision_run_id" >"$TMP_DIR/decision-auto-resolved-list.json"
  ticket_id="$(python3 - "$TMP_DIR/decision-auto-resolved-list.json" <<'PY'
import json
import sys
items = json.load(open(sys.argv[1], encoding="utf-8")).get("decisions") or []
if not items:
    raise SystemExit("completed decision scenario did not expose a Decision record")
print(items[0].get("id", ""))
PY
)"
  ticket_path="$(url_encode "$ticket_id")"
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_path?workspaceId=$workspace_id" >"$TMP_DIR/decision-auto-resolved-detail.json"
  grep '"decisionKind": "provide_missing_info"' "$TMP_DIR/decision-auto-resolved-detail.json" >/dev/null
  grep '"state": "resolved"' "$TMP_DIR/decision-auto-resolved-detail.json" >/dev/null
else
  echo "decision scenario state=$decision_state, want blocked_on_decision or completed" >&2
  exit 1
fi
wait_run_state "$decision_run_id" "completed" "$TMP_DIR/decision-after-resolve-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$decision_run_id/observability?workspaceId=$workspace_id" | grep '"terminal": true' >/dev/null
assert_run_recovery_read_models "$decision_run_id" "decision" "completed" "provider_native"
curl -fsS "$BASE_URL/api/v1/workorch/memory?workspaceId=$workspace_id" >"$TMP_DIR/memory-after-decision.json"
grep 'Confirmed implementation convention after decision.' "$TMP_DIR/memory-after-decision.json" >/dev/null

activate_workspace_execution "$TMP_DIR/approval-activation.json"
grep '"state": "blocked_on_approval"' "$TMP_DIR/approval-activation.json" >/dev/null
approval_run_id="$(json_path "$TMP_DIR/approval-activation.json" "activation.run.id")"
approval_id="$(json_path "$TMP_DIR/approval-activation.json" "activation.run.blockingApprovalId")"
approval_path="$(url_encode "$approval_id")"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$approval_run_id/status?workspaceId=$workspace_id" | grep '"state": "blocked_on_approval"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/approvals?workspaceId=$workspace_id" | grep "\"id\": \"$approval_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/approvals?workspaceId=$workspace_id&runId=$approval_run_id" | grep "\"id\": \"$approval_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/approvals/$approval_path?workspaceId=$workspace_id" >"$TMP_DIR/approval-detail.json"
[[ "$(json_path "$TMP_DIR/approval-detail.json" "approval.action")" == "continue risky test path" ]]
[[ "$(json_path "$TMP_DIR/approval-detail.json" "approval.context.scenario")" == "approval" ]]
curl -fsS -X POST "$BASE_URL/api/v1/workorch/approvals/$approval_path/reject?workspaceId=$workspace_id" | grep '"state": "rejected"' >/dev/null
wait_run_state "$approval_run_id" "failed" "$TMP_DIR/approval-rejected-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$approval_run_id/observability?workspaceId=$workspace_id" | grep '"terminal": true' >/dev/null
assert_run_recovery_read_models "$approval_run_id" "approval-rejected" "failed" "provider_native"

activate_workspace_execution "$TMP_DIR/multi-gate-decision-activation.json"
grep '"state": "blocked_on_decision"' "$TMP_DIR/multi-gate-decision-activation.json" >/dev/null
multi_gate_run_id="$(json_path "$TMP_DIR/multi-gate-decision-activation.json" "activation.run.id")"
multi_gate_ticket_id="$(json_path "$TMP_DIR/multi-gate-decision-activation.json" "activation.run.blockingTicketId")"
multi_gate_ticket_path="$(url_encode "$multi_gate_ticket_id")"
curl -fsS "$BASE_URL/api/v1/workorch/decisions/$multi_gate_ticket_path?workspaceId=$workspace_id" | grep '"decisionKind": "provide_missing_info"' >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$multi_gate_ticket_path/resolve?workspaceId=$workspace_id" -H 'Content-Type: application/json' \
  -d '{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"llm-acceptance","nextGate":"approval"}}' | grep '"resolutionType": "provide_missing_info"' >/dev/null
wait_run_state "$multi_gate_run_id" "blocked_on_approval" "$TMP_DIR/multi-gate-approval-status.json"
multi_gate_approval_id="$(json_path "$TMP_DIR/multi-gate-approval-status.json" "status.blockingApprovalId")"
multi_gate_approval_path="$(url_encode "$multi_gate_approval_id")"
curl -fsS "$BASE_URL/api/v1/workorch/approvals/$multi_gate_approval_path?workspaceId=$workspace_id" >"$TMP_DIR/multi-gate-approval-detail.json"
grep '"state": "pending"' "$TMP_DIR/multi-gate-approval-detail.json" >/dev/null
[[ "$(json_path "$TMP_DIR/multi-gate-approval-detail.json" "approval.action")" == "complete multi-gate LLM path" ]]
[[ "$(json_path "$TMP_DIR/multi-gate-approval-detail.json" "approval.context.scenario")" == "decision_then_approval" ]]
curl -fsS -X POST "$BASE_URL/api/v1/workorch/approvals/$multi_gate_approval_path/approve?workspaceId=$workspace_id" | grep '"state": "approved"' >/dev/null
wait_run_state "$multi_gate_run_id" "completed" "$TMP_DIR/multi-gate-completed-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$multi_gate_run_id/snapshot?workspaceId=$workspace_id" >"$TMP_DIR/multi-gate-snapshot.json"
grep "\"id\": \"$multi_gate_ticket_id\"" "$TMP_DIR/multi-gate-snapshot.json" >/dev/null
grep "\"id\": \"$multi_gate_approval_id\"" "$TMP_DIR/multi-gate-snapshot.json" >/dev/null
grep 'multi gate decision then approval can complete' "$TMP_DIR/multi-gate-snapshot.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$multi_gate_run_id/status?workspaceId=$workspace_id" | grep '"state": "completed"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$multi_gate_run_id/observability?workspaceId=$workspace_id" >"$TMP_DIR/multi-gate-observability.json"
grep '"terminal": true' "$TMP_DIR/multi-gate-observability.json" >/dev/null
assert_run_recovery_read_models "$multi_gate_run_id" "multi-gate" "completed" "provider_native"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$TMP_DIR/workspace-snapshot-after-governance.json"
grep "\"id\": \"$multi_gate_run_id\"" "$TMP_DIR/workspace-snapshot-after-governance.json" >/dev/null
grep "\"id\": \"$multi_gate_ticket_id\"" "$TMP_DIR/workspace-snapshot-after-governance.json" >/dev/null
grep "\"id\": \"$multi_gate_approval_id\"" "$TMP_DIR/workspace-snapshot-after-governance.json" >/dev/null

activate_workspace_execution "$TMP_DIR/schema-failure-activation.json"
schema_run_id="$(json_path "$TMP_DIR/schema-failure-activation.json" "activation.run.id")"
wait_run_state_any "$schema_run_id" "$TMP_DIR/schema-failure-status.json" "failed" "superseded" "blocked_on_decision"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$schema_run_id/snapshot?workspaceId=$workspace_id" >"$TMP_DIR/schema-failure-snapshot.json"
grep '"llm_schema_invalid"' "$TMP_DIR/schema-failure-snapshot.json" >/dev/null
schema_run_state="$(json_path "$TMP_DIR/schema-failure-status.json" "status.state")"
if [[ "$schema_run_state" == "blocked_on_decision" ]]; then
  schema_ticket_id="$(json_path "$TMP_DIR/schema-failure-status.json" "status.blockingTicketId")"
  schema_ticket_path="$(url_encode "$schema_ticket_id")"
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$schema_ticket_path?workspaceId=$workspace_id" >"$TMP_DIR/schema-failure-decision.json"
  grep '"recoveryPolicy": "runtime_repair_exhausted_requires_strategy"' "$TMP_DIR/schema-failure-decision.json" >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$schema_ticket_path/resolve?workspaceId=$workspace_id" -H 'Content-Type: application/json' \
    -d '{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"llm-acceptance","recoveryAction":"terminate"}}' | grep '"resolutionType": "reject_current_path"' >/dev/null
  wait_run_state "$schema_run_id" "failed" "$TMP_DIR/schema-failure-terminated-status.json"
  schema_run_state="failed"
fi
curl -fsS "$BASE_URL/api/v1/workorch/runs/$schema_run_id/observability?workspaceId=$workspace_id" | grep '"terminal": true' >/dev/null
if [[ "$schema_run_state" == "superseded" ]]; then
  assert_run_recovery_read_models "$schema_run_id" "schema-failure" "superseded" "provider_native"
  schema_successor_run_id="$(find_replan_successor_run "$schema_run_id" "$TMP_DIR/schema-failure-workspace-snapshot.json")"
  [[ -n "$schema_successor_run_id" ]]
  wait_run_state "$schema_successor_run_id" "completed" "$TMP_DIR/schema-failure-successor-status.json"
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$schema_successor_run_id/snapshot?workspaceId=$workspace_id" >"$TMP_DIR/schema-failure-successor-snapshot.json"
  grep '"replanContinuation"' "$TMP_DIR/schema-failure-successor-snapshot.json" >/dev/null
  grep "\"predecessorRunId\": \"$schema_run_id\"" "$TMP_DIR/schema-failure-successor-snapshot.json" >/dev/null
  assert_run_recovery_read_models "$schema_successor_run_id" "schema-failure-successor" "completed" "provider_native"
else
  assert_run_recovery_read_models "$schema_run_id" "schema-failure" "failed" "provider_native"
fi

activate_workspace_execution_with_auto_memory "$TMP_DIR/memory-recall-activation.json"
grep '"state": "completed"' "$TMP_DIR/memory-recall-activation.json" >/dev/null
memory_run_id="$(json_path "$TMP_DIR/memory-recall-activation.json" "activation.run.id")"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$memory_run_id/snapshot?workspaceId=$workspace_id" >"$TMP_DIR/memory-recall-snapshot.json"
grep '"memories"' "$TMP_DIR/memory-recall-snapshot.json" >/dev/null
grep '"memoryRetrieval"' "$TMP_DIR/memory-recall-snapshot.json" >/dev/null
grep "\"$MEMORY_ID\"" "$TMP_DIR/memory-recall-snapshot.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$memory_run_id/observability?workspaceId=$workspace_id" >"$TMP_DIR/memory-recall-observability.json"
grep '"terminal": true' "$TMP_DIR/memory-recall-observability.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$memory_run_id/context-bundle?workspaceId=$workspace_id" >"$TMP_DIR/memory-recall-context-bundle.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$memory_run_id/memory-recalls?workspaceId=$workspace_id" >"$TMP_DIR/memory-recall-run-api.json"
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
for required in ["activation", "step_input"]:
    if required not in sources:
        errors.append(f"run memory-recalls API missing source {required!r}: {sources}")

if not any(memory_id in (item.get("retrievedMemoryIds") or []) for item in run_api):
    errors.append(f"run memory-recalls API missing recalled memory {memory_id}")

section = next((item for item in (bundle.get("sections") or []) if item.get("id") == "memory_recall"), None)
if not section:
    errors.append("context bundle missing memory_recall section")
else:
    section_refs = set(section.get("evidenceRefs") or [])
    if memory_id not in section_refs:
        errors.append(f"context bundle memory_recall section missing memory id {memory_id}: {section_refs}")
    section_traces = ((section.get("data") or {}).get("memoryRecallTraces") or [])
    missing = run_trace_ids - trace_ids(section_traces)
    if missing:
        errors.append(f"context bundle memory_recall section missing trace ids: {sorted(missing)}")

timeline_sources = {item.get("source") for item in (observability.get("timeline") or [])}
if "memory_recall" not in timeline_sources:
    errors.append(f"observability timeline missing memory_recall source: {timeline_sources}")

event_topics = {item.get("topic") for item in events}
if "/memory/recall/recorded" not in event_topics:
    errors.append(f"event stream missing /memory/recall/recorded: {event_topics}")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY
assert_run_recovery_read_models "$memory_run_id" "memory-recall" "completed" "provider_native"

activate_workspace_execution_with_provider "$TMP_DIR/stream-native-activation.json" "verify-stream-native"
grep '"state": "completed"' "$TMP_DIR/stream-native-activation.json" >/dev/null
stream_native_run_id="$(json_path "$TMP_DIR/stream-native-activation.json" "activation.run.id")"
assert_run_recovery_read_models "$stream_native_run_id" "stream-native" "completed" "provider_native"
assert_run_llm_usage_semantics "$stream_native_run_id" "provider_native" "true"

activate_workspace_execution_with_provider "$TMP_DIR/stream-estimated-activation.json" "verify-stream-estimated"
grep '"state": "completed"' "$TMP_DIR/stream-estimated-activation.json" >/dev/null
stream_estimated_run_id="$(json_path "$TMP_DIR/stream-estimated-activation.json" "activation.run.id")"
assert_run_recovery_read_models "$stream_estimated_run_id" "stream-estimated" "completed" "estimated"
assert_run_llm_usage_semantics "$stream_estimated_run_id" "estimated" "true"

assert_workspace_llm_usage_breakdown "$workspace_id"
assert_workspace_recovery_read_models "final"
curl -fsS "$BASE_URL/metrics" >"$TMP_DIR/metrics.txt"
grep 'workorch_http_requests_total{endpoint="configuration_update"}' "$TMP_DIR/metrics.txt" >/dev/null
grep 'workorch_http_requests_total{endpoint="workspace_execution_activation"}' "$TMP_DIR/metrics.txt" >/dev/null
grep 'workorch_http_requests_total{endpoint="run_observability"}' "$TMP_DIR/metrics.txt" >/dev/null
grep 'workorch_http_requests_total{endpoint="approval_approve"}' "$TMP_DIR/metrics.txt" >/dev/null
grep 'workorch_http_requests_total{endpoint="approval_reject"}' "$TMP_DIR/metrics.txt" >/dev/null

echo "workorch LLM workspace execution verification passed for deployed workorch service $BASE_URL"
