package common

import runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

const ModuleID = runtimemodel.RuntimeModuleMCP

const StepKindToolCall = runtimemodel.StepKindMCPToolCall
const StepKindResourceRead = runtimemodel.StepKindMCPResourceRead
const StepKindPromptGet = runtimemodel.StepKindMCPPromptGet

// RuntimeCapabilityKinds is the MCP owner's complete Runtime capability inventory.
func RuntimeCapabilityKinds() []string {
	return []string{StepKindToolCall, StepKindResourceRead, StepKindPromptGet}
}
