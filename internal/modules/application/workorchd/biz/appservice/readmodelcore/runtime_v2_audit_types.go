package readmodelcore

// RuntimeV2Audit is a metadata-only projection of the persisted Runtime V2
// aggregate. It is deliberately read-only: it gives deployed verifiers and
// operators enough identity, freshness and transition lineage to audit a Run
// without serializing the aggregate itself or exposing owner-private content.
type RuntimeV2Audit struct {
	WorkspaceID        string                           `json:"workspaceId,omitempty"`
	RunID              string                           `json:"runId,omitempty"`
	SchemaVersion      uint32                           `json:"schemaVersion,omitempty"`
	StateRevision      uint64                           `json:"stateRevision,omitempty"`
	EvidenceSequence   uint64                           `json:"evidenceSequence,omitempty"`
	DefinitionRevision string                           `json:"definitionRevision,omitempty"`
	EvidenceRevision   string                           `json:"evidenceRevision,omitempty"`
	Focus              RuntimeV2AuditFocus              `json:"focus"`
	RemainingFocusIDs  []string                         `json:"remainingFocusIds,omitempty"`
	CompletedFocusIDs  []string                         `json:"completedFocusIds,omitempty"`
	Coverage           []RuntimeV2AuditCoverage         `json:"coverage,omitempty"`
	AcceptedEvidence   []RuntimeV2AuditEvidence         `json:"acceptedEvidence,omitempty"`
	RejectedEvidence   []RuntimeV2AuditEvidence         `json:"rejectedEvidence,omitempty"`
	Grounding          RuntimeV2AuditGrounding          `json:"grounding"`
	Authority          RuntimeV2AuditAuthority          `json:"authority"`
	CandidateAttempts  []RuntimeV2AuditCandidateAttempt `json:"candidateAttempts,omitempty"`
	Dispatches         []RuntimeV2AuditDispatch         `json:"dispatches,omitempty"`
	Repair             RuntimeV2AuditRepair             `json:"repair"`
	Assessments        []RuntimeV2AuditAssessment       `json:"assessments,omitempty"`
	Terminal           *RuntimeV2AuditTerminal          `json:"terminal,omitempty"`
}

type RuntimeV2AuditEvidence struct {
	ID                  string   `json:"id,omitempty"`
	Kind                string   `json:"kind,omitempty"`
	AuthoritySnapshotID string   `json:"authoritySnapshotId,omitempty"`
	DispatchID          string   `json:"dispatchId,omitempty"`
	OperationID         string   `json:"operationId,omitempty"`
	Revision            string   `json:"revision,omitempty"`
	FailureClass        string   `json:"failureClass,omitempty"`
	FailureCode         string   `json:"failureCode,omitempty"`
	FailureSignature    string   `json:"failureSignature,omitempty"`
	TargetRefs          []string `json:"targetRefs,omitempty"`
	CoverageWorkItemIDs []string `json:"coverageWorkItemIds,omitempty"`
}

type RuntimeV2AuditFocus struct {
	ID                  string   `json:"id,omitempty"`
	SliceID             string   `json:"sliceId,omitempty"`
	CoverageWorkItemIDs []string `json:"coverageWorkItemIds,omitempty"`
	TargetRefs          []string `json:"targetRefs,omitempty"`
	TargetRole          string   `json:"targetRole,omitempty"`
	RequireAssessment   bool     `json:"requireAssessment,omitempty"`
}

type RuntimeV2AuditCoverage struct {
	ID                     string   `json:"id,omitempty"`
	SliceID                string   `json:"sliceId,omitempty"`
	State                  string   `json:"state,omitempty"`
	EvidenceRefs           []string `json:"evidenceRefs,omitempty"`
	LastAssessmentID       string   `json:"lastAssessmentId,omitempty"`
	ReopenedByEvidenceRefs []string `json:"reopenedByEvidenceRefs,omitempty"`
}

type RuntimeV2AuditGrounding struct {
	TargetSelection                 *RuntimeV2AuditTargetSelection          `json:"targetSelection,omitempty"`
	FailureTarget                   *RuntimeV2AuditFailureTarget            `json:"failureTarget,omitempty"`
	ProviderCeiling                 *RuntimeV2AuditProviderCeiling          `json:"providerCeiling,omitempty"`
	TargetSnapshots                 []RuntimeV2AuditTargetSnapshot          `json:"targetSnapshots,omitempty"`
	ActivePrerequisites             []RuntimeV2AuditPrerequisite            `json:"activePrerequisites,omitempty"`
	RetiredPrerequisites            []RuntimeV2AuditPrerequisite            `json:"retiredPrerequisites,omitempty"`
	PrerequisiteObservations        []RuntimeV2AuditPrerequisiteObservation `json:"prerequisiteObservations,omitempty"`
	RetiredPrerequisiteObservations []RuntimeV2AuditPrerequisiteObservation `json:"retiredPrerequisiteObservations,omitempty"`
	OperationAdmissibilities        []RuntimeV2AuditOperationAdmissibility  `json:"operationAdmissibilities,omitempty"`
}

