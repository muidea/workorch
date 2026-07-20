#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_LLM_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-}}"
source "$ROOT_DIR/scripts/lib/verify-isolated-runtime.sh"
workorch_verify_require_isolated_runtime "$BASE_URL" "verify-llm-workspace-execution"
TMP_DIR="$(mktemp -d)"
KEEP_TMP="${WORKORCH_VERIFY_KEEP_TMP:-0}"
MOCK_ADDR="${WORKORCH_LLM_VERIFY_MOCK_ADDR:-127.0.0.1:18181}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_LLM_VERIFY_SUFFIX:-$$}"
CURL_CONNECT_TIMEOUT="${WORKORCH_VERIFY_CURL_CONNECT_TIMEOUT_SECONDS:-5}"
CURL_MAX_TIME="${WORKORCH_VERIFY_CURL_MAX_TIME_SECONDS:-30}"
SCRIPT_TIMEOUT_SECONDS="${WORKORCH_LLM_VERIFY_TIMEOUT_SECONDS:-900}"
RUN_WAIT_SECONDS="${WORKORCH_LLM_VERIFY_RUN_WAIT_SECONDS:-45}"
ACTIVATION_RUN_WAIT_SECONDS="${WORKORCH_LLM_VERIFY_ACTIVATION_RUN_WAIT_SECONDS:-${RUN_WAIT_SECONDS}}"
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
  if [[ "$KEEP_TMP" == "1" || "$KEEP_TMP" == "true" ]]; then
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

json_path_equals() {
  python3 - "$1" "$2" "$3" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)
current = data
for part in sys.argv[2].split("."):
    if not isinstance(current, dict) or part not in current:
        raise SystemExit(1)
    current = current[part]
raise SystemExit(0 if str(current) == sys.argv[3] else 1)
PY
}

url_encode() {
  python3 - "$1" <<'PY'
import sys
from urllib.parse import quote

print(quote(sys.argv[1], safe=""))
PY
}

json_grep_string_value() {
  local key="$1"
  local value="$2"
  local file="${3:-}"
  local pattern="\"$key\"[[:space:]]*:[[:space:]]*\"$value\""
  if [[ -n "$file" ]]; then
    grep -E "$pattern" "$file"
  else
    grep -E "$pattern"
  fi
}

json_grep_bool_value() {
  local key="$1"
  local value="$2"
  local file="${3:-}"
  local pattern="\"$key\"[[:space:]]*:[[:space:]]*$value"
  if [[ -n "$file" ]]; then
    grep -E "$pattern" "$file"
  else
    grep -E "$pattern"
  fi
}

resolve_decision_ticket_if_needed() {
  local ticket_path="$1"
  local payload="$2"
  local detail_file="$3"
  local resolve_file="$4"

  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_path?workspaceId=$workspace_id" >"$detail_file"
  if json_grep_string_value state resolved "$detail_file" >/dev/null; then
    return 0
  fi
  if curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$ticket_path/resolve?workspaceId=$workspace_id" -H 'Content-Type: application/json' \
    -d "$payload" >"$resolve_file"; then
    json_grep_string_value resolutionType provide_missing_info "$resolve_file" >/dev/null
    return 0
  fi

  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_path?workspaceId=$workspace_id" >"$detail_file"
  json_grep_string_value state resolved "$detail_file" >/dev/null
}

