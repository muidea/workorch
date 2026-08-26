package memory

import (
	"context"
	"sync"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

// EventStreamWorkspaceDeletionRepository is shared by every repository in one
// memory-backed Eventstream owner set. Its lock is the linearization guard for
// append, seal, ordinary mutation, and exact purge.
type EventStreamWorkspaceDeletionRepository struct {
	mu    sync.RWMutex
	items map[common.WorkspaceID]*eventstreammodel.WorkspaceDeletion
}

func NewEventStreamWorkspaceDeletionRepository() *EventStreamWorkspaceDeletionRepository {
	return &EventStreamWorkspaceDeletionRepository{items: map[common.WorkspaceID]*eventstreammodel.WorkspaceDeletion{}}
}

func (r *EventStreamWorkspaceDeletionRepository) Create(_ context.Context, item *eventstreammodel.WorkspaceDeletion) error {
	if r == nil || item == nil || item.Validate() != nil || item.Revision != 1 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items == nil {
		r.items = map[common.WorkspaceID]*eventstreammodel.WorkspaceDeletion{}
	}
	if r.items[item.WorkspaceID] != nil {
		return common.ErrConflict
	}
	r.items[item.WorkspaceID] = eventstreammodel.CloneWorkspaceDeletion(item)
	return nil
}

func (r *EventStreamWorkspaceDeletionRepository) Get(_ context.Context, workspaceID common.WorkspaceID) (*eventstreammodel.WorkspaceDeletion, error) {
	if r == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item := r.items[workspaceID]
	if item == nil {
		return nil, common.ErrNotFound
	}
	if item.Validate() != nil {
		return nil, common.ErrConflict
	}
	return eventstreammodel.CloneWorkspaceDeletion(item), nil
}

func (r *EventStreamWorkspaceDeletionRepository) CompareAndSwap(_ context.Context, item *eventstreammodel.WorkspaceDeletion, expected uint64) error {
	if r == nil || item == nil || item.Validate() != nil || expected == 0 || item.Revision != expected+1 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.items[item.WorkspaceID]
	if current == nil || current.Validate() != nil || current.Revision != expected || current.Scope() != item.Scope() ||
		current.FenceID != item.FenceID || current.SealAckHash != item.SealAckHash || current.RequestedAt != item.RequestedAt {
		return common.ErrConflict
	}
	r.items[item.WorkspaceID] = eventstreammodel.CloneWorkspaceDeletion(item)
	return nil
}

func (r *EventStreamWorkspaceDeletionRepository) WithWorkspaceActiveFence(ctx context.Context, workspaceID common.WorkspaceID, operation func(context.Context) error) error {
	if r == nil || workspaceID == "" || operation == nil {
		return common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.items[workspaceID] != nil {
		return common.ErrNotFound
	}
	return operation(context.WithValue(ctx, workspaceDeletionGuardContextKey{}, workspaceDeletionGuardContext{owner: "eventstream", workspaceID: workspaceID}))
}

func (r *EventStreamWorkspaceDeletionRepository) WithWorkspacePurgeFence(ctx context.Context, expected eventstreammodel.WorkspaceDeletion, operation func(context.Context) error) error {
	if r == nil || expected.Validate() != nil || expected.Phase != eventstreammodel.WorkspaceDeletionPhaseFenced || expected.Mode != deletioncontract.ModePurge || operation == nil {
		return common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item := r.items[expected.WorkspaceID]
	if item == nil {
		return common.ErrNotFound
	}
	if *item != expected {
		return common.ErrConflict
	}
	return operation(context.WithValue(ctx, workspaceDeletionGuardContextKey{}, workspaceDeletionGuardContext{owner: "eventstream", workspaceID: expected.WorkspaceID}))
}
