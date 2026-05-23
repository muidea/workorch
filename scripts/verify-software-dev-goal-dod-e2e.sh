#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_SOFTWARE_DEV_E2E_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
MOCK_ADDR="${WORKORCH_SOFTWARE_DEV_E2E_MOCK_ADDR:-127.0.0.1:18193}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_SOFTWARE_DEV_E2E_SUFFIX:-$$}"
E2E_MARKER="software-dev-e2e-$VERIFY_SUFFIX"
TMP_DIR="${WORKORCH_SOFTWARE_DEV_E2E_ARTIFACT_DIR:-$(mktemp -d)}"
MOCK_PID=""
MOCK_LOG="$TMP_DIR/software-dev-e2e-llm-mock.log"
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
  if [[ "$LOCK_HELD" == "1" ]]; then
    rmdir "$LOCK_DIR" >/dev/null 2>&1 || true
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

json_path_or_empty() {
  python3 - "$1" "$2" <<'PY'
import json
import sys

try:
    with open(sys.argv[1], "r", encoding="utf-8") as handle:
        data = json.load(handle)
    current = data
    for part in sys.argv[2].split("."):
        current = current[part]
except Exception:
    print("")
else:
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

wait_run_state_any() {
  local run_id="$1"
  local output_file="$2"
  shift 2
  local deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/status" >"$output_file"
    if python3 - "$output_file" "$@" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
state = (data.get("status") or {}).get("state")
raise SystemExit(0 if state in set(sys.argv[2:]) else 1)
PY
    then
      return 0
    fi
    sleep 1
  done
  cat "$output_file" >&2
  return 1
}

list_workspace_run_ids() {
  local workspace_id="$1"
  local output_file="$TMP_DIR/${workspace_id}-runs.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" >"$output_file"
  python3 - "$output_file" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
runs = data.get("runs") or []
ids = []
for item in runs:
    run_id = str(item.get("id") or "").strip()
    if run_id:
        ids.append(run_id)
print(",".join(ids))
PY
}

recover_activation_run_id() {
  local workspace_id="$1"
  local known_run_ids_csv="$2"
  local output_file="$3"
  local deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" >"$output_file"; then
      local recovered
      recovered="$(python3 - "$output_file" "$known_run_ids_csv" <<'PY'
import json
import re
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
known = {item.strip() for item in sys.argv[2].split(",") if item.strip()}
runs = data.get("runs") or []
base_candidates = []
fallback_candidates = []
for item in runs:
    run_id = str(item.get("id") or "").strip()
    if not run_id or run_id in known:
        continue
    numeric_match = re.fullmatch(r"run-(\d+)", run_id)
    if numeric_match:
        base_candidates.append((int(numeric_match.group(1)), run_id))
        continue
    fallback_candidates.append(run_id)
if base_candidates:
    base_candidates.sort(reverse=True)
    print(base_candidates[0][1])
elif fallback_candidates:
    print(sorted(fallback_candidates)[-1])
PY
)"
      if [[ -n "$recovered" ]]; then
        printf '%s\n' "$recovered"
        return 0
      fi
    fi
    sleep 1
  done
  return 1
}

activate_workspace_execution() {
  local workspace_id="$1"
  local payload="$2"
  local output_file="$3"
  local known_run_ids_csv="$4"
  local recovery_file="$TMP_DIR/${workspace_id}-activation-recovery-runs.json"
  local run_id=""

  if ! curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
    -H 'Content-Type: application/json' \
    -d "$payload" >"$output_file"; then
    echo "execution activation response unavailable, attempting run recovery" >&2
  fi

  run_id="$(json_path_or_empty "$output_file" "activation.run.id")"
  if [[ -n "$run_id" ]]; then
    printf '%s\n' "$run_id"
    return 0
  fi

  echo "execution activation response missing valid run id, recovering from workspace runs" >&2
  recover_activation_run_id "$workspace_id" "$known_run_ids_csv" "$recovery_file"
}

