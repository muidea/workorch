#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
BASE_URL="${WORKORCH_INCREMENTAL_ARTIFACT_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
MOCK_ADDR="${WORKORCH_INCREMENTAL_ARTIFACT_VERIFY_MOCK_ADDR:-127.0.0.1:18187}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_INCREMENTAL_ARTIFACT_VERIFY_SUFFIX:-$$}"
CURL_CONNECT_TIMEOUT="${WORKORCH_VERIFY_CURL_CONNECT_TIMEOUT_SECONDS:-5}"
CURL_MAX_TIME="${WORKORCH_VERIFY_CURL_MAX_TIME_SECONDS:-30}"
SCRIPT_TIMEOUT_SECONDS="${WORKORCH_INCREMENTAL_ARTIFACT_VERIFY_TIMEOUT_SECONDS:-240}"
RUN_WAIT_SECONDS="${WORKORCH_INCREMENTAL_ARTIFACT_VERIFY_RUN_WAIT_SECONDS:-90}"
ACTIVATION_RUN_WAIT_SECONDS="${WORKORCH_INCREMENTAL_ARTIFACT_VERIFY_ACTIVATION_RUN_WAIT_SECONDS:-60}"
PROVIDER_ID="verify-incremental-artifact-$VERIFY_SUFFIX"
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
  echo "incremental artifact e2e verification timed out after ${SCRIPT_TIMEOUT_SECONDS}s for deployed workorch service $BASE_URL" >&2
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
    echo "incremental artifact e2e verification exceeded ${SCRIPT_TIMEOUT_SECONDS}s; terminating" >&2
    kill -TERM "$parent_pid" >/dev/null 2>&1 || true
  ) &
  WATCHDOG_PID=$!
}

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "incremental artifact e2e verification failed for deployed workorch service $BASE_URL" >&2
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

