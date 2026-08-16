#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
source "$ROOT_DIR/scripts/lib/go-test-must-run.sh"

echo "-> AG-E2E-01/02/03/04/05/06/07: auto governance ticket/approval/doD/budget policy"
go test ./internal/modules/kernel/governance/biz -count=1

echo "-> Governance owners: typed ticket/approval transitions and idempotency"
run_go_test ./internal/modules/blocks/decision/biz \
  -run '^(TestService_RuntimeGovernanceProposalStaysOpenUntilFinalize|TestService_RuntimeGovernanceCommittedProposalReplaysExactResolution|TestService_RuntimeGovernanceRejectsUnallowedActionBeforeProposal)$' \
  -count=1
run_go_test ./internal/modules/blocks/approval/biz \
  -run '^(TestService_EnsureApproval_IsIdempotentAndRejectsBindingMismatch|TestService_EnsureApprovalResolution_IsIdempotentAndRejectsOppositeTerminalState)$' \
  -count=1

echo "-> SM-2/SM-4: committed governance facts drive only Orchestrator-owned continuation"
run_go_test ./internal/modules/kernel/orchestrator/biz \
  -run '^(TestRuntimeFileBackedOperationGovernanceGrantFreshProcessCrashRestart|TestRuntimeGovernanceDependencyPreservesSuspendedOperationBinding|TestRuntimeGovernanceDependencyRejectsGenericFailure)$' \
  -count=1

echo "-> AG-E2E-08: Web read model includes auto governance summary"
run_go_test ./internal/modules/application/workorchd/biz/appservice \
  -run '^(TestReadDecisionWithBackendIncludesAutoGovernanceSummary|TestReadApprovalWithBackendIncludesAutoGovernanceSummary)$' \
  -count=1

echo "-> AG-E2E-09: TUI shows compact auto governance status"
run_go_test ./internal/services/workorch/service/tui \
  -run '^(TestSnapshotGovernanceFiltersCurrentDisplayRun|TestModelDecisionResolvedRefreshesWorkspaceState|TestModelApprovalResolvedRefreshesWorkspaceState)$' \
  -count=1

echo "-> HTTP application: resolutions do not invoke legacy Run resume"
run_go_test ./internal/modules/application/workorchd/service/server \
  -run '^(TestDecisionResolveEndpointResolvesTicketWithoutLegacyRunResume|TestApprovalApproveEndpointApprovesApprovalWithoutLegacyRunResume|TestApprovalRejectEndpointRejectsApprovalWithoutLegacyRunResume)$' \
  -count=1

echo "auto governance ticket/approval verification passed"