verify_software_dev_profile_snapshot() {
  local workspace_id_for_run="$1"
  local run_id="$2"
  local snapshot_file="$3"
  local results_metadata_file="$TMP_DIR/${run_id}-profile-results-metadata.json"
  local results_file="$TMP_DIR/${run_id}-profile-results.json"
  curl -fsS "$BASE_URL/api/v1/workorch/runs/$run_id/results?limit=500" >"$results_metadata_file"
  python3 - "$results_metadata_file" "$results_file" "$snapshot_file" "$BASE_URL" "$workspace_id_for_run" "$run_id" <<'PY'
import json
import subprocess
import sys
from urllib.parse import quote

metadata_path, output_path, snapshot_path, base_url, workspace_id, run_id = sys.argv[1:]

def normalize_json_names(value):
    if isinstance(value, list):
        return [normalize_json_names(item) for item in value]
    if not isinstance(value, dict):
        return value
    normalized = {}
    for key, item in value.items():
        if isinstance(key, str) and key.isupper():
            normalized_key = key.lower()
        elif isinstance(key, str) and key[:1].isupper():
            normalized_key = key[:1].lower() + key[1:]
        else:
            normalized_key = key
        normalized[normalized_key] = normalize_json_names(item)
    return normalized

data = json.load(open(metadata_path, encoding="utf-8"))
results = data.get("results") or []
for item in results:
    result_id = str(item.get("id") or "")
    if not result_id:
        continue
    if item.get("doDAssessment"):
        continue
    ref = f"workorch://workspaces/{workspace_id}/runs/{run_id}/results/{result_id}/dod-assessment.json"
    url = base_url.rstrip("/") + "/api/v1/workorch/runtime/payloads?ref=" + quote(ref, safe="")
    try:
        raw = subprocess.check_output(["curl", "-fsS", url], text=True)
        assessment = json.loads(raw)
    except Exception:
        continue
    if isinstance(assessment, dict) and assessment:
        item["doDAssessment"] = normalize_json_names(assessment)
try:
    snapshot_doc = json.load(open(snapshot_path, encoding="utf-8"))
except Exception:
    snapshot_doc = {}
snapshot = snapshot_doc.get("snapshot") if isinstance(snapshot_doc, dict) else None
if isinstance(snapshot, dict):
    assessments_by_id = {
        str(item.get("id") or ""): item.get("doDAssessment")
        for item in results
        if isinstance(item.get("doDAssessment"), dict) and item.get("doDAssessment")
    }
    for item in snapshot.get("results") or []:
        result_id = str(item.get("id") or "")
        if result_id in assessments_by_id and not item.get("doDAssessment"):
            item["doDAssessment"] = assessments_by_id[result_id]
    json.dump(snapshot_doc, open(snapshot_path, "w", encoding="utf-8"), ensure_ascii=False, indent=2)
json.dump({"results": results}, open(output_path, "w", encoding="utf-8"), ensure_ascii=False, indent=2)
PY
  "$ROOT_DIR/scripts/verify-workorch-runtime-profile-software-dev.sh" --snapshot "$snapshot_file" "$results_file"
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

e2e_marker = os.environ.get("WORKORCH_SOFTWARE_DEV_E2E_MARKER", "")
run_scenarios = {}
scenario_counts = {}

BUGGY_SOURCE = "package calculator\n\nfunc Add(a, b int) int { return a - b }\n"
FIXED_SOURCE = "package calculator\n\nfunc Add(a, b int) int { return a + b }\n"
FIXED_DIFF = "--- a/calculator.go\n+++ b/calculator.go\n@@ -1,3 +1,3 @@\n package calculator\n \n-func Add(a, b int) int { return a - b }\n+func Add(a, b int) int { return a + b }\n"
NEW_PROJECT_SOURCE = "package targetproject\n\nfunc Enabled() bool { return true }\n"
NEW_PROJECT_TEST = "package targetproject\n\nimport \"testing\"\n\nfunc TestEnabled(t *testing.T) {\n\tif !Enabled() {\n\t\tt.Fatal(\"Enabled() = false, want true\")\n\t}\n}\n"
NEW_PROJECT_README = "# Target Project\n\nGenerated by software-dev e2e to verify empty workspace initialization.\n"

def parse_request(payload):
    messages = payload.get("messages") or []
    merged_content = "\n".join(str(item.get("content", "")) for item in messages)
    run_match = re.search(r'"runId"\s*:\s*"([^"]+)"', merged_content)
    step_match = re.search(r'"stepId"\s*:\s*"([^"]+)"', merged_content)
    run_id = run_match.group(1) if run_match else ""
    step_id = step_match.group(1) if step_match else ""
    return run_id, step_id, merged_content, messages

def scenario_for_run(run_id, merged_content):
    if run_id == "probe":
        return -2
    if run_id in run_scenarios:
        return run_scenarios[run_id]
    if not e2e_marker or e2e_marker not in merged_content:
        return -1
    if "-replan-" in run_id:
        if "从空 workspace 初始化" in merged_content or "项目还必须生成 README.md" in merged_content:
            scenario = 7
        elif "完成软件开发闭环" in merged_content:
            scenario = 6
        else:
            scenario = -1
    elif "graph follow-up 超出扩展上限" in merged_content:
        scenario = 5
    elif "no-actionable continuation" in merged_content or "没有可执行后续动作" in merged_content:
        scenario = 4
    elif "从空 workspace 初始化" in merged_content or "项目还必须生成 README.md" in merged_content:
        count = scenario_counts.get("empty_workspace", 0)
        scenario_counts["empty_workspace"] = count + 1
        scenario = 2 if count == 0 else 3 if count == 1 else 7
    elif "完成软件开发闭环" in merged_content:
        count = scenario_counts.get("main_repair", 0)
        scenario_counts["main_repair"] = count + 1
        scenario = 0 if count == 0 else 1 if count == 1 else 6
    else:
        scenario = -1
    run_scenarios[run_id] = scenario
    return scenario

def is_repair_step(step_id):
    return "zz-llm-repair-" in step_id or "/repair-" in step_id

def is_after_graph_step(step_id):
    return "zz-llm-after-graph-" in step_id or "/follow-up-graph-" in step_id

def is_no_actionable_followup_step(step_id):
    return "zz-llm-after-no-actionable-continuation-" in step_id or "follow-up-no-actionable" in step_id

def is_graph_limit_followup_step(step_id):
    return "zz-llm-after-graph-limit-" in step_id or "follow-up-graph-limit" in step_id or "graph-limit" in step_id

def response_for(run_id, step_id, merged_content):
    if run_id == "probe":
        return {"completionState": "failed", "thoughtSummary": "probe"}
    scenario = scenario_for_run(run_id, merged_content)
    if scenario < 0:
        return {
            "completionState": "failed",
            "thoughtSummary": "Ignoring a non-current e2e run during software-dev verification.",
            "dodAssessment": {
                "satisfied": False,
                "summary": "Request did not contain the current e2e marker.",
                "items": [
                    {"item": "current e2e marker is present", "state": "failed", "evidenceRefs": [step_id]}
                ]
            }
        }
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
        if is_after_graph_step(step_id):
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
                        {"item": "代码修改通过 Patch 证据可追溯", "state": "satisfied", "evidenceRefs": ["fix-source"]},
                        {"item": "go test ./... 最终通过", "state": "satisfied", "evidenceRefs": ["run-tests-fixed"]},
                        {"item": "结果包含 DoD 评估和交付物索引", "state": "satisfied", "evidenceRefs": [step_id]}
                    ],
                    "goalCoverage": {
                        "satisfied": True,
                        "items": [
                            {"goal": "完成软件开发闭环", "state": "satisfied", "evidenceRefs": ["fix-source", "run-tests-fixed"]}
                        ]
                    },
                    "acceptanceDimensions": {
                        "satisfied": True,
                        "items": [
                            {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": ["run-tests-fixed"]},
                            {"id": "runtime_entrypoint", "state": "satisfied", "evidenceRefs": ["run-tests-fixed"]}
                        ]
                    },
                    "deliverableCoverage": {
                        "satisfied": True,
                        "items": [
                            {"id": "source_change", "state": "satisfied", "artifactRefs": ["fix-source"]},
                            {"id": "validation_report", "state": "satisfied", "artifactRefs": ["run-tests-fixed"]},
                            {"id": "coverage_map", "state": "satisfied", "artifactRefs": [step_id]},
                            {"id": "user_deliverable", "state": "satisfied", "artifactRefs": ["fix-source"]}
                        ]
                    }
                }
            }
        if is_repair_step(step_id):
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
        if is_repair_step(step_id):
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
        if is_after_graph_step(step_id):
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
                        {"item": "空 workspace 不要求用户提供源码来源", "state": "satisfied", "evidenceRefs": ["write-go-mod", "write-source"]},
                        {"item": "生成最小可运行 Go 项目", "state": "satisfied", "evidenceRefs": ["write-go-mod", "write-source", "write-tests"]},
                        {"item": "go test ./... 必须包含有效断言并通过", "state": "satisfied", "evidenceRefs": ["run-empty-project-tests"]},
                        {"item": "README.md 必须作为交付物证据可追溯", "state": "satisfied", "evidenceRefs": ["write-readme"]},
                        {"item": "目标项目外部依赖配置必须按 target_workspace/external_dependency 作用域处理", "state": "satisfied", "evidenceRefs": [step_id]},
                        {"item": "敏感认证信息不得进入 snapshot、observability、event 或 LLM context", "state": "satisfied", "evidenceRefs": [step_id]}
                    ],
                    "goalCoverage": {
                        "satisfied": True,
                    "items": [
                        {"goal": f"[{e2e_marker}] 从空 workspace 初始化一个最小 Go 项目，并完成可追溯的开发、测试和交付。", "state": "satisfied", "evidenceRefs": ["write-go-mod", "write-source", "write-tests", "run-empty-project-tests"]},
                        {"goal": f"[{e2e_marker}] 补充目标：项目还必须生成 README.md，并说明这是从空 workspace 初始化的新项目。", "state": "satisfied", "evidenceRefs": ["write-readme"]}
                        ]
                    },
                    "acceptanceDimensions": {
                        "satisfied": True,
                        "items": [
                            {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": ["run-empty-project-tests"]}
                        ]
                    },
                    "deliverableCoverage": {
                        "satisfied": True,
                        "items": [
                            {"id": "source_change", "state": "satisfied", "artifactRefs": ["write-go-mod", "write-source", "write-tests", "write-readme"]},
                            {"id": "validation_report", "state": "satisfied", "artifactRefs": ["run-empty-project-tests"]},
                            {"id": "coverage_map", "state": "satisfied", "artifactRefs": [step_id]},
                            {"id": "user_deliverable", "state": "satisfied", "artifactRefs": ["write-source", "write-readme"]}
                        ]
                    }
                }
            }
        if "llm-after-decision" in step_id or "after-decision" in step_id:
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
                                "content": "module example.com/targetproject\n\ngo 1.22\n",
                                "coverageTargets": ["生成最小可运行 Go 项目"]
                            }
                        },
                        {
                            "id": "write-source",
                            "kind": "patch/apply",
                            "dependsOn": ["write-go-mod"],
                            "input": {
                                "patchType": "file_write",
                                "targetRef": "feature.go",
                                "content": NEW_PROJECT_SOURCE,
                                "coverageTargets": ["生成最小可运行 Go 项目", f"[{e2e_marker}] 从空 workspace 初始化一个最小 Go 项目，并完成可追溯的开发、测试和交付。"]
                            }
                        },
                        {
                            "id": "write-tests",
                            "kind": "patch/apply",
                            "dependsOn": ["write-source"],
                            "input": {
                                "patchType": "file_write",
                                "targetRef": "feature_test.go",
                                "content": NEW_PROJECT_TEST,
                                "coverageTargets": ["go test ./... 必须包含有效断言并通过"]
                            }
                        },
                        {
                            "id": "write-readme",
                            "kind": "patch/apply",
                            "dependsOn": ["write-tests"],
                            "input": {
                                "patchType": "file_write",
                                "targetRef": "README.md",
                                "content": NEW_PROJECT_README,
                                "coverageTargets": ["README.md 必须作为交付物证据可追溯", f"[{e2e_marker}] 补充目标：项目还必须生成 README.md，并说明这是从空 workspace 初始化的新项目。"]
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
        if is_repair_step(step_id):
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
    if scenario == 4:
        if step_id.startswith(run_id + "/step-1"):
            return {
                "completionState": "continue",
                "thoughtSummary": "Plan a bounded evidence collection graph.",
                "executionGraph": {
                    "nodes": [
                        {
                            "id": "collect-evidence",
                            "kind": "shell/command",
                            "input": {"command": "sh", "args": ["-c", "printf continuation-evidence"]}
                        }
                    ]
                }
            }
        if is_no_actionable_followup_step(step_id):
            return {
                "completionState": "completed",
                "thoughtSummary": "No-actionable continuation was converted into validation.",
                "dodAssessment": {
                    "satisfied": True,
                    "summary": "Graph exhausted validation completed with available evidence.",
                    "goalOutcome": "The run completed after graph exhaustion validation.",
                    "implementationSummary": "No source change was required for this continuation contract scenario.",
                    "verificationSummary": "The evidence collection command completed and the validation follow-up produced DoD evidence.",
                    "items": [
                        {"item": "no-actionable continuation 必须进入 validation follow-up 并完成 DoD 评估", "state": "satisfied", "evidenceRefs": ["collect-evidence", step_id]}
                    ],
                    "goalCoverage": {
                        "satisfied": True,
                        "items": [
                            {"goal": f"[{e2e_marker}] 验证当 LLM 返回 continue 但没有可执行后续动作时，系统会进入 validation follow-up 并完成 DoD 评估。", "state": "satisfied", "evidenceRefs": ["collect-evidence", step_id]}
                        ]
                    },
                    "acceptanceDimensions": {
                        "satisfied": True,
                        "items": [
                            {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": ["collect-evidence", step_id]}
                        ]
                    },
                    "deliverableCoverage": {
                        "satisfied": True,
                        "items": [
                            {"id": "source_change", "state": "satisfied", "artifactRefs": ["collect-evidence", step_id]},
                            {"id": "validation_report", "state": "satisfied", "artifactRefs": ["collect-evidence", step_id]},
                            {"id": "coverage_map", "state": "satisfied", "artifactRefs": [step_id]},
                            {"id": "user_deliverable", "state": "satisfied", "artifactRefs": ["collect-evidence", step_id]}
                        ]
                    }
                }
            }
        if is_after_graph_step(step_id):
            return {
                "completionState": "continue",
                "thoughtSummary": "Incorrectly requests continuation without a new actionable step."
            }
        return {"completionState": "continue", "thoughtSummary": "Continue execution."}
    if scenario == 5:
        if step_id.startswith(run_id + "/step-1"):
            return {
                "completionState": "continue",
                "thoughtSummary": "Plan graph expansion limit closure seed.",
                "executionGraph": {
                    "nodes": [
                        {
                            "id": "gfa-seed",
                            "kind": "shell/command",
                            "input": {"command": "sh", "args": ["-c", "printf gfa-seed"]}
                        }
                    ]
                }
            }
        if is_graph_limit_followup_step(step_id):
            return {
                "completionState": "completed",
                "thoughtSummary": "Graph expansion limit was converted into assessment-only closure.",
                "dodAssessment": {
                    "satisfied": True,
                    "summary": "Graph follow-up expansion limit entered assessment-only closure.",
                    "goalOutcome": "The run completed through assessment-only closure after bounded expansion.",
                    "implementationSummary": "No source change was required for this runtime closure contract scenario.",
                    "verificationSummary": "The graph limit closure path produced DoD evidence instead of another execution graph.",
                    "items": [
                        {"item": "graph follow-up 超限后必须进入 assessment-only 并完成 DoD 评估", "state": "satisfied", "evidenceRefs": [step_id]},
                        {"item": "Run Snapshot 必须暴露 graph expansion limit 与 assessment-only closure trace", "state": "satisfied", "evidenceRefs": [step_id]}
                    ],
                    "goalCoverage": {
                        "satisfied": True,
                        "items": [
                            {"goal": f"[{e2e_marker}] 验证 graph follow-up 超出扩展上限后，系统必须进入 assessment-only 收口并完成 DoD 评估。", "state": "satisfied", "evidenceRefs": [step_id]}
                        ]
                    },
                    "acceptanceDimensions": {
                        "satisfied": True,
                        "items": [
                            {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": [step_id]}
                        ]
                    },
                    "deliverableCoverage": {
                        "satisfied": True,
                        "items": [
                            {"id": "source_change", "state": "satisfied", "artifactRefs": [step_id]},
                            {"id": "validation_report", "state": "satisfied", "artifactRefs": [step_id]},
                            {"id": "coverage_map", "state": "satisfied", "artifactRefs": [step_id]},
                            {"id": "user_deliverable", "state": "satisfied", "artifactRefs": [step_id]}
                        ]
                    }
                }
            }
        if is_after_graph_step(step_id):
            suffix = re.sub(r"[^A-Za-z0-9]+", "-", step_id)[-48:].strip("-") or "next"
            return {
                "completionState": "continue",
                "thoughtSummary": "Requests another graph expansion until the runtime boundary converts it to assessment-only.",
                "executionGraph": {
                    "nodes": [
                        {
                            "id": "gfa-follow-up-" + suffix,
                            "kind": "shell/command",
                            "input": {"command": "sh", "args": ["-c", "printf gfa-follow-up"]}
                        }
                    ]
                }
            }
        return {"completionState": "continue", "thoughtSummary": "Continue execution."}
    if scenario == 6:
        if step_id.endswith("/step-1"):
            return {
                "completionState": "continue",
                "thoughtSummary": "Replan continuation applies a stable source write and reruns tests.",
                "executionGraph": {
                    "nodes": [
                        {
                            "id": "write-fixed-source-replan",
                            "kind": "patch/apply",
                            "input": {
                                "patchType": "file_write",
                                "targetRef": "calculator.go",
                                "content": FIXED_SOURCE
                            }
                        },
                        {
                            "id": "run-tests-replan",
                            "kind": "shell/command",
                            "dependsOn": ["write-fixed-source-replan"],
                            "input": {"command": "go", "args": ["test", "./..."]}
                        }
                    ]
                }
            }
        return {
            "completionState": "completed",
            "thoughtSummary": "Replan continuation completed after repair exhaustion.",
            "dodAssessment": {
                "satisfied": True,
                "summary": "Repair exhaustion was automatically replanned and closed by a continuation run.",
                "goalOutcome": "A continuation run was created after auto governance selected replan.",
                "implementationSummary": "The continuation run rewrote calculator.go through patch evidence.",
                "verificationSummary": "go test ./... passed in the continuation run.",
                "items": [
                    {"item": "代码修改通过 Patch 证据可追溯", "state": "satisfied", "evidenceRefs": ["write-fixed-source-replan"]},
                    {"item": "go test ./... 最终通过", "state": "satisfied", "evidenceRefs": ["run-tests-replan"]},
                    {"item": "结果包含 DoD 评估和交付物索引", "state": "satisfied", "evidenceRefs": [step_id]}
                ],
                "goalCoverage": {
                    "satisfied": True,
                    "items": [
                        {"goal": "完成软件开发闭环", "state": "satisfied", "evidenceRefs": ["write-fixed-source-replan", "run-tests-replan"]}
                    ]
                },
                "acceptanceDimensions": {
                    "satisfied": True,
                    "items": [
                        {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": ["run-tests-replan"]},
                        {"id": "runtime_entrypoint", "state": "satisfied", "evidenceRefs": ["run-tests-replan"]}
                    ]
                },
                "deliverableCoverage": {
                    "satisfied": True,
                    "items": [
                        {"id": "source_change", "state": "satisfied", "artifactRefs": ["write-fixed-source-replan"]},
                        {"id": "validation_report", "state": "satisfied", "artifactRefs": ["run-tests-replan"]},
                        {"id": "coverage_map", "state": "satisfied", "artifactRefs": [step_id]},
                        {"id": "user_deliverable", "state": "satisfied", "artifactRefs": ["write-fixed-source-replan"]}
                    ]
                }
            }
        }
    if scenario == 7:
        return {
            "completionState": "completed",
            "thoughtSummary": "Weak-evidence replan continuation closed with complete DoD assessment.",
            "dodAssessment": {
                "satisfied": True,
                "summary": "Continuation after weak-evidence repair exhaustion completed with current DoD coverage.",
                "goalOutcome": "The cumulative empty-workspace goals remain satisfied.",
                "implementationSummary": "Existing generated files and README remain in place.",
                "verificationSummary": "The continuation produced a formal DoD assessment after replan.",
                "items": [
                    {"item": "空 workspace 不要求用户提供源码来源", "state": "satisfied", "evidenceRefs": [step_id]},
                    {"item": "生成最小可运行 Go 项目", "state": "satisfied", "evidenceRefs": [step_id]},
                    {"item": "go test ./... 必须包含有效断言并通过", "state": "satisfied", "evidenceRefs": [step_id]},
                    {"item": "README.md 必须作为交付物证据可追溯", "state": "satisfied", "evidenceRefs": [step_id]},
                    {"item": "目标项目外部依赖配置必须按 target_workspace/external_dependency 作用域处理", "state": "satisfied", "evidenceRefs": [step_id]},
                    {"item": "敏感认证信息不得进入 snapshot、observability、event 或 LLM context", "state": "satisfied", "evidenceRefs": [step_id]}
                ],
                "goalCoverage": {
                    "satisfied": True,
                    "items": [
                        {"goal": f"[{e2e_marker}] 从空 workspace 初始化一个最小 Go 项目，并完成可追溯的开发、测试和交付。", "state": "satisfied", "evidenceRefs": [step_id]},
                        {"goal": f"[{e2e_marker}] 补充目标：项目还必须生成 README.md，并说明这是从空 workspace 初始化的新项目。", "state": "satisfied", "evidenceRefs": [step_id]}
                    ]
                },
                "acceptanceDimensions": {
                    "satisfied": True,
                    "items": [
                        {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": [step_id]}
                    ]
                },
                "deliverableCoverage": {
                    "satisfied": True,
                    "items": [
                        {"id": "source_change", "state": "satisfied", "artifactRefs": [step_id]},
                        {"id": "validation_report", "state": "satisfied", "artifactRefs": [step_id]},
                        {"id": "coverage_map", "state": "satisfied", "artifactRefs": [step_id]},
                        {"id": "user_deliverable", "state": "satisfied", "artifactRefs": [step_id]}
                    ]
                }
            }
        }
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
        run_id, step_id, merged_content, messages = parse_request(payload)
        request_log = os.environ.get("WORKORCH_SOFTWARE_DEV_E2E_REQUEST_LOG", "")
        if request_log:
            with open(request_log, "a", encoding="utf-8") as handle:
                handle.write(json.dumps({
                    "runId": run_id,
                    "stepId": step_id,
                    "messageCount": len(messages),
                    "roles": [str(item.get("role", "")) for item in messages],
                    "content": merged_content,
                    "messages": messages,
                }, ensure_ascii=False, separators=(",", ":")) + "\n")
        decision = response_for(run_id, step_id, merged_content)
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
    WORKORCH_SOFTWARE_DEV_E2E_MARKER="$E2E_MARKER" \
    WORKORCH_SOFTWARE_DEV_E2E_REQUEST_LOG="$TMP_DIR/software-dev-e2e-llm-requests.jsonl" \
    python3 "$TMP_DIR/software_dev_e2e_mock.py" >>"$MOCK_LOG" 2>&1 &
  MOCK_PID=$!
}

trap report_failure ERR
trap cleanup EXIT

cd "$ROOT_DIR"
acquire_llm_config_lock
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
  -d '{"title":"software-dev srd e2e","description":"verify goal/dod auto repair closure and repair exhaustion escalation","deliverableDirectory":"'"$workspace_workdir"'","templateId":"software-dev"}' >"$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"['"$E2E_MARKER"'] 完成软件开发闭环：通过 executionGraph 修改代码并运行测试，失败时自动修复，最终基于证据输出 DoD 评估。"}' >"$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
[[ -n "$goal_id" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["代码修改通过 Patch 证据可追溯","go test ./... 最终通过","结果包含 DoD 评估和交付物索引"]}' >"$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"
[[ -n "$dod_id" ]]

# Scenario-1: software-dev automatic repair and delivery closure.
scenario1_known_run_ids="$(list_workspace_run_ids "$workspace_id")"
scenario1_run_id="$(activate_workspace_execution \
  "$workspace_id" \
  '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"verify","priority":2}' \
  "$TMP_DIR/scenario1-activation.json" \
  "$scenario1_known_run_ids")"
wait_run_state "$scenario1_run_id" "completed" "$TMP_DIR/scenario1-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario1_run_id/snapshot" >"$TMP_DIR/scenario1-snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario1_run_id/observability" >"$TMP_DIR/scenario1-observability.json"
verify_software_dev_profile_snapshot "$workspace_id" "$scenario1_run_id" "$TMP_DIR/scenario1-snapshot.json"

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

# Scenario-2: repair loop exhausted, replan resolution resumes the run,
# then the server schedules a continuation run. Auto-governance may resolve
# the ticket directly; otherwise this e2e resolves it explicitly so the
# continuation contract is still verified under stricter configurations.
scenario2_known_run_ids="$(list_workspace_run_ids "$workspace_id")"
scenario2_run_id="$(activate_workspace_execution \
  "$workspace_id" \
  '{"goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"verify","dialogLanguage":"zh-CN","priority":2}' \
  "$TMP_DIR/scenario2-activation.json" \
  "$scenario2_known_run_ids")"
wait_run_state_any "$scenario2_run_id" "$TMP_DIR/scenario2-status.json" "superseded" "blocked_on_decision"
scenario2_state="$(json_path "$TMP_DIR/scenario2-status.json" "status.state")"
scenario2_resolution_mode="auto"
if [[ "$scenario2_state" == "blocked_on_decision" ]]; then
  scenario2_resolution_mode="manual"
  scenario2_ticket_id="$(json_path "$TMP_DIR/scenario2-status.json" "status.blockingTicketId")"
  scenario2_ticket_path="$(url_encode "$scenario2_ticket_id")"
  curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$scenario2_ticket_path/resolve" -H 'Content-Type: application/json' \
    -d '{"resolutionType":"replan","resolutionPayload":{"operator":"software-dev-e2e","reason":"repair exhausted replan continuation verification"}}' >"$TMP_DIR/scenario2-ticket-resolved.json"
  wait_run_state "$scenario2_run_id" "superseded" "$TMP_DIR/scenario2-status.json"
fi
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario2_run_id/snapshot" >"$TMP_DIR/scenario2-snapshot.json"
deadline=$((SECONDS + 60))
while (( SECONDS <= deadline )); do
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$TMP_DIR/scenario2-workspace-snapshot.json"
  if python3 - "$TMP_DIR/scenario2-workspace-snapshot.json" "$scenario1_run_id" "$scenario2_run_id" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8")).get("snapshot") or {}
excluded = {sys.argv[2], sys.argv[3]}
runs = snapshot.get("runs") or []
continuations = [item for item in runs if item.get("id") not in excluded and item.get("state") == "completed"]
raise SystemExit(0 if continuations else 1)
PY
  then
    break
  fi
  sleep 1
done

python3 - "$TMP_DIR/scenario2-snapshot.json" "$TMP_DIR/scenario2-workspace-snapshot.json" "$scenario1_run_id" "$scenario2_run_id" "$scenario2_resolution_mode" <<'PY'
import json
import sys

run_snapshot = json.load(open(sys.argv[1], encoding="utf-8"))["snapshot"]
workspace_snapshot = json.load(open(sys.argv[2], encoding="utf-8")).get("snapshot") or {}
scenario1_run_id = sys.argv[3]
scenario2_run_id = sys.argv[4]
resolution_mode = sys.argv[5]
errors = []
run = run_snapshot.get("run") or {}
if run.get("state") != "superseded":
    errors.append(f"run.state={run.get('state')!r}, want superseded")
repair_loop = (run_snapshot.get("taskProgress") or {}).get("repairLoop") or {}
if repair_loop.get("state") not in {"escalated_decision", "terminated"}:
    errors.append(f"repairLoop.state={repair_loop.get('state')!r}, want escalated_decision or terminated after auto replan")
if int(repair_loop.get("attemptCount") or 0) < 3:
    errors.append(f"repairLoop.attemptCount={repair_loop.get('attemptCount')!r}, want >=3")
routing = ((run.get("executionContext") or {}).get("soulRouting") or {})
if routing.get("status") != "resolved":
    errors.append(f"soulRouting.status={routing.get('status')!r}, want resolved")
bindings = routing.get("bindings") or {}
execution = bindings.get("execution") or {}
if execution.get("soulId") != "soul-software-dev-execution":
    errors.append(f"soulRouting.execution={execution!r}, want soul-software-dev-execution")
decisions = run_snapshot.get("decisions") or []
replan_decisions = [item for item in decisions if item.get("runId") == scenario2_run_id and item.get("resolutionType") == "replan"]
if not replan_decisions:
    errors.append(f"decisions={decisions!r}, want resolved replan decision for repair exhaustion")
ticket = replan_decisions[-1] if replan_decisions else {}
reason = str(ticket.get("reason") or "")
if "自动修复尝试已耗尽" not in reason:
    errors.append(f"decision reason missing repair exhaustion: {reason!r}")
question = str(ticket.get("question") or "")
if "请补充缺失的" not in question or "重新规划" not in question:
    errors.append(f"decision question missing Chinese repair guidance: {question!r}")
if ticket.get("state") != "resolved":
    errors.append(f"decision state={ticket.get('state')!r}, want resolved")
if resolution_mode == "auto" and not ticket.get("autoGovernance"):
    errors.append(f"decision autoGovernance={ticket.get('autoGovernance')!r}, want auto-governance trace")
runs = workspace_snapshot.get("runs") or []
continuations = [item for item in runs if item.get("id") not in {scenario1_run_id, scenario2_run_id} and item.get("state") == "completed"]
if not continuations:
    errors.append(f"workspace runs={runs!r}, want completed replan continuation")
results = workspace_snapshot.get("results") or []
continuation_id = continuations[-1].get("id") if continuations else ""
if not any(item.get("runId") == continuation_id and item.get("state") == "accepted" for item in results):
    errors.append(f"results={results!r}, want accepted result from continuation {continuations[-1].get('id') if continuations else ''}")
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
  -d '{"title":"software-dev empty workspace srd33","description":"verify empty project initialization with cumulative goal/dod and redaction","deliverableDirectory":"'"$empty_workspace_workdir"'","templateId":"software-dev"}' >"$TMP_DIR/scenario3-workspace.json"
scenario3_workspace_id="$(json_path "$TMP_DIR/scenario3-workspace.json" "workspace.id")"
[[ -n "$scenario3_workspace_id" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"['"$E2E_MARKER"'] 从空 workspace 初始化一个最小 Go 项目，并完成可追溯的开发、测试和交付。"}' >"$TMP_DIR/scenario3-goal-initial.json"
scenario3_goal_initial_id="$(json_path "$TMP_DIR/scenario3-goal-initial.json" "goal.id")"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["空 workspace 不要求用户提供源码来源","生成最小可运行 Go 项目","go test ./... 必须包含有效断言并通过"]}' >"$TMP_DIR/scenario3-dod-initial.json"
scenario3_dod_initial_id="$(json_path "$TMP_DIR/scenario3-dod-initial.json" "dod.id")"

scenario3_known_run_ids="$(list_workspace_run_ids "$scenario3_workspace_id")"
scenario3_run_id="$(activate_workspace_execution \
  "$scenario3_workspace_id" \
  '{"goalId":"'"$scenario3_goal_initial_id"'","doDId":"'"$scenario3_dod_initial_id"'","llmProviderId":"verify","dialogLanguage":"zh-CN","priority":2}' \
  "$TMP_DIR/scenario3-activation.json" \
  "$scenario3_known_run_ids")"
wait_run_state "$scenario3_run_id" "blocked_on_decision" "$TMP_DIR/scenario3-blocked-status.json"
scenario3_ticket_id="$(json_path "$TMP_DIR/scenario3-blocked-status.json" "status.blockingTicketId")"
scenario3_ticket_path="$(url_encode "$scenario3_ticket_id")"
curl -fsS "$BASE_URL/api/v1/workorch/decisions/$scenario3_ticket_path" >"$TMP_DIR/scenario3-ticket-open.json"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"['"$E2E_MARKER"'] 补充目标：项目还必须生成 README.md，并说明这是从空 workspace 初始化的新项目。"}' >"$TMP_DIR/scenario3-goal-added.json"
scenario3_goal_added_id="$(json_path "$TMP_DIR/scenario3-goal-added.json" "goal.id")"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["README.md 必须作为交付物证据可追溯","目标项目外部依赖配置必须按 target_workspace/external_dependency 作用域处理","敏感认证信息不得进入 snapshot、observability、event 或 LLM context"]}' >"$TMP_DIR/scenario3-dod-added.json"
scenario3_dod_added_id="$(json_path "$TMP_DIR/scenario3-dod-added.json" "dod.id")"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/decisions/$scenario3_ticket_path/resolve" -H 'Content-Type: application/json' \
  -d '{"resolutionType":"provide_missing_info","resolutionPayload":{"operator":"srd33-e2e","scope":"target_workspace","dependencyScope":"external_dependency","answer":"endpoint=https://target.example.test/v1 model=target-model apiKey=sk-targetsecret123456","externalDependency":{"endpoint":"https://target.example.test/v1","model":"target-model","apiKey":"sk-targetsecret123456","password":"target-password-secret"}}}' >"$TMP_DIR/scenario3-ticket-resolved.json"
wait_run_state_any "$scenario3_run_id" "$TMP_DIR/scenario3-completed-status.json" "completed" "superseded"
scenario3_effective_run_id="$scenario3_run_id"
if [[ "$(json_path "$TMP_DIR/scenario3-completed-status.json" "status.state")" == "superseded" ]]; then
  deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/snapshot" >"$TMP_DIR/scenario3-workspace-snapshot.json"
    scenario3_successor_run_id="$(python3 - "$TMP_DIR/scenario3-workspace-snapshot.json" "$scenario3_run_id" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8")).get("snapshot") or {}
original_run_id = sys.argv[2]
runs = snapshot.get("runs") or []
completed = [item for item in runs if item.get("id") != original_run_id and item.get("state") == "completed"]
print(completed[-1].get("id", "") if completed else "")
PY
)"
    if [[ -n "$scenario3_successor_run_id" ]]; then
      scenario3_effective_run_id="$scenario3_successor_run_id"
      break
    fi
    sleep 1
  done
  if [[ "$scenario3_effective_run_id" == "$scenario3_run_id" ]]; then
    cat "$TMP_DIR/scenario3-workspace-snapshot.json" >&2
    exit 1
  fi
