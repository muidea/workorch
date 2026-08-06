package appmodelcore

import (
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type ApprovalPage struct {
	WorkspaceID string                        `json:"workspaceId,omitempty"`
	RunID       string                        `json:"runId,omitempty"`
	Approvals   []projectioncontract.Approval `json:"approvals,omitempty"`
	projectioncontract.CursorPage
}

type ResolveApprovalCommand struct {
	Action  string                       `json:"action"`
	Payload workorcheventpayload.Payload `json:"payload,omitempty"`
}

type BatchResolveApprovalCommand struct {
	ApprovalIDs []string                     `json:"approvalIds"`
	Action      string                       `json:"action"`
	Payload     workorcheventpayload.Payload `json:"payload,omitempty"`
}
