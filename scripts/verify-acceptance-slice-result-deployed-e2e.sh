#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
KEEP_TMP="${WORKORCH_VERIFY_KEEP_TMP:-0}"
BASE_URL="${WORKORCH_ACCEPTANCE_SLICE_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
MOCK_ADDR="${WORKORCH_ACCEPTANCE_SLICE_VERIFY_MOCK_ADDR:-127.0.0.1:18193}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_ACCEPTANCE_SLICE_VERIFY_SUFFIX:-$$}"
ACTIVATION_RUN_WAIT_SECONDS="${WORKORCH_ACCEPTANCE_SLICE_ACTIVATION_RUN_WAIT_SECONDS:-30}"
WORKSPACE_ID=""
MOCK_PID=""
MOCK_LOG="$TMP_DIR/llm-mock.log"

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "workorch acceptance slice deployed e2e failed for service $BASE_URL" >&2
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

trap report_failure ERR
trap cleanup EXIT

start_mock_provider() {
  cat >"$TMP_DIR/llm_mock_provider.py" <<'PY'
import json
import os
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

run_calls = {}

def request_identity(payload):
    merged = "\n".join(str(item.get("content", "")) for item in payload.get("messages") or [])
    run_matches = re.findall(r'"runId"\s*:\s*"([^"]+)"', merged)
    step_matches = re.findall(r'"stepId"\s*:\s*"([^"]+)"', merged)
    workspace_matches = re.findall(r'"workspaceId"\s*:\s*"([^"]+)"', merged)
    run_id = run_matches[-1] if run_matches else "probe"
    step_id = step_matches[-1] if step_matches else run_id + "/step"
    workspace_id = workspace_matches[-1] if workspace_matches else ""
    return workspace_id, run_id, step_id

def goal_coverage(step_id, second_state):
    return {
        "satisfied": second_state == "satisfied",
        "items": [
            {"requirement": "first slice item", "state": "satisfied", "evidenceRefs": [step_id]},
            {"requirement": "second slice item", "state": second_state, "evidenceRefs": [step_id] if second_state == "satisfied" else [], "reason": "second slice remains open" if second_state != "satisfied" else ""},
        ],
    }

def decision_payload(workspace_id, run_id, step_id):
    if run_id == "probe":
        return {"completionState": "failed", "thoughtSummary": "probe"}
    calls = run_calls.get(run_id, 0) + 1
    run_calls[run_id] = calls
    if calls == 1:
        return {
            "thoughtSummary": "Close only the first acceptance slice and leave the second slice for repair.",
            "completionState": "completed",
            "dodAssessment": {
                "satisfied": True,
                "summary": "First acceptance slice is satisfied; second slice still needs evidence.",
                "items": [
                    {"item": "first slice item", "state": "satisfied", "evidenceRefs": [step_id]},
                    {"item": "second slice item", "state": "failed", "evidenceRefs": [], "reason": "second slice still open"},
                ],
                "goalCoverage": goal_coverage(step_id, "failed"),
            },
        }
    return {
        "thoughtSummary": "Repair closes the remaining acceptance slice.",
        "completionState": "completed",
        "dodAssessment": {
            "satisfied": True,
            "summary": "Both acceptance slices are satisfied.",
            "items": [
                {"item": "first slice item", "state": "satisfied", "evidenceRefs": [step_id]},
                {"item": "second slice item", "state": "satisfied", "evidenceRefs": [step_id]},
            ],
            "goalCoverage": goal_coverage(step_id, "satisfied"),
        },
    }

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path == "/v1/embeddings":
            encoded = json.dumps({"data": [{"embedding": [0.1, 0.2, 0.3]}]}).encode("utf-8")
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
        workspace_id, run_id, step_id = request_identity(payload)
        decision = decision_payload(workspace_id, run_id, step_id)
        print(f"runId={run_id} stepId={step_id} call={run_calls.get(run_id, 0)} state={decision.get('completionState')}", flush=True)
        content = json.dumps(decision, separators=(",", ":"))
        response = {"choices": [{"message": {"content": content}}], "usage": {"prompt_tokens": 64, "completion_tokens": 16, "total_tokens": 80}}
        encoded = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)

host, port_text = os.environ["WORKORCH_ACCEPTANCE_SLICE_VERIFY_MOCK_ADDR"].split(":", 1)
ThreadingHTTPServer((host, int(port_text)), Handler).serve_forever()
PY
  WORKORCH_ACCEPTANCE_SLICE_VERIFY_MOCK_ADDR="$MOCK_ADDR" python3 "$TMP_DIR/llm_mock_provider.py" >>"$MOCK_LOG" 2>&1 &
  MOCK_PID=$!
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

