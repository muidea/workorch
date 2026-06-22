package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ApproveApprovalWithSession(ctx context.Context, session *RuntimeSession, approvalID string) (ret Approval, err error) {
	return ResolveApprovalWithSession(ctx, session, approvalID, ResolveApprovalCommand{Action: "approve"})
}

func ResolveApprovalWithSession(ctx context.Context, session *RuntimeSession, approvalID string, cmd ResolveApprovalCommand) (ret Approval, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (Approval, error) {
		return governanceports.ResolveApprovalViewWithCommand(ctx, deps.hub, deps.traces, common.ApprovalID(approvalID), cmd, deps.workspaceScope)
	})
}

func ApproveApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string) (Approval, error) {
	return ResolveApprovalWithBackend(ctx, backend, dataRoot, approvalID, ResolveApprovalCommand{Action: "approve"})
}

func ResolveApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string, cmd ResolveApprovalCommand) (Approval, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Approval, error) {
		return ResolveApprovalWithSession(ctx, session, approvalID, cmd)
	})
}

func ApproveApprovalFromEnv(ctx context.Context, approvalID string) (Approval, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (Approval, error) {
		return ApproveApprovalWithSession(ctx, session, approvalID)
	})
}

func RejectApprovalWithSession(ctx context.Context, session *RuntimeSession, approvalID string) (ret Approval, err error) {
	return ResolveApprovalWithSession(ctx, session, approvalID, ResolveApprovalCommand{Action: "reject"})
}

func BatchResolveApprovalsWithSession(ctx context.Context, session *RuntimeSession, cmd BatchResolveApprovalCommand) (ret []Approval, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) ([]Approval, error) {
		return governanceports.BatchResolveApprovalViews(ctx, deps.hub, deps.traces, cmd, deps.workspaceScope)
	})
}

func RejectApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string) (Approval, error) {
	return ResolveApprovalWithBackend(ctx, backend, dataRoot, approvalID, ResolveApprovalCommand{Action: "reject"})
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
