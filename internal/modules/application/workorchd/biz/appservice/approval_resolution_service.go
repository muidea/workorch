package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ApproveApprovalWithSession(ctx context.Context, session *RuntimeSession, approvalID string) (ret Approval, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (Approval, error) {
		return governanceports.ResolveApprovalView(ctx, deps.hub, deps.traces, common.ApprovalID(approvalID), "approve", deps.workspaceScope)
	})
}

func ApproveApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string) (Approval, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Approval, error) {
		return ApproveApprovalWithSession(ctx, session, approvalID)
	})
}

func ApproveApprovalFromEnv(ctx context.Context, approvalID string) (Approval, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (Approval, error) {
		return ApproveApprovalWithSession(ctx, session, approvalID)
	})
}

func RejectApprovalWithSession(ctx context.Context, session *RuntimeSession, approvalID string) (ret Approval, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (Approval, error) {
		return governanceports.ResolveApprovalView(ctx, deps.hub, deps.traces, common.ApprovalID(approvalID), "reject", deps.workspaceScope)
	})
}

func BatchResolveApprovalsWithSession(ctx context.Context, session *RuntimeSession, cmd BatchResolveApprovalCommand) (ret []Approval, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) ([]Approval, error) {
		return governanceports.BatchResolveApprovalViews(ctx, deps.hub, deps.traces, cmd, deps.workspaceScope)
	})
}

func RejectApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string) (Approval, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Approval, error) {
		return RejectApprovalWithSession(ctx, session, approvalID)
	})
}

func BatchResolveApprovalsWithBackend(ctx context.Context, backend string, dataRoot string, cmd BatchResolveApprovalCommand) ([]Approval, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]Approval, error) {
		return BatchResolveApprovalsWithSession(ctx, session, cmd)
	})
}

func RejectApprovalFromEnv(ctx context.Context, approvalID string) (Approval, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (Approval, error) {
		return RejectApprovalWithSession(ctx, session, approvalID)
	})
}

func BatchResolveApprovalsFromEnv(ctx context.Context, cmd BatchResolveApprovalCommand) ([]Approval, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) ([]Approval, error) {
		return BatchResolveApprovalsWithSession(ctx, session, cmd)
	})
}
