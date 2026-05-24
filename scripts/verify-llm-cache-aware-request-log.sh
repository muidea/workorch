#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKSPACE_ID="${1:-${WORKORCH_VERIFY_WORKSPACE_ID:-}}"
RUN_FILTER="${2:-${WORKORCH_VERIFY_RUN_ID:-}}"
DATA_DIR="${WORKORCH_DATA_DIR:-$ROOT_DIR/.workorch-data}"
C4_SECTION_MAX_BYTES="${WORKORCH_CACHE_VERIFY_C4_SECTION_MAX_BYTES:-262144}"
C4_TOTAL_MAX_BYTES="${WORKORCH_CACHE_VERIFY_C4_TOTAL_MAX_BYTES:-393216}"
NON_CACHEABLE_MAX_RATIO="${WORKORCH_CACHE_VERIFY_NON_CACHEABLE_MAX_RATIO:-0.80}"

if [[ -z "$WORKSPACE_ID" ]]; then
  echo "usage: $0 <workspace-id> [run-id]" >&2
  echo "or set WORKORCH_VERIFY_WORKSPACE_ID and optional WORKORCH_VERIFY_RUN_ID" >&2
  exit 2
fi

WORKSPACE_DIR="$DATA_DIR/internal/workspaces/$WORKSPACE_ID"
if [[ ! -d "$WORKSPACE_DIR" ]]; then
  echo "workspace data directory not found: $WORKSPACE_DIR" >&2
  exit 2
fi

python3 - "$WORKSPACE_DIR" "$RUN_FILTER" "$C4_SECTION_MAX_BYTES" "$C4_TOTAL_MAX_BYTES" "$NON_CACHEABLE_MAX_RATIO" <<'PY'
import glob
import json
import os
import sys

workspace_dir = sys.argv[1]
run_filter = sys.argv[2].strip()
c4_section_max_bytes = int(sys.argv[3])
c4_total_max_bytes = int(sys.argv[4])
non_cacheable_max_ratio = float(sys.argv[5])
request_paths = sorted(glob.glob(os.path.join(workspace_dir, "runs", "*", "llm-calls", "*", "*", "request.json")))
if run_filter:
    request_paths = [path for path in request_paths if f"{os.sep}runs{os.sep}{run_filter}{os.sep}" in path]
if not request_paths:
    raise SystemExit(f"no llm request logs found under {workspace_dir}")

