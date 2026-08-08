#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_TUI_LONG_OUTPUT_BASE_URL:-${WORKORCH_BASE_URL:-}}"
source "$ROOT_DIR/scripts/lib/verify-isolated-runtime.sh"
workorch_verify_require_isolated_runtime "$BASE_URL" "verify-tui-long-output-e2e"
TMP_DIR="${WORKORCH_TUI_LONG_OUTPUT_ARTIFACT_DIR:-$(mktemp -d)}"
MOCK_ADDR="${WORKORCH_TUI_LONG_OUTPUT_MOCK_ADDR:-127.0.0.1:18182}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_TUI_LONG_OUTPUT_SUFFIX:-$$}"
ACTIVATION_RUN_WAIT_SECONDS="${WORKORCH_TUI_LONG_OUTPUT_ACTIVATION_RUN_WAIT_SECONDS:-60}"
PROVIDER_ID="verify-tui-long-output-$VERIFY_SUFFIX"
SKILL_ID="skill-tui-long-output-$VERIFY_SUFFIX"
SOUL_ID="soul-tui-long-output-$VERIFY_SUFFIX"
WORKSPACE_TITLE="TUI long output e2e $VERIFY_SUFFIX"
MOCK_PID=""
MOCK_LOG="$TMP_DIR/tui-long-output-llm-mock.log"
KEEP_ARTIFACTS="${WORKORCH_TUI_LONG_OUTPUT_KEEP_ARTIFACTS:-1}"
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
  echo "TUI long output e2e failed" >&2
  echo "baseUrl=$BASE_URL" >&2
  echo "artifacts=$TMP_DIR" >&2
  if [[ -f "$MOCK_LOG" ]]; then
    echo "--- tui-long-output-llm-mock.log ---" >&2
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
  if [[ -f "$TMP_DIR/original-llm-update.json" ]]; then
    curl -fsS -H "Content-Type: application/json" -X PUT \
      "$BASE_URL/api/v1/workorch/configuration/llm" \
      -d @"$TMP_DIR/original-llm-update.json" >/dev/null 2>&1 || true
  fi
  if [[ -n "$MOCK_PID" ]]; then
    kill "$MOCK_PID" >/dev/null 2>&1 || true
    wait "$MOCK_PID" >/dev/null 2>&1 || true
  fi
  if [[ "$LOCK_HELD" == "1" ]]; then
    rmdir "$LOCK_DIR" >/dev/null 2>&1 || true
  fi
  if [[ "$KEEP_ARTIFACTS" != "1" && $status -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  fi
}

trap report_failure ERR
trap cleanup EXIT

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

start_mock_provider() {
  cat >"$TMP_DIR/tui_long_output_llm_mock.py" <<'PY'
import json
import os
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

REQUEST_LOG = os.environ.get("WORKORCH_TUI_LONG_OUTPUT_MOCK_REQUEST_LOG", "")

def long_lines(prefix, count):
    return "\n".join(f"{prefix} {idx:02d} - viewport scroll verification marker" for idx in range(1, count + 1))

def completion_payload(request):
    request_text = request.get("requestText", "")
    required_state_match = re.search(r'"requiredState"\s*:\s*"([^"]+)"', request_text)
    focus_match = re.search(r'"focus"\s*:\s*\{\s*"id"\s*:\s*"([^"]+)"', request_text)
    coverage_match = re.search(r'"focus"\s*:\s*\{.*?"coverageWorkItemIds"\s*:\s*\[\s*"([^"]+)"', request_text)
    candidate_mode_by_state = {
        "need_state_change": "produce_state_change",
        "need_validation": "produce_validation",
        "need_assessment": "produce_assessment",
        "need_governance": "request_governance",
    }
    # RuntimeAction is not part of the Runtime prompt. Only a catalog probe
    # lacks a sealed requiredState, for which the response body is irrelevant.
    action = candidate_mode_by_state.get(
        required_state_match.group(1) if required_state_match else "",
        "produce_validation",
    )
    focus_id = focus_match.group(1) if focus_match else "focus-001"
    coverage_id = coverage_match.group(1) if coverage_match else "cwi-001"
    validation_op_id = f"validate-{focus_id}"
    goal = long_lines("LONG-GOAL-OUTCOME", 40)
    implementation = long_lines("LONG-IMPLEMENTATION", 80)
    verification = long_lines("LONG-VERIFICATION", 24)
    if action == "produce_validation":
        return {
            "operations": [{
                "id": validation_op_id,
                "kind": "file/read",
                "coverageWorkItemIds": [coverage_id],
                "targetRefs": ["runtime-validation-source.txt"],
                "parameters": [
                    {"name": "path", "string": "runtime-validation-source.txt"},
                    {"name": "maxBytes", "integer": 16384},
                ],
            }],
        }
    if action == "produce_assessment":
        return {
            "focusId": focus_id,
            "coverageWorkItemIds": [coverage_id],
            "satisfied": True,
            "summary": "\n".join([goal, implementation, verification]),
            # file/read emits a stable workspace URI as an accepted evidence
            # ref. The dispatch/event IDs are intentionally volatile and must
            # not be guessed by a provider fixture.
            "evidenceRefs": ["workorch://workspace/runtime-validation-source.txt"],
        }
    if action == "request_governance":
        return {"externalDependency": {"kind": "none", "reason": "deterministic viewport verification does not require governance"}}
    # This profile normally enters validation directly. Keep an explicit typed
    # state-change candidate for a future profile change instead of returning
    # legacy completionState/thoughtSummary fields.
    return {
        "operations": [{
            "id": f"state-{focus_id}",
            "kind": "file/edit",
            "coverageWorkItemIds": [coverage_id],
            "parameters": [],
        }],
    }

def legacy_completion_payload(request):
    # Kept only as a readable fixture for the detailed assessment text below;
    # Runtime never receives this legacy shape.
    step_id = request.get("stepId", "") or "step-1"
    goal = long_lines("LONG-GOAL-OUTCOME", 40)
    implementation = long_lines("LONG-IMPLEMENTATION", 80)
    verification = long_lines("LONG-VERIFICATION", 24)
    return {
        "thoughtSummary": "Generated deterministic long output for TUI viewport verification.",
        "completionState": "completed",
        "dodAssessment": {
            "satisfied": True,
            "summary": "TUI long output e2e completed with more than one terminal screen of content.",
            "goalOutcome": goal,
            "implementationSummary": implementation,
            "verificationSummary": verification,
            "items": [
                {
                    "item": "Final Result contains LONG-IMPLEMENTATION 70",
                    "state": "satisfied",
                    "evidenceRefs": [step_id]
                },
                {
                    "item": "Final Result is long enough to require PgDown/PgUp viewport scrolling",
                    "state": "satisfied",
                    "evidenceRefs": [step_id]
                },
                {
                    "item": "Workspace monitor remains visible while main display scrolls",
                    "state": "satisfied",
                    "evidenceRefs": [step_id]
                }
            ],
            "goalCoverage": {
                "satisfied": True,
                "items": [
                    {
                        "goal": "Generate deterministic long final output for TUI viewport verification.",
                        "state": "satisfied",
                        "evidenceRefs": [step_id]
                    }
                ]
            },
            "acceptanceDimensions": {
                "satisfied": True,
                "items": [
                    {
                        "id": "functional_behavior",
                        "state": "satisfied",
                        "evidenceRefs": [step_id]
                    },
                    {
                        "id": "deliverable",
                        "state": "satisfied",
                        "evidenceRefs": [step_id]
                    }
                ]
            },
            "deliverableCoverage": {
                "satisfied": True,
                "items": [
                    {
                        "id": "source_change",
                        "state": "satisfied",
                        "artifactRefs": [step_id]
                    },
                    {
                        "id": "validation_report",
                        "state": "satisfied",
                        "artifactRefs": [step_id]
                    },
                    {
                        "id": "coverage_map",
                        "state": "satisfied",
                        "artifactRefs": [step_id]
                    },
                    {
                        "id": "user_deliverable",
                        "state": "satisfied",
                        "artifactRefs": [step_id]
                    }
                ]
            },
            "knownLimitations": ["Manual terminal drag selection must be confirmed in the operator terminal."],
            "residualRisks": ["Terminal emulator key binding differences can affect PgUp/PgDown delivery."],
            "nextActions": ["Open the generated workspace in TUI, press PgDown/PgUp, and verify the side panel stays fixed."]
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
        size = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(size)
        if self.path == "/v1/embeddings":
            response = {"data": [{"embedding": [0.1, 0.2, 0.3]}]}
        elif self.path == "/v1/chat/completions":
            payload = json.loads(body or b"{}")
            if REQUEST_LOG:
                with open(REQUEST_LOG, "w", encoding="utf-8") as handle:
                    json.dump(payload, handle, ensure_ascii=False, indent=2)
            payload_text = json.dumps(payload, ensure_ascii=False)
            id_matches = re.findall(r'run-\d+(?:-replan-\d+)?(?:/[A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)*)?', payload_text)
            def run_sort_key(value):
                base = value.split("/", 1)[0]
                match = re.match(r"run-(\d+)", base)
                return int(match.group(1)) if match else -1
            step_candidates = [
                item for item in id_matches
                if re.search(r'/(step-\d+|repair-[A-Za-z0-9._-]+|provider-retry-[A-Za-z0-9._-]+|follow-up-[A-Za-z0-9._-]+|graph-[A-Za-z0-9._-]+)$', item)
            ]
            step_match = max(step_candidates, key=run_sort_key, default="")
            run_candidates = [item.split("/", 1)[0] for item in id_matches]
            run_match = max(run_candidates, key=run_sort_key, default="")
            if not step_match and run_match:
                step_match = run_match + "/step-1"
            request_text = "\n".join(str(item.get("content", "")) for item in payload.get("messages", []) if isinstance(item, dict))
            provider_request = {"runId": run_match, "stepId": step_match, "requestText": request_text}
            content = json.dumps(completion_payload(provider_request), separators=(",", ":"))
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

host, port_text = os.environ.get("WORKORCH_TUI_LONG_OUTPUT_MOCK_ADDR", "127.0.0.1:18182").split(":", 1)
ThreadingHTTPServer((host, int(port_text)), Handler).serve_forever()
PY
  WORKORCH_TUI_LONG_OUTPUT_MOCK_ADDR="$MOCK_ADDR" \
  WORKORCH_TUI_LONG_OUTPUT_MOCK_REQUEST_LOG="$TMP_DIR/tui-long-output-last-provider-request.json" \
    python3 "$TMP_DIR/tui_long_output_llm_mock.py" >>"$MOCK_LOG" 2>&1 &
  MOCK_PID=$!
}

wait_ready() {
  for _ in $(seq 1 50); do
    if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1 && curl -fsS "$BASE_URL/readyz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl -fsS "$BASE_URL/healthz" >/dev/null
  curl -fsS "$BASE_URL/readyz" >/dev/null
}

wait_mock() {
  for _ in $(seq 1 50); do
    if curl -fsS -X POST "$MOCK_BASE_URL/chat/completions" \
      -H 'Content-Type: application/json' \
      -d '{"messages":[{"role":"user","content":"{\"runId\": \"probe\", \"stepId\": \"probe\"}"}]}' >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl -fsS -X POST "$MOCK_BASE_URL/chat/completions" \
    -H 'Content-Type: application/json' \
    -d '{"messages":[{"role":"user","content":"{\"runId\": \"probe\", \"stepId\": \"probe\"}"}]}' >/dev/null
}

wait_run_state() {
  local workspace_id="$1"
  local run_id="$2"
  local expected_state="$3"
  local output_file="$4"
  local deadline=$((SECONDS + 180))

  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status?waitState=$expected_state&waitTimeoutMillis=1000&waitIntervalMillis=100" >"$output_file"
    if python3 - "$output_file" "$expected_state" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)
status = data.get("status") or {}
if status.get("state") != sys.argv[2]:
    raise SystemExit(1)
PY
    then
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
if data.get("intent") != "restart":
    errors.append(f"intent is {data.get('intent')!r}, want restart")
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

assert_runtime_result() {
  python3 - "$TMP_DIR/snapshot.json" "$workspace_id" "$run_id" <<'PY'
import json
import sys

snapshot_path, workspace_id, run_id = sys.argv[1:]
data = json.load(open(snapshot_path, encoding="utf-8")).get("snapshot", {})
errors = []
run = data.get("run") or {}
if run.get("id") != run_id or run.get("workspaceId") != workspace_id:
    errors.append(f"snapshot identity mismatch: run={run}")
if run.get("state") != "completed":
    errors.append(f"run state is {run.get('state')!r}, want completed")
results = [item for item in data.get("results", []) if item.get("runId") == run_id and item.get("state") == "accepted"]
if not results:
    errors.append("accepted result missing")
for result in results:
    assessment = result.get("doDAssessment") or {}
    if assessment.get("satisfied") is not True:
        errors.append(f"accepted result has unsatisfied typed assessment: {assessment}")
        break
    if len(assessment.get("items") or []) < 3:
        errors.append(f"typed assessment items missing: {assessment}")
        break
    if not (result.get("evidenceRefs") or []):
        errors.append(f"accepted result has no evidence refs: {result}")
        break
if errors:
    for item in errors:
        print(item, file=sys.stderr)
    sys.exit(1)
PY
}

wait_runtime_result_snapshot() {
  local deadline=$((SECONDS + 180))

  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/snapshot" >"$TMP_DIR/snapshot.json"
    if assert_runtime_result >/dev/null 2>"$TMP_DIR/snapshot-assert.err"; then
      return 0
    fi
    sleep 1
  done

  cat "$TMP_DIR/snapshot-assert.err" >&2 || true
  return 1
}

post_execution_activation() {
  local payload="$TMP_DIR/activation-request.json"
  local status=""
  local workspace_revision
  workspace_revision="$(curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id" | python3 -c 'import json,sys; print(json.load(sys.stdin)["workspace"]["revision"])')"
  curl -fsS -X PUT "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/llm-selection" \
    -H 'Content-Type: application/json' \
    -d '{"providerId":"'"$PROVIDER_ID"'","modelId":"verify-model","expectedRevision":'"$workspace_revision"'}' >"$TMP_DIR/llm-selection.json"
  curl -fsS -X PUT "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/soul-bindings" \
    -H 'Content-Type: application/json' \
    -d '{"defaultSoulId":"'"$SOUL_ID"'","bindings":{"planning":"'"$SOUL_ID"'","execution":"'"$SOUL_ID"'","decision":"'"$SOUL_ID"'","approval":"'"$SOUL_ID"'","validation":"'"$SOUL_ID"'","artifact":"'"$SOUL_ID"'"}}' >"$TMP_DIR/soul-bindings.json"
  cat >"$payload" <<EOF
{"intent":"restart","requestId":"$activation_request_id","reason":"$activation_reason","source":"e2e","priority":2}
EOF
  # At process startup configuration propagation and scheduler watcher
  # registration are asynchronous. Retry only transient 5xx responses with
  # the same request ID, preserving scheduler idempotency.
  for attempt in $(seq 1 5); do
    status="$(curl -sS -o "$TMP_DIR/activation.json" -w '%{http_code}' -X POST \
      "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
      -H 'Content-Type: application/json' -d @"$payload")"
    if [[ "$status" =~ ^20[0-9]$ ]]; then
      return 0
    fi
    if [[ "$status" =~ ^5[0-9][0-9]$ ]] && (( attempt < 5 )); then
      sleep 1
      continue
    fi
    echo "execution activation returned HTTP $status" >&2
    cat "$TMP_DIR/activation.json" >&2 || true
    return 1
  done
}

mkdir -p "$TMP_DIR"
acquire_llm_config_lock
start_mock_provider
wait_ready
wait_mock

curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
write_llm_update_payloads
curl -fsS -X PUT "$BASE_URL/api/v1/workorch/configuration/llm" -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/verify-llm-update.json" >"$TMP_DIR/configuration-updated.json"
grep '"apiKeyConfigured": true' "$TMP_DIR/configuration-updated.json" >/dev/null

DELIVERABLE_DIR="$TMP_DIR/deliverable"
mkdir -p "$DELIVERABLE_DIR"
python3 - >"$DELIVERABLE_DIR/runtime-validation-source.txt" <<'PY'
for index in range(1, 121):
    print(f"LONG-IMPLEMENTATION {index:03d} - deterministic Runtime validation source for terminal viewport verification")
PY
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
  -d '{"title":"'"$WORKSPACE_TITLE"'","description":"TUI viewport long output e2e workspace","deliverableDirectory":"'"$DELIVERABLE_DIR"'","templateId":"generic-governed"}' >"$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"Generate deterministic long final output for TUI viewport verification."}' >"$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["Final Result contains LONG-IMPLEMENTATION 70","Final Result is long enough to require PgDown/PgUp viewport scrolling","Workspace monitor remains visible while main display scrolls"]}' >"$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/skills" -H 'Content-Type: application/json' \
  -d '{"id":"'"$SKILL_ID"'","name":"TUI long output e2e skill","version":"v1","description":"Creates deterministic long final output for TUI viewport verification.","triggerRules":["tui long output"],"instruction":"Return a completed dodAssessment with long user-facing result details.","requiredToolKinds":["llm/chat"],"constraints":["do not call external tools"]}' >"$TMP_DIR/skill.json"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/souls" -H 'Content-Type: application/json' \
  -d '{"id":"'"$SOUL_ID"'","name":"TUI Long Output E2E Soul","description":"Soul for deterministic long-output viewport verification.","businessDomains":["workorch","tui"],"soulMd":"# TUI Long Output E2E\n\nComplete directly through LLM and return long final result details for viewport verification.","allowedSkillRefs":["'"$SKILL_ID"'"],"allowedToolKinds":["llm/chat"]}' >"$TMP_DIR/soul.json"

