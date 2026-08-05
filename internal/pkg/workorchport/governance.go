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
	AutoGovernance         *projectioncontract.AutoGovernanceSummary
	ResolvedAt             string
	ResolvedBy             string
	CreatedAt              string
	UpdatedAt              string
}

// DecisionInfoFromProjection converts the complete immutable decision
// projection into the strongly typed process port value. It is the single
// projection-to-port boundary so local and remote backends cannot silently
// expose different subsets of the decision contract.
func DecisionInfoFromProjection(item projectioncontract.Decision) DecisionInfo {
	return DecisionInfo{
		ID:                     common.TicketID(item.ID),
		WorkspaceID:            common.WorkspaceID(item.WorkspaceID),
		RunID:                  common.RunID(item.RunID),
		StepID:                 common.StepID(item.StepID),
		Reason:                 item.Reason,
		DecisionKind:           item.DecisionKind,
		Question:               item.Question,
		Context:                item.Context.Clone(),
		FailureClass:           item.FailureClass,
		FailureSignature:       item.FailureSignature,
		AllowedRecoveryActions: append([]string(nil), item.AllowedRecoveryActions...),
		State:                  item.State,
		ResolutionID:           item.ResolutionID,
		ResolutionType:         item.ResolutionType,
		ResolutionPayload:      item.ResolutionPayload.Clone(),
		ContinuationAction:     item.ContinuationAction,
		ResumeRunID:            common.RunID(item.ResumeRunID),
		SuccessorRunID:         common.RunID(item.SuccessorRunID),
		AutoGovernance:         cloneAutoGovernanceSummary(item.AutoGovernance),
		ResolvedAt:             item.ResolvedAt,
		ResolvedBy:             item.ResolvedBy,
		CreatedAt:              item.CreatedAt,
		UpdatedAt:              item.UpdatedAt,
	}
}

func DecisionInfoPtrFromProjection(item *projectioncontract.Decision) *DecisionInfo {
	if item == nil {
		return nil
	}
	ret := DecisionInfoFromProjection(*item)
	return &ret
}

func DecisionInfosFromProjection(items []projectioncontract.Decision) []DecisionInfo {
	ret := make([]DecisionInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, DecisionInfoFromProjection(item))
	}
	return ret
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
	AutoGovernance *projectioncontract.AutoGovernanceSummary
	ResolvedAt     string
	ResolvedBy     string
	CreatedAt      string
	UpdatedAt      string
}

// ApprovalInfoFromProjection converts the complete immutable approval
// projection into the strongly typed process port value.
func ApprovalInfoFromProjection(item projectioncontract.Approval) ApprovalInfo {
	return ApprovalInfo{
		ID:             common.ApprovalID(item.ID),
		WorkspaceID:    common.WorkspaceID(item.WorkspaceID),
		RunID:          common.RunID(item.RunID),
		StepID:         common.StepID(item.StepID),
		Reason:         item.Reason,
		Action:         item.Action,
		Context:        item.Context.Clone(),
		State:          item.State,
		AutoGovernance: cloneAutoGovernanceSummary(item.AutoGovernance),
		ResolvedAt:     item.ResolvedAt,
		ResolvedBy:     item.ResolvedBy,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

func ApprovalInfoPtrFromProjection(item *projectioncontract.Approval) *ApprovalInfo {
	if item == nil {
		return nil
	}
	ret := ApprovalInfoFromProjection(*item)
	return &ret
}

func ApprovalInfosFromProjection(items []projectioncontract.Approval) []ApprovalInfo {
	ret := make([]ApprovalInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, ApprovalInfoFromProjection(item))
	}
	return ret
}

func cloneAutoGovernanceSummary(item *projectioncontract.AutoGovernanceSummary) *projectioncontract.AutoGovernanceSummary {
	if item == nil {
		return nil
	}
	ret := *item
	ret.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
	return &ret
}

type ApprovalBackend interface {
	Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*ApprovalInfo, error)
	Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*ApprovalInfo, error)
	BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...workorcheventpayload.Payload) ([]ApprovalInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]ApprovalInfo, error)
	Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*ApprovalInfo, error)
}