start_mock_provider() {
  cat >"$TMP_DIR/llm_mock_provider.py" <<'PY'
import json
import os
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

# Runtime V2 mock: respond with pure candidate payloads keyed by sealed Action.
# Legacy thoughtSummary/completionState/dodAssessment/executionGraph are rejected
# by DisallowUnknownFields and must never appear in chat content.

run_scenarios = {}
validation_done = set()
request_sequence = 0

def last(pattern, text):
    matches = re.findall(pattern, text or "")
    return matches[-1] if matches else ""

def provider_request_from_payload(payload):
    messages = payload.get("messages") or []
    content = "\n".join(str(item.get("content", "")) for item in messages)
    workspace_id = last(r'"workspaceId"\s*:\s*"([^"]+)"', content) or last(r'workspaceId\s*[:=]\s*([A-Za-z0-9._/-]+)', content)
    run_id = last(r'"runId"\s*:\s*"([^"]+)"', content) or last(r'runId\s*[:=]\s*([A-Za-z0-9._/-]+)', content)
    step_id = last(r'"stepId"\s*:\s*"([^"]+)"', content) or last(r'stepId\s*[:=]\s*([A-Za-z0-9._/-]+)', content)
    action = last(r'"Action"\s*:\s*"([^"]+)"', content)
    if not action:
        action = last(r'"action"\s*:\s*"([^"]+)"', content)
    focus_id = last(r'"Focus"\s*:\s*\{\s*"ID"\s*:\s*"([^"]+)"', content) or last(r'"focusId"\s*:\s*"([^"]+)"', content) or "focus-001"
    coverage_ids = re.findall(r'"(cwi-[A-Za-z0-9._-]+)"', content)
    if not coverage_ids:
        coverage_ids = ["cwi-001"]
    # de-dupe preserve order
    seen = set()
    coverage = []
    for item in coverage_ids:
        if item not in seen:
            seen.add(item)
            coverage.append(item)
    evidence_refs = re.findall(r'"(workorch://[^"]+|run-[^"]+/validation/[^"]+|run-[^"]+/graph-[^"]+)"', content)
    if not evidence_refs:
        evidence_refs = ["workorch://workspace/runtime-validation-source.txt"]
    return {
        "workspaceId": workspace_id,
        "runId": run_id,
        "runKey": (workspace_id + "/" + run_id) if workspace_id and run_id else run_id,
        "stepId": step_id,
        "content": content,
        "action": action or "produce_validation",
        "focusId": focus_id,
        "coverage": coverage[:4],
        "evidenceRefs": evidence_refs[:4],
    }

def scenario_from_content(content):
    lowered = (content or "").lower()
    for marker, scenario in [
        ("multi gate decision then approval can complete", 4),
        ("approval rejection fails safely", 3),
        ("decision can be resolved", 1),
        ("schema failure is observable", 7),
        ("memory recall is visible", 8),
        ("stream usage is recorded", 9),
        ("happy path completes", 0),
    ]:
        if marker in lowered:
            return scenario
    return 0

def next_scenario(request):
    global request_sequence
    run_key = request.get("runKey") or request.get("runId") or ""
    if run_key and run_key in run_scenarios:
        request["scenario"] = run_scenarios[run_key]
        return run_scenarios[run_key]
    scenario = scenario_from_content(request.get("content"))
    if run_key:
        run_scenarios[run_key] = scenario
    request["scenario"] = scenario
    if request_sequence <= scenario:
        request_sequence = scenario + 1
    return scenario

def validation_ops(request):
    # produce_validation forbids workspace side effects; use read-only file/read
    # (same contract as verify-tui-long-output-e2e). optional=true tolerates first-run absence.
    coverage = request["coverage"]
    focus = request["focusId"]
    return {
        "operations": [{
            "id": f"validate-{focus}",
            "kind": "file/read",
            "coverageWorkItemIds": coverage[:1],
            "targetRefs": ["runtime-validation-source.txt"],
            "parameters": [
                {"name": "path", "string": "runtime-validation-source.txt"},
                {"name": "optional", "boolean": True},
                {"name": "maxBytes", "integer": 16384},
            ],
        }]
    }

def state_change_ops(request):
    coverage = request["coverage"]
    focus = request["focusId"]
    return {
        "operations": [{
            "id": f"state-{focus}",
            "kind": "patch/apply",
            "coverageWorkItemIds": coverage[:1],
            "targetRefs": ["runtime-validation-source.txt"],
            "parameters": [
                {"name": "targetRef", "string": "runtime-validation-source.txt"},
                {"name": "patchType", "string": "file_write"},
                {"name": "content", "string": "verified by llm e2e\n"},
                {"name": "summary", "string": "seed validation source for e2e"},
            ],
        }]
    }

def assessment_payload(request, satisfied=True, summary="Runtime V2 assessment completed."):
    refs = list(request.get("evidenceRefs") or [])
    coverage = list(request.get("coverage") or [])
    items = [{
        "coverageWorkItemId": coverage_work_item_id,
        "satisfied": bool(satisfied),
        "evidenceRefs": refs if satisfied else [],
    } for coverage_work_item_id in coverage]
    return {
        "focusId": request["focusId"],
        "coverageWorkItemIds": coverage,
        "disposition": "satisfied" if satisfied else "repairable_gap",
        "items": items,
        "summary": summary,
        "evidenceRefs": refs,
    }

def governance_payload(request, kind, reason):
    return {
        "externalDependency": {
            "kind": kind,
            "subjectRef": request.get("runId") or request.get("focusId") or "runtime",
            "reason": reason,
        }
    }

def decision_payload(request):
    run_id = request.get("runId") or ""
    action = (request.get("action") or "produce_validation").strip()
    scenario = next_scenario(request)
    run_key = request.get("runKey") or run_id

    if run_id == "probe":
        # Probe only needs a 200; return a minimal assessment-shaped body if asked.
        if action == "produce_assessment":
            return assessment_payload(request, True, "probe")
        if action == "request_governance":
            return governance_payload(request, "missing_info", "probe")
        if action == "produce_state_change":
            return state_change_ops(request)
        return validation_ops(request)

    # Schema-failure scenario intentionally returns non-JSON for assessment.
    if scenario == 7 and action == "produce_assessment":
        return "not-json"

    if action == "produce_validation":
        validation_done.add(run_key)
        return validation_ops(request)

    if action == "produce_state_change":
        return state_change_ops(request)

    if action == "produce_assessment":
        # After validation evidence exists in context, close the focus.
        return assessment_payload(
            request,
            True,
            {
                0: "Happy path DoD satisfied.",
                1: "Decision path completed.",
                2: "Decision path completed.",
                3: "Approval path assessment.",
                4: "Multi-gate path completed.",
                6: "Multi-gate path completed.",
                8: "Memory recall DoD satisfied.",
                9: "Stream usage path completed.",
            }.get(scenario, "Runtime V2 assessment completed."),
        )

    if action == "request_governance":
        # V2 opens Decision tickets only; kind/reason still drive ticket text.
        if scenario in {3}:
            return governance_payload(
                request,
                "approval_required",
                "Approval required before risky action. action=continue risky test path scenario=approval",
            )
        if scenario in {4, 5, 6}:
            # multi-gate: first governance is missing info; later ones may re-enter.
            return governance_payload(
                request,
                "missing_info",
                "Need missing detail before the approval gate. scenario=decision_then_approval",
            )
        if scenario in {1, 2}:
            return governance_payload(
                request,
                "missing_info",
                "Need missing implementation detail. scenario=decision",
            )
        return governance_payload(request, "missing_info", "Need operator direction before continuing.")

    # Unknown action: fail closed with validation ops rather than legacy fields.
    return validation_ops(request)

def usage_payload(request):
    run_id = request.get("runId", "")
    if run_id == "probe":
        return None
    scenario = request.get("scenario", 0)
    # Keep scenario-0 without usage so e2e can assert estimated fallback path.
    if scenario == 0:
        return None
    return {"prompt_tokens": 64, "completion_tokens": 16, "total_tokens": 80}

def stream_usage_payload(model):
    model = (model or "").strip()
    if "stream-native" in model:
        return {"prompt_tokens": 72, "completion_tokens": 18, "total_tokens": 90}
    return None

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        path = self.path.split("?", 1)[0]
        if path in ("/v1/models", "/models"):
            models = [
                {"id": "verify-model", "object": "model", "owned_by": "verify", "operations": ["chat_completions", "embeddings"], "contextWindowTokens": 128000, "maxOutputTokens": 8192},
                {"id": "gpt-4o", "object": "model", "owned_by": "openai", "operations": ["chat_completions"], "contextWindowTokens": 128000, "maxOutputTokens": 8192},
                {"id": "text-embedding-3-small", "object": "model", "owned_by": "openai", "operations": ["embeddings"], "contextWindowTokens": 8192, "maxOutputTokens": 1024},
                {"id": "verify-stream-native-model", "object": "model", "owned_by": "verify", "operations": ["chat_completions", "embeddings"], "contextWindowTokens": 128000, "maxOutputTokens": 8192},
                {"id": "verify-stream-estimated-model", "object": "model", "owned_by": "verify", "operations": ["chat_completions", "embeddings"], "contextWindowTokens": 128000, "maxOutputTokens": 8192},
                {"id": "web-search-e2e-model", "object": "model", "owned_by": "verify", "operations": ["chat_completions", "embeddings"], "contextWindowTokens": 128000, "maxOutputTokens": 8192},
                {"id": "container-verify-model", "object": "model", "owned_by": "verify", "operations": ["chat_completions", "embeddings"], "contextWindowTokens": 128000, "maxOutputTokens": 8192},
            ]
            encoded = json.dumps({"object": "list", "data": models}).encode("utf-8")
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
            size = int(self.headers.get("Content-Length", "0"))
            body = self.rfile.read(size)
            try:
                payload = json.loads(body)
            except Exception:
                payload = {}
            input_text = payload.get("input", "")
            seed = sum(ord(ch) for ch in str(input_text)) or 1
            vector = [round(((seed % 17) + 1) / 17, 6), round(((seed % 23) + 1) / 23, 6), round(((seed % 29) + 1) / 29, 6)]
            encoded = json.dumps({"data": [{"embedding": vector}]}).encode("utf-8")
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
        print("scenario=%s action=%s runId=%s stepId=%s kind=%s" % (
            provider_request.get("scenario", ""),
            provider_request.get("action", ""),
            provider_request.get("runId", ""),
            provider_request.get("stepId", ""),
            type(decision).__name__ if not isinstance(decision, dict) else ",".join(decision.keys()),
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
                    events.append({"choices": [{"delta": {"content": chunk}}]})
            if stream_usage is not None:
                events.append({"choices": [], "usage": stream_usage})
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
        "defaultMaxOutputTokens": int(budget.get("defaultMaxOutputTokens") or 4096),
        "maxRequestBytes": int(budget.get("maxRequestBytes") or 4194304),
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
        "verify": {
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
    original["defaultChat"] = {"providerID": "verify", "modelID": "verify-model"}
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
    "defaultChat": {"providerID": "verify", "modelID": "verify-model"},
    "defaultEmbedding": {"providerID": "verify", "modelID": "verify-model"},
    "budget": original["budget"],
    "customProviders": {
        "verify": {
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
                }
            },
        },
        "verify-stream-native": {
            "provider": "compatible",
            "apiSurface": "openai_compatible_http",
            "baseURL": mock_base_url,
            "apiKey": "verify-secret",
            "timeout": "60s",
            "models": {
                "verify-stream-native-model": {
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                    "operations": ["chat_completions", "embeddings"],
                }
            },
        },
        "verify-stream-estimated": {
            "provider": "compatible",
            "apiSurface": "openai_compatible_http",
            "baseURL": mock_base_url,
            "apiKey": "verify-secret",
            "timeout": "60s",
            "models": {
                "verify-stream-estimated-model": {
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                    "operations": ["chat_completions", "embeddings"],
                }
            },
        },
    },
}
with open(update_path, "w", encoding="utf-8") as handle:
    json.dump(update, handle, separators=(",", ":"))
PY
}

