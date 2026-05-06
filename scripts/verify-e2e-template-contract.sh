#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

python3 - <<'PY'
from pathlib import Path
import re
import sys

errors = []
scripts = sorted(Path("scripts").glob("verify*.sh"))

for script in scripts:
    text = script.read_text(encoding="utf-8")
    lines = text.splitlines()
    for index, line in enumerate(lines):
        creates_workspace = (
            "/api/v1/workorch/workspaces" in line
            and "/api/v1/workorch/workspaces/" not in line
            and (
                "POST" in line
                or re.search(r"\bpost_json(_file)?\b", line)
            )
        )
        if not creates_workspace:
            continue
        context = "\n".join(lines[index:index + 12])
        if "templateId" in context:
            continue
        generated_payload_has_template = (
            "workspace-request.json" in context and '"templateId": "weather-query"' in text
        ) or (
            "workspace-create.json" in context and '"templateId": template_id' in text
        )
        if generated_payload_has_template:
            continue
        errors.append(f"{script}:{index + 1}: workspace creation payload must include templateId")

if errors:
    print("e2e template contract failed:", file=sys.stderr)
    for item in errors:
        print(f"- {item}", file=sys.stderr)
    sys.exit(1)

print("e2e template contract passed")
PY
