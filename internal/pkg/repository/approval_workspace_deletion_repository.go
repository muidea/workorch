package repository

import (
	"context"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

type ApprovalWorkspaceDeletionRepository interface {
	Create(context.Context, *approvalmodel.WorkspaceDeletion) error
	Get(context.Context, common.WorkspaceID) (*approvalmodel.WorkspaceDeletion, error)
	CompareAndSwap(context.Context, *approvalmodel.WorkspaceDeletion, uint64) error
	WithWorkspaceActiveFence(context.Context, common.WorkspaceID, func(context.Context) error) error
	WithWorkspacePurgeFence(context.Context, deletioncontract.Scope, func(context.Context) error) error
}
