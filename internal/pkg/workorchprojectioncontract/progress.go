package workorchprojectioncontract

// BlockingResourceRef is the exact, owner-produced governance resource that
// blocks the canonical active Run. RequiredAction and AllowedUserActions are
// copied from that owner resource; they are not Runtime transition authority.
type BlockingResourceRef struct {
	OwnerKind          string   `json:"ownerKind"`
	ResourceID         string   `json:"resourceId"`
	WorkspaceID        string   `json:"workspaceId"`
	RunID              string   `json:"runId"`
	State              string   `json:"state"`
	RequiredAction     string   `json:"requiredAction,omitempty"`
	AllowedUserActions []string `json:"allowedUserActions,omitempty"`
}

// RecoveryPlan is a factual read projection of owner state and the exact
// blocker identity. It deliberately carries no Runtime transition hint.
// SuggestedNavigation is display-only and must never authorize a mutation.
type RecoveryPlan struct {
	Mode                string               `json:"mode"`
	RunID               string               `json:"runId,omitempty"`
	StepID              string               `json:"stepId,omitempty"`
	BlockingTicketID    string               `json:"blockingTicketId,omitempty"`
	BlockingApprovalID  string               `json:"blockingApprovalId,omitempty"`
	CheckpointID        string               `json:"checkpointId,omitempty"`
	FailureClass        string               `json:"failureClass,omitempty"`
	FailureSignature    string               `json:"failureSignature,omitempty"`
	Reason              string               `json:"reason,omitempty"`
	BlockingResource    *BlockingResourceRef `json:"blockingResource,omitempty"`
	SuggestedNavigation string               `json:"suggestedNavigation,omitempty"`
}

func CloneRecoveryPlan(item RecoveryPlan) RecoveryPlan {
	if item.BlockingResource == nil {
		return item
	}
	resource := *item.BlockingResource
	resource.AllowedUserActions = append([]string(nil), item.BlockingResource.AllowedUserActions...)
	item.BlockingResource = &resource
	return item
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
