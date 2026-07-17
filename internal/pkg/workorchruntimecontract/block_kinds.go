package workorchruntimecontract

import "strings"

const (
	RuntimeModuleFile         = "blocks/file"
	RuntimeModuleGit          = "blocks/git"
	RuntimeModuleLLM          = "blocks/llm"
	RuntimeModuleMCP          = "blocks/mcp"
	RuntimeModuleShell        = "blocks/shell"
	RuntimeModuleSSH          = "blocks/ssh"
	RuntimeModuleWeb          = "blocks/web"
	RuntimeModuleOrchestrator = "kernel/orchestrator"
	RuntimeModuleDecision     = "blocks/decision"
)

const (
	StepKindFileList                         = "file/list"
	StepKindFileRead                         = "file/read"
	StepKindFileEdit                         = "file/edit"
	StepKindGitRead                          = "git/read"
	StepKindGitWrite                         = "git/write"
	StepKindLLMWorkspaceExecution            = "llm/workspace_execution"
	StepKindLLMWorkspacePlan                 = "llm/workspace-plan"
	StepKindMCPToolCall                      = "mcp/tool_call"
	StepKindMCPResourceRead                  = "mcp/resource_read"
	StepKindMCPPromptGet                     = "mcp/prompt_get"
	StepKindShellCommand                     = "shell/command"
	StepKindSSHCommand                       = "ssh/command"
	StepKindWebSearch                        = "web/search"
	StepKindWebFetch                         = "web/fetch"
	RuntimeBlockKindPatchApply               = "patch/apply"
	RuntimeBlockKindPatchPreview             = "patch/preview"
	RuntimeBlockKindPatchRevert              = "patch/revert"
	RuntimeBlockKindDecisionRequest          = "decision/request"
	RuntimeBlockKindApprovalRequest          = "approval/request"
	RuntimeBlockKindMemoryWrite              = "memory/write"
	RuntimeBlockKindDoDAssessment            = "dod/assessment"
	RuntimeBlockKindManagedValidationSession = "validation/managed_session"
)

func CanonicalRuntimeBlockKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case StepKindLLMWorkspaceExecution, StepKindLLMWorkspacePlan:
		return StepKindLLMWorkspaceExecution
	case "patchRequests", "patch/request":
		return RuntimeBlockKindPatchApply
	default:
		return strings.TrimSpace(kind)
	}
}

// RuntimeModuleForOperationKind returns the owner module for a concrete operation kind.
// Authority is RuntimeOperationDescriptorForKind; unknown kinds return empty string.
func RuntimeModuleForOperationKind(kind string) string {
	descriptor, ok := RuntimeOperationDescriptorForKind(kind)
	if !ok {
		return ""
	}
	return descriptor.OwnerModuleID
}

// OperationHasWorkspaceSideEffects reports whether a concrete operation may mutate
// workspace or remote state. Kind-level effect comes from RuntimeOperationDescriptor.
// Prefer OperationHasWorkspaceSideEffectsWithCapabilities when sealed MCP capability
// metadata is available.
func OperationHasWorkspaceSideEffects(operation RuntimeOperation) bool {
	return OperationHasWorkspaceSideEffectsWithCapabilities(operation, nil)
}

// OperationHasWorkspaceSideEffectsWithCapabilities is the capability-aware side-effect
// gate. Descriptor EffectClass is the kind-level authority. Unknown kinds default to
// mutating. MCP tool calls may be narrowed to non-mutating only when owner-sealed
// RiskLevel=read_only capability metadata matches. Candidate-declared stateChanging is
// never consulted as a side-effect safety authority.
func OperationHasWorkspaceSideEffectsWithCapabilities(operation RuntimeOperation, capabilities []RuntimeMCPCapabilityContext) bool {
	operation = operation.Normalized()
	kind := CanonicalRuntimeBlockKind(operation.Kind)
	descriptor, ok := RuntimeOperationDescriptorForKind(kind)
	if !ok {
		// Unknown kinds must not enter validation as "pure".
		return true
	}
	if kind == StepKindMCPToolCall {
		return mcpToolCallHasWorkspaceSideEffects(operation, capabilities)
	}
	return operationEffectHasWorkspaceSideEffects(descriptor.EffectClass)
}

func mcpToolCallHasWorkspaceSideEffects(operation RuntimeOperation, capabilities []RuntimeMCPCapabilityContext) bool {
	serverID := strings.TrimSpace(operationStringParameter(operation.Parameters, "serverId"))
	tool := strings.TrimSpace(operationStringParameter(operation.Parameters, "tool"))
	if serverID == "" || tool == "" || len(capabilities) == 0 {
		// Unknown / unmatched capability: deny validation entry.
		return true
	}
	for _, capability := range capabilities {
		if !capability.Enabled {
			continue
		}
		if strings.TrimSpace(string(capability.ServerID)) != serverID {
			continue
		}
		if strings.TrimSpace(capability.Kind) != "" && CanonicalRuntimeBlockKind(capability.Kind) != StepKindMCPToolCall {
			continue
		}
		if strings.TrimSpace(capability.Name) != tool {
			continue
		}
		return !mcpCapabilityRiskIsReadOnly(capability.RiskLevel)
	}
	return true
}

func mcpCapabilityRiskIsReadOnly(riskLevel string) bool {
	switch strings.ToLower(strings.TrimSpace(riskLevel)) {
	case "read_only", "readonly", "read-only", "l0":
		return true
	default:
		return false
	}
}
