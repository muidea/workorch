#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_SKILLHUB_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-}}"
source "$ROOT_DIR/scripts/lib/verify-isolated-runtime.sh"
workorch_verify_require_isolated_runtime "$BASE_URL" "verify-skillhub-workspace-execution"
TMP_DIR="$(mktemp -d)"
MOCK_ADDR="${WORKORCH_SKILLHUB_VERIFY_MOCK_ADDR:-127.0.0.1:18191}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_SKILLHUB_VERIFY_SUFFIX:-$$}"
GLOBAL_SKILL_ID="${WORKORCH_VERIFY_GLOBAL_SKILL_ID:-go-multi-module-dev}"
RUN_WAIT_SECONDS="${WORKORCH_SKILLHUB_VERIFY_RUN_WAIT_SECONDS:-90}"
ACTIVATION_RUN_WAIT_SECONDS="${WORKORCH_SKILLHUB_VERIFY_ACTIVATION_RUN_WAIT_SECONDS:-60}"
MCP_SERVER_ID="mcp-skillhub-auto-$VERIFY_SUFFIX"
REPOSITORY_SKILL_ID="skill-skillhub-auto-mcp-$VERIFY_SUFFIX"
SOUL_ID="soul-skillhub-auto-$VERIFY_SUFFIX"
MOCK_PID=""
MOCK_LOG="$TMP_DIR/llm-mock.log"
SKILL_HUB_DIR="${WORKORCH_SKILLHUB_HOME:-$HOME/.skill-hub}"
SKILL_HUB_STATE="$SKILL_HUB_DIR/state.json"
SKILL_HUB_STATE_BACKUP="$TMP_DIR/skillhub-state-original.json"
SKILL_HUB_STATE_REPAIRED=0

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "workorch SkillHub workspace execution verification failed for deployed workorch service $BASE_URL" >&2
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
  if [[ "$SKILL_HUB_STATE_REPAIRED" == "1" && -f "$SKILL_HUB_STATE_BACKUP" ]]; then
    cp "$SKILL_HUB_STATE_BACKUP" "$SKILL_HUB_STATE" >/dev/null 2>&1 || true
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

start_mock_provider() {
  cat >"$TMP_DIR/llm_mock_provider.py" <<'PY'
import json
import os
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

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
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length) if length else b"{}"
        try:
            request = json.loads(body.decode("utf-8"))
        except Exception:
            request = {}
        if self.path.endswith("/chat/completions"):
            user_content = "\n".join(str(item.get("content", "")) for item in request.get("messages") or [])
            matched_step = re.search(r'"stepId"\s*:\s*"([^"]+)"', user_content)
            step_id = matched_step.group(1) if matched_step else "step-1"
            payload = {
                "thoughtSummary": "SkillHub automatic skill selection completed.",
                "completionState": "completed",
                "dodAssessment": {
                    "satisfied": True,
                    "summary": "SkillHub global and workspace skills were selected automatically; required MCP was auto-bound.",
                    "items": [
                        {
                            "item": "go multi module dev SkillHub global skill is selected",
                            "state": "satisfied",
                            "evidenceRefs": [step_id]
                        },
                        {
                            "item": "go multi module dev SkillHub workspace skill is selected",
                            "state": "satisfied",
                            "evidenceRefs": [step_id]
                        },
                        {
                            "item": "skillhub auto mcp required MCP is auto-bound",
                            "state": "satisfied",
                            "evidenceRefs": [step_id]
                        },
                        {
                            "item": "skill feedback memory is written",
                            "state": "satisfied",
                            "evidenceRefs": [step_id]
                        }
                    ],
                    "goalCoverage": {
                        "satisfied": True,
                        "items": [
                            {
                                "requirement": "verify go multi module dev skillhub auto mcp execution",
                                "state": "satisfied",
                                "evidenceRefs": [step_id]
                            }
                        ]
                    }
                }
            }
            content = json.dumps(payload, ensure_ascii=False)
            response = {"choices": [{"message": {"role": "assistant", "content": content}}]}
        elif self.path.endswith("/embeddings"):
            response = {"data": [{"embedding": [0.1, 0.2, 0.3], "index": 0}], "model": "verify-embedding"}
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
        return

