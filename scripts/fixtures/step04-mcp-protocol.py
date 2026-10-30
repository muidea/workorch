#!/usr/bin/env python3
"""Isolated protocol-only MCP fixture: no business operation is accepted."""
import json
import os
from pathlib import Path
import sys

root = Path(os.environ["WORKORCH_STEP04_FIXTURE_ROOT"])
for line in sys.stdin:
    request = json.loads(line)
    with (root / "mcp-protocol-calls.jsonl").open("a", encoding="utf-8") as stream:
        stream.write(json.dumps(request, ensure_ascii=False) + "\n")
    method = request["method"]
    if method == "notifications/initialized":
        continue
    if method == "initialize":
        result = {"protocolVersion": "2025-11-25", "capabilities": {"tools": {}}, "serverInfo": {"name": "step04-fixture", "version": "1"}}
    elif method == "tools/list":
        # The integration fullflow uses the same protocol-only transport with
        # its exact owner allowlist. No business operation is implemented.
        names = json.loads(os.environ.get("WORKORCH_STEP04_FIXTURE_TOOLS", '["inspect"]'))
        if not isinstance(names, list) or not names or len(names) != len(set(names)) or not all(isinstance(n, str) and n for n in names):
            raise RuntimeError("invalid fixture tool inventory")
        result = {"tools": [{"name": name, "description": "protocol fixture", "inputSchema": {"type": "object"}} for name in names]}
    else:
        raise RuntimeError("preparation invoked a business method: " + method)
    print(json.dumps({"jsonrpc": "2.0", "id": request["id"], "result": result}), flush=True)