fi
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario3_effective_run_id/snapshot" >"$TMP_DIR/scenario3-snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario3_effective_run_id/observability" >"$TMP_DIR/scenario3-observability.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario3_effective_run_id/context-bundle" >"$TMP_DIR/scenario3-context-bundle.json"
curl -fsS "$BASE_URL/api/v1/workorch/events?runId=$scenario3_effective_run_id&limit=200" >"$TMP_DIR/scenario3-events.json"
curl -fsS "$BASE_URL/api/v1/workorch/decisions/$scenario3_ticket_path" >"$TMP_DIR/scenario3-ticket-final.json"
verify_software_dev_profile_snapshot "$scenario3_workspace_id" "$scenario3_effective_run_id" "$TMP_DIR/scenario3-snapshot.json"

python3 - \
  "$TMP_DIR/scenario3-ticket-open.json" \
  "$TMP_DIR/scenario3-ticket-final.json" \
  "$TMP_DIR/scenario3-snapshot.json" \
  "$TMP_DIR/scenario3-observability.json" \
  "$TMP_DIR/scenario3-context-bundle.json" \
  "$TMP_DIR/scenario3-events.json" \
  "$TMP_DIR/software-dev-e2e-llm-requests.jsonl" \
  "$scenario3_effective_run_id" \
  "$empty_workspace_workdir" \
  "$scenario3_goal_initial_id" \
  "$scenario3_goal_added_id" \
  "$scenario3_dod_initial_id" \
  "$scenario3_dod_added_id" <<'PY'
