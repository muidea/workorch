#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ADDR_PORT="${WORKORCH_VERIFY_PORT:-18081}"
BASE_URL="http://127.0.0.1:$ADDR_PORT"
IMAGE_TAG="${WORKORCH_VERIFY_IMAGE:-workorch:verify}"
BACKEND="${WORKORCH_VERIFY_BACKEND:-memory}"
MOCK_PORT="${WORKORCH_VERIFY_MOCK_PORT:-18193}"
MOCK_BASE_URL="http://host.docker.internal:$MOCK_PORT/v1"
TMP_DIR="$(mktemp -d)"
DATA_DIR="$TMP_DIR/workorch-data"
CID=""
LOG_PATH="$TMP_DIR/container.log"
CONFIG_DIR="$TMP_DIR/config"
MOCK_PID=""
MOCK_LOG="$TMP_DIR/llm-mock.log"

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "workorch container verification failed for backend=$BACKEND" >&2
  if [[ -f "$MOCK_LOG" ]]; then
    echo "--- llm mock logs ---" >&2
    tail -n 200 "$MOCK_LOG" >&2 || true
  fi
  for json_file in "$TMP_DIR"/*.json; do
    if [[ -f "$json_file" ]]; then
      echo "--- $(basename "$json_file") ---" >&2
      sed -n '1,220p' "$json_file" >&2 || true
    fi
  done
  if [[ -n "$CID" ]]; then
    echo "--- docker logs ---" >&2
    docker logs "$CID" >&2 || true
  fi
}

json_contains() {
  local pattern="$1"
  shift
  if [[ $# -gt 0 ]]; then
    grep -Eq "$pattern" "$@"
  else
    local input
    input="$(cat)"
    grep -Eq "$pattern" <<<"$input"
  fi
}

cleanup() {
  if [[ -n "$CID" ]]; then
    docker rm -f "$CID" >/dev/null 2>&1 || true
  fi
  if [[ -n "$MOCK_PID" ]]; then
    kill "$MOCK_PID" >/dev/null 2>&1 || true
    wait "$MOCK_PID" >/dev/null 2>&1 || true
  fi
  docker image rm "$IMAGE_TAG" >/dev/null 2>&1 || true
  if [[ "$BACKEND" == "file" && -d "$DATA_DIR" ]]; then
    docker run --rm -v "$DATA_DIR:/var/lib/workorch" alpine sh -c "chown -R $(id -u):$(id -g) /var/lib/workorch >/dev/null 2>&1 || true" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR"
}

start_mock_provider() {
  cat >"$TMP_DIR/llm_mock_provider.py" <<'PY'
import json
import os
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def parse_provider_request(payload):
    user_content = payload.get("messages", [{}])[-1].get("content", "")
    try:
        request = json.loads(user_content)
    except Exception:
        request = {}
    request["raw"] = user_content
    if not request.get("runId"):
        run_match = re.search(r'"runId"\s*:\s*"([^"]+)"', user_content)
        if run_match:
            request["runId"] = run_match.group(1)
    if not request.get("stepId"):
        step_match = re.search(r'"stepId"\s*:\s*"([^"]+)"', user_content)
        if step_match:
            request["stepId"] = step_match.group(1)
    return request


def decision_payload(request):
    run_id = request.get("runId", "")
    step_id = request.get("stepId", "") or f"{run_id}/step-1"
    if run_id == "probe":
        return {"completionState": "failed", "thoughtSummary": "probe"}
    if "llm-after-decision" in step_id or "llm-after-approval" in step_id:
        return {
            "thoughtSummary": "Governance gate resolved; container verification can complete.",
            "completionState": "completed",
            "dodAssessment": {
                "satisfied": True,
                "summary": "Container verification completed after governance resolution.",
                "items": [{
                    "item": "container governance run completes after resolution",
                    "state": "satisfied",
                    "evidenceRefs": [step_id],
                }],
            },
        }
    if "decision-block" in run_id:
        return {
            "thoughtSummary": "Container verification requires a decision.",
            "completionState": "blocked_on_decision",
            "requiredDecision": {
                "reason": "Container verification decision gate.",
                "decisionKind": "provide_missing_info",
                "question": "Provide the verification decision input.",
                "context": {"runId": run_id},
            },
        }
    if "approval-block" in run_id:
        return {
            "thoughtSummary": "Container verification requires approval.",
            "completionState": "blocked_on_approval",
            "requiredApproval": {
                "reason": "Container verification approval gate.",
                "action": "approve container verification path",
                "context": {"runId": run_id},
            },
        }
    return {
        "thoughtSummary": "Container verification completed.",
        "completionState": "completed",
        "dodAssessment": {
            "satisfied": True,
            "summary": "Container verification run completed.",
            "items": [{
                "item": "workspace can be created and run can complete",
                "state": "satisfied",
                "evidenceRefs": [step_id],
            }],
        },
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
        size = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(size)
        if self.path == "/v1/embeddings":
            response = {"data": [{"embedding": [0.1, 0.2, 0.3]}]}
        elif self.path == "/v1/chat/completions":
            payload = json.loads(body or b"{}")
            request = parse_provider_request(payload)
            decision = decision_payload(request)
            print(
                "decision run=%s step=%s state=%s" % (
                    request.get("runId", ""),
                    request.get("stepId", ""),
                    decision.get("completionState", ""),
                ),
                flush=True,
            )
            content = json.dumps(decision, separators=(",", ":"))
            response = {"choices": [{"message": {"content": content}}]}
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


port = int(os.environ.get("WORKORCH_VERIFY_MOCK_PORT", "18193"))
ThreadingHTTPServer(("0.0.0.0", port), Handler).serve_forever()
PY
  WORKORCH_VERIFY_MOCK_PORT="$MOCK_PORT" python3 "$TMP_DIR/llm_mock_provider.py" >>"$MOCK_LOG" 2>&1 &
  MOCK_PID=$!
}

wait_mock() {
  for _ in $(seq 1 50); do
    if curl -fsS -X POST "http://127.0.0.1:$MOCK_PORT/v1/chat/completions" \
      -H "Content-Type: application/json" \
      -d '{"messages":[{"role":"user","content":"{\"runId\": \"probe\", \"stepId\": \"probe\"}"}]}' >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl -fsS -X POST "http://127.0.0.1:$MOCK_PORT/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"messages":[{"role":"user","content":"{\"runId\": \"probe\", \"stepId\": \"probe\"}"}]}' >/dev/null
}

start_container() {
	local data_dir="/var/lib/workorch"
  mkdir -p "$CONFIG_DIR"
  cat >"$CONFIG_DIR/application.toml" <<EOF
endpointName = "workorch"

[server]
addr = ":8080"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "30s"
idleTimeout = "60s"
shutdownTimeout = "5s"
maxConcurrentRuns = 1

[repository]
backend = "$BACKEND"
dataDir = "$data_dir"

[llm.aiProxy]
baseURL = "$MOCK_BASE_URL"
apiKey = "container-verify-key"
timeout = "180s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "verify"
modelID = "container-verify-model"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25

[llm.customProviders.verify]
provider = "compatible"
apiSurface = "openai_compatible_http"
baseURL = "$MOCK_BASE_URL"
apiKey = "container-verify-key"
timeout = "60s"

[llm.customProviders.verify.models.container-verify-model]
contextWindowTokens = 128000
maxOutputTokens = 8192
operations = ["chat_completions", "embeddings"]
EOF
  local docker_args=(run -d --add-host=host.docker.internal:host-gateway -p "$ADDR_PORT:8080" -v "$CONFIG_DIR/application.toml:/etc/workorch/application.toml:ro")
  if [[ "$BACKEND" == "file" ]]; then
    docker_args+=(
      -v "$DATA_DIR:/var/lib/workorch"
    )
  fi
  docker_args+=("$IMAGE_TAG")
  CID="$(docker "${docker_args[@]}")"
}

restart_container() {
  docker rm -f "$CID" >/dev/null
  CID=""
  start_container
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

wait_run_state() {
  local run_id="$1"
  local expected_state="$2"
  local output_path="$3"
  for _ in $(seq 1 50); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$output_path"
    if json_contains '"state"[[:space:]]*:[[:space:]]*"'"$expected_state"'"' "$output_path"; then
      return 0
    fi
    sleep 0.2
  done
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$output_path"
  json_contains '"state"[[:space:]]*:[[:space:]]*"'"$expected_state"'"' "$output_path"
}

trap report_failure ERR
trap cleanup EXIT

cd "$ROOT_DIR"
docker build -t "$IMAGE_TAG" .

mkdir -p "$DATA_DIR"
chmod 0777 "$DATA_DIR"

start_mock_provider
wait_mock
start_container
wait_ready

curl -fsS "$BASE_URL/healthz" | json_contains '"status"[[:space:]]*:[[:space:]]*"ok"'
curl -fsS "$BASE_URL/readyz" | json_contains '"status"[[:space:]]*:[[:space:]]*"ready"'
curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
grep '"apiKeyConfigured": true' "$TMP_DIR/configuration.json" >/dev/null
grep '"baseUrl": "'"$MOCK_BASE_URL"'"' "$TMP_DIR/configuration.json" >/dev/null
curl -fsS "$BASE_URL/metrics" | grep 'workorch_http_requests_total' >/dev/null
curl -fsS "$BASE_URL/console" | grep 'workorch Console' >/dev/null
curl -fsS "$BASE_URL/console/assets/workorch-console.css" | grep 'workorch-section' >/dev/null
curl -fsS "$BASE_URL/console/assets/workorch-console.js" | grep '业务总览' >/dev/null
curl -fsS "$BASE_URL/console/assets/workorch-console.js" | grep '治理队列' >/dev/null
curl -fsS "$BASE_URL/console/assets/workorch-console.js" | grep '工作空间详情' >/dev/null
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
  -d '{"title":"verify workspace","description":"verify workspace description","templateId":"generic-governed"}' >"$TMP_DIR/workspace.json"
workspace_id="$(sed -n 's/.*"id": "\([^"]*\)".*/\1/p' "$TMP_DIR/workspace.json" | head -n1)"
[[ -n "$workspace_id" ]]
grep '"title": "verify workspace"' "$TMP_DIR/workspace.json" >/dev/null
grep '"description": "verify workspace description"' "$TMP_DIR/workspace.json" >/dev/null
if [[ "$BACKEND" == "file" ]]; then
  grep "\"deliverableDirectory\": \".*/.workorch-workspaces/$workspace_id\"" "$TMP_DIR/workspace.json" >/dev/null
