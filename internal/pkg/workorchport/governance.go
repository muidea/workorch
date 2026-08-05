package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/governancesemantics"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

const (
	DecisionResolutionTypeReplan             = governancesemantics.DecisionResolutionTypeReplan
	DecisionResolutionTypeRejectCurrentPath  = governancesemantics.DecisionResolutionTypeRejectCurrentPath
	DecisionResolutionTypeProvideMissingInfo = governancesemantics.DecisionResolutionTypeProvideMissingInfo
)

type ResolveDecisionRequest struct {
	WorkspaceID    common.WorkspaceID
	TicketID       common.TicketID
	ResolutionType string
	Payload        workorcheventpayload.Payload
}

type DecisionBackend interface {
	Resolve(ctx context.Context, req ResolveDecisionRequest) (*projectioncontract.Decision, error)
	BatchResolve(ctx context.Context, wsID common.WorkspaceID, ticketIDs []common.TicketID, resolutionType string, payload workorcheventpayload.Payload) ([]projectioncontract.Decision, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Decision, error)
	Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*projectioncontract.Decision, error)
}

type ApprovalBackend interface {
	Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*projectioncontract.Approval, error)
	Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*projectioncontract.Approval, error)
	BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...workorcheventpayload.Payload) ([]projectioncontract.Approval, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Approval, error)
	Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*projectioncontract.Approval, error)
}
