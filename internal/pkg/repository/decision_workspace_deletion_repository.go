package repository

import (
	"context"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

type DecisionWorkspaceDeletionRepository interface {
	Create(context.Context, *decisionmodel.WorkspaceDeletion) error
	Get(context.Context, common.WorkspaceID) (*decisionmodel.WorkspaceDeletion, error)
	CompareAndSwap(context.Context, *decisionmodel.WorkspaceDeletion, uint64) error
	WithWorkspaceActiveFence(context.Context, common.WorkspaceID, func(context.Context) error) error
	WithWorkspacePurgeFence(context.Context, deletioncontract.Scope, func(context.Context) error) error
}
