package orchestrator

import (
	"strings"

	filecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	mcpcommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	sshcommon "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/common"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	"github.com/muidea/workorch/internal/pkg/common"
)

type runtimeStepKindRegistration struct {
	StepKind       string
	RuntimeOwner   string
	ExecutorModule string
	Responsibility string
}

func runtimeStepKindRegistrations() []runtimeStepKindRegistration {
	return []runtimeStepKindRegistration{
		{StepKind: llmcommon.StepKindWorkspaceExecution, RuntimeOwner: llmcommon.ModuleID, ExecutorModule: llmcommon.ModuleID, Responsibility: "LLM workspace execution, assessment-only mode, repair and replan output normalization"},
		{StepKind: mcpcommon.StepKindToolCall, RuntimeOwner: mcpcommon.ModuleID, ExecutorModule: mcpcommon.ModuleID, Responsibility: "MCP tool invocation"},
		{StepKind: mcpcommon.StepKindResourceRead, RuntimeOwner: mcpcommon.ModuleID, ExecutorModule: mcpcommon.ModuleID, Responsibility: "MCP resource read"},
		{StepKind: mcpcommon.StepKindPromptGet, RuntimeOwner: mcpcommon.ModuleID, ExecutorModule: mcpcommon.ModuleID, Responsibility: "MCP prompt read"},
		{StepKind: shellcommon.StepKindCommand, RuntimeOwner: shellcommon.ModuleID, ExecutorModule: shellcommon.ModuleID, Responsibility: "bounded shell command execution"},
		{StepKind: filecommon.StepKindList, RuntimeOwner: filecommon.ModuleID, ExecutorModule: filecommon.ModuleID, Responsibility: "workspace file listing"},
		{StepKind: filecommon.StepKindRead, RuntimeOwner: filecommon.ModuleID, ExecutorModule: filecommon.ModuleID, Responsibility: "workspace file read"},
		{StepKind: filecommon.StepKindEdit, RuntimeOwner: filecommon.ModuleID, ExecutorModule: filecommon.ModuleID, Responsibility: "workspace file edit preview"},
		{StepKind: llmcommon.RuntimeBlockKindPatchPreview, RuntimeOwner: ModuleID, ExecutorModule: ModuleID, Responsibility: "controlled workspace patch preview and governance-linked patch evidence"},
		{StepKind: llmcommon.RuntimeBlockKindPatchApply, RuntimeOwner: ModuleID, ExecutorModule: ModuleID, Responsibility: "controlled workspace patch application and governance-linked patch evidence"},
		{StepKind: llmcommon.RuntimeBlockKindPatchRevert, RuntimeOwner: ModuleID, ExecutorModule: ModuleID, Responsibility: "controlled workspace patch revert metadata"},
		{StepKind: gitcommon.StepKindRead, RuntimeOwner: gitcommon.ModuleID, ExecutorModule: gitcommon.ModuleID, Responsibility: "repository state, diff, log and object read"},
		{StepKind: gitcommon.StepKindWrite, RuntimeOwner: gitcommon.ModuleID, ExecutorModule: gitcommon.ModuleID, Responsibility: "repository write operation"},
		{StepKind: webcommon.StepKindSearch, RuntimeOwner: webcommon.ModuleID, ExecutorModule: webcommon.ModuleID, Responsibility: "web search"},
		{StepKind: webcommon.StepKindFetch, RuntimeOwner: webcommon.ModuleID, ExecutorModule: webcommon.ModuleID, Responsibility: "web fetch"},
		{StepKind: sshcommon.StepKindCommand, RuntimeOwner: sshcommon.ModuleID, ExecutorModule: sshcommon.ModuleID, Responsibility: "bounded remote command execution"},
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
