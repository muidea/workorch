#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_SOFTWARE_DEV_E2E_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
MOCK_ADDR="${WORKORCH_SOFTWARE_DEV_E2E_MOCK_ADDR:-127.0.0.1:18193}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_SOFTWARE_DEV_E2E_SUFFIX:-$$}"
TMP_DIR="${WORKORCH_SOFTWARE_DEV_E2E_ARTIFACT_DIR:-$(mktemp -d)}"
MOCK_PID=""
MOCK_LOG="$TMP_DIR/software-dev-e2e-llm-mock.log"

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
}

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "software-dev goal/dod e2e failed" >&2
  echo "baseUrl=$BASE_URL artifacts=$TMP_DIR" >&2
  if [[ -f "$MOCK_LOG" ]]; then
    echo "--- software-dev-e2e-llm-mock.log ---" >&2
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

url_encode() {
  python3 - "$1" <<'PY'
import sys
from urllib.parse import quote
print(quote(sys.argv[1], safe=""))
PY
}

wait_ready() {
  for _ in $(seq 1 60); do
    if curl -fsS "$BASE_URL/readyz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl -fsS "$BASE_URL/readyz" >/dev/null
}

wait_mock() {
  for _ in $(seq 1 60); do
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

wait_run_state() {
  local run_id="$1"
  local expected_state="$2"
  local output_file="$3"
  local deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/status" >"$output_file"
    if python3 - "$output_file" "$expected_state" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
raise SystemExit(0 if (data.get("status") or {}).get("state") == sys.argv[2] else 1)
PY
    then
      return 0
    fi
    sleep 1
  done
  cat "$output_file" >&2
  return 1
}

write_llm_update_payloads() {
  python3 - "$TMP_DIR/configuration.json" "$TMP_DIR/original-llm-update.json" "$TMP_DIR/verify-llm-update.json" "$MOCK_BASE_URL" <<'PY'
import json
import sys

config_path, original_path, update_path, mock_base_url = sys.argv[1:]
data = json.load(open(config_path, encoding="utf-8"))
llm = data.get("configuration", {}).get("llm", {})
providers = llm.get("providers") or []
default_provider = llm.get("defaultProviderId") or "verify"
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
        "id": "verify",
        "provider": "compatible",
        "baseUrl": mock_base_url,
        "model": "verify-model",
        "timeout": "60s",
        "disabled": False,
        "chatCompletions": True,
        "streaming": False,
        "nonChatOperations": ["embeddings"],
    })
    original["defaultProviderId"] = "verify"
    original["defaultEmbeddingProviderId"] = "verify"
with open(original_path, "w", encoding="utf-8") as handle:
    json.dump(original, handle, separators=(",", ":"))

updated_providers = [item for item in original["providers"] if item["id"] != "verify"]
updated_providers.append({
    "id": "verify",
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
    "defaultProviderId": "verify",
    "defaultEmbeddingProviderId": "verify",
    "providers": updated_providers,
}
with open(update_path, "w", encoding="utf-8") as handle:
    json.dump(update, handle, separators=(",", ":"))
PY
}

