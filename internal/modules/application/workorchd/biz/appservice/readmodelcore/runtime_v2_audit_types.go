package readmodelcore

// RuntimeV2Audit is a metadata-only projection of the persisted Runtime V2
// aggregate. It is deliberately read-only: it gives deployed verifiers and
// operators enough identity, freshness and transition lineage to audit a Run
// without serializing the aggregate itself or exposing owner-private content.
type RuntimeV2Audit struct {
	WorkspaceID        string                   `json:"workspaceId,omitempty"`
	RunID              string                   `json:"runId,omitempty"`
	SchemaVersion      uint32                   `json:"schemaVersion,omitempty"`
	StateRevision      uint64                   `json:"stateRevision,omitempty"`
	EvidenceSequence   uint64                   `json:"evidenceSequence,omitempty"`
	DefinitionRevision string                   `json:"definitionRevision,omitempty"`
	EvidenceRevision   string                   `json:"evidenceRevision,omitempty"`
	Focus              RuntimeV2AuditFocus      `json:"focus"`
	RemainingFocusIDs  []string                 `json:"remainingFocusIds,omitempty"`
	CompletedFocusIDs  []string                 `json:"completedFocusIds,omitempty"`
	Coverage           []RuntimeV2AuditCoverage `json:"coverage,omitempty"`
	// CoverageBindings is the complete immutable activation declaration across
	// current, remaining and completed focuses. It is audit-only: the
	// Orchestrator SM-2 reducer remains the sole coverage-credit authority.
	// Keeping the full set is required to prove a completed Run did not lose the
	// binding lineage of an earlier focus.
	CoverageBindings  []RuntimeV2AuditCoverageBinding  `json:"coverageBindings,omitempty"`
	AcceptedEvidence  []RuntimeV2AuditEvidence         `json:"acceptedEvidence,omitempty"`
	RejectedEvidence  []RuntimeV2AuditEvidence         `json:"rejectedEvidence,omitempty"`
	Grounding         RuntimeV2AuditGrounding          `json:"grounding"`
	Authority         RuntimeV2AuditAuthority          `json:"authority"`
	CandidateAttempts []RuntimeV2AuditCandidateAttempt `json:"candidateAttempts,omitempty"`
	Dispatches        []RuntimeV2AuditDispatch         `json:"dispatches,omitempty"`
	OperationReceipts []RuntimeV2AuditOperationReceipt `json:"operationReceipts,omitempty"`
	Repair            RuntimeV2AuditRepair             `json:"repair"`
	// Failure is the current persisted Runtime failure fact.  It is read-only
	// audit data; its original diagnostic is preserved separately from stable
	// categorization so clients cannot mistake diagnostic text for authority.
	Failure *RuntimeV2AuditFailure `json:"failure,omitempty"`
	// RepairDirective is a separate projection of the persisted canonical
	// authority. It deliberately is not inferred from Assessment observations.
	RepairDirective *RuntimeRepairDirective    `json:"repairDirective,omitempty"`
	Assessments     []RuntimeV2AuditAssessment `json:"assessments,omitempty"`
	Terminal        *RuntimeV2AuditTerminal    `json:"terminal,omitempty"`
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
	// BindingRefs are sealed SM-3 authorization identities.  CWI credit below
	// is an SM-2-derived result, never a candidate assertion.
	BindingRefs         []string `json:"bindingRefs,omitempty"`
	CoverageWorkItemIDs []string `json:"coverageWorkItemIds,omitempty"`
	Quality             string   `json:"quality,omitempty"`
	ZeroWork            bool     `json:"zeroWork,omitempty"`
	EvidenceReason      string   `json:"evidenceReason,omitempty"`
	EvidenceRefs        []string `json:"evidenceRefs,omitempty"`
	EvidenceTags        []string `json:"evidenceTags,omitempty"`
	ValidationRecipeID  string   `json:"validationRecipeId,omitempty"`
	ExecutedChecks      []string `json:"executedChecks,omitempty"`
	PassedChecks        []string `json:"passedChecks,omitempty"`
	FailedChecks        []string `json:"failedChecks,omitempty"`
	SkippedChecks       []string `json:"skippedChecks,omitempty"`
}