import json
import sys
from pathlib import Path

ticket_open_path, ticket_final_path, snapshot_path, observability_path, context_path, events_path, request_log_path, scenario3_run_id, workdir, goal_initial, goal_added, dod_initial, dod_added = sys.argv[1:]
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

scenario_requests = [item for item in request_records if item.get("runId") == scenario3_run_id]
if not scenario_requests:
    errors.append("scenario3 llm requests missing")
else:
    if not any(int(item.get("messageCount") or 0) >= 3 for item in scenario_requests):
        errors.append(f"scenario3 messageCount invalid: {[item.get('messageCount') for item in scenario_requests]}")
    for item in scenario_requests:
        roles = item.get("roles") or []
        if "system" not in roles or "user" not in roles:
            errors.append(f"scenario3 request roles missing system/user: {roles!r}")
            break
    merged = "\n".join(str(item.get("content") or "") for item in scenario_requests)
    for marker in [
        "Original Goal Contract (Do not rewrite or compress)",
        "Original DoD Contract (Do not rewrite or compress)",
        "Effective Goal and DoD View",
    ]:
        if marker not in merged:
            errors.append(f"scenario3 prompt missing marker: {marker!r}")
    for text in [
        "从空 workspace 初始化一个最小 Go 项目",
        "补充目标：项目还必须生成 README.md",
        "空 workspace 不要求用户提供源码来源",
        "README.md 必须作为交付物证据可追溯",
    ]:
        if text not in merged:
            errors.append(f"scenario3 prompt missing cumulative goal/dod text: {text!r}")
    if "\"decisionResolution\"" not in merged:
        errors.append("scenario3 prompt missing decisionResolution after ticket resolve")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY

# Scenario-4: weak verification evidence must not satisfy DoD and must enter repair/escalation.
scenario4_known_run_ids="$(list_workspace_run_ids "$scenario3_workspace_id")"
scenario4_run_id="$(activate_workspace_execution \
  "$scenario3_workspace_id" \
  '{"goalId":"'"$scenario3_goal_added_id"'","doDId":"'"$scenario3_dod_added_id"'","llmProviderId":"verify","dialogLanguage":"zh-CN","priority":2}' \
  "$TMP_DIR/scenario4-activation.json" \
  "$scenario4_known_run_ids")"
wait_run_state_any "$scenario4_run_id" "$TMP_DIR/scenario4-blocked-status.json" "blocked_on_decision" "superseded"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario4_run_id/snapshot" >"$TMP_DIR/scenario4-snapshot.json"

python3 - "$TMP_DIR/scenario4-snapshot.json" "$TMP_DIR/scenario4-blocked-status.json" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8")).get("snapshot") or {}
status = json.load(open(sys.argv[2], encoding="utf-8")).get("status") or {}
errors = []
run = snapshot.get("run") or {}
if run.get("state") not in {"blocked_on_decision", "superseded"}:
    errors.append(f"run.state={run.get('state')!r}, want blocked_on_decision or superseded")