start_mock_provider() {
  cat >"$TMP_DIR/software_dev_e2e_mock.py" <<'PY'
import json
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import os

initial_runs = []

BUGGY_SOURCE = "package calculator\n\nfunc Add(a, b int) int { return a - b }\n"
FIXED_DIFF = "--- a/calculator.go\n+++ b/calculator.go\n@@ -1,3 +1,3 @@\n package calculator\n \n-func Add(a, b int) int { return a - b }\n+func Add(a, b int) int { return a + b }\n"
NEW_PROJECT_SOURCE = "package targetproject\n\nfunc Enabled() bool { return true }\n"
NEW_PROJECT_TEST = "package targetproject\n\nimport \"testing\"\n\nfunc TestEnabled(t *testing.T) {\n\tif !Enabled() {\n\t\tt.Fatal(\"Enabled() = false, want true\")\n\t}\n}\n"
NEW_PROJECT_README = "# Target Project\n\nGenerated by software-dev e2e to verify empty workspace initialization.\n"

def parse_request(payload):
    messages = payload.get("messages") or []
    user_content = messages[-1].get("content", "") if messages else ""
    run_match = re.search(r'"runId": "([^"]+)"', user_content)
    step_match = re.search(r'"stepId": "([^"]+)"', user_content)
    run_id = run_match.group(1) if run_match else ""
    step_id = step_match.group(1) if step_match else ""
    return run_id, step_id, user_content

def scenario_for_run(run_id):
    if run_id not in initial_runs:
        initial_runs.append(run_id)
    return initial_runs.index(run_id)

def response_for(run_id, step_id):
    if run_id == "probe":
        return {"completionState": "failed", "thoughtSummary": "probe"}
    scenario = scenario_for_run(run_id)
    if scenario == 0:
        if step_id.endswith("/step-1"):
            return {
                "completionState": "continue",
                "thoughtSummary": "Plan graph for patch write and test execution.",
                "executionGraph": {
                    "nodes": [
                        {
                            "id": "write-buggy-source",
                            "kind": "patch/apply",
                            "input": {
                                "patchType": "file_write",
                                "targetRef": "calculator.go",
                                "content": BUGGY_SOURCE
                            }
                        },
                        {
                            "id": "run-tests",
                            "kind": "shell/command",
                            "dependsOn": ["write-buggy-source"],
                            "input": {"command": "go", "args": ["test", "./..."]}
                        }
                    ]
                }
            }
        if "zz-llm-after-graph-" in step_id:
            return {
                "completionState": "completed",
                "thoughtSummary": "DoD satisfied after repair.",
                "dodAssessment": {
                    "satisfied": True,
                    "summary": "Software development DoD satisfied after automatic repair.",
                    "goalOutcome": "Add now returns the sum of two integers.",
                    "implementationSummary": "Repaired calculator.go via unified diff.",
                    "verificationSummary": "go test ./... passed after repair.",
                    "items": [
                        {"item": "implementation fixed", "state": "satisfied", "evidenceRefs": ["run-tests-fixed"]},
                        {"item": "tests pass", "state": "satisfied", "evidenceRefs": ["run-tests-fixed"]}
                    ]
                }
            }
        if "zz-llm-repair-" in step_id:
            return {
                "completionState": "continue",
                "thoughtSummary": "Repair by applying unified diff and rerunning tests.",
                "executionGraph": {
                    "nodes": [
                        {
                            "id": "fix-source",
                            "kind": "patch/apply",
                            "input": {
                                "patchType": "unified_diff",
                                "targetRef": "calculator.go",
                                "diff": FIXED_DIFF
                            }
                        },
                        {
                            "id": "run-tests-fixed",
                            "kind": "shell/command",
                            "dependsOn": ["fix-source"],
                            "input": {"command": "go", "args": ["test", "./..."]}
                        }
                    ]
                }
            }
        return {"completionState": "continue", "thoughtSummary": "Continue execution."}
    if scenario == 1:
        if step_id.startswith(run_id + "/step-1"):
            return {
                "completionState": "continue",
                "thoughtSummary": "Plan failing test command.",
                "executionGraph": {
                    "nodes": [
                        {
                            "id": "run-tests",
                            "kind": "shell/command",
                            "input": {
                                "command": "bash",
                                "args": ["-lc", "echo forced_failure >&2; exit 1"]
                            }
                        }
                    ]
                }
            }
        if "zz-llm-repair-" in step_id:
            attempt = 1
            match = re.search(r"-(\d+)$", step_id)
            if match:
                attempt = int(match.group(1))
            return {
                "completionState": "continue",
                "thoughtSummary": f"Repair attempt {attempt} still runs failing command.",
                "executionGraph": {
                    "nodes": [
                        {
                            "id": f"run-tests-retry-{attempt}",
                            "kind": "shell/command",
                            "input": {
                                "command": "bash",
                                "args": ["-lc", f"echo forced_failure_attempt_{attempt} >&2; exit 1"]
                            }
                        }
                    ]
                }
            }
        return {"completionState": "continue", "thoughtSummary": "Continue execution."}
    if scenario == 2:
        if step_id.startswith(run_id + "/step-1"):
            return {
                "completionState": "blocked_on_decision",
                "thoughtSummary": "Target workspace external dependency configuration is required before integration verification.",
                "requiredDecision": {
                    "reason": "目标项目外部依赖配置缺失，无法完成集成验证。",
                    "decisionKind": "provide_missing_info",
                    "question": "请提供目标项目测试所需的外部依赖配置，包含 endpoint、model 与认证 secret；该信息仅属于 target_workspace/external_dependency。",
                    "context": {
                        "scope": "target_workspace",
                        "dependencyScope": "external_dependency",
                        "requiredFields": ["endpoint", "model", "apiKey"],
                        "sourceAvailability": "empty_workspace_new_project"
                    }
                }
            }
        if "zz-llm-after-graph-" in step_id:
            return {
                "completionState": "completed",
                "thoughtSummary": "Empty workspace project generation and verification are complete.",
                "dodAssessment": {
                    "satisfied": True,
                    "summary": "Complex software-dev DoD satisfied for empty workspace initialization.",
                    "goalOutcome": "Created a minimal Go project from an empty workspace and verified it.",
                    "implementationSummary": "Generated go.mod, feature.go, feature_test.go, and README.md.",
                    "verificationSummary": "go test ./... passed with an effective assertion.",
                    "items": [
                        {"item": "empty workspace initializes without source request", "state": "satisfied", "evidenceRefs": ["write-go-mod", "write-source"]},
                        {"item": "external dependency config is scoped to target workspace", "state": "satisfied", "evidenceRefs": [step_id]},
                        {"item": "tests include effective assertion", "state": "satisfied", "evidenceRefs": ["run-empty-project-tests"]},
                        {"item": "deliverable evidence is indexed", "state": "satisfied", "evidenceRefs": ["write-readme", "run-empty-project-tests"]}
                    ]
                }
            }
        if "llm-after-decision" in step_id:
            return {
                "completionState": "continue",
                "thoughtSummary": "Decision information was provided; create a minimal project from the empty workspace and run tests.",
                "executionGraph": {
                    "nodes": [
                        {
                            "id": "write-go-mod",
                            "kind": "patch/apply",
                            "input": {
                                "patchType": "file_write",
                                "targetRef": "go.mod",
                                "content": "module example.com/targetproject\n\ngo 1.22\n"
                            }
                        },
                        {
                            "id": "write-source",
                            "kind": "patch/apply",
                            "dependsOn": ["write-go-mod"],
                            "input": {
                                "patchType": "file_write",
                                "targetRef": "feature.go",
                                "content": NEW_PROJECT_SOURCE
                            }
                        },
                        {
                            "id": "write-tests",
                            "kind": "patch/apply",
                            "dependsOn": ["write-source"],
                            "input": {
                                "patchType": "file_write",
                                "targetRef": "feature_test.go",
                                "content": NEW_PROJECT_TEST
                            }
                        },
                        {
                            "id": "write-readme",
                            "kind": "patch/apply",
                            "dependsOn": ["write-tests"],
                            "input": {
                                "patchType": "file_write",
                                "targetRef": "README.md",
                                "content": NEW_PROJECT_README
                            }
                        },
                        {
                            "id": "run-empty-project-tests",
                            "kind": "shell/command",
                            "dependsOn": ["write-readme"],
                            "input": {"command": "go", "args": ["test", "./..."]}
                        }
                    ]
                }
            }
        return {"completionState": "continue", "thoughtSummary": "Continue execution."}
    if scenario == 3:
        if step_id.startswith(run_id + "/step-1"):
            return {
                "completionState": "completed",
                "thoughtSummary": "Weak verification was reported as if it were enough.",
                "dodAssessment": {
                    "satisfied": True,
                    "summary": "Weak evidence should be rejected by the platform.",
                    "goalOutcome": "No implementation evidence.",
                    "implementationSummary": "No files were changed.",
                    "verificationSummary": "go test ./... passed with [no test files].",
                    "items": [
                        {"item": "tests pass", "state": "satisfied", "evidenceRefs": [step_id]}
                    ]
                }
            }
        if "zz-llm-repair-" in step_id:
            return {
                "completionState": "completed",
                "thoughtSummary": "Repair still reports weak verification as if it were enough.",
                "dodAssessment": {
                    "satisfied": True,
                    "summary": "Weak evidence should still be rejected by the platform.",
                    "goalOutcome": "No new implementation evidence.",
                    "implementationSummary": "No files were changed during repair.",
                    "verificationSummary": "go test ./... passed with [no test files].",
                    "items": [
                        {"item": "tests pass", "state": "satisfied", "evidenceRefs": [step_id]}
                    ]
                }
            }
        return {"completionState": "continue", "thoughtSummary": "Continue execution."}
    return {"completionState": "continue", "thoughtSummary": "Continue execution."}

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
        run_id, step_id, user_content = parse_request(payload)
        request_log = os.environ.get("WORKORCH_SOFTWARE_DEV_E2E_REQUEST_LOG", "")
        if request_log:
            with open(request_log, "a", encoding="utf-8") as handle:
                handle.write(json.dumps({"runId": run_id, "stepId": step_id, "content": user_content}, ensure_ascii=False, separators=(",", ":")) + "\n")
        decision = response_for(run_id, step_id)
        content = json.dumps(decision, separators=(",", ":"))
        response = {"choices": [{"message": {"content": content}}]}
        encoded = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)