addr = os.environ.get("MOCK_ADDR", "127.0.0.1:18191")
host, port = addr.rsplit(":", 1)
ThreadingHTTPServer((host, int(port)), Handler).serve_forever()
PY
  MOCK_ADDR="$MOCK_ADDR" python3 "$TMP_DIR/llm_mock_provider.py" >"$MOCK_LOG" 2>&1 &
  MOCK_PID=$!
  sleep 0.5
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

assert_global_skill_enabled() {
  skill-hub status --global --json --agent codex >"$TMP_DIR/skillhub-global-status.json"
  python3 - "$TMP_DIR/skillhub-global-status.json" "$GLOBAL_SKILL_ID" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
skill_id = sys.argv[2]
for item in data.get("items") or []:
    if item.get("skill_id") == skill_id and item.get("agent") == "codex":
        sys.exit(0)
print(f"global SkillHub skill {skill_id!r} is not enabled for codex; run: skill-hub use --pattern {skill_id!r} --global --agent codex && skill-hub apply --pattern {skill_id!r} --global --agent codex", file=sys.stderr)
sys.exit(1)
PY
}

ensure_skillhub_project_state_valid() {
  if [[ ! -f "$SKILL_HUB_STATE" ]]; then
    return
  fi
  if python3 - "$SKILL_HUB_STATE" <<'PY' >/dev/null 2>&1
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    json.load(handle)
PY
  then
    return
  fi
  cp "$SKILL_HUB_STATE" "$SKILL_HUB_STATE_BACKUP"
  printf '{}\n' >"$SKILL_HUB_STATE"
  SKILL_HUB_STATE_REPAIRED=1
  echo "temporarily repaired invalid SkillHub project state for verification: $SKILL_HUB_STATE" >&2
}

validate_snapshot() {
  python3 - "$TMP_DIR/run-snapshot.json" "$workspace_id" "$GLOBAL_SKILL_ID" "$REPOSITORY_SKILL_ID" "$MCP_SERVER_ID" "$SOUL_ID" <<'PY'
import json
import sys

snapshot_path, workspace_id, global_skill_id, repo_skill_id, mcp_server_id, expected_soul_id = sys.argv[1:]
data = json.load(open(snapshot_path, encoding="utf-8"))
run = data.get("snapshot", {}).get("run", {})
ctx = run.get("executionContext") or {}
skill_ids = ctx.get("skillIds") or []
mcp_ids = ctx.get("mcpServerIds") or []
selection = ctx.get("skillSelection") or {}
mcp_selection = ctx.get("mcpSelection") or {}
routing = ctx.get("soulRouting") or {}
bindings = routing.get("bindings") or {}
execution_binding = bindings.get("execution") or {}
expected_global = f"skillhub:global:{global_skill_id}"
expected_workspace = f"skillhub:workspace:{workspace_id}:{global_skill_id}"
errors = []
if run.get("state") != "completed":
    errors.append(f"run state is {run.get('state')!r}, want completed")
if routing.get("status") != "resolved":
    errors.append(f"soulRouting.status is {routing.get('status')!r}, want resolved")
if execution_binding.get("soulId") != expected_soul_id:
    errors.append(f"execution soul binding is {execution_binding!r}, want {expected_soul_id}")
for expected in [expected_global, expected_workspace, repo_skill_id]:
    if expected not in skill_ids:
        errors.append(f"missing selected skill {expected!r}: {skill_ids}")
if mcp_server_id not in mcp_ids:
    errors.append(f"missing selected MCP {mcp_server_id!r}: {mcp_ids}")
if selection.get("policy") != "goal_dod_auto_match" or selection.get("status") != "selected":
    errors.append(f"unexpected skillSelection: {selection}")
if mcp_server_id not in (mcp_selection.get("autoBoundMCPServerIds") or []):
    errors.append(f"required MCP was not auto-bound: {mcp_selection}")
if not (data.get("snapshot", {}).get("results") or []):
    errors.append("missing accepted Result")
if not (data.get("snapshot", {}).get("artifacts") or []):
    errors.append("missing Artifact")
if errors:
    for item in errors:
        print(item, file=sys.stderr)
    sys.exit(1)
PY
}

