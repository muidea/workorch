#!/usr/bin/env bash

set -euo pipefail

TMP_DIR="$(mktemp -d)"
BASE_URL="${WORKORCH_WEB_SEARCH_E2E_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
LLM_MOCK_ADDR="${WORKORCH_WEB_SEARCH_E2E_LLM_MOCK_ADDR:-127.0.0.1:18191}"
WEB_MOCK_ADDR="${WORKORCH_WEB_SEARCH_E2E_WEB_MOCK_ADDR:-127.0.0.1:18192}"
LLM_MOCK_BASE_URL="http://$LLM_MOCK_ADDR/v1"
WEB_MOCK_BASE_URL="http://$WEB_MOCK_ADDR"
VERIFY_SUFFIX="${WORKORCH_WEB_SEARCH_E2E_SUFFIX:-$$}"
LLM_PROVIDER_ID="web-search-e2e-llm-$VERIFY_SUFFIX"
WEB_PROVIDER_ID="web-search-e2e-web-$VERIFY_SUFFIX"
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
    "defaultProviderId": llm.get("defaultProviderId") or "",
    "defaultEmbeddingProviderId": llm.get("defaultEmbeddingProviderId") or "",
    "providers": [clean_llm(item) for item in llm.get("providers") or [] if item.get("id")],
}
if not original_llm["providers"]:
    original_llm = {
        "defaultProviderId": llm_provider_id,
        "defaultEmbeddingProviderId": "",
        "providers": [{
            "id": llm_provider_id,
            "provider": "compatible",
            "baseUrl": llm_base_url,
            "model": "web-search-e2e-model",
            "timeout": "60s",
            "disabled": False,
            "chatCompletions": True,
            "streaming": False,
            "nonChatOperations": [],
        }],
    }
updated_llm_providers = [item for item in original_llm["providers"] if item["id"] != llm_provider_id]
updated_llm_providers.append({
    "id": llm_provider_id,
    "provider": "compatible",
    "baseUrl": llm_base_url,
    "apiKey": "web-search-e2e-secret",
    "model": "web-search-e2e-model",
    "timeout": "60s",
    "disabled": False,
    "chatCompletions": True,
    "streaming": False,
    "nonChatOperations": ["embeddings"],
})
updated_llm = {
    "defaultProviderId": llm_provider_id,
    "defaultEmbeddingProviderId": llm_provider_id,
    "providers": updated_llm_providers,
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
  local run_id="$1"
  local expected_state="$2"
  local output_file="$3"
  local deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/status?waitState=$expected_state&waitTimeoutMillis=1000&waitIntervalMillis=100" >"$output_file"
    if grep '"state": "'"$expected_state"'"' "$output_file" >/dev/null; then
      return 0
    fi
    sleep 1
  done
  cat "$output_file" >&2
  return 1
}

assert_web_search_traceability() {
  local run_id="$1"
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/snapshot" >"$TMP_DIR/run-snapshot.json"
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/observability" >"$TMP_DIR/run-observability.json"
  curl -fsS "$BASE_URL/api/v1/workorch/events?runId=$run_id&limit=100" >"$TMP_DIR/run-events.json"
  python3 - "$TMP_DIR/run-snapshot.json" "$TMP_DIR/run-observability.json" "$TMP_DIR/run-events.json" "$run_id" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8")).get("snapshot") or {}
observability = json.load(open(sys.argv[2], encoding="utf-8")).get("observability") or {}
events = json.load(open(sys.argv[3], encoding="utf-8")).get("eventRecords") or []
run_id = sys.argv[4]
errors = []

run = snapshot.get("run") or {}
if run.get("id") != run_id or run.get("state") != "completed":
    errors.append(f"run state invalid: {run}")
routing = (run.get("executionContext") or {}).get("soulRouting") or {}
bindings = routing.get("bindings") or {}
execution_binding = bindings.get("execution") or {}
if routing.get("status") != "resolved":
    errors.append(f"soulRouting.status is {routing.get('status')!r}, want resolved")
if execution_binding.get("soulId") != "soul-generic-governed-execution":
    errors.append(f"execution soul binding is {execution_binding!r}, want soul-generic-governed-execution")

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

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  -H 'Content-Type: application/json' \
  -d '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"'"$LLM_PROVIDER_ID"'","priority":2}' >"$TMP_DIR/activation.json"
run_id="$(json_path "$TMP_DIR/activation.json" "activation.run.id")"
[[ -n "$run_id" ]]

wait_run_state "$run_id" "completed" "$TMP_DIR/run-completed.json"
assert_web_search_traceability "$run_id"

echo "workorch Web Search e2e verification passed for deployed workorch service $BASE_URL run=$run_id"
