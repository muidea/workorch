package repository

import (
	"context"

	approval "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ApprovalRepository interface {
	Create(context.Context, *approval.Approval) error
	Get(context.Context, common.ApprovalID) (*approval.Approval, error)
	GetScoped(context.Context, common.WorkspaceID, common.RunID, common.ApprovalID) (*approval.Approval, error)
	List(context.Context) ([]*approval.Approval, error)
	ListByRunID(context.Context, common.RunID) ([]*approval.Approval, error)
	ListByRunScope(context.Context, common.WorkspaceID, common.RunID) ([]*approval.Approval, error)
	ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*approval.Approval, error)
	Update(context.Context, *approval.Approval) error
	Delete(context.Context, common.ApprovalID) error
	DeleteScoped(context.Context, common.WorkspaceID, common.RunID, common.ApprovalID) error
}
