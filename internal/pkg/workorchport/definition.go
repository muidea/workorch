package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type GoalBackend interface {
	Define(ctx context.Context, wsID common.WorkspaceID, text string) (*workorchmanagementcontract.Goal, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.Goal, error)
	Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*workorchmanagementcontract.Goal, error)
	Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*workorchmanagementcontract.Goal, error)
}

type DoDBackend interface {
	Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*workorchmanagementcontract.DoD, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.DoD, error)
	Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*workorchmanagementcontract.DoD, error)
	Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*workorchmanagementcontract.DoD, error)
}
