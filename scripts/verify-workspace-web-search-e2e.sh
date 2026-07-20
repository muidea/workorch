#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_WEB_SEARCH_E2E_BASE_URL:-${WORKORCH_BASE_URL:-}}"
source "$ROOT_DIR/scripts/lib/verify-isolated-runtime.sh"
workorch_verify_require_isolated_runtime "$BASE_URL" "verify-workspace-web-search-e2e"
TMP_DIR="$(mktemp -d)"
KEEP_ARTIFACTS="${WORKORCH_WEB_SEARCH_E2E_KEEP_ARTIFACTS:-0}"
LLM_MOCK_ADDR="${WORKORCH_WEB_SEARCH_E2E_LLM_MOCK_ADDR:-127.0.0.1:18191}"
WEB_MOCK_ADDR="${WORKORCH_WEB_SEARCH_E2E_WEB_MOCK_ADDR:-127.0.0.1:18192}"
LLM_MOCK_BASE_URL="http://$LLM_MOCK_ADDR/v1"
WEB_MOCK_BASE_URL="http://$WEB_MOCK_ADDR"
VERIFY_SUFFIX="${WORKORCH_WEB_SEARCH_E2E_SUFFIX:-$$}"
ACTIVATION_RUN_WAIT_SECONDS="${WORKORCH_WEB_SEARCH_E2E_ACTIVATION_RUN_WAIT_SECONDS:-60}"
LLM_PROVIDER_ID="web-search-e2e-llm-$VERIFY_SUFFIX"
WEB_PROVIDER_ID="web-search-e2e-web-$VERIFY_SUFFIX"
SOUL_ID="soul-web-search-e2e-$VERIFY_SUFFIX"
LLM_MOCK_PID=""
WEB_MOCK_PID=""
LLM_MOCK_LOG="$TMP_DIR/llm-mock.log"
WEB_MOCK_LOG="$TMP_DIR/web-mock.log"
LOCK_DIR="/tmp/workorch-e2e-llm-config.lock"
LOCK_HELD=0