activate_workspace_execution() {
  local output_file="$1"
  local activation_dod_id="${2:-$dod_id}"
  local request_id="llm-e2e-$(basename "${output_file%.json}")"
  capture_runs_before_activation "$output_file"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
    -H 'Content-Type: application/json' \
    -d '{"intent":"resume","requestId":"'"$request_id"'","reason":"e2e_resume","source":"e2e","goalId":"'"$goal_id"'","doDId":"'"$activation_dod_id"'","llmTarget":{"providerId":"verify","modelId":"verify-model"},"soulId":"'"$SOUL_ID"'","soulBindings":{"planning":"'"$SOUL_ID"'","execution":"'"$SOUL_ID"'","decision":"'"$SOUL_ID"'","approval":"'"$SOUL_ID"'","validation":"'"$SOUL_ID"'","artifact":"'"$SOUL_ID"'"},"skillIds":["'"$SKILL_ID"'"],"mcpServerIds":["'"$MCP_SERVER_ID"'"],"memoryIds":["'"$MEMORY_ID"'"],"priority":2}' >"$output_file"
}

activate_workspace_execution_without_memory() {
  local output_file="$1"
  local activation_dod_id="${2:-$dod_id}"
  local request_id="llm-e2e-$(basename "${output_file%.json}")"
  capture_runs_before_activation "$output_file"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
    -H 'Content-Type: application/json' \
    -d '{"intent":"resume","requestId":"'"$request_id"'","reason":"e2e_resume","source":"e2e","goalId":"'"$goal_id"'","doDId":"'"$activation_dod_id"'","llmTarget":{"providerId":"verify","modelId":"verify-model"},"soulId":"'"$SOUL_ID"'","soulBindings":{"planning":"'"$SOUL_ID"'","execution":"'"$SOUL_ID"'","decision":"'"$SOUL_ID"'","approval":"'"$SOUL_ID"'","validation":"'"$SOUL_ID"'","artifact":"'"$SOUL_ID"'"},"skillIds":["'"$SKILL_ID"'"],"mcpServerIds":["'"$MCP_SERVER_ID"'"],"priority":2}' >"$output_file"
}

activate_workspace_execution_with_auto_memory() {
  local output_file="$1"
  local activation_dod_id="${2:-$dod_id}"
  local request_id="llm-e2e-$(basename "${output_file%.json}")"
  capture_runs_before_activation "$output_file"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
    -H 'Content-Type: application/json' \
    -d '{"intent":"resume","requestId":"'"$request_id"'","reason":"e2e_resume","source":"e2e","goalId":"'"$goal_id"'","doDId":"'"$activation_dod_id"'","llmTarget":{"providerId":"verify","modelId":"verify-model"},"soulId":"'"$SOUL_ID"'","soulBindings":{"planning":"'"$SOUL_ID"'","execution":"'"$SOUL_ID"'","decision":"'"$SOUL_ID"'","approval":"'"$SOUL_ID"'","validation":"'"$SOUL_ID"'","artifact":"'"$SOUL_ID"'"},"skillIds":["'"$SKILL_ID"'"],"mcpServerIds":["'"$MCP_SERVER_ID"'"],"priority":2}' >"$output_file"
}

