package common

import runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

const ModuleID = runtimemodel.RuntimeModuleShell

const StepKindCommand = runtimemodel.StepKindShellCommand

// ValidationRecipeAdapterProjectNative is the Shell-owned resolver contract
// for a project-native validation command. Profiles may name this adapter,
// but only Shell may translate it from File-owner manifest facts into an
// executable recipe.
const ValidationRecipeAdapterProjectNative = "shell/project-native-validation/v1"

// Validation owner diagnostic codes are Shell-owned, stable facts. Profiles
// may match these exact values; Runtime core must not infer them from stderr.
const (
	ValidationDiagnosticCodeModuleManifestParseFailed = "module_manifest_parse_failed"
	ValidationDiagnosticCodeSourceCompileFailed       = "source_compile_failed"
	ValidationDiagnosticCodeTestFailed                = "test_failed"
)

// ProspectiveSatisfactionContractRef identifies the Shell-owner pure query
// that evaluates whether an owner-resolved validation recipe would become
// selectable under a prospective workspace manifest.
const ProspectiveSatisfactionContractRef = "shell/selectable-validation-surface/prospective-workspace-manifest/v1"

// RuntimeCapabilityKinds is the Shell owner's complete Runtime capability inventory.
func RuntimeCapabilityKinds() []string {
	return []string{StepKindCommand, runtimemodel.RuntimeBlockKindManagedValidationSession}
}
