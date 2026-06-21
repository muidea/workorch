#!/usr/bin/env bash

set -euo pipefail

BASE_URL="${WORKORCH_REAL_VECTOR_MEMORY_BASE_URL:-http://127.0.0.1:8080}"
MOCK_ADDR="${WORKORCH_REAL_VECTOR_MEMORY_MOCK_ADDR:-127.0.0.1:18189}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
PROVIDER_ID="${WORKORCH_REAL_VECTOR_MEMORY_PROVIDER_ID:-verify_vector_memory_$$}"
MATCH_MEMORY_ID="${WORKORCH_REAL_VECTOR_MEMORY_MATCH_ID:-memory-semantic-match-$$}"
OTHER_MEMORY_ID="${WORKORCH_REAL_VECTOR_MEMORY_OTHER_ID:-memory-semantic-other-$$}"
TMP_DIR="${WORKORCH_REAL_VECTOR_MEMORY_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_REAL_VECTOR_MEMORY_KEEP_ARTIFACTS:-1}"
RUN_WAIT_SECONDS="${WORKORCH_REAL_VECTOR_MEMORY_RUN_WAIT_SECONDS:-180}"
ACTIVATION_RUN_WAIT_SECONDS="${WORKORCH_REAL_VECTOR_MEMORY_ACTIVATION_RUN_WAIT_SECONDS:-60}"
MOCK_PID=""
MOCK_LOG="$TMP_DIR/vector-memory-mock.log"

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
  echo "real service vector Memory recall verification failed" >&2
  echo "artifacts=$TMP_DIR" >&2
  if [[ -f "$MOCK_LOG" ]]; then
    echo "--- vector-memory-mock.log ---" >&2
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

wait_run_state() {
  local run_id="$1"
  local expected_state="$2"
  local output_file="$3"
  local deadline=$((SECONDS + RUN_WAIT_SECONDS))
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

start_mock_provider() {
  cat >"$TMP_DIR/vector_memory_mock_provider.py" <<'PY'
import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        size = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(size) or b"{}")
        if self.path == "/v1/embeddings":
            response = {"data": [{"embedding": [1.0, 0.0, 0.0]}]}
        elif self.path == "/v1/chat/completions":
            decision = {
                "thoughtSummary": "Vector Memory recall selected the matching workspace context.",
                "completionState": "completed",
                "dodAssessment": {
                    "satisfied": True,
                    "summary": "Semantic vector Memory recall selected the expected memory item.",
                    "items": [{
                        "item": "semantic vector memory recall selects the matching memory",
                        "state": "satisfied",
                        "evidenceRefs": [os.environ.get("WORKORCH_REAL_VECTOR_MEMORY_MATCH_ID", "memory-semantic-match")]
                    }]
                }
            }
            response = {"choices": [{"message": {"content": json.dumps(decision, separators=(",", ":"))}}]}
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

host, port_text = os.environ.get("WORKORCH_REAL_VECTOR_MEMORY_MOCK_ADDR", "127.0.0.1:18189").split(":", 1)
ThreadingHTTPServer((host, int(port_text)), Handler).serve_forever()
PY
  WORKORCH_REAL_VECTOR_MEMORY_MOCK_ADDR="$MOCK_ADDR" WORKORCH_REAL_VECTOR_MEMORY_MATCH_ID="$MATCH_MEMORY_ID" python3 "$TMP_DIR/vector_memory_mock_provider.py" >>"$MOCK_LOG" 2>&1 &
  MOCK_PID=$!
}

wait_mock() {
  for _ in $(seq 1 50); do
    if curl -fsS -X POST "$MOCK_BASE_URL/embeddings" \
      -H "Content-Type: application/json" \
      -d '{"model":"embedding-model","input":"probe"}' >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl -fsS -X POST "$MOCK_BASE_URL/embeddings" \
    -H "Content-Type: application/json" \
    -d '{"model":"embedding-model","input":"probe"}' >/dev/null
}

write_llm_update_payloads() {
  python3 - "$TMP_DIR/configuration.json" "$TMP_DIR/original-llm-update.json" "$TMP_DIR/verify-llm-update.json" "$PROVIDER_ID" "$MOCK_BASE_URL" <<'PY'
import json
import sys

config_path, original_path, update_path, provider_id, mock_base_url = sys.argv[1:]
data = json.load(open(config_path, encoding="utf-8"))
llm = data.get("configuration", {}).get("llm", {})
providers = llm.get("providers") or []
default_provider = llm.get("defaultProviderId") or provider_id

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
    "defaultEmbeddingProviderId": llm.get("defaultEmbeddingProviderId") or "",
    "providers": [clean_provider(item) for item in providers if item.get("id")]
}
with open(original_path, "w", encoding="utf-8") as handle:
    json.dump(original, handle, separators=(",", ":"))

updated_providers = [item for item in original["providers"] if item["id"] != provider_id]
updated_providers.append({
    "id": provider_id,
    "provider": "compatible",
    "baseUrl": mock_base_url,
    "apiKey": "verify-secret",
    "model": "embedding-model",
    "timeout": "30s",
    "disabled": False,
    "chatCompletions": True,
    "streaming": False,
    "nonChatOperations": ["embeddings"],
})
update = {
    "defaultProviderId": default_provider,
    "defaultEmbeddingProviderId": provider_id,
    "providers": updated_providers,
}
if not any(item["id"] == update["defaultProviderId"] for item in updated_providers):
    update["defaultProviderId"] = provider_id