if run.get("state") == "blocked_on_decision" and not status.get("blockingTicketId"):
    errors.append(f"status.blockingTicketId={status.get('blockingTicketId')!r}, want repair escalation ticket")
repair_loop = (snapshot.get("taskProgress") or {}).get("repairLoop") or {}
if repair_loop.get("state") not in {"escalated_decision", "terminated"}:
    errors.append(f"repairLoop.state={repair_loop.get('state')!r}, want escalated_decision or terminated")
if int(repair_loop.get("attemptCount") or 0) < 3:
    errors.append(f"repairLoop.attemptCount={repair_loop.get('attemptCount')!r}, want >=3")
results = snapshot.get("results") or []
rejected = [item for item in results if item.get("state") == "rejected"]
if not rejected:
    errors.append(f"results={results!r}, want rejected result")
else:
    rejected_text = json.dumps(rejected, ensure_ascii=False).lower()
    if not ("weak" in rejected_text or "dod assessment rejected" in rejected_text or "completion gate" in rejected_text):
        errors.append(f"rejected results={rejected!r}, want weak verification or completion-gate rejection")
if any(((item.get("doDAssessment") or {}).get("satisfied") is True and item.get("state") == "accepted") for item in results):
    errors.append("weak satisfied DoD produced accepted result")
