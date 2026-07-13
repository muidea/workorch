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

func RuntimeModuleForOperationKind(kind string) string {
	switch CanonicalRuntimeBlockKind(kind) {
	case StepKindFileList, StepKindFileRead, StepKindFileEdit, RuntimeBlockKindPatchApply, RuntimeBlockKindPatchPreview, RuntimeBlockKindPatchRevert:
		return RuntimeModuleFile
	case StepKindGitRead, StepKindGitWrite:
		return RuntimeModuleGit
	case StepKindMCPToolCall, StepKindMCPResourceRead, StepKindMCPPromptGet:
		return RuntimeModuleMCP
	case StepKindShellCommand:
		return RuntimeModuleShell
	case StepKindSSHCommand:
		return RuntimeModuleSSH
	case StepKindWebSearch, StepKindWebFetch:
		return RuntimeModuleWeb
	default:
		return ""
	}
}