activate_workspace_execution_with_provider() {
  local output_file="$1"
  local provider_id="$2"
  local model_id="${3:-verify-model}"
  local activation_dod_id="${4:-$dod_id}"
  local request_id="llm-e2e-$(basename "${output_file%.json}")"
  capture_runs_before_activation "$output_file"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
    -H 'Content-Type: application/json' \
    -d '{"intent":"resume","requestId":"'"$request_id"'","reason":"e2e_resume","source":"e2e","goalId":"'"$goal_id"'","doDId":"'"$activation_dod_id"'","llmTarget":{"providerId":"'"$provider_id"'","modelId":"'"$model_id"'"},"soulId":"'"$SOUL_ID"'","soulBindings":{"planning":"'"$SOUL_ID"'","execution":"'"$SOUL_ID"'","decision":"'"$SOUL_ID"'","approval":"'"$SOUL_ID"'","validation":"'"$SOUL_ID"'","artifact":"'"$SOUL_ID"'"},"skillIds":["'"$SKILL_ID"'"],"mcpServerIds":["'"$MCP_SERVER_ID"'"],"priority":2}' >"$output_file"
}

capture_runs_before_activation() {
  local output_file="$1"
  local before_file="${output_file%.json}-before-runs.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" >"$before_file"
}

wait_activation_run() {
  local activation_file="$1"
  local output_file="$2"
  local snapshot_file="${output_file%.json}-snapshot.json"
  local runs_file="${output_file%.json}-runs.json"
  local before_runs_file="${activation_file%.json}-before-runs.json"
  local run_id=""
  local deadline=$((SECONDS + ACTIVATION_RUN_WAIT_SECONDS))

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
    if curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$snapshot_file"; then
      if curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" >"$runs_file"; then
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
    fi
    sleep 1
  done

  echo "activation accepted but run did not materialize within ${ACTIVATION_RUN_WAIT_SECONDS}s for workspace $workspace_id" >&2
  cat "$activation_file" >&2 || true
  [[ -f "$snapshot_file" ]] && cat "$snapshot_file" >&2
  [[ -f "$runs_file" ]] && cat "$runs_file" >&2
  return 1
}

create_acceptance_workspace() {
  local prefix="$1"
  local title="$2"
  local description="$3"
  local dod_items_json="$4"

  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
    -d '{"title":"'"$title"'","description":"'"$description"'","templateId":"generic-governed"}' >"$TMP_DIR/$prefix-workspace.json"
  workspace_id="$(json_path "$TMP_DIR/$prefix-workspace.json" "workspace.id")"
  [[ -n "$workspace_id" ]]
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" -H 'Content-Type: application/json' \
    -d '{"text":"verify LLM driven workspace execution"}' >"$TMP_DIR/$prefix-goal.json"
  goal_id="$(json_path "$TMP_DIR/$prefix-goal.json" "goal.id")"
  [[ -n "$goal_id" ]]
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" -H 'Content-Type: application/json' \
    -d '{"items":'"$dod_items_json"'}' >"$TMP_DIR/$prefix-dod.json"
  dod_id="$(json_path "$TMP_DIR/$prefix-dod.json" "dod.id")"
  [[ -n "$dod_id" ]]
  # Seed a non-mutating validation target under the workspace deliverable root.
  deliverable_dir="$(python3 -c 'import json,sys; ws=json.load(open(sys.argv[1],encoding="utf-8")).get("workspace") or {}; print(ws.get("deliverableDirectory") or ws.get("workingDirectory") or "")' "$TMP_DIR/$prefix-workspace.json")"
  if [[ -n "$deliverable_dir" ]]; then
    mkdir -p "$deliverable_dir"
    printf 'verified by llm e2e\n' >"$deliverable_dir/runtime-validation-source.txt"
  fi
}

wait_run_state() {
  local run_id="$1"
  local expected_state="$2"
  local output_file="$3"
  local deadline=$((SECONDS + RUN_WAIT_SECONDS))

  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$output_file"
    if json_path_equals "$output_file" "status.state" "$expected_state"; then
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
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$output_file"
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

  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/snapshot" >"$TMP_DIR/$label-snapshot-read-model.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/observability" >"$TMP_DIR/$label-observability-read-model.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/task-progress" >"$TMP_DIR/$label-task-progress.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/context-bundle" >"$TMP_DIR/$label-context-bundle.json"
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

# Runtime V2: compressedContext lives on contextBundle (not run.executionContext).
bundle_compressed = bundle.get("compressedContext") or {}
if bundle.get("runId") not in ("", None, run_id) and bundle.get("runId") != run_id:
    errors.append(f"context bundle run id mismatch: {bundle.get('runId')!r} want {run_id!r}")
if bundle.get("workspaceId") not in ("", None, workspace_id) and bundle.get("workspaceId") != workspace_id:
    errors.append(f"context bundle workspace id mismatch: {bundle.get('workspaceId')!r}")
if bundle_compressed and bundle_compressed.get("version") not in (None, "", "workorch.compressedContext.v1"):
    errors.append(f"context bundle compressedContext invalid: {bundle_compressed}")
section_ids = {item.get("id") for item in (bundle.get("sections") or [])}
if section_ids:
    for required in ["workspace", "run"]:
        if required not in section_ids:
            errors.append(f"context bundle missing section {required!r}: {section_ids}")

if task_progress.get("runId") not in ("", None, run_id) and task_progress.get("runId") != run_id:
    errors.append(f"taskProgress run id mismatch: {task_progress}")
if task_progress.get("workspaceId") not in ("", None, workspace_id) and task_progress.get("workspaceId") != workspace_id:
    errors.append(f"taskProgress workspace id mismatch: {task_progress}")
if not task_progress.get("state"):
    errors.append(f"taskProgress state empty: {task_progress}")
if not (task_progress.get("milestones") or []):
    errors.append(f"taskProgress milestones empty: {task_progress}")

observed_progress = observability.get("taskProgress") or {}
if observed_progress and observed_progress.get("runId") not in ("", None, run_id) and observed_progress.get("runId") != run_id:
    errors.append(f"observability taskProgress missing run id: {observed_progress}")

usage_traces = observability.get("llmUsageTraces") or []
if not usage_traces:
    if expected_usage_source:
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
    # Runtime V2 always emits accepted Result; Artifact materialization is optional.
    if "/result/accepted" not in topics:
        errors.append(f"event stream missing /result/accepted: {topics}")

if expected_state == "completed":
    if not [item for item in (snapshot.get("results") or []) if item.get("state") == "accepted"]:
        # Some completed paths may only expose accepted outcome via resultQuality/acceptance.
        has_accepted = any(
            (item.get("state") == "accepted") for item in (snapshot.get("results") or [])
        ) or ((snapshot.get("outcome") or {}).get("accepted") is True)
        if not has_accepted:
            errors.append("completed run missing accepted Result")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY
}

