package appmodelcore

import (
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type ApprovalQuery struct {
	WorkspaceID   string
	RunID         string
	CursorAfterID string
	Limit         int
}

type ApprovalPage struct {
	WorkspaceID       string                        `json:"workspaceId,omitempty"`
	RunID             string                        `json:"runId,omitempty"`
	Approvals         []projectioncontract.Approval `json:"approvals,omitempty"`
	Limit             int                           `json:"limit"`
	CursorAfterID     string                        `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string                        `json:"nextCursorAfterId,omitempty"`
	HasMore           bool                          `json:"hasMore"`
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
