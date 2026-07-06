package appmodelcore

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"

type Decision struct {
	ID                     string                                `json:"id"`
	WorkspaceID            string                                `json:"workspaceId"`
	RunID                  string                                `json:"runId"`
	StepID                 string                                `json:"stepId,omitempty"`
	Reason                 string                                `json:"reason"`
	DecisionKind           string                                `json:"decisionKind,omitempty"`
	Question               string                                `json:"question,omitempty"`
	Context                map[string]any                        `json:"context,omitempty"`
	FailureClass           string                                `json:"failureClass,omitempty"`
	FailureSignature       string                                `json:"failureSignature,omitempty"`
	AllowedRecoveryActions []string                              `json:"allowedRecoveryActions,omitempty"`
	State                  string                                `json:"state"`
	ResolutionID           string                                `json:"resolutionId,omitempty"`
	ResolutionType         string                                `json:"resolutionType,omitempty"`
	ResolutionPayload      map[string]any                        `json:"resolutionPayload,omitempty"`
	ContinuationAction     string                                `json:"continuationAction,omitempty"`
	ResumeRunID            string                                `json:"resumeRunId,omitempty"`
	SuccessorRunID         string                                `json:"successorRunId,omitempty"`
	AutoGovernance         *governancecore.AutoGovernanceSummary `json:"autoGovernance,omitempty"`
	ResolvedAt             string                                `json:"resolvedAt,omitempty"`
	ResolvedBy             string                                `json:"resolvedBy,omitempty"`
	CreatedAt              string                                `json:"createdAt,omitempty"`
	UpdatedAt              string                                `json:"updatedAt,omitempty"`
}

type DecisionQuery struct {
	WorkspaceID   string
	RunID         string
	CursorAfterID string
	Limit         int
}

type DecisionPage struct {
	WorkspaceID       string     `json:"workspaceId,omitempty"`
	RunID             string     `json:"runId,omitempty"`
	Decisions         []Decision `json:"decisions,omitempty"`
	Limit             int        `json:"limit"`
	CursorAfterID     string     `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string     `json:"nextCursorAfterId,omitempty"`
	HasMore           bool       `json:"hasMore"`
}

type ResolveDecisionCommand struct {
	ResolutionType    string         `json:"resolutionType"`
	ResolutionPayload map[string]any `json:"resolutionPayload,omitempty"`
}

type BatchResolveDecisionCommand struct {
	TicketIDs         []string       `json:"ticketIds"`
	ResolutionType    string         `json:"resolutionType"`
	ResolutionPayload map[string]any `json:"resolutionPayload,omitempty"`
}
