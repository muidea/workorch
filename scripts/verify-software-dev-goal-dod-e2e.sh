#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/workorch-go-build}"
export GOCACHE

echo "=== software-dev Goal/DoD Runtime convergence verification ==="

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
  -run 'Test(DriveRuntimeFileBackedTwoFocusStateValidationAssessmentAndRestart|RuntimeFileBackedEvidenceCoverageFocusAdvanceCrashRestart)$' \
  -count=1

echo "software-dev Goal/DoD Runtime convergence verification passed"
