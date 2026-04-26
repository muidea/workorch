#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
BIN_PATH="$TMP_DIR/workorchd"
ADDR="${WORKORCH_LLM_VERIFY_ADDR:-127.0.0.1:18180}"
MOCK_ADDR="${WORKORCH_LLM_VERIFY_MOCK_ADDR:-127.0.0.1:18181}"
BASE_URL="http://$ADDR"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
BACKEND="${WORKORCH_LLM_VERIFY_BACKEND:-memory}"
DATA_DIR="${WORKORCH_LLM_VERIFY_DATA_DIR:-$TMP_DIR/workorch-data}"
SERVER_PID=""
MOCK_PID=""
SERVER_LOG="$TMP_DIR/workorchd.log"
MOCK_LOG="$TMP_DIR/llm-mock.log"
CONFIG_DIR="$TMP_DIR/config"

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "workorchd LLM workspace execution verification failed for backend=$BACKEND" >&2
  if [[ -f "$SERVER_LOG" ]]; then
    echo "--- workorchd.log ---" >&2
    tail -n 200 "$SERVER_LOG" >&2 || true
  fi
  if [[ -f "$MOCK_LOG" ]]; then
    echo "--- llm-mock.log ---" >&2
    tail -n 200 "$MOCK_LOG" >&2 || true
  fi
}

