package common

import runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

const ModuleID = runtimemodel.RuntimeModuleSSH

const StepKindCommand = runtimemodel.StepKindSSHCommand

// RuntimeCapabilityKinds is the SSH owner's complete Runtime V2 capability inventory.
func RuntimeCapabilityKinds() []string { return []string{StepKindCommand} }
