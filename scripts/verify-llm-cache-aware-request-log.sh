#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKSPACE_ID="${1:-${WORKORCH_VERIFY_WORKSPACE_ID:-}}"
DATA_DIR="${WORKORCH_DATA_DIR:-$ROOT_DIR/.workorch-data}"

if [[ -z "$WORKSPACE_ID" ]]; then
  echo "usage: $0 <workspace-id>" >&2
  echo "or set WORKORCH_VERIFY_WORKSPACE_ID" >&2
  exit 2
fi

WORKSPACE_DIR="$DATA_DIR/internal/workspaces/$WORKSPACE_ID"
if [[ ! -d "$WORKSPACE_DIR" ]]; then
  echo "workspace data directory not found: $WORKSPACE_DIR" >&2
  exit 2
fi

python3 - "$WORKSPACE_DIR" <<'PY'
import glob
import json
import os
import sys

workspace_dir = sys.argv[1]
request_paths = sorted(glob.glob(os.path.join(workspace_dir, "runs", "*", "llm-calls", "*", "*", "request.json")))
if not request_paths:
    raise SystemExit(f"no llm request logs found under {workspace_dir}")

checked = 0
errors = []
prev = None
for path in request_paths:
    with open(path, "r", encoding="utf-8") as fh:
        req = json.load(fh)
    prompt_context = req.get("promptContext") or {}
    sections = prompt_context.get("sections") or []
    turns = prompt_context.get("turns") or []
    budget = req.get("contextBudget") or {}
    cache_plan = budget.get("cachePlan") or {}
    rel = os.path.relpath(path, workspace_dir)
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

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)

print(f"llm cache-aware request log verification passed workspace={os.path.basename(workspace_dir)} requests={checked}")
PY
