#!/usr/bin/env bash
# RVC-R15: deterministic file-backed Runtime V2 positive-convergence verifier.
# Uses a real file-backed Run repository plus the concrete File owner, alongside
# focused contract/crash fixtures; no public LLM.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ARTIFACT_DIR="$(realpath -m "${WORKORCH_E2E_ARTIFACT_DIR:-${TMPDIR:-/tmp}/workorch-e2e/runtime-v2-positive-convergence/$(date -u +%Y%m%dT%H%M%SZ)}")"
REPOSITORY_E2E_ARTIFACT_DIR="$(realpath -m "$ROOT_DIR/.workorch-e2e")"
if [[ "$ARTIFACT_DIR" == "$REPOSITORY_E2E_ARTIFACT_DIR" || "$ARTIFACT_DIR" == "$REPOSITORY_E2E_ARTIFACT_DIR/"* ]]; then
  echo "WORKORCH_E2E_ARTIFACT_DIR must not point to the repository .workorch-e2e directory" >&2
  exit 2
fi
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
  -run 'TestRuntimeEffectScope|TestValidateEffectScopes|TestRequiredOutputRequiresExplicitEffectScopes|TestRuntimeOperationDescriptorEffectScope|TestRuntimeOperationExecutionRecord|TestRuntimeRepairDirective|TestRuntimeTermination|TestFailureClassDisposition|TestResolveExpectedEffectScope|TestResolveOperationIdempotencyClass|TestSealRejects|TestSealAccepts|TestSealMCPCapability|TestSyncRepairDirective|TestApplyRepairDirective|TestDeriveRuntimeRepairHintsObeys|TestRuntimeCandidateContextFromFactsProjects|TestClearRepairDirective|TestRuntimeOperationDescriptorsAreStableCompleteAndUnique|TestFeasibleOperationKindsIntersectAcceptedCapabilitiesAndEffect|TestValidateAssessmentDispositionRequiresExplicitAuthorityAndValidatesPartial|TestRuntimeRepairLedgerBeginCycleIsIdempotentAndBudgeted|TestMCPRegistryKindNormalizes|TestRuntimeOperationGovernanceGrant|TestRuntimeRecoveryBudgetsAreIndependentAndBounded|TestRuntimeOperationContractVariants|TestRuntimeCandidateRequiresAcceptedOperationWhenDeclared'

# RVC-R9/R10 seal executions + evidence adjudication + claim/fence path via controller fixtures.
run_pkg ./internal/modules/kernel/orchestrator/biz/runtimev2 \
  -run 'TestAdjudicateEvidenceKind|TestSealDispatchCreatesOperationExecution|TestPositiveConvergenceFixture_|TestEvaluateCandidateAdmission|TestActiveRejectedOperationShapes|TestAdmitCandidateProposalAtomicallyClosesReplayAttemptAndExhaustsSecondAttempt|TestAdmitCandidateProposalIdentifiesInvalidDuplicateByCandidateStructure|TestAdmitAndSealCandidateAttemptKeepsRejectedAndAcceptedDuplicateDeliveriesIdempotent|TestRuntimeAggregateRejectsTerminalDecisionWithPendingCandidateAttempt|TestRuntimeAggregateRequiresReducerTransitionAfterCandidateAttemptOneFailure|TestReduceRuntimeStateDeterminedTerminalFailureOverridesFreshnessReacquisition|TestRuntimeControllerCandidateRetryExhaustionClosesAtomicallyWithFreshnessDebt|TestRuntimeControllerTerminalDriveFailureClosesPendingCandidateWithFreshnessDebt|TestReduceRuntimeStateRepairableAssessmentDoesNotTerminal|TestReduceRuntimeStateSameFocusRepairThenReassess|TestReduceRuntimeStateRepairBudgetExhausted|TestReduceRuntimeStateValidationOnlyCapEscalatesToStateChange|TestInvalidateNonSatisfiedAssessmentsForFocus|TestRuntimeControllerOwnsDispatch|TestRuntimeControllerCommitsTypedAssessment|TestRuntimeControllerCommitsGovernanceContinue|TestRecordOperationGovernanceRequiredDoesNotUseExecutionRepair|TestRuntimeControllerRecoversOwnerCommittedReceiptWithoutReplayingSideEffect|TestRuntimeControllerRecoversReceiptWhenOwnerResponseIsLostAfterCommit|TestRuntimeControllerTurnsClaimedWithoutReceiptIntoOutcomeUnknownWithoutReplay|TestRuntimeControllerReplaysClaimedSafeOperationWithinRecoveryBudget|TestRuntimeControllerOperationReceiptFenceAndTerminalUniqueness|TestRuntimeControllerRoutesDurableNoOpReceiptToStructuredSameFocusRepair|TestRuntimeControllerPersistsIndependentRecoveryBudget|TestReduceRuntimeStateRoutesExhaustedProviderTransportToGovernance|TestCapabilityStatePrerequisiteRequiresOwnerConfirmedSnapshot|TestStaleCapabilityStateReceiptRefreshesPrerequisiteWithoutRepairDebt'

