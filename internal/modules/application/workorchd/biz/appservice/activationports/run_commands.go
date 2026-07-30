package activationports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const activationRunCommandSource = "application/workorchd/workspace-activation"

// ReleaseFreshActivationRunSlot forwards the already-persisted Run owner fact
// needed to release a non-resumable active slot before an explicit fresh
// activation. It never reads or updates Workspace persistence directly and it
// never asks Orchestrator to load, terminate, or migrate the incompatible
// aggregate.
func ReleaseFreshActivationRunSlot(ctx context.Context, hub magicevent.Hub, run *runmodel.Run) error {
	if hub == nil || run == nil || run.WorkspaceID == "" || run.ID == "" || run.RuntimeState == nil || run.RuntimeActivationStatus == nil {
		return common.ErrInvalidArgument
	}
	status := run.RuntimeActivationStatus
	if status.Disposition != runmodel.RuntimeActivationDispositionFreshActivationRequired ||
		status.RuntimeStateRef == "" || status.ActualSchemaVersion == 0 || status.SupportedSchemaVersion == 0 ||
		status.ActualSchemaVersion == status.SupportedSchemaVersion {
		return common.ErrInvalidArgument
	}
	runtimeStateRevision := run.RuntimeState.StateRevision
	if runtimeStateRevision == 0 || runevents.RuntimeStateRef(run.WorkspaceID, run.ID, runtimeStateRevision) != status.RuntimeStateRef {
		return common.ErrConflict
	}
	command := workspaceevents.ReleaseFreshActivationRunSlotFactCommand{
		WorkspaceID:                        run.WorkspaceID,
		RunID:                              run.ID,
		SourceOwner:                        runevents.CommandDestination,
		CausationID:                        runevents.RunStateChangedDataFromModel(run).CausationID,
		RuntimeStateRevision:               runtimeStateRevision,
		RuntimeStateRef:                    status.RuntimeStateRef,
		RuntimeActivationDisposition:       status.Disposition,
		ActualRuntimeStateSchemaVersion:    status.ActualSchemaVersion,
		SupportedRuntimeStateSchemaVersion: status.SupportedSchemaVersion,
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandReleaseFreshActivationRunSlot, workspaceevents.FreshActivationFactForwarderSource, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, workspaceevents.TopicWorkspaceCommandReleaseFreshActivationRunSlot, "workspace", string(run.WorkspaceID), run.WorkspaceID, run.ID, "")
	_, err := ownerevent.WorkspaceLifecycle(hub.Send(event), workspaceevents.TopicWorkspaceCommandReleaseFreshActivationRunSlot)
	return err
}

func SupersedeRun(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, activationRunCommandSource, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.RequestRuntimeTerminationCommand{WorkspaceID: workspaceID, RunID: runID, Kind: runtimecontract.RuntimeTerminationSuperseded, RequestedBy: activationRunCommandSource, Reason: "workspace execution superseded"})
	ownerevent.BindLane(event, orchestratorevents.TopicOrchestratorRequestRuntimeTermination, "run", string(runID), workspaceID, runID, "")
	response := hub.Send(event)
	if response == nil {
		return common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return err
	}
	data, err := response.Get()
	if err != nil {
		return err
	}
	receipt, ok := data.(orchestratorevents.RequestRuntimeTerminationResult)
	if !ok || !receipt.WorkspaceReleased {
		return common.ErrConflict
	}
	return nil
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
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandActivate, activationRunCommandSource, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.ActivateRunCommand{WorkspaceID: workspaceID, RunID: runID})
	ownerevent.BindLane(event, runevents.TopicRunCommandActivate, "run", string(runID), workspaceID, runID, "")
	_, err := ownerevent.RunLifecycle(hub.Send(event), runevents.TopicRunCommandActivate)
	return err
}

// CancelRun records an activation-side administrative abort. Runtime failure
// terminals are produced only by the sealed SM-2/SM-4 path, never by an
// application-side direct Run failure command.
func CancelRun(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID, reason string) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, activationRunCommandSource, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.RequestRuntimeTerminationCommand{WorkspaceID: workspaceID, RunID: runID, Kind: runtimecontract.RuntimeTerminationCancelled, RequestedBy: activationRunCommandSource, Reason: reason})
	ownerevent.BindLane(event, orchestratorevents.TopicOrchestratorRequestRuntimeTermination, "run", string(runID), workspaceID, runID, "")
	response := hub.Send(event)
	if response == nil {
		return common.ErrInvalidArgument
	}
	return response.Error()
}
