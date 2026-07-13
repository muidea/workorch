package runports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CreateRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" {
		return common.ErrInvalidArgument
	}
	kind := item.InitialStepKind
	if kind == "" {
		kind = llmcommon.StepKindWorkspaceExecution
	}
	_, err := sendRunMagicCommand[runevents.CreateRunCommand, runevents.RunCommandResult](ctx, hub, runevents.TopicRunCommandCreate, runevents.CommandDestination, "run", string(item.ID), item.WorkspaceID, item.ID, runevents.CreateRunCommand{
		RunID:              item.ID,
		WorkspaceID:        item.WorkspaceID,
		GoalID:             item.GoalID,
		DoDID:              item.DoDID,
		DefinitionRevision: item.DefinitionRevision,
		InitialStepKind:    kind,
		RuntimeBootstrap:   item.RuntimeBootstrap,
		Priority:           item.Priority,
	})
	return err
}

func StartRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	_, err := sendRunMagicCommand[orchestratorevents.StartRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandStartRun, orchestratorevents.CommandDestination, "run", string(item.ID), item.WorkspaceID, item.ID, orchestratorevents.StartRunCommand{
		RunID: item.ID,
	})
	return err
}

func ResumeRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	_, err := sendRunMagicCommand[orchestratorevents.ResumeRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandResumeRun, orchestratorevents.CommandDestination, "run", string(item.ID), item.WorkspaceID, item.ID, orchestratorevents.ResumeRunCommand{
		WorkspaceID: item.WorkspaceID,
		RunID:       item.ID,
	})
	return err
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