checked = 0
errors = []
warnings = []
prev = None
run_audit = {}
for path in request_paths:
    with open(path, "r", encoding="utf-8") as fh:
        req = json.load(fh)
    prompt_context = req.get("promptContext") or {}
    sections = prompt_context.get("sections") or []
    turns = prompt_context.get("turns") or []
    budget = req.get("contextBudget") or {}
    cache_plan = budget.get("cachePlan") or {}
    rel = os.path.relpath(path, workspace_dir)
    run_id = req.get("runId") or rel.split(os.sep)[1]
    if not sections:
        errors.append(f"{rel}: promptContext.sections missing")
        continue
    if not turns:
        errors.append(f"{rel}: promptContext.turns missing")
        continue
    if not cache_plan:
        errors.append(f"{rel}: contextBudget.cachePlan missing")
        continue
    section_ids = [item.get("id") for item in sections]
    turn_section_ids = [item.get("sectionId") for item in turns]
    if section_ids != turn_section_ids:
        errors.append(f"{rel}: section order does not match turn sectionIds")
    for required in ("stablePrefixHash", "workspaceDefinitionHash", "capabilityContractHash", "volatileTailHash"):
        if not cache_plan.get(required):
            errors.append(f"{rel}: cachePlan.{required} missing")
    adapter_hint = cache_plan.get("adapterHint") or {}
    if not adapter_hint.get("providerMapping"):
        errors.append(f"{rel}: cachePlan.adapterHint.providerMapping missing")
    if not any(str(item).startswith("C0") for item in section_ids):
        errors.append(f"{rel}: C0 section missing")
    if not any(str(item).startswith("C1") for item in section_ids):
        errors.append(f"{rel}: C1 section missing")
    if not any(str(item).startswith("C4") for item in section_ids):
        errors.append(f"{rel}: C4 section missing")
    section_hashes = cache_plan.get("sectionHashes") or []
    hashed_ids = [item.get("sectionId") for item in section_hashes]
    missing_hashes = [item for item in section_ids if item not in hashed_ids]
    if missing_hashes:
        errors.append(f"{rel}: sectionHashes missing {missing_hashes}")
    c4_total = 0
    for section in sections:
        section_id = str(section.get("id") or "")
        if not section_id.startswith("C4"):
            continue
        size = len(str(section.get("content") or "").strip().encode("utf-8"))
        c4_total += size
        if size > c4_section_max_bytes:
            warnings.append(f"{rel}: C4 section {section_id} is {size} bytes, exceeds {c4_section_max_bytes}")
    if c4_total > c4_total_max_bytes:
        warnings.append(f"{rel}: C4 volatile tail is {c4_total} bytes, exceeds {c4_total_max_bytes}")
    usage_path = os.path.join(os.path.dirname(path), "usage.json")
    if os.path.exists(usage_path):
        try:
            with open(usage_path, "r", encoding="utf-8") as usage_fh:
                usage = json.load(usage_fh)
            cache_usage = usage.get("cacheUsage") or {}
            input_tokens = int(usage.get("inputTokens") or 0)
            non_cacheable = int(cache_usage.get("nonCacheableInputTokens") or 0)
            if input_tokens > 0 and non_cacheable / input_tokens > non_cacheable_max_ratio:
                warnings.append(
                    f"{rel}: nonCacheableInputTokens ratio {non_cacheable / input_tokens:.2f} exceeds {non_cacheable_max_ratio:.2f}"
                )
        except Exception as exc:
            warnings.append(f"{rel}: unable to parse usage.json for cache quality audit: {exc}")
    audit = run_audit.setdefault(run_id, {
        "stablePrefixHashes": set(),
        "workspaceDefinitionHashes": set(),
        "capabilityContractHashes": set(),
        "cacheKeys": set(),
    })
    if cache_plan.get("stablePrefixHash"):
        audit["stablePrefixHashes"].add(cache_plan.get("stablePrefixHash"))
    if cache_plan.get("workspaceDefinitionHash"):
        audit["workspaceDefinitionHashes"].add(cache_plan.get("workspaceDefinitionHash"))
    if cache_plan.get("capabilityContractHash"):
        audit["capabilityContractHashes"].add(cache_plan.get("capabilityContractHash"))
    if cache_plan.get("cacheKey"):
        audit["cacheKeys"].add(cache_plan.get("cacheKey"))
    if prev:
        if prev["workspaceDefinitionHash"] != cache_plan.get("workspaceDefinitionHash"):
            breakers = cache_plan.get("cacheBreakers") or []
            if not any((item.get("sectionId") == "C1" or str(item.get("sectionId", "")).startswith("C1")) for item in breakers):
                errors.append(f"{rel}: C1 hash changed without C1 cacheBreaker")
    prev = {
        "workspaceDefinitionHash": cache_plan.get("workspaceDefinitionHash"),
        "stablePrefixHash": cache_plan.get("stablePrefixHash"),
    }
    checked += 1

for run_id, audit in sorted(run_audit.items()):
    if len(audit["stablePrefixHashes"]) > 1:
        errors.append(f"{run_id}: stablePrefixHash changed within one run: {sorted(audit['stablePrefixHashes'])}")
    if len(audit["cacheKeys"]) > 4:
        warnings.append(f"{run_id}: cacheKey split into {len(audit['cacheKeys'])} groups; inspect C1/C2/C4 churn")
    if len(audit["capabilityContractHashes"]) > 3:
        warnings.append(f"{run_id}: capabilityContractHash split into {len(audit['capabilityContractHashes'])} groups; inspect dynamic capability/governance content in C2")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)

for item in warnings:
    print("warning: " + item, file=sys.stderr)

print(f"llm cache-aware request log verification passed workspace={os.path.basename(workspace_dir)} requests={checked}")
PY
