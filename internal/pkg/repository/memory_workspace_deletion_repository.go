package repository

import (
	"context"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

type MemoryWorkspaceDeletionRepository interface {
	Create(context.Context, *memorymodel.WorkspaceDeletion) error
	Get(context.Context, common.WorkspaceID) (*memorymodel.WorkspaceDeletion, error)
	CompareAndSwap(context.Context, *memorymodel.WorkspaceDeletion, uint64) error
	WithWorkspaceActiveFence(context.Context, common.WorkspaceID, func(context.Context) error) error
	WithWorkspacePurgeFence(context.Context, deletioncontract.Scope, func(context.Context) error) error
}
