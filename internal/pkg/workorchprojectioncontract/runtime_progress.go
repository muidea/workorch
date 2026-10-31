package workorchprojectioncontract

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// RuntimeProgress is a read-only product projection of persisted Runtime
// progress. Consumers must not reverse-drive Runtime authority from it.
type RuntimeProgress struct {
	ImplementationWork         *RuntimeImplementationProgress `json:"implementationWork,omitempty"`
	TerminalAcknowledgedOwners []string                       `json:"terminalAcknowledgedOwners,omitempty"`
	// RunEvidence is whole-Run accounting. The flat evidence counters below
	// count current-focus diagnostic lineage. Missing whole-Run accounting
	// is unavailable information, not zero progress.
	RunEvidence                     *RuntimeEvidenceCounts                      `json:"runEvidence,omitempty"`
	OriginalDiagnostic              *runtimecontract.RuntimeOriginalDiagnostic  `json:"originalDiagnostic,omitempty"`
	LatestAcceptedEvidenceAt        string                                      `json:"latestAcceptedEvidenceAt,omitempty"`
	LatestDurableEffectAt           string                                      `json:"latestDurableEffectAt,omitempty"`
	DurableEffectCount              int                                         `json:"durableEffectCount"`
	SatisfiedCWICount               int                                         `json:"satisfiedCwiCount"`
	ValidationFailureFingerprint    string                                      `json:"validationFailureFingerprint,omitempty"`
	ValidationFailureOccurrences    int                                         `json:"validationFailureOccurrences"`
	StateRevision                   uint64                                      `json:"stateRevision,omitempty"`
	EvidenceSequence                uint64                                      `json:"evidenceSequence,omitempty"`
	CompletedDispatchCount          int                                         `json:"completedDispatchCount"`
	ActiveDispatchID                string                                      `json:"activeDispatchId,omitempty"`
	CandidateAttemptCount           int                                         `json:"candidateAttemptCount"`
	AcceptedEvidenceCount           int                                         `json:"acceptedEvidenceCount"`
	AcceptedValidationEvidenceCount int                                         `json:"acceptedValidationEvidenceCount"`
	RejectedEvidenceCount           int                                         `json:"rejectedEvidenceCount"`
	MissingEvidenceKinds            []string                                    `json:"missingEvidenceKinds,omitempty"`
	CurrentFocusID                  string                                      `json:"currentFocusId,omitempty"`
	ConvergenceState                string                                      `json:"convergenceState,omitempty"`
	TerminalCode                    string                                      `json:"terminalCode,omitempty"`
	FailureClass                    string                                      `json:"failureClass,omitempty"`
	FailureCode                     string                                      `json:"failureCode,omitempty"`
	FailureSignature                string                                      `json:"failureSignature,omitempty"`
	FailureMessage                  string                                      `json:"failureMessage,omitempty"`
	RepairHintCodes                 []string                                    `json:"repairHintCodes,omitempty"`
	PrerequisiteRefreshState        string                                      `json:"prerequisiteRefreshState,omitempty"`
	PrerequisiteRefreshUsed         int                                         `json:"prerequisiteRefreshUsed,omitempty"`
	PrerequisiteRefreshLimit        int                                         `json:"prerequisiteRefreshLimit,omitempty"`
	PrerequisiteInvalidations       []RuntimePrerequisiteRefreshInvalidation    `json:"prerequisiteInvalidations,omitempty"`
	ExecutionBudget                 *runtimecontract.RuntimeExecutionBudgetView `json:"executionBudget,omitempty"`
	RecoverableWait                 *runtimecontract.RuntimeRecoverableWaitView `json:"recoverableWait,omitempty"`
}

// Counts persisted Runtime facts only. In particular, an owner-local physical
// effect or saved receipt not admitted to Runtime cannot be inferred here.
type RuntimeImplementationProgress struct {
	ReservedRounds      int `json:"reservedRounds"`
	CompletedRounds     int `json:"completedRounds"`
	CommittedReceipts   int `json:"committedReceipts"`
	UnknownExecutions   int `json:"unknownExecutions"`
	SemanticGenerations int `json:"semanticGenerations"`
	ArchivedSegments    int `json:"archivedSegments"`
}

type RuntimeEvidenceCounts struct {
	Accepted           int `json:"accepted"`
	AcceptedValidation int `json:"acceptedValidation"`
	Rejected           int `json:"rejected"`
	RejectedValidation int `json:"rejectedValidation"`
}

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

type RuntimePrerequisiteResourceChange struct {
	OwnerModuleID  string `json:"ownerModuleId,omitempty"`
	ResourceDomain string `json:"resourceDomain,omitempty"`
	SubjectRef     string `json:"subjectRef,omitempty"`
	BeforeRevision string `json:"beforeRevision,omitempty"`
	AfterRevision  string `json:"afterRevision,omitempty"`
}

func CloneRuntimeProgress(item *RuntimeProgress) *RuntimeProgress {
	if item == nil {
		return nil
	}
	ret := *item
	if item.ImplementationWork != nil {
		value := *item.ImplementationWork
		ret.ImplementationWork = &value
	}
	ret.TerminalAcknowledgedOwners = append([]string(nil), item.TerminalAcknowledgedOwners...)
	if item.RunEvidence != nil {
		counts := *item.RunEvidence
		ret.RunEvidence = &counts
	}
	ret.OriginalDiagnostic = item.OriginalDiagnostic.CloneOriginal()
	ret.MissingEvidenceKinds = append([]string(nil), item.MissingEvidenceKinds...)
	ret.RepairHintCodes = append([]string(nil), item.RepairHintCodes...)
	if item.ExecutionBudget != nil {
		value := item.ExecutionBudget.Clone()
		ret.ExecutionBudget = &value
	}
	if item.RecoverableWait != nil {
		value := item.RecoverableWait.Clone()
		ret.RecoverableWait = &value
	}
	if item.PrerequisiteInvalidations != nil {
		ret.PrerequisiteInvalidations = make([]RuntimePrerequisiteRefreshInvalidation, 0, len(item.PrerequisiteInvalidations))
		for _, invalidation := range item.PrerequisiteInvalidations {
			invalidation.PrerequisiteIDs = append([]string(nil), invalidation.PrerequisiteIDs...)
			invalidation.ObservationIDs = append([]string(nil), invalidation.ObservationIDs...)
			invalidation.ResourceChanges = append([]RuntimePrerequisiteResourceChange(nil), invalidation.ResourceChanges...)
			ret.PrerequisiteInvalidations = append(ret.PrerequisiteInvalidations, invalidation)
		}
	}
	return &ret
}
