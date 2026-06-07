package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadWorkspaceSoulBindingsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret WorkspaceSoulBindings, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (WorkspaceSoulBindings, error) {
		return workspaceports.ReadWorkspaceSoulBindings(ctx, hub, workspaceID)
	})
}

func ReadWorkspaceSoulBindingsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (WorkspaceSoulBindings, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (WorkspaceSoulBindings, error) {
		return ReadWorkspaceSoulBindingsWithSession(ctx, session, workspaceID)
	})
}

func UpdateWorkspaceSoulBindingsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, cmd WorkspaceSoulBindings) (ret WorkspaceSoulBindings, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (WorkspaceSoulBindings, error) {
		return workspaceports.UpdateWorkspaceSoulBindingsView(ctx, hub, workspaceID, cmd)
	})
}

func UpdateWorkspaceSoulBindingsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd WorkspaceSoulBindings) (WorkspaceSoulBindings, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (WorkspaceSoulBindings, error) {
		return UpdateWorkspaceSoulBindingsWithSession(ctx, session, workspaceID, cmd)
	})
}