type RuntimeV2AuditFocus struct {
	ID                  string                          `json:"id,omitempty"`
	SliceID             string                          `json:"sliceId,omitempty"`
	CoverageWorkItemIDs []string                        `json:"coverageWorkItemIds,omitempty"`
	TargetRefs          []string                        `json:"targetRefs,omitempty"`
	TargetRole          string                          `json:"targetRole,omitempty"`
	RequireAssessment   bool                            `json:"requireAssessment,omitempty"`
	CoverageBindings    []RuntimeV2AuditCoverageBinding `json:"coverageBindings,omitempty"`
}

// RuntimeV2AuditCoverageBinding is a read-only rendering of the immutable
// activation declaration.  It contains no coverage state or action: readers
// correlate this declaration with evidence.BindingRefs and owner receipts.
type RuntimeV2AuditCoverageBinding struct {
	ID                       string                        `json:"id,omitempty"`
	CoverageWorkItemID       string                        `json:"coverageWorkItemId,omitempty"`
	FocusID                  string                        `json:"focusId,omitempty"`
	DefinitionRevision       string                        `json:"definitionRevision,omitempty"`
	Hash                     string                        `json:"hash,omitempty"`
	Proofs                   []RuntimeV2AuditCoverageProof `json:"proofs,omitempty"`
	TargetRefs               []string                      `json:"targetRefs,omitempty"`
	TargetSelectorRef        string                        `json:"targetSelectorRef,omitempty"`
	TargetSelectorOwnerRef   string                        `json:"targetSelectorOwnerRef,omitempty"`
	TargetSelectorSubjectRef string                        `json:"targetSelectorSubjectRef,omitempty"`
	ProspectiveTargetRef     string                        `json:"prospectiveTargetRef,omitempty"`
	DeclarationRef           string                        `json:"declarationRef,omitempty"`
	PrerequisiteRefs         []string                      `json:"prerequisiteRefs,omitempty"`
}

type RuntimeV2AuditCoverageProof struct {
	EvidenceKind             string   `json:"evidenceKind,omitempty"`
	RequiredEffectScopes     []string `json:"requiredEffectScopes,omitempty"`
	AllowedOperationKinds    []string `json:"allowedOperationKinds,omitempty"`
	AllowedOperationVariants []string `json:"allowedOperationVariants,omitempty"`
	ValidationRecipeID       string   `json:"validationRecipeId,omitempty"`
	RequiredCheckRefs        []string `json:"requiredCheckRefs,omitempty"`
	MetricConstraints        []string `json:"metricConstraints,omitempty"`
	ArtifactRefs             []string `json:"artifactRefs,omitempty"`
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
	ConvergenceState          string `json:"convergenceState,omitempty"`
	FocusID                   string `json:"focusId,omitempty"`
	RequiredOutputHash        string `json:"requiredOutputHash,omitempty"`
	DefinitionRevision        string `json:"definitionRevision,omitempty"`
	EvidenceRevision          string `json:"evidenceRevision,omitempty"`
	CapabilityStateHash       string `json:"capabilityStateHash,omitempty"`
	PrerequisiteFreshnessHash string `json:"prerequisiteFreshnessHash,omitempty"`
	OperationContractRevision string `json:"operationContractRevision,omitempty"`
	Attempt                   int    `json:"attempt,omitempty"`
}

