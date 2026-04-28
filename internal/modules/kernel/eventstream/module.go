package eventstream

import (
	"context"

	eventstreambiz "github.com/muidea/workorch/internal/modules/kernel/eventstream/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/common"
	eventstreamservice "github.com/muidea/workorch/internal/modules/kernel/eventstream/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = eventstreamservice.Service

type Dependencies struct {
	EventRecords repository.EventRecordRepository
	EventHub     events.Hub
}

type Module struct {
	bizPtr     *eventstreambiz.Service
	servicePtr *eventstreamservice.Module
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup(deps Dependencies) error {
	if deps.EventRecords == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = eventstreambiz.NewService(deps.EventRecords, deps.EventHub)
	m.servicePtr = eventstreamservice.New(eventstreamservice.Dependencies{
		ListEvents: m.bizPtr.ListEvents,
	})
	return nil
}

func (m *Module) Run() error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	return m.bizPtr.Start()
}

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	if m.bizPtr != nil {
		m.bizPtr.Stop()
	}
	m.servicePtr = nil
	m.bizPtr = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}
