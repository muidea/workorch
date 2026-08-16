#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/workorch-go-build}"
export GOCACHE

echo "=== file-backed Runtime restart and owner re-drive verification ==="

echo "-> Run Owner publishes opaque recovery inventory and retains current aggregate"
go test ./internal/modules/blocks/run/... \
  -run 'Test(BuildRuntimeRecoveryInventoryContainsOnlyRunOwnedOpaqueFacts|GMMR1SM4RecoveryInventoryAvailableAfterSetupBeforeRun|ReplacementTerminationStartupRecoveryKeepsWorkspaceScope|WorkspaceExecutionResumeRetainsCurrentRunOpaqueCWILedger)$' \
  -count=1

echo "-> Orchestrator waits for owner readiness and schedules only active aggregates"
go test ./internal/modules/kernel/orchestrator/biz \
  -run 'Test(GMMR1SM4OrchestratorStartFailsWithoutRecoveryInventory|GMMR1SM4StartupRecoverySchedulesBootstrapOnlyActiveRun|GMMR3StartupRecoveryWaitsForRunCommandsReady|RuntimeStartupRecoveryDeliversPendingTerminalThroughOwnerChain)$' \
  -count=1

echo "-> workspace restart ingress remains activation-based"
go test ./internal/modules/application/workorchd/service/server \
  -run 'Test(WorkorchDWorkspaceRestartHTTPProjectsAcceptedSemanticPlanningActivation|WorkorchDUnsupportedMutatingAPINativeRoutesNotRegistered)$' \
  -count=1

echo "file-backed Runtime restart verification passed"