// RuntimeV2AuditOperationReceipt is the bounded projection of a persisted
// capability owner receipt. Feedback is copied as structured metadata only;
// it cannot alter Runtime state or issue a command.
type RuntimeV2AuditOperationReceipt struct {
	ExecutionKey        string   `json:"executionKey,omitempty"`
	Fence               uint64   `json:"fence,omitempty"`
	OperationID         string   `json:"operationId,omitempty"`
	Outcome             string   `json:"outcome,omitempty"`
	ObservedEffectScope string   `json:"observedEffectScope,omitempty"`
	Changed             bool     `json:"changed,omitempty"`
	FeedbackDomain      string   `json:"feedbackDomain,omitempty"`
	StableCategory      string   `json:"stableCategory,omitempty"`
	OwnerModuleID       string   `json:"ownerModuleId,omitempty"`
	OriginalClass       string   `json:"originalClass,omitempty"`
	OriginalCode        string   `json:"originalCode,omitempty"`
	OriginalMessage     string   `json:"originalMessage,omitempty"`
	Signature           string   `json:"signature,omitempty"`
	TargetRefs          []string `json:"targetRefs,omitempty"`
	EvidenceRefs        []string `json:"evidenceRefs,omitempty"`
	EvidenceTags        []string `json:"evidenceTags,omitempty"`
	ValidationRecipeID  string   `json:"validationRecipeId,omitempty"`
	ExecutedChecks      []string `json:"executedChecks,omitempty"`
	PassedChecks        []string `json:"passedChecks,omitempty"`
	FailedChecks        []string `json:"failedChecks,omitempty"`
	SkippedChecks       []string `json:"skippedChecks,omitempty"`
	ExpectedRevision    string   `json:"expectedRevision,omitempty"`
	ObservedRevision    string   `json:"observedRevision,omitempty"`
	DetailsRef          string   `json:"detailsRef,omitempty"`
	DetailsHash         string   `json:"detailsHash,omitempty"`
	ContentType         string   `json:"contentType,omitempty"`
	Truncated           bool     `json:"truncated,omitempty"`
	Redacted            bool     `json:"redacted,omitempty"`
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
	// Operations is the sealed SM-3 operation identity needed to audit the
	// bindingRef -> receipt chain. It is a projection of the persisted dispatch,
	// not an executable command or a second operation authority.
	Operations []RuntimeV2AuditSealedOperation `json:"operations,omitempty"`
}

