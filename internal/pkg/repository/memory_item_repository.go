package repository

import (
	"context"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type MemoryItemRepository interface {
	Create(context.Context, *memorymodel.MemoryItem) error
	Get(context.Context, common.MemoryID) (*memorymodel.MemoryItem, error)
	ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*memorymodel.MemoryItem, error)
	Search(context.Context, workorchquerycontract.MemorySearchQuery) ([]*memorymodel.MemoryItem, error)
	Update(context.Context, *memorymodel.MemoryItem) error
	Delete(context.Context, common.MemoryID) error
}
