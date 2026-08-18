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
// Prefer OperationHasWorkspaceSideEffectsWithPolicies when sealed owner facts
// are available.
func OperationHasWorkspaceSideEffects(operation RuntimeOperation) bool {
	return OperationHasWorkspaceSideEffectsWithPolicies(operation, nil)
}

// OperationHasWorkspaceSideEffectsWithPolicies is the owner-fact-aware
// side-effect gate. Unknown effects fail closed as mutating.
func OperationHasWorkspaceSideEffectsWithPolicies(operation RuntimeOperation, policies []RuntimeOperationPolicyFact) bool {
	scope := ResolveExpectedEffectScope(operation, policies)
	switch scope {
	case RuntimeEffectNone, RuntimeEffectEphemeralSession, RuntimeEffectGovernanceOnly:
		return false
	case RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable, RuntimeEffectUnknown:
		return true
	default:
		return true
	}
}