start_mock_provider() {
  cat >"$TMP_DIR/llm_mock_provider.py" <<'PY'
import hashlib
import json
import os
import re
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

CONTENT = "neutral artifact content\n"
CONTENT_HASH = "sha256:" + hashlib.sha256(CONTENT.encode("utf-8")).hexdigest()
chunk_attempts = {}
chunk_attempts_lock = threading.Lock()

def extract_section_json(merged, title):
    marker = title + "\n"
    idx = merged.rfind(marker)
    if idx < 0:
        return {}
    start = merged.find("{", idx + len(marker))
    if start < 0:
        return {}
    depth = 0
    in_string = False
    escape = False
    for pos in range(start, len(merged)):
        ch = merged[pos]
        if in_string:
            if escape:
                escape = False
            elif ch == "\\":
                escape = True
            elif ch == '"':
                in_string = False
            continue
        if ch == '"':
            in_string = True
        elif ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                try:
                    return json.loads(merged[start:pos + 1])
                except Exception:
                    return {}
    return {}

def provider_request_from_payload(payload):
    messages = payload.get("messages") or []
    merged = "\n".join(str(item.get("content", "")) for item in messages)
    workspace_run = extract_section_json(merged, "Workspace and run context")
    current_action = extract_section_json(merged, "Current Requested Action")
    stage_contract = extract_section_json(merged, "Stage Contract") or extract_section_json(merged, "Runtime stage contract")
    repair_action = extract_section_json(merged, "Current Repair Action Package") or extract_section_json(merged, "Repair action package")
    def last(pattern):
        matches = re.findall(pattern, merged)
        return matches[-1] if matches else ""
    return {
        "runId": workspace_run.get("runId") or last(r'"runId"\s*:\s*"([^"]+)"'),
        "stepId": workspace_run.get("stepId") or last(r'"stepId"\s*:\s*"([^"]+)"'),
        "outputMode": stage_contract.get("outputMode") or stage_contract.get("requestedOutputMode") or last(r'"outputMode"\s*:\s*"([^"]+)"'),
        "phase": ((stage_contract.get("artifactOutputContract") or {}).get("phase") or stage_contract.get("artifactOutputPhase") or last(r'"phase"\s*:\s*"([^"]+)"')),
        "artifactContinuationId": stage_contract.get("artifactContinuationId") or last(r'"artifactContinuationId"\s*:\s*"([^"]+)"'),
        "artifactContextState": ((stage_contract.get("artifactContinuationContext") or {}).get("state") or ""),
        "currentActionType": current_action.get("type") or "",
        "requiredOutput": repair_action.get("requiredOutput") or current_action.get("requiredOutput") or {},
        "merged": merged,
    }

def full_assessment(step_id):
    evidence = [step_id, f"{step_id}/validation/runtime-step", "source_change"]
    return {
        "completionState": "completed",
        "thoughtSummary": "The incremental artifact apply produced the required neutral deliverable and evidence.",
        "dodAssessment": {
            "satisfied": True,
            "summary": "Incremental artifact delivery completed with state-changing evidence.",
            "goalOutcome": "The neutral deliverable was produced through incremental artifact assembly and apply.",
            "implementationSummary": "The runtime assembled chunks and applied the deliverable inside the workspace boundary.",
            "verificationSummary": "Artifact continuation events and applied evidence are present.",
            "items": [
                {
                    "item": "incremental artifact content is delivered",
                    "state": "satisfied",
                    "evidenceRefs": evidence,
                },
                {
                    "item": "artifact continuation lifecycle is observable without raw payload leakage",
                    "state": "satisfied",
                    "evidenceRefs": evidence,
                },
            ],
            "goalCoverage": {
                "satisfied": True,
                "items": [
                    {
                        "id": "goal-incremental-artifact",
                        "requirement": "Produce the neutral deliverable through incremental artifact output.",
                        "state": "satisfied",
                        "evidenceRefs": evidence,
                    }
                ],
            },
            "deliverableCoverage": {
                "satisfied": True,
                "items": [
                    {
                        "id": "source_change",
                        "deliverable": "source_change",
                        "path": "deliverable.txt",
                        "state": "satisfied",
                        "evidenceRefs": evidence,
                        "artifactRefs": ["artifact-001"],
                    }
                ],
            },
        },
    }

def decision_payload(request):
    if request.get("runId") == "probe":
        return {"completionState": "failed", "thoughtSummary": "probe"}
    if request.get("phase") == "artifact_context":
        return {
            "completionState": "continue",
            "executionGraph": {
                "nodes": [
                    {
                        "id": "read-current-target",
                        "kind": "file/read",
                        "input": {"targetRef": "deliverable.txt", "optional": True},
                    }
                ]
            },
        }
    if request.get("artifactContextState") == "chunking" or request.get("phase") == "artifact_chunk":
        run_id = request.get("runId") or "unknown"
        with chunk_attempts_lock:
            attempt = chunk_attempts.get(run_id, 0) + 1
            chunk_attempts[run_id] = attempt
        if attempt == 1:
            return {
                "completionState": "continue",
                "executionGraph": {
                    "nodes": [
                        {
                            "id": "read-before-chunk",
                            "kind": "file/read",
                            "input": {"targetRef": "deliverable.txt", "optional": True},
                        }
                    ]
                },
            }
        return {
            "completionState": "continue",
            "artifactChunk": {
                "artifactId": "artifact-001",
                "targetRef": "deliverable.txt",
                "sequence": 1,
                "content": CONTENT,
                "chunkHash": CONTENT_HASH,
                "isFinal": True,
            },
        }
    if request.get("currentActionType") == "assess_only" or re.search(r'"completionFollowUp"\s*:\s*true', request.get("merged", "")):
        return full_assessment(request.get("stepId") or "completion-follow-up")
    return {
        "completionState": "continue",
        "artifactPlan": {
            "artifacts": [
                {
                    "artifactId": "artifact-001",
                    "targetRef": "deliverable.txt",
                    "operation": "replace",
                    "coverageTargets": ["source_change"],
                    "estimatedChunks": 1,
                    "estimatedBytes": len(CONTENT.encode("utf-8")),
                }
            ]
        },
    }

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path == "/v1/embeddings":
            response = {"data": [{"embedding": [0.1, 0.2, 0.3]}]}
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
        decision = decision_payload(provider_request)
        print(
            "runId=%s stepId=%s mode=%s phase=%s artifactState=%s state=%s" % (
                provider_request.get("runId", ""),
                provider_request.get("stepId", ""),
                provider_request.get("outputMode", ""),
                provider_request.get("phase", ""),
                provider_request.get("artifactContextState", ""),
                decision.get("completionState", ""),
            ),
            flush=True,
        )
        response = {
            "choices": [{"message": {"content": json.dumps(decision, separators=(",", ":"))}}],
            "usage": {"prompt_tokens": 96, "completion_tokens": 24, "total_tokens": 120},
        }
        encoded = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)