# VPC: receipt quality and effective-progress/no-progress replay remain part
# of the deterministic file gate, not only of an in-process unit suite.
run_pkg ./internal/modules/blocks/shell/biz \
  -run 'Test.*Validation|Test.*GoTest.*Observation|TestShellOwnerDoesNotInferFailureTargetFromValidationCommand'

# VPC-8 owner-observation boundary: MCP may bind an explicit structured
# failure target; Shell must leave the target absent until an equally typed
# external protocol exists. Neither owner may infer it from command input.
run_pkg ./internal/modules/kernel/orchestrator/biz/runtimev2 \
  -run 'TestMCPOwnerObservedFailureTargetBindsThroughEventHubAndFileSnapshot'

run_pkg ./internal/modules/kernel/orchestrator/biz/runtimev2 \
  -run 'TestAcceptedValidationEvidenceInvalidatesAssessmentWithoutResettingRepairLedger|TestAcceptedDurableStateChangeResetsRepairProgressWindow|TestRuntimeControllerRoutesDurableNoOpReceiptToStructuredSameFocusRepair|TestReduceRuntimeStateWithDirectiveSnapshotsBoundedOwnerWorkspaceScope|TestEvaluateCandidateAdmissionAllowsOwnerGroundedWorkspaceScopeThroughFileAdmissibility'

# RVC-R14 Run-owner typed startup inventory.
run_pkg ./internal/modules/kernel/run \
  -run 'TestBuildRuntimeV2RecoveryInventoryClassifiesAuthorityState'

# Concrete File owner consumes only an already validated typed governance grant.
run_pkg ./internal/modules/blocks/file/... \
  -run 'TestRuntimeAdapterPatchApply(ConsumesValidatedApprovalGrant|AllowsLargeWorkspaceLocalSourceChange)|TestSupportedRuntimeOperationVariantsMatchDescriptor|TestRuntimeAdapterAcquireRuntimeTargetSelectionIsCardinalityBounded'

# Only an owner-persisted governance receipt may suspend an operation for approval.
run_pkg ./internal/modules/kernel/orchestrator/biz \
  -run 'TestRuntimeOperationExecutionResultUsesOnlyPersistedReceiptGovernance'

run_pkg ./internal/modules/blocks/git/... \
  -run 'TestCapabilityStateReceiptStoreIsDurableAndIdempotent|TestSupportedRuntimeOperationVariantsMatchDescriptor|TestValidateGitCapabilityStatePredecessorReceiptsRequiresExactDurableBinding|TestGitOwnerCapabilityBoundAdmissibilityRejectsMismatchedProbeBindingAsStale'

# Candidate attempt 0/1, restart idempotent drive path, feasibility.
run_pkg ./internal/modules/kernel/orchestrator/biz \
	-run 'TestDriveRuntimeV2FileBackedRunExecutesDurableFileOwnerAndRestarts|TestRuntimeV2FileBackedGroundedTargetStateSequenceRestartsAtCommittedBoundaries|TestRuntimeV2FileBackedEmptyWorkspaceCreationSelectionContinuesThroughCandidate|TestRuntimeV2FileBackedEmptyWorkspaceCreationReceiptCrashRestartDoesNotReplayCreate|TestRuntimeV2FileBackedBoundedWorkspaceScopeAllowsNewDeliverable|TestRuntimeV2FileBackedAmbiguousTargetSelectionFailsWithoutProvider|TestRuntimeV2FileBackedMCPOwnerFailureTargetReceiptRestartMatrix|TestRuntimeV2FileBackedPatchReplayProofRestartsAfterOwnerReceiptCASFailure|TestRuntimeV2StartupRecoveryDeliversPendingTerminalThroughOwnerChain|TestDriveRuntimeV2FileBackedRestartRecoversCrashAfterOwnerReceipt|TestRuntimeV2FileBackedClaimEvidenceAndDispatchCloseCrashMatrix|TestRuntimeV2FileBackedGitCapabilityStateSequenceMatrix|TestRuntimeV2FileBackedGitRestartAfterInitEvidenceCommitContinuesSequence|TestRuntimeV2FileBackedGitQueryReceiptCrashBeforePrerequisiteCommitReusesSnapshot|TestRuntimeV2FileBackedGitSnapshotCommitCrashBeforeCandidateReusesCommittedSnapshot|TestRuntimeV2FileBackedGitStaleRefreshCrashRestartDoesNotReplayOldDispatch|TestRuntimeV2FileBackedPatchVariantMatrix|TestRuntimeV2FileBackedPatchVariantRestartAfterOwnerReceipt|TestDriveRuntimeV2FileBackedTwoFocusStateValidationAssessmentAndRestart|TestDriveRuntimeV2CompletesTypedPathAndIsRestartIdempotent|TestDriveRuntimeV2CandidateFailureStopsAfterSameContractRetry|TestRequestAndSealRuntimeV2CandidateContinuesProviderLimitedOutput|TestRequestAndSealRuntimeV2CandidateRestartsFromPersistedOutputChunk|TestRequestAndSealRuntimeV2CandidateFailsWhenOutputContinuationBudgetExhausted|TestRuntimeV2MainlinePartialReplayClosesAttemptThenSealsCorrectedRetry|TestRuntimeV2EnsureCandidateFeasibleBlocksProviderWithoutKinds|TestRuntimeV2GovernanceDependencyPreservesSuspendedOperationBinding|TestRuntimeGovernanceResolutionCommitsThroughTypedEventHubCommand|TestRuntimeV2StateRepositoryCreationSelectionCommitCrashRestartIsAtomic|TestStopWaitsForQueuedDecisionEvidenceHandlerBeforeClearingDependencies|TestStopWaitsForQueuedRuntimeOperationReceiptHandlerBeforeClearingDependencies|TestStopWaitsForQueuedRuntimeGovernanceResolutionHandlerBeforeClearingDependencies'

