package orchestrator

import (
	"context"
	"fmt"

	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type runEventRepository struct {
	hub events.Hub
}

func (r runEventRepository) Create(ctx context.Context, item *runmodel.Run) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicRunCommandCreate, "run", string(item.ID), item.WorkspaceID, item.ID, map[string]any{
		events.PayloadKeyRun: item,
	})
	return err
}

func (r runEventRepository) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicRunCommandGet, "run", string(id), "", id, map[string]any{
		events.PayloadKeyRunID: string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyRun].(*runmodel.Run)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r runEventRepository) List(ctx context.Context) ([]*runmodel.Run, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicRunCommandList, "run", "all", "", "", nil)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyRuns].([]*runmodel.Run)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r runEventRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicRunCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyRuns].([]*runmodel.Run)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r runEventRepository) Update(ctx context.Context, item *runmodel.Run) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicRunCommandUpdate, "run", string(item.ID), item.WorkspaceID, item.ID, map[string]any{
		events.PayloadKeyRun: item,
	})
	return err
}

func (r runEventRepository) Delete(ctx context.Context, id common.RunID) error {
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicRunCommandDelete, "run", string(id), "", id, map[string]any{
		events.PayloadKeyRunID: string(id),
	})
	return err
}

type workspaceEventRepository struct {
	hub events.Hub
}

func (r workspaceEventRepository) Create(ctx context.Context, item *workspacemodel.Workspace) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicWorkspaceCommandCreate, "workspace", string(item.ID), item.ID, "", map[string]any{
		events.PayloadKeyWorkspace: item,
	})
	return err
}

func (r workspaceEventRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicWorkspaceCommandGet, "workspace", string(id), id, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyWorkspace].(*workspacemodel.Workspace)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r workspaceEventRepository) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicWorkspaceCommandList, "workspace", "all", "", "", nil)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyWorkspaces].([]*workspacemodel.Workspace)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r workspaceEventRepository) Update(ctx context.Context, item *workspacemodel.Workspace) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicWorkspaceCommandUpdate, "workspace", string(item.ID), item.ID, item.ActiveRunID, map[string]any{
		events.PayloadKeyWorkspace: item,
	})
	return err
}

func (r workspaceEventRepository) Delete(ctx context.Context, id common.WorkspaceID) error {
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicWorkspaceCommandDelete, "workspace", string(id), id, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(id),
	})
	return err
}

type memoryEventRepository struct {
	hub events.Hub
}

func (r memoryEventRepository) Create(ctx context.Context, item *memorymodel.MemoryItem) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryCommandCreate, "memory", string(item.ID), item.WorkspaceID, item.RunID, map[string]any{
		events.PayloadKeyMemoryItem: item,
	})
	return err
}

func (r memoryEventRepository) Get(ctx context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryCommandGet, "memory", string(id), "", "", map[string]any{
		events.PayloadKeyMemoryID: string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyMemoryItem].(*memorymodel.MemoryItem)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r memoryEventRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyMemoryItems].([]*memorymodel.MemoryItem)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r memoryEventRepository) Search(ctx context.Context, query repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryCommandSearch, "workspace", string(query.WorkspaceID), query.WorkspaceID, "", map[string]any{
		events.PayloadKeyMemorySearchQuery: query,
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyMemoryItems].([]*memorymodel.MemoryItem)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r memoryEventRepository) Update(ctx context.Context, item *memorymodel.MemoryItem) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryCommandUpdate, "memory", string(item.ID), item.WorkspaceID, item.RunID, map[string]any{
		events.PayloadKeyMemoryItem: item,
	})
	return err
}

func (r memoryEventRepository) Delete(ctx context.Context, id common.MemoryID) error {
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryCommandDelete, "memory", string(id), "", "", map[string]any{
		events.PayloadKeyMemoryID: string(id),
	})
	return err
}

type memoryRecallTraceEventRepository struct {
	hub events.Hub
}

func (r memoryRecallTraceEventRepository) Create(ctx context.Context, item *memorymodel.MemoryRecallTrace) error {
	return r.Upsert(ctx, item)
}

func (r memoryRecallTraceEventRepository) Upsert(ctx context.Context, item *memorymodel.MemoryRecallTrace) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryRecallTraceCommandUpsert, "memory_recall_trace", string(item.ID), item.WorkspaceID, item.RunID, map[string]any{
		events.PayloadKeyMemoryRecallTrace: item,
	})
	return err
}

func (r memoryRecallTraceEventRepository) Get(ctx context.Context, id common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryRecallTraceCommandGet, "memory_recall_trace", string(id), "", "", map[string]any{
		events.PayloadKeyTraceID: string(id),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyMemoryRecallTrace].(*memorymodel.MemoryRecallTrace)
	if !ok || item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r memoryRecallTraceEventRepository) List(ctx context.Context, query repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	response, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryRecallTraceCommandList, "memory_recall_trace", "list", query.WorkspaceID, query.RunID, map[string]any{
		events.PayloadKeyMemoryRecallTraceQuery: query,
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyMemoryRecallTraces].([]*memorymodel.MemoryRecallTrace)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func (r memoryRecallTraceEventRepository) Delete(ctx context.Context, id common.MemoryRecallTraceID) error {
	_, err := publishStateEventCommand(ctx, r.hub, events.TopicMemoryRecallTraceCommandDelete, "memory_recall_trace", string(id), "", "", map[string]any{
		events.PayloadKeyTraceID: string(id),
	})
	return err
}

func publishStateEventCommand(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, payload map[string]any) (events.Envelope, error) {
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
	return events.Envelope{}, fmt.Errorf("state event command %s returned no response", topic)
}