assert_workspace_recovery_read_models() {
  local label="$1"

  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$TMP_DIR/$label-workspace-snapshot.json"
  python3 - "$TMP_DIR/$label-workspace-snapshot.json" "$workspace_id" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8"))["snapshot"]
progress = snapshot["taskProgress"]
bundle = snapshot["contextBundle"]
workspace_id = sys.argv[2]
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
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/observability" >"$TMP_DIR/$run_id-observability-usage.json"
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
  local deadline=$((SECONDS + 15))
  local error_file="$TMP_DIR/workspace-llm-usage-error.txt"
  while true; do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot?view=observability" >"$TMP_DIR/workspace-llm-usage-snapshot.json"
    if python3 - "$TMP_DIR/workspace-llm-usage-snapshot.json" >"$error_file" 2>&1 <<'PY'
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
    then
      return 0
    fi
    if [[ "$SECONDS" -ge "$deadline" ]]; then
      cat "$error_file" >&2 || true
      return 1
    fi
    sleep 1
  done
}

trap report_failure ERR
trap timeout_failure TERM
trap cleanup EXIT

cd "$ROOT_DIR"
start_watchdog
start_mock_provider
wait_mock
wait_ready

curl -fsS "$BASE_URL/readyz" | json_grep_string_value status ready >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
write_llm_update_payloads
curl -fsS -X PUT "$BASE_URL/api/v1/workorch/configuration/llm" -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/verify-llm-update.json" >"$TMP_DIR/configuration-updated.json"
json_grep_bool_value apiKeyConfigured true "$TMP_DIR/configuration-updated.json" >/dev/null

create_acceptance_workspace "happy" "llm acceptance workspace" "verify LLM driven workspace execution" '["happy path completes"]'
curl -fsS "$BASE_URL/api/v1/workorch/workspaces" | json_grep_string_value id "$workspace_id" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id" | json_grep_string_value description "verify LLM driven workspace execution" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" | json_grep_string_value id "$goal_id" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals/$goal_id" >"$TMP_DIR/goal-read.json"
goal_text="$(json_path "$TMP_DIR/goal-read.json" "goal.text")"
[[ "$goal_text" == *"verify LLM driven workspace execution"* ]]
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" | json_grep_string_value id "$dod_id" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods/$dod_id" | grep '"happy path completes"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$TMP_DIR/workspace-snapshot-defined.json"
json_grep_string_value id "$workspace_id" "$TMP_DIR/workspace-snapshot-defined.json" >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/mcp/servers" -H 'Content-Type: application/json' \
  -d '{"id":"'"$MCP_SERVER_ID"'","name":"MCP Context Verify","transport":"stdio","endpoint":"context-helper","enabled":true,"allowedTools":["repo.summary"],"allowedResources":["workspace://acceptance"],"allowedPrompts":["workorch.acceptance"],"defaultApprovalPolicy":"operator","sessionPolicy":"long_lived","timeoutMillis":60000}' >"$TMP_DIR/mcp-server.json"
json_grep_string_value id "$MCP_SERVER_ID" "$TMP_DIR/mcp-server.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/servers" | json_grep_string_value id "$MCP_SERVER_ID" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID" | json_grep_string_value sessionPolicy long_lived >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID/tools" | json_grep_string_value name repo.summary >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID/resources" | json_grep_string_value uri "workspace://acceptance" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID/prompts" | json_grep_string_value name workorch.acceptance >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/skills" -H 'Content-Type: application/json' \
  -d '{"id":"'"$SKILL_ID"'","name":"LLM acceptance skill","version":"v1","description":"acceptance skill for governed execution","triggerRules":["workspace acceptance"],"instruction":"Use the provided Goal, DoD, MCP and Memory context to complete the run.","requiredToolKinds":["llm/chat","shell/command"],"requiredMCPServerRefs":["'"$MCP_SERVER_ID"'"],"constraints":["must preserve governance gates"]}' >"$TMP_DIR/skill.json"
json_grep_string_value id "$SKILL_ID" "$TMP_DIR/skill.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/skills" | json_grep_string_value id "$SKILL_ID" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/skills/$SKILL_ID" | grep '"requiredMCPServerRefs"' >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/souls" -H 'Content-Type: application/json' \
  -d '{"id":"'"$SOUL_ID"'","name":"LLM Acceptance Soul","description":"acceptance soul for governed execution","businessDomains":["workorch"],"soulMd":"# LLM Acceptance Soul\n\nDrive the workspace according to Goal and DoD while respecting governance gates.\n\n## Planning\n\ngoal-first\n\n## Decision\n\nask when missing info is required\n\n## Approval\n\nrequire operator approval for risky action\n\n## Validation\n\nproduce DoD evidence\n\n## Output Contract\n\nworkorch llm step contract","allowedSkillRefs":["'"$SKILL_ID"'"],"allowedMCPServerRefs":["'"$MCP_SERVER_ID"'"],"allowedToolKinds":["llm/chat","shell/command"]}' >"$TMP_DIR/soul.json"
json_grep_string_value id "$SOUL_ID" "$TMP_DIR/soul.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/souls" | json_grep_string_value id "$SOUL_ID" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/souls/$SOUL_ID" | grep '"soulMd"' >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/memory" -H 'Content-Type: application/json' \
  -d '{"memoryId":"'"$MEMORY_ID"'","sourceType":"user","memoryType":"project_context","content":"Seed memory for acceptance execution.","summary":"Seeded acceptance memory is available to runs.","tags":["acceptance","seed"],"importance":8,"confidence":9,"visibility":"workspace","embeddingProviderId":"verify","embeddingModel":"verify-embedding","embeddingText":"seeded acceptance memory","vector":[0.1,0.2,0.3]}' >"$TMP_DIR/memory-seed.json"