with open(update_path, "w", encoding="utf-8") as handle:
    json.dump(update, handle, separators=(",", ":"))
PY
}

validate_result() {
  python3 - "$TMP_DIR/activation.json" "$TMP_DIR/snapshot.json" "$PROVIDER_ID" "$MATCH_MEMORY_ID" "$OTHER_MEMORY_ID" <<'PY'
import json
import sys

activation = json.load(open(sys.argv[1], encoding="utf-8"))["activation"]
snapshot = json.load(open(sys.argv[2], encoding="utf-8"))["snapshot"]
provider_id = sys.argv[3]
match_memory_id = sys.argv[4]
other_memory_id = sys.argv[5]
context = activation["run"].get("executionContext") or {}
retrieval = context.get("memoryRetrieval") or {}
memory_ids = context.get("memoryIds") or []
run = snapshot.get("run") or {}
routing = (run.get("executionContext") or {}).get("soulRouting") or {}
bindings = routing.get("bindings") or {}
execution_binding = bindings.get("execution") or {}
errors = []

if retrieval.get("policy") != "semantic_vector_search":
    errors.append(f"memoryRetrieval.policy is {retrieval.get('policy')!r}, want semantic_vector_search")
if retrieval.get("status") != "selected":
    errors.append(f"memoryRetrieval.status is {retrieval.get('status')!r}, want selected")
if retrieval.get("providerId") != provider_id:
    errors.append(f"memoryRetrieval.providerId is {retrieval.get('providerId')!r}, want {provider_id}")
if retrieval.get("operation") != "embeddings":
    errors.append(f"memoryRetrieval.operation is {retrieval.get('operation')!r}, want embeddings")
if retrieval.get("vectorDimension") != 3:
    errors.append(f"memoryRetrieval.vectorDimension is {retrieval.get('vectorDimension')!r}, want 3")
if not retrieval.get("queryTextSha256"):
    errors.append("memoryRetrieval.queryTextSha256 is missing")
if match_memory_id not in memory_ids:
    errors.append(f"{match_memory_id} missing from memoryIds={memory_ids}")
if other_memory_id in memory_ids:
    errors.append(f"{other_memory_id} unexpectedly selected memoryIds={memory_ids}")
if run.get("state") != "completed":
    errors.append(f"run state is {run.get('state')!r}, want completed: {run.get('failureReason', '')}")
if routing.get("status") != "resolved":
    errors.append(f"soulRouting.status is {routing.get('status')!r}, want resolved")
if execution_binding.get("soulId") != "soul-generic-governed-execution":
    errors.append(f"execution soul binding is {execution_binding!r}, want soul-generic-governed-execution")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)

print(f"workspaceId={activation['workspace']['id']}")
print(f"runId={run['id']}")
print(f"runState={run['state']}")
print(f"retrievalPolicy={retrieval['policy']}")
print(f"retrievedMemoryIds={','.join(memory_ids)}")
print(f"vectorDimension={retrieval['vectorDimension']}")
PY
}

trap report_failure ERR
trap cleanup EXIT

mkdir -p "$TMP_DIR"
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

post_json "$BASE_URL/api/v1/workorch/workspaces" \
  '{"title":"vector Memory recall verification","description":"Verify deployed vector Memory recall through default embedding provider.","templateId":"generic-governed"}' \
  "$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]

post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" \
  '{"text":"Use semantic vector memory recall to select the matching memory before planning."}' \
  "$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
[[ -n "$goal_id" ]]

post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" \
  '{"items":["semantic vector memory recall selects the matching memory"]}' \
  "$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"
[[ -n "$dod_id" ]]

post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/memory" \
  '{"memoryId":"'"$MATCH_MEMORY_ID"'","sourceType":"user","memoryType":"project_context","summary":"matching semantic memory","content":"This memory must be selected by vector recall.","embeddingProviderId":"'"$PROVIDER_ID"'","embeddingModel":"embedding-model","embeddingText":"matching semantic memory","vector":[1,0,0]}' \
  "$TMP_DIR/memory-match.json"
post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/memory" \
  '{"memoryId":"'"$OTHER_MEMORY_ID"'","sourceType":"user","memoryType":"project_context","summary":"unrelated semantic memory","content":"This memory must not be selected by vector recall.","embeddingProviderId":"'"$PROVIDER_ID"'","embeddingModel":"embedding-model","embeddingText":"unrelated semantic memory","vector":[0,1,0]}' \
  "$TMP_DIR/memory-other.json"

activation_request_id="real-vector-memory-e2e-$$"
activation_reason="e2e_resume"
capture_runs_before_activation "$TMP_DIR/activation-before-runs.json"
post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  '{"intent":"resume","requestId":"'"$activation_request_id"'","reason":"'"$activation_reason"'","source":"e2e","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"'"$PROVIDER_ID"'"}' \
  "$TMP_DIR/activation.json"
validate_activation_command_response "$TMP_DIR/activation.json" "$activation_request_id" "$activation_reason"
run_id="$(wait_activation_run "$TMP_DIR/activation-before-runs.json" "$TMP_DIR/activation-runs.json" "$TMP_DIR/activation-run.json")"
[[ -n "$run_id" ]]

wait_run_state "$run_id" "completed" "$TMP_DIR/run-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/snapshot" >"$TMP_DIR/snapshot.json"
validate_result

echo "real service vector Memory recall verification passed for provider=$PROVIDER_ID"
echo "artifacts=$TMP_DIR"
