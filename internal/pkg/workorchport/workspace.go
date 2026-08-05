package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type WorkspaceBackend interface {
	Create(ctx context.Context, req workorchmanagementcontract.WorkspaceCreateRequest) (*projectioncontract.Workspace, error)
	Get(ctx context.Context, id common.WorkspaceID) (*projectioncontract.Workspace, error)
	List(ctx context.Context) ([]projectioncontract.Workspace, error)
	Snapshot(ctx context.Context, id common.WorkspaceID) (*projectioncontract.WorkspaceSnapshot, error)
	RuntimeView(ctx context.Context, req ReadWorkspaceRuntimeViewRequest) (*projectioncontract.WorkspaceRuntimeView, error)
	UpdateLLMTarget(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*projectioncontract.Workspace, error)
	Delete(ctx context.Context, id common.WorkspaceID, purge bool) error
}