// RuntimeV2AuditSealedOperation deliberately exposes only the bounded
// authorization identity used by C12 audit verification. Candidate parameters
// and owner-private payloads remain excluded.
type RuntimeV2AuditSealedOperation struct {
	ID          string   `json:"id,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	BindingRefs []string `json:"bindingRefs,omitempty"`
	TargetRefs  []string `json:"targetRefs,omitempty"`
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
	// ActiveGap values are SM-2 ledger metadata. The producer's complete
	// claimed missing kinds, target refs and reason belong exclusively to the
	// corresponding Assessment observation below.
	ActiveGapFailureClass     string `json:"activeGapFailureClass,omitempty"`
	ActiveGapFailureCode      string `json:"activeGapFailureCode,omitempty"`
	ActiveGapFailureSignature string `json:"activeGapFailureSignature,omitempty"`
	LastProgressRevision      string `json:"lastProgressRevision,omitempty"`
	Exhausted                 bool   `json:"exhausted,omitempty"`
	TerminalDeliveryAttempts  int    `json:"terminalDeliveryAttempts"`
}

// RuntimeV2AuditFailure projects a bounded stable failure classification and
// the producer-authored safe original diagnostic.  Neither field drives an
// action: RepairDirective remains the only projected repair authority.
type RuntimeV2AuditFailure struct {
	Class              string                            `json:"class,omitempty"`
	Code               string                            `json:"code,omitempty"`
	Signature          string                            `json:"signature,omitempty"`
	Message            string                            `json:"message,omitempty"`
	TargetRefs         []string                          `json:"targetRefs,omitempty"`
	OriginalDiagnostic *RuntimeV2AuditOriginalDiagnostic `json:"originalDiagnostic,omitempty"`
}

// RuntimeV2AuditOriginalDiagnostic is a safe copy of the producer diagnostic
// already validated by the owner contract.  It is intentionally display/audit
// only and carries no retry, repair, governance, or terminal instruction.
type RuntimeV2AuditOriginalDiagnostic struct {
	OwnerModuleID    string `json:"ownerModuleId,omitempty"`
	OriginalClass    string `json:"originalClass,omitempty"`
	OriginalCode     string `json:"originalCode,omitempty"`
	OriginalMessage  string `json:"originalMessage,omitempty"`
	Signature        string `json:"signature,omitempty"`
	ExpectedRevision string `json:"expectedRevision,omitempty"`
	ObservedRevision string `json:"observedRevision,omitempty"`
	Redacted         bool   `json:"redacted,omitempty"`
	OmittedReason    string `json:"omittedReason,omitempty"`
}

type RuntimeV2AuditAssessment struct {
	// Assessment is a safe, complete producer observation. It is retained for
	// audit and diagnostics only: none of its fields select the next action.
	ID              string `json:"id,omitempty"`
	FocusID         string `json:"focusId,omitempty"`
	SliceID         string `json:"sliceId,omitempty"`
	Scope           string `json:"scope,omitempty"`
	RequirementID   string `json:"requirementId,omitempty"`
	RequirementText string `json:"requirementText,omitempty"`
	// ClaimedDisposition is the LLM producer's original observation. It is
	// diagnostic-only and never selects an action, retry, ticket, or terminal.
	ClaimedDisposition string `json:"claimedDisposition,omitempty"`
	// CanonicalDisposition is derived by the Orchestrator from persisted owner
	// evidence and coverage. It is displayed beside, never substituted for, the
	// producer observation.
	CanonicalDisposition string                         `json:"canonicalDisposition,omitempty"`
	CoverageWorkItemIDs  []string                       `json:"coverageWorkItemIds,omitempty"`
	Summary              string                         `json:"summary,omitempty"`
	EvidenceRefs         []string                       `json:"evidenceRefs,omitempty"`
	ArtifactRefs         []string                       `json:"artifactRefs,omitempty"`
	Items                []RuntimeV2AuditAssessmentItem `json:"items,omitempty"`
	Gap                  *RuntimeV2AuditAssessmentGap   `json:"gap,omitempty"`
}

type RuntimeV2AuditAssessmentItem struct {
	CoverageWorkItemID string   `json:"coverageWorkItemId,omitempty"`
	Satisfied          bool     `json:"satisfied"`
	Reason             string   `json:"reason,omitempty"`
	EvidenceRefs       []string `json:"evidenceRefs,omitempty"`
	ArtifactRefs       []string `json:"artifactRefs,omitempty"`
}

// RuntimeV2AuditAssessmentGap preserves the producer's safe original
// diagnostic. It remains distinct from the canonical RepairDirective.
type RuntimeV2AuditAssessmentGap struct {
	CoverageWorkItemIDs  []string `json:"coverageWorkItemIds,omitempty"`
	MissingEvidenceKinds []string `json:"missingEvidenceKinds,omitempty"`
	TargetRefs           []string `json:"targetRefs,omitempty"`
	EvidenceRefs         []string `json:"evidenceRefs,omitempty"`
	Reason               string   `json:"reason,omitempty"`
	FailureClass         string   `json:"failureClass,omitempty"`
	FailureCode          string   `json:"failureCode,omitempty"`
	FailureSignature     string   `json:"failureSignature,omitempty"`
}

type RuntimeV2AuditTerminal struct {
	Code                     string `json:"code,omitempty"`
	Summary                  string `json:"summary,omitempty"`
	AdministrativeKind       string `json:"administrativeKind,omitempty"`
	AdministrativeRevision   uint64 `json:"administrativeRevision,omitempty"`
	TerminalDeliveryAttempts int    `json:"terminalDeliveryAttempts,omitempty"`
}
