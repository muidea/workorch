package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ResolveDecisionWithSession(ctx context.Context, session *runtimecore.Session, ticketID string, cmd appmodelcore.ResolveDecisionCommand) (ret workorchprojectioncontract.Decision, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (workorchprojectioncontract.Decision, error) {
		return governanceports.ResolveDecisionView(ctx, deps.hub, deps.traces, common.TicketID(ticketID), cmd, deps.workspaceScope)
	})
}

func BatchResolveDecisionsWithSession(ctx context.Context, session *runtimecore.Session, cmd appmodelcore.BatchResolveDecisionCommand) (ret []workorchprojectioncontract.Decision, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) ([]workorchprojectioncontract.Decision, error) {
		return governanceports.BatchResolveDecisionViews(ctx, deps.hub, deps.traces, cmd, deps.workspaceScope)
	})
}

func ResolveDecisionWithBackend(ctx context.Context, backend string, dataRoot string, ticketID string, cmd appmodelcore.ResolveDecisionCommand) (workorchprojectioncontract.Decision, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchprojectioncontract.Decision, error) {
		return ResolveDecisionWithSession(ctx, session, ticketID, cmd)
	})
}

func BatchResolveDecisionsWithBackend(ctx context.Context, backend string, dataRoot string, cmd appmodelcore.BatchResolveDecisionCommand) ([]workorchprojectioncontract.Decision, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]workorchprojectioncontract.Decision, error) {
		return BatchResolveDecisionsWithSession(ctx, session, cmd)
	})
}

func ResolveDecisionFromEnv(ctx context.Context, ticketID string, cmd appmodelcore.ResolveDecisionCommand) (workorchprojectioncontract.Decision, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (workorchprojectioncontract.Decision, error) {
		return ResolveDecisionWithSession(ctx, session, ticketID, cmd)
	})
}

func BatchResolveDecisionsFromEnv(ctx context.Context, cmd appmodelcore.BatchResolveDecisionCommand) ([]workorchprojectioncontract.Decision, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) ([]workorchprojectioncontract.Decision, error) {
		return BatchResolveDecisionsWithSession(ctx, session, cmd)
	})
}
