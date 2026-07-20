package readmodelcore

type TaskProgress struct {
	WorkspaceID        string              `json:"workspaceId"`
	RunID              string              `json:"runId,omitempty"`
	State              string              `json:"state"`
	Summary            string              `json:"summary,omitempty"`
	CurrentStepID      string              `json:"currentStepId,omitempty"`
	CurrentStepKind    string              `json:"currentStepKind,omitempty"`
	CurrentStepState   string              `json:"currentStepState,omitempty"`
	CurrentActionLabel string              `json:"currentActionLabel,omitempty"`
	DiagnosticLineage  string              `json:"diagnosticLineage,omitempty"`
	CurrentMilestone   string              `json:"currentMilestone,omitempty"`
	BlockingTicketID   string              `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string              `json:"blockingApprovalId,omitempty"`
	Milestones         []ProgressMilestone `json:"milestones,omitempty"`
	Items              []ProgressItem      `json:"items,omitempty"`
	Blockers           []ProgressBlocker   `json:"blockers,omitempty"`
	Recovery           RecoveryPlan        `json:"recovery"`
	RepairLoop         RepairLoop          `json:"repairLoop"`
	AssessmentOutcome  AssessmentOutcome   `json:"assessmentOutcome,omitempty"`
	EvidenceRefs       []string            `json:"evidenceRefs,omitempty"`
	RuntimeV2          *RuntimeV2Progress  `json:"runtimeV2,omitempty"`
}

// RuntimeV2Progress is a product-boundary summary projected from Run.RuntimeState.
// It is read-only and must never reverse-drive runtime authority.
type RuntimeV2Progress struct {
	StateRevision          uint64 `json:"stateRevision,omitempty"`
	EvidenceSequence       uint64 `json:"evidenceSequence,omitempty"`
	CompletedDispatchCount int    `json:"completedDispatchCount"`
	ActiveDispatchID       string `json:"activeDispatchId,omitempty"`
	CandidateAttemptCount  int    `json:"candidateAttemptCount"`
	AcceptedEvidenceCount  int    `json:"acceptedEvidenceCount"`
	// AcceptedValidationEvidenceCount is projected by the read model from
	// accepted validation-kind evidence. Presenters must not infer validation
	// completion from generic accepted evidence.
	AcceptedValidationEvidenceCount int      `json:"acceptedValidationEvidenceCount"`
	RejectedEvidenceCount           int      `json:"rejectedEvidenceCount"`
	MissingEvidenceKinds            []string `json:"missingEvidenceKinds,omitempty"`
	CurrentFocusID                  string   `json:"currentFocusId,omitempty"`
	CurrentAction                   string   `json:"currentAction,omitempty"`
	FailureClass                    string   `json:"failureClass,omitempty"`
	FailureCode                     string   `json:"failureCode,omitempty"`
	FailureSignature                string   `json:"failureSignature,omitempty"`
	FailureMessage                  string   `json:"failureMessage,omitempty"`
	RepairHintCodes                 []string `json:"repairHintCodes,omitempty"`
	// PrerequisiteRefresh is a read-only audit of generic freshness recovery.
	// It is deliberately separate from candidate/repair/provider retry counters.
	PrerequisiteRefreshState               string                                   `json:"prerequisiteRefreshState,omitempty"`
	PrerequisiteRefreshUsed                int                                      `json:"prerequisiteRefreshUsed,omitempty"`
	PrerequisiteRefreshLimit               int                                      `json:"prerequisiteRefreshLimit,omitempty"`
	PrerequisiteResourceChangeRefreshUsed  int                                      `json:"prerequisiteResourceChangeRefreshUsed,omitempty"`
	PrerequisiteResourceChangeRefreshLimit int                                      `json:"prerequisiteResourceChangeRefreshLimit,omitempty"`
	PrerequisiteInvalidations              []RuntimePrerequisiteRefreshInvalidation `json:"prerequisiteInvalidations,omitempty"`
}

// RuntimePrerequisiteRefreshInvalidation is a metadata-only rendering of one
// persisted Runtime V2 invalidation record. It is intentionally unable to
// alter facts, dispatches, budgets or lifecycle state.
type RuntimePrerequisiteRefreshInvalidation struct {
	Signature        string                              `json:"signature,omitempty"`
	Source           string                              `json:"source,omitempty"`
	PrerequisiteIDs  []string                            `json:"prerequisiteIds,omitempty"`
	ObservationIDs   []string                            `json:"observationIds,omitempty"`
	DispatchID       string                              `json:"dispatchId,omitempty"`
	OperationID      string                              `json:"operationId,omitempty"`
	ReceiptHash      string                              `json:"receiptHash,omitempty"`
	OwnerModuleID    string                              `json:"ownerModuleId,omitempty"`
	ResourceDomain   string                              `json:"resourceDomain,omitempty"`
	SubjectRef       string                              `json:"subjectRef,omitempty"`
	ExpectedRevision string                              `json:"expectedRevision,omitempty"`
	ObservedRevision string                              `json:"observedRevision,omitempty"`
	Reason           string                              `json:"reason,omitempty"`
	ResourceChanges  []RuntimePrerequisiteResourceChange `json:"resourceChanges,omitempty"`
}

// RuntimePrerequisiteResourceChange is the safe display subset of a resource
// change. It contains the owner-observed revision identity, never owner-private
// file contents, provider credentials or transport state.
type RuntimePrerequisiteResourceChange struct {
	OwnerModuleID  string `json:"ownerModuleId,omitempty"`
	ResourceDomain string `json:"resourceDomain,omitempty"`
	SubjectRef     string `json:"subjectRef,omitempty"`
	BeforeRevision string `json:"beforeRevision,omitempty"`
	AfterRevision  string `json:"afterRevision,omitempty"`
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

type ProgressItem struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	State        string   `json:"state"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
}

type ProgressBlocker struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	State       string `json:"state"`
	Reason      string `json:"reason,omitempty"`
	EvidenceRef string `json:"evidenceRef,omitempty"`
}
