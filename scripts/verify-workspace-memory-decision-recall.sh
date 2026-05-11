#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKSPACE_ID="${1:-${WORKORCH_MEMORY_VERIFY_WORKSPACE_ID:-}}"
RUN_ID="${2:-${WORKORCH_MEMORY_VERIFY_RUN_ID:-}}"
DATA_ROOT="${WORKORCH_MEMORY_VERIFY_DATA_ROOT:-$ROOT_DIR/.workorch-data/workspaces}"

if [[ -z "$WORKSPACE_ID" ]]; then
  echo "usage: $0 <workspace-id> [run-id]" >&2
  echo "or set WORKORCH_MEMORY_VERIFY_WORKSPACE_ID and optional WORKORCH_MEMORY_VERIFY_RUN_ID" >&2
  exit 2
fi

WORKSPACE_DIR="$DATA_ROOT/$WORKSPACE_ID"
MEMORY_FILE="$WORKSPACE_DIR/memory_item.json"
TRACE_FILE="$WORKSPACE_DIR/memory_recall_trace.json"

if [[ ! -f "$MEMORY_FILE" ]]; then
  echo "missing memory file: $MEMORY_FILE" >&2
  exit 1
fi
if [[ ! -f "$TRACE_FILE" ]]; then
  echo "missing memory recall trace file: $TRACE_FILE" >&2
  exit 1
fi

python3 - "$MEMORY_FILE" "$TRACE_FILE" "$WORKSPACE_ID" "$RUN_ID" <<'PY'
import json
import sys

memory_path, trace_path, workspace_id, run_id = sys.argv[1:]

def load_items(path):
    with open(path, "r", encoding="utf-8") as handle:
        data = json.load(handle)
    if isinstance(data, dict):
        return list(data.values())
    if isinstance(data, list):
        return data
    raise SystemExit(f"unsupported json root in {path}: {type(data).__name__}")

def field(item, *names):
    for name in names:
        if name in item:
            return item[name]
    return None

memories = load_items(memory_path)
traces = load_items(trace_path)
if run_id:
    run_traces = [item for item in traces if field(item, "RunID", "runId") == run_id]
else:
    step_traces = [item for item in traces if field(item, "Source", "source") == "step_input"]
    if not step_traces:
        raise SystemExit("no step_input memory recall traces found")
    run_id = field(step_traces[-1], "RunID", "runId")
    run_traces = [item for item in traces if field(item, "RunID", "runId") == run_id]

run_memories = [item for item in memories if field(item, "RunID", "runId") == run_id]
high_value_types = {
    "decision",
    "dod_feedback",
    "execution_lesson",
    "project_context",
    "skill_feedback",
    "tool_feedback",
}
high_value_memories = [
    item for item in run_memories
    if field(item, "MemoryType", "memoryType") in high_value_types
]
if not high_value_memories:
    raise SystemExit(f"run {run_id} has no high-value memory items")

step_traces = [
    item for item in run_traces
    if field(item, "Source", "source") == "step_input"
]
if not step_traces:
    raise SystemExit(f"run {run_id} has no step_input memory recall traces")

dynamic_traces = [
    item for item in step_traces
    if field(item, "Policy", "policy") == "step_input_dynamic_workspace_memory"
]
if not dynamic_traces:
    raise SystemExit(f"run {run_id} has no dynamic step_input memory recall traces")

selected_high_value = []
for trace in dynamic_traces:
    selected = set(field(trace, "SelectedMemoryIDs", "selectedMemoryIds") or field(trace, "RetrievedMemoryIDs", "retrievedMemoryIds") or [])
    for memory in high_value_memories:
        if field(memory, "ID", "id") in selected:
            selected_high_value.append(field(memory, "ID", "id"))

if not selected_high_value:
    raise SystemExit(f"run {run_id} high-value memories were not selected by dynamic step_input recall")

structured = [
    item for item in dynamic_traces
    if field(item, "CandidateMemoryIDs", "candidateMemoryIds") is not None
    and field(item, "SelectedMemoryIDs", "selectedMemoryIds") is not None
    and field(item, "TypeQuota", "typeQuota") is not None
    and field(item, "SelectionScores", "selectionScores") is not None
]
if not structured:
    raise SystemExit(f"run {run_id} dynamic recall traces lack structured selection evidence")

print("workspace memory decision recall verification passed")
print(f"workspaceId={workspace_id}")
print(f"runId={run_id}")
print(f"highValueMemoryCount={len(high_value_memories)}")
print(f"dynamicTraceCount={len(dynamic_traces)}")
print("selectedHighValueMemoryIds=" + ",".join(sorted(set(selected_high_value))))
PY