wait_mock() {
  for _ in $(seq 1 50); do
    if curl -fsS -X POST "http://$MOCK_ADDR/v1/chat/completions" \
      -H 'Content-Type: application/json' \
      -d '{"messages":[{"role":"user","content":"{\"runId\":\"probe\",\"stepId\":\"probe\"}"}]}' >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl -fsS -X POST "http://$MOCK_ADDR/v1/chat/completions" \
    -H 'Content-Type: application/json' \
    -d '{"messages":[{"role":"user","content":"{\"runId\":\"probe\",\"stepId\":\"probe\"}"}]}' >/dev/null
}

write_llm_update_payloads() {
  python3 - "$TMP_DIR/configuration.json" "$TMP_DIR/original-llm-update.json" "$TMP_DIR/verify-llm-update.json" "$MOCK_BASE_URL" <<'PY'
import json
import sys

config_path, original_path, update_path, mock_base_url = sys.argv[1:]
data = json.load(open(config_path, encoding="utf-8"))
llm = data.get("configuration", {}).get("llm", {})
providers = llm.get("providers") or []
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

providers = [clean_provider(item) for item in providers if isinstance(item, dict) and item.get("id")]
original = {
    "defaultProviderId": llm.get("defaultProviderId") or "",
    "defaultEmbeddingProviderId": llm.get("defaultEmbeddingProviderId") or "",
    "modelCapabilities": model_capabilities,
    "providers": providers,
}
verify = {
    "defaultProviderId": "acceptance-slice-verify",
    "defaultEmbeddingProviderId": llm.get("defaultEmbeddingProviderId") or "",
    "modelCapabilities": model_capabilities,
    "providers": providers + [{
        "id": "acceptance-slice-verify",
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
    }],
}
json.dump(original, open(original_path, "w", encoding="utf-8"), ensure_ascii=False)
json.dump(verify, open(update_path, "w", encoding="utf-8"), ensure_ascii=False)
PY
}

json_field() {
  python3 - "$1" "$2" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
current = data
for part in sys.argv[2].split("."):
    current = current[part]
print(current)
PY
}

capture_runs_before_activation() {
  local output_file="$1"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs" >"$output_file"
}

wait_activation_run() {
  local activation_file="$1"
  local before_runs_file="$2"
  local output_file="$3"
  local runs_file="$TMP_DIR/activation-runs.json"
  local run_id=""
  local deadline=$((SECONDS + ACTIVATION_RUN_WAIT_SECONDS))

  run_id="$(python3 - "$activation_file" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
for path in (("activation", "run", "id"), ("run", "id")):
    current = data
    for part in path:
        if not isinstance(current, dict):
            current = {}
            break
        current = current.get(part)
    if current:
        print(current)
        raise SystemExit(0)
raise SystemExit(1)
PY
)" && {
    python3 - "$activation_file" "$run_id" "$output_file" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
run_id = sys.argv[2]
run = ((data.get("activation") or {}).get("run") or data.get("run") or {"id": run_id})
json.dump({"run": run}, open(sys.argv[3], "w", encoding="utf-8"), ensure_ascii=False, indent=2)
PY
    echo "$run_id"
    return 0
  }

  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs" >"$runs_file"; then
      run_id="$(python3 - "$runs_file" "$before_runs_file" <<'PY'
import json
import sys

runs = json.load(open(sys.argv[1], encoding="utf-8")).get("runs") or []
before = set()
try:
    before_data = json.load(open(sys.argv[2], encoding="utf-8"))
    before = {str(item.get("id") or "") for item in before_data.get("runs") or [] if item.get("id")}
except FileNotFoundError:
    before = set()
new_runs = [item for item in runs if item.get("id") and str(item.get("id")) not in before]
if new_runs:
    print(new_runs[-1]["id"])
    raise SystemExit(0)
raise SystemExit(1)
PY
)" && {
        python3 - "$runs_file" "$run_id" "$output_file" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
run_id = sys.argv[2]
run = next((item for item in data.get("runs") or [] if item.get("id") == run_id), {"id": run_id})
json.dump({"run": run}, open(sys.argv[3], "w", encoding="utf-8"), ensure_ascii=False, indent=2)
PY
        echo "$run_id"
        return 0
      }
    fi
    sleep 1
  done

  echo "activation accepted but run did not materialize within ${ACTIVATION_RUN_WAIT_SECONDS}s for workspace $WORKSPACE_ID" >&2
  cat "$activation_file" >&2 || true
  [[ -f "$runs_file" ]] && cat "$runs_file" >&2
  return 1
}