if run.get("state") == "superseded":
    decisions = snapshot.get("decisions") or []
    if not any(item.get("runId") == run.get("id") and item.get("state") == "resolved" and item.get("resolutionType") == "replan" for item in decisions):
        errors.append(f"decisions={decisions!r}, want resolved replan decision for superseded weak-evidence run")
steps = snapshot.get("steps") or []
if not any("zz-llm-repair-" in str(item.get("id") or "") or "/repair-" in str(item.get("id") or "") for item in steps):
    errors.append("missing runtime repair follow-up step for weak verification evidence")
if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY

# Scenario-5: continue without actionable continuation must enter validation follow-up instead of failing the run.
scenario5_workspace_workdir="$TMP_DIR/no-actionable-continuation-workspace"
mkdir -p "$scenario5_workspace_workdir"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
  -d '{"title":"software-dev no-actionable continuation","description":"verify graph exhausted validation follow-up","deliverableDirectory":"'"$scenario5_workspace_workdir"'","templateId":"software-dev"}' >"$TMP_DIR/scenario5-workspace.json"
scenario5_workspace_id="$(json_path "$TMP_DIR/scenario5-workspace.json" "workspace.id")"
[[ -n "$scenario5_workspace_id" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario5_workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"['"$E2E_MARKER"'] 验证当 LLM 返回 continue 但没有可执行后续动作时，系统会进入 validation follow-up 并完成 DoD 评估。"}' >"$TMP_DIR/scenario5-goal.json"
scenario5_goal_id="$(json_path "$TMP_DIR/scenario5-goal.json" "goal.id")"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario5_workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["no-actionable continuation 必须进入 validation follow-up 并完成 DoD 评估"]}' >"$TMP_DIR/scenario5-dod.json"
scenario5_dod_id="$(json_path "$TMP_DIR/scenario5-dod.json" "dod.id")"

