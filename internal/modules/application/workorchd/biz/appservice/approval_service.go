package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadApprovalWithSession(ctx context.Context, session *RuntimeSession, approvalID string) (ret Approval, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (Approval, error) {
		return governanceports.ApprovalView(ctx, deps.hub, deps.traces, common.ApprovalID(approvalID), deps.workspaceScope)
	})
}

func ReadApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string) (Approval, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Approval, error) {
		return ReadApprovalWithSession(ctx, session, approvalID)
	})
}

func ReadApprovalFromEnv(ctx context.Context, approvalID string) (Approval, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (Approval, error) {
		return ReadApprovalWithSession(ctx, session, approvalID)
	})
}

func ListApprovalsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, runID string) (ret []Approval, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) ([]Approval, error) {
		return governanceports.ApprovalViewsForQuery(ctx, deps.hub, deps.traces, workspaceID, runID)
	})
}

func ListApprovalsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string) ([]Approval, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]Approval, error) {
		return ListApprovalsWithSession(ctx, session, workspaceID, runID)
	})
}

func ListApprovalPageWithSession(ctx context.Context, session *RuntimeSession, query ApprovalQuery) (ret ApprovalPage, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (ApprovalPage, error) {
		return governanceports.ApprovalPage(ctx, deps.hub, deps.traces, query)
	})
}

func ListApprovalPageWithBackend(ctx context.Context, backend string, dataRoot string, query ApprovalQuery) (ApprovalPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (ApprovalPage, error) {
		return ListApprovalPageWithSession(ctx, session, query)
	})
}

func ListApprovalsFromEnv(ctx context.Context, workspaceID string, runID string) ([]Approval, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) ([]Approval, error) {
		return ListApprovalsWithSession(ctx, session, workspaceID, runID)
	})
}
