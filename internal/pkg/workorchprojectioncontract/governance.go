package workorchprojectioncontract

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

// Decision is the complete owner-neutral projection of a decision ticket.
type Decision struct {
	ID                     string                       `json:"id"`
	WorkspaceID            string                       `json:"workspaceId"`
	RunID                  string                       `json:"runId"`
	StepID                 string                       `json:"stepId,omitempty"`
	Reason                 string                       `json:"reason"`
	DecisionKind           string                       `json:"decisionKind,omitempty"`
	Question               string                       `json:"question,omitempty"`
	Context                workorcheventpayload.Payload `json:"context,omitempty"`
	FailureClass           string                       `json:"failureClass,omitempty"`
	FailureSignature       string                       `json:"failureSignature,omitempty"`
	AllowedRecoveryActions []string                     `json:"allowedRecoveryActions,omitempty"`
	State                  string                       `json:"state"`
	ResolutionID           string                       `json:"resolutionId,omitempty"`
	ResolutionType         string                       `json:"resolutionType,omitempty"`
	ResolutionPayload      workorcheventpayload.Payload `json:"resolutionPayload,omitempty"`
	ContinuationAction     string                       `json:"continuationAction,omitempty"`
	ResumeRunID            string                       `json:"resumeRunId,omitempty"`
	SuccessorRunID         string                       `json:"successorRunId,omitempty"`
	AutoGovernance         *AutoGovernanceSummary       `json:"autoGovernance,omitempty"`
	ResolvedAt             string                       `json:"resolvedAt,omitempty"`
	ResolvedBy             string                       `json:"resolvedBy,omitempty"`
	CreatedAt              string                       `json:"createdAt,omitempty"`
	UpdatedAt              string                       `json:"updatedAt,omitempty"`
}

// Approval is the complete owner-neutral projection of an approval ticket.
type Approval struct {
	ID             string                       `json:"id"`
	WorkspaceID    string                       `json:"workspaceId"`
	RunID          string                       `json:"runId"`
	StepID         string                       `json:"stepId,omitempty"`
	Reason         string                       `json:"reason"`
	Action         string                       `json:"action,omitempty"`
	Context        workorcheventpayload.Payload `json:"context,omitempty"`
	State          string                       `json:"state"`
	AutoGovernance *AutoGovernanceSummary       `json:"autoGovernance,omitempty"`
	ResolvedAt     string                       `json:"resolvedAt,omitempty"`
	ResolvedBy     string                       `json:"resolvedBy,omitempty"`
	CreatedAt      string                       `json:"createdAt,omitempty"`
	UpdatedAt      string                       `json:"updatedAt,omitempty"`
}

// AutoGovernanceSummary is the immutable projection of automated governance.
type AutoGovernanceSummary struct {
	TraceID           string   `json:"traceId"`
	Action            string   `json:"action"`
	Status            string   `json:"status"`
	RiskLevel         string   `json:"riskLevel,omitempty"`
	PolicyMatched     string   `json:"policyMatched,omitempty"`
	PolicyBlocked     string   `json:"policyBlocked,omitempty"`
	Confidence        float64  `json:"confidence,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	EvidenceRefs      []string `json:"evidenceRefs,omitempty"`
	PromptFingerprint string   `json:"promptFingerprint,omitempty"`
	OccurredAt        string   `json:"occurredAt,omitempty"`
}
