package activationports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const activationRunCommandSource = "application/workorchd/workspace-activation"

func SupersedeRun(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandSupersedeRun, activationRunCommandSource, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.SupersedeRunCommand{
		RunID: runID,
	})
	ownerevent.BindLane(event, orchestratorevents.TopicOrchestratorCommandSupersedeRun, "run", string(runID), workspaceID, runID, "")
	_, err := ownerevent.Orchestrator(hub.Send(event), orchestratorevents.TopicOrchestratorCommandSupersedeRun)
	return err
}

func CreateWorkspaceExecutionRun(ctx context.Context, hub magicevent.Hub, runID common.RunID, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID, definitionRevision string, runtimeBootstrap *runtimecontract.RuntimeBootstrap, priority int, lineage runmodel.RunLineage) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandCreate, activationRunCommandSource, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.CreateRunCommand{
		RunID:              runID,
		WorkspaceID:        workspaceID,
		GoalID:             goalID,
		DoDID:              dodID,
		DefinitionRevision: definitionRevision,
		InitialStepKind:    llmcommon.StepKindWorkspaceExecution,
		RuntimeBootstrap:   runtimeBootstrap,
		Lineage:            lineage,
		Priority:           priority,
	})
	ownerevent.BindLane(event, runevents.TopicRunCommandCreate, "run", string(runID), workspaceID, runID, "")
	_, err := ownerevent.Run(hub.Send(event), runevents.TopicRunCommandCreate)
	return err
}

func LinkRunSuccessor(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, predecessorRunID common.RunID, successorRunID common.RunID, expectedSuccessorRunID common.RunID) error {
	if hub == nil || workspaceID == "" || predecessorRunID == "" || successorRunID == "" {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandLinkSuccessor, activationRunCommandSource, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.LinkRunSuccessorCommand{
		WorkspaceID:            workspaceID,
		PredecessorRunID:       predecessorRunID,
		SuccessorRunID:         successorRunID,
		ExpectedSuccessorRunID: expectedSuccessorRunID,
	})
	ownerevent.BindLane(event, runevents.TopicRunCommandLinkSuccessor, "run", string(predecessorRunID), workspaceID, predecessorRunID, "")
	_, err := ownerevent.Run(hub.Send(event), runevents.TopicRunCommandLinkSuccessor)
	return err
}

func StartRun(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandStartRun, activationRunCommandSource, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.StartRunCommand{
		RunID: runID,
	})
	ownerevent.BindLane(event, orchestratorevents.TopicOrchestratorCommandStartRun, "run", string(runID), workspaceID, runID, "")
	_, err := ownerevent.Orchestrator(hub.Send(event), orchestratorevents.TopicOrchestratorCommandStartRun)
	return err
}

func FailRun(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID, failureReason string) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandFailRun, activationRunCommandSource, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.FailRunCommand{
		RunID:  runID,
		Reason: failureReason,
	})
	ownerevent.BindLane(event, orchestratorevents.TopicOrchestratorCommandFailRun, "run", string(runID), workspaceID, runID, "")
	_, err := ownerevent.Orchestrator(hub.Send(event), orchestratorevents.TopicOrchestratorCommandFailRun)
	return err
}
