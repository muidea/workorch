package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadDecisionWithSession(ctx context.Context, session *runtimecore.Session, ticketID string) (ret appmodelcore.Decision, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (appmodelcore.Decision, error) {
		return governanceports.DecisionView(ctx, deps.hub, deps.traces, common.TicketID(ticketID), deps.workspaceScope)
	})
}

func ReadDecisionWithBackend(ctx context.Context, backend string, dataRoot string, ticketID string) (appmodelcore.Decision, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.Decision, error) {
		return ReadDecisionWithSession(ctx, session, ticketID)
	})
}

func ReadDecisionFromEnv(ctx context.Context, ticketID string) (appmodelcore.Decision, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.Decision, error) {
		return ReadDecisionWithSession(ctx, session, ticketID)
	})
}

func ListDecisionsWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string, runID string) (ret []appmodelcore.Decision, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) ([]appmodelcore.Decision, error) {
		return governanceports.DecisionViewsForQuery(ctx, deps.hub, deps.traces, workspaceID, runID)
	})
}

func ListDecisionsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string) ([]appmodelcore.Decision, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]appmodelcore.Decision, error) {
		return ListDecisionsWithSession(ctx, session, workspaceID, runID)
	})
}

func ListDecisionPageWithSession(ctx context.Context, session *runtimecore.Session, query appmodelcore.DecisionQuery) (ret appmodelcore.DecisionPage, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (appmodelcore.DecisionPage, error) {
		return governanceports.DecisionPage(ctx, deps.hub, deps.traces, query)
	})
}

func ListDecisionPageWithBackend(ctx context.Context, backend string, dataRoot string, query appmodelcore.DecisionQuery) (appmodelcore.DecisionPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.DecisionPage, error) {
		return ListDecisionPageWithSession(ctx, session, query)
	})
}

func ListDecisionsFromEnv(ctx context.Context, workspaceID string, runID string) ([]appmodelcore.Decision, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) ([]appmodelcore.Decision, error) {
		return ListDecisionsWithSession(ctx, session, workspaceID, runID)
	})
}
