#!/usr/bin/env bash

# Weather is profile/capability data; governance transitions remain owned by
# Decision, Approval and Orchestrator. This deterministic gate verifies those
# boundaries without teaching a mock provider a second execution protocol.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

go test ./internal/pkg/workorchdefaults -count=1
go test ./internal/modules/blocks/mcp/biz \
  -run 'Test(MCPOwnerSemanticBehaviorProbeMatrix|MCPOwnerEmitsStructuredFailureTargetObservationThroughTypedReceipt|MCPOwnerEmitsExactToolValidationObservationThroughTypedReceipt)$' \
  -count=1
"$ROOT_DIR/scripts/verify-approval-decision-resolution-e2e.sh"

echo "weather capability governance verification passed"