// RuntimeV2AuditPrerequisite exposes prerequisite lifecycle binding without
// exposing owner-private fact content.  The projection remains read-only.
type RuntimeV2AuditPrerequisite struct {
	ID            string `json:"id,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Scope         string `json:"scope,omitempty"`
	FocusID       string `json:"focusId,omitempty"`
	OwnerModuleID string `json:"ownerModuleId,omitempty"`
}

// RuntimeV2AuditProviderCeiling exposes the immutable model capability sealed
// into this Run. It is metadata only and intentionally carries no provider
// credentials, endpoint or request content.
type RuntimeV2AuditProviderCeiling struct {
	ProviderID               string `json:"providerId,omitempty"`
	ModelID                  string `json:"modelId,omitempty"`
	ContextWindowTokens      int    `json:"contextWindowTokens,omitempty"`
	ProviderMaxOutputTokens  int    `json:"providerMaxOutputTokens,omitempty"`
	EffectiveMaxOutputTokens int    `json:"effectiveMaxOutputTokens,omitempty"`
	MaxInputTokens           int    `json:"maxInputTokens,omitempty"`
	MaxBytes                 int    `json:"maxBytes,omitempty"`
	CapabilityRevision       string `json:"capabilityRevision,omitempty"`
	ConfigurationRevision    string `json:"configurationRevision,omitempty"`
}

type RuntimeV2AuditTargetSelection struct {
	PrerequisiteID        string   `json:"prerequisiteId,omitempty"`
	SnapshotID            string   `json:"snapshotId,omitempty"`
	OwnerModuleID         string   `json:"ownerModuleId,omitempty"`
	TargetRole            string   `json:"targetRole,omitempty"`
	SelectorRef           string   `json:"selectorRef,omitempty"`
	SelectorOwnerRef      string   `json:"selectorOwnerRef,omitempty"`
	SelectorSubjectRef    string   `json:"selectorSubjectRef,omitempty"`
	SelectorCandidateRefs []string `json:"selectorCandidateRefs,omitempty"`
	Disposition           string   `json:"disposition,omitempty"`
	TargetRefs            []string `json:"targetRefs,omitempty"`
	ProspectiveTargetRef  string   `json:"prospectiveTargetRef,omitempty"`
	ObservedRevision      string   `json:"observedRevision,omitempty"`
}

type RuntimeV2AuditFailureTarget struct {
	Signature        string   `json:"signature,omitempty"`
	OwnerModuleID    string   `json:"ownerModuleId,omitempty"`
	SubjectRef       string   `json:"subjectRef,omitempty"`
	TargetRole       string   `json:"targetRole,omitempty"`
	TargetRefs       []string `json:"targetRefs,omitempty"`
	ObservedRevision string   `json:"observedRevision,omitempty"`
	DiagnosticClass  string   `json:"diagnosticClass,omitempty"`
	EvidenceRefs     []string `json:"evidenceRefs,omitempty"`
	ObservationHash  string   `json:"observationHash,omitempty"`
}

type RuntimeV2AuditTargetSnapshot struct {
	TargetRef    string `json:"targetRef,omitempty"`
	ContentHash  string `json:"contentHash,omitempty"`
	ContentState string `json:"contentState,omitempty"`
	Size         int64  `json:"size,omitempty"`
	ObservedAt   string `json:"observedAt,omitempty"`
}

type RuntimeV2AuditPrerequisiteObservation struct {
	PrerequisiteID string                              `json:"prerequisiteId,omitempty"`
	Kind           string                              `json:"kind,omitempty"`
	OwnerModuleID  string                              `json:"ownerModuleId,omitempty"`
	ObservationID  string                              `json:"observationId,omitempty"`
	ContentHash    string                              `json:"contentHash,omitempty"`
	Dependencies   []RuntimeV2AuditFreshnessDependency `json:"dependencies,omitempty"`
}

type RuntimeV2AuditFreshnessDependency struct {
	Domain        string `json:"domain,omitempty"`
	OwnerModuleID string `json:"ownerModuleId,omitempty"`
	SubjectRef    string `json:"subjectRef,omitempty"`
	Revision      string `json:"revision,omitempty"`
}

type RuntimeV2AuditOperationAdmissibility struct {
	PrerequisiteID     string   `json:"prerequisiteId,omitempty"`
	SnapshotID         string   `json:"snapshotId,omitempty"`
	AcquisitionBinding string   `json:"acquisitionBinding,omitempty"`
	OwnerModuleID      string   `json:"ownerModuleId,omitempty"`
	OperationKinds     []string `json:"operationKinds,omitempty"`
	TargetRefs         []string `json:"targetRefs,omitempty"`
	SelectorRefs       []string `json:"selectorRefs,omitempty"`
	ObservedRevision   string   `json:"observedRevision,omitempty"`
}

type RuntimeV2AuditAuthority struct {
	SnapshotID                string `json:"snapshotId,omitempty"`
	State                     string `json:"state,omitempty"`
	Action                    string `json:"action,omitempty"`
	FocusID                   string `json:"focusId,omitempty"`
	RequiredOutputHash        string `json:"requiredOutputHash,omitempty"`
	DefinitionRevision        string `json:"definitionRevision,omitempty"`
	EvidenceRevision          string `json:"evidenceRevision,omitempty"`
	CapabilityStateHash       string `json:"capabilityStateHash,omitempty"`
	PrerequisiteFreshnessHash string `json:"prerequisiteFreshnessHash,omitempty"`
	OperationContractRevision string `json:"operationContractRevision,omitempty"`
	Attempt                   int    `json:"attempt,omitempty"`
}

type RuntimeV2AuditCandidateAttempt struct {
	AuthorityContractID string `json:"authorityContractId,omitempty"`
	AuthoritySnapshotID string `json:"authoritySnapshotId,omitempty"`
	State               string `json:"state,omitempty"`
	Attempt             int    `json:"attempt,omitempty"`
	StablePrefixHash    string `json:"stablePrefixHash,omitempty"`
	ProposalPersisted   bool   `json:"proposalPersisted,omitempty"`
	FeasibilityCount    int    `json:"feasibilityCount,omitempty"`
	FailureClass        string `json:"failureClass,omitempty"`
	FailureCode         string `json:"failureCode,omitempty"`
	FailureSignature    string `json:"failureSignature,omitempty"`
}

type RuntimeV2AuditDispatch struct {
	ID                  string `json:"id,omitempty"`
	AuthoritySnapshotID string `json:"authoritySnapshotId,omitempty"`
	State               string `json:"state,omitempty"`
	CandidateHash       string `json:"candidateHash,omitempty"`
	CompletedOperations int    `json:"completedOperations,omitempty"`
}

type RuntimeV2AuditRepair struct {
	FocusID string `json:"focusId,omitempty"`
	SliceID string `json:"sliceId,omitempty"`
	// These counters are part of the audit contract even when they are zero.
	// Omitting zero made a healthy fresh Run indistinguishable from a projection
	// that had failed to map the repair ledger.
	RepairAttempts                int      `json:"repairAttempts"`
	ValidationOnlyRetries         int      `json:"validationOnlyRetries"`
	NoProgressCount               int      `json:"noProgressCount"`
	NoProgressOperationShapeCount int      `json:"noProgressOperationShapeCount,omitempty"`
	ActiveGapCoverageWorkItemIDs  []string `json:"activeGapCoverageWorkItemIds,omitempty"`
	ActiveGapMissingEvidenceKinds []string `json:"activeGapMissingEvidenceKinds,omitempty"`
	ActiveGapTargetRefs           []string `json:"activeGapTargetRefs,omitempty"`
	ActiveGapFailureSignature     string   `json:"activeGapFailureSignature,omitempty"`
	LastProgressRevision          string   `json:"lastProgressRevision,omitempty"`
	Exhausted                     bool     `json:"exhausted,omitempty"`
	TerminalDeliveryAttempts      int      `json:"terminalDeliveryAttempts"`
}

type RuntimeV2AuditAssessment struct {
	ID                      string   `json:"id,omitempty"`
	FocusID                 string   `json:"focusId,omitempty"`
	SliceID                 string   `json:"sliceId,omitempty"`
	Disposition             string   `json:"disposition,omitempty"`
	CoverageWorkItemIDs     []string `json:"coverageWorkItemIds,omitempty"`
	GapCoverageWorkItemIDs  []string `json:"gapCoverageWorkItemIds,omitempty"`
	GapMissingEvidenceKinds []string `json:"gapMissingEvidenceKinds,omitempty"`
	GapTargetRefs           []string `json:"gapTargetRefs,omitempty"`
	GapFailureSignature     string   `json:"gapFailureSignature,omitempty"`
}

type RuntimeV2AuditTerminal struct {
	Code                     string `json:"code,omitempty"`
	Summary                  string `json:"summary,omitempty"`
	AdministrativeKind       string `json:"administrativeKind,omitempty"`
	AdministrativeRevision   uint64 `json:"administrativeRevision,omitempty"`
	TerminalDeliveryAttempts int    `json:"terminalDeliveryAttempts,omitempty"`
}