acquire_llm_config_lock() {
  local deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    if mkdir "$LOCK_DIR" >/dev/null 2>&1; then
      LOCK_HELD=1
      return 0
    fi
    sleep 0.2
  done
  echo "timed out acquiring LLM config lock: $LOCK_DIR" >&2
  return 1
}

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "workorch Web Search e2e verification failed for deployed workorch service $BASE_URL" >&2
  for log_file in "$LLM_MOCK_LOG" "$WEB_MOCK_LOG"; do
    [[ -f "$log_file" ]] || continue
    echo "--- $log_file ---" >&2
    tail -n 200 "$log_file" >&2 || true
  done
  for file in "$TMP_DIR"/*.json; do
    [[ -f "$file" ]] || continue
    echo "--- $file ---" >&2
    python3 -m json.tool "$file" >&2 || cat "$file" >&2
  done
}

cleanup() {
  if [[ -f "$TMP_DIR/original-llm.json" ]]; then
    curl -fsS -H "Content-Type: application/json" -X PUT \
      "$BASE_URL/api/v1/workorch/configuration/llm" \
      -d @"$TMP_DIR/original-llm.json" >/dev/null 2>&1 || true
  fi
  if [[ -f "$TMP_DIR/original-web-search.json" ]]; then
    curl -fsS -H "Content-Type: application/json" -X PUT \
      "$BASE_URL/api/v1/workorch/configuration/web-search" \
      -d @"$TMP_DIR/original-web-search.json" >/dev/null 2>&1 || true
  fi
  if [[ -n "$LLM_MOCK_PID" ]]; then
    kill "$LLM_MOCK_PID" >/dev/null 2>&1 || true
    wait "$LLM_MOCK_PID" >/dev/null 2>&1 || true
  fi
  if [[ -n "$WEB_MOCK_PID" ]]; then
    kill "$WEB_MOCK_PID" >/dev/null 2>&1 || true
    wait "$WEB_MOCK_PID" >/dev/null 2>&1 || true
  fi
  if [[ "$LOCK_HELD" == "1" ]]; then
    rmdir "$LOCK_DIR" >/dev/null 2>&1 || true
  fi
  if [[ "$KEEP_ARTIFACTS" == "1" ]]; then
    echo "web search e2e artifacts retained at $TMP_DIR" >&2
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

acquire_llm_config_lock

start_llm_mock() {
  cat >"$TMP_DIR/llm_mock_provider.py" <<'PY'
import json
import os
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

calls = {}
source_url = "https://93.184.216.34/workorch-web-search-source"

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

def decision_payload(run_id, step_id):
    if run_id == "probe":
        return {"completionState": "failed", "thoughtSummary": "probe"}
    count = calls.get(run_id, 0) + 1
    calls[run_id] = count
    if count == 1:
        return {
            "thoughtSummary": "Search and fetch governed web evidence.",
            "completionState": "continue",
            "executionGraph": {
                "nodes": [
                    {
                        "id": "web-search",
                        "kind": "web/search",
                        "input": {
                            "query": "workorch governed web search e2e",
                            "maxResults": 2,
                            "reason": "verify source traceability"
                        }
                    },
                    {
                        "id": "web-fetch",
                        "kind": "web/fetch",
                        "dependsOn": ["web-search"],
                        "input": {
                            "url": source_url,
                            "extractMode": "summary",
                            "reason": "verify fetched source traceability"
                        }
                    }
                ]
            }
        }
    return {
        "thoughtSummary": "Fetched web source was used as governed evidence.",
        "completionState": "completed",
        "dodAssessment": {
            "satisfied": True,
            "summary": "Web Search e2e DoD satisfied with traced search and fetch evidence.",
            "items": [
                {
                    "item": "web/search and web/fetch produce traceable sources",
                    "state": "satisfied",
                    "evidenceRefs": [step_id]
                }
            ],
            "goalCoverage": {
                "satisfied": True,
                "items": [
                    {
                        "requirement": "verify governed Web Search source traceability",
                        "state": "satisfied",
                        "evidenceRefs": [step_id]
                    }
                ]
            }
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
        if self.path == "/v1/embeddings":
            response = {"data": [{"embedding": [0.11, 0.22, 0.33]}]}
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
        provider_request = provider_request_from_payload(payload)
        content = json.dumps(decision_payload(provider_request["runId"], provider_request["stepId"]), separators=(",", ":"))
        response = {"choices": [{"message": {"content": content}}]}
        encoded = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)

host, port_text = os.environ.get("WORKORCH_WEB_SEARCH_E2E_LLM_MOCK_ADDR", "127.0.0.1:18191").split(":", 1)
ThreadingHTTPServer((host, int(port_text)), Handler).serve_forever()
PY
  WORKORCH_WEB_SEARCH_E2E_LLM_MOCK_ADDR="$LLM_MOCK_ADDR" python3 "$TMP_DIR/llm_mock_provider.py" >>"$LLM_MOCK_LOG" 2>&1 &
  LLM_MOCK_PID=$!
}

start_web_mock() {
  cat >"$TMP_DIR/web_mock_provider.py" <<'PY'
import datetime as dt
import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

source_url = "https://93.184.216.34/workorch-web-search-source"

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
        size = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(size) or b"{}")
        if self.path == "/search":
            response = {
                "results": [
                    {
                        "title": "WorkOrch Web Search E2E Source",
                        "url": source_url,
                        "snippet": "Governed web search e2e source returned by mock provider.",
                        "source": "mock-web-search",
                        "publishedAt": "2026-05-06T00:00:00Z"
                    }
                ]
            }
        elif self.path == "/fetch":
            response = {
                "url": payload.get("url") or source_url,
                "finalUrl": payload.get("url") or source_url,
                "title": "WorkOrch Web Search E2E Source",
                "content": "This source confirms that governed web search and fetch evidence are traceable.",
                "summary": "Governed web search and fetch evidence are traceable.",
                "contentType": "text/plain",
                "retrievedAt": dt.datetime.now(dt.timezone.utc).isoformat(),
                "publishedAt": "2026-05-06T00:00:00Z",
                "source": "mock-web-fetch",
                "truncated": False
            }
        else:
            self.send_response(404)
            self.end_headers()
            return
        encoded = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)

host, port_text = os.environ.get("WORKORCH_WEB_SEARCH_E2E_WEB_MOCK_ADDR", "127.0.0.1:18192").split(":", 1)
ThreadingHTTPServer((host, int(port_text)), Handler).serve_forever()
PY
  WORKORCH_WEB_SEARCH_E2E_WEB_MOCK_ADDR="$WEB_MOCK_ADDR" python3 "$TMP_DIR/web_mock_provider.py" >>"$WEB_MOCK_LOG" 2>&1 &
  WEB_MOCK_PID=$!
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

wait_mocks() {
  for _ in $(seq 1 50); do
    if curl -fsS -X POST "$LLM_MOCK_BASE_URL/chat/completions" \
      -H 'Content-Type: application/json' \
      -d '{"messages":[{"role":"user","content":"{\"runId\": \"probe\", \"stepId\": \"probe\"}"}]}' >/dev/null 2>&1 &&
      curl -fsS -X POST "$WEB_MOCK_BASE_URL/search" \
        -H 'Content-Type: application/json' \
        -d '{"query":"probe"}' >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl -fsS -X POST "$LLM_MOCK_BASE_URL/chat/completions" -H 'Content-Type: application/json' \
    -d '{"messages":[{"role":"user","content":"{\"runId\": \"probe\", \"stepId\": \"probe\"}"}]}' >/dev/null
  curl -fsS -X POST "$WEB_MOCK_BASE_URL/search" -H 'Content-Type: application/json' -d '{"query":"probe"}' >/dev/null
}

write_configuration_payloads() {
  python3 - "$TMP_DIR/configuration.json" "$TMP_DIR/original-llm.json" "$TMP_DIR/updated-llm.json" "$TMP_DIR/original-web-search.json" "$TMP_DIR/updated-web-search.json" "$LLM_MOCK_BASE_URL" "$WEB_MOCK_BASE_URL" "$LLM_PROVIDER_ID" "$WEB_PROVIDER_ID" <<'PY'
import json
import sys

config_path, original_llm_path, updated_llm_path, original_web_path, updated_web_path, llm_base_url, web_base_url, llm_provider_id, web_provider_id = sys.argv[1:]
data = json.load(open(config_path, encoding="utf-8"))
configuration = data.get("configuration") or {}
llm = configuration.get("llm") or {}
web = configuration.get("webSearch") or {}

def clean_llm(item):
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

def clean_web(item):
    return {
        "id": item.get("id", ""),
        "provider": item.get("provider", "http"),
        "searchEndpoint": item.get("searchEndpoint", ""),
        "fetchEndpoint": item.get("fetchEndpoint", ""),
        "timeout": item.get("timeout", ""),
        "maxResults": item.get("maxResults") or 0,
        "maxFetchBytes": item.get("maxFetchBytes") or 0,
        "maxRedirects": item.get("maxRedirects") or 0,
        "allowDomains": item.get("allowDomains") or [],
        "denyDomains": item.get("denyDomains") or [],
        "disabled": bool(item.get("disabled", False)),
    }

original_llm = {
    "aiProxy": {
        "baseURL": (llm.get("aiProxy") or {}).get("baseUrl") or (llm.get("aiProxy") or {}).get("baseURL") or llm_base_url,
        "timeout": (llm.get("aiProxy") or {}).get("timeout") or "180s",
        "catalogRequestTimeout": (llm.get("aiProxy") or {}).get("catalogRequestTimeout") or "5s",
        "catalogTTL": (llm.get("aiProxy") or {}).get("catalogTTL") or "5m",
        "catalogMaxStaleAge": (llm.get("aiProxy") or {}).get("catalogMaxStaleAge") or "24h",
        "maxCatalogResponseBytes": int((llm.get("aiProxy") or {}).get("maxCatalogResponseBytes") or 1048576),
        "maxCatalogModels": int((llm.get("aiProxy") or {}).get("maxCatalogModels") or 1000),
    },
    "defaultChat": {
        "providerID": (llm.get("defaultChat") or {}).get("providerId") or (llm.get("defaultChat") or {}).get("providerID") or llm_provider_id,
        "modelID": (llm.get("defaultChat") or {}).get("modelId") or (llm.get("defaultChat") or {}).get("modelID") or "web-search-e2e-model",
    },
    "budget": {
        "contextUtilizationRatio": float((llm.get("budget") or {}).get("contextUtilizationRatio") or 0.75),
        "estimatedTokensPerByte": float((llm.get("budget") or {}).get("estimatedTokensPerByte") or 0.25),
        "defaultMaxOutputTokens": int((llm.get("budget") or {}).get("defaultMaxOutputTokens") or 4096),
        "maxRequestBytes": int((llm.get("budget") or {}).get("maxRequestBytes") or 4194304),
    },
    "customProviders": {},
}
for item in llm.get("customProviders") or []:
    pid = (item.get("id") or "").strip()
    if not pid:
        continue
    models = {}
    for model in item.get("models") or []:
        mid = (model.get("id") or "").strip()
        if not mid:
            continue
        models[mid] = {
            "contextWindowTokens": int(model.get("contextWindowTokens") or 128000),
            "maxOutputTokens": int(model.get("maxOutputTokens") or 8192),
            "operations": list(model.get("operations") or ["chat_completions"]),
        }
    original_llm["customProviders"][pid] = {
        "provider": item.get("provider") or "compatible",
        "apiSurface": item.get("apiSurface") or "openai_compatible_http",
        "baseURL": item.get("baseUrl") or item.get("baseURL") or llm_base_url,
        "timeout": item.get("timeout") or "60s",
        "models": models or {"web-search-e2e-model": {"contextWindowTokens": 128000, "maxOutputTokens": 8192, "operations": ["chat_completions", "embeddings"]}},
    }
updated_llm = {
    "aiProxy": {
        "baseURL": llm_base_url,
        "apiKey": "web-search-e2e-secret",
        "timeout": "180s",
        "catalogRequestTimeout": "5s",
        "catalogTTL": "5m",
        "catalogMaxStaleAge": "24h",
        "maxCatalogResponseBytes": 1048576,
        "maxCatalogModels": 1000,
        "modelOverrides": {
            "web-search-e2e-model": {"contextWindowTokens": 128000, "maxOutputTokens": 8192, "operations": ["chat_completions", "embeddings"]},
        },
    },
    "defaultChat": {"providerID": llm_provider_id, "modelID": "web-search-e2e-model"},
    "defaultEmbedding": {"providerID": llm_provider_id, "modelID": "web-search-e2e-model"},
    "budget": original_llm["budget"],
    "customProviders": {
        llm_provider_id: {
            "provider": "compatible",
            "apiSurface": "openai_compatible_http",
            "baseURL": llm_base_url,
            "apiKey": "web-search-e2e-secret",
            "timeout": "60s",
            "models": {
                "web-search-e2e-model": {
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                    "operations": ["chat_completions", "embeddings"],
                }
            },
        }
    },
}

original_web = {
    "defaultProviderId": web.get("defaultProviderId") or "",
    "providers": [clean_web(item) for item in web.get("providers") or [] if item.get("id")],
}
updated_web_providers = [item for item in original_web["providers"] if item["id"] != web_provider_id]
updated_web_providers.append({
    "id": web_provider_id,
    "provider": "http",
    "searchEndpoint": web_base_url + "/search",
    "fetchEndpoint": web_base_url + "/fetch",
    "apiKey": "web-search-e2e-secret",
    "timeout": "20s",
    "maxResults": 8,
    "maxFetchBytes": 262144,
    "maxRedirects": 3,
    "allowDomains": [],
    "denyDomains": [],
    "disabled": False,
})
updated_web = {
    "defaultProviderId": web_provider_id,
    "providers": updated_web_providers,
}

for path, payload in [
    (original_llm_path, original_llm),
    (updated_llm_path, updated_llm),
    (original_web_path, original_web),
    (updated_web_path, updated_web),
]:
    with open(path, "w", encoding="utf-8") as handle:
        json.dump(payload, handle, separators=(",", ":"))
PY
}

wait_run_state() {
  local workspace_id="$1"
  local run_id="$2"
  local expected_state="$3"
  local output_file="$4"
  local deadline=$((SECONDS + 60))
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

assert_web_search_traceability() {
  local workspace_id="$1"
  local run_id="$2"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/snapshot" >"$TMP_DIR/run-snapshot.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/observability" >"$TMP_DIR/run-observability.json"
  curl -fsS "$BASE_URL/api/v1/workorch/events?workspaceId=$workspace_id&runId=$run_id&limit=100" >"$TMP_DIR/run-events.json"
  python3 - "$TMP_DIR/run-snapshot.json" "$TMP_DIR/run-observability.json" "$TMP_DIR/run-events.json" "$run_id" "$SOUL_ID" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8")).get("snapshot") or {}
observability = json.load(open(sys.argv[2], encoding="utf-8")).get("observability") or {}
events = json.load(open(sys.argv[3], encoding="utf-8")).get("eventRecords") or []
run_id = sys.argv[4]
expected_soul_id = sys.argv[5]
errors = []

run = snapshot.get("run") or {}
if run.get("id") != run_id or run.get("state") != "completed":
    errors.append(f"run state invalid: {run}")
routing = (run.get("executionContext") or {}).get("soulRouting") or {}
bindings = routing.get("bindings") or {}
execution_binding = bindings.get("execution") or {}
if routing.get("status") != "resolved":
    errors.append(f"soulRouting.status is {routing.get('status')!r}, want resolved")
if execution_binding.get("soulId") != expected_soul_id:
    errors.append(f"execution soul binding is {execution_binding!r}, want {expected_soul_id!r}")

sources = snapshot.get("webSources") or []
observed_sources = observability.get("webSources") or []
if not sources:
    errors.append("snapshot missing webSources")
if not observed_sources:
    errors.append("observability missing webSources")

all_sources = sources + observed_sources
if not any(item.get("sources") for item in all_sources):
    errors.append(f"webSources missing search sources: {all_sources}")
if not any(item.get("fetchedSources") for item in all_sources):
    errors.append(f"webSources missing fetchedSources: {all_sources}")

fetched = [source for trace in all_sources for source in (trace.get("fetchedSources") or [])]
if not any(item.get("contentHash") and item.get("evidenceRef") for item in fetched):
    errors.append(f"fetched source missing hash/evidence: {fetched}")
if not any(trace.get("policyDecision") == "allowed" and trace.get("evidenceRefs") for trace in all_sources):
    errors.append(f"web source trace missing allowed policy/evidence: {all_sources}")
if any((item.get("content") or item.get("summary", "")).startswith("This source confirms") for item in fetched):
    errors.append(f"fetched source exposed raw content in read model: {fetched}")

artifacts = snapshot.get("artifacts") or []
if not any("web-source-index" in (item.get("uri") or "") for item in artifacts):
    errors.append(f"missing web-source-index artifact: {artifacts}")

topics = {item.get("topic") for item in events}
if "/step/state/changed" not in topics:
    errors.append(f"event stream missing step state changed topic: {topics}")
event_payloads = [item.get("payload") or {} for item in events]
if not any(payload.get("kind") in {"web/search", "web/fetch"} for payload in event_payloads):
    errors.append(f"event stream missing web step payload: {event_payloads}")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY
}

trap report_failure ERR
trap cleanup EXIT

start_llm_mock
start_web_mock
wait_mocks
wait_ready

curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
write_configuration_payloads
curl -fsS -X PUT "$BASE_URL/api/v1/workorch/configuration/llm" -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/updated-llm.json" >"$TMP_DIR/configuration-llm-updated.json"
curl -fsS -X PUT "$BASE_URL/api/v1/workorch/configuration/web-search" -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/updated-web-search.json" >"$TMP_DIR/configuration-web-search-updated.json"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
  -d '{"title":"web search e2e workspace","description":"verify governed Web Search source traceability","templateId":"generic-governed"}' >"$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"verify governed Web Search source traceability"}' >"$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
[[ -n "$goal_id" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["web/search and web/fetch produce traceable sources"]}' >"$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"
[[ -n "$dod_id" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/souls" -H 'Content-Type: application/json' \
  -d '{"id":"'"$SOUL_ID"'","name":"Web Search E2E Soul","description":"Allow governed web search and fetch execution for Web Search e2e verification.","businessDomains":["workorch","web"],"soulMd":"# Web Search E2E Soul\n\nExecute governed web search and fetch steps and produce evidence-backed DoD assessment.\n\n## Planning\n\ngoal-first\n\n## Decision\n\nask only when information is missing\n\n## Approval\n\nrespect runtime governance for risky actions\n\n## Validation\n\nrequire concrete source evidence\n\n## Output Contract\n\nworkorch llm step contract","allowedToolKinds":["llm/chat","web/search","web/fetch"]}' >"$TMP_DIR/soul.json"

activation_request_id="web-search-e2e-$VERIFY_SUFFIX"
activation_reason="e2e_resume"
capture_runs_before_activation "$TMP_DIR/activation-before-runs.json"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  -H 'Content-Type: application/json' \
  -d '{"intent":"resume","requestId":"'"$activation_request_id"'","reason":"'"$activation_reason"'","source":"e2e","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmTarget":{"providerId":"'"$LLM_PROVIDER_ID"'","modelId":"web-search-e2e-model"}","soulId":"'"$SOUL_ID"'","soulBindings":{"planning":"'"$SOUL_ID"'","execution":"'"$SOUL_ID"'","decision":"'"$SOUL_ID"'","approval":"'"$SOUL_ID"'","validation":"'"$SOUL_ID"'","artifact":"'"$SOUL_ID"'"},"priority":2}' >"$TMP_DIR/activation.json"
validate_activation_command_response "$TMP_DIR/activation.json" "$activation_request_id" "$activation_reason"
run_id="$(wait_activation_run "$TMP_DIR/activation-before-runs.json" "$TMP_DIR/activation-runs.json" "$TMP_DIR/activation-run.json")"
[[ -n "$run_id" ]]

wait_run_state "$workspace_id" "$run_id" "completed" "$TMP_DIR/run-completed.json"
assert_web_search_traceability "$workspace_id" "$run_id"

echo "workorch Web Search e2e verification passed for deployed workorch service $BASE_URL run=$run_id"
