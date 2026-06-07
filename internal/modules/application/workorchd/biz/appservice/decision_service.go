package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadDecisionWithSession(ctx context.Context, session *RuntimeSession, ticketID string) (ret Decision, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (Decision, error) {
		return governanceports.DecisionView(ctx, deps.hub, deps.traces, common.TicketID(ticketID), deps.workspaceScope)
	})
}

func ReadDecisionWithBackend(ctx context.Context, backend string, dataRoot string, ticketID string) (Decision, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Decision, error) {
		return ReadDecisionWithSession(ctx, session, ticketID)
	})
}

func ReadDecisionFromEnv(ctx context.Context, ticketID string) (Decision, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (Decision, error) {
		return ReadDecisionWithSession(ctx, session, ticketID)
	})
}

func ListDecisionsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, runID string) (ret []Decision, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) ([]Decision, error) {
		return governanceports.DecisionViewsForQuery(ctx, deps.hub, deps.traces, workspaceID, runID)
	})
}

func ListDecisionsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string) ([]Decision, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]Decision, error) {
		return ListDecisionsWithSession(ctx, session, workspaceID, runID)
	})
}

func ListDecisionPageWithSession(ctx context.Context, session *RuntimeSession, query DecisionQuery) (ret DecisionPage, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (DecisionPage, error) {
		return governanceports.DecisionPage(ctx, deps.hub, deps.traces, query)
	})
}

func ListDecisionPageWithBackend(ctx context.Context, backend string, dataRoot string, query DecisionQuery) (DecisionPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (DecisionPage, error) {
		return ListDecisionPageWithSession(ctx, session, query)
	})
}

func ListDecisionsFromEnv(ctx context.Context, workspaceID string, runID string) ([]Decision, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) ([]Decision, error) {
		return ListDecisionsWithSession(ctx, session, workspaceID, runID)
	})
}