run_pkg ./internal/modules/kernel/orchestrator/biz \
  -run 'TestRuntimeV2ValidationDiscovery'

# RVC-R17/R18: Decision durable proposal saga and aggregate-only blocked Resume gate.
run_pkg ./internal/modules/blocks/decision/... \
  -run 'TestService_RuntimeGovernanceProposalStaysOpenUntilFinalize|TestService_RuntimeGovernanceRejectsUnallowedActionBeforeProposal|TestRuntimeGovernanceResolutionDoesNotResolveTicketBeforeAggregateCommit'

run_pkg ./internal/modules/kernel/orchestrator/biz \
  -run 'TestRuntimeV2BlockedDecisionRequiresCommittedAggregateResolution'

# RVC-R20/R21: descriptor-driven provider schema, required-operation wire mapping,
# and concrete LLM audit DTO wire payload.
run_pkg ./internal/modules/blocks/llm/... \
  -run 'TestRuntimeV2CandidateSchema(DerivesPatchTargetRequirementsFromOperationContract|RestrictsVariantsToSealedAuthority)|TestDecodeCandidatePayloadPreservesRequiredOperationRef|TestRequestCandidateClassifiesOutputLimitSchemaTruncation|TestResolveProviderModelTarget(UsesCatalogOutputCapabilityWithoutLocalCap|HonorsExplicitLocalOutputCap)|TestRuntimeV2CandidateContextProjectionPreservesTargetGroundingInVolatileAuthority'

run_pkg ./internal/pkg/events \
  -run 'TestLLMUsagePayloadUsesTypedAuditDTOs'

# Continuation / unresolved rehydrate.
run_pkg ./internal/modules/application/workorchd/biz/appservice/activationcore \
  -run 'TestCompileRuntimeV2CoverageRehydratesUnresolvedContinuation|TestCompileRuntimeV2CoverageGapAnalysisWhenContinuationLacksStructuredGaps|TestCompileRuntimeV2CoverageSlicesBoundLargeDoD|TestRuntimeV2BootstrapBuildsBoundedGitCapabilityStateFixture'

run_pkg ./internal/modules/application/workorchd/biz/appservice/governanceports \
  -run 'TestBatchResolveDecisionViewsRejectsMixedAllowedActionsBeforeWrites'

# Projection generation recovery / ready.
run_pkg ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore \
  -run 'TestStoreProjectionGenerationRecovery|TestStoreDeletedWorkspaceClearsGenerationFailure|TestStoreRestartDoesNotCarryGenerationFailures|TestManagerMetricsDoNotDriveProjectionHealth'

# Usage hydrate + terminal release.
run_pkg ./internal/modules/kernel/orchestrator/biz \
  -run 'TestProjectRuntimeV2CompleteBypassesLegacyResultGate|TestProjectRuntimeV2TerminalConvergesAfterRunUpdateFailure|TestProjectRuntimeV2TerminalDoesNotStealLiveActiveRun'

# P5: terminal authority is committed before projection. Recovery may only
# replay Result -> Workspace -> Run delivery, including every response-loss
# boundary, and must not re-drive a candidate or operation.
run_pkg ./internal/modules/kernel/orchestrator/biz \
  -run 'TestDeliverRuntimeV2PrerequisiteRefreshExhaustedRestartsAfterTerminalPersistenceBeforeResult|TestDeliverRuntimeV2TerminalRestartsAfterResultPersistenceBeforeOwnerProjection|TestDeliverRuntimeV2TerminalRestartsAfterOwnerProjectionAcknowledgementLoss|TestRuntimeV2StartupRecoveryDeliversPendingTerminalThroughOwnerChain'

run_pkg ./internal/modules/application/workorchd/biz/appservice/readmodelports \
  -run 'TestRunObservabilityHydratesLLMUsagePayloads|TestRunSnapshotBuildsSnapshotFromReaders'

run_pkg ./internal/modules/application/workorchd/service/server \
  -run 'TestServerMetricsObserveRunStateIsExactlyOnceAndMonotonic'

echo "runtime-v2 positive-convergence file verification passed" | tee -a "$LOG"
echo "artifacts=$ARTIFACT_DIR" | tee -a "$LOG"
