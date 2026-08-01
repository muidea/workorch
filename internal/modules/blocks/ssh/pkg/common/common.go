package common

import runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

const ModuleID = runtimemodel.RuntimeModuleSSH

const StepKindCommand = runtimemodel.StepKindSSHCommand

// RuntimeCapabilityKinds is the SSH owner's complete Runtime capability inventory.
func RuntimeCapabilityKinds() []string { return []string{StepKindCommand} }
