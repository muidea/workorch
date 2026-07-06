package activationcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CreatedWorkspaceRunExecutionContext(runID common.RunID, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID, executionContext map[string]any, priority int, fallbackLanguage string) map[string]any {
	createdRun := &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		GoalID:           goalID,
		DoDID:            dodID,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: runtimecore.CloneContextMap(executionContext),
		State:            common.RunStatePending,
		Priority:         priority,
	}
	ret := runtimecore.CloneContextMap(executionContext)
	if ret == nil {
		ret = map[string]any{}
	}
	if compressed := runtimecore.CreatedRunCompressedContext(createdRun, runtimecore.RunDialogLanguage(createdRun, fallbackLanguage)); compressed != nil {
		ret["compressedContext"] = compressed
	}
	return ret
}
