package common

import runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

const ModuleID = runtimemodel.RuntimeModuleShell

const StepKindCommand = runtimemodel.StepKindShellCommand

// ProspectiveSatisfactionContractRef identifies the Shell-owner pure query
// that evaluates whether an activation-sealed validation recipe would become
// selectable under a prospective workspace manifest.
const ProspectiveSatisfactionContractRef = "shell/selectable-validation-surface/prospective-workspace-manifest/v1"

// RuntimeCapabilityKinds is the Shell owner's complete Runtime capability inventory.
func RuntimeCapabilityKinds() []string {
	return []string{StepKindCommand, runtimemodel.RuntimeBlockKindManagedValidationSession}
}
