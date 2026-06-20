package readmodelcore

type AcceptanceSlice struct {
	ID                    string   `json:"id,omitempty"`
	Title                 string   `json:"title,omitempty"`
	State                 string   `json:"state,omitempty"`
	Priority              int      `json:"priority,omitempty"`
	Scope                 string   `json:"scope,omitempty"`
	CoverageWorkItemIDs   []string `json:"coverageWorkItemIds,omitempty"`
	RequiredEvidenceKinds []string `json:"requiredEvidenceKinds,omitempty"`
	CompletionPolicy      string   `json:"completionPolicy,omitempty"`
	ResultPolicy          string   `json:"resultPolicy,omitempty"`
	CreatedAt             string   `json:"createdAt,omitempty"`
	ActivatedAt           string   `json:"activatedAt,omitempty"`
	ClosedAt              string   `json:"closedAt,omitempty"`
}

type CoverageFocus struct {
	ID                    string   `json:"id,omitempty"`
	SliceID               string   `json:"sliceId,omitempty"`
	CoverageWorkItemIDs   []string `json:"coverageWorkItemIds,omitempty"`
	FailureClass          string   `json:"failureClass,omitempty"`
	FailureSignature      string   `json:"failureSignature,omitempty"`
	RequiredEvidenceKinds []string `json:"requiredEvidenceKinds,omitempty"`
	AcceptedEvidenceRefs  []string `json:"acceptedEvidenceRefs,omitempty"`
	RejectedEvidenceRefs  []string `json:"rejectedEvidenceRefs,omitempty"`
	TargetRefs            []string `json:"targetRefs,omitempty"`
	State                 string   `json:"state,omitempty"`
	Attempt               int      `json:"attempt,omitempty"`
}

type ConvergenceBudget struct {
	State                       string `json:"state,omitempty"`
	Decision                    string `json:"decision,omitempty"`
	MaxRepairAttemptsPerFocus   int    `json:"maxRepairAttemptsPerFocus,omitempty"`
	MaxLLMRoundsPerSlice        int    `json:"maxLLMRoundsPerSlice,omitempty"`
	MaxRepeatedFailureSignature int    `json:"maxRepeatedFailureSignature,omitempty"`
	MaxValidationOnlyRetries    int    `json:"maxValidationOnlyRetries,omitempty"`
	MaxNoCoverageDeltaRounds    int    `json:"maxNoCoverageDeltaRounds,omitempty"`
	MaxContextBytesPerRepair    int    `json:"maxContextBytesPerRepair,omitempty"`
	RepairAttempts              int    `json:"repairAttempts,omitempty"`
	LLMRounds                   int    `json:"llmRounds,omitempty"`
	RepeatedFailureSignatures   int    `json:"repeatedFailureSignatures,omitempty"`
	ValidationOnlyRetries       int    `json:"validationOnlyRetries,omitempty"`
	NoCoverageDeltaRounds       int    `json:"noCoverageDeltaRounds,omitempty"`
	LastDecisionReason          string `json:"lastDecisionReason,omitempty"`
}

type AcceptanceSummary struct {
	AcceptedSlices int `json:"acceptedSlices"`
	ActiveSlices   int `json:"activeSlices"`
	OpenSlices     int `json:"openSlices"`
	FailedSlices   int `json:"failedSlices"`
}

type AcceptanceProjection struct {
	ActiveSliceID string             `json:"activeSliceId,omitempty"`
	Slices        []AcceptanceSlice  `json:"slices,omitempty"`
	CurrentFocus  *CoverageFocus     `json:"currentFocus,omitempty"`
	Budget        *ConvergenceBudget `json:"budget,omitempty"`
	Summary       AcceptanceSummary  `json:"summary"`
}