host, port_text = os.environ.get("WORKORCH_SOFTWARE_DEV_E2E_MOCK_ADDR", "127.0.0.1:18193").split(":", 1)
ThreadingHTTPServer((host, int(port_text)), Handler).serve_forever()
PY
  WORKORCH_SOFTWARE_DEV_E2E_MOCK_ADDR="$MOCK_ADDR" \
    WORKORCH_SOFTWARE_DEV_E2E_REQUEST_LOG="$TMP_DIR/software-dev-e2e-llm-requests.jsonl" \
    python3 "$TMP_DIR/software_dev_e2e_mock.py" >>"$MOCK_LOG" 2>&1 &
  MOCK_PID=$!
}

trap report_failure ERR
trap cleanup EXIT

cd "$ROOT_DIR"
start_mock_provider
wait_mock
wait_ready

curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
write_llm_update_payloads
curl -fsS -X PUT "$BASE_URL/api/v1/workorch/configuration/llm" -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/verify-llm-update.json" >"$TMP_DIR/configuration-updated.json"

workspace_workdir="$TMP_DIR/workspace-dev"
mkdir -p "$workspace_workdir"
cat >"$workspace_workdir/go.mod" <<'EOF'
module example.com/srddev

go 1.22
EOF
cat >"$workspace_workdir/calculator.go" <<'EOF'
package calculator

