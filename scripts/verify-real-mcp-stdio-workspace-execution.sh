#!/usr/bin/env bash

# Compatibility entry. The supported MCP stdio gate uses a managed isolated
# service, a current semantic-plan/operations provider fixture and real stdio
# transport through the MCP Owner.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export WORKORCH_BASE_URL="${WORKORCH_REAL_MCP_STDIO_BASE_URL:-${WORKORCH_BASE_URL:-}}"
export WORKORCH_MCP_STDIO_VERIFY_MOCK_ADDR="${WORKORCH_MCP_STDIO_VERIFY_MOCK_ADDR:-${WORKORCH_REAL_MCP_STDIO_MOCK_ADDR:-127.0.0.1:18188}}"
export WORKORCH_MCP_STDIO_VERIFY_ARTIFACT_DIR="${WORKORCH_MCP_STDIO_VERIFY_ARTIFACT_DIR:-${WORKORCH_REAL_MCP_STDIO_ARTIFACT_DIR:-}}"

exec "$ROOT_DIR/scripts/verify-mcp-stdio-workspace-execution.sh" "$@"
