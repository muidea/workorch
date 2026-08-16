#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/workorch-go-build}"
export GOCACHE

echo "=== governance owner resolution and Runtime continuation verification ==="

echo "-> Decision and Approval owners"
go test ./internal/modules/blocks/decision/... ./internal/modules/blocks/approval/... -count=1

echo "-> typed EventHub resolution, suspended dispatch binding and restart replay"
go test ./internal/modules/kernel/orchestrator/biz \
  -run 'TestRuntime(GovernanceResolutionCommitsThroughTypedEventHubCommand|GovernanceResolutionFileBackedRestartReplaysExactlyOnce|FileBackedBoundGovernanceRestartPreservesBindingWithoutCandidateCall|GovernanceDependencyPreservesSuspendedOperationBinding)$' \
  -count=1

echo "-> HTTP resolves governance owner records without a Run lifecycle mutation"
go test ./internal/modules/application/workorchd/service/server \
  -run 'Test(DecisionResolveEndpointResolvesTicketWithoutLegacyRunResume|DecisionResolveEndpointRejectCurrentPathWithoutLegacyRunResume|ApprovalApproveEndpointApprovesApprovalWithoutLegacyRunResume|ApprovalRejectEndpointRejectsApprovalWithoutLegacyRunResume|WorkorchDUnsupportedMutatingAPINativeRoutesNotRegistered)$' \
  -count=1

echo "governance owner resolution verification passed"
