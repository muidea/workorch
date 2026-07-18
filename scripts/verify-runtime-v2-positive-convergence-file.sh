#!/usr/bin/env bash
# RVC-R15: deterministic file-backed Runtime V2 positive-convergence verifier.
# Uses a real file-backed Run repository plus the concrete File owner, alongside
# focused contract/crash fixtures; no public LLM.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ARTIFACT_DIR="${WORKORCH_E2E_ARTIFACT_DIR:-/tmp/workorch-e2e/runtime-v2-positive-convergence/$(date -u +%Y%m%dT%H%M%SZ)}"
mkdir -p "$ARTIFACT_DIR"
LOG="$ARTIFACT_DIR/verify.log"

echo "runtime-v2 positive-convergence file verifier" | tee "$LOG"
echo "artifacts=$ARTIFACT_DIR" | tee -a "$LOG"

run_pkg() {
  local pkg="$1"
  shift
  echo "==> go test $pkg $*" | tee -a "$LOG"
  if ! go test "$pkg" "$@" -count=1 2>&1 | tee -a "$LOG"; then
    echo "FAIL: $pkg $*" | tee -a "$LOG"
    echo "artifacts retained at $ARTIFACT_DIR" >&2
    exit 1
  fi
}

# RVC-R5/R6/R7/R8 contract gates: effect scope, seal, repair directive, MCP normalize.
run_pkg ./internal/pkg/workorchruntimecontract \
  -run 'TestRuntimeEffectScope|TestValidateEffectScopes|TestRequiredOutputRequiresExplicitEffectScopes|TestRuntimeOperationDescriptorEffectScope|TestRuntimeOperationExecutionRecord|TestRuntimeRepairDirective|TestRuntimeTermination|TestFailureClassDisposition|TestResolveExpectedEffectScope|TestResolveOperationIdempotencyClass|TestSealRejects|TestSealAccepts|TestSealMCPCapability|TestSyncRepairDirective|TestApplyRepairDirective|TestDeriveRuntimeRepairHintsObeys|TestRuntimeCandidateContextFromFactsProjects|TestClearRepairDirective|TestRuntimeOperationDescriptorsAreStableCompleteAndUnique|TestFeasibleOperationKindsIntersectAcceptedCapabilitiesAndEffect|TestValidateAssessmentDispositionRequiresExplicitAuthorityAndValidatesPartial|TestRuntimeRepairLedgerBeginCycleIsIdempotentAndBudgeted|TestMCPRegistryKindNormalizes|TestRuntimeOperationGovernanceGrant|TestRuntimeRecoveryBudgetsAreIndependentAndBounded'

# RVC-R9/R10 seal executions + evidence adjudication + claim/fence path via controller fixtures.
run_pkg ./internal/modules/kernel/orchestrator/biz/runtimev2 \
  -run 'TestAdjudicateEvidenceKind|TestSealDispatchCreatesOperationExecution|TestPositiveConvergenceFixture_|TestReduceRuntimeStateRepairableAssessmentDoesNotTerminal|TestReduceRuntimeStateSameFocusRepairThenReassess|TestReduceRuntimeStateRepairBudgetExhausted|TestReduceRuntimeStateValidationOnlyCapEscalatesToStateChange|TestInvalidateNonSatisfiedAssessmentsForFocus|TestRuntimeControllerOwnsDispatch|TestRuntimeControllerCommitsTypedAssessment|TestRuntimeControllerCommitsGovernanceContinue|TestRecordOperationGovernanceRequiredDoesNotUseExecutionRepair|TestRuntimeControllerRecoversOwnerCommittedReceiptWithoutReplayingSideEffect|TestRuntimeControllerRecoversReceiptWhenOwnerResponseIsLostAfterCommit|TestRuntimeControllerTurnsClaimedWithoutReceiptIntoOutcomeUnknownWithoutReplay|TestRuntimeControllerReplaysClaimedSafeOperationWithinRecoveryBudget|TestRuntimeControllerOperationReceiptFenceAndTerminalUniqueness|TestRuntimeControllerRoutesDurableNoOpReceiptToStructuredSameFocusRepair|TestRuntimeControllerPersistsIndependentRecoveryBudget|TestReduceRuntimeStateRoutesExhaustedProviderTransportToGovernance'

# RVC-R14 Run-owner typed startup inventory.
run_pkg ./internal/modules/kernel/run \
  -run 'TestBuildRuntimeV2RecoveryInventoryClassifiesAuthorityState'

# Concrete File owner consumes only an already validated typed governance grant.
run_pkg ./internal/modules/blocks/file/... \
  -run 'TestRuntimeAdapterPatchApplyConsumesValidatedApprovalGrant'

# Candidate attempt 0/1, restart idempotent drive path, feasibility.
run_pkg ./internal/modules/kernel/orchestrator \
  -run 'TestDriveRuntimeV2FileBackedRunExecutesDurableFileOwnerAndRestarts|TestDriveRuntimeV2FileBackedRestartRecoversCrashAfterOwnerReceipt|TestRuntimeV2FileBackedClaimEvidenceAndDispatchCloseCrashMatrix|TestDriveRuntimeV2FileBackedTwoFocusStateValidationAssessmentAndRestart|TestDriveRuntimeV2CompletesTypedPathAndIsRestartIdempotent|TestDriveRuntimeV2CandidateFailureStopsAfterSameContractRetry|TestRuntimeV2EnsureCandidateFeasibleBlocksProviderWithoutKinds|TestRuntimeV2GovernanceDependencyPreservesSuspendedOperationBinding|TestRuntimeGovernanceResolutionCommitsThroughTypedEventHubCommand'

# Continuation / unresolved rehydrate.
run_pkg ./internal/modules/application/workorchd/biz/appservice/activationcore \
  -run 'TestCompileRuntimeV2CoverageRehydratesUnresolvedContinuation|TestCompileRuntimeV2CoverageGapAnalysisWhenContinuationLacksStructuredGaps|TestCompileRuntimeV2CoverageSlicesBoundLargeDoD'

# Projection generation recovery / ready.
run_pkg ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore \
  -run 'TestStoreProjectionGenerationRecovery|TestStoreDeletedWorkspaceClearsGenerationFailure|TestStoreRestartDoesNotCarryGenerationFailures|TestManagerMetricsDoNotDriveProjectionHealth'

# Usage hydrate + terminal release.
run_pkg ./internal/modules/kernel/orchestrator/biz \
  -run 'TestProjectRuntimeV2CompleteBypassesLegacyResultGate|TestProjectRuntimeV2TerminalConvergesAfterRunUpdateFailure|TestProjectRuntimeV2TerminalDoesNotStealLiveActiveRun'

run_pkg ./internal/modules/application/workorchd/biz/appservice/readmodelports \
  -run 'TestRunObservabilityHydratesLLMUsagePayloads|TestRunSnapshotBuildsSnapshotFromReaders'

echo "runtime-v2 positive-convergence file verification passed" | tee -a "$LOG"
echo "artifacts=$ARTIFACT_DIR" | tee -a "$LOG"