activation_request_id="tui-long-output-e2e-$VERIFY_SUFFIX"
activation_reason="e2e_restart"
capture_runs_before_activation "$TMP_DIR/activation-before-runs.json"
post_execution_activation
validate_activation_command_response "$TMP_DIR/activation.json" "$activation_request_id" "$activation_reason"
run_id="$(wait_activation_run "$TMP_DIR/activation-before-runs.json" "$TMP_DIR/activation-runs.json" "$TMP_DIR/activation-run.json")"
[[ -n "$run_id" ]]

wait_run_state "$workspace_id" "$run_id" "completed" "$TMP_DIR/status.json"
wait_runtime_result_snapshot

if [[ "${WORKORCH_TUI_LONG_OUTPUT_VERIFY_TERMINAL_RENDER:-1}" == "1" || "${WORKORCH_TUI_LONG_OUTPUT_VERIFY_TERMINAL_RENDER:-1}" == "true" ]]; then
  WORKORCH_TUI_TERMINAL_BASE_URL="$BASE_URL" \
  WORKORCH_TUI_TERMINAL_WORKSPACE_ID="$workspace_id" \
  WORKORCH_TUI_TERMINAL_WORKSPACE_TITLE="$WORKSPACE_TITLE" \
  WORKORCH_TUI_TERMINAL_EXERCISE_SCROLL=1 \
  WORKORCH_TUI_TERMINAL_KEEP_ARTIFACTS="$KEEP_ARTIFACTS" \
  WORKORCH_TUI_TERMINAL_ARTIFACT_DIR="$TMP_DIR/tui-terminal-render" \
    python3 "$ROOT_DIR/scripts/verify-tui-terminal-render-smoke.py"
fi

cat <<EOF
TUI long output e2e passed.

baseUrl:      $BASE_URL
workspace:    $workspace_id
workspaceTitle: $WORKSPACE_TITLE
run:          $run_id
artifacts:    $TMP_DIR

Automated TUI contract coverage:
- The deployed service produced a long Result with LONG-IMPLEMENTATION markers.
- verify-tui-agent covers keyboard PgDown/PgUp scrolling, fixed workspace monitor, and mouse reporting disabled.
- verify-tui-workspace-switch-visual records text snapshots for workspace switch refresh and inactive event isolation.
- verify-tui-terminal-render exercises the real Bubble Tea renderer through a PTY against a deployed service.
- this e2e opens the long-output workspace through the PTY renderer, sends PgDown/PgUp, verifies terminal output contains LONG-IMPLEMENTATION, asserts the Workspaces panel plus bottom prompt/help rows remain visible after scroll, and fails if the TUI emits xterm mouse reporting enable sequences.
- terminal-native drag selection is covered by proxy contract: TUI viewport scrolling stays keyboard-only and the PTY raw output must not enable xterm mouse reporting, so the application does not intercept normal terminal selection/copy.
EOF
