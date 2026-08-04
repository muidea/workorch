package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func ReadWorkspaceSoulBindingsWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string) (ret workorchmanagementcontract.WorkspaceSoulBindings, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.WorkspaceSoulBindings, error) {
		return workspaceports.ReadWorkspaceSoulBindings(ctx, hub, workspaceID)
	})
}

func ReadWorkspaceSoulBindingsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (workorchmanagementcontract.WorkspaceSoulBindings, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.WorkspaceSoulBindings, error) {
		return ReadWorkspaceSoulBindingsWithSession(ctx, session, workspaceID)
	})
}

func UpdateWorkspaceSoulBindingsWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string, cmd workorchmanagementcontract.WorkspaceSoulBindings) (ret workorchmanagementcontract.WorkspaceSoulBindings, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.WorkspaceSoulBindings, error) {
		return workspaceports.UpdateWorkspaceSoulBindingsView(ctx, hub, workspaceID, cmd)
	})
}

func UpdateWorkspaceSoulBindingsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd workorchmanagementcontract.WorkspaceSoulBindings) (workorchmanagementcontract.WorkspaceSoulBindings, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.WorkspaceSoulBindings, error) {
		return UpdateWorkspaceSoulBindingsWithSession(ctx, session, workspaceID, cmd)
	})
}
