package appmodelcore

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"

type Approval struct {
	ID             string                                `json:"id"`
	WorkspaceID    string                                `json:"workspaceId"`
	RunID          string                                `json:"runId"`
	StepID         string                                `json:"stepId,omitempty"`
	Reason         string                                `json:"reason"`
	Action         string                                `json:"action,omitempty"`
	Context        map[string]any                        `json:"context,omitempty"`
	State          string                                `json:"state"`
	AutoGovernance *governancecore.AutoGovernanceSummary `json:"autoGovernance,omitempty"`
	ResolvedAt     string                                `json:"resolvedAt,omitempty"`
	ResolvedBy     string                                `json:"resolvedBy,omitempty"`
	CreatedAt      string                                `json:"createdAt,omitempty"`
	UpdatedAt      string                                `json:"updatedAt,omitempty"`
}

type ApprovalQuery struct {
	WorkspaceID   string
	RunID         string
	CursorAfterID string
	Limit         int
}

type ApprovalPage struct {
	WorkspaceID       string     `json:"workspaceId,omitempty"`
	RunID             string     `json:"runId,omitempty"`
	Approvals         []Approval `json:"approvals,omitempty"`
	Limit             int        `json:"limit"`
	CursorAfterID     string     `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string     `json:"nextCursorAfterId,omitempty"`
	HasMore           bool       `json:"hasMore"`
}

type ResolveApprovalCommand struct {
	Action  string         `json:"action"`
	Payload map[string]any `json:"payload,omitempty"`
}

type BatchResolveApprovalCommand struct {
	ApprovalIDs []string       `json:"approvalIds"`
	Action      string         `json:"action"`
	Payload     map[string]any `json:"payload,omitempty"`
}
