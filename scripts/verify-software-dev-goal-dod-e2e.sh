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
  local workspace_id="$1"
  local run_id="$2"
  local expected_state="$3"
  local output_file="$4"
  local deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$output_file"
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
  local workspace_id="$1"
  local run_id="$2"
  local output_file="$3"
  shift 3
  local deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$output_file"
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
  local results_metadata_file="$TMP_DIR/${workspace_id_for_run}-${run_id}-profile-results-metadata.json"
  local results_file="$TMP_DIR/${workspace_id_for_run}-${run_id}-profile-results.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id_for_run/runs/$run_id/results?limit=500" >"$results_metadata_file"
  python3 - "$results_metadata_file" "$results_file" "$snapshot_file" "$TMP_DIR/configuration.json" "$BASE_URL" "$workspace_id_for_run" "$run_id" <<'PY'
import json
import os
import subprocess
import sys
from urllib.parse import quote

metadata_path, output_path, snapshot_path, config_path, base_url, workspace_id, run_id = sys.argv[1:]

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

data = normalize_json_names(json.load(open(metadata_path, encoding="utf-8")))
results = data.get("results") or []
try:
    config_doc = normalize_json_names(json.load(open(config_path, encoding="utf-8")))
except Exception:
    config_doc = {}
data_root = str(((config_doc.get("configuration") or {}).get("repository") or {}).get("dataDir") or "").strip()
try:
    snapshot_doc = normalize_json_names(json.load(open(snapshot_path, encoding="utf-8")))
except Exception:
    snapshot_doc = {}
snapshot = snapshot_doc.get("snapshot") if isinstance(snapshot_doc, dict) else None
snapshot_assessments_by_id = {}
if isinstance(snapshot, dict):
    snapshot_assessments_by_id = {
        str(item.get("id") or ""): item.get("doDAssessment")
        for item in snapshot.get("results") or []
        if isinstance(item, dict) and isinstance(item.get("doDAssessment"), dict) and item.get("doDAssessment")
    }
for item in results:
    result_id = str(item.get("id") or "")
    if not result_id:
        continue
    if item.get("doDAssessment"):
        continue
    if isinstance(snapshot_assessments_by_id.get(result_id), dict):
        item["doDAssessment"] = snapshot_assessments_by_id[result_id]
        continue
    if data_root:
        local_path = os.path.join(data_root, "internal", "workspaces", workspace_id, "runs", run_id, "results", *result_id.split("/")) + ".json"
        try:
            payload = normalize_json_names(json.load(open(local_path, encoding="utf-8")))
        except Exception:
            payload = {}
        if isinstance(payload, dict) and isinstance(payload.get("doDAssessment"), dict) and payload.get("doDAssessment"):
            item["doDAssessment"] = payload["doDAssessment"]
            continue
    ref = f"workorch://workspaces/{workspace_id}/runs/{run_id}/results/{result_id}.json"
    url = base_url.rstrip("/") + "/api/v1/workorch/runtime/payloads?ref=" + quote(ref, safe="")
    try:
        raw = subprocess.check_output(["curl", "-fsS", url], text=True)
        payload = normalize_json_names(json.loads(raw))
    except Exception:
        continue
    if isinstance(payload, dict) and isinstance(payload.get("doDAssessment"), dict) and payload.get("doDAssessment"):
        item["doDAssessment"] = payload["doDAssessment"]
    elif isinstance(payload, dict) and payload:
        item["doDAssessment"] = payload
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
        "model": "gpt-codex-1m",
        "timeout": "60s",
        "disabled": False,
        "chatCompletions": True,
        "streaming": False,
        "nonChatOperations": ["embeddings"],
        "contextProfile": {
            "profileMode": "manual",
            "contextWindowTokens": 1000000,
            "maxInputTokens": 700000,
            "reservedOutputTokens": 62500,
            "safetyRatio": 0.8,
            "contextUtilizationRatio": 0.75,
            "bytesHardLimit": 2800000,
            "maxRequestBytes": 4194304,
            "estimatedTokensPerByte": 0.25,
        },
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
    "model": "gpt-codex-1m",
    "timeout": "60s",
    "disabled": False,
    "chatCompletions": True,
    "streaming": False,
    "nonChatOperations": ["embeddings"],
    "contextProfile": {
        "profileMode": "manual",
        "contextWindowTokens": 1000000,
        "maxInputTokens": 700000,
        "reservedOutputTokens": 62500,
        "safetyRatio": 0.8,
        "contextUtilizationRatio": 0.75,
        "bytesHardLimit": 2800000,
        "maxRequestBytes": 4194304,
        "estimatedTokensPerByte": 0.25,
    },
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
REPLAN_TEST = "package calculator\n\nimport \"testing\"\n\nfunc TestAddReplanClosure(t *testing.T) {\n\tif got := Add(4, 6); got != 10 {\n\t\tt.Fatalf(\"Add(4,6)=%d, want 10\", got)\n\t}\n}\n"
NEW_PROJECT_SOURCE = "package targetproject\n\nfunc Enabled() bool { return true }\n"
NEW_PROJECT_TEST = "package targetproject\n\nimport \"testing\"\n\nfunc TestEnabled(t *testing.T) {\n\tif !Enabled() {\n\t\tt.Fatal(\"Enabled() = false, want true\")\n\t}\n}\n"
NEW_PROJECT_README = "# Target Project\n\nGenerated by software-dev e2e to verify empty workspace initialization.\n"
CUMULATIVE_PROJECT_SOURCE = "package targetproject\n\nfunc CumulativeVerified() bool { return Enabled() }\n"
CUMULATIVE_PROJECT_TEST = "package targetproject\n\nimport \"testing\"\n\nfunc TestCumulativeVerified(t *testing.T) {\n\tif !CumulativeVerified() {\n\t\tt.Fatal(\"CumulativeVerified() = false, want true\")\n\t}\n}\n"
CUMULATIVE_PROJECT_README = NEW_PROJECT_README + "\nCumulative verification confirms the README deliverable remains traceable.\n"

def parse_request(payload):
    messages = payload.get("messages") or []
    merged_content = "\n".join(str(item.get("content", "")) for item in messages)
    run_match = re.search(r'"runId"\s*:\s*"([^"]+)"', merged_content)
    step_match = re.search(r'"stepId"\s*:\s*"([^"]+)"', merged_content)
    run_id = run_match.group(1) if run_match else ""
    step_id = step_match.group(1) if step_match else ""
    return run_id, step_id, merged_content, messages

def scenario_cache_key(run_id, merged_content):
    workspace_match = re.search(r'"workspaceId"\s*:\s*"([^"]+)"', str(merged_content or ""))
    workspace_id = workspace_match.group(1) if workspace_match else ""
    return f"{workspace_id}:{run_id}" if workspace_id else run_id

def scenario_for_run(run_id, merged_content):
    if run_id == "probe":
        return -2
    cache_key = scenario_cache_key(run_id, merged_content)
    if cache_key in run_scenarios:
        return run_scenarios[cache_key]
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
        scenario = 2 if count == 0 else 7 if count == 1 else 3
    elif "完成软件开发闭环" in merged_content:
        count = scenario_counts.get("main_repair", 0)
        scenario_counts["main_repair"] = count + 1
        scenario = 0 if count == 0 else 1 if count == 1 else 6
    else:
        scenario = -1
    run_scenarios[cache_key] = scenario
    return scenario

def is_repair_step(step_id):
    return "zz-llm-repair-" in step_id or "/repair-" in step_id

def is_after_graph_step(step_id):
    return "zz-llm-after-graph-" in step_id or "/follow-up-graph-" in step_id

def is_no_actionable_followup_step(step_id):
    return "zz-llm-after-no-actionable-continuation-" in step_id or "follow-up-no-actionable" in step_id

def is_graph_limit_followup_step(step_id):
    return "zz-llm-after-graph-limit-" in step_id or "follow-up-graph-limit" in step_id or "graph-limit" in step_id

def capability_policy_skip():
    return [{
        "policyId": "software-dev-code-semantics-guidance",
        "category": "code_semantics",
        "reason": "deterministic e2e fixture uses direct patch and validation evidence; semantic capability is intentionally skipped",
        "fallbackPolicy": "fallback_to_file_read_and_build_output_without_blocking"
    }]

CALCULATOR_GIT_PATHS = ["go.mod", "calculator.go", "calculator_test.go"]
REPLAN_GIT_PATHS = ["go.mod", "calculator.go", "calculator_test.go", "calculator_replan_test.go"]
NEW_PROJECT_GIT_PATHS = ["go.mod", "feature.go", "feature_test.go", "README.md"]
CUMULATIVE_PROJECT_GIT_PATHS = NEW_PROJECT_GIT_PATHS + ["cumulative_feature.go", "cumulative_feature_test.go"]
RUNTIME_CLOSURE_GIT_PATHS = ["runtime-closure-evidence.txt"]

def git_snapshot_nodes(depends_on, pathspecs=None, message="record software-dev e2e workspace snapshot"):
    if isinstance(depends_on, str):
        depends = [depends_on]
    else:
        depends = list(depends_on or [])
    if pathspecs is None:
        pathspecs = CALCULATOR_GIT_PATHS
    return [
        {
            "id": "git-init",
            "kind": "git/write",
            "dependsOn": depends,
            "input": {"operation": "init"}
        },
        {
            "id": "git-add",
            "kind": "git/write",
            "dependsOn": ["git-init"],
            "input": {"operation": "add", "pathspecs": pathspecs}
        },
        {
            "id": "git-commit",
            "kind": "git/write",
            "dependsOn": ["git-add"],
            "input": {
                "operation": "commit",
                "message": message,
                "coverageTargets": ["version_control_snapshot"],
                "coverageTargetToken": "version_control_snapshot"
            }
        }
    ]

