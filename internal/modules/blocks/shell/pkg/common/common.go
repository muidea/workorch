package common

import runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

const ModuleID = runtimemodel.RuntimeModuleShell

const StepKindCommand = runtimemodel.StepKindShellCommand

// Validation owner diagnostic codes are Shell-owned, stable facts. Profiles
// may match these exact values; Runtime core must not infer them from stderr.
const (
	ValidationDiagnosticCodeGoModuleParseFailed   = "go_module_parse_failed"
	ValidationDiagnosticCodeGoSourceCompileFailed = "go_source_compile_failed"
	ValidationDiagnosticCodeGoTestFailed          = "go_test_failed"
)

// ProspectiveSatisfactionContractRef identifies the Shell-owner pure query
// that evaluates whether an activation-sealed validation recipe would become
// selectable under a prospective workspace manifest.
const ProspectiveSatisfactionContractRef = "shell/selectable-validation-surface/prospective-workspace-manifest/v1"

// RuntimeCapabilityKinds is the Shell owner's complete Runtime capability inventory.
func RuntimeCapabilityKinds() []string {
	return []string{StepKindCommand, runtimemodel.RuntimeBlockKindManagedValidationSession}
}
