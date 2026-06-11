package orchestrator

import (
	"context"
	"fmt"

	resultcontracts "github.com/muidea/workorch/internal/modules/kernel/result/pkg/contracts"
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
	_, err := publishResultEventCommand(ctx, r.hub, resultevents.TopicResultCommandCreate, "result", item.WorkspaceID, item.RunID, string(item.ID), map[string]any{
		events.PayloadKeyCommand: resultcontracts.CreateResultCommand{Result: item},
	})
	return err
}

func (r resultEventResultRepository) Get(ctx context.Context, id common.ResultID) (*resultmodel.Result, error) {
	response, err := publishResultEventCommand(ctx, r.hub, resultevents.TopicResultCommandGet, "result", "", "", string(id), map[string]any{
		events.PayloadKeyCommand: resultcontracts.GetResultCommand{ResultID: id},
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyResult].(*resultmodel.Result)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r resultEventResultRepository) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	response, err := publishResultEventCommand(ctx, r.hub, resultevents.TopicResultCommandListByWorkspace, "workspace", workspaceID, "", string(workspaceID), map[string]any{
		events.PayloadKeyCommand: resultcontracts.ListResultsByWorkspaceCommand{WorkspaceID: workspaceID},
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*resultmodel.Result)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r resultEventResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	response, err := publishResultEventCommand(ctx, r.hub, resultevents.TopicResultCommandListByRun, "run", "", runID, string(runID), map[string]any{
		events.PayloadKeyCommand: resultcontracts.ListResultsByRunCommand{RunID: runID},
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*resultmodel.Result)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r resultEventResultRepository) Update(ctx context.Context, item *resultmodel.Result) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishResultEventCommand(ctx, r.hub, resultevents.TopicResultCommandUpdate, "result", item.WorkspaceID, item.RunID, string(item.ID), map[string]any{
		events.PayloadKeyCommand: resultcontracts.UpdateResultCommand{Result: item},
	})
	return err
}

func (r resultEventResultRepository) Delete(ctx context.Context, id common.ResultID) error {
	_, err := publishResultEventCommand(ctx, r.hub, resultevents.TopicResultCommandDelete, "result", "", "", string(id), map[string]any{
		events.PayloadKeyCommand: resultcontracts.DeleteResultCommand{ResultID: id},
	})
	return err
}

type resultEventArtifactRepository struct {
	hub events.Hub
}

func (r resultEventArtifactRepository) Create(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishResultEventCommand(ctx, r.hub, resultevents.TopicArtifactCommandCreate, "artifact", item.WorkspaceID, item.RunID, string(item.ID), map[string]any{
		events.PayloadKeyCommand: resultcontracts.CreateArtifactCommand{Artifact: item},
	})
	return err
}

func (r resultEventArtifactRepository) Get(ctx context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	response, err := publishResultEventCommand(ctx, r.hub, resultevents.TopicArtifactCommandGet, "artifact", "", "", string(id), map[string]any{
		events.PayloadKeyCommand: resultcontracts.GetArtifactCommand{ArtifactID: id},
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyArtifact].(*resultmodel.Artifact)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r resultEventArtifactRepository) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	response, err := publishResultEventCommand(ctx, r.hub, resultevents.TopicArtifactCommandListByWorkspace, "workspace", workspaceID, "", string(workspaceID), map[string]any{
		events.PayloadKeyCommand: resultcontracts.ListArtifactsByWorkspaceCommand{WorkspaceID: workspaceID},
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*resultmodel.Artifact)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r resultEventArtifactRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	response, err := publishResultEventCommand(ctx, r.hub, resultevents.TopicArtifactCommandListByRun, "run", "", runID, string(runID), map[string]any{
		events.PayloadKeyCommand: resultcontracts.ListArtifactsByRunCommand{RunID: runID},
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*resultmodel.Artifact)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r resultEventArtifactRepository) ListByResult(ctx context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	response, err := publishResultEventCommand(ctx, r.hub, resultevents.TopicArtifactCommandListByResult, "result", "", "", string(resultID), map[string]any{
		events.PayloadKeyCommand: resultcontracts.ListArtifactsByResultCommand{ResultID: resultID},
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*resultmodel.Artifact)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r resultEventArtifactRepository) Update(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishResultEventCommand(ctx, r.hub, resultevents.TopicArtifactCommandUpdate, "artifact", item.WorkspaceID, item.RunID, string(item.ID), map[string]any{
		events.PayloadKeyCommand: resultcontracts.UpdateArtifactCommand{Artifact: item},
	})
	return err
}

func (r resultEventArtifactRepository) Delete(ctx context.Context, id common.ArtifactID) error {
	_, err := publishResultEventCommand(ctx, r.hub, resultevents.TopicArtifactCommandDelete, "artifact", "", "", string(id), map[string]any{
		events.PayloadKeyCommand: resultcontracts.DeleteArtifactCommand{ArtifactID: id},
	})
	return err
}

func publishResultEventCommand(ctx context.Context, hub events.Hub, topic string, subjectType string, workspaceID common.WorkspaceID, runID common.RunID, subjectID string, payload map[string]any) (events.Envelope, error) {
	if hub == nil || topic == "" {
		return events.Envelope{}, common.ErrInvalidArgument
	}
	if workspaceID == "" {
		workspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	if workspaceID != "" {
		if payload == nil {
			payload = map[string]any{}
		}
		payload[events.PayloadKeyWorkspaceID] = string(workspaceID)
	}
	err := hub.Publish(ctx, events.Envelope{
		Topic:        topic,
		SourceModule: ModuleID,
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		Payload:      payload,
	})
	if response, ok := events.ResponseEnvelope(err); ok {
		return response, nil
	}
	if err != nil {
		return events.Envelope{}, err
	}
	return events.Envelope{}, fmt.Errorf("result event command %s returned no response", topic)
}
