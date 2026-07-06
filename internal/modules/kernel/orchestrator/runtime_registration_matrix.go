package orchestrator

import (
	"strings"

	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeStepKindRegistration struct {
	StepKind       string
	RuntimeOwner   string
	ExecutorModule string
	Responsibility string
}

func runtimeStepKindRegistrations() []runtimeStepKindRegistration {
	return []runtimeStepKindRegistration{
		{StepKind: runtimemodel.StepKindLLMWorkspaceExecution, RuntimeOwner: runtimemodel.RuntimeModuleLLM, ExecutorModule: runtimemodel.RuntimeModuleLLM, Responsibility: "LLM workspace execution, assessment-only mode, repair and replan output normalization"},
		{StepKind: runtimemodel.StepKindMCPToolCall, RuntimeOwner: runtimemodel.RuntimeModuleMCP, ExecutorModule: runtimemodel.RuntimeModuleMCP, Responsibility: "MCP tool invocation"},
		{StepKind: runtimemodel.StepKindMCPResourceRead, RuntimeOwner: runtimemodel.RuntimeModuleMCP, ExecutorModule: runtimemodel.RuntimeModuleMCP, Responsibility: "MCP resource read"},
		{StepKind: runtimemodel.StepKindMCPPromptGet, RuntimeOwner: runtimemodel.RuntimeModuleMCP, ExecutorModule: runtimemodel.RuntimeModuleMCP, Responsibility: "MCP prompt read"},
		{StepKind: runtimemodel.StepKindShellCommand, RuntimeOwner: runtimemodel.RuntimeModuleShell, ExecutorModule: runtimemodel.RuntimeModuleShell, Responsibility: "bounded shell command execution"},
		{StepKind: runtimemodel.StepKindFileList, RuntimeOwner: runtimemodel.RuntimeModuleFile, ExecutorModule: runtimemodel.RuntimeModuleFile, Responsibility: "workspace file listing"},
		{StepKind: runtimemodel.StepKindFileRead, RuntimeOwner: runtimemodel.RuntimeModuleFile, ExecutorModule: runtimemodel.RuntimeModuleFile, Responsibility: "workspace file read"},
		{StepKind: runtimemodel.StepKindFileEdit, RuntimeOwner: runtimemodel.RuntimeModuleFile, ExecutorModule: runtimemodel.RuntimeModuleFile, Responsibility: "workspace file edit preview"},
		{StepKind: runtimemodel.RuntimeBlockKindPatchPreview, RuntimeOwner: ModuleID, ExecutorModule: runtimemodel.RuntimeModuleFile, Responsibility: "controlled workspace patch preview and governance-linked patch evidence"},
		{StepKind: runtimemodel.RuntimeBlockKindPatchApply, RuntimeOwner: ModuleID, ExecutorModule: runtimemodel.RuntimeModuleFile, Responsibility: "controlled workspace patch application and governance-linked patch evidence"},
		{StepKind: runtimemodel.RuntimeBlockKindPatchRevert, RuntimeOwner: ModuleID, ExecutorModule: runtimemodel.RuntimeModuleFile, Responsibility: "controlled workspace patch revert metadata"},
		{StepKind: runtimemodel.StepKindGitRead, RuntimeOwner: runtimemodel.RuntimeModuleGit, ExecutorModule: runtimemodel.RuntimeModuleGit, Responsibility: "repository state, diff, log and object read"},
		{StepKind: runtimemodel.StepKindGitWrite, RuntimeOwner: runtimemodel.RuntimeModuleGit, ExecutorModule: runtimemodel.RuntimeModuleGit, Responsibility: "repository write operation"},
		{StepKind: runtimemodel.StepKindWebSearch, RuntimeOwner: runtimemodel.RuntimeModuleWeb, ExecutorModule: runtimemodel.RuntimeModuleWeb, Responsibility: "web search"},
		{StepKind: runtimemodel.StepKindWebFetch, RuntimeOwner: runtimemodel.RuntimeModuleWeb, ExecutorModule: runtimemodel.RuntimeModuleWeb, Responsibility: "web fetch"},
		{StepKind: runtimemodel.StepKindSSHCommand, RuntimeOwner: runtimemodel.RuntimeModuleSSH, ExecutorModule: runtimemodel.RuntimeModuleSSH, Responsibility: "bounded remote command execution"},
	}
}

func registerRuntimeStepKinds(registry *orchestratorbiz.RuntimeAdapterRegistry, adapter orchestratorbiz.RuntimeAdapter) error {
	if registry == nil || adapter == nil {
		return common.ErrInvalidArgument
	}
	seen := map[string]struct{}{}
	for _, item := range runtimeStepKindRegistrations() {
		stepKind := strings.TrimSpace(item.StepKind)
		if stepKind == "" {
			return common.ErrInvalidArgument
		}
		if _, ok := seen[stepKind]; ok {
			return common.ErrConflict
		}
		seen[stepKind] = struct{}{}
		if err := registry.Register(stepKind, adapter); err != nil {
			return err
		}
	}
	return nil
}
