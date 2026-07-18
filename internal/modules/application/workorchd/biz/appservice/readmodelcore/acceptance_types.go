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
	FocusID                      string `json:"focusId,omitempty"`
	SliceID                      string `json:"sliceId,omitempty"`
	State                        string `json:"state,omitempty"`
	Decision                     string `json:"decision,omitempty"`
	MaxRepairAttemptsPerFocus    int    `json:"maxRepairAttemptsPerFocus,omitempty"`
	MaxLLMRoundsPerSlice         int    `json:"maxLLMRoundsPerSlice,omitempty"`
	MaxRepeatedFailureSignature  int    `json:"maxRepeatedFailureSignature,omitempty"`
	MaxValidationOnlyRetries     int    `json:"maxValidationOnlyRetries,omitempty"`
	MaxNoCoverageDeltaRounds     int    `json:"maxNoCoverageDeltaRounds,omitempty"`
	MaxContextBytesPerRepair     int    `json:"maxContextBytesPerRepair,omitempty"`
	RepairAttempts               int    `json:"repairAttempts,omitempty"`
	LLMRounds                    int    `json:"llmRounds,omitempty"`
	RepeatedFailureSignatures    int    `json:"repeatedFailureSignatures,omitempty"`
	ValidationOnlyRetries        int    `json:"validationOnlyRetries,omitempty"`
	NoCoverageDeltaRounds        int    `json:"noCoverageDeltaRounds,omitempty"`
	LastNoDeltaReason            string `json:"lastNoDeltaReason,omitempty"`
	LastDecisionReason           string `json:"lastDecisionReason,omitempty"`
	MaxProviderTransportAttempts int    `json:"maxProviderTransportAttempts,omitempty"`
	ProviderTransportAttempts    int    `json:"providerTransportAttempts,omitempty"`
	MaxOperationRecoveryAttempts int    `json:"maxOperationRecoveryAttempts,omitempty"`
	OperationRecoveryAttempts    int    `json:"operationRecoveryAttempts,omitempty"`
	MaxCASReloadAttempts         int    `json:"maxCasReloadAttempts,omitempty"`
	CASReloadAttempts            int    `json:"casReloadAttempts,omitempty"`
	MaxTerminalDeliveryAttempts  int    `json:"maxTerminalDeliveryAttempts,omitempty"`
	TerminalDeliveryAttempts     int    `json:"terminalDeliveryAttempts,omitempty"`
}

type AcceptanceSummary struct {
	AcceptedSlices int `json:"acceptedSlices"`
	ActiveSlices   int `json:"activeSlices"`
	OpenSlices     int `json:"openSlices"`
	FailedSlices   int `json:"failedSlices"`
}

type AcceptanceProjection struct {
	ActiveSliceID    string                   `json:"activeSliceId,omitempty"`
	Slices           []AcceptanceSlice        `json:"slices,omitempty"`
	CurrentFocus     *CoverageFocus           `json:"currentFocus,omitempty"`
	Budget           *ConvergenceBudget       `json:"budget,omitempty"`
	RepairDirective  *RuntimeRepairDirective  `json:"repairDirective,omitempty"`
	CapabilityStates []RuntimeCapabilityState `json:"capabilityStates,omitempty"`
	Governance       []RuntimeGovernanceState `json:"governance,omitempty"`
	Operations       []RuntimeOperationStatus `json:"operations,omitempty"`
	Summary          AcceptanceSummary        `json:"summary"`
}

type RuntimeRepairDirective struct {
	ID                    string   `json:"id,omitempty"`
	RequiredAction        string   `json:"requiredAction,omitempty"`
	RequiredEffectScopes  []string `json:"requiredEffectScopes,omitempty"`
	RequiredEvidenceKinds []string `json:"requiredEvidenceKinds,omitempty"`
	TargetRefs            []string `json:"targetRefs,omitempty"`
	RepairAttempt         int      `json:"repairAttempt,omitempty"`
}

type RuntimeOperationStatus struct {
	DispatchID           string `json:"dispatchId,omitempty"`
	OperationID          string `json:"operationId,omitempty"`
	Kind                 string `json:"kind,omitempty"`
	VariantID            string `json:"variantId,omitempty"`
	RequiredOperationRef string `json:"requiredOperationRef,omitempty"`
	ExecutionKey         string `json:"executionKey,omitempty"`
	Fence                uint64 `json:"fence,omitempty"`
	CapabilityStateHash  string `json:"capabilityStateHash,omitempty"`
	ExpectedEffectScope  string `json:"expectedEffectScope,omitempty"`
	ObservedEffectScope  string `json:"observedEffectScope,omitempty"`
	ExecutionState       string `json:"executionState,omitempty"`
	Outcome              string `json:"outcome,omitempty"`
	ReceiptRef           string `json:"receiptRef,omitempty"`
	ReceiptHash          string `json:"receiptHash,omitempty"`
	GovernanceState      string `json:"governanceState,omitempty"`
	GovernanceTicketID   string `json:"governanceTicketId,omitempty"`
	OutcomeUnknown       bool   `json:"outcomeUnknown,omitempty"`
}

// RuntimeCapabilityState is an audit-safe, metadata-only projection of the
// owner-confirmed prerequisite that was sealed into Runtime V2 authority. It
// intentionally excludes owner-private/raw state content.
type RuntimeCapabilityState struct {
	RequirementID             string                          `json:"requirementId,omitempty"`
	SnapshotID                string                          `json:"snapshotId,omitempty"`
	AcquisitionKey            string                          `json:"acquisitionKey,omitempty"`
	OwnerModuleID             string                          `json:"ownerModuleId,omitempty"`
	CapabilityStateKind       string                          `json:"capabilityStateKind,omitempty"`
	SubjectRef                string                          `json:"subjectRef,omitempty"`
	ObservedRevision          string                          `json:"observedRevision,omitempty"`
	ContentHash               string                          `json:"contentHash,omitempty"`
	Disposition               string                          `json:"disposition,omitempty"`
	RequiredOperationSequence []RuntimeRequiredOperationState `json:"requiredOperationSequence,omitempty"`
	QueryReceiptRef           string                          `json:"queryReceiptRef,omitempty"`
	ObservedAt                string                          `json:"observedAt,omitempty"`
	InvalidatedByReceiptRefs  []string                        `json:"invalidatedByReceiptRefs,omitempty"`
}

type RuntimeRequiredOperationState struct {
	RequirementID           string   `json:"requirementId,omitempty"`
	Kind                    string   `json:"kind,omitempty"`
	VariantID               string   `json:"variantId,omitempty"`
	DependsOnRequirementIDs []string `json:"dependsOnRequirementIds,omitempty"`
	Reason                  string   `json:"reason,omitempty"`
}

// RuntimeGovernanceState projects the aggregate's request/resolution binding.
// The Decision owner remains authoritative for the ticket itself; this value
// makes the Runtime V2 side of that cross-owner binding observable.
type RuntimeGovernanceState struct {
	TicketID            string `json:"ticketId,omitempty"`
	AuthoritySnapshotID string `json:"authoritySnapshotId,omitempty"`
	DispatchID          string `json:"dispatchId,omitempty"`
	DependencyKind      string `json:"dependencyKind,omitempty"`
	SubjectRef          string `json:"subjectRef,omitempty"`
	ResolutionID        string `json:"resolutionId,omitempty"`
	Outcome             string `json:"outcome,omitempty"`
}