wait_run_state() {
  local run_id="$1"
  local expected_state="$2"
  local output_file="$3"
  local deadline=$((SECONDS + RUN_WAIT_SECONDS))

  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$output_file"
    if python3 - "$output_file" "$expected_state" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
state = (data.get("status") or {}).get("state")
raise SystemExit(0 if state == sys.argv[2] else 1)
PY
    then
      return 0
    fi
    sleep 1
  done

  echo "timed out waiting for run $run_id state $expected_state" >&2
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
    for item in errors:
        print(item, file=sys.stderr)
    sys.exit(1)
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
before = set()
try:
    before_data = json.load(open(sys.argv[2], encoding="utf-8"))
    before = {str(item.get("id") or "") for item in before_data.get("runs", []) if item.get("id")}
except FileNotFoundError:
    before = set()
new_runs = [
    item for item in data.get("runs", [])
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

data = json.load(open(sys.argv[1], encoding="utf-8"))
run_id = sys.argv[2]
runs = data.get("runs", [])
run = next((item for item in runs if item.get("id") == run_id), {"id": run_id})
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
  cat "$runs_file" >&2 2>/dev/null || true
  return 1
}

trap report_failure ERR
trap cleanup EXIT

command -v skill-hub >/dev/null
ensure_skillhub_project_state_valid
assert_global_skill_enabled
start_mock_provider

curl -fsS "$BASE_URL/readyz" | grep '"status": "ready"' >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
write_llm_update_payloads
curl -fsS -X PUT "$BASE_URL/api/v1/workorch/configuration/llm" -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/verify-llm-update.json" >"$TMP_DIR/configuration-updated.json"
grep '"apiKeyConfigured": true' "$TMP_DIR/configuration-updated.json" >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
  -d '{"title":"skillhub execution workspace","description":"verify SkillHub global and workspace Skill selection","templateId":"generic-governed"}' >"$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
deliverable_directory="$(json_path "$TMP_DIR/workspace.json" "workspace.deliverableDirectory")"
[[ -n "$workspace_id" ]]
[[ -n "$deliverable_directory" ]]
mkdir -p "$deliverable_directory"
(cd "$deliverable_directory" && printf 'y\n' | skill-hub use --pattern "$GLOBAL_SKILL_ID" >/dev/null && skill-hub apply --pattern "$GLOBAL_SKILL_ID" >/dev/null)

curl -fsS "$BASE_URL/api/v1/workorch/skills?workspaceId=$workspace_id" >"$TMP_DIR/skills.json"
grep "\"id\": \"skillhub:global:$GLOBAL_SKILL_ID\"" "$TMP_DIR/skills.json" >/dev/null
grep "\"id\": \"skillhub:workspace:$workspace_id:$GLOBAL_SKILL_ID\"" "$TMP_DIR/skills.json" >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/mcp/servers" -H 'Content-Type: application/json' \
  -d '{"id":"'"$MCP_SERVER_ID"'","name":"SkillHub Auto MCP","transport":"stdio","endpoint":"skillhub-helper","enabled":true,"allowedTools":["repo.summary"],"timeoutMillis":60000}' >"$TMP_DIR/mcp-server.json"
grep "\"id\": \"$MCP_SERVER_ID\"" "$TMP_DIR/mcp-server.json" >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/skills" -H 'Content-Type: application/json' \
  -d '{"id":"'"$REPOSITORY_SKILL_ID"'","name":"SkillHub Auto MCP Skill","version":"v1","description":"repository skill used to verify required MCP auto binding","triggerRules":["skillhub auto mcp"],"instruction":"Require the configured MCP server when this skill is selected.","requiredToolKinds":["llm/chat"],"requiredMCPServerRefs":["'"$MCP_SERVER_ID"'"],"constraints":["generic Skill to MCP binding only"]}' >"$TMP_DIR/repository-skill.json"
grep "\"id\": \"$REPOSITORY_SKILL_ID\"" "$TMP_DIR/repository-skill.json" >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/souls" -H 'Content-Type: application/json' \
  -d '{"id":"'"$SOUL_ID"'","name":"SkillHub Auto Selection Soul","description":"scope SkillHub e2e auto selection to the current verification data","businessDomains":["workorch"],"soulMd":"# SkillHub Auto Selection Soul\n\nUse only the allowed verification skills and produce evidence-backed DoD assessment.\n\n## Planning\n\ngoal-first\n\n## Decision\n\ncomplete when evidence is sufficient\n\n## Approval\n\nno approval needed for deterministic verification\n\n## Validation\n\nrequire DoD evidence\n\n## Output Contract\n\nworkorch llm step contract","allowedSkillRefs":["skillhub:global:'"$GLOBAL_SKILL_ID"'","skillhub:workspace:'"$workspace_id"':'"$GLOBAL_SKILL_ID"'","'"$REPOSITORY_SKILL_ID"'"],"allowedMCPServerRefs":["'"$MCP_SERVER_ID"'"],"allowedToolKinds":["llm/chat"]}' >"$TMP_DIR/soul.json"
grep "\"id\": \"$SOUL_ID\"" "$TMP_DIR/soul.json" >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"verify go multi module dev skillhub auto mcp execution"}' >"$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["go multi module dev SkillHub global skill is selected","go multi module dev SkillHub workspace skill is selected","skillhub auto mcp required MCP is auto-bound","skill feedback memory is written"]}' >"$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"

