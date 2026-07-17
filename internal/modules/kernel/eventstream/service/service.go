package service

import (
	"context"

	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service interface {
	ListEvents(context.Context, repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)
	HighWatermarks(context.Context, []common.WorkspaceID) ([]eventstreamevents.WorkspaceHighWatermark, error)
	PurgeWorkspaceEvents(context.Context, common.WorkspaceID) (int, error)
}

type Dependencies struct {
	ListEvents           func(context.Context, repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)
	HighWatermarks       func(context.Context, []common.WorkspaceID) ([]eventstreamevents.WorkspaceHighWatermark, error)
	PurgeWorkspaceEvents func(context.Context, common.WorkspaceID) (int, error)
}

type Module struct {
	listEvents           func(context.Context, repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)
	highWatermarks       func(context.Context, []common.WorkspaceID) ([]eventstreamevents.WorkspaceHighWatermark, error)
	purgeWorkspaceEvents func(context.Context, common.WorkspaceID) (int, error)
}

func New(deps Dependencies) *Module {
	return &Module{listEvents: deps.ListEvents, highWatermarks: deps.HighWatermarks, purgeWorkspaceEvents: deps.PurgeWorkspaceEvents}
}

func (m *Module) HighWatermarks(ctx context.Context, workspaceIDs []common.WorkspaceID) ([]eventstreamevents.WorkspaceHighWatermark, error) {
	if m == nil || m.highWatermarks == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.highWatermarks(ctx, workspaceIDs)
}

func (m *Module) ListEvents(ctx context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	if m == nil || m.listEvents == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.listEvents(ctx, query)
}

func (m *Module) PurgeWorkspaceEvents(ctx context.Context, workspaceID common.WorkspaceID) (int, error) {
	if m == nil || m.purgeWorkspaceEvents == nil {
		return 0, common.ErrInvalidArgument
	}
	return m.purgeWorkspaceEvents(ctx, workspaceID)
}