func Add(a, b int) int { return a + b }
EOF
cat >"$workspace_workdir/calculator_test.go" <<'EOF'
package calculator

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatalf("Add(2,3)=%d, want 5", Add(2, 3))
	}
}
EOF

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
  -d '{"title":"software-dev srd e2e","description":"verify goal/dod auto repair closure and repair exhaustion escalation","workingDirectory":"'"$workspace_workdir"'","templateId":"software-dev"}' >"$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"完成软件开发闭环：通过 executionGraph 修改代码并运行测试，失败时自动修复，最终基于证据输出 DoD 评估。"}' >"$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
[[ -n "$goal_id" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["代码修改通过 Patch 证据可追溯","go test ./... 最终通过","结果包含 DoD 评估和交付物索引"]}' >"$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"
[[ -n "$dod_id" ]]

# Scenario-1: software-dev automatic repair and delivery closure.
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  -H 'Content-Type: application/json' \
  -d '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"verify","priority":2}' >"$TMP_DIR/scenario1-activation.json"
scenario1_run_id="$(json_path "$TMP_DIR/scenario1-activation.json" "activation.run.id")"
wait_run_state "$scenario1_run_id" "completed" "$TMP_DIR/scenario1-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario1_run_id/snapshot" >"$TMP_DIR/scenario1-snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario1_run_id/observability" >"$TMP_DIR/scenario1-observability.json"

python3 - "$TMP_DIR/scenario1-snapshot.json" "$workspace_workdir/calculator.go" <<'PY'
import json
import sys
from pathlib import Path

snapshot = json.load(open(sys.argv[1], encoding="utf-8"))["snapshot"]
target = Path(sys.argv[2]).read_text(encoding="utf-8")
errors = []
run = snapshot.get("run") or {}
if run.get("state") != "completed":
    errors.append(f"run.state={run.get('state')!r}, want completed")
if "return a + b" not in target:
    errors.append("calculator.go did not converge to fixed implementation")
patches = snapshot.get("patches") or []
if len(patches) < 2:
    errors.append(f"patch evidence count={len(patches)}, want >=2")
if not any(item.get("patchType") == "unified_diff" for item in patches):
    errors.append("missing unified_diff repair patch evidence")
results = snapshot.get("results") or []
accepted = [item for item in results if item.get("state") == "accepted"]
if not accepted:
    errors.append("missing accepted result")
artifacts = snapshot.get("artifacts") or []
if not any("```diff" in (item.get("summary") or "") for item in artifacts):
    errors.append("missing diff artifact summary")
assessment = (accepted[-1].get("doDAssessment") if accepted else {}) or {}
if assessment.get("satisfied") is not True:
    errors.append(f"dodAssessment.satisfied={assessment.get('satisfied')!r}, want true")
if not (assessment.get("items") or []):
    errors.append("dodAssessment.items empty")
repair_loop = (snapshot.get("taskProgress") or {}).get("repairLoop") or {}
if int(repair_loop.get("attemptCount") or 0) < 1:
    errors.append(f"repairLoop.attemptCount={repair_loop.get('attemptCount')!r}, want >=1")
routing = ((run.get("executionContext") or {}).get("soulRouting") or {})
if routing.get("status") != "resolved":
    errors.append(f"soulRouting.status={routing.get('status')!r}, want resolved")
bindings = routing.get("bindings") or {}
execution = bindings.get("execution") or {}
if execution.get("soulId") != "soul-software-dev-execution":
    errors.append(f"soulRouting.execution={execution!r}, want soul-software-dev-execution")
if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY

# Scenario-2: repair loop exhausted then escalated decision.
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  -H 'Content-Type: application/json' \
  -d '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"verify","dialogLanguage":"zh-CN","priority":2}' >"$TMP_DIR/scenario2-activation.json"
scenario2_run_id="$(json_path "$TMP_DIR/scenario2-activation.json" "activation.run.id")"
wait_run_state "$scenario2_run_id" "blocked_on_decision" "$TMP_DIR/scenario2-status.json"
scenario2_ticket_id="$(json_path "$TMP_DIR/scenario2-status.json" "status.blockingTicketId")"
scenario2_ticket_path="$(url_encode "$scenario2_ticket_id")"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario2_run_id/snapshot" >"$TMP_DIR/scenario2-snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/decisions/$scenario2_ticket_path" >"$TMP_DIR/scenario2-ticket.json"

python3 - "$TMP_DIR/scenario2-snapshot.json" "$TMP_DIR/scenario2-ticket.json" "$scenario2_ticket_id" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8"))["snapshot"]
ticket = json.load(open(sys.argv[2], encoding="utf-8"))["decision"]
ticket_id = sys.argv[3]
errors = []
run = snapshot.get("run") or {}
if run.get("state") != "blocked_on_decision":
    errors.append(f"run.state={run.get('state')!r}, want blocked_on_decision")
if run.get("blockingTicketId") != ticket_id:
    errors.append(f"blockingTicketId={run.get('blockingTicketId')!r}, want {ticket_id!r}")
repair_loop = (snapshot.get("taskProgress") or {}).get("repairLoop") or {}
if repair_loop.get("state") != "escalated_decision":
    errors.append(f"repairLoop.state={repair_loop.get('state')!r}, want escalated_decision")
if int(repair_loop.get("attemptCount") or 0) < 3:
    errors.append(f"repairLoop.attemptCount={repair_loop.get('attemptCount')!r}, want >=3")
if repair_loop.get("escalationTicketId") != ticket_id:
    errors.append(f"repairLoop.escalationTicketId={repair_loop.get('escalationTicketId')!r}, want {ticket_id!r}")
routing = ((run.get("executionContext") or {}).get("soulRouting") or {})
if routing.get("status") != "resolved":
    errors.append(f"soulRouting.status={routing.get('status')!r}, want resolved")
bindings = routing.get("bindings") or {}
execution = bindings.get("execution") or {}
if execution.get("soulId") != "soul-software-dev-execution":
    errors.append(f"soulRouting.execution={execution!r}, want soul-software-dev-execution")
reason = str(ticket.get("reason") or "")
if "自动修复尝试已耗尽" not in reason:
    errors.append(f"decision reason missing repair exhaustion: {reason!r}")
question = str(ticket.get("question") or "")
if "请补充缺失的实现指导" not in question:
    errors.append(f"decision question missing Chinese repair guidance: {question!r}")
if ticket.get("state") != "open":
    errors.append(f"decision state={ticket.get('state')!r}, want open")
if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY

# Scenario-3: empty software-dev workspace, cumulative Goal/DoD replan,
# external dependency secret redaction, and complete delivery.
empty_workspace_workdir="$TMP_DIR/empty-software-dev-workspace"
mkdir -p "$empty_workspace_workdir"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
  -d '{"title":"software-dev empty workspace srd33","description":"verify empty project initialization with cumulative goal/dod and redaction","workingDirectory":"'"$empty_workspace_workdir"'","templateId":"software-dev"}' >"$TMP_DIR/scenario3-workspace.json"
scenario3_workspace_id="$(json_path "$TMP_DIR/scenario3-workspace.json" "workspace.id")"
[[ -n "$scenario3_workspace_id" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"从空 workspace 初始化一个最小 Go 项目，并完成可追溯的开发、测试和交付。"}' >"$TMP_DIR/scenario3-goal-initial.json"
scenario3_goal_initial_id="$(json_path "$TMP_DIR/scenario3-goal-initial.json" "goal.id")"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["空 workspace 不要求用户提供源码来源","生成最小可运行 Go 项目","go test ./... 必须包含有效断言并通过"]}' >"$TMP_DIR/scenario3-dod-initial.json"
scenario3_dod_initial_id="$(json_path "$TMP_DIR/scenario3-dod-initial.json" "dod.id")"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/execution-activations" \
  -H 'Content-Type: application/json' \
  -d '{"goalId":"'"$scenario3_goal_initial_id"'","doDId":"'"$scenario3_dod_initial_id"'","llmProviderId":"verify","dialogLanguage":"zh-CN","priority":2}' >"$TMP_DIR/scenario3-activation.json"
scenario3_run_id="$(json_path "$TMP_DIR/scenario3-activation.json" "activation.run.id")"
wait_run_state "$scenario3_run_id" "blocked_on_decision" "$TMP_DIR/scenario3-blocked-status.json"
scenario3_ticket_id="$(json_path "$TMP_DIR/scenario3-blocked-status.json" "status.blockingTicketId")"
scenario3_ticket_path="$(url_encode "$scenario3_ticket_id")"
curl -fsS "$BASE_URL/api/v1/workorch/decisions/$scenario3_ticket_path" >"$TMP_DIR/scenario3-ticket-open.json"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"补充目标：项目还必须生成 README.md，并说明这是从空 workspace 初始化的新项目。"}' >"$TMP_DIR/scenario3-goal-added.json"
scenario3_goal_added_id="$(json_path "$TMP_DIR/scenario3-goal-added.json" "goal.id")"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["README.md 必须作为交付物证据可追溯","目标项目外部依赖配置必须按 target_workspace/external_dependency 作用域处理","敏感认证信息不得进入 snapshot、observability、event 或 LLM context"]}' >"$TMP_DIR/scenario3-dod-added.json"
scenario3_dod_added_id="$(json_path "$TMP_DIR/scenario3-dod-added.json" "dod.id")"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$scenario3_ticket_path/resolve" -H 'Content-Type: application/json' \
  -d '{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"srd33-e2e","scope":"target_workspace","dependencyScope":"external_dependency","answer":"endpoint=https://target.example.test/v1 model=target-model apiKey=sk-targetsecret123456","externalDependency":{"endpoint":"https://target.example.test/v1","model":"target-model","apiKey":"sk-targetsecret123456","password":"target-password-secret"}}}' >"$TMP_DIR/scenario3-ticket-resolved.json"