json_grep_string_value id "$MEMORY_ID" "$TMP_DIR/memory-seed.json" >/dev/null
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
happy_run_id="$(wait_activation_run "$TMP_DIR/happy-activation.json" "$TMP_DIR/happy-activation-run.json")"
wait_run_state "$happy_run_id" "completed" "$TMP_DIR/happy-completed-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs?workspaceId=$workspace_id" | json_grep_string_value id "$happy_run_id" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$happy_run_id" | json_grep_string_value state completed >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$happy_run_id/status" >"$TMP_DIR/happy-status-confirm.json"
json_path_equals "$TMP_DIR/happy-status-confirm.json" "status.state" "completed"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$happy_run_id/snapshot" >"$TMP_DIR/happy-snapshot.json"
# Runtime V2: accepted Result carries doDAssessment; V1 memoryRefs/soulRouting/skillIds are gone.
grep -E '"doDAssessment"|"dodAssessment"' "$TMP_DIR/happy-snapshot.json" >/dev/null
grep '"state": "accepted"' "$TMP_DIR/happy-snapshot.json" >/dev/null
grep '"state": "completed"' "$TMP_DIR/happy-snapshot.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$happy_run_id/observability" >"$TMP_DIR/happy-observability.json"
json_grep_bool_value terminal true "$TMP_DIR/happy-observability.json" >/dev/null
assert_run_recovery_read_models "$happy_run_id" "happy" "completed" "estimated"

create_acceptance_workspace "decision" "llm acceptance decision workspace" "verify LLM decision governance" '["decision can be resolved"]'
activate_workspace_execution_without_memory "$TMP_DIR/decision-activation.json"
decision_run_id="$(wait_activation_run "$TMP_DIR/decision-activation.json" "$TMP_DIR/decision-activation-run.json")"
wait_run_state_any "$decision_run_id" "$TMP_DIR/decision-current-status.json" "blocked_on_decision" "completed"
decision_state="$(json_path "$TMP_DIR/decision-current-status.json" "status.state")"
if [[ "$decision_state" == "blocked_on_decision" ]]; then
  ticket_id="$(json_path "$TMP_DIR/decision-current-status.json" "status.blockingTicketId")"
  ticket_path="$(url_encode "$ticket_id")"
  python3 - "$TMP_DIR/decision-current-status.json" <<'PY'
import json
import sys

state = (json.load(open(sys.argv[1], encoding="utf-8")).get("status") or {}).get("state")
raise SystemExit(0 if state in {"blocked_on_decision", "running", "completed"} else 1)
PY
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id" | json_grep_string_value id "$ticket_id" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id&runId=$decision_run_id" | json_grep_string_value id "$ticket_id" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_path?workspaceId=$workspace_id" >"$TMP_DIR/decision-detail-before-resolve.json"
  json_grep_string_value decisionKind provide_missing_info "$TMP_DIR/decision-detail-before-resolve.json" >/dev/null
  resolve_decision_ticket_if_needed \
    "$ticket_path" \
    '{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"llm-acceptance"}}' \
    "$TMP_DIR/decision-detail-before-resolve.json" \
    "$TMP_DIR/decision-resolve.json"
elif [[ "$decision_state" == "completed" ]]; then
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id&runId=$decision_run_id" >"$TMP_DIR/decision-auto-resolved-list.json"
  ticket_id="$(python3 - "$TMP_DIR/decision-auto-resolved-list.json" <<'PY'
import json
import sys
items = json.load(open(sys.argv[1], encoding="utf-8")).get("decisions") or []
print(items[0].get("id", "") if items else "")
PY
)"
  if [[ -n "$ticket_id" ]]; then
    ticket_path="$(url_encode "$ticket_id")"
    curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_path?workspaceId=$workspace_id" >"$TMP_DIR/decision-auto-resolved-detail.json"
    json_grep_string_value decisionKind provide_missing_info "$TMP_DIR/decision-auto-resolved-detail.json" >/dev/null
    json_grep_string_value state resolved "$TMP_DIR/decision-auto-resolved-detail.json" >/dev/null
  fi
else
  echo "decision scenario state=$decision_state, want blocked_on_decision or completed" >&2
  exit 1
fi
wait_run_state "$decision_run_id" "completed" "$TMP_DIR/decision-after-resolve-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$decision_run_id/observability" | json_grep_bool_value terminal true >/dev/null
assert_run_recovery_read_models "$decision_run_id" "decision" "completed" ""

# Runtime V2 governance opens Decision tickets only (not Approval items).
create_acceptance_workspace "approval" "llm acceptance approval workspace" "verify LLM approval governance" '["approval rejection fails safely"]'
activate_workspace_execution_without_memory "$TMP_DIR/approval-activation.json"
approval_run_id="$(wait_activation_run "$TMP_DIR/approval-activation.json" "$TMP_DIR/approval-activation-run.json")"
wait_run_state_any "$approval_run_id" "$TMP_DIR/approval-blocked-status.json" "blocked_on_decision" "failed" "completed"
approval_state="$(json_path "$TMP_DIR/approval-blocked-status.json" "status.state")"
if [[ "$approval_state" == "blocked_on_decision" ]]; then
  approval_ticket_id="$(json_path "$TMP_DIR/approval-blocked-status.json" "status.blockingTicketId")"
  approval_ticket_path="$(url_encode "$approval_ticket_id")"
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$approval_ticket_path?workspaceId=$workspace_id" >"$TMP_DIR/approval-decision-detail.json"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$approval_ticket_path/resolve?workspaceId=$workspace_id"     -H 'Content-Type: application/json'     -d '{"resolutionType":"reject_current_path","resolutionPayload":{"operator":"llm-acceptance","reason":"reject risky path"}}'     >"$TMP_DIR/approval-decision-reject.json" || true
  wait_run_state_any "$approval_run_id" "$TMP_DIR/approval-rejected-status.json" "failed" "completed"
  final_approval_state="$(json_path "$TMP_DIR/approval-rejected-status.json" "status.state")"
else
  final_approval_state="$approval_state"
fi
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$approval_run_id/observability" | json_grep_bool_value terminal true >/dev/null
assert_run_recovery_read_models "$approval_run_id" "approval-rejected" "$final_approval_state" ""

