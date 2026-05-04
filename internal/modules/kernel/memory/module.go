package memory

import (
	"context"

	memorybiz "github.com/muidea/workorch/internal/modules/kernel/memory/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	memoryservice "github.com/muidea/workorch/internal/modules/kernel/memory/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = memoryservice.Service
type CreateMemoryItemCommand = memoryservice.CreateMemoryItemCommand
type UpdateMemoryItemCommand = memoryservice.UpdateMemoryItemCommand

type Dependencies struct {
	MemoryItems repository.MemoryItemRepository
}

type Module struct {
	bizPtr     *memorybiz.Service
	servicePtr *memoryservice.Module
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup(deps Dependencies) error {
	if deps.MemoryItems == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = memorybiz.NewService(deps.MemoryItems)
	m.servicePtr = memoryservice.New(memoryservice.Dependencies{
		CreateMemoryItem:      m.bizPtr.CreateMemoryItem,
		UpdateMemoryItem:      m.bizPtr.UpdateMemoryItem,
		GetMemoryItem:         m.bizPtr.GetMemoryItem,
		DeleteMemoryItem:      m.bizPtr.DeleteMemoryItem,
		ListMemoryByWorkspace: m.bizPtr.ListMemoryByWorkspace,
		SearchMemory:          m.bizPtr.SearchMemory,
	})
	return nil
}

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	m.servicePtr = nil
	m.bizPtr = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}
