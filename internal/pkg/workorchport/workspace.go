package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type WorkspaceBackend interface {
	Create(ctx context.Context, req workorchmanagementcontract.WorkspaceCreateRequest) (*projectioncontract.Workspace, error)
	Get(ctx context.Context, id common.WorkspaceID) (*projectioncontract.Workspace, error)
	List(ctx context.Context) ([]projectioncontract.Workspace, error)
	Snapshot(ctx context.Context, id common.WorkspaceID) (*projectioncontract.WorkspaceSnapshot, error)
	RuntimeView(ctx context.Context, query workorchquerycontract.WorkspaceRuntimeViewQuery) (*projectioncontract.WorkspaceRuntimeView, error)
	UpdateLLMSelection(ctx context.Context, id common.WorkspaceID, selection *runtimecontract.ProviderInferenceSelection, expectedRevision uint64) (*projectioncontract.Workspace, error)
	Delete(ctx context.Context, id common.WorkspaceID, purge bool) error
}
