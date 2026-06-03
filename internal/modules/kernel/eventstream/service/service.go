package service

import (
	"context"

	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service interface {
	ListEvents(context.Context, repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)
	PurgeWorkspaceEvents(context.Context, common.WorkspaceID) (int, error)
}

type Dependencies struct {
	ListEvents           func(context.Context, repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)
	PurgeWorkspaceEvents func(context.Context, common.WorkspaceID) (int, error)
}

type Module struct {
	listEvents           func(context.Context, repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)
	purgeWorkspaceEvents func(context.Context, common.WorkspaceID) (int, error)
}

func New(deps Dependencies) *Module {
	return &Module{listEvents: deps.ListEvents, purgeWorkspaceEvents: deps.PurgeWorkspaceEvents}
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