def git_snapshot_deliverable(evidence_ref="git-commit"):
    return {
        "id": "version_control_snapshot",
        "state": "satisfied",
        "evidenceRefs": [evidence_ref],
        "artifactRefs": [evidence_ref]
    }

def no_actionable_state_change_graph_response():
    return {
        "completionState": "continue",
        "thoughtSummary": "Produce the missing runtime closure deliverable evidence and version-control snapshot.",
        "capabilityPolicySkips": capability_policy_skip(),
        "executionGraph": {
            "nodes": [
                {
                    "id": "write-runtime-closure-evidence",
                    "kind": "patch/apply",
                    "input": {
                        "patchType": "file_write",
                        "targetRef": "runtime-closure-evidence.txt",
                        "content": "no-actionable continuation closure evidence\n",
                        "coverageTargets": [
                            "no-actionable continuation 必须进入 validation follow-up 并完成 DoD 评估",
                            "source_change",
                            "user_deliverable"
                        ]
                    }
                },
                {
                    "id": "validate-runtime-closure",
                    "kind": "shell/command",
                    "dependsOn": ["write-runtime-closure-evidence"],
                    "input": {
                        "command": "sh",
                        "args": ["-c", "test -s runtime-closure-evidence.txt"],
                        "coverageTargets": [
                            "no-actionable continuation 必须进入 validation follow-up 并完成 DoD 评估",
                            "validation_report"
                        ]
                    }
                },
                *git_snapshot_nodes("validate-runtime-closure", pathspecs=RUNTIME_CLOSURE_GIT_PATHS)
            ]
        }
    }

def no_actionable_concrete_evidence_response():
    return {
        "completionState": "continue",
        "thoughtSummary": "Collect bounded runtime closure evidence before state-changing repair.",
        "executionGraph": {
            "nodes": [
                {
                    "id": "read-runtime-closure-evidence",
                    "kind": "file/read",
                    "input": {
                        "path": "runtime-closure-evidence.txt",
                        "targetRef": "runtime-closure-evidence.txt",
                        "optional": True,
                        "boundedReason": "load current runtime closure evidence for no-actionable continuation repair"
                    }
                },
                {
                    "id": "validate-runtime-closure-evidence",
                    "kind": "shell/command",
                    "dependsOn": ["read-runtime-closure-evidence"],
                    "input": {
                        "command": "sh",
                        "args": ["-c", "if test -s runtime-closure-evidence.txt; then printf runtime-closure-evidence-present; else printf runtime-closure-evidence-missing; fi"],
                        "targetRef": "runtime-closure-evidence.txt",
                        "verificationScope": "runtime closure evidence",
                        "boundedReason": "verify concrete target evidence without mutating the workspace"
                    }
                }
            ]
        }
    }

def graph_limit_concrete_evidence_response():
    return {
        "completionState": "continue",
        "thoughtSummary": "Collect bounded graph-limit closure evidence before state-changing repair.",
        "executionGraph": {
            "nodes": [
                {
                    "id": "read-gfa-closure-evidence",
                    "kind": "file/read",
                    "input": {
                        "path": "runtime-closure-evidence.txt",
                        "targetRef": "runtime-closure-evidence.txt",
                        "optional": True,
                        "boundedReason": "load current graph-limit closure evidence"
                    }
                },
                {
                    "id": "validate-gfa-closure-evidence",
                    "kind": "shell/command",
                    "dependsOn": ["read-gfa-closure-evidence"],
                    "input": {
                        "command": "sh",
                        "args": ["-c", "if test -s runtime-closure-evidence.txt; then printf gfa-closure-evidence-present; else printf gfa-closure-evidence-missing; fi"],
                        "targetRef": "runtime-closure-evidence.txt",
                        "verificationScope": "graph follow-up closure evidence",
                        "boundedReason": "verify graph-limit evidence target without mutating the workspace"
                    }
                }
            ]
        }
    }

def graph_limit_seed_graph_response():
    return {
        "completionState": "continue",
        "thoughtSummary": "Produce graph expansion limit closure seed and version-control snapshot.",
        "capabilityPolicySkips": capability_policy_skip(),
        "executionGraph": {
            "nodes": [
                {
                    "id": "gfa-seed",
                    "kind": "patch/apply",
                    "input": {
                        "patchType": "file_write",
                        "targetRef": "runtime-closure-evidence.txt",
                        "content": "gfa-seed\n",
                        "coverageTargets": [
                            "graph follow-up 超限后必须进入 assessment-only 并完成 DoD 评估",
                            "Run Snapshot 必须暴露 graph expansion limit 与 assessment-only closure trace",
                            "source_change",
                            "user_deliverable"
                        ]
                    }
                },
                {
                    "id": "validate-gfa-seed",
                    "kind": "shell/command",
                    "dependsOn": ["gfa-seed"],
                    "input": {
                        "command": "sh",
                        "args": ["-c", "test -s runtime-closure-evidence.txt"],
                        "validationEvidence": {
                            "mode": "runtime_behavior",
                            "observationType": "runtime_observation"
                        },
                        "coverageTargets": [
                            "graph follow-up 超限后必须进入 assessment-only 并完成 DoD 评估",
                            "validation_report"
                        ]
                    }
                },
                *git_snapshot_nodes("validate-gfa-seed", pathspecs=RUNTIME_CLOSURE_GIT_PATHS)
            ]
        }
    }

def has_completed_no_actionable_state_change_graph(run_id, merged_content):
    text = str(merged_content or "")
    return (
        (
            re.search(re.escape(run_id) + r'/graph-\d+/001-write-runtime-closure-evidence', text) is not None
            or "write-runtime-closure-evidence" in text
        )
        and (
            re.search(re.escape(run_id) + r'/graph-\d+/002-validate-runtime-closure', text) is not None
            or "validate-runtime-closure" in text
        )
        and (
            re.search(re.escape(run_id) + r'/graph-\d+/005-git-commit', text) is not None
            or "git-commit" in text
        )
    )

def latest_no_actionable_state_change_graph_id(run_id, merged_content):
    text = str(merged_content or "")
    graph_numbers = []
    for match in re.finditer(re.escape(run_id) + r'/graph-(\d+)/005-git-commit', text):
        try:
            graph_numbers.append(int(match.group(1)))
        except ValueError:
            pass
    if not graph_numbers:
        return "graph-001"
    return f"graph-{max(graph_numbers):03d}"

def has_completed_graph_limit_seed_graph(run_id, merged_content):
    text = str(merged_content or "")
    return (
        (
            re.search(re.escape(run_id) + r'/graph-\d+/001-gfa-seed', text) is not None
            or "gfa-seed" in text
        )
        and (
            re.search(re.escape(run_id) + r'/graph-\d+/002-validate-gfa-seed', text) is not None
            or "validate-gfa-seed" in text
        )
        and (
            re.search(re.escape(run_id) + r'/graph-\d+/005-git-commit', text) is not None
            or "git-commit" in text
        )
    )

def latest_graph_limit_seed_graph_id(run_id, merged_content):
    text = str(merged_content or "")
    graph_numbers = []
    for match in re.finditer(re.escape(run_id) + r'/graph-(\d+)/005-git-commit', text):
        try:
            graph_numbers.append(int(match.group(1)))
        except ValueError:
            pass
    if not graph_numbers:
        return "graph-001"
    return f"graph-{max(graph_numbers):03d}"

