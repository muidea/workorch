package orchestrator

// Result adapter is internal to the Runtime owner Biz.

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type resultEventResultRepository struct {
	hub magicevent.Hub
}

func (r resultEventResultRepository) Create(ctx context.Context, item *resultmodel.Result) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(resultevents.TopicResultCommandCreate, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.CreateResultCommand{Result: item})
	bindOwnerCommandLane(event, resultevents.TopicResultCommandCreate, "result", string(item.ID), item.WorkspaceID, item.RunID)
	_, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicResultCommandCreate)
	return err
}

func (r resultEventResultRepository) Get(ctx context.Context, id common.ResultID) (*resultmodel.Result, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(resultevents.TopicResultCommandGet, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.GetResultCommand{ResultID: id})
	bindOwnerCommandLane(event, resultevents.TopicResultCommandGet, "result", string(id), common.RuntimeWorkspaceIDFromContext(ctx), "")
	response, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicResultCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Result == nil {
		return nil, common.ErrNotFound
	}
	return response.Result, nil
}

func (r resultEventResultRepository) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(resultevents.TopicResultCommandListByWorkspace, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.ListResultsByWorkspaceCommand{WorkspaceID: workspaceID})
	bindOwnerCommandLane(event, resultevents.TopicResultCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "")
	response, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicResultCommandListByWorkspace)
	if err != nil {
		return nil, err
	}
	return response.Results, nil
}

func (r resultEventResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(resultevents.TopicResultCommandListByRun, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.ListResultsByRunCommand{RunID: runID})
	bindOwnerCommandLane(event, resultevents.TopicResultCommandListByRun, "run", string(runID), workspaceID, runID)
	response, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicResultCommandListByRun)
	if err != nil {
		return nil, err
	}
	return response.Results, nil
}

func (r resultEventResultRepository) Update(ctx context.Context, item *resultmodel.Result) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(resultevents.TopicResultCommandUpdate, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.UpdateResultCommand{Result: item})
	bindOwnerCommandLane(event, resultevents.TopicResultCommandUpdate, "result", string(item.ID), item.WorkspaceID, item.RunID)
	_, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicResultCommandUpdate)
	return err
}

func (r resultEventResultRepository) CompareAndSwap(ctx context.Context, item *resultmodel.Result, expectedRevision uint64) error {
	if item == nil || item.Revision != expectedRevision {
		return common.ErrInvalidArgument
	}
	return r.Update(ctx, item)
}

func (r resultEventResultRepository) Delete(ctx context.Context, id common.ResultID) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(resultevents.TopicResultCommandDelete, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.DeleteResultCommand{ResultID: id})
	bindOwnerCommandLane(event, resultevents.TopicResultCommandDelete, "result", string(id), workspaceID, "")
	_, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicResultCommandDelete)
	return err
}

type resultEventArtifactRepository struct {
	hub magicevent.Hub
}

func (r resultEventArtifactRepository) Create(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandCreate, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.CreateArtifactCommand{Artifact: item})
	bindOwnerCommandLane(event, resultevents.TopicArtifactCommandCreate, "artifact", string(item.ID), item.WorkspaceID, item.RunID)
	_, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicArtifactCommandCreate)
	return err
}

func (r resultEventArtifactRepository) Get(ctx context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandGet, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.GetArtifactCommand{ArtifactID: id})
	bindOwnerCommandLane(event, resultevents.TopicArtifactCommandGet, "artifact", string(id), workspaceID, "")
	response, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicArtifactCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Artifact == nil {
		return nil, common.ErrNotFound
	}
	return response.Artifact, nil
}

func (r resultEventArtifactRepository) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandListByWorkspace, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.ListArtifactsByWorkspaceCommand{WorkspaceID: workspaceID})
	bindOwnerCommandLane(event, resultevents.TopicArtifactCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "")
	response, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicArtifactCommandListByWorkspace)
	if err != nil {
		return nil, err
	}
	return response.Artifacts, nil
}

func (r resultEventArtifactRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandListByRun, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.ListArtifactsByRunCommand{RunID: runID})
	bindOwnerCommandLane(event, resultevents.TopicArtifactCommandListByRun, "run", string(runID), workspaceID, runID)
	response, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicArtifactCommandListByRun)
	if err != nil {
		return nil, err
	}
	return response.Artifacts, nil
}

func (r resultEventArtifactRepository) ListByResult(ctx context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandListByResult, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.ListArtifactsByResultCommand{ResultID: resultID})
	bindOwnerCommandLane(event, resultevents.TopicArtifactCommandListByResult, "result", string(resultID), workspaceID, "")
	response, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicArtifactCommandListByResult)
	if err != nil {
		return nil, err
	}
	return response.Artifacts, nil
}

func (r resultEventArtifactRepository) Update(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandUpdate, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.UpdateArtifactCommand{Artifact: item})
	bindOwnerCommandLane(event, resultevents.TopicArtifactCommandUpdate, "artifact", string(item.ID), item.WorkspaceID, item.RunID)
	_, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicArtifactCommandUpdate)
	return err
}

func (r resultEventArtifactRepository) Delete(ctx context.Context, id common.ArtifactID) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandDelete, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.DeleteArtifactCommand{ArtifactID: id})
	bindOwnerCommandLane(event, resultevents.TopicArtifactCommandDelete, "artifact", string(id), workspaceID, "")
	_, err := resultCommandResponse(r.hub.Send(event), resultevents.TopicArtifactCommandDelete)
	return err
}
