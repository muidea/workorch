package runports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func CreateRun(ctx context.Context, hub events.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" {
		return common.ErrInvalidArgument
	}
	kind := item.InitialStepKind
	if kind == "" {
		kind = llmcommon.StepKindWorkspaceExecution
	}
	return ownercommandports.Publish(ctx, hub, events.TopicRunCommandCreate, "run", string(item.ID), item.WorkspaceID, item.ID, map[string]any{
		events.PayloadKeyCommand: runcontracts.CreateRunCommand{
			RunID:              item.ID,
			WorkspaceID:        item.WorkspaceID,
			GoalID:             item.GoalID,
			DoDID:              item.DoDID,
			DefinitionRevision: item.DefinitionRevision,
			InitialStepKind:    kind,
			ExecutionContext:   item.ExecutionContext,
			Priority:           item.Priority,
		},
	})
}

func StartRun(ctx context.Context, hub events.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return ownercommandports.Publish(ctx, hub, events.TopicRunCommandStart, "run", string(item.ID), item.WorkspaceID, item.ID, map[string]any{
		events.PayloadKeyCommand: runcontracts.StartRunCommand{RunID: item.ID},
	})
}

func ResumeRun(ctx context.Context, hub events.Hub, item *runmodel.Run, executionContextPatch map[string]any) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return ownercommandports.Publish(ctx, hub, events.TopicOrchestratorCommandResumeRun, "run", string(item.ID), item.WorkspaceID, item.ID, map[string]any{
		events.PayloadKeyCommand: runcontracts.ResumeRunCommand{
			WorkspaceID:           item.WorkspaceID,
			RunID:                 item.ID,
			ExecutionContextPatch: cloneMap(executionContextPatch),
		},
	})
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