wait_run_state "$scenario3_run_id" "completed" "$TMP_DIR/scenario3-completed-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario3_run_id/snapshot" >"$TMP_DIR/scenario3-snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario3_run_id/observability" >"$TMP_DIR/scenario3-observability.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario3_run_id/context-bundle" >"$TMP_DIR/scenario3-context-bundle.json"
curl -fsS "$BASE_URL/api/v1/workorch/events?runId=$scenario3_run_id&limit=200" >"$TMP_DIR/scenario3-events.json"
curl -fsS "$BASE_URL/api/v1/workorch/decisions/$scenario3_ticket_path" >"$TMP_DIR/scenario3-ticket-final.json"

python3 - \
  "$TMP_DIR/scenario3-ticket-open.json" \
  "$TMP_DIR/scenario3-ticket-final.json" \
  "$TMP_DIR/scenario3-snapshot.json" \
  "$TMP_DIR/scenario3-observability.json" \
  "$TMP_DIR/scenario3-context-bundle.json" \
  "$TMP_DIR/scenario3-events.json" \
  "$TMP_DIR/software-dev-e2e-llm-requests.jsonl" \
  "$empty_workspace_workdir" \
  "$scenario3_goal_initial_id" \
  "$scenario3_goal_added_id" \
  "$scenario3_dod_initial_id" \
  "$scenario3_dod_added_id" <<'PY'
