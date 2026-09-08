#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/workorch-go-build}"
export GOCACHE

echo "=== software-dev contract verification (reference fixtures and scripted provider, not deployed DCE) ==="

echo "-> fixed entrypoint grader rejects known wrong implementations and temporary noise preserves failure comparison"
go test ./internal/modules/blocks/shell/biz \
  -run 'Test(StdioEnvelopeReferenceGraderCalibration|ShellAssertionComparisonUsesRealManagedTemporaryPaths|ShellAssertionComparisonNormalizesOnlyOwnedTemporaryNoise|ShellAssertionFeedbackRequiresFailedScopedNativeOutput|ShellReportedAssertionRejectsRawValuesAndAmbiguousGrammar)$' \
  -count=1

echo "-> diagnostic identity, safe provider feedback and same-owner rejection CAS remain compatible"
go test ./internal/pkg/workorchruntimecontract ./internal/modules/blocks/llm/biz ./internal/modules/kernel/orchestrator/biz/runtime \
  -run 'Test(Assertion.*|RejectedValidation.*|RuntimeProviderRequestPreservesStablePrefixAndTypedContext|RuntimeProviderValidationRejectsMalformedDiagnosticExtension)$' \
  -count=1

echo "-> Goal/DoD compiles into bounded, ordered coverage focuses"
go test ./internal/modules/application/workorchd/biz/appservice/activationcore \
  -run 'Test(RuntimeBootstrapBuildsOrderedDoDFocuses|RuntimeBootstrapSoftwareDevProfileIncludesManagedValidation|CompileRuntimeCoverageFocusesKeepsDeliverableStateChange|CompileRuntimeCoverageFocusesRejectsUngroundedRequiredTarget)$' \
  -count=1

echo "-> semantic plan and evidence binding remain private SM-2 transitions"
go test ./internal/modules/kernel/orchestrator/biz/runtime \
  -run 'Test(RuntimeSemanticPlanningPrivateTransitionSequenceIsPersistedAndIdempotent|CoverageEvidenceBindingReceiptCrashRecoveryCreditsExactlyOnce|CoverageEvidenceCreditRequiresExactSealedBindingAndOwnerReceipt)$' \
  -count=1

echo "-> two-focus state-change/validation convergence survives file-backed restart"
go test ./internal/modules/kernel/orchestrator/biz \
  -run 'Test(DriveRuntimeFileBackedTwoFocusStateValidationAssessmentAndRestart|RuntimeFileBackedEvidenceCoverageFocusAdvanceCrashRestart|DriveRuntimeAssertionFeedbackRepairsBehaviorAcrossRestart)$' \
  -count=1

echo "software-dev contract/reference-grader verification passed; deployed Goal completion and release DCE are not established"
