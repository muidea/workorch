package workorchprojectioncontract

// RecoveryPlan is a read-only presentation of the already-selected recovery
// posture. It is not a command and cannot transition a Run.
type RecoveryPlan struct {
	Mode               string `json:"mode"`
	NextAction         string `json:"nextAction,omitempty"`
	RunID              string `json:"runId,omitempty"`
	StepID             string `json:"stepId,omitempty"`
	BlockingTicketID   string `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string `json:"blockingApprovalId,omitempty"`
	CheckpointID       string `json:"checkpointId,omitempty"`
	FailureClass       string `json:"failureClass,omitempty"`
	FailureSignature   string `json:"failureSignature,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

type AssessmentOutcome struct {
	State                       string `json:"state,omitempty"`
	AcceptedSlices              int    `json:"acceptedSlices,omitempty"`
	AcceptedResultCount         int    `json:"acceptedResultCount,omitempty"`
	RejectedGlobalAssessments   int    `json:"rejectedGlobalAssessments,omitempty"`
	SatisfiedCoverageCount      int    `json:"satisfiedCoverageCount,omitempty"`
	RemainingCoverageCount      int    `json:"remainingCoverageCount,omitempty"`
	NewlySatisfiedCoverageCount int    `json:"newlySatisfiedCoverageCount,omitempty"`
	LatestAcceptedResultID      string `json:"latestAcceptedResultId,omitempty"`
	LatestRejectedResultID      string `json:"latestRejectedResultId,omitempty"`
	LatestRejectedReason        string `json:"latestRejectedReason,omitempty"`
}

type ProgressMilestone struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	State        string   `json:"state"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
}