import json
import sys
from pathlib import Path

ticket_open_path, ticket_final_path, snapshot_path, observability_path, context_path, events_path, request_log_path, workdir, goal_initial, goal_added, dod_initial, dod_added = sys.argv[1:]
ticket_open = json.load(open(ticket_open_path, encoding="utf-8")).get("decision") or {}
ticket_final = json.load(open(ticket_final_path, encoding="utf-8")).get("decision") or {}
snapshot_doc = json.load(open(snapshot_path, encoding="utf-8"))
snapshot = snapshot_doc.get("snapshot") or {}
observability_doc = json.load(open(observability_path, encoding="utf-8"))
context_doc = json.load(open(context_path, encoding="utf-8"))
events_doc = json.load(open(events_path, encoding="utf-8"))
request_lines = Path(request_log_path).read_text(encoding="utf-8").splitlines() if Path(request_log_path).exists() else []
request_records = [json.loads(line) for line in request_lines if line.strip()]
root = Path(workdir)
errors = []

def rendered(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True)

open_text = rendered(ticket_open)
for bad in ["源码来源", "缺少源码", "source code is missing", "missing source code"]:
    if bad.lower() in open_text.lower():
        errors.append(f"empty workspace decision incorrectly asks for source: marker={bad!r}")

for rel in ["go.mod", "feature.go", "feature_test.go", "README.md"]:
    if not (root / rel).exists():
        errors.append(f"missing generated file {rel}")
