package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type DecisionInfo struct {
	ID             common.TicketID
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	Reason         string
	Question       string
	State          string
	ResolutionType string
	AutoGovernance *AutoGovernanceSummaryInfo
}

type ResolveDecisionRequest struct {
	TicketID       common.TicketID
	ResolutionType string
	Payload        map[string]any
}

type DecisionBackend interface {
	Resolve(ctx context.Context, req ResolveDecisionRequest) (*DecisionInfo, error)
	BatchResolve(ctx context.Context, ticketIDs []common.TicketID, resolutionType string, payload map[string]any) ([]DecisionInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]DecisionInfo, error)
	Get(ctx context.Context, id common.TicketID) (*DecisionInfo, error)
}

type ApprovalInfo struct {
	ID             common.ApprovalID
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	Reason         string
	Action         string
	Context        map[string]any
	State          string
	AutoGovernance *AutoGovernanceSummaryInfo
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
	Approve(ctx context.Context, id common.ApprovalID) (*ApprovalInfo, error)
	Reject(ctx context.Context, id common.ApprovalID) (*ApprovalInfo, error)
	BatchResolve(ctx context.Context, ids []common.ApprovalID, action string) ([]ApprovalInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]ApprovalInfo, error)
	Get(ctx context.Context, id common.ApprovalID) (*ApprovalInfo, error)
}
