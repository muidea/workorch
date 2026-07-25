package common

import runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

const ModuleID = runtimemodel.RuntimeModuleShell

const StepKindCommand = runtimemodel.StepKindShellCommand

// RuntimeCapabilityKinds is the Shell owner's complete Runtime V2 capability inventory.
func RuntimeCapabilityKinds() []string {
	return []string{StepKindCommand, runtimemodel.RuntimeBlockKindManagedValidationSession}
}