host, port = os.environ.get("WORKORCH_INCREMENTAL_ARTIFACT_VERIFY_MOCK_ADDR", "127.0.0.1:18187").split(":", 1)
ThreadingHTTPServer((host, int(port)), Handler).serve_forever()
PY
  WORKORCH_INCREMENTAL_ARTIFACT_VERIFY_MOCK_ADDR="$MOCK_ADDR" python3 "$TMP_DIR/llm_mock_provider.py" >"$MOCK_LOG" 2>&1 &
  MOCK_PID=$!
  sleep 0.5
}

wait_mock() {
  for _ in $(seq 1 40); do
    if curl -fsS -X POST "http://$MOCK_ADDR/v1/chat/completions" \
      -H "Content-Type: application/json" \
      -d '{"messages":[{"role":"user","content":"{\"runId\":\"probe\",\"stepId\":\"probe\"}"}]}' >/dev/null 2>&1; then
      return
    fi
    sleep 0.25
  done
  echo "mock LLM provider did not become ready at http://$MOCK_ADDR" >&2
  exit 1
}

wait_ready() {
  for _ in $(seq 1 40); do
    if curl -fsS "$BASE_URL/readyz" | grep '"status": "ready"' >/dev/null; then
      return
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

config_path, original_path, update_path, mock_base_url, provider_id = sys.argv[1:]
data = json.load(open(config_path, encoding="utf-8"))
llm = data.get("configuration", {}).get("llm", {})
providers = llm.get("providers") or []
default_provider = llm.get("defaultProviderId") or provider_id
default_embedding_provider = llm.get("defaultEmbeddingProviderId") or ""

def clean_provider(item):
    inference = item.get("inferenceProfile") or {}
    context = item.get("contextProfile") or {}
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
        "inferenceProfile": {
            key: inference[key]
            for key in ("temperature", "maxOutputTokens", "reasoningEffort", "thinkingBudgetTokens")
            if key in inference
        },
        "contextProfile": {
            key: context[key]
            for key in (
                "profileMode",
                "contextWindowTokens",
                "maxInputTokens",
                "reservedOutputTokens",
                "safetyRatio",
                "contextUtilizationRatio",
                "bytesHardLimit",
                "maxRequestBytes",
                "estimatedTokensPerByte",
            )
            if key in context
        },
    }

original = {
    "defaultProviderId": default_provider,
    "defaultEmbeddingProviderId": default_embedding_provider,
    "providers": [clean_provider(item) for item in providers if item.get("id")],
}
if not original["providers"]:
    original["providers"].append({
        "id": provider_id,
        "provider": "compatible",
        "baseUrl": mock_base_url,
        "model": "verify-model",
        "timeout": "60s",
        "disabled": False,
        "chatCompletions": True,
        "streaming": False,
        "nonChatOperations": [],
    })
    original["defaultProviderId"] = provider_id
with open(original_path, "w", encoding="utf-8") as handle:
    json.dump(original, handle, separators=(",", ":"))

