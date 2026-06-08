#!/usr/bin/env bash

set -euo pipefail

TMP_DIR="$(mktemp -d)"
BASE_URL="${WORKORCH_SKILLHUB_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
MOCK_ADDR="${WORKORCH_SKILLHUB_VERIFY_MOCK_ADDR:-127.0.0.1:18191}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_SKILLHUB_VERIFY_SUFFIX:-$$}"
GLOBAL_SKILL_ID="${WORKORCH_VERIFY_GLOBAL_SKILL_ID:-go-multi-module-dev}"
MCP_SERVER_ID="mcp-skillhub-auto-$VERIFY_SUFFIX"
REPOSITORY_SKILL_ID="skill-skillhub-auto-mcp-$VERIFY_SUFFIX"
SOUL_ID="soul-skillhub-auto-$VERIFY_SUFFIX"
MOCK_PID=""
MOCK_LOG="$TMP_DIR/llm-mock.log"

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
providers = llm.get("providers") or []
default_provider = llm.get("defaultProviderId") or "verify-skillhub"
default_embedding_provider = llm.get("defaultEmbeddingProviderId") or ""

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
    "defaultEmbeddingProviderId": default_embedding_provider,
    "providers": [clean_provider(item) for item in providers if item.get("id")],
}
if not original["providers"]:
    original["providers"].append({
        "id": "verify-skillhub",
        "provider": "compatible",
        "baseUrl": mock_base_url,
        "model": "verify-model",
        "timeout": "60s",
        "disabled": False,
        "chatCompletions": True,
        "streaming": False,
        "nonChatOperations": ["embeddings"],
    })
    original["defaultProviderId"] = "verify-skillhub"
with open(original_path, "w", encoding="utf-8") as handle:
    json.dump(original, handle, separators=(",", ":"))

updated_providers = [item for item in original["providers"] if item["id"] != "verify-skillhub"]
updated_providers.append({
    "id": "verify-skillhub",
    "provider": "compatible",
    "baseUrl": mock_base_url,
    "apiKey": "verify-secret",
    "model": "verify-model",
    "timeout": "60s",
    "disabled": False,
    "chatCompletions": True,
    "streaming": False,
    "nonChatOperations": ["embeddings"],
})
update = {
    "defaultProviderId": "verify-skillhub",
    "defaultEmbeddingProviderId": original.get("defaultEmbeddingProviderId", ""),
    "providers": updated_providers,
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

trap report_failure ERR
trap cleanup EXIT

command -v skill-hub >/dev/null
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

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  -H 'Content-Type: application/json' \
  -d '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"verify-skillhub","soulId":"'"$SOUL_ID"'","soulBindings":{"planning":"'"$SOUL_ID"'","execution":"'"$SOUL_ID"'","decision":"'"$SOUL_ID"'","approval":"'"$SOUL_ID"'","validation":"'"$SOUL_ID"'","artifact":"'"$SOUL_ID"'"},"priority":2}' >"$TMP_DIR/activation.json"
grep '"state": "completed"' "$TMP_DIR/activation.json" >/dev/null
run_id="$(json_path "$TMP_DIR/activation.json" "activation.run.id")"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/snapshot" >"$TMP_DIR/run-snapshot.json"
validate_snapshot
curl -fsS "$BASE_URL/api/v1/workorch/memory?workspaceId=$workspace_id" >"$TMP_DIR/memory.json"
grep 'Selected skills completed run' "$TMP_DIR/memory.json" >/dev/null
grep "$REPOSITORY_SKILL_ID" "$TMP_DIR/memory.json" >/dev/null

echo "workorch SkillHub workspace execution verification passed for deployed workorch service $BASE_URL"
