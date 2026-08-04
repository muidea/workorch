package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadApprovalWithSession(ctx context.Context, session *runtimecore.Session, approvalID string) (ret appmodelcore.Approval, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (appmodelcore.Approval, error) {
		return governanceports.ApprovalView(ctx, deps.hub, deps.traces, common.ApprovalID(approvalID), deps.workspaceScope)
	})
}

func ReadApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string) (appmodelcore.Approval, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.Approval, error) {
		return ReadApprovalWithSession(ctx, session, approvalID)
	})
}

func ReadApprovalFromEnv(ctx context.Context, approvalID string) (appmodelcore.Approval, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.Approval, error) {
		return ReadApprovalWithSession(ctx, session, approvalID)
	})
}

func ListApprovalsWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string, runID string) (ret []appmodelcore.Approval, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) ([]appmodelcore.Approval, error) {
		return governanceports.ApprovalViewsForQuery(ctx, deps.hub, deps.traces, workspaceID, runID)
	})
}

func ListApprovalsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string) ([]appmodelcore.Approval, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]appmodelcore.Approval, error) {
		return ListApprovalsWithSession(ctx, session, workspaceID, runID)
	})
}

func ListApprovalPageWithSession(ctx context.Context, session *runtimecore.Session, query appmodelcore.ApprovalQuery) (ret appmodelcore.ApprovalPage, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (appmodelcore.ApprovalPage, error) {
		return governanceports.ApprovalPage(ctx, deps.hub, deps.traces, query)
	})
}

func ListApprovalPageWithBackend(ctx context.Context, backend string, dataRoot string, query appmodelcore.ApprovalQuery) (appmodelcore.ApprovalPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.ApprovalPage, error) {
		return ListApprovalPageWithSession(ctx, session, query)
	})
}

func ListApprovalsFromEnv(ctx context.Context, workspaceID string, runID string) ([]appmodelcore.Approval, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) ([]appmodelcore.Approval, error) {
		return ListApprovalsWithSession(ctx, session, workspaceID, runID)
	})
}