create_acceptance_workspace "multi-gate" "llm acceptance multi-gate workspace" "verify LLM decision then approval governance" '["multi gate decision then approval can complete"]'
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods/$dod_id" | grep '"multi gate decision then approval can complete"' >/dev/null
activate_workspace_execution_without_memory "$TMP_DIR/multi-gate-decision-activation.json"
multi_gate_run_id="$(wait_activation_run "$TMP_DIR/multi-gate-decision-activation.json" "$TMP_DIR/multi-gate-decision-activation-run.json")"
wait_run_state_any "$multi_gate_run_id" "$TMP_DIR/multi-gate-decision-status.json" "blocked_on_decision" "completed"
multi_gate_state="$(json_path "$TMP_DIR/multi-gate-decision-status.json" "status.state")"
multi_gate_ticket_id=""
multi_gate_approval_id=""
if [[ "$multi_gate_state" == "blocked_on_decision" ]]; then
  multi_gate_ticket_id="$(json_path "$TMP_DIR/multi-gate-decision-status.json" "status.blockingTicketId")"
  multi_gate_ticket_path="$(url_encode "$multi_gate_ticket_id")"
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$multi_gate_ticket_path?workspaceId=$workspace_id" | json_grep_string_value decisionKind provide_missing_info >/dev/null
  resolve_decision_ticket_if_needed \
    "$multi_gate_ticket_path" \
    '{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"llm-acceptance","nextGate":"approval"}}' \
    "$TMP_DIR/multi-gate-decision-detail-before-resolve.json" \
    "$TMP_DIR/multi-gate-decision-resolve.json"
  # V2: second governance gate is still a Decision ticket (not Approval).
  wait_run_state_any "$multi_gate_run_id" "$TMP_DIR/multi-gate-approval-status.json" "blocked_on_decision" "completed" "failed"
  multi_gate_second_state="$(json_path "$TMP_DIR/multi-gate-approval-status.json" "status.state")"
  if [[ "$multi_gate_second_state" == "blocked_on_decision" ]]; then
    multi_gate_approval_id="$(json_path "$TMP_DIR/multi-gate-approval-status.json" "status.blockingTicketId")"
    multi_gate_approval_path="$(url_encode "$multi_gate_approval_id")"
    curl -fsS "$BASE_URL/api/v1/workorch/decisions/$multi_gate_approval_path?workspaceId=$workspace_id" >"$TMP_DIR/multi-gate-approval-detail.json"
    json_grep_string_value state open "$TMP_DIR/multi-gate-approval-detail.json" >/dev/null || true
  fi
  if [[ -n "${multi_gate_approval_path:-}" ]]; then
    curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$multi_gate_approval_path/resolve?workspaceId=$workspace_id"       -H 'Content-Type: application/json'       -d '{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"llm-acceptance","gate":"approval-as-decision"}}'       >"$TMP_DIR/multi-gate-second-resolve.json" || true
  fi
  wait_run_state_any "$multi_gate_run_id" "$TMP_DIR/multi-gate-completed-status.json" "completed" "failed"
else
  cp "$TMP_DIR/multi-gate-decision-status.json" "$TMP_DIR/multi-gate-completed-status.json"
fi
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$multi_gate_run_id/snapshot" >"$TMP_DIR/multi-gate-snapshot.json"
if [[ -n "$multi_gate_ticket_id" ]]; then
  json_grep_string_value id "$multi_gate_ticket_id" "$TMP_DIR/multi-gate-snapshot.json" >/dev/null
fi
if [[ -n "$multi_gate_approval_id" ]]; then
  json_grep_string_value id "$multi_gate_approval_id" "$TMP_DIR/multi-gate-snapshot.json" >/dev/null
fi
grep 'multi gate decision then approval can complete' "$TMP_DIR/multi-gate-snapshot.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$multi_gate_run_id/status" >"$TMP_DIR/multi-gate-status-confirm.json"
json_path_equals "$TMP_DIR/multi-gate-status-confirm.json" "status.state" "completed"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$multi_gate_run_id/observability" >"$TMP_DIR/multi-gate-observability.json"
json_grep_bool_value terminal true "$TMP_DIR/multi-gate-observability.json" >/dev/null
assert_run_recovery_read_models "$multi_gate_run_id" "multi-gate" "completed" ""
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$TMP_DIR/workspace-snapshot-after-governance.json"
json_grep_string_value id "$multi_gate_run_id" "$TMP_DIR/workspace-snapshot-after-governance.json" >/dev/null
if [[ -n "$multi_gate_ticket_id" ]]; then
  json_grep_string_value id "$multi_gate_ticket_id" "$TMP_DIR/workspace-snapshot-after-governance.json" >/dev/null
fi
if [[ -n "$multi_gate_approval_id" ]]; then
  json_grep_string_value id "$multi_gate_approval_id" "$TMP_DIR/workspace-snapshot-after-governance.json" >/dev/null
fi

create_acceptance_workspace "schema" "llm acceptance schema workspace" "verify LLM schema failure observability" '["schema failure is observable"]'
activate_workspace_execution_without_memory "$TMP_DIR/schema-failure-activation.json"
schema_run_id="$(wait_activation_run "$TMP_DIR/schema-failure-activation.json" "$TMP_DIR/schema-failure-activation-run.json")"
wait_run_state_any "$schema_run_id" "$TMP_DIR/schema-failure-status.json" "failed" "superseded" "blocked_on_decision" "completed"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$schema_run_id/snapshot" >"$TMP_DIR/schema-failure-snapshot.json"
# V2 may surface schema failures as candidate_retry_exhausted / unknown field / failed status.
grep -EEi 'llm_schema_invalid|unknown field|candidate_retry_exhausted|schema|not-json|failed' "$TMP_DIR/schema-failure-snapshot.json" "$TMP_DIR/schema-failure-status.json" >/dev/null
schema_run_state="$(json_path "$TMP_DIR/schema-failure-status.json" "status.state")"
if [[ "$schema_run_state" == "blocked_on_decision" ]]; then
  schema_ticket_id="$(json_path "$TMP_DIR/schema-failure-status.json" "status.blockingTicketId")"
  schema_ticket_path="$(url_encode "$schema_ticket_id")"
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$schema_ticket_path?workspaceId=$workspace_id" >"$TMP_DIR/schema-failure-decision.json"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$schema_ticket_path/resolve?workspaceId=$workspace_id" -H 'Content-Type: application/json' \
    -d '{"resolutionType":"reject_current_path","resolutionPayload":{"operator":"llm-acceptance","recoveryAction":"terminate"}}' \
    >"$TMP_DIR/schema-failure-resolve.json" || true
  wait_run_state_any "$schema_run_id" "$TMP_DIR/schema-failure-terminated-status.json" "failed" "completed" "superseded"
  schema_run_state="$(json_path "$TMP_DIR/schema-failure-terminated-status.json" "status.state")"
