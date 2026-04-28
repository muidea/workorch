package service

import (
	"context"

	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service interface {
	ListEvents(context.Context, repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)
}

type Dependencies struct {
	ListEvents func(context.Context, repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)
}

type Module struct {
	listEvents func(context.Context, repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)
}

func New(deps Dependencies) *Module {
	return &Module{listEvents: deps.ListEvents}
}

func (m *Module) ListEvents(ctx context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	return m.listEvents(ctx, query)
}
