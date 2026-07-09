package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type resultEventResultRepository struct {
	hub events.Hub
}

func (r resultEventResultRepository) Create(ctx context.Context, item *resultmodel.Result) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendResultEventCommand(ctx, r.hub, resultevents.TopicResultCommandCreate, "result", item.WorkspaceID, item.RunID, string(item.ID), resultevents.CreateResultCommand{Result: item})
	return err
}

func (r resultEventResultRepository) Get(ctx context.Context, id common.ResultID) (*resultmodel.Result, error) {
	response, err := sendResultEventCommand(ctx, r.hub, resultevents.TopicResultCommandGet, "result", "", "", string(id), resultevents.GetResultCommand{ResultID: id})
	if err != nil {
		return nil, err
	}
	if response.Result == nil {
		return nil, common.ErrNotFound
	}
	return response.Result, nil
}

func (r resultEventResultRepository) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	response, err := sendResultEventCommand(ctx, r.hub, resultevents.TopicResultCommandListByWorkspace, "workspace", workspaceID, "", string(workspaceID), resultevents.ListResultsByWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return response.Results, nil
}

func (r resultEventResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	response, err := sendResultEventCommand(ctx, r.hub, resultevents.TopicResultCommandListByRun, "run", "", runID, string(runID), resultevents.ListResultsByRunCommand{RunID: runID})
	if err != nil {
		return nil, err
	}
	return response.Results, nil
}

func (r resultEventResultRepository) Update(ctx context.Context, item *resultmodel.Result) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendResultEventCommand(ctx, r.hub, resultevents.TopicResultCommandUpdate, "result", item.WorkspaceID, item.RunID, string(item.ID), resultevents.UpdateResultCommand{Result: item})
	return err
}

func (r resultEventResultRepository) Delete(ctx context.Context, id common.ResultID) error {
	_, err := sendResultEventCommand(ctx, r.hub, resultevents.TopicResultCommandDelete, "result", "", "", string(id), resultevents.DeleteResultCommand{ResultID: id})
	return err
}

type resultEventArtifactRepository struct {
	hub events.Hub
}

func (r resultEventArtifactRepository) Create(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendResultEventCommand(ctx, r.hub, resultevents.TopicArtifactCommandCreate, "artifact", item.WorkspaceID, item.RunID, string(item.ID), resultevents.CreateArtifactCommand{Artifact: item})
	return err
}

func (r resultEventArtifactRepository) Get(ctx context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	response, err := sendResultEventCommand(ctx, r.hub, resultevents.TopicArtifactCommandGet, "artifact", "", "", string(id), resultevents.GetArtifactCommand{ArtifactID: id})
	if err != nil {
		return nil, err
	}
	if response.Artifact == nil {
		return nil, common.ErrNotFound
	}
	return response.Artifact, nil
}

func (r resultEventArtifactRepository) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	response, err := sendResultEventCommand(ctx, r.hub, resultevents.TopicArtifactCommandListByWorkspace, "workspace", workspaceID, "", string(workspaceID), resultevents.ListArtifactsByWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return response.Artifacts, nil
}

func (r resultEventArtifactRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	response, err := sendResultEventCommand(ctx, r.hub, resultevents.TopicArtifactCommandListByRun, "run", "", runID, string(runID), resultevents.ListArtifactsByRunCommand{RunID: runID})
	if err != nil {
		return nil, err
	}
	return response.Artifacts, nil
}

func (r resultEventArtifactRepository) ListByResult(ctx context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	response, err := sendResultEventCommand(ctx, r.hub, resultevents.TopicArtifactCommandListByResult, "result", "", "", string(resultID), resultevents.ListArtifactsByResultCommand{ResultID: resultID})
	if err != nil {
		return nil, err
	}
	return response.Artifacts, nil
}

func (r resultEventArtifactRepository) Update(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := sendResultEventCommand(ctx, r.hub, resultevents.TopicArtifactCommandUpdate, "artifact", item.WorkspaceID, item.RunID, string(item.ID), resultevents.UpdateArtifactCommand{Artifact: item})
	return err
}

func (r resultEventArtifactRepository) Delete(ctx context.Context, id common.ArtifactID) error {
	_, err := sendResultEventCommand(ctx, r.hub, resultevents.TopicArtifactCommandDelete, "artifact", "", "", string(id), resultevents.DeleteArtifactCommand{ArtifactID: id})
	return err
}

type resultRawMagicHubProvider interface {
	RawHub() magicevent.Hub
}

func sendResultEventCommand[Command any](ctx context.Context, hub events.Hub, topic string, subjectType string, workspaceID common.WorkspaceID, runID common.RunID, subjectID string, command Command) (resultevents.ResultCommandResult, error) {
	if hub == nil || topic == "" {
		return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
	}
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	provider, ok := hub.(resultRawMagicHubProvider)
	if !ok || provider.RawHub() == nil {
		return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(topic, ModuleID, resultevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(events.EventHubLaneKey(events.Envelope{
		Topic:       topic,
		SubjectType: subjectType,
		SubjectID:   subjectID,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))
	response := provider.RawHub().Send(ev)
	if response == nil {
		return resultevents.ResultCommandResult{}, fmt.Errorf("result event command %s returned no response", topic)
	}
	if err := response.Error(); err != nil {
		return resultevents.ResultCommandResult{}, err
	}
	data, err := response.Get()
	if err != nil {
		return resultevents.ResultCommandResult{}, err
	}
	result, ok := data.(resultevents.ResultCommandResult)
	if !ok {
		return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
	}
	return result, nil
}
