package orchestrator

// Result read/delete adapters are internal to Orchestrator cleanup. Runtime
// terminal writes use only the Result owner's typed SM-4 delivery contract.

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type resultEventResultRepository struct{ hub magicevent.Hub }

func (r resultEventResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(resultevents.TopicResultCommandListByRun, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.ListResultsByRunCommand{RunID: runID})
	bindOwnerCommandLane(event, resultevents.Contracts(), resultevents.TopicResultCommandListByRun, "run", string(runID), workspaceID, runID)
	response, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicResultCommandListByRun)
	if err != nil {
		return nil, err
	}
	return resultevents.CloneResults(response.Results), nil
}

func (r resultEventResultRepository) Delete(ctx context.Context, id common.ResultID) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(resultevents.TopicResultCommandDelete, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.DeleteResultCommand{ResultID: id})
	bindOwnerCommandLane(event, resultevents.Contracts(), resultevents.TopicResultCommandDelete, "result", string(id), workspaceID, "")
	_, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicResultCommandDelete)
	return err
}

type resultEventArtifactRepository struct{ hub magicevent.Hub }

func (r resultEventArtifactRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandListByRun, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.ListArtifactsByRunCommand{RunID: runID})
	bindOwnerCommandLane(event, resultevents.Contracts(), resultevents.TopicArtifactCommandListByRun, "run", string(runID), workspaceID, runID)
	response, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicArtifactCommandListByRun)
	if err != nil {
		return nil, err
	}
	return resultevents.CloneArtifacts(response.Artifacts), nil
}

func (r resultEventArtifactRepository) Delete(ctx context.Context, id common.ArtifactID) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandDelete, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.DeleteArtifactCommand{ArtifactID: id})
	bindOwnerCommandLane(event, resultevents.Contracts(), resultevents.TopicArtifactCommandDelete, "artifact", string(id), workspaceID, "")
	_, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicArtifactCommandDelete)
	return err
}
