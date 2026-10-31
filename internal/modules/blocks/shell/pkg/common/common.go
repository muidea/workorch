package common

import runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

const ModuleID = runtimemodel.RuntimeModuleShell

const StepKindCommand = runtimemodel.StepKindShellCommand

// ValidationRecipeAdapterProjectNative is the Shell-owned resolver contract
// for a project-native validation command. Profiles may name this adapter,
// but only Shell may translate it from File-owner manifest facts into an
// executable recipe.
const ValidationRecipeAdapterProjectNative = "shell/project-native-validation"

// ValidationRecipeAdapterProjectEntrypoint observes the declared project entrypoint.
const ValidationRecipeAdapterProjectEntrypoint = "shell/entrypoint-validation"

// Reviewed and isolated inputs require independent exact-content review.
// Each adapter names a distinct execution and isolation scope.
const ValidationRecipeAdapterReviewedEntrypoint = "shell/reviewed-entrypoint-validation"
const ValidationRecipeAdapterIsolatedEntrypoint = "shell/isolated-package-validation"

// 多检查包以完整 requirement 固定成员，不扩大既有单检查协议。
const ValidationRecipeAdapterConjunctiveEntrypoint = "shell/conjunctive-package-validation"

// 根 module 四阶段合取与 package-only 验证属于不同执行范围。
const ValidationRecipeAdapterProjectConjunction = "shell/project-conjunction-validation"

// Diagnostic variants add bounded failure reproduction to project validation.
const ValidationRecipeAdapterProjectFailureDiagnostic = "shell/project-failure-diagnostic-validation"
const ValidationRecipeAdapterProjectMultiFailureDiagnostic = "shell/project-multi-failure-validation"

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
