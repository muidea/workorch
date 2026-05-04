#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="${WORKORCH_TUI_LONG_OUTPUT_ARTIFACT_DIR:-$(mktemp -d)}"
BASE_URL="${WORKORCH_TUI_LONG_OUTPUT_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
MOCK_ADDR="${WORKORCH_TUI_LONG_OUTPUT_MOCK_ADDR:-127.0.0.1:18182}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_TUI_LONG_OUTPUT_SUFFIX:-$$}"
PROVIDER_ID="verify-tui-long-output-$VERIFY_SUFFIX"
SKILL_ID="skill-tui-long-output-$VERIFY_SUFFIX"
SOUL_ID="soul-tui-long-output-$VERIFY_SUFFIX"
WORKSPACE_TITLE="TUI long output e2e $VERIFY_SUFFIX"
MOCK_PID=""
MOCK_LOG="$TMP_DIR/tui-long-output-llm-mock.log"
KEEP_ARTIFACTS="${WORKORCH_TUI_LONG_OUTPUT_KEEP_ARTIFACTS:-1}"

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

def long_lines(prefix, count):
    return "\n".join(f"{prefix} {idx:02d} - viewport scroll verification marker" for idx in range(1, count + 1))

def completion_payload(request):
    run_id = request.get("runId", "")
    step_id = request.get("stepId", "") or f"{run_id}/step-1"
    if run_id == "probe":
        return {"completionState": "failed", "thoughtSummary": "probe"}
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
            "items": [{
                "item": "TUI can display a long final result and keyboard-scroll the viewport",
                "state": "satisfied",
                "evidenceRefs": [step_id]
            }],
            "knownLimitations": ["Manual terminal drag selection must be confirmed in the operator terminal."],
            "residualRisks": ["Terminal emulator key binding differences can affect PgUp/PgDown delivery."],
            "nextActions": ["Open the generated workspace in TUI, press PgDown/PgUp, and verify the side panel stays fixed."]
        }
    }

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        size = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(size)
        if self.path == "/v1/embeddings":
            response = {"data": [{"embedding": [0.1, 0.2, 0.3]}]}
        elif self.path == "/v1/chat/completions":
            payload = json.loads(body or b"{}")
            user_content = payload.get("messages", [{}])[-1].get("content", "")
            run_match = re.search(r'"runId": "([^"]+)"', user_content)
            step_match = re.search(r'"stepId": "([^"]+)"', user_content)
            provider_request = {
                "runId": run_match.group(1) if run_match else "",
                "stepId": step_match.group(1) if step_match else "",
            }
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
  WORKORCH_TUI_LONG_OUTPUT_MOCK_ADDR="$MOCK_ADDR" python3 "$TMP_DIR/tui_long_output_llm_mock.py" >>"$MOCK_LOG" 2>&1 &
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

write_llm_update_payloads() {
  python3 - "$TMP_DIR/configuration.json" "$TMP_DIR/original-llm-update.json" "$TMP_DIR/verify-llm-update.json" "$MOCK_BASE_URL" "$PROVIDER_ID" <<'PY'
import json
import sys

config_path, original_path, update_path, mock_base_url, provider_id = sys.argv[1:]
data = json.load(open(config_path, encoding="utf-8"))
llm = data.get("configuration", {}).get("llm", {})
providers = llm.get("providers") or []
default_provider = llm.get("defaultProviderId") or ""
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
        "id": provider_id,
        "provider": "compatible",
        "baseUrl": mock_base_url,
        "model": "tui-long-output-model",
        "timeout": "60s",
        "disabled": False,
        "chatCompletions": True,
        "streaming": False,
        "nonChatOperations": ["embeddings"],
    })
    original["defaultProviderId"] = provider_id
    original["defaultEmbeddingProviderId"] = provider_id
with open(original_path, "w", encoding="utf-8") as handle:
    json.dump(original, handle, separators=(",", ":"))

updated_providers = [item for item in original["providers"] if item["id"] != provider_id]
updated_providers.append({
    "id": provider_id,
    "provider": "compatible",
    "baseUrl": mock_base_url,
    "apiKey": "verify-secret",
    "model": "tui-long-output-model",
    "timeout": "60s",
    "disabled": False,
    "chatCompletions": True,
    "streaming": False,
    "nonChatOperations": ["embeddings"],
})
update = {
    "defaultProviderId": provider_id,
    "defaultEmbeddingProviderId": provider_id,
    "providers": updated_providers,
}
with open(update_path, "w", encoding="utf-8") as handle:
    json.dump(update, handle, separators=(",", ":"))
PY
}

assert_long_result() {
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
else:
    result = results[-1]
    detail = result.get("summaryDetail") or {}
    text = "\n".join(str(detail.get(key) or "") for key in ["goalOutcome", "implementationSummary", "verificationSummary"])
    if "LONG-IMPLEMENTATION 70" not in text:
        errors.append("long implementation marker LONG-IMPLEMENTATION 70 missing from Result summaryDetail")
    if len([line for line in text.splitlines() if line.strip()]) < 100:
        errors.append("Result summaryDetail is not long enough for viewport verification")
artifacts = [item for item in data.get("artifacts", []) if item.get("runId") == run_id]
final_summaries = [
    item for item in artifacts
    if item.get("category") == "final_summary" or str(item.get("id", "")).endswith("/artifact/result-summary")
]
if not final_summaries:
    errors.append("final summary artifact missing")
else:
    summary = final_summaries[-1].get("summary") or ""
    if "LONG-IMPLEMENTATION 70" not in summary:
        errors.append("final summary artifact is missing long output marker")
if errors:
    for item in errors:
        print(item, file=sys.stderr)
    sys.exit(1)
PY
}

mkdir -p "$TMP_DIR"
start_mock_provider
wait_ready
wait_mock

curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
write_llm_update_payloads
curl -fsS -X PUT "$BASE_URL/api/v1/workorch/configuration/llm" -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/verify-llm-update.json" >"$TMP_DIR/configuration-updated.json"
grep '"apiKeyConfigured": true' "$TMP_DIR/configuration-updated.json" >/dev/null

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
  -d '{"title":"'"$WORKSPACE_TITLE"'","description":"TUI viewport long output e2e workspace"}' >"$TMP_DIR/workspace.json"
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

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  -H 'Content-Type: application/json' \
  -d '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"'"$PROVIDER_ID"'","soulId":"'"$SOUL_ID"'","skillIds":["'"$SKILL_ID"'"],"priority":2}' >"$TMP_DIR/activation.json"
run_id="$(json_path "$TMP_DIR/activation.json" "activation.run.id")"
[[ -n "$run_id" ]]

curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/status" >"$TMP_DIR/status.json"
grep '"state": "completed"' "$TMP_DIR/status.json" >/dev/null
curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/snapshot" >"$TMP_DIR/snapshot.json"
assert_long_result

cat <<EOF
TUI long output e2e passed.

baseUrl:      $BASE_URL
workspace:    $workspace_id
workspaceTitle: $WORKSPACE_TITLE
run:          $run_id
artifacts:    $TMP_DIR

Manual TUI confirmation:
1. Open workorch TUI.
2. Switch to workspace "$WORKSPACE_TITLE" ($workspace_id).
3. Confirm the Result contains LONG-IMPLEMENTATION markers.
4. Press PgDown/PgUp:
   - main Result area should scroll;
   - right Workspaces panel should stay fixed;
   - bottom prompt/help rows should stay fixed;
   - terminal mouse drag selection should still copy visible text normally.
EOF