updated = [item for item in original["providers"] if item["id"] != provider_id]
updated.append({
    "id": provider_id,
    "provider": "compatible",
    "baseUrl": mock_base_url,
    "apiKey": "verify-secret",
    "model": "verify-model",
    "timeout": "60s",
    "disabled": False,
    "chatCompletions": True,
    "streaming": False,
    "nonChatOperations": [],
    "contextProfile": {
        "profileMode": "manual",
        "contextWindowTokens": 32768,
        "maxInputTokens": 24000,
        "reservedOutputTokens": 4096,
        "safetyRatio": 0.8,
        "contextUtilizationRatio": 0.8,
        "bytesHardLimit": 262144,
        "maxRequestBytes": 262144,
        "estimatedTokensPerByte": 0.25,
    },
})
update = {
    "defaultProviderId": provider_id,
    "defaultEmbeddingProviderId": original.get("defaultEmbeddingProviderId", ""),
    "providers": updated,
}
with open(update_path, "w", encoding="utf-8") as handle:
    json.dump(update, handle, separators=(",", ":"))
PY
}

wait_run_terminal() {
  local run_id="$1"
  local workspace_id="$2"
  local status_file="$3"
  local state=""
  local end=$((SECONDS + RUN_WAIT_SECONDS))
  while [[ $SECONDS -lt $end ]]; do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$status_file"
    state="$(json_path "$status_file" "status.state")"
    case "$state" in
      completed|failed|canceled|blocked_on_decision|blocked_on_approval)
        echo "$state"
        return
        ;;
    esac
    sleep 1
  done
  echo "timeout waiting for run $run_id to reach terminal/blocking state" >&2
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
  local end=$((SECONDS + ACTIVATION_RUN_WAIT_SECONDS))
  while [[ $SECONDS -lt $end ]]; do
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

