package memory

import (
	"context"

	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type CreateMemoryItemCommand struct {
	MemoryID    common.MemoryID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	SourceType  string
	SourceRef   string
	MemoryType  string
	Content     string
	Summary     string
	Tags        []string
	Importance  int
	Confidence  int
	Visibility  string
}

type UpdateMemoryItemCommand = CreateMemoryItemCommand

type Service interface {
	CreateMemoryItem(context.Context, CreateMemoryItemCommand) error
	UpdateMemoryItem(context.Context, UpdateMemoryItemCommand) error
	GetMemoryItem(context.Context, common.MemoryID) (*memorymodel.MemoryItem, error)
	ListMemoryByWorkspace(context.Context, common.WorkspaceID) ([]*memorymodel.MemoryItem, error)
	SearchMemory(context.Context, repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error)
}

type Dependencies struct {
	CreateMemoryItem      func(context.Context, CreateMemoryItemCommand) error
	UpdateMemoryItem      func(context.Context, UpdateMemoryItemCommand) error
	GetMemoryItem         func(context.Context, common.MemoryID) (*memorymodel.MemoryItem, error)
	ListMemoryByWorkspace func(context.Context, common.WorkspaceID) ([]*memorymodel.MemoryItem, error)
	SearchMemory          func(context.Context, repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error)
}

type Module struct {
	createMemoryItem      func(context.Context, CreateMemoryItemCommand) error
	updateMemoryItem      func(context.Context, UpdateMemoryItemCommand) error
	getMemoryItem         func(context.Context, common.MemoryID) (*memorymodel.MemoryItem, error)
	listMemoryByWorkspace func(context.Context, common.WorkspaceID) ([]*memorymodel.MemoryItem, error)
	searchMemory          func(context.Context, repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error)
}

func New(deps Dependencies) *Module {
	return &Module{
		createMemoryItem:      deps.CreateMemoryItem,
		updateMemoryItem:      deps.UpdateMemoryItem,
		getMemoryItem:         deps.GetMemoryItem,
		listMemoryByWorkspace: deps.ListMemoryByWorkspace,
		searchMemory:          deps.SearchMemory,
	}
}

func (m *Module) CreateMemoryItem(ctx context.Context, cmd CreateMemoryItemCommand) error {
	if m == nil || m.createMemoryItem == nil {
		return common.ErrInvalidArgument
	}
	return m.createMemoryItem(ctx, cmd)
}

func (m *Module) UpdateMemoryItem(ctx context.Context, cmd UpdateMemoryItemCommand) error {
	if m == nil || m.updateMemoryItem == nil {
		return common.ErrInvalidArgument
	}
	return m.updateMemoryItem(ctx, cmd)
}

func (m *Module) GetMemoryItem(ctx context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	if m == nil || m.getMemoryItem == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.getMemoryItem(ctx, id)
}

func (m *Module) ListMemoryByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	if m == nil || m.listMemoryByWorkspace == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.listMemoryByWorkspace(ctx, workspaceID)
}

func (m *Module) SearchMemory(ctx context.Context, query repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	if m == nil || m.searchMemory == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.searchMemory(ctx, query)
}
