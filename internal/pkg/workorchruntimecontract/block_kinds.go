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
	StepKindFileWrite                        = "file/write"
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
	case "file/write_file":
		return "file/write"
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
// gate based on ResolveExpectedEffectScope. Unknown kinds default to mutating. MCP tool
// calls may be narrowed only by owner-sealed EffectScope. Shell/SSH may be narrowed by
// explicit stateChanging=false at seal time; missing stateChanging keeps the durable
// kind default.
func OperationHasWorkspaceSideEffectsWithCapabilities(operation RuntimeOperation, capabilities []RuntimeMCPCapabilityContext) bool {
	// Prefer concrete ExpectedEffectScope (descriptor + params + sealed MCP metadata).
	scope := ResolveExpectedEffectScope(operation, capabilities)
	switch scope {
	case RuntimeEffectNone, RuntimeEffectEphemeralSession, RuntimeEffectGovernanceOnly:
		return false
	case RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable, RuntimeEffectUnknown:
		return true
	default:
		return true
	}
}
