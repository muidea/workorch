#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/workorch-go-build}"
export GOCACHE

echo "=== software requirement Runtime orchestration verification ==="

"$ROOT_DIR/scripts/verify-software-dev-code-agent.sh"
"$ROOT_DIR/scripts/verify-software-dev-goal-dod-e2e.sh"

echo "-> read models and HTTP activation remain non-authoritative"
go test ./internal/modules/application/workorchd/biz/appservice/... -count=1
go test ./internal/modules/application/workorchd/service/server \
  -run 'Test(WorkspaceExecutionActivationEndpointCreatesAndStartsRun|WorkorchDActivationHTTPProjectsRequestID|WorkorchDActivationHTTPReturnsWhileSchedulerIsStillRunning|WorkorchDAPINativeRoutesRegistered|WorkorchDUnsupportedMutatingAPINativeRoutesNotRegistered)$' \
  -count=1

echo "software requirement Runtime orchestration verification passed"
