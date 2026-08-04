package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/governancesemantics"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const (
	DecisionResolutionTypeReplan             = governancesemantics.DecisionResolutionTypeReplan
	DecisionResolutionTypeRejectCurrentPath  = governancesemantics.DecisionResolutionTypeRejectCurrentPath
	DecisionResolutionTypeProvideMissingInfo = governancesemantics.DecisionResolutionTypeProvideMissingInfo
)

type DecisionInfo struct {
	ID                     common.TicketID
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	StepID                 common.StepID
	Reason                 string
	DecisionKind           string
	Question               string
	Context                workorcheventpayload.Payload
	FailureClass           string
	FailureSignature       string
	AllowedRecoveryActions []string
	State                  string
	ResolutionID           string
	ResolutionType         string
	ResolutionPayload      workorcheventpayload.Payload
	ContinuationAction     string
	ResumeRunID            common.RunID
	SuccessorRunID         common.RunID
	AutoGovernance         *AutoGovernanceSummaryInfo
	ResolvedAt             string
	ResolvedBy             string
	CreatedAt              string
	UpdatedAt              string
}

type ResolveDecisionRequest struct {
	WorkspaceID    common.WorkspaceID
	TicketID       common.TicketID
	ResolutionType string
	Payload        workorcheventpayload.Payload
}

type DecisionBackend interface {
	Resolve(ctx context.Context, req ResolveDecisionRequest) (*DecisionInfo, error)
	BatchResolve(ctx context.Context, wsID common.WorkspaceID, ticketIDs []common.TicketID, resolutionType string, payload workorcheventpayload.Payload) ([]DecisionInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]DecisionInfo, error)
	Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*DecisionInfo, error)
}

type ApprovalInfo struct {
	ID             common.ApprovalID
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	StepID         common.StepID
	Reason         string
	Action         string
	Context        workorcheventpayload.Payload
	State          string
	AutoGovernance *AutoGovernanceSummaryInfo
	ResolvedAt     string
	ResolvedBy     string
	CreatedAt      string
	UpdatedAt      string
}

type AutoGovernanceSummaryInfo struct {
	TraceID           string
	Action            string
	Status            string
	RiskLevel         string
	PolicyMatched     string
	PolicyBlocked     string
	Confidence        float64
	Reason            string
	EvidenceRefs      []string
	PromptFingerprint string
	OccurredAt        string
}

type ApprovalBackend interface {
	Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*ApprovalInfo, error)
	Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*ApprovalInfo, error)
	BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...workorcheventpayload.Payload) ([]ApprovalInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]ApprovalInfo, error)
	Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*ApprovalInfo, error)
}
