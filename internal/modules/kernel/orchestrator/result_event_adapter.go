package orchestrator

import (
	"context"
	"fmt"

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
	_, err := publishResultEventCommand(ctx, r.hub, events.TopicResultCommandCreate, "result", item.WorkspaceID, item.RunID, string(item.ID), map[string]any{
		events.PayloadKeyResult: item,
	})
	return err
}

func (r resultEventResultRepository) Get(ctx context.Context, id common.ResultID) (*resultmodel.Result, error) {
	response, err := publishResultEventCommand(ctx, r.hub, events.TopicResultCommandGet, "result", "", "", string(id), map[string]any{
		events.PayloadKeyResultID: string(id),
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
	response, err := publishResultEventCommand(ctx, r.hub, events.TopicResultCommandListByWorkspace, "workspace", workspaceID, "", string(workspaceID), map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
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
	response, err := publishResultEventCommand(ctx, r.hub, events.TopicResultCommandListByRun, "run", "", runID, string(runID), map[string]any{
		events.PayloadKeyRunID: string(runID),
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
	_, err := publishResultEventCommand(ctx, r.hub, events.TopicResultCommandUpdate, "result", item.WorkspaceID, item.RunID, string(item.ID), map[string]any{
		events.PayloadKeyResult: item,
	})
	return err
}

func (r resultEventResultRepository) Delete(ctx context.Context, id common.ResultID) error {
	_, err := publishResultEventCommand(ctx, r.hub, events.TopicResultCommandDelete, "result", "", "", string(id), map[string]any{
		events.PayloadKeyResultID: string(id),
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
	_, err := publishResultEventCommand(ctx, r.hub, events.TopicArtifactCommandCreate, "artifact", item.WorkspaceID, item.RunID, string(item.ID), map[string]any{
		events.PayloadKeyArtifact: item,
	})
	return err
}

func (r resultEventArtifactRepository) Get(ctx context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	response, err := publishResultEventCommand(ctx, r.hub, events.TopicArtifactCommandGet, "artifact", "", "", string(id), map[string]any{
		events.PayloadKeyArtifactID: string(id),
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
	response, err := publishResultEventCommand(ctx, r.hub, events.TopicArtifactCommandListByWorkspace, "workspace", workspaceID, "", string(workspaceID), map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
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
	response, err := publishResultEventCommand(ctx, r.hub, events.TopicArtifactCommandListByRun, "run", "", runID, string(runID), map[string]any{
		events.PayloadKeyRunID: string(runID),
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
	response, err := publishResultEventCommand(ctx, r.hub, events.TopicArtifactCommandListByResult, "result", "", "", string(resultID), map[string]any{
		events.PayloadKeyResultID: string(resultID),
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
	_, err := publishResultEventCommand(ctx, r.hub, events.TopicArtifactCommandUpdate, "artifact", item.WorkspaceID, item.RunID, string(item.ID), map[string]any{
		events.PayloadKeyArtifact: item,
	})
	return err
}

func (r resultEventArtifactRepository) Delete(ctx context.Context, id common.ArtifactID) error {
	_, err := publishResultEventCommand(ctx, r.hub, events.TopicArtifactCommandDelete, "artifact", "", "", string(id), map[string]any{
		events.PayloadKeyArtifactID: string(id),
	})
	return err
}

func publishResultEventCommand(ctx context.Context, hub events.Hub, topic string, subjectType string, workspaceID common.WorkspaceID, runID common.RunID, subjectID string, payload map[string]any) (events.Envelope, error) {
	if hub == nil || topic == "" {
		return events.Envelope{}, common.ErrInvalidArgument
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
