package repository

import (
	"context"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

type ResultWorkspaceDeletionRepository interface {
	Create(context.Context, *resultmodel.WorkspaceDeletion) error
	Get(context.Context, common.WorkspaceID) (*resultmodel.WorkspaceDeletion, error)
	CompareAndSwap(context.Context, *resultmodel.WorkspaceDeletion, uint64) error
	WithWorkspaceActiveFence(context.Context, common.WorkspaceID, func(context.Context) error) error
	WithWorkspacePurgeFence(context.Context, deletioncontract.Scope, func(context.Context) error) error
}
