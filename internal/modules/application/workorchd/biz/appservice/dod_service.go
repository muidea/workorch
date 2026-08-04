package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func DefineDoDWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string, items []string) (ret workorchmanagementcontract.DoD, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.DoD, error) {
		return definitionports.DefineOrConsolidateWorkspaceDoDView(ctx, hub, common.WorkspaceID(workspaceID), items)
	})
}

func DefineDoDWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, items []string) (workorchmanagementcontract.DoD, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.DoD, error) {
		return DefineDoDWithSession(ctx, session, workspaceID, items)
	})
}

func DefineDoDFromEnv(ctx context.Context, workspaceID string, items []string) (workorchmanagementcontract.DoD, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.DoD, error) {
		return DefineDoDWithSession(ctx, session, workspaceID, items)
	})
}

func UpdateDoDWithSession(ctx context.Context, session *runtimecore.Session, dodID string, items []string) (ret workorchmanagementcontract.DoD, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.DoD, error) {
		return definitionports.UpdateDoDView(ctx, hub, common.DoDID(dodID), items)
	})
}

func UpdateDoDWithBackend(ctx context.Context, backend string, dataRoot string, dodID string, items []string) (workorchmanagementcontract.DoD, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.DoD, error) {
		return UpdateDoDWithSession(ctx, session, dodID, items)
	})
}

func UpdateDoDFromEnv(ctx context.Context, dodID string, items []string) (workorchmanagementcontract.DoD, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.DoD, error) {
		return UpdateDoDWithSession(ctx, session, dodID, items)
	})
}

func ReadDoDWithSession(ctx context.Context, session *runtimecore.Session, dodID string) (ret workorchmanagementcontract.DoD, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.DoD, error) {
		return definitionports.ReadDoDView(ctx, hub, common.DoDID(dodID))
	})
}

func ReadDoDWithBackend(ctx context.Context, backend string, dataRoot string, dodID string) (workorchmanagementcontract.DoD, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.DoD, error) {
		return ReadDoDWithSession(ctx, session, dodID)
	})
}

func ListDoDsWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string) (ret []workorchmanagementcontract.DoD, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) ([]workorchmanagementcontract.DoD, error) {
		return definitionports.ListConsolidatedWorkspaceDoDViews(ctx, hub, common.WorkspaceID(workspaceID))
	})
}

func ListDoDsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]workorchmanagementcontract.DoD, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]workorchmanagementcontract.DoD, error) {
		return ListDoDsWithSession(ctx, session, workspaceID)
	})
}
