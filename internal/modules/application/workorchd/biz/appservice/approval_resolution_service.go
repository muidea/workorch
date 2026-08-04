package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ApproveApprovalWithSession(ctx context.Context, session *runtimecore.Session, approvalID string) (ret appmodelcore.Approval, err error) {
	return ResolveApprovalWithSession(ctx, session, approvalID, appmodelcore.ResolveApprovalCommand{Action: "approve"})
}

func ResolveApprovalWithSession(ctx context.Context, session *runtimecore.Session, approvalID string, cmd appmodelcore.ResolveApprovalCommand) (ret appmodelcore.Approval, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (appmodelcore.Approval, error) {
		return governanceports.ResolveApprovalViewWithCommand(ctx, deps.hub, deps.traces, common.ApprovalID(approvalID), cmd, deps.workspaceScope)
	})
}

func ApproveApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string) (appmodelcore.Approval, error) {
	return ResolveApprovalWithBackend(ctx, backend, dataRoot, approvalID, appmodelcore.ResolveApprovalCommand{Action: "approve"})
}

func ResolveApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string, cmd appmodelcore.ResolveApprovalCommand) (appmodelcore.Approval, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.Approval, error) {
		return ResolveApprovalWithSession(ctx, session, approvalID, cmd)
	})
}

func ApproveApprovalFromEnv(ctx context.Context, approvalID string) (appmodelcore.Approval, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.Approval, error) {
		return ApproveApprovalWithSession(ctx, session, approvalID)
	})
}

func RejectApprovalWithSession(ctx context.Context, session *runtimecore.Session, approvalID string) (ret appmodelcore.Approval, err error) {
	return ResolveApprovalWithSession(ctx, session, approvalID, appmodelcore.ResolveApprovalCommand{Action: "reject"})
}

func BatchResolveApprovalsWithSession(ctx context.Context, session *runtimecore.Session, cmd appmodelcore.BatchResolveApprovalCommand) (ret []appmodelcore.Approval, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) ([]appmodelcore.Approval, error) {
		return governanceports.BatchResolveApprovalViews(ctx, deps.hub, deps.traces, cmd, deps.workspaceScope)
	})
}

func RejectApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string) (appmodelcore.Approval, error) {
	return ResolveApprovalWithBackend(ctx, backend, dataRoot, approvalID, appmodelcore.ResolveApprovalCommand{Action: "reject"})
}

func BatchResolveApprovalsWithBackend(ctx context.Context, backend string, dataRoot string, cmd appmodelcore.BatchResolveApprovalCommand) ([]appmodelcore.Approval, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]appmodelcore.Approval, error) {
		return BatchResolveApprovalsWithSession(ctx, session, cmd)
	})
}

func RejectApprovalFromEnv(ctx context.Context, approvalID string) (appmodelcore.Approval, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.Approval, error) {
		return RejectApprovalWithSession(ctx, session, approvalID)
	})
}

func BatchResolveApprovalsFromEnv(ctx context.Context, cmd appmodelcore.BatchResolveApprovalCommand) ([]appmodelcore.Approval, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) ([]appmodelcore.Approval, error) {
		return BatchResolveApprovalsWithSession(ctx, session, cmd)
	})
}
