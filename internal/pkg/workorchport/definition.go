package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type GoalInfo struct {
	ID          common.GoalID
	WorkspaceID common.WorkspaceID
	Text        string
}

type GoalBackend interface {
	Define(ctx context.Context, wsID common.WorkspaceID, text string) (*GoalInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]GoalInfo, error)
	Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*GoalInfo, error)
	Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*GoalInfo, error)
}

type DoDInfo struct {
	ID          common.DoDID
	WorkspaceID common.WorkspaceID
	Items       []string
	State       string
}

type DoDBackend interface {
	Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*DoDInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]DoDInfo, error)
	Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*DoDInfo, error)
	Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*DoDInfo, error)
}
