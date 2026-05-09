#!/usr/bin/env bash

set -euo pipefail

INTERACTIONS_DIR="${WORKORCH_INTERACTIONS_DIR:-/home/rangh/aispace/ai-proxy/interactions}"
SAMPLE_LIMIT="${WORKORCH_INTERACTIONS_SAMPLE_LIMIT:-20}"
STRICT="${WORKORCH_INTERACTIONS_STRICT:-0}"

if [[ ! -d "$INTERACTIONS_DIR" ]]; then
  echo "warning: interactions directory not found: $INTERACTIONS_DIR" >&2
  if [[ "$STRICT" == "1" ]]; then
    exit 2
  fi
  exit 0
fi

python3 - "$INTERACTIONS_DIR" "$SAMPLE_LIMIT" "$STRICT" <<'PY'
import json
import os
import sys
from pathlib import Path

root = Path(sys.argv[1])
sample_limit = max(1, int(sys.argv[2]))
strict = sys.argv[3] == "1"

records = []
for child in sorted(root.iterdir()):
    if not child.is_dir():
        continue
    request_path = child / "request.json"
    upstream_path = child / "upstream_request.json"
    if not request_path.exists():
        continue
    try:
        request = json.loads(request_path.read_text(encoding="utf-8"))
    except Exception:
        continue
    upstream = {}
    if upstream_path.exists():
        try:
            upstream = json.loads(upstream_path.read_text(encoding="utf-8"))
        except Exception:
            upstream = {}
    url = str(upstream.get("url") or "")
    body_bytes = int(upstream.get("body_bytes") or 0)
    records.append({
        "id": child.name,
        "request": request,
        "url": url,
        "body_bytes": body_bytes,
    })

if not records:
    print("no interaction records found", file=sys.stderr)
    raise SystemExit(1)

codex_records = []
workorch_records = []
for item in records:
    req = item["request"]
    url = item["url"]
    if "input" in req and isinstance(req.get("input"), list):
        codex_records.append(item)
        continue
    if "messages" in req and isinstance(req.get("messages"), list):
        if "/chat/completions" in url or req.get("response_format"):
            workorch_records.append(item)

if not workorch_records:
    print("no workorch chat-completions interactions found", file=sys.stderr)
    raise SystemExit(1)

sample = workorch_records[-sample_limit:]
new_style = []
old_style = []
errors = []

for item in sample:
    req = item["request"]
    messages = req.get("messages") or []
    roles = [str(m.get("role") or "") for m in messages if isinstance(m, dict)]
    system_count = sum(1 for role in roles if role == "system")
    merged = "\n".join(str(m.get("content") or "") for m in messages if isinstance(m, dict))
    is_new = (
        len(messages) >= 3
        and system_count == 1
        and "user" in roles
        and "Original Goal Contract (Do not rewrite or compress)" in merged
        and "Original DoD Contract (Do not rewrite or compress)" in merged
        and "Effective Goal and DoD View" in merged
    )
    if is_new:
        new_style.append(item)
    else:
        old_style.append(item)
        errors.append({
            "id": item["id"],
            "messageCount": len(messages),
            "roles": roles,
            "systemCount": system_count,
            "hasGoalMarker": "Original Goal Contract (Do not rewrite or compress)" in merged,
            "hasDoDMarker": "Original DoD Contract (Do not rewrite or compress)" in merged,
            "hasEffectiveMarker": "Effective Goal and DoD View" in merged,
        })

print(f"interaction root: {root}")
print(f"codex records: {len(codex_records)}")
print(f"workorch records: {len(workorch_records)}")
print(f"sample size: {len(sample)}")
print(f"new-style workorch prompt records: {len(new_style)}")
print(f"legacy-style workorch prompt records: {len(old_style)}")

if codex_records:
    codex_body = [item["body_bytes"] for item in codex_records if item["body_bytes"] > 0]
    if codex_body:
        print(f"codex body_bytes avg: {sum(codex_body)//len(codex_body)}")
if workorch_records:
    workorch_body = [item["body_bytes"] for item in workorch_records if item["body_bytes"] > 0]
    if workorch_body:
        print(f"workorch body_bytes avg: {sum(workorch_body)//len(workorch_body)}")

if strict and old_style:
    print("strict mode failed: legacy style records found in sample", file=sys.stderr)
    for item in errors[:10]:
        print(json.dumps(item, ensure_ascii=False), file=sys.stderr)
    raise SystemExit(1)

if not new_style:
    print("warning: no new-style codex-like workorch prompt records found in sample", file=sys.stderr)
    for item in errors[:10]:
        print(json.dumps(item, ensure_ascii=False), file=sys.stderr)
    if strict:
        raise SystemExit(1)
    print("tip: rerun after generating new interactions or set WORKORCH_INTERACTIONS_STRICT=1 for strict gating")
    raise SystemExit(0)

print("codex-like prompt interaction verification passed")
PY