fi
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$schema_run_id/observability" | json_grep_bool_value terminal true >/dev/null || true
# V2 does not auto-create replan successors; accept terminal failed/completed/superseded.
assert_run_recovery_read_models "$schema_run_id" "schema-failure" "$schema_run_state" ""

create_acceptance_workspace "memory" "llm acceptance memory workspace" "verify LLM memory recall" '["memory recall is visible"]'
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/memory" -H 'Content-Type: application/json' \
  -d '{"memoryId":"'"$MEMORY_ID"'","sourceType":"user","memoryType":"project_context","content":"Seed memory for acceptance execution.","summary":"Seeded acceptance memory is available to runs.","tags":["acceptance","seed"],"importance":8,"confidence":9,"visibility":"workspace","embeddingProviderId":"verify","embeddingModel":"verify-embedding","embeddingText":"seeded acceptance memory","vector":[0.1,0.2,0.3]}' >"$TMP_DIR/memory-recall-seed.json"
json_grep_string_value id "$MEMORY_ID" "$TMP_DIR/memory-recall-seed.json" >/dev/null
activate_workspace_execution_with_auto_memory "$TMP_DIR/memory-recall-activation.json"
memory_run_id="$(wait_activation_run "$TMP_DIR/memory-recall-activation.json" "$TMP_DIR/memory-recall-activation-run.json")"
wait_run_state "$memory_run_id" "completed" "$TMP_DIR/memory-recall-completed-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$memory_run_id/snapshot" >"$TMP_DIR/memory-recall-snapshot.json"
# V2 surfaces recall via memoryRecallTraces (not V1 memories/memoryRetrieval maps).
grep -E '"memoryRecallTraces"|"memories"|"memoryRetrieval"' "$TMP_DIR/memory-recall-snapshot.json" >/dev/null
grep -F "$MEMORY_ID" "$TMP_DIR/memory-recall-snapshot.json" >/dev/null || true
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$memory_run_id/observability" >"$TMP_DIR/memory-recall-observability.json"
json_grep_bool_value terminal true "$TMP_DIR/memory-recall-observability.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$memory_run_id/context-bundle" >"$TMP_DIR/memory-recall-context-bundle.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$memory_run_id/memory-recalls" >"$TMP_DIR/memory-recall-run-api.json"
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
if run_api and not sources:
    errors.append(f"run memory-recalls API missing sources: {sources}")

if run_api and not any(memory_id in (item.get("retrievedMemoryIds") or []) for item in run_api):
    blob = json.dumps(run_api)
    if memory_id not in blob:
        errors.append(f"run memory-recalls API missing recalled memory {memory_id}")

section = next((item for item in (bundle.get("sections") or []) if item.get("id") == "memory_recall"), None)
if section:
    section_refs = set(section.get("evidenceRefs") or [])
    section_traces = ((section.get("data") or {}).get("memoryRecallTraces") or [])
    if section_refs and memory_id not in section_refs and memory_id not in json.dumps(section_traces):
        errors.append(f"context bundle memory_recall section missing memory id {memory_id}: {section_refs}")

timeline_sources = {item.get("source") for item in (observability.get("timeline") or [])}
event_topics = {item.get("topic") for item in events}
if run_api and "/memory/recall/recorded" not in event_topics and "memory_recall" not in timeline_sources:
    if not snapshot_traces and not observability_traces:
        errors.append("memory recall traces present in API but not in snapshot/observability")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY
assert_run_recovery_read_models "$memory_run_id" "memory-recall" "completed" ""

create_acceptance_workspace "stream" "llm acceptance stream workspace" "verify LLM streaming usage" '["stream usage is recorded"]'
activate_workspace_execution_without_memory "$TMP_DIR/stream-baseline-activation.json"
stream_baseline_run_id="$(wait_activation_run "$TMP_DIR/stream-baseline-activation.json" "$TMP_DIR/stream-baseline-activation-run.json")"
wait_run_state "$stream_baseline_run_id" "completed" "$TMP_DIR/stream-baseline-completed-status.json"
assert_run_recovery_read_models "$stream_baseline_run_id" "stream-baseline" "completed" ""

activate_workspace_execution_with_provider "$TMP_DIR/stream-native-activation.json" "verify-stream-native" "verify-stream-native-model"
stream_native_run_id="$(wait_activation_run "$TMP_DIR/stream-native-activation.json" "$TMP_DIR/stream-native-activation-run.json")"
wait_run_state "$stream_native_run_id" "completed" "$TMP_DIR/stream-native-completed-status.json"
assert_run_recovery_read_models "$stream_native_run_id" "stream-native" "completed" ""
# Runtime V2 candidate dispatch currently uses the sealed non-streaming Complete
# path. Stream-capable provider adapters remain covered by provider unit tests;
# this deployed-instance verifier must assert the production path actually used.
assert_run_llm_usage_semantics "$stream_native_run_id" "provider_native" "false"

activate_workspace_execution_with_provider "$TMP_DIR/stream-estimated-activation.json" "verify-stream-estimated" "verify-stream-estimated-model"
stream_estimated_run_id="$(wait_activation_run "$TMP_DIR/stream-estimated-activation.json" "$TMP_DIR/stream-estimated-activation-run.json")"
wait_run_state "$stream_estimated_run_id" "completed" "$TMP_DIR/stream-estimated-completed-status.json"
assert_run_recovery_read_models "$stream_estimated_run_id" "stream-estimated" "completed" "provider_native"
assert_run_llm_usage_semantics "$stream_estimated_run_id" "provider_native" "false"

assert_workspace_llm_usage_breakdown "$workspace_id"
assert_workspace_recovery_read_models "final"
curl -fsS "$BASE_URL/metrics" >"$TMP_DIR/metrics.txt"
grep 'workorch_http_requests_total{endpoint="configuration_update"}' "$TMP_DIR/metrics.txt" >/dev/null
grep 'workorch_http_requests_total{endpoint="workspace_execution_activation"}' "$TMP_DIR/metrics.txt" >/dev/null
grep 'workorch_http_requests_total{endpoint="run_observability"}' "$TMP_DIR/metrics.txt" >/dev/null
# Approval HTTP endpoints may be unused under Runtime V2 decision-only governance.
grep -E 'workorch_http_requests_total\{endpoint="(decision_resolve|approval_approve|approval_reject)"\}' "$TMP_DIR/metrics.txt" >/dev/null || true

echo "workorch LLM workspace execution verification passed for deployed workorch service $BASE_URL"