activation_request_id="skillhub-e2e-$VERIFY_SUFFIX"
activation_reason="e2e_resume"
capture_runs_before_activation "$TMP_DIR/activation-before-runs.json"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  -H 'Content-Type: application/json' \
  -d '{"intent":"resume","requestId":"'"$activation_request_id"'","reason":"'"$activation_reason"'","source":"e2e","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmTarget":{"providerId":"verify","modelId":"verify-model"},"soulId":"'"$SOUL_ID"'","soulBindings":{"planning":"'"$SOUL_ID"'","execution":"'"$SOUL_ID"'","decision":"'"$SOUL_ID"'","approval":"'"$SOUL_ID"'","validation":"'"$SOUL_ID"'","artifact":"'"$SOUL_ID"'"},"priority":2}' >"$TMP_DIR/activation.json"
validate_activation_command_response "$TMP_DIR/activation.json" "$activation_request_id" "$activation_reason"
run_id="$(wait_activation_run "$TMP_DIR/activation-before-runs.json" "$TMP_DIR/activation-runs.json" "$TMP_DIR/activation-run.json")"
wait_run_state "$run_id" "completed" "$TMP_DIR/run-status-completed.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/snapshot" >"$TMP_DIR/run-snapshot.json"
validate_snapshot
curl -fsS "$BASE_URL/api/v1/workorch/memory?workspaceId=$workspace_id" >"$TMP_DIR/memory.json"
grep 'Selected skills completed run' "$TMP_DIR/memory.json" >/dev/null
grep "$REPOSITORY_SKILL_ID" "$TMP_DIR/memory.json" >/dev/null

echo "workorch SkillHub workspace execution verification passed for deployed workorch service $BASE_URL"
