package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func DefineDoDWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, items []string) (ret DoD, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (DoD, error) {
		return definitionports.DefineOrConsolidateWorkspaceDoDView(ctx, hub, common.WorkspaceID(workspaceID), items)
	})
}

func DefineDoDWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, items []string) (DoD, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (DoD, error) {
		return DefineDoDWithSession(ctx, session, workspaceID, items)
	})
}

func DefineDoDFromEnv(ctx context.Context, workspaceID string, items []string) (DoD, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (DoD, error) {
		return DefineDoDWithSession(ctx, session, workspaceID, items)
	})
}

func UpdateDoDWithSession(ctx context.Context, session *RuntimeSession, dodID string, items []string) (ret DoD, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (DoD, error) {
		return definitionports.UpdateDoDView(ctx, hub, common.DoDID(dodID), items)
	})
}

func UpdateDoDWithBackend(ctx context.Context, backend string, dataRoot string, dodID string, items []string) (DoD, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (DoD, error) {
		return UpdateDoDWithSession(ctx, session, dodID, items)
	})
}

func UpdateDoDFromEnv(ctx context.Context, dodID string, items []string) (DoD, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (DoD, error) {
		return UpdateDoDWithSession(ctx, session, dodID, items)
	})
}

func ReadDoDWithSession(ctx context.Context, session *RuntimeSession, dodID string) (ret DoD, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (DoD, error) {
		return definitionports.ReadDoDView(ctx, hub, common.DoDID(dodID))
	})
}

func ReadDoDWithBackend(ctx context.Context, backend string, dataRoot string, dodID string) (DoD, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (DoD, error) {
		return ReadDoDWithSession(ctx, session, dodID)
	})
}

func ListDoDsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret []DoD, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) ([]DoD, error) {
		return definitionports.ListConsolidatedWorkspaceDoDViews(ctx, hub, common.WorkspaceID(workspaceID))
	})
}

func ListDoDsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]DoD, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]DoD, error) {
		return ListDoDsWithSession(ctx, session, workspaceID)
	})
}