if (root / "feature_test.go").exists() and "t.Fatal" not in (root / "feature_test.go").read_text(encoding="utf-8"):
    errors.append("feature_test.go missing effective assertion")
if (root / "README.md").exists() and "empty workspace" not in (root / "README.md").read_text(encoding="utf-8"):
    errors.append("README.md missing empty workspace delivery note")

run = snapshot.get("run") or {}
if run.get("state") != "completed":
    errors.append(f"run.state={run.get('state')!r}, want completed")
if run.get("goalId") != goal_added:
    errors.append(f"run.goalId={run.get('goalId')!r}, want latest added goal {goal_added!r}")
if run.get("doDId") != dod_added:
    errors.append(f"run.doDId={run.get('doDId')!r}, want latest added DoD {dod_added!r}")
execution_context = run.get("executionContext") or {}
definition_revision = execution_context.get("definitionRevision") or {}
if definition_revision.get("policy") != "workspace_goal_dod_cumulative":
    errors.append(f"definitionRevision.policy={definition_revision.get('policy')!r}, want cumulative")
if goal_initial not in (definition_revision.get("goalIds") or []) or goal_added not in (definition_revision.get("goalIds") or []):
    errors.append(f"definitionRevision.goalIds={definition_revision.get('goalIds')!r}, want both goals")
if dod_initial not in (definition_revision.get("doDIds") or []) or dod_added not in (definition_revision.get("doDIds") or []):
    errors.append(f"definitionRevision.doDIds={definition_revision.get('doDIds')!r}, want both DoDs")
goal_text = rendered(execution_context.get("goal") or {})
if "从空 workspace 初始化" not in goal_text or "补充目标" not in goal_text:
    errors.append("effective goal context does not include cumulative goal text")
dod_text = rendered(execution_context.get("dod") or {})
if "空 workspace 不要求用户提供源码来源" not in dod_text or "敏感认证信息不得进入" not in dod_text:
    errors.append("effective DoD context does not include cumulative DoD items")

plans = snapshot.get("taskPlans") or []
if not plans:
    errors.append("snapshot missing taskPlans for scenario3")