def graph_limit_completed_assessment(run_id, step_id, merged_content):
    graph = latest_graph_limit_seed_graph_id(run_id, merged_content)
    return {
        "completionState": "completed",
        "thoughtSummary": "Graph expansion limit was converted into assessment-only closure.",
        "dodAssessment": {
            "satisfied": True,
            "summary": "Graph follow-up expansion limit entered assessment-only closure.",
            "goalOutcome": "The run completed through assessment-only closure after bounded expansion.",
            "implementationSummary": "Wrote runtime-closure-evidence.txt as the graph limit closure seed.",
            "verificationSummary": "The graph limit closure path produced DoD evidence instead of another execution graph.",
            "items": [
                {"item": "graph follow-up 超限后必须进入 assessment-only 并完成 DoD 评估", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-gfa-seed", step_id]},
                {"item": "Run Snapshot 必须暴露 graph expansion limit 与 assessment-only closure trace", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-gfa-seed", step_id]}
            ],
            "goalCoverage": {
                "satisfied": True,
                "items": [
                    {"goal": f"[{e2e_marker}] 验证 graph follow-up 超出扩展上限后，系统必须进入 assessment-only 收口并完成 DoD 评估。", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-gfa-seed", step_id]}
                ]
            },
            "acceptanceDimensions": {
                "satisfied": True,
                "items": [
                    {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-gfa-seed", step_id]}
                ]
            },
            "deliverableCoverage": {
                "satisfied": True,
                "items": [
                    {"id": "source_change", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-gfa-seed"], "artifactRefs": [f"{run_id}/{graph}/001-gfa-seed", step_id]},
                    {"id": "validation_report", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/002-validate-gfa-seed", step_id], "artifactRefs": [f"{run_id}/{graph}/002-validate-gfa-seed", step_id]},
                    {"id": "coverage_map", "state": "satisfied", "evidenceRefs": [step_id], "artifactRefs": [step_id]},
                    git_snapshot_deliverable(f"{run_id}/{graph}/005-git-commit"),
                    {"id": "user_deliverable", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-gfa-seed"], "artifactRefs": [f"{run_id}/{graph}/001-gfa-seed", step_id]}
                ]
            }
        }
    }

def needs_version_control_snapshot(merged_content):
    text = str(merged_content or "")
    markers = [
        "deliverableCoverage missing required deliverables: version_control_snapshot",
        "missing required deliverables: version_control_snapshot",
        "requiredOutput\":{\"acceptedKinds\":[\"git/write\"]",
        "acceptedOperations\":[\"commit\"]",
    ]
    return any(marker in text for marker in markers)

def needs_concrete_failure_evidence(merged_content):
    text = str(merged_content or "")
    return (
        '"mustProduceConcreteFailureEvidence":true' in text
        or '"concreteFailureEvidenceRequired":true' in text
        or '"repairObjective":"acquire_concrete_failure_evidence"' in text
    )

def needs_state_changing_repair(merged_content):
    text = str(merged_content or "")
    return (
        '"stateChangingOutputRequired":true' in text
        or '"repairObjective":"state_changing_output"' in text
        or '"mustIncludeStateChangingNode":true' in text
    )

def needs_concrete_validation_evidence(merged_content):
    text = str(merged_content or "")
    return (
        '"repairObjective":"produce_concrete_validation_evidence"' in text
        or "concrete validation evidence repair must return" in text
        or "mustDeclareValidationEvidenceMode" in text
        or "mustDeclareValidationEvidenceObservation" in text
    )

def has_completed_repair_graph_evidence(merged_content):
    text = str(merged_content or "")
    return "fix-source" in text and "run-tests-fixed" in text and "git-commit" in text

def has_completed_replan_graph_evidence(merged_content):
    text = str(merged_content or "")
    return "write-replan-regression-test" in text and "run-tests-replan" in text and "git-commit" in text

def needs_completed_repair_assessment(step_id, merged_content):
    text = str(merged_content or "")
    return (
        "graph-002/005-git-commit" in text
        and (
            "concrete validation evidence repair must return" in text
            or '"currentRuntimeStage":"verification"' in text
            or '"runtimeStage":"verification"' in text
            or "provider-retry" in step_id
        )
    )

def concrete_failure_evidence_response():
    return {
        "completionState": "continue",
        "thoughtSummary": "Acquire bounded concrete failure evidence before producing a state-changing repair.",
        "executionGraph": {
            "nodes": [
                {
                    "id": "read-current-source",
                    "kind": "file/read",
                    "input": {
                        "path": "calculator.go",
                        "targetRef": "calculator.go",
                        "boundedReason": "load current calculator implementation before repair"
                    }
                },
                {
                    "id": "collect-failing-test",
                    "kind": "shell/command",
                    "dependsOn": ["read-current-source"],
                    "input": {
                        "command": "bash",
                        "args": ["-lc", "go test ./... 2>&1 || true"],
                        "targetRef": "calculator.go",
                        "verificationScope": "go test ./...",
                        "boundedReason": "capture current failing test output without mutating workspace"
                    }
                }
            ]
        }
    }

def git_snapshot_repair_response(merged_content=""):
    text = str(merged_content or "")
    if "cumulative_feature.go" in text or "write-cumulative-source" in text:
        pathspecs = CUMULATIVE_PROJECT_GIT_PATHS
    elif "feature.go" in text or "write-source" in text or "README.md" in text:
        pathspecs = NEW_PROJECT_GIT_PATHS
    else:
        pathspecs = CALCULATOR_GIT_PATHS
    return {
        "completionState": "continue",
        "thoughtSummary": "Produce the missing version-control snapshot evidence.",
        "executionGraph": {
            "nodes": git_snapshot_nodes([], pathspecs=pathspecs)
        }
    }

def main_repair_graph_response():
    return {
        "completionState": "continue",
        "thoughtSummary": "Repair by applying unified diff and rerunning tests.",
        "capabilityPolicySkips": capability_policy_skip(),
        "executionGraph": {
            "nodes": [
                {
                    "id": "fix-source",
                    "kind": "patch/apply",
                    "input": {
                        "patchType": "unified_diff",
                        "targetRef": "calculator.go",
                        "diff": FIXED_DIFF,
                        "failedSurface": "targetRef=calculator.go",
                        "rootCauseTarget": "calculator.go",
                        "repairTargetDecision": "patch calculator.go to correct Add implementation",
                        "repairMode": "direct_state_change",
                        "forbiddenRepair": "do not patch tests when implementation target is available",
                        "verificationPlan": "run bounded go test ./... after patch",
                        "coverageTargets": [
                            "代码修改通过 Patch 证据可追溯",
                            "完成软件开发闭环",
                            "source_change",
                            "user_deliverable"
                        ]
                    }
                },
                {
                    "id": "run-tests-fixed",
                    "kind": "shell/command",
                    "dependsOn": ["fix-source"],
                    "input": {
                        "command": "go",
                        "args": ["test", "./..."],
                        "coverageTargets": [
                            "go test ./... 最终通过",
                            "结果包含 DoD 评估和交付物索引",
                            "validation_report"
                        ]
                    }
                },
                *git_snapshot_nodes("run-tests-fixed")
            ]
        }
    }

def replan_continuation_graph_response():
    return {
        "completionState": "continue",
        "thoughtSummary": "Replan continuation adds bounded regression evidence and reruns tests.",
        "capabilityPolicySkips": capability_policy_skip(),
        "executionGraph": {
            "nodes": [
                {
                    "id": "write-replan-regression-test",
                    "kind": "patch/apply",
                    "input": {
                        "patchType": "file_write",
                        "targetRef": "calculator_replan_test.go",
                        "content": REPLAN_TEST,
                        "failedSurface": "targetRef=calculator.go",
                        "rootCauseTarget": "calculator_replan_test.go",
                        "repairTargetDecision": "add regression coverage for current repaired implementation",
                        "repairMode": "direct_state_change",
                        "forbiddenRepair": "do not reuse stale unified diff hunks",
                        "verificationPlan": "run bounded go test ./... after regression test write",
                        "coverageTargets": [
                            "代码修改通过 Patch 证据可追溯",
                            "go test ./... 最终通过",
                            "完成软件开发闭环",
                            "source_change",
                            "user_deliverable"
                        ]
                    }
                },
                {
                    "id": "run-tests-replan",
                    "kind": "shell/command",
                    "dependsOn": ["write-replan-regression-test"],
                    "input": {
                        "command": "go",
                        "args": ["test", "./..."],
                        "coverageTargets": [
                            "go test ./... 最终通过",
                            "结果包含 DoD 评估和交付物索引",
                            "validation_report"
                        ]
                    }
                },
                *git_snapshot_nodes("run-tests-replan", pathspecs=REPLAN_GIT_PATHS)
            ]
        }
    }

def empty_workspace_evidence_response():
    return {
        "completionState": "continue",
        "thoughtSummary": "Collect bounded evidence that the target workspace is empty before creating project files.",
        "executionGraph": {
            "nodes": [
                {
                    "id": "list-empty-workspace",
                    "kind": "file/list",
                    "input": {
                        "path": ".",
                        "boundedReason": "inspect empty target workspace before project initialization"
                    }
                },
                {
                    "id": "collect-empty-workspace-evidence",
                    "kind": "shell/command",
                    "dependsOn": ["list-empty-workspace"],
                    "input": {
                        "command": "bash",
                        "args": ["-lc", "find . -maxdepth 2 -type f -print | sort; go test ./... 2>&1 || true"],
                        "verificationScope": "empty Go workspace initialization gap",
                        "boundedReason": "record current file inventory and concrete go test failure without mutating workspace"
                    }
                }
            ]
        }
    }

def empty_project_graph_response():
    return {
        "completionState": "continue",
        "thoughtSummary": "Decision information was provided; create a minimal project from the empty workspace and run tests.",
        "capabilityPolicySkips": capability_policy_skip(),
        "executionGraph": {
            "nodes": [
                {
                    "id": "write-go-mod",
                    "kind": "patch/apply",
                    "input": {
                        "patchType": "file_write",
                        "targetRef": "go.mod",
                        "content": "module example.com/targetproject\n\ngo 1.22\n",
                        "coverageTargets": ["生成最小可运行 Go 项目", "source_change"]
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
                        "coverageTargets": ["生成最小可运行 Go 项目", "source_change", "user_deliverable", f"[{e2e_marker}] 从空 workspace 初始化一个最小 Go 项目，并完成可追溯的开发、测试和交付。"]
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
                        "coverageTargets": ["go test ./... 必须包含有效断言并通过", "source_change"]
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
                        "coverageTargets": ["README.md 必须作为交付物证据可追溯", "user_deliverable", f"[{e2e_marker}] 补充目标：项目还必须生成 README.md，并说明这是从空 workspace 初始化的新项目。"]
                    }
                },
                {
                    "id": "run-empty-project-tests",
                    "kind": "shell/command",
                    "dependsOn": ["write-readme"],
                    "input": {
                        "command": "go",
                        "args": ["test", "./..."],
                        "coverageTargets": [
                            "go test ./... 必须包含有效断言并通过",
                            "目标项目外部依赖配置必须按 target_workspace/external_dependency 作用域处理",
                            "敏感认证信息不得进入 snapshot、observability、event 或 LLM context",
                            "validation_report"
                        ]
                    }
                },
                *git_snapshot_nodes("run-empty-project-tests", pathspecs=NEW_PROJECT_GIT_PATHS)
            ]
        }
    }

def cumulative_empty_project_graph_response():
    return {
        "completionState": "continue",
        "thoughtSummary": "Existing project evidence was found; add current-run cumulative delivery evidence and rerun tests.",
        "capabilityPolicySkips": capability_policy_skip(),
        "executionGraph": {
            "nodes": [
                {
                    "id": "collect-cumulative-baseline",
                    "kind": "shell/command",
                    "input": {
                        "command": "bash",
                        "args": ["-lc", "test -f README.md && grep -q 'empty workspace' README.md && test -f go.mod"],
                        "verificationScope": "cumulative empty workspace baseline",
                        "boundedReason": "confirm existing project deliverables before cumulative update"
                    }
                },
                {
                    "id": "write-cumulative-source",
                    "kind": "patch/apply",
                    "dependsOn": ["collect-cumulative-baseline"],
                    "input": {
                        "patchType": "file_write",
                        "targetRef": "cumulative_feature.go",
                        "content": CUMULATIVE_PROJECT_SOURCE,
                        "coverageTargets": ["生成最小可运行 Go 项目", "source_change", "user_deliverable", f"[{e2e_marker}] 从空 workspace 初始化一个最小 Go 项目，并完成可追溯的开发、测试和交付。"]
                    }
                },
                {
                    "id": "write-cumulative-tests",
                    "kind": "patch/apply",
                    "dependsOn": ["write-cumulative-source"],
                    "input": {
                        "patchType": "file_write",
                        "targetRef": "cumulative_feature_test.go",
                        "content": CUMULATIVE_PROJECT_TEST,
                        "coverageTargets": ["go test ./... 必须包含有效断言并通过", "source_change"]
                    }
                },
                {
                    "id": "update-readme",
                    "kind": "patch/apply",
                    "dependsOn": ["write-cumulative-tests"],
                    "input": {
                        "patchType": "text_replace",
                        "targetRef": "README.md",
                        "oldText": NEW_PROJECT_README,
                        "newText": CUMULATIVE_PROJECT_README,
                        "coverageTargets": ["README.md 必须作为交付物证据可追溯", "user_deliverable", f"[{e2e_marker}] 补充目标：项目还必须生成 README.md，并说明这是从空 workspace 初始化的新项目。"]
                    }
                },
                {
                    "id": "run-empty-project-tests",
                    "kind": "shell/command",
                    "dependsOn": ["update-readme"],
                    "input": {
                        "command": "go",
                        "args": ["test", "./..."],
                        "coverageTargets": [
                            "go test ./... 必须包含有效断言并通过",
                            "目标项目外部依赖配置必须按 target_workspace/external_dependency 作用域处理",
                            "敏感认证信息不得进入 snapshot、observability、event 或 LLM context",
                            "validation_report"
                        ]
                    }
                },
                *git_snapshot_nodes("run-empty-project-tests", pathspecs=CUMULATIVE_PROJECT_GIT_PATHS)
            ]
        }
    }

def has_completed_empty_project_graph_evidence(merged_content):
    text = str(merged_content or "")
    return (
        (
            re.search(r'run-001/graph-\d+/001-write-go-mod', text) is not None
            or "write-go-mod" in text
        )
        and (
            re.search(r'run-001/graph-\d+/004-write-readme', text) is not None
            or "write-readme" in text
        )
        and (
            re.search(r'run-001/graph-\d+/005-run-empty-project-tests', text) is not None
            or "run-empty-project-tests" in text
        )
        and (
            re.search(r'run-001/graph-\d+/008-git-commit', text) is not None
            or "git-commit" in text
        )
    )

def has_completed_cumulative_empty_project_graph_evidence(run_id, merged_content):
    text = str(merged_content or "")
    return (
        (
            re.search(re.escape(run_id) + r'/graph-\d+/002-write-cumulative-source', text) is not None
            or "write-cumulative-source" in text
        )
        and (
            re.search(re.escape(run_id) + r'/graph-\d+/004-update-readme', text) is not None
            or "update-readme" in text
        )
        and (
            re.search(re.escape(run_id) + r'/graph-\d+/005-run-empty-project-tests', text) is not None
            or "run-empty-project-tests" in text
        )
        and (
            re.search(re.escape(run_id) + r'/graph-\d+/008-git-commit', text) is not None
            or "git-commit" in text
        )
    )

def needs_completed_empty_project_assessment(step_id, merged_content):
    text = str(merged_content or "")
    return (
        has_completed_empty_project_graph_evidence(text)
        and (
            "concrete validation evidence repair must return" in text
            or '"repairObjective":"produce_concrete_validation_evidence"' in text
            or '"mustProduceConcreteValidationEvidence":true' in text
            or '"currentRuntimeStage":"verification"' in text
            or '"runtimeStage":"verification"' in text
            or "provider-retry" in step_id
        )
    )

def needs_completed_cumulative_empty_project_assessment(run_id, step_id, merged_content):
    text = str(merged_content or "")
    return (
        has_completed_cumulative_empty_project_graph_evidence(run_id, text)
        and (
            "concrete validation evidence repair must return" in text
            or '"repairObjective":"produce_concrete_validation_evidence"' in text
            or '"mustProduceConcreteValidationEvidence":true' in text
            or '"currentRuntimeStage":"verification"' in text
            or '"runtimeStage":"verification"' in text
            or "provider-retry" in step_id
            or is_after_graph_step(step_id)
        )
    )

def latest_empty_project_graph_id(run_id, merged_content):
    text = str(merged_content or "")
    graph_numbers = []
    for match in re.finditer(re.escape(run_id) + r'/graph-(\d+)/008-git-commit', text):
        try:
            graph_numbers.append(int(match.group(1)))
        except ValueError:
            pass
    if not graph_numbers:
        return ""
    return f"graph-{max(graph_numbers):03d}"

def latest_empty_workspace_evidence_graph_id(run_id, merged_content):
    text = str(merged_content or "")
    graph_numbers = []
    for match in re.finditer(re.escape(run_id) + r'/graph-(\d+)/002-collect-empty-workspace-evidence', text):
        try:
            graph_numbers.append(int(match.group(1)))
        except ValueError:
            pass
    if not graph_numbers:
        return "graph-001"
    return f"graph-{max(graph_numbers):03d}"

def empty_workspace_gap_assessment(run_id, step_id, merged_content):
    graph = latest_empty_workspace_evidence_graph_id(run_id, merged_content)
    evidence_refs = [
        f"{run_id}/{graph}/001-list-empty-workspace",
        f"{run_id}/{graph}/002-collect-empty-workspace-evidence",
    ]
    return {
        "completionState": "completed",
        "thoughtSummary": "Empty workspace evidence was collected; implementation artifacts are still missing.",
        "dodAssessment": {
            "satisfied": False,
            "summary": "The workspace is empty and still needs a state-changing project initialization.",
            "goalOutcome": "No project files have been generated yet.",
            "implementationSummary": "No implementation artifact exists after evidence acquisition.",
            "verificationSummary": "Bounded evidence shows the workspace has no Go module or tests yet.",
            "items": [
                {"item": "空 workspace 不要求用户提供源码来源", "state": "satisfied", "evidenceRefs": evidence_refs},
                {"item": "生成最小可运行 Go 项目", "state": "failed", "evidenceRefs": evidence_refs, "reason": "go.mod and source files are absent."},
                {"item": "go test ./... 必须包含有效断言并通过", "state": "failed", "evidenceRefs": evidence_refs, "reason": "No Go module or test files exist yet."},
                {"item": "README.md 必须作为交付物证据可追溯", "state": "failed", "evidenceRefs": evidence_refs, "reason": "README.md has not been generated."},
                {"item": "目标项目外部依赖配置必须按 target_workspace/external_dependency 作用域处理", "state": "failed", "evidenceRefs": evidence_refs, "reason": "No target project exists to apply dependency scoping."},
                {"item": "敏感认证信息不得进入 snapshot、observability、event 或 LLM context", "state": "failed", "evidenceRefs": evidence_refs, "reason": "No final snapshot or observability validation exists yet."}
            ],
            "goalCoverage": {
                "satisfied": False,
                "items": [
                    {"goal": f"[{e2e_marker}] 从空 workspace 初始化一个最小 Go 项目，并完成可追溯的开发、测试和交付。", "state": "failed", "evidenceRefs": evidence_refs, "reason": "State-changing project initialization has not run."},
                    {"goal": f"[{e2e_marker}] 补充目标：项目还必须生成 README.md，并说明这是从空 workspace 初始化的新项目。", "state": "failed", "evidenceRefs": evidence_refs, "reason": "README.md has not been generated."}
                ]
            },
            "deliverableCoverage": {
                "satisfied": False,
                "items": [
                    {"id": "source_change", "state": "failed", "evidenceRefs": evidence_refs},
                    {"id": "validation_report", "state": "failed", "evidenceRefs": evidence_refs},
                    {"id": "coverage_map", "state": "failed", "evidenceRefs": [step_id]},
                    {"id": "version_control_snapshot", "state": "failed", "evidenceRefs": evidence_refs},
                    {"id": "user_deliverable", "state": "failed", "evidenceRefs": evidence_refs}
                ]
            }
        }
    }

def completed_empty_project_assessment(run_id, step_id, merged_content):
    graph = latest_empty_project_graph_id(run_id, merged_content)
    if graph == "":
        graph = "graph-001"
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
                {"item": "空 workspace 不要求用户提供源码来源", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-write-go-mod", f"{run_id}/{graph}/002-write-source"]},
                {"item": "生成最小可运行 Go 项目", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-write-go-mod", f"{run_id}/{graph}/002-write-source", f"{run_id}/{graph}/003-write-tests"]},
                {"item": "go test ./... 必须包含有效断言并通过", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"]},
                {"item": "README.md 必须作为交付物证据可追溯", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/004-write-readme"]},
                {"item": "目标项目外部依赖配置必须按 target_workspace/external_dependency 作用域处理", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"]},
                {"item": "敏感认证信息不得进入 snapshot、observability、event 或 LLM context", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"]}
            ],
            "goalCoverage": {
                "satisfied": True,
                "items": [
                    {"goal": f"[{e2e_marker}] 从空 workspace 初始化一个最小 Go 项目，并完成可追溯的开发、测试和交付。", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-write-go-mod", f"{run_id}/{graph}/002-write-source", f"{run_id}/{graph}/003-write-tests", f"{run_id}/{graph}/005-run-empty-project-tests"]},
                    {"goal": f"[{e2e_marker}] 补充目标：项目还必须生成 README.md，并说明这是从空 workspace 初始化的新项目。", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/004-write-readme"]}
                ]
            },
            "acceptanceDimensions": {
                "satisfied": True,
                "items": [
                    {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"]}
                ]
            },
            "deliverableCoverage": {
                "satisfied": True,
                "items": [
                    {"id": "source_change", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-write-go-mod", f"{run_id}/{graph}/002-write-source", f"{run_id}/{graph}/003-write-tests", f"{run_id}/{graph}/004-write-readme"], "artifactRefs": [f"{run_id}/{graph}/001-write-go-mod", f"{run_id}/{graph}/002-write-source", f"{run_id}/{graph}/003-write-tests", f"{run_id}/{graph}/004-write-readme"]},
                    {"id": "validation_report", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"], "artifactRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"]},
                    {"id": "coverage_map", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"], "artifactRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"]},
                    git_snapshot_deliverable(f"{run_id}/{graph}/008-git-commit"),
                    {"id": "user_deliverable", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/002-write-source", f"{run_id}/{graph}/004-write-readme"], "artifactRefs": [f"{run_id}/{graph}/002-write-source", f"{run_id}/{graph}/004-write-readme"]}
                ]
            }
        }
    }

def completed_cumulative_empty_project_assessment(run_id, step_id, merged_content):
    graph = latest_empty_project_graph_id(run_id, merged_content)
    if graph == "":
        graph = "graph-001"
    return {
        "completionState": "completed",
        "thoughtSummary": "Cumulative empty-workspace project verification is complete with fresh current-run evidence.",
        "dodAssessment": {
            "satisfied": True,
            "summary": "Complex software-dev DoD satisfied for cumulative empty workspace verification.",
            "goalOutcome": "Kept the initialized Go project valid and added current-run cumulative delivery evidence.",
            "implementationSummary": "Added cumulative_feature.go, cumulative_feature_test.go, and updated README.md.",
            "verificationSummary": "go test ./... passed with the original and cumulative assertions.",
            "items": [
                {"item": "空 workspace 不要求用户提供源码来源", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-collect-cumulative-baseline", f"{run_id}/{graph}/002-write-cumulative-source"]},
                {"item": "生成最小可运行 Go 项目", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/002-write-cumulative-source", f"{run_id}/{graph}/003-write-cumulative-tests"]},
                {"item": "go test ./... 必须包含有效断言并通过", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"]},
                {"item": "README.md 必须作为交付物证据可追溯", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-collect-cumulative-baseline", f"{run_id}/{graph}/004-update-readme"]},
                {"item": "目标项目外部依赖配置必须按 target_workspace/external_dependency 作用域处理", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"]},
                {"item": "敏感认证信息不得进入 snapshot、observability、event 或 LLM context", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"]}
            ],
            "goalCoverage": {
                "satisfied": True,
                "items": [
                    {"goal": f"[{e2e_marker}] 从空 workspace 初始化一个最小 Go 项目，并完成可追溯的开发、测试和交付。", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/002-write-cumulative-source", f"{run_id}/{graph}/003-write-cumulative-tests", f"{run_id}/{graph}/005-run-empty-project-tests"]},
                    {"goal": f"[{e2e_marker}] 补充目标：项目还必须生成 README.md，并说明这是从空 workspace 初始化的新项目。", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-collect-cumulative-baseline", f"{run_id}/{graph}/004-update-readme"]}
                ]
            },
            "acceptanceDimensions": {
                "satisfied": True,
                "items": [
                    {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"]}
                ]
            },
            "deliverableCoverage": {
                "satisfied": True,
                "items": [
                    {"id": "source_change", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/002-write-cumulative-source", f"{run_id}/{graph}/003-write-cumulative-tests", f"{run_id}/{graph}/004-update-readme"], "artifactRefs": [f"{run_id}/{graph}/002-write-cumulative-source", f"{run_id}/{graph}/003-write-cumulative-tests", f"{run_id}/{graph}/004-update-readme"]},
                    {"id": "validation_report", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"], "artifactRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"]},
                    {"id": "coverage_map", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"], "artifactRefs": [f"{run_id}/{graph}/005-run-empty-project-tests"]},
                    git_snapshot_deliverable(f"{run_id}/{graph}/008-git-commit"),
                    {"id": "user_deliverable", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/002-write-cumulative-source", f"{run_id}/{graph}/004-update-readme"], "artifactRefs": [f"{run_id}/{graph}/002-write-cumulative-source", f"{run_id}/{graph}/004-update-readme"]}
                ]
            }
        }
    }

def completed_main_repair_assessment(run_id, step_id):
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
                {"item": "代码修改通过 Patch 证据可追溯", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/001-fix-source"]},
                {"item": "go test ./... 最终通过", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/002-run-tests-fixed"]},
                {"item": "结果包含 DoD 评估和交付物索引", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/002-run-tests-fixed"]}
            ],
            "goalCoverage": {
                "satisfied": True,
                "items": [
                    {"goal": f"[{e2e_marker}] 完成软件开发闭环：通过 executionGraph 修改代码并运行测试，失败时自动修复，最终基于证据输出 DoD 评估。", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/001-fix-source", f"{run_id}/graph-002/002-run-tests-fixed"]}
                ]
            },
            "acceptanceDimensions": {
                "satisfied": True,
                "items": [
                    {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/002-run-tests-fixed"]},
                    {"id": "runtime_entrypoint", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/002-run-tests-fixed"]}
                ]
            },
            "deliverableCoverage": {
                "satisfied": True,
                "items": [
                    {"id": "source_change", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/001-fix-source"], "artifactRefs": [f"{run_id}/graph-002/001-fix-source"]},
                    {"id": "validation_report", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/002-run-tests-fixed"], "artifactRefs": [f"{run_id}/graph-002/002-run-tests-fixed"]},
                    {"id": "coverage_map", "state": "satisfied", "evidenceRefs": [step_id], "artifactRefs": [step_id]},
                    git_snapshot_deliverable(f"{run_id}/graph-002/005-git-commit"),
                    {"id": "user_deliverable", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/001-fix-source"], "artifactRefs": [f"{run_id}/graph-002/001-fix-source"]}
                ]
            }
        }
    }

def completed_replan_assessment(run_id, step_id):
    return {
        "completionState": "completed",
        "thoughtSummary": "Replan continuation completed after repair exhaustion.",
        "dodAssessment": {
            "satisfied": True,
            "summary": "Repair exhaustion was automatically replanned and closed by a continuation run.",
            "goalOutcome": "A continuation run was created after auto governance selected replan.",
            "implementationSummary": "The continuation run added regression test coverage through patch evidence.",
            "verificationSummary": "go test ./... passed in the continuation run.",
            "items": [
                {"item": "代码修改通过 Patch 证据可追溯", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/001-write-replan-regression-test"]},
                {"item": "go test ./... 最终通过", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/002-run-tests-replan"]},
                {"item": "结果包含 DoD 评估和交付物索引", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/002-run-tests-replan"]}
            ],
            "goalCoverage": {
                "satisfied": True,
                "items": [
                    {"goal": "完成软件开发闭环", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/001-write-replan-regression-test", f"{run_id}/graph-002/002-run-tests-replan"]}
                ]
            },
            "acceptanceDimensions": {
                "satisfied": True,
                "items": [
                    {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/002-run-tests-replan"]},
                    {"id": "runtime_entrypoint", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/002-run-tests-replan"]}
                ]
            },
            "deliverableCoverage": {
                "satisfied": True,
                "items": [
                    {"id": "source_change", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/001-write-replan-regression-test"], "artifactRefs": [f"{run_id}/graph-002/001-write-replan-regression-test"]},
                    {"id": "validation_report", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/002-run-tests-replan"], "artifactRefs": [f"{run_id}/graph-002/002-run-tests-replan"]},
                    {"id": "coverage_map", "state": "satisfied", "evidenceRefs": [step_id], "artifactRefs": [step_id]},
                    git_snapshot_deliverable(f"{run_id}/graph-002/005-git-commit"),
                    {"id": "user_deliverable", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/001-write-replan-regression-test"], "artifactRefs": [f"{run_id}/graph-002/001-write-replan-regression-test"]}
                ]
            }
        }
    }

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
        if needs_completed_repair_assessment(step_id, merged_content):
            return completed_main_repair_assessment(run_id, step_id)
        if step_id.endswith("/step-1"):
            if needs_concrete_failure_evidence(merged_content):
                return concrete_failure_evidence_response()
            return {
                "completionState": "continue",
                "thoughtSummary": "Plan graph for patch write and test execution.",
                "capabilityPolicySkips": capability_policy_skip(),
                "executionGraph": {
                    "nodes": [
                        {
                            "id": "write-buggy-source",
                            "kind": "patch/apply",
                            "input": {
                                "patchType": "file_write",
                                "targetRef": "calculator.go",
                                "content": BUGGY_SOURCE,
                                "coverageTargets": [
                                    "代码修改通过 Patch 证据可追溯",
                                    "完成软件开发闭环"
                                ]
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
            if "git-commit" not in step_id and not has_completed_repair_graph_evidence(merged_content) and needs_state_changing_repair(merged_content):
                return main_repair_graph_response()
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
                        {"item": "代码修改通过 Patch 证据可追溯", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/001-fix-source"]},
                        {"item": "go test ./... 最终通过", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/002-run-tests-fixed"]},
                        {"item": "结果包含 DoD 评估和交付物索引", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/002-run-tests-fixed"]}
                    ],
                    "goalCoverage": {
                        "satisfied": True,
                        "items": [
                            {"goal": f"[{e2e_marker}] 完成软件开发闭环：通过 executionGraph 修改代码并运行测试，失败时自动修复，最终基于证据输出 DoD 评估。", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/001-fix-source", f"{run_id}/graph-002/002-run-tests-fixed"]}
                        ]
                    },
                    "acceptanceDimensions": {
                        "satisfied": True,
                        "items": [
                            {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/002-run-tests-fixed"]},
                            {"id": "runtime_entrypoint", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/002-run-tests-fixed"]}
                        ]
                    },
                    "deliverableCoverage": {
                        "satisfied": True,
                        "items": [
                            {"id": "source_change", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/001-fix-source"], "artifactRefs": [f"{run_id}/graph-002/001-fix-source"]},
                            {"id": "validation_report", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/002-run-tests-fixed"], "artifactRefs": [f"{run_id}/graph-002/002-run-tests-fixed"]},
                            {"id": "coverage_map", "state": "satisfied", "artifactRefs": [step_id]},
                            git_snapshot_deliverable(f"{run_id}/graph-002/005-git-commit"),
                            {"id": "user_deliverable", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-002/001-fix-source"], "artifactRefs": [f"{run_id}/graph-002/001-fix-source"]}
                        ]
                    }
                }
            }
        if is_repair_step(step_id):
            if needs_concrete_failure_evidence(merged_content):
                return concrete_failure_evidence_response()
            if needs_version_control_snapshot(merged_content) and "002-run-tests" not in step_id:
                return git_snapshot_repair_response(merged_content)
            return main_repair_graph_response()
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
        if needs_completed_empty_project_assessment(step_id, merged_content):
            return completed_empty_project_assessment(run_id, step_id, merged_content)
        if has_completed_empty_project_graph_evidence(merged_content):
            return completed_empty_project_assessment(run_id, step_id, merged_content)
        if is_repair_step(step_id) or is_no_actionable_followup_step(step_id):
            if needs_concrete_failure_evidence(merged_content):
                return empty_workspace_evidence_response()
            if needs_state_changing_repair(merged_content):
                return empty_project_graph_response()
            return empty_project_graph_response()
        if is_after_graph_step(step_id) and "collect-empty-workspace-evidence" in step_id:
            if needs_concrete_failure_evidence(merged_content):
                return empty_workspace_gap_assessment(run_id, step_id, merged_content)
            return empty_project_graph_response()
        if is_after_graph_step(step_id):
            return completed_empty_project_assessment(run_id, step_id, merged_content)
        if "llm-after-decision" in step_id or "after-decision" in step_id:
            if needs_concrete_failure_evidence(merged_content):
                return empty_workspace_evidence_response()
            return empty_project_graph_response()
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
            if needs_concrete_failure_evidence(merged_content):
                return no_actionable_concrete_evidence_response()
            return {
                "completionState": "continue",
                "thoughtSummary": "Plan closure evidence and version-control snapshot before no-actionable continuation.",
                "executionGraph": {
                    "nodes": [
                        {
                            "id": "collect-evidence",
                            "kind": "shell/command",
                            "input": {
                                "command": "sh",
                                "args": ["-c", "printf continuation-evidence > runtime-closure-evidence.txt"],
                                "coverageTargets": [
                                    "no-actionable continuation 必须进入 validation follow-up 并完成 DoD 评估",
                                    "source_change",
                                    "validation_report",
                                    "user_deliverable"
                                ]
                            }
                        },
                        *git_snapshot_nodes("collect-evidence", pathspecs=RUNTIME_CLOSURE_GIT_PATHS)
                    ]
                }
            }
        if is_repair_step(step_id):
            if has_completed_no_actionable_state_change_graph(run_id, merged_content):
                graph = latest_no_actionable_state_change_graph_id(run_id, merged_content)
                return {
                    "completionState": "completed",
                    "thoughtSummary": "No-actionable continuation was closed after runtime repair produced deliverable evidence.",
                    "dodAssessment": {
                        "satisfied": True,
                        "summary": "Graph exhausted validation completed with current runtime closure evidence.",
                        "goalOutcome": "The run completed after no-actionable validation follow-up and bounded repair.",
                        "implementationSummary": "Wrote runtime-closure-evidence.txt as the required closure deliverable.",
                        "verificationSummary": "Validated the closure file and committed the evidence snapshot.",
                        "items": [
                            {"item": "no-actionable continuation 必须进入 validation follow-up 并完成 DoD 评估", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/002-validate-runtime-closure"]}
                        ],
                        "goalCoverage": {
                            "satisfied": True,
                            "items": [
                                {"goal": f"[{e2e_marker}] 验证当 LLM 返回 continue 但没有可执行后续动作时，系统会进入 validation follow-up 并完成 DoD 评估。", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-write-runtime-closure-evidence", f"{run_id}/{graph}/002-validate-runtime-closure"]}
                            ]
                        },
                        "acceptanceDimensions": {
                            "satisfied": True,
                            "items": [
                                {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/002-validate-runtime-closure"]}
                            ]
                        },
                        "deliverableCoverage": {
                            "satisfied": True,
                            "items": [
                                {"id": "source_change", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-write-runtime-closure-evidence"], "artifactRefs": [f"{run_id}/{graph}/001-write-runtime-closure-evidence"]},
                                {"id": "validation_report", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/002-validate-runtime-closure"], "artifactRefs": [f"{run_id}/{graph}/002-validate-runtime-closure"]},
                                {"id": "coverage_map", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/002-validate-runtime-closure"], "artifactRefs": [f"{run_id}/{graph}/002-validate-runtime-closure"]},
                                git_snapshot_deliverable(f"{run_id}/{graph}/005-git-commit"),
                                {"id": "user_deliverable", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-write-runtime-closure-evidence"], "artifactRefs": [f"{run_id}/{graph}/001-write-runtime-closure-evidence"]}
                            ]
                        }
                    }
                }
            if needs_concrete_failure_evidence(merged_content):
                return no_actionable_concrete_evidence_response()
            if needs_state_changing_repair(merged_content) or needs_version_control_snapshot(merged_content) or "requires at least one traceable direct evidenceRef" in merged_content:
                return no_actionable_state_change_graph_response()
            return no_actionable_state_change_graph_response()
        if is_no_actionable_followup_step(step_id):
            if has_completed_no_actionable_state_change_graph(run_id, merged_content):
                graph = latest_no_actionable_state_change_graph_id(run_id, merged_content)
                return {
                    "completionState": "completed",
                    "thoughtSummary": "No-actionable continuation was converted into validation after state-changing repair.",
                    "dodAssessment": {
                        "satisfied": True,
                        "summary": "Graph exhausted validation completed with current runtime closure evidence.",
                        "goalOutcome": "The run completed after graph exhaustion validation.",
                        "implementationSummary": "Wrote runtime-closure-evidence.txt as the required closure deliverable.",
                        "verificationSummary": "Validated the closure file and committed the evidence snapshot.",
                        "items": [
                            {"item": "no-actionable continuation 必须进入 validation follow-up 并完成 DoD 评估", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/002-validate-runtime-closure"]}
                        ],
                        "goalCoverage": {
                            "satisfied": True,
                            "items": [
                                {"goal": f"[{e2e_marker}] 验证当 LLM 返回 continue 但没有可执行后续动作时，系统会进入 validation follow-up 并完成 DoD 评估。", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-write-runtime-closure-evidence", f"{run_id}/{graph}/002-validate-runtime-closure"]}
                            ]
                        },
                        "acceptanceDimensions": {
                            "satisfied": True,
                            "items": [
                                {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/002-validate-runtime-closure"]}
                            ]
                        },
                        "deliverableCoverage": {
                            "satisfied": True,
                            "items": [
                                {"id": "source_change", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-write-runtime-closure-evidence"], "artifactRefs": [f"{run_id}/{graph}/001-write-runtime-closure-evidence"]},
                                {"id": "validation_report", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/002-validate-runtime-closure"], "artifactRefs": [f"{run_id}/{graph}/002-validate-runtime-closure"]},
                                {"id": "coverage_map", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/002-validate-runtime-closure"], "artifactRefs": [f"{run_id}/{graph}/002-validate-runtime-closure"]},
                                git_snapshot_deliverable(f"{run_id}/{graph}/005-git-commit"),
                                {"id": "user_deliverable", "state": "satisfied", "evidenceRefs": [f"{run_id}/{graph}/001-write-runtime-closure-evidence"], "artifactRefs": [f"{run_id}/{graph}/001-write-runtime-closure-evidence"]}
                            ]
                        }
                    }
                }
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
                        {"item": "no-actionable continuation 必须进入 validation follow-up 并完成 DoD 评估", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/001-collect-evidence", step_id]}
                    ],
                    "goalCoverage": {
                        "satisfied": True,
                        "items": [
                            {"goal": f"[{e2e_marker}] 验证当 LLM 返回 continue 但没有可执行后续动作时，系统会进入 validation follow-up 并完成 DoD 评估。", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/001-collect-evidence", step_id]}
                        ]
                    },
                    "acceptanceDimensions": {
                        "satisfied": True,
                        "items": [
                            {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/001-collect-evidence", step_id]}
                        ]
                    },
                    "deliverableCoverage": {
                        "satisfied": True,
                        "items": [
                            {"id": "source_change", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/001-collect-evidence"], "artifactRefs": [f"{run_id}/graph-001/001-collect-evidence", step_id]},
                            {"id": "validation_report", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/001-collect-evidence"], "artifactRefs": [f"{run_id}/graph-001/001-collect-evidence", step_id]},
                            {"id": "coverage_map", "state": "satisfied", "evidenceRefs": [step_id], "artifactRefs": [step_id]},
                            git_snapshot_deliverable(f"{run_id}/graph-001/004-git-commit"),
                            {"id": "user_deliverable", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/001-collect-evidence"], "artifactRefs": [f"{run_id}/graph-001/001-collect-evidence", step_id]}
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
            if needs_concrete_failure_evidence(merged_content):
                return graph_limit_concrete_evidence_response()
            return graph_limit_seed_graph_response()
        if is_repair_step(step_id):
            if needs_concrete_validation_evidence(merged_content) and has_completed_graph_limit_seed_graph(run_id, merged_content):
                return graph_limit_completed_assessment(run_id, step_id, merged_content)
            if needs_concrete_failure_evidence(merged_content):
                return graph_limit_concrete_evidence_response()
            if needs_state_changing_repair(merged_content) or needs_version_control_snapshot(merged_content) or "requires at least one traceable direct evidenceRef" in merged_content:
                return graph_limit_seed_graph_response()
            if has_completed_graph_limit_seed_graph(run_id, merged_content):
                return graph_limit_seed_graph_response()
            return graph_limit_seed_graph_response()
        if is_graph_limit_followup_step(step_id):
            return graph_limit_completed_assessment(run_id, step_id, merged_content)
        if is_no_actionable_followup_step(step_id):
            if has_completed_graph_limit_seed_graph(run_id, merged_content):
                return graph_limit_completed_assessment(run_id, step_id, merged_content)
            if needs_concrete_failure_evidence(merged_content):
                return graph_limit_concrete_evidence_response()
            return graph_limit_seed_graph_response()
        if is_after_graph_step(step_id):
            if needs_concrete_validation_evidence(merged_content) and has_completed_graph_limit_seed_graph(run_id, merged_content):
                return graph_limit_completed_assessment(run_id, step_id, merged_content)
            if needs_concrete_failure_evidence(merged_content):
                return graph_limit_concrete_evidence_response()
            if not has_completed_graph_limit_seed_graph(run_id, merged_content):
                return graph_limit_seed_graph_response()
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
        if has_completed_replan_graph_evidence(merged_content):
            return completed_replan_assessment(run_id, step_id)
        if step_id.endswith("/step-1"):
            if needs_concrete_failure_evidence(merged_content):
                return concrete_failure_evidence_response()
            return replan_continuation_graph_response()
        if is_repair_step(step_id):
            if needs_concrete_failure_evidence(merged_content):
                return concrete_failure_evidence_response()
            return replan_continuation_graph_response()
        return completed_replan_assessment(run_id, step_id)
    if scenario == 7:
        if needs_completed_cumulative_empty_project_assessment(run_id, step_id, merged_content):
            return completed_cumulative_empty_project_assessment(run_id, step_id, merged_content)
        if needs_completed_empty_project_assessment(step_id, merged_content):
            return completed_empty_project_assessment(run_id, step_id, merged_content)
        if step_id.endswith("/step-1") or is_repair_step(step_id):
            if needs_concrete_failure_evidence(merged_content):
                return empty_workspace_evidence_response()
            if needs_state_changing_repair(merged_content):
                return cumulative_empty_project_graph_response()
            return cumulative_empty_project_graph_response()
        if is_after_graph_step(step_id) and "collect-empty-workspace-evidence" in step_id:
            if needs_concrete_failure_evidence(merged_content):
                return empty_workspace_gap_assessment(run_id, step_id, merged_content)
            return cumulative_empty_project_graph_response()
        if is_after_graph_step(step_id):
            return {
                "completionState": "completed",
                "thoughtSummary": "Weak-evidence replan continuation closed with current-run delivery and verification evidence.",
                "dodAssessment": {
                    "satisfied": True,
                    "summary": "Continuation after weak-evidence repair exhaustion completed with current-run DoD coverage.",
                    "goalOutcome": "The cumulative empty-workspace goals are satisfied with fresh current-run evidence.",
                    "implementationSummary": "Regenerated the minimal project files and README in the current run.",
                    "verificationSummary": "The current run re-executed go test ./... with effective assertions.",
                    "items": [
                        {"item": "空 workspace 不要求用户提供源码来源", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/001-write-go-mod", f"{run_id}/graph-001/002-write-source"]},
                        {"item": "生成最小可运行 Go 项目", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/001-write-go-mod", f"{run_id}/graph-001/002-write-source", f"{run_id}/graph-001/003-write-tests"]},
                        {"item": "go test ./... 必须包含有效断言并通过", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/005-run-empty-project-tests"]},
                        {"item": "README.md 必须作为交付物证据可追溯", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/004-write-readme"]},
                        {"item": "目标项目外部依赖配置必须按 target_workspace/external_dependency 作用域处理", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/005-run-empty-project-tests"]},
                        {"item": "敏感认证信息不得进入 snapshot、observability、event 或 LLM context", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/005-run-empty-project-tests"]}
                    ],
                    "goalCoverage": {
                        "satisfied": True,
                        "items": [
                            {"goal": f"[{e2e_marker}] 从空 workspace 初始化一个最小 Go 项目，并完成可追溯的开发、测试和交付。", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/001-write-go-mod", f"{run_id}/graph-001/002-write-source", f"{run_id}/graph-001/003-write-tests", f"{run_id}/graph-001/005-run-empty-project-tests"]},
                            {"goal": f"[{e2e_marker}] 补充目标：项目还必须生成 README.md，并说明这是从空 workspace 初始化的新项目。", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/004-write-readme"]}
                        ]
                    },
                    "acceptanceDimensions": {
                        "satisfied": True,
                        "items": [
                            {"id": "functional_behavior", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/005-run-empty-project-tests"]}
                        ]
                    },
                    "deliverableCoverage": {
                        "satisfied": True,
                        "items": [
                            {"id": "source_change", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/001-write-go-mod", f"{run_id}/graph-001/002-write-source", f"{run_id}/graph-001/003-write-tests", f"{run_id}/graph-001/004-write-readme"], "artifactRefs": [f"{run_id}/graph-001/001-write-go-mod", f"{run_id}/graph-001/002-write-source", f"{run_id}/graph-001/003-write-tests", f"{run_id}/graph-001/004-write-readme"]},
                            {"id": "validation_report", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/005-run-empty-project-tests"], "artifactRefs": [f"{run_id}/graph-001/005-run-empty-project-tests"]},
                            {"id": "coverage_map", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/005-run-empty-project-tests"], "artifactRefs": [step_id]},
                            git_snapshot_deliverable(f"{run_id}/graph-001/008-git-commit"),
                            {"id": "user_deliverable", "state": "satisfied", "evidenceRefs": [f"{run_id}/graph-001/002-write-source", f"{run_id}/graph-001/004-write-readme"], "artifactRefs": [f"{run_id}/graph-001/002-write-source", f"{run_id}/graph-001/004-write-readme"]}
                        ]
                    }
                }
            }
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
                        git_snapshot_deliverable(step_id),
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

func Add(a, b int) int { return a - b }
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
wait_run_state "$workspace_id" "$scenario1_run_id" "completed" "$TMP_DIR/scenario1-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$scenario1_run_id/snapshot" >"$TMP_DIR/scenario1-snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$scenario1_run_id/observability" >"$TMP_DIR/scenario1-observability.json"
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
if len(patches) < 1:
    errors.append(f"patch evidence count={len(patches)}, want >=1")
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
wait_run_state_any "$workspace_id" "$scenario2_run_id" "$TMP_DIR/scenario2-status.json" "superseded" "blocked_on_decision"
scenario2_state="$(json_path "$TMP_DIR/scenario2-status.json" "status.state")"
scenario2_resolution_mode="auto"
if [[ "$scenario2_state" == "blocked_on_decision" ]]; then
  scenario2_resolution_mode="manual"
  scenario2_ticket_id="$(json_path "$TMP_DIR/scenario2-status.json" "status.blockingTicketId")"
  scenario2_ticket_path="$(url_encode "$scenario2_ticket_id")"
  scenario2_resolve_status="$(curl -sS -o "$TMP_DIR/scenario2-ticket-resolved.json" -w '%{http_code}' \
    -X POST "$BASE_URL/api/v1/workorch/decisions/$scenario2_ticket_path/resolve" -H 'Content-Type: application/json' \
    -d '{"resolutionType":"replan","resolutionPayload":{"operator":"software-dev-e2e","reason":"repair exhausted replan continuation verification"}}')"
  if [[ "$scenario2_resolve_status" != "200" && "$scenario2_resolve_status" != "409" ]]; then
    cat "$TMP_DIR/scenario2-ticket-resolved.json" >&2
    exit 1
  fi
  wait_run_state "$workspace_id" "$scenario2_run_id" "superseded" "$TMP_DIR/scenario2-status.json"
fi
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$scenario2_run_id/snapshot" >"$TMP_DIR/scenario2-snapshot.json"
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
wait_run_state_any "$scenario3_workspace_id" "$scenario3_run_id" "$TMP_DIR/scenario3-initial-status.json" "completed" "superseded"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/runs/$scenario3_run_id/snapshot" >"$TMP_DIR/scenario3-initial-snapshot.json"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/goals" -H 'Content-Type: application/json' \
  -d '{"text":"['"$E2E_MARKER"'] 补充目标：项目还必须生成 README.md，并说明这是从空 workspace 初始化的新项目。"}' >"$TMP_DIR/scenario3-goal-added.json"
scenario3_goal_added_id="$(json_path "$TMP_DIR/scenario3-goal-added.json" "goal.id")"

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/dods" -H 'Content-Type: application/json' \
  -d '{"items":["README.md 必须作为交付物证据可追溯","目标项目外部依赖配置必须按 target_workspace/external_dependency 作用域处理","敏感认证信息不得进入 snapshot、observability、event 或 LLM context"]}' >"$TMP_DIR/scenario3-dod-added.json"
scenario3_dod_added_id="$(json_path "$TMP_DIR/scenario3-dod-added.json" "dod.id")"

scenario3_cumulative_known_run_ids="$(list_workspace_run_ids "$scenario3_workspace_id")"
scenario3_cumulative_run_id="$(activate_workspace_execution \
  "$scenario3_workspace_id" \
  '{"goalId":"'"$scenario3_goal_added_id"'","doDId":"'"$scenario3_dod_added_id"'","llmProviderId":"verify","dialogLanguage":"zh-CN","priority":2}' \
  "$TMP_DIR/scenario3-cumulative-activation.json" \
  "$scenario3_cumulative_known_run_ids")"
wait_run_state_any "$scenario3_workspace_id" "$scenario3_cumulative_run_id" "$TMP_DIR/scenario3-completed-status.json" "completed" "superseded"
scenario3_effective_run_id="$scenario3_cumulative_run_id"
if [[ "$(json_path "$TMP_DIR/scenario3-completed-status.json" "status.state")" == "superseded" ]]; then
  deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/snapshot" >"$TMP_DIR/scenario3-workspace-snapshot.json"
    scenario3_successor_run_id="$(python3 - "$TMP_DIR/scenario3-workspace-snapshot.json" "$scenario3_cumulative_run_id" <<'PY'
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
  if [[ "$scenario3_effective_run_id" == "$scenario3_cumulative_run_id" ]]; then
    cat "$TMP_DIR/scenario3-workspace-snapshot.json" >&2
    exit 1
  fi
fi
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/runs/$scenario3_effective_run_id/snapshot" >"$TMP_DIR/scenario3-snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/runs/$scenario3_effective_run_id/observability" >"$TMP_DIR/scenario3-observability.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/runs/$scenario3_effective_run_id/context-bundle" >"$TMP_DIR/scenario3-context-bundle.json"
curl -fsS "$BASE_URL/api/v1/workorch/events?workspaceId=$scenario3_workspace_id&runId=$scenario3_effective_run_id&limit=200" >"$TMP_DIR/scenario3-events.json"
verify_software_dev_profile_snapshot "$scenario3_workspace_id" "$scenario3_effective_run_id" "$TMP_DIR/scenario3-snapshot.json"

python3 - \
  "$TMP_DIR/scenario3-initial-snapshot.json" \
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

initial_snapshot_path, snapshot_path, observability_path, context_path, events_path, request_log_path, scenario3_run_id, workdir, goal_initial, goal_added, dod_initial, dod_added = sys.argv[1:]
initial_snapshot_doc = json.load(open(initial_snapshot_path, encoding="utf-8"))
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

initial_snapshot = initial_snapshot_doc.get("snapshot") or {}
initial_run = initial_snapshot.get("run") or {}
if initial_run.get("state") != "completed":
    errors.append(f"initial run.state={initial_run.get('state')!r}, want completed")
open_text = rendered(initial_snapshot.get("decisions") or [])
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
definition_impact = execution_context.get("definitionChangeImpact") or {}
if definition_impact.get("policy") != "workspace_goal_dod_cumulative_latest_refines_previous":
    errors.append(f"definitionChangeImpact.policy={definition_impact.get('policy')!r}, want cumulative latest-refines-previous")
goal_change = definition_impact.get("goalChange") or {}
dod_change = definition_impact.get("doDChange") or {}
goal_revisions = goal_change.get("definitionRevisions") or []
dod_revisions = dod_change.get("definitionRevisions") or []
if not any(int(item.get("revisionCount") or 0) >= 2 for item in goal_revisions):
    errors.append(f"goal definition revisions={goal_revisions!r}, want cumulative revisionCount >= 2")
if not any(int(item.get("revisionCount") or 0) >= 2 for item in dod_revisions):
    errors.append(f"DoD definition revisions={dod_revisions!r}, want cumulative revisionCount >= 2")
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
patch_targets = {item.get("targetRef") for item in patches}
required_patch_targets = {"README.md", "cumulative_feature.go", "cumulative_feature_test.go"}
if len(patches) < len(required_patch_targets):
    errors.append(f"patch count={len(patches)}, want cumulative project patch evidence")
if not required_patch_targets.issubset(patch_targets):
    errors.append(f"patch targets={sorted(patch_targets)!r}, want {sorted(required_patch_targets)!r}")
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
    "initial_snapshot": initial_snapshot_doc,
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
wait_run_state_any "$scenario3_workspace_id" "$scenario4_run_id" "$TMP_DIR/scenario4-blocked-status.json" "blocked_on_decision" "superseded"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$scenario3_workspace_id/runs/$scenario4_run_id/snapshot" >"$TMP_DIR/scenario4-snapshot.json"

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
wait_run_state "$scenario5_workspace_id" "$scenario5_run_id" "completed" "$TMP_DIR/scenario5-completed-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$scenario5_workspace_id/runs/$scenario5_run_id/snapshot" >"$TMP_DIR/scenario5-snapshot.json"

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
    has_strict_recovery = any("follow-up-graph" in str(item.get("id") or "") and item.get("state") == "failed" for item in steps)
    has_repair_closure = any("write-runtime-closure-evidence" in str(item.get("id") or "") for item in steps) and any("git-commit" in str(item.get("id") or "") for item in steps)
    if not (has_strict_recovery and has_repair_closure):
        errors.append("missing no-actionable continuation recovery trace")
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
  -d '{"title":"runtime graph follow-up closure","description":"verify graph follow-up expansion limit assessment-only closure","deliverableDirectory":"'"$scenario6_workspace_workdir"'","templateId":"generic-governed"}' >"$TMP_DIR/scenario6-workspace.json"
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
wait_run_state "$scenario6_workspace_id" "$scenario6_run_id" "completed" "$TMP_DIR/scenario6-completed-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$scenario6_workspace_id/runs/$scenario6_run_id/snapshot" >"$TMP_DIR/scenario6-snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$scenario6_workspace_id/runs/$scenario6_run_id/observability" >"$TMP_DIR/scenario6-observability.json"

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
has_graph_limit_step = any("zz-llm-after-graph-limit-" in str(item.get("id") or "") or "graph-limit" in str(item.get("id") or "") for item in steps)
has_concrete_validation_closure = any(
    "follow-up-graph" in str(item.get("id") or "")
    and item.get("state") == "completed"
    for item in steps
)
if not has_graph_limit_step and not has_concrete_validation_closure:
    errors.append("missing graph expansion limit or concrete-validation assessment closure step")
if any("must_not_execute" in str(item.get("id") or "") for item in steps):
    errors.append("assessment-only executionGraph was executed unexpectedly")
results = snapshot.get("results") or []
if not any(item.get("state") == "accepted" for item in results):
    errors.append(f"results={results!r}, want accepted result")
traces = snapshot.get("closureTraces") or []
if has_graph_limit_step:
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
echo "scenario3InitialRun=$scenario3_run_id"
echo "scenario3Run=$scenario3_effective_run_id"
echo "scenario4Run=$scenario4_run_id"
echo "scenario5Run=$scenario5_run_id"
echo "scenario6Run=$scenario6_run_id"
