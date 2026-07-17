#!/usr/bin/env bash
# RVC-17: deterministic file-backed Runtime V2 positive-convergence verifier.
# Uses Go unit/integration fixtures (testkit-compatible controller path); no public LLM.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ARTIFACT_DIR="${WORKORCH_E2E_ARTIFACT_DIR:-.workorch-e2e/runtime-v2-positive-convergence/$(date -u +%Y%m%dT%H%M%SZ)}"
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

# Core positive-convergence fixture sequences (RVC-17 1-8, restart reload).
run_pkg ./internal/modules/kernel/orchestrator/biz/runtimev2 \
  -run 'TestPositiveConvergenceFixture_'

# Candidate attempt 0/1, restart idempotent drive path.
run_pkg ./internal/modules/kernel/orchestrator \
  -run 'TestDriveRuntimeV2CompletesTypedPathAndIsRestartIdempotent|TestDriveRuntimeV2CandidateFailureStopsAfterSameContractRetry|TestRuntimeV2EnsureCandidateFeasibleBlocksProviderWithoutKinds'

# Reducer repair / budget / same-focus.
run_pkg ./internal/modules/kernel/orchestrator/biz/runtimev2 \
  -run 'TestReduceRuntimeStateRepairableAssessmentDoesNotTerminal|TestReduceRuntimeStateSameFocusRepairThenReassess|TestReduceRuntimeStateRepairBudgetExhausted|TestReduceRuntimeStateValidationOnlyCapEscalatesToStateChange|TestInvalidateNonSatisfiedAssessmentsForFocus'

# Continuation / unresolved rehydrate (RVC-17 #9).
run_pkg ./internal/modules/application/workorchd/biz/appservice/activationcore \
  -run 'TestCompileRuntimeV2CoverageRehydratesUnresolvedContinuation|TestCompileRuntimeV2CoverageGapAnalysisWhenContinuationLacksStructuredGaps|TestCompileRuntimeV2CoverageSlicesBoundLargeDoD'

# Descriptor / managed validation feasibility (supporting #4).
run_pkg ./internal/pkg/workorchruntimecontract \
  -run 'TestRuntimeOperationDescriptorsAreStableCompleteAndUnique|TestFeasibleOperationKindsIntersectAcceptedCapabilitiesAndEffect|TestDeriveAssessmentDispositionLegacyAndPartial|TestRuntimeRepairLedgerBeginCycleIsIdempotentAndBudgeted'

# Projection generation recovery / ready (RVC-17 #10).
run_pkg ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore \
  -run 'TestStoreProjectionGenerationRecovery|TestStoreDeletedWorkspaceClearsGenerationFailure|TestStoreRestartDoesNotCarryGenerationFailures|TestManagerMetricsDoNotDriveProjectionHealth'

# Usage hydrate + terminal release (supporting progress/terminal consistency).
run_pkg ./internal/modules/kernel/orchestrator/biz \
  -run 'TestProjectRuntimeV2CompleteBypassesLegacyResultGate|TestProjectRuntimeV2TerminalConvergesAfterRunUpdateFailure|TestProjectRuntimeV2TerminalDoesNotStealLiveActiveRun'

run_pkg ./internal/modules/application/workorchd/biz/appservice/readmodelports \
  -run 'TestRunObservabilityHydratesLLMUsagePayloads|TestRunSnapshotBuildsSnapshotFromReaders'

echo "runtime-v2 positive-convergence file verification passed" | tee -a "$LOG"
echo "artifacts=$ARTIFACT_DIR" | tee -a "$LOG"
