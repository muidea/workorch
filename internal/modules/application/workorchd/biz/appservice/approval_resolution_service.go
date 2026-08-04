package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ApproveApprovalWithSession(ctx context.Context, session *runtimecore.Session, approvalID string) (ret workorchprojectioncontract.Approval, err error) {
	return ResolveApprovalWithSession(ctx, session, approvalID, appmodelcore.ResolveApprovalCommand{Action: "approve"})
}

func ResolveApprovalWithSession(ctx context.Context, session *runtimecore.Session, approvalID string, cmd appmodelcore.ResolveApprovalCommand) (ret workorchprojectioncontract.Approval, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) (workorchprojectioncontract.Approval, error) {
		return governanceports.ResolveApprovalViewWithCommand(ctx, deps.hub, deps.traces, common.ApprovalID(approvalID), cmd, deps.workspaceScope)
	})
}

func ApproveApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string) (workorchprojectioncontract.Approval, error) {
	return ResolveApprovalWithBackend(ctx, backend, dataRoot, approvalID, appmodelcore.ResolveApprovalCommand{Action: "approve"})
}

func ResolveApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string, cmd appmodelcore.ResolveApprovalCommand) (workorchprojectioncontract.Approval, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchprojectioncontract.Approval, error) {
		return ResolveApprovalWithSession(ctx, session, approvalID, cmd)
	})
}

func ApproveApprovalFromEnv(ctx context.Context, approvalID string) (workorchprojectioncontract.Approval, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (workorchprojectioncontract.Approval, error) {
		return ApproveApprovalWithSession(ctx, session, approvalID)
	})
}

func RejectApprovalWithSession(ctx context.Context, session *runtimecore.Session, approvalID string) (ret workorchprojectioncontract.Approval, err error) {
	return ResolveApprovalWithSession(ctx, session, approvalID, appmodelcore.ResolveApprovalCommand{Action: "reject"})
}

func BatchResolveApprovalsWithSession(ctx context.Context, session *runtimecore.Session, cmd appmodelcore.BatchResolveApprovalCommand) (ret []workorchprojectioncontract.Approval, err error) {
	return withGovernanceSession(ctx, session, func(ctx context.Context, deps governanceSessionDependencies) ([]workorchprojectioncontract.Approval, error) {
		return governanceports.BatchResolveApprovalViews(ctx, deps.hub, deps.traces, cmd, deps.workspaceScope)
	})
}

func RejectApprovalWithBackend(ctx context.Context, backend string, dataRoot string, approvalID string) (workorchprojectioncontract.Approval, error) {
	return ResolveApprovalWithBackend(ctx, backend, dataRoot, approvalID, appmodelcore.ResolveApprovalCommand{Action: "reject"})
}

func BatchResolveApprovalsWithBackend(ctx context.Context, backend string, dataRoot string, cmd appmodelcore.BatchResolveApprovalCommand) ([]workorchprojectioncontract.Approval, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]workorchprojectioncontract.Approval, error) {
		return BatchResolveApprovalsWithSession(ctx, session, cmd)
	})
}

func RejectApprovalFromEnv(ctx context.Context, approvalID string) (workorchprojectioncontract.Approval, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (workorchprojectioncontract.Approval, error) {
		return RejectApprovalWithSession(ctx, session, approvalID)
	})
}

func BatchResolveApprovalsFromEnv(ctx context.Context, cmd appmodelcore.BatchResolveApprovalCommand) ([]workorchprojectioncontract.Approval, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) ([]workorchprojectioncontract.Approval, error) {
		return BatchResolveApprovalsWithSession(ctx, session, cmd)
	})
}