validate_incremental_artifact_result() {
  python3 - "$TMP_DIR/snapshot.json" "$TMP_DIR/events.json" "$workspace_id" "$run_id" "$deliverable_directory" <<'PY'
import json
import pathlib
import sys

snapshot_path, events_path, workspace_id, run_id, deliverable_directory = sys.argv[1:]
snapshot = json.load(open(snapshot_path, encoding="utf-8")).get("snapshot") or {}
events = json.load(open(events_path, encoding="utf-8")).get("eventRecords") or []
errors = []
run = snapshot.get("run") or {}
if run.get("id") != run_id or run.get("workspaceId") != workspace_id:
    errors.append(f"snapshot identity mismatch: {run}")
if run.get("state") != "completed":
    errors.append(f"run state = {run.get('state')!r}, want completed")

target = pathlib.Path(deliverable_directory) / "deliverable.txt"
if not target.exists():
    errors.append(f"deliverable missing: {target}")
else:
    content = target.read_text(encoding="utf-8")
    if content != "neutral artifact content\n":
        errors.append(f"deliverable content = {content!r}")

continuations = snapshot.get("artifactContinuations") or []
if not continuations:
    errors.append("snapshot missing artifactContinuations")
if not any(item.get("targetRef") == "deliverable.txt" and item.get("state") == "applied" for item in continuations):
    errors.append(f"artifactContinuations missing applied deliverable: {continuations}")

event_topics = {item.get("topic") for item in events}
required_topics = {
    "/artifact_continuation/created",
    "/artifact_continuation/chunk_rejected",
    "/artifact_continuation/chunk_recorded",
    "/artifact_continuation/assembled",
    "/artifact_continuation/applied",
}
missing_topics = sorted(required_topics - event_topics)
if missing_topics:
    errors.append(f"missing artifact continuation events: {missing_topics}; present={sorted(event_topics)}")

results = snapshot.get("results") or []
if not results:
    errors.append("snapshot missing accepted result")
serialized_snapshot = json.dumps(snapshot, ensure_ascii=False)
serialized_events = json.dumps(events, ensure_ascii=False)
for forbidden in ["neutral artifact content", '"content":"', '"diff":"', '"patch":"', '"body":"']:
    if forbidden in serialized_events:
        errors.append(f"event payload leaked forbidden artifact payload marker {forbidden!r}")
if "neutral artifact content" in serialized_snapshot:
    errors.append("snapshot leaked raw artifact content")
for forbidden_key in ['"content":"neutral artifact content\\n"', '"diff":"', '"patch":"']:
    if forbidden_key in serialized_snapshot:
        errors.append(f"snapshot leaked forbidden payload key {forbidden_key!r}")

mock_log = pathlib.Path(snapshot_path).with_name("llm-mock.log")
if mock_log.exists():
    log_text = mock_log.read_text(encoding="utf-8", errors="replace")
    if "phase=artifact_context" not in log_text:
        errors.append("mock log missing artifact_context phase")
    if log_text.count("phase=artifact_chunk") < 2:
        errors.append("mock log missing retried artifact_chunk phase after context graph")

if errors:
    raise SystemExit("\n".join(errors))
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
  -d '{"title":"incremental artifact e2e","description":"verify neutral incremental artifact runtime contract","deliverableDirectory":"'"$workspace_workdir"'","templateId":"generic-governed"}' >"$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
deliverable_directory="$(json_path "$TMP_DIR/workspace.json" "workspace.deliverableDirectory")"
[[ -n "$workspace_id" ]]
[[ -n "$deliverable_directory" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"Produce the neutral deliverable through incremental artifact output."}' >"$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["incremental artifact content is delivered","artifact continuation lifecycle is observable without raw payload leakage"]}' >"$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"

activation_request_id="incremental-artifact-e2e-$VERIFY_SUFFIX"
activation_reason="e2e_resume"
python3 - "$TMP_DIR/activation-request.json" "$goal_id" "$dod_id" "$PROVIDER_ID" "$activation_request_id" "$activation_reason" <<'PY'
import json
import sys

path, goal_id, dod_id, provider_id, request_id, reason = sys.argv[1:]
payload = {
    "intent": "resume",
    "requestId": request_id,
    "reason": reason,
    "source": "e2e",
    "goalId": goal_id,
    "doDId": dod_id,
    "llmProviderId": provider_id,
    "priority": 1,
    "executionContextPatch": {
        "stateChangingOutputRequired": True,
        "repairObjective": "produce_state_changing_output",
        "requestedOutputMode": "incrementalArtifact",
        "repairActionPackage": {
            "requiredOutput": {
                "mustIncludeStateChangingNode": True,
                "estimatedPatchContentBytes": 20000,
                "expectedStateChangingArtifactCount": 1,
            },
        },
        "executionContract": {
            "stateChangePolicy": "must_produce_state_change",
            "profileContracts": {
                "neutral": {
                    "deliverableExpectations": [
                        {
                            "id": "source_change",
                            "required": True,
                            "description": "A neutral workspace deliverable must be changed through runtime-managed artifact output.",
                        }
                    ],
                },
            },
        },
    },
}
with open(path, "w", encoding="utf-8") as handle:
    json.dump(payload, handle, separators=(",", ":"))
PY

capture_runs_before_activation "$TMP_DIR/activation-before-runs.json"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/activation-request.json" >"$TMP_DIR/activation.json"
validate_activation_command_response "$TMP_DIR/activation.json" "$activation_request_id" "$activation_reason"
run_id="$(wait_activation_run "$TMP_DIR/activation-before-runs.json" "$TMP_DIR/activation-runs.json" "$TMP_DIR/activation-run.json")"
[[ -n "$run_id" ]]

terminal_state="$(wait_run_terminal "$run_id" "$workspace_id" "$TMP_DIR/status.json")"
if [[ "$terminal_state" != "completed" ]]; then
  echo "run $run_id reached state $terminal_state, want completed" >&2
  exit 1
fi

curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/snapshot" >"$TMP_DIR/snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/events?workspaceId=$workspace_id&runId=$run_id&limit=200" >"$TMP_DIR/events.json"
validate_incremental_artifact_result

echo "workorch incremental artifact e2e verification passed for deployed workorch service $BASE_URL workspace=$workspace_id run=$run_id"
