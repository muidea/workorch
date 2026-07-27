package runports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
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
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandCreate, "application/workorchd/appservice", runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.CreateRunCommand{
		RunID:              item.ID,
		WorkspaceID:        item.WorkspaceID,
		GoalID:             item.GoalID,
		DoDID:              item.DoDID,
		DefinitionRevision: item.DefinitionRevision,
		InitialStepKind:    kind,
		RuntimeBootstrap:   item.RuntimeBootstrap,
		Lineage:            item.Lineage,
		Priority:           item.Priority,
	})
	ownerevent.BindLane(event, runevents.TopicRunCommandCreate, "run", string(item.ID), item.WorkspaceID, item.ID, "")
	_, err := ownerevent.Run(hub.Send(event), runevents.TopicRunCommandCreate)
	return err
}

func StartRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandStartRun, "application/workorchd/appservice", orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.StartRunCommand{
		RunID: item.ID,
	})
	ownerevent.BindLane(event, orchestratorevents.TopicOrchestratorCommandStartRun, "run", string(item.ID), item.WorkspaceID, item.ID, "")
	_, err := ownerevent.Orchestrator(hub.Send(event), orchestratorevents.TopicOrchestratorCommandStartRun)
	return err
}

func ResumeRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandResumeRun, "application/workorchd/appservice", orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.ResumeRunCommand{
		WorkspaceID: item.WorkspaceID,
		RunID:       item.ID,
	})
	ownerevent.BindLane(event, orchestratorevents.TopicOrchestratorCommandResumeRun, "run", string(item.ID), item.WorkspaceID, item.ID, "")
	_, err := ownerevent.Orchestrator(hub.Send(event), orchestratorevents.TopicOrchestratorCommandResumeRun)
	return err
}
