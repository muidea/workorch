#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_CONTRACT_VIOLATION_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-}}"
source "$ROOT_DIR/scripts/lib/verify-isolated-runtime.sh"
workorch_verify_require_isolated_runtime "$BASE_URL" "verify-contract-violation-recovery-e2e"
TMP_DIR="${WORKORCH_CONTRACT_VIOLATION_VERIFY_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_TMP="${WORKORCH_VERIFY_KEEP_TMP:-0}"
MOCK_ADDR="${WORKORCH_CONTRACT_VIOLATION_VERIFY_MOCK_ADDR:-}"
MOCK_BASE_URL=""
VERIFY_SUFFIX="${WORKORCH_CONTRACT_VIOLATION_VERIFY_SUFFIX:-$$}"
CURL_CONNECT_TIMEOUT="${WORKORCH_VERIFY_CURL_CONNECT_TIMEOUT_SECONDS:-5}"
CURL_MAX_TIME="${WORKORCH_VERIFY_CURL_MAX_TIME_SECONDS:-30}"
SCRIPT_TIMEOUT_SECONDS="${WORKORCH_CONTRACT_VIOLATION_VERIFY_TIMEOUT_SECONDS:-240}"
RUN_WAIT_SECONDS="${WORKORCH_CONTRACT_VIOLATION_VERIFY_RUN_WAIT_SECONDS:-90}"
ACTIVATION_RUN_WAIT_SECONDS="${WORKORCH_CONTRACT_VIOLATION_VERIFY_ACTIVATION_RUN_WAIT_SECONDS:-60}"
PROVIDER_ID="verify-contract-violation-$VERIFY_SUFFIX"
MOCK_PID=""
WATCHDOG_PID=""
MOCK_LOG="$TMP_DIR/llm-mock.log"
workspace_id=""

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

timeout_failure() {
  echo "contract violation recovery e2e verification timed out after ${SCRIPT_TIMEOUT_SECONDS}s for deployed workorch service $BASE_URL" >&2
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
    echo "contract violation recovery e2e verification exceeded ${SCRIPT_TIMEOUT_SECONDS}s; terminating" >&2
    kill -TERM "$parent_pid" >/dev/null 2>&1 || true
  ) &
  WATCHDOG_PID=$!
}

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "contract violation recovery e2e verification failed for deployed workorch service $BASE_URL" >&2
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
  if [[ -n "${workspace_id:-}" ]]; then
    curl -fsS -X DELETE "$BASE_URL/api/v1/workorch/workspaces/$workspace_id?purge=true" >/dev/null 2>&1 || true
  fi
  if [[ -n "$MOCK_PID" ]]; then
    kill "$MOCK_PID" >/dev/null 2>&1 || true
    wait "$MOCK_PID" >/dev/null 2>&1 || true
  fi
  if [[ "$KEEP_TMP" == "1" || "$KEEP_TMP" == "true" || $status -ne 0 ]]; then
    echo "keeping verification temp dir: $TMP_DIR" >&2
  else
    rm -rf "$TMP_DIR"
  fi
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

attempts = {}

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
        "runKey": (workspace_id + "/" + run_id) if workspace_id and run_id else run_id,
        "stepId": step_id,
        "hasRecovery": "contractViolationRecovery" in merged,
    }

def complete_assessment(step_id):
    evidence = [step_id or "contract-violation-recovery-step"]
    return {
        "thoughtSummary": "The corrected response satisfies the same runtime contract with traceable evidence.",
        "completionState": "completed",
        "dodAssessment": {
            "satisfied": True,
            "summary": "The neutral contract violation recovery path completed.",
            "items": [
                {
                    "item": "contract violation recovery completes without manual replan",
                    "state": "satisfied",
                    "evidenceRefs": evidence,
                }
            ],
            "goalCoverage": {
                "satisfied": True,
                "items": [
                    {
                        "requirement": "verify recoverable contract violation is retried automatically",
                        "state": "satisfied",
                        "evidenceRefs": evidence,
                    }
                ],
            },
        },
    }

def violating_assessment():
    return {
        "thoughtSummary": "This intentionally violates the neutral evidence-ref contract.",
        "completionState": "completed",
        "dodAssessment": {
            "satisfied": True,
            "summary": "Goal coverage intentionally omits evidenceRefs.",
            "items": [
                {
                    "item": "contract violation recovery completes without manual replan",
                    "state": "satisfied",
                    "evidenceRefs": ["initial-assessment-step"],
                }
            ],
            "goalCoverage": {
                "satisfied": True,
                "items": [
                    {
                        "requirement": "verify recoverable contract violation is retried automatically",
                        "state": "satisfied",
                    }
                ],
            },
        },
    }