wait_run_completed() {
  local run_id="$1"
  for _ in $(seq 1 120); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$run_id/status" >"$TMP_DIR/run-status.json"
    if python3 - "$TMP_DIR/run-status.json" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
raise SystemExit(0 if ((data.get("status") or {}).get("state") == "completed") else 1)
PY
    then
      return 0
    fi
    sleep 0.5
  done
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$run_id/snapshot" >"$TMP_DIR/run-snapshot-timeout.json" || true
  return 1
}

validate_results() {
  python3 - "$TMP_DIR/results.json" "$TMP_DIR/run-snapshot.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
snapshot = json.load(open(sys.argv[2], encoding="utf-8"))
results = data.get("results") or []
slice_results = [item for item in results if item.get("sourceType") == "acceptance_slice" and item.get("state") == "accepted"]
final_results = [item for item in results if item.get("sourceType") == "dod_assessment" and item.get("state") == "accepted"]
if len(slice_results) < 1:
    raise SystemExit(f"missing accepted acceptance_slice result: {results!r}")
if len(final_results) != 1:
    raise SystemExit(f"missing exactly one final accepted dod_assessment result: {results!r}")
snapshot_results = ((snapshot.get("snapshot") or {}).get("results") or snapshot.get("results") or [])
snapshot_slice_results = [item for item in snapshot_results if item.get("sourceType") == "acceptance_slice" and item.get("state") == "accepted"]
if not snapshot_slice_results:
    raise SystemExit(f"missing accepted acceptance_slice result in snapshot detail: {snapshot_results!r}")
diagnostics = ((snapshot_slice_results[0].get("summaryDetail") or {}).get("diagnostics") or {})
if diagnostics.get("resultScope") != "acceptance_slice" or diagnostics.get("finalCompletionGateState") != "not_final_result":
    raise SystemExit(f"slice diagnostics not projected: {diagnostics!r}")
snapshot_final_results = [item for item in snapshot_results if item.get("sourceType") == "dod_assessment" and item.get("state") == "accepted"]
if len(snapshot_final_results) != 1:
    raise SystemExit(f"missing exactly one final accepted dod_assessment result in snapshot detail: {snapshot_results!r}")
assessment = snapshot_final_results[0].get("doDAssessment") or {}
items = assessment.get("items") or []
if not assessment.get("satisfied") or len(items) < 2:
    raise SystemExit(f"final assessment incomplete: {assessment!r}")
if any((item.get("state") or "") != "satisfied" for item in items):
    raise SystemExit(f"final assessment has non-satisfied item: {items!r}")
PY
}

mkdir -p "$TMP_DIR"
start_mock_provider
wait_mock
wait_ready

curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
write_llm_update_payloads
curl -fsS -H "Content-Type: application/json" -X PUT \
  "$BASE_URL/api/v1/workorch/configuration/llm" \
  -d @"$TMP_DIR/verify-llm-update.json" >"$TMP_DIR/llm-update.json"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
  -d '{"title":"acceptance slice deployed e2e '"$VERIFY_SUFFIX"'","description":"verify partial slice result and final closure","templateId":"generic-governed"}' >"$TMP_DIR/workspace.json"
WORKSPACE_ID="$(json_field "$TMP_DIR/workspace.json" workspace.id)"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/goals" -H 'Content-Type: application/json' \
  -d '{"text":"first slice item\nsecond slice item"}' >"$TMP_DIR/goal.json"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/dods" -H 'Content-Type: application/json' \
  -d '{"items":["first slice item","second slice item"]}' >"$TMP_DIR/dod.json"

goal_id="$(json_field "$TMP_DIR/goal.json" goal.id)"
dod_id="$(json_field "$TMP_DIR/dod.json" dod.id)"
capture_runs_before_activation "$TMP_DIR/activation-before-runs.json"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/execution-activations" \
  -H 'Content-Type: application/json' \
  -d '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"acceptance-slice-verify","executionContextPatch":{"executionContract":{"acceptanceSlicePolicy":{"schemaVersion":"workorch.acceptance_slice_policy.v1","maxWorkItems":1,"completionPolicy":"all_required_items_accepted","resultPolicy":"slice_result_allowed"}}}}' >"$TMP_DIR/activation.json"

run_id="$(wait_activation_run "$TMP_DIR/activation.json" "$TMP_DIR/activation-before-runs.json" "$TMP_DIR/activation-run.json")"
wait_run_completed "$run_id"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$run_id/results?limit=100" >"$TMP_DIR/results.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$run_id/snapshot" >"$TMP_DIR/run-snapshot.json"
validate_results

echo "acceptance slice deployed e2e verification passed for deployed workorch service $BASE_URL workspace=$WORKSPACE_ID run=$run_id"
