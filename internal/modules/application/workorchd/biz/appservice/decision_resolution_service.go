package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ResolveDecisionWithSession(ctx context.Context, session *RuntimeSession, ticketID string, cmd ResolveDecisionCommand) (ret Decision, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (Decision, error) {
		return governanceports.ResolveDecisionView(ctx, deps.hub, deps.traces, common.TicketID(ticketID), cmd, deps.workspaceScope)
	})
}

func BatchResolveDecisionsWithSession(ctx context.Context, session *RuntimeSession, cmd BatchResolveDecisionCommand) (ret []Decision, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) ([]Decision, error) {
		return governanceports.BatchResolveDecisionViews(ctx, deps.hub, deps.traces, cmd, deps.workspaceScope)
	})
}

func ResolveDecisionWithBackend(ctx context.Context, backend string, dataRoot string, ticketID string, cmd ResolveDecisionCommand) (Decision, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Decision, error) {
		return ResolveDecisionWithSession(ctx, session, ticketID, cmd)
	})
}

func BatchResolveDecisionsWithBackend(ctx context.Context, backend string, dataRoot string, cmd BatchResolveDecisionCommand) ([]Decision, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]Decision, error) {
		return BatchResolveDecisionsWithSession(ctx, session, cmd)
	})
}

func ResolveDecisionFromEnv(ctx context.Context, ticketID string, cmd ResolveDecisionCommand) (Decision, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (Decision, error) {
		return ResolveDecisionWithSession(ctx, session, ticketID, cmd)
	})
}

func BatchResolveDecisionsFromEnv(ctx context.Context, cmd BatchResolveDecisionCommand) ([]Decision, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) ([]Decision, error) {
		return BatchResolveDecisionsWithSession(ctx, session, cmd)
	})
}
