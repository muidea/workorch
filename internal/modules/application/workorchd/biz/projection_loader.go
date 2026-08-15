package biz

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
)

// applicationProjectionLoader adapts the already-validated application read
// ports to the Biz-owned projection component. It carries no Session, Hub, or
// repository and cannot issue an owner write command.
type applicationProjectionLoader struct {
	config server.Config
	ports  server.WorkspacePorts
}

func (l applicationProjectionLoader) ListWorkspaces(ctx context.Context) ([]appmodelcore.Workspace, error) {
	return l.ports.ListWorkspaces(ctx, l.config.Backend, l.config.DataRoot)
}

func (l applicationProjectionLoader) ReadWorkspace(ctx context.Context, workspaceID string) (appmodelcore.Workspace, error) {
	return l.ports.ReadWorkspace(ctx, l.config.Backend, l.config.DataRoot, workspaceID)
}

func (l applicationProjectionLoader) ReadWorkspaceSnapshot(ctx context.Context, workspaceID string) (readmodelcore.WorkspaceSnapshotState, error) {
	return l.ports.ReadProjectionSnapshot(ctx, l.config.Backend, l.config.DataRoot, workspaceID)
}