else
  grep "\"deliverableDirectory\": \".*/.workorch-workspaces/$workspace_id\"" "$TMP_DIR/workspace.json" >/dev/null
fi
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"verify workorch service delivery"}' >"$TMP_DIR/goal.json"
goal_id="$(sed -n 's/.*"id": "\([^"]*\)".*/\1/p' "$TMP_DIR/goal.json" | head -n1)"
[[ -n "$goal_id" ]]
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["workspace can be created","run can be completed"]}' >"$TMP_DIR/dod.json"
dod_id="$(sed -n 's/.*"id": "\([^"]*\)".*/\1/p' "$TMP_DIR/dod.json" | head -n1)"
[[ -n "$dod_id" ]]
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" | grep "\"id\": \"$goal_id\"" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" | grep "\"id\": \"$dod_id\"" >/dev/null
if [[ "$BACKEND" == "file" ]]; then
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" -H 'Content-Type: application/json' \
    -d '{"runId":"run-verify","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'"}' >"$TMP_DIR/run-verify-create.json"
  json_contains '"id"[[:space:]]*:[[:space:]]*"run-verify"' "$TMP_DIR/run-verify-create.json"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/run-verify/start" >"$TMP_DIR/run-verify-start.json"
  json_contains '"state"[[:space:]]*:[[:space:]]*"completed"' "$TMP_DIR/run-verify-start.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/run-verify/snapshot" >"$TMP_DIR/run-verify-snapshot.json"
  json_contains '"id"[[:space:]]*:[[:space:]]*"run-verify"' "$TMP_DIR/run-verify-snapshot.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/run-verify/status" >"$TMP_DIR/run-verify-status.json"
  json_contains '"state"[[:space:]]*:[[:space:]]*"completed"' "$TMP_DIR/run-verify-status.json"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" -H 'Content-Type: application/json' \
    -d '{"runId":"run-runtime-decision-block-verify","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'"}' | json_contains '"id"[[:space:]]*:[[:space:]]*"run-runtime-decision-block-verify"'
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/run-runtime-decision-block-verify/start" >"$TMP_DIR/run-runtime-decision-block-verify.json"
  json_contains '"state"[[:space:]]*:[[:space:]]*"blocked_on_decision"' "$TMP_DIR/run-runtime-decision-block-verify.json"
  ticket_id="$(sed -n 's/.*"blockingTicketId": "\(.*\)".*/\1/p' "$TMP_DIR/run-runtime-decision-block-verify.json" | head -n1)"
  [[ -n "$ticket_id" ]]
  curl -fsS "$BASE_URL/api/v1/workorch/decisions" | grep "\"id\": \"$ticket_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_id?workspaceId=$workspace_id" | json_contains '"state"[[:space:]]*:[[:space:]]*"open"'
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id" | grep "\"id\": \"$ticket_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id&runId=run-runtime-decision-block-verify" | grep "\"id\": \"$ticket_id\"" >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$ticket_id/resolve?workspaceId=$workspace_id" -H 'Content-Type: application/json' \
    -d '{"resolutionType":"provide_missing_info"}' | json_contains '"resolutionType"[[:space:]]*:[[:space:]]*"provide_missing_info"'
  wait_run_state "run-runtime-decision-block-verify" "completed" "$TMP_DIR/run-runtime-decision-block-verify-resumed-status.json"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" -H 'Content-Type: application/json' \
    -d '{"runId":"run-runtime-decision-block-rejected-verify","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'"}' | json_contains '"id"[[:space:]]*:[[:space:]]*"run-runtime-decision-block-rejected-verify"'
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/run-runtime-decision-block-rejected-verify/start" >"$TMP_DIR/run-runtime-decision-block-rejected-verify.json"
  reject_ticket_id="$(sed -n 's/.*"blockingTicketId": "\(.*\)".*/\1/p' "$TMP_DIR/run-runtime-decision-block-rejected-verify.json" | head -n1)"
  [[ -n "$reject_ticket_id" ]]
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$reject_ticket_id/resolve?workspaceId=$workspace_id" -H 'Content-Type: application/json' \
    -d '{"resolutionType":"reject_current_path","resolutionPayload":{"operator":"verify"}}' | json_contains '"resolutionType"[[:space:]]*:[[:space:]]*"reject_current_path"'
  wait_run_state "run-runtime-decision-block-rejected-verify" "failed" "$TMP_DIR/run-runtime-decision-block-rejected-verify-resumed-status.json"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" -H 'Content-Type: application/json' \
    -d '{"runId":"run-runtime-approval-block-verify","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'"}' | json_contains '"id"[[:space:]]*:[[:space:]]*"run-runtime-approval-block-verify"'
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/run-runtime-approval-block-verify/start" >"$TMP_DIR/run-runtime-approval-block-verify.json"
  json_contains '"state"[[:space:]]*:[[:space:]]*"blocked_on_approval"' "$TMP_DIR/run-runtime-approval-block-verify.json"
  approval_id="$(sed -n 's/.*"blockingApprovalId": "\(.*\)".*/\1/p' "$TMP_DIR/run-runtime-approval-block-verify.json" | head -n1)"
  [[ -n "$approval_id" ]]
  curl -fsS "$BASE_URL/api/v1/workorch/approvals" | grep "\"id\": \"$approval_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/approvals/$approval_id?workspaceId=$workspace_id" | json_contains '"state"[[:space:]]*:[[:space:]]*"pending"'
  curl -fsS "$BASE_URL/api/v1/workorch/approvals?workspaceId=$workspace_id" | grep "\"id\": \"$approval_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/approvals?workspaceId=$workspace_id&runId=run-runtime-approval-block-verify" | grep "\"id\": \"$approval_id\"" >/dev/null
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/approvals/$approval_id/approve?workspaceId=$workspace_id" | json_contains '"state"[[:space:]]*:[[:space:]]*"approved"'
  wait_run_state "run-runtime-approval-block-verify" "completed" "$TMP_DIR/run-runtime-approval-block-verify-resumed-status.json"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" -H 'Content-Type: application/json' \
    -d '{"runId":"run-runtime-approval-block-rejected-verify","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'"}' | json_contains '"id"[[:space:]]*:[[:space:]]*"run-runtime-approval-block-rejected-verify"'
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/run-runtime-approval-block-rejected-verify/start" >"$TMP_DIR/run-runtime-approval-block-rejected-verify.json"
  reject_approval_id="$(sed -n 's/.*"blockingApprovalId": "\(.*\)".*/\1/p' "$TMP_DIR/run-runtime-approval-block-rejected-verify.json" | head -n1)"
  [[ -n "$reject_approval_id" ]]
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/approvals/$reject_approval_id/reject?workspaceId=$workspace_id" | json_contains '"state"[[:space:]]*:[[:space:]]*"rejected"'
  wait_run_state "run-runtime-approval-block-rejected-verify" "failed" "$TMP_DIR/run-runtime-approval-block-rejected-verify-resumed-status.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces" | grep "\"id\": \"$workspace_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id" | json_contains '"title"[[:space:]]*:[[:space:]]*"verify workspace"'
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" | grep "\"id\": \"$workspace_id\"" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/runs" | json_contains '"id"[[:space:]]*:[[:space:]]*"run-verify"'
  curl -fsS "$BASE_URL/api/v1/workorch/runs?workspaceId=$workspace_id" | json_contains '"id"[[:space:]]*:[[:space:]]*"run-verify"'
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/run-verify" | grep "\"workspaceId\": \"$workspace_id\"" >/dev/null
  [[ -f "$DATA_DIR/system/workspace_index.json" ]]
  grep "\"workspaceId\":\"$workspace_id\"" "$DATA_DIR/system/workspace_index.json" >/dev/null
  grep "\"workspaceRoot\":\"workspaces/$workspace_id\"" "$DATA_DIR/system/workspace_index.json" >/dev/null
  for name in workspace.json goals.json dods.json; do
    [[ -f "$DATA_DIR/workspaces/$workspace_id/$name" ]]
  done
  for name in run.json step.json; do
    [[ -f "$DATA_DIR/workspaces/$workspace_id/runs/run-verify/$name" ]]
  done
  for name in ticket.json resolution.json; do
    [[ -f "$DATA_DIR/workspaces/$workspace_id/runs/run-runtime-decision-block-verify/$name" ]]
  done
  for name in approval.json; do
    [[ -f "$DATA_DIR/workspaces/$workspace_id/runs/run-runtime-approval-block-verify/$name" ]]
  done

  restart_container
  wait_ready

  curl -fsS "$BASE_URL/readyz" | json_contains '"status"[[:space:]]*:[[:space:]]*"ready"'
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id" | json_contains '"title"[[:space:]]*:[[:space:]]*"verify workspace"'
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals/$goal_id" | json_contains '"text"[[:space:]]*:[[:space:]]*"verify workorch service delivery"'
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods/$dod_id" | json_contains '"state"[[:space:]]*:[[:space:]]*"ready"'
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" | json_contains '"id"[[:space:]]*:[[:space:]]*"run-verify"'
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/run-verify" | json_contains '"state"[[:space:]]*:[[:space:]]*"completed"'
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/run-verify/snapshot" | json_contains '"id"[[:space:]]*:[[:space:]]*"run-verify"'
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/run-verify/status" | json_contains '"state"[[:space:]]*:[[:space:]]*"completed"'
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/run-runtime-decision-block-verify" | json_contains '"state"[[:space:]]*:[[:space:]]*"completed"'
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/run-runtime-decision-block-rejected-verify" | json_contains '"state"[[:space:]]*:[[:space:]]*"failed"'
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_id?workspaceId=$workspace_id" | json_contains '"state"[[:space:]]*:[[:space:]]*"resolved"'
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$reject_ticket_id?workspaceId=$workspace_id" | json_contains '"resolutionType"[[:space:]]*:[[:space:]]*"reject_current_path"'
  curl -fsS "$BASE_URL/api/v1/workorch/approvals/$approval_id?workspaceId=$workspace_id" | json_contains '"state"[[:space:]]*:[[:space:]]*"approved"'
  curl -fsS "$BASE_URL/api/v1/workorch/approvals/$reject_approval_id?workspaceId=$workspace_id" | json_contains '"state"[[:space:]]*:[[:space:]]*"rejected"'
fi

echo "workorch container verification passed for backend=$BACKEND"