scenario5_known_run_ids="$(list_workspace_run_ids "$scenario5_workspace_id")"
scenario5_run_id="$(activate_workspace_execution \
  "$scenario5_workspace_id" \
  '{"goalId":"'"$scenario5_goal_id"'","doDId":"'"$scenario5_dod_id"'","llmProviderId":"verify","dialogLanguage":"zh-CN","priority":2}' \
  "$TMP_DIR/scenario5-activation.json" \
  "$scenario5_known_run_ids")"
wait_run_state "$scenario5_run_id" "completed" "$TMP_DIR/scenario5-completed-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario5_run_id/snapshot" >"$TMP_DIR/scenario5-snapshot.json"

python3 - "$TMP_DIR/scenario5-snapshot.json" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8")).get("snapshot") or {}
errors = []
run = snapshot.get("run") or {}
if run.get("state") != "completed":
    errors.append(f"run.state={run.get('state')!r}, want completed")
failure_reason = str(run.get("failureReason") or "")
if "completionState continue has no actionable continuation" in failure_reason:
    errors.append(f"run failed with no-actionable continuation: {failure_reason}")
steps = snapshot.get("steps") or []
if not any("zz-llm-after-no-actionable-continuation-" in str(item.get("id") or "") or "follow-up-no-actionable" in str(item.get("id") or "") for item in steps):
    errors.append("missing no-actionable continuation validation follow-up step")
results = snapshot.get("results") or []
if not any(item.get("state") == "accepted" for item in results):
    errors.append(f"results={results!r}, want accepted result")
if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)
PY

# Scenario-6: graph follow-up expansion limit must enter assessment-only closure
# and expose closureTraces in the read model.
scenario6_workspace_workdir="$TMP_DIR/graph-follow-up-assessment-workspace"
mkdir -p "$scenario6_workspace_workdir"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" -H 'Content-Type: application/json' \
  -d '{"title":"software-dev graph follow-up closure","description":"verify graph follow-up expansion limit assessment-only closure","deliverableDirectory":"'"$scenario6_workspace_workdir"'","templateId":"software-dev"}' >"$TMP_DIR/scenario6-workspace.json"
scenario6_workspace_id="$(json_path "$TMP_DIR/scenario6-workspace.json" "workspace.id")"
[[ -n "$scenario6_workspace_id" ]]

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario6_workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"['"$E2E_MARKER"'] 验证 graph follow-up 超出扩展上限后，系统必须进入 assessment-only 收口并完成 DoD 评估。"}' >"$TMP_DIR/scenario6-goal.json"
scenario6_goal_id="$(json_path "$TMP_DIR/scenario6-goal.json" "goal.id")"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario6_workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["graph follow-up 超限后必须进入 assessment-only 并完成 DoD 评估","Run Snapshot 必须暴露 graph expansion limit 与 assessment-only closure trace"]}' >"$TMP_DIR/scenario6-dod.json"
scenario6_dod_id="$(json_path "$TMP_DIR/scenario6-dod.json" "dod.id")"

scenario6_known_run_ids="$(list_workspace_run_ids "$scenario6_workspace_id")"
scenario6_run_id="$(activate_workspace_execution \
  "$scenario6_workspace_id" \
  '{"goalId":"'"$scenario6_goal_id"'","doDId":"'"$scenario6_dod_id"'","llmProviderId":"verify","dialogLanguage":"zh-CN","priority":2}' \
  "$TMP_DIR/scenario6-activation.json" \
  "$scenario6_known_run_ids")"
wait_run_state "$scenario6_run_id" "completed" "$TMP_DIR/scenario6-completed-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario6_run_id/snapshot" >"$TMP_DIR/scenario6-snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/runs/$scenario6_run_id/observability" >"$TMP_DIR/scenario6-observability.json"

python3 - "$TMP_DIR/scenario6-snapshot.json" "$TMP_DIR/scenario6-observability.json" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8")).get("snapshot") or {}
observability = json.load(open(sys.argv[2], encoding="utf-8")).get("observability") or {}
errors = []
run = snapshot.get("run") or {}
if run.get("state") != "completed":
    errors.append(f"run.state={run.get('state')!r}, want completed")
steps = snapshot.get("steps") or []
if not any("zz-llm-after-graph-limit-" in str(item.get("id") or "") or "graph-limit" in str(item.get("id") or "") for item in steps):
    errors.append("missing graph expansion limit assessment-only follow-up step")
if any("must_not_execute" in str(item.get("id") or "") for item in steps):
    errors.append("assessment-only executionGraph was executed unexpectedly")
results = snapshot.get("results") or []
if not any(item.get("state") == "accepted" for item in results):
    errors.append(f"results={results!r}, want accepted result")
traces = snapshot.get("closureTraces") or []
if not any(item.get("graphFollowUpExpansionLimitExceeded") and item.get("ignoredExecutionGraph") for item in traces):
    errors.append(f"closureTraces={traces!r}, want graph expansion limit trace")
if not any(item.get("assessmentOnly") and item.get("graphExpansionLimitFollowUp") for item in traces):
    errors.append(f"closureTraces={traces!r}, want assessment-only graph limit trace")
obs_traces = observability.get("closureTraces") or []
if len(obs_traces) != len(traces):
    errors.append(f"observability.closureTraces={obs_traces!r}, want same count as snapshot {len(traces)}")
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
echo "scenario5Run=$scenario5_run_id"
echo "scenario6Run=$scenario6_run_id"
