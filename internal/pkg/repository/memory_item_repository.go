package repository

import (
	"context"

	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type MemorySearchQuery struct {
	WorkspaceID common.WorkspaceID
	Query       string
	Tags        []string
	Types       []string
	Limit       int
}

type MemoryItemRepository interface {
	Create(context.Context, *memorymodel.MemoryItem) error
	Get(context.Context, common.MemoryID) (*memorymodel.MemoryItem, error)
	ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*memorymodel.MemoryItem, error)
	Search(context.Context, MemorySearchQuery) ([]*memorymodel.MemoryItem, error)
	Update(context.Context, *memorymodel.MemoryItem) error
}