def decision_payload(request):
    if request.get("runId") == "probe":
        return {"completionState": "failed", "thoughtSummary": "probe"}
    key = request.get("runKey") or "global"
    attempts[key] = attempts.get(key, 0) + 1
    if request.get("hasRecovery"):
        return complete_assessment(request.get("stepId"))
    if attempts[key] == 1:
        return violating_assessment()
    return complete_assessment(request.get("stepId"))

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
        request = provider_request(payload)
        decision = decision_payload(request)
        print("runId=%s stepId=%s recovery=%s state=%s" % (
            request.get("runId", ""),
            request.get("stepId", ""),
            request.get("hasRecovery", False),
            decision.get("completionState", ""),
        ), flush=True)
        content = json.dumps(decision, separators=(",", ":"))
        response = {
            "choices": [{"message": {"content": content}}],
            "usage": {
                "prompt_tokens": 96,
                "completion_tokens": 32,
                "total_tokens": 128,
            },
        }
        encoded = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)

host, port_text = os.environ.get("WORKORCH_CONTRACT_VIOLATION_VERIFY_MOCK_ADDR", "127.0.0.1:18189").split(":", 1)
ThreadingHTTPServer((host, int(port_text)), Handler).serve_forever()
'''
open(sys.stdout.fileno(), "w", encoding="utf-8", closefd=False).write(source.replace("127.0.0.1:18189", mock_addr))
PY
  WORKORCH_CONTRACT_VIOLATION_VERIFY_MOCK_ADDR="$MOCK_ADDR" python3 "$TMP_DIR/llm_mock_provider.py" >>"$MOCK_LOG" 2>&1 &
  MOCK_PID=$!
}

wait_mock() {
  for _ in $(seq 1 50); do
    if curl -fsS -X POST "$MOCK_BASE_URL/chat/completions" \
      -H 'Content-Type: application/json' \
      -d '{"messages":[{"role":"user","content":"{\"runId\":\"probe\",\"stepId\":\"probe\"}"}]}' >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl -fsS -X POST "$MOCK_BASE_URL/chat/completions" \
    -H 'Content-Type: application/json' \
    -d '{"messages":[{"role":"user","content":"{\"runId\":\"probe\",\"stepId\":\"probe\"}"}]}' >/dev/null
}

wait_ready() {
  for _ in $(seq 1 60); do
    if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  echo "workorch service did not become ready at $BASE_URL" >&2
  exit 1
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

wait_activation_run() {
  local activation_file="$1"
  local workspace_id="$2"
  local before_runs_file="$3"
  local output_file="$4"
  local deadline=$((SECONDS + ACTIVATION_RUN_WAIT_SECONDS))
  local run_id=""

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
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" >"$output_file"
    run_id="$(python3 - "$output_file" "$before_runs_file" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
before = set()
try:
    before_data = json.load(open(sys.argv[2], encoding="utf-8"))
    before = {str(item.get("id") or "") for item in before_data.get("runs", []) if item.get("id")}
except FileNotFoundError:
    before = set()
runs = data.get("runs") or []
new_runs = [
    item for item in runs
    if item.get("id") and str(item.get("id")) not in before
]
if new_runs:
    print(new_runs[-1].get("id") or "")
    raise SystemExit(0)
raise SystemExit(1)
PY
)" && {
      echo "$run_id"
      return 0
    }
    sleep 1
  done

  echo "activation accepted but run did not materialize for workspace $workspace_id" >&2
  cat "$activation_file" >&2 || true
  [[ -f "$output_file" ]] && cat "$output_file" >&2
  return 1
}

wait_run_terminal() {
  local run_id="$1"
  local workspace_id="$2"
  local output_file="$3"
  local deadline=$((SECONDS + RUN_WAIT_SECONDS))
  local state=""

  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$output_file"
    state="$(python3 - "$output_file" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
print((data.get("status") or {}).get("state") or "")
PY
)"
    case "$state" in
      completed|failed|canceled|blocked_on_decision|blocked_on_approval)
        echo "$state"
        return 0
        ;;
    esac
    sleep 1
  done

  echo "timeout waiting for run $run_id to reach terminal/blocking state" >&2
  cat "$output_file" >&2 || true
  return 1
}

validate_result() {
  python3 - "$TMP_DIR/status.json" "$TMP_DIR/snapshot.json" "$TMP_DIR/events.json" "$MOCK_LOG" "$workspace_id" "$run_id" <<'PY'
import json
import pathlib
import sys

status_path, snapshot_path, events_path, mock_log_path, workspace_id, run_id = sys.argv[1:]
status = json.load(open(status_path, encoding="utf-8")).get("status") or {}
snapshot = json.load(open(snapshot_path, encoding="utf-8")).get("snapshot") or {}
events = json.load(open(events_path, encoding="utf-8")).get("eventRecords") or []
mock_log = pathlib.Path(mock_log_path).read_text(encoding="utf-8", errors="replace")
errors = []

if status.get("state") != "completed":
    errors.append(f"status state = {status.get('state')!r}, want completed")
run = snapshot.get("run") or {}
if run.get("id") != run_id or run.get("workspaceId") != workspace_id:
    errors.append(f"snapshot identity mismatch: {run}")
if run.get("state") != "completed":
    errors.append(f"snapshot run state = {run.get('state')!r}, want completed")

topics = [item.get("topic") for item in events]
for topic in [
    "/orchestrator/contract_violation/recorded",
    "/orchestrator/contract_violation/retry_requested",
]:
    if topic not in topics:
        errors.append(f"event stream missing {topic}; present={sorted(set(topics))}")

serialized_snapshot = json.dumps(snapshot, ensure_ascii=False)
serialized_events = json.dumps(events, ensure_ascii=False)
if "contract violation recovery completes without manual replan" not in serialized_snapshot:
    errors.append("completed snapshot missing neutral DoD item")
if "dod_assessment_missing_evidence_refs" not in serialized_events and "dod_assessment_missing_evidence_refs" not in serialized_snapshot:
    errors.append("missing contract violation rule dod_assessment_missing_evidence_refs")
if '"state":"blocked_on_decision"' in serialized_snapshot or '"state":"blocked_on_decision"' in serialized_events:
    errors.append("recoverable contract violation entered blocked_on_decision")
if mock_log.count("runId=") < 3:
    errors.append(f"mock provider was called too few times: {mock_log}")
if "recovery=True" not in mock_log:
    errors.append(f"mock provider did not receive contractViolationRecovery prompt: {mock_log}")
if "recovery=False" not in mock_log:
    errors.append(f"mock provider did not record initial violating request: {mock_log}")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
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

curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
write_llm_update_payloads
curl -fsS -X PUT "$BASE_URL/api/v1/workorch/configuration/llm" -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/verify-llm-update.json" >"$TMP_DIR/configuration-updated.json"
grep '"apiKeyConfigured": true' "$TMP_DIR/configuration-updated.json" >/dev/null

workspace_workdir="$TMP_DIR/workspace-deliverable"
mkdir -p "$workspace_workdir"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
  -d '{"title":"contract violation recovery e2e","description":"verify recoverable contract violation is retried automatically","deliverableDirectory":"'"$workspace_workdir"'","templateId":"generic-governed"}' >"$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"verify recoverable contract violation is retried automatically"}' >"$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["contract violation recovery completes without manual replan"]}' >"$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"

python3 - "$TMP_DIR/activation-request.json" "$goal_id" "$dod_id" "$PROVIDER_ID" "$VERIFY_SUFFIX" <<'PY'
import json
import sys

path, goal_id, dod_id, provider_id, suffix = sys.argv[1:]
payload = {
    "intent": "resume",
    "requestId": f"contract-violation-e2e-{suffix}",
    "reason": "e2e_resume",
    "source": "e2e",
    "goalId": goal_id,
    "doDId": dod_id,
    "llmTarget": {"providerId": provider_id, "modelId": "verify-model"},
    "priority": 1,
}
with open(path, "w", encoding="utf-8") as handle:
    json.dump(payload, handle, separators=(",", ":"))
PY

curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" >"$TMP_DIR/activation-before-runs.json"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/activation-request.json" >"$TMP_DIR/activation.json"
run_id="$(wait_activation_run "$TMP_DIR/activation.json" "$workspace_id" "$TMP_DIR/activation-before-runs.json" "$TMP_DIR/activation-runs.json")"
[[ -n "$run_id" ]]

terminal_state="$(wait_run_terminal "$run_id" "$workspace_id" "$TMP_DIR/status.json")"
if [[ "$terminal_state" != "completed" ]]; then
  echo "run $run_id reached state $terminal_state, want completed" >&2
  exit 1
fi

curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$TMP_DIR/status.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/snapshot" >"$TMP_DIR/snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/events?workspaceId=$workspace_id&runId=$run_id&limit=200" >"$TMP_DIR/events.json"
validate_result

echo "workorch contract violation recovery e2e verification passed for deployed workorch service $BASE_URL workspace=$workspace_id run=$run_id"
