package memory

import (
	"context"

	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type CreateMemoryItemCommand struct {
	MemoryID            common.MemoryID
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	SourceType          string
	SourceRef           string
	MemoryType          string
	Content             string
	Summary             string
	Tags                []string
	Importance          int
	Confidence          int
	Visibility          string
	EmbeddingProviderID string
	EmbeddingModel      string
	EmbeddingText       string
	Vector              []float64
}

type UpdateMemoryItemCommand = CreateMemoryItemCommand

type Service interface {
	CreateMemoryItem(context.Context, CreateMemoryItemCommand) error
	UpdateMemoryItem(context.Context, UpdateMemoryItemCommand) error
	GetMemoryItem(context.Context, common.MemoryID) (*memorymodel.MemoryItem, error)
	DeleteMemoryItem(context.Context, common.MemoryID) error
	ListMemoryByWorkspace(context.Context, common.WorkspaceID) ([]*memorymodel.MemoryItem, error)
	SearchMemory(context.Context, repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error)
	UpsertMemoryRecallTrace(context.Context, *memorymodel.MemoryRecallTrace) error
	GetMemoryRecallTrace(context.Context, common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error)
	ListMemoryRecallTraces(context.Context, repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error)
	DeleteMemoryRecallTrace(context.Context, common.MemoryRecallTraceID) error
}

type Dependencies struct {
	CreateMemoryItem      func(context.Context, CreateMemoryItemCommand) error
	UpdateMemoryItem      func(context.Context, UpdateMemoryItemCommand) error
	GetMemoryItem         func(context.Context, common.MemoryID) (*memorymodel.MemoryItem, error)
	DeleteMemoryItem      func(context.Context, common.MemoryID) error
	ListMemoryByWorkspace func(context.Context, common.WorkspaceID) ([]*memorymodel.MemoryItem, error)
	SearchMemory          func(context.Context, repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error)
	UpsertRecallTrace     func(context.Context, *memorymodel.MemoryRecallTrace) error
	GetRecallTrace        func(context.Context, common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error)
	ListRecallTraces      func(context.Context, repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error)
	DeleteRecallTrace     func(context.Context, common.MemoryRecallTraceID) error
}

type Module struct {
	createMemoryItem      func(context.Context, CreateMemoryItemCommand) error
	updateMemoryItem      func(context.Context, UpdateMemoryItemCommand) error
	getMemoryItem         func(context.Context, common.MemoryID) (*memorymodel.MemoryItem, error)
	deleteMemoryItem      func(context.Context, common.MemoryID) error
	listMemoryByWorkspace func(context.Context, common.WorkspaceID) ([]*memorymodel.MemoryItem, error)
	searchMemory          func(context.Context, repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error)
	upsertRecallTrace     func(context.Context, *memorymodel.MemoryRecallTrace) error
	getRecallTrace        func(context.Context, common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error)
	listRecallTraces      func(context.Context, repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error)
	deleteRecallTrace     func(context.Context, common.MemoryRecallTraceID) error
}

func New(deps Dependencies) *Module {
	return &Module{
		createMemoryItem:      deps.CreateMemoryItem,
		updateMemoryItem:      deps.UpdateMemoryItem,
		getMemoryItem:         deps.GetMemoryItem,
		deleteMemoryItem:      deps.DeleteMemoryItem,
		listMemoryByWorkspace: deps.ListMemoryByWorkspace,
		searchMemory:          deps.SearchMemory,
		upsertRecallTrace:     deps.UpsertRecallTrace,
		getRecallTrace:        deps.GetRecallTrace,
		listRecallTraces:      deps.ListRecallTraces,
		deleteRecallTrace:     deps.DeleteRecallTrace,
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

func (m *Module) DeleteMemoryItem(ctx context.Context, id common.MemoryID) error {
	if m == nil || m.deleteMemoryItem == nil {
		return common.ErrInvalidArgument
	}
	return m.deleteMemoryItem(ctx, id)
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

func (m *Module) UpsertMemoryRecallTrace(ctx context.Context, trace *memorymodel.MemoryRecallTrace) error {
	if m == nil || m.upsertRecallTrace == nil {
		return common.ErrInvalidArgument
	}
	return m.upsertRecallTrace(ctx, trace)
}

func (m *Module) GetMemoryRecallTrace(ctx context.Context, id common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error) {
	if m == nil || m.getRecallTrace == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.getRecallTrace(ctx, id)
}

func (m *Module) ListMemoryRecallTraces(ctx context.Context, query repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	if m == nil || m.listRecallTraces == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.listRecallTraces(ctx, query)
}

func (m *Module) DeleteMemoryRecallTrace(ctx context.Context, id common.MemoryRecallTraceID) error {
	if m == nil || m.deleteRecallTrace == nil {
		return common.ErrInvalidArgument
	}
	return m.deleteRecallTrace(ctx, id)
}
