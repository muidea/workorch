#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/workorch-go-build}"
export GOCACHE

echo "=== software-dev Runtime profile verification ==="

go test ./internal/pkg/workorchdefaults \
  -run 'Test(EmbeddedSoftwareDevTemplateDoDItemsCoverScopeEvidenceAndRisk|ValidationPreparationContractIsSoftwareDevelopmentScoped|EmbeddedTemplateStagePolicySchemaWhenDeclared|EmbeddedSoftwareDevSoulsUseCurrentRuntimeDocuments)$' \
  -count=1

go test ./internal/modules/blocks/shell/biz \
  -run '^TestShellValidationDiagnosticClassifiesGoDependencyMetadataBeforeGenericBuildFailure$' \
  -count=1

go test ./internal/modules/application/workorchd/biz/appservice/activationports \
  -run '^TestBuiltinSoftwareDevProfilePassesDefinitionAdmissionBeforeRun$' \
  -count=1

go test ./internal/modules/application/workorchd/biz/appservice/activationcore \
  -run 'Test(RuntimeBootstrapSoftwareDevProfileIncludesManagedValidation|RuntimeBootstrapBuildsBoundedGitCapabilityStateFixture|CompileRuntimeCoverageFocusesKeepsDeliverableStateChange)$' \
  -count=1

go test ./internal/modules/kernel/orchestrator/biz/runtime \
  -run '^TestRejectedTypedValidationWithoutFailureTargetOpensProfileRepairGeneration$' \
  -count=1

go test ./internal/modules/kernel/orchestrator/biz \
  -run 'Test(RuntimeFileBackedPatchVariantMatrix|DriveRuntimeFileBackedTwoFocusStateValidationAssessmentAndRestart)$' \
  -count=1
go test ./internal/pkg/workorchruntimecontract \
  -run 'TestRuntimeSemanticExistingTargetPayloadAndCorrectionUseOwnerReplacementKind' \
  -count=1

echo "software-dev Runtime profile verification passed"