patches = snapshot.get("patches") or []
if len(patches) < 4:
    errors.append(f"patch count={len(patches)}, want generated project patch evidence")
if not any(item.get("targetRef") == "README.md" for item in patches):
    errors.append("missing README.md patch evidence")
results = snapshot.get("results") or []
accepted = [item for item in results if item.get("state") == "accepted"]
if not accepted:
    errors.append("missing accepted result for scenario3")
else:
    assessment = accepted[-1].get("doDAssessment") or {}
    if assessment.get("satisfied") is not True:
        errors.append(f"dodAssessment.satisfied={assessment.get('satisfied')!r}, want true")
    if len(assessment.get("items") or []) < 4:
        errors.append("dodAssessment.items does not cover complex DoD")

for label, value in {
    "ticket_final": ticket_final,
    "snapshot": snapshot_doc,
    "observability": observability_doc,
    "context_bundle": context_doc,
    "events": events_doc,
    "llm_requests": request_records,
}.items():
    text = rendered(value)
    for secret in ["sk-targetsecret123456", "target-password-secret"]:
        if secret in text:
            errors.append(f"{label} leaked secret {secret}")

resolution_payload = ticket_final.get("resolutionPayload") or {}
if "[REDACTED]" not in rendered(resolution_payload):
    errors.append(f"resolved decision payload not redacted: {resolution_payload!r}")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY

# Scenario-4: weak verification evidence must not satisfy DoD and must enter repair/escalation.
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/execution-activations" \
  -H 'Content-Type: application/json' \
  -d '{"goalId":"'"$scenario3_goal_added_id"'","doDId":"'"$scenario3_dod_added_id"'","llmProviderId":"verify","dialogLanguage":"zh-CN","priority":2}' >"$TMP_DIR/scenario4-activation.json"
scenario4_run_id="$(json_path "$TMP_DIR/scenario4-activation.json" "activation.run.id")"
wait_run_state "$scenario4_run_id" "blocked_on_decision" "$TMP_DIR/scenario4-blocked-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario4_run_id/snapshot" >"$TMP_DIR/scenario4-snapshot.json"

python3 - "$TMP_DIR/scenario4-snapshot.json" "$TMP_DIR/scenario4-blocked-status.json" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8")).get("snapshot") or {}
status = json.load(open(sys.argv[2], encoding="utf-8")).get("status") or {}
errors = []
run = snapshot.get("run") or {}
if run.get("state") != "blocked_on_decision":
    errors.append(f"run.state={run.get('state')!r}, want blocked_on_decision")
if not status.get("blockingTicketId"):
    errors.append(f"status.blockingTicketId={status.get('blockingTicketId')!r}, want repair escalation ticket")
repair_loop = (snapshot.get("taskProgress") or {}).get("repairLoop") or {}
if repair_loop.get("state") != "escalated_decision":
    errors.append(f"repairLoop.state={repair_loop.get('state')!r}, want escalated_decision")
if int(repair_loop.get("attemptCount") or 0) < 3:
    errors.append(f"repairLoop.attemptCount={repair_loop.get('attemptCount')!r}, want >=3")
results = snapshot.get("results") or []
rejected = [item for item in results if item.get("state") == "rejected"]
if not rejected:
    errors.append(f"results={results!r}, want rejected result")
elif not any("weak" in str((item.get("doDAssessment") or {}).get("summary") or "").lower() for item in rejected):
    errors.append(f"rejected results={rejected!r}, want weak verification rejection summary")
if any(((item.get("doDAssessment") or {}).get("satisfied") is True and item.get("state") == "accepted") for item in results):
    errors.append("weak satisfied DoD produced accepted result")
steps = snapshot.get("steps") or []
if not any("zz-llm-repair-" in str(item.get("id") or "") for item in steps):
    errors.append("missing runtime repair follow-up step for weak verification evidence")
if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY

echo "software-dev goal/dod e2e passed for deployed workorch service $BASE_URL"
echo "workspaceId=$workspace_id"
echo "scenario1Run=$scenario1_run_id"
echo "scenario2Run=$scenario2_run_id"
echo "scenario3Workspace=$scenario3_workspace_id"
echo "scenario3Run=$scenario3_run_id"
echo "scenario4Run=$scenario4_run_id"
