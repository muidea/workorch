package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ResolveDecisionWithSession(ctx context.Context, session *runtimecore.Session, ticketID string, cmd appmodelcore.ResolveDecisionCommand) (ret appmodelcore.Decision, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (appmodelcore.Decision, error) {
		return governanceports.ResolveDecisionView(ctx, deps.hub, deps.traces, common.TicketID(ticketID), cmd, deps.workspaceScope)
	})
}

func BatchResolveDecisionsWithSession(ctx context.Context, session *runtimecore.Session, cmd appmodelcore.BatchResolveDecisionCommand) (ret []appmodelcore.Decision, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) ([]appmodelcore.Decision, error) {
		return governanceports.BatchResolveDecisionViews(ctx, deps.hub, deps.traces, cmd, deps.workspaceScope)
	})
}

func ResolveDecisionWithBackend(ctx context.Context, backend string, dataRoot string, ticketID string, cmd appmodelcore.ResolveDecisionCommand) (appmodelcore.Decision, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.Decision, error) {
		return ResolveDecisionWithSession(ctx, session, ticketID, cmd)
	})
}

func BatchResolveDecisionsWithBackend(ctx context.Context, backend string, dataRoot string, cmd appmodelcore.BatchResolveDecisionCommand) ([]appmodelcore.Decision, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]appmodelcore.Decision, error) {
		return BatchResolveDecisionsWithSession(ctx, session, cmd)
	})
}

func ResolveDecisionFromEnv(ctx context.Context, ticketID string, cmd appmodelcore.ResolveDecisionCommand) (appmodelcore.Decision, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.Decision, error) {
		return ResolveDecisionWithSession(ctx, session, ticketID, cmd)
	})
}

func BatchResolveDecisionsFromEnv(ctx context.Context, cmd appmodelcore.BatchResolveDecisionCommand) ([]appmodelcore.Decision, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) ([]appmodelcore.Decision, error) {
		return BatchResolveDecisionsWithSession(ctx, session, cmd)
	})
}