cleanup() {
  if [[ -n "$SERVER_PID" ]]; then
    kill -INT "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
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

def decision_payload(request):
    run_id = request.get("runId", "")
    step_id = request.get("stepId", "")
    if run_id == "probe":
        return {"completionState": "failed", "thoughtSummary": "probe"}
    if "llm-after-decision" in step_id:
        return {
            "thoughtSummary": "Decision information provided; DoD is now satisfied.",
            "completionState": "completed",
            "memoryWrites": [{
                "memoryType": "decision",
                "summary": "Operator provided missing implementation direction.",
                "content": "Decision ticket was resolved through the LLM acceptance script.",
                "tags": ["decision", "acceptance"]
            }],
            "dodAssessment": {
                "satisfied": True,
                "summary": "Decision path completed.",
                "items": [{"item": "decision can be resolved", "state": "satisfied", "evidenceRefs": [step_id]}]
            }
        }
    if run_id not in initial_runs:
        initial_runs.append(run_id)
    scenario = initial_runs.index(run_id)
    if scenario == 0:
        return {
            "thoughtSummary": "Happy path completed with evidence.",
            "completionState": "completed",
            "memoryWrites": [{
                "memoryType": "execution_lesson",
                "summary": "LLM happy path acceptance produced reusable workspace memory.",
                "content": "The first LLM-driven run completed and wrote memory for future recall.",
                "tags": ["llm", "happy-path"]
            }],
            "dodAssessment": {
                "satisfied": True,
                "summary": "Happy path DoD satisfied.",
                "items": [{"item": "happy path completes", "state": "satisfied", "evidenceRefs": [step_id]}]
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
            "thoughtSummary": "Risky action requires approval.",
            "completionState": "blocked_on_approval",
            "requiredApproval": {
                "reason": "Approval required before risky action.",
                "action": "continue risky test path",
                "context": {"scenario": "approval"}
            }
        }
    if scenario == 3:
        return "not-json"
    return {
        "thoughtSummary": "Memory recall path completed.",
        "completionState": "completed",
        "dodAssessment": {
            "satisfied": True,
            "summary": "Memory recall DoD satisfied.",
            "items": [{"item": "memory recall is visible", "state": "satisfied", "evidenceRefs": [step_id]}]
        }
    }

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path != "/v1/chat/completions":
            self.send_response(404)
            self.end_headers()
            return
        size = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(size)
        payload = json.loads(body)
        user_content = payload["messages"][-1]["content"]
        run_match = re.search(r'"runId": "([^"]+)"', user_content)
        step_match = re.search(r'"stepId": "([^"]+)"', user_content)
        provider_request = {
            "runId": run_match.group(1) if run_match else "",
            "stepId": step_match.group(1) if step_match else "",
        }
        decision = decision_payload(provider_request)
        content = decision if isinstance(decision, str) else json.dumps(decision, separators=(",", ":"))
        response = {"choices": [{"message": {"content": content}}]}
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

start_server() {
  mkdir -p "$CONFIG_DIR"
  cat >"$CONFIG_DIR/application.toml" <<EOF
endpointName = "workorchd"

[server]
addr = "$ADDR"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "30s"
idleTimeout = "60s"
shutdownTimeout = "5s"
maxConcurrentRuns = 1

[repository]
backend = "$BACKEND"
dataDir = "$DATA_DIR"

[llm]
defaultProvider = "verify"

[llm.providers.verify]
provider = "compatible"
baseURL = "$MOCK_BASE_URL"
apiKey = "verify-secret"
model = "verify-model"
timeout = "60s"
disabled = false
EOF
  "$BIN_PATH" -config-dir "$CONFIG_DIR" >>"$SERVER_LOG" 2>&1 &
  SERVER_PID=$!
}

stop_server() {
  if [[ -z "$SERVER_PID" ]]; then
    return
  fi
  kill -INT "$SERVER_PID" >/dev/null 2>&1 || true
  wait "$SERVER_PID" >/dev/null 2>&1 || true
  SERVER_PID=""
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

activate_workspace_execution() {
  local output_file="$1"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
    -H 'Content-Type: application/json' \
    -d '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"verify"}' >"$output_file"
}

trap report_failure ERR
trap cleanup EXIT

cd "$ROOT_DIR"
go build -o "$BIN_PATH" ./application/workorchd

mkdir -p "$DATA_DIR"
start_mock_provider
wait_mock
start_server
wait_ready

curl -fsS "$BASE_URL/readyz" | grep '"status": "ready"' >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
  -d '{"title":"llm acceptance workspace","description":"verify LLM driven workspace execution"}' >"$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"verify LLM driven workspace execution"}' >"$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
[[ -n "$goal_id" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["happy path completes","decision can be resolved","approval rejection fails safely","schema failure is observable","memory recall is visible"]}' >"$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"
[[ -n "$dod_id" ]]

activate_workspace_execution "$TMP_DIR/happy-activation.json"
grep '"state": "completed"' "$TMP_DIR/happy-activation.json" >/dev/null
happy_run_id="$(json_path "$TMP_DIR/happy-activation.json" "activation.run.id")"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$happy_run_id/snapshot" >"$TMP_DIR/happy-snapshot.json"
grep '"dodAssessment"' "$TMP_DIR/happy-snapshot.json" >/dev/null
grep '"memoryRefs"' "$TMP_DIR/happy-snapshot.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/memory?workspaceId=$workspace_id" >"$TMP_DIR/memory-after-happy.json"
grep 'LLM happy path acceptance produced reusable workspace memory' "$TMP_DIR/memory-after-happy.json" >/dev/null

activate_workspace_execution "$TMP_DIR/decision-activation.json"
grep '"state": "blocked_on_decision"' "$TMP_DIR/decision-activation.json" >/dev/null
decision_run_id="$(json_path "$TMP_DIR/decision-activation.json" "activation.run.id")"
ticket_id="$(json_path "$TMP_DIR/decision-activation.json" "activation.run.blockingTicketId")"
ticket_path="$(url_encode "$ticket_id")"
curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_path" | grep '"decisionKind": "provide_missing_info"' >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$ticket_path/resolve" -H 'Content-Type: application/json' \
  -d '{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"llm-acceptance"}}' | grep '"resolutionType": "provide_missing_info"' >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/$decision_run_id/resume" | grep '"state": "completed"' >/dev/null

activate_workspace_execution "$TMP_DIR/approval-activation.json"
grep '"state": "blocked_on_approval"' "$TMP_DIR/approval-activation.json" >/dev/null
approval_run_id="$(json_path "$TMP_DIR/approval-activation.json" "activation.run.id")"
approval_id="$(json_path "$TMP_DIR/approval-activation.json" "activation.run.blockingApprovalId")"
approval_path="$(url_encode "$approval_id")"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/approvals/$approval_path/reject" | grep '"state": "rejected"' >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/runs/$approval_run_id/resume" | grep '"state": "failed"' >/dev/null

activate_workspace_execution "$TMP_DIR/schema-failure-activation.json"
grep '"state": "failed"' "$TMP_DIR/schema-failure-activation.json" >/dev/null
schema_run_id="$(json_path "$TMP_DIR/schema-failure-activation.json" "activation.run.id")"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$schema_run_id/snapshot" >"$TMP_DIR/schema-failure-snapshot.json"
grep '"llm_schema_invalid"' "$TMP_DIR/schema-failure-snapshot.json" >/dev/null

activate_workspace_execution "$TMP_DIR/memory-recall-activation.json"
grep '"state": "completed"' "$TMP_DIR/memory-recall-activation.json" >/dev/null
memory_run_id="$(json_path "$TMP_DIR/memory-recall-activation.json" "activation.run.id")"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$memory_run_id/snapshot" >"$TMP_DIR/memory-recall-snapshot.json"
grep '"memories"' "$TMP_DIR/memory-recall-snapshot.json" >/dev/null
grep 'LLM happy path acceptance produced reusable workspace memory' "$TMP_DIR/memory-recall-snapshot.json" >/dev/null

if [[ "$BACKEND" == "file" ]]; then
  [[ -f "$DATA_DIR/memory_item.json" ]]
  stop_server
  start_server
  wait_ready
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$happy_run_id" | grep '"state": "completed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$decision_run_id" | grep '"state": "completed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$approval_run_id" | grep '"state": "failed"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$schema_run_id/snapshot" | grep '"llm_schema_invalid"' >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/memory?workspaceId=$workspace_id" | grep 'LLM happy path acceptance produced reusable workspace memory' >/dev/null
fi

echo "workorchd LLM workspace execution verification passed for backend=$BACKEND"
