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
	if workspaceID == "" || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]*resultmodel.Result, 0)
	cursor := ""
	seen := map[string]struct{}{}
	for page := 0; page < 1000; page++ {
		event := magicevent.NewEventWithContext(resultevents.TopicResultCommandListByRun, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.ListResultsByRunCommand{WorkspaceID: workspaceID, RunID: runID, Cursor: cursor, Limit: resultevents.MaxListLimit})
		bindOwnerCommandLane(event, resultevents.Contracts(), resultevents.TopicResultCommandListByRun, "run", string(runID), workspaceID, runID)
		response, err := resultCommandResponse[resultevents.ListResultsByRunResult](r.hub.Send(event), resultevents.TopicResultCommandListByRun)
		if err != nil {
			return nil, err
		}
		ret = append(ret, resultevents.CloneResults(response.Results)...)
		if response.NextCursor == "" {
			return ret, nil
		}
		if _, ok := seen[response.NextCursor]; ok {
			return nil, common.ErrConflict
		}
		seen[response.NextCursor] = struct{}{}
		cursor = response.NextCursor
	}
	return nil, common.ErrConflict
}

func (r resultEventResultRepository) Delete(ctx context.Context, id common.ResultID) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" || id == "" {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(resultevents.TopicResultCommandDelete, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.DeleteResultCommand{WorkspaceID: workspaceID, ResultID: id})
	bindOwnerCommandLane(event, resultevents.Contracts(), resultevents.TopicResultCommandDelete, "result", string(id), workspaceID, "")
	_, err := resultCommandResponse[resultevents.DeleteResultResult](r.hub.Send(event), resultevents.TopicResultCommandDelete)
	return err
}

type resultEventArtifactRepository struct{ hub magicevent.Hub }

func (r resultEventArtifactRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]*resultmodel.Artifact, 0)
	cursor := ""
	seen := map[string]struct{}{}
	for page := 0; page < 1000; page++ {
		event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandListByRun, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.ListArtifactsByRunCommand{WorkspaceID: workspaceID, RunID: runID, Cursor: cursor, Limit: resultevents.MaxListLimit})
		bindOwnerCommandLane(event, resultevents.Contracts(), resultevents.TopicArtifactCommandListByRun, "run", string(runID), workspaceID, runID)
		response, err := resultCommandResponse[resultevents.ListArtifactsByRunResult](r.hub.Send(event), resultevents.TopicArtifactCommandListByRun)
		if err != nil {
			return nil, err
		}
		ret = append(ret, resultevents.CloneArtifacts(response.Artifacts)...)
		if response.NextCursor == "" {
			return ret, nil
		}
		if _, ok := seen[response.NextCursor]; ok {
			return nil, common.ErrConflict
		}
		seen[response.NextCursor] = struct{}{}
		cursor = response.NextCursor
	}
	return nil, common.ErrConflict
}

func (r resultEventArtifactRepository) Delete(ctx context.Context, id common.ArtifactID) error {
	if r.hub == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" || id == "" {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandDelete, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.DeleteArtifactCommand{WorkspaceID: workspaceID, ArtifactID: id})
	bindOwnerCommandLane(event, resultevents.Contracts(), resultevents.TopicArtifactCommandDelete, "artifact", string(id), workspaceID, "")
	_, err := resultCommandResponse[resultevents.DeleteArtifactResult](r.hub.Send(event), resultevents.TopicArtifactCommandDelete)
	return err
}
