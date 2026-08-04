package readmodelcore

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// RuntimeAudit is a metadata-only projection of the persisted Runtime
// aggregate. It is deliberately read-only: it gives deployed verifiers and
// operators enough identity, freshness and transition lineage to audit a Run
// without serializing the aggregate itself or exposing owner-private content.
type RuntimeAudit struct {
	WorkspaceID        string                 `json:"workspaceId,omitempty"`
	RunID              string                 `json:"runId,omitempty"`
	SchemaVersion      uint32                 `json:"schemaVersion,omitempty"`
	StateRevision      uint64                 `json:"stateRevision,omitempty"`
	EvidenceSequence   uint64                 `json:"evidenceSequence,omitempty"`
	DefinitionRevision string                 `json:"definitionRevision,omitempty"`
	EvidenceRevision   string                 `json:"evidenceRevision,omitempty"`
	Focus              RuntimeAuditFocus      `json:"focus"`
	RemainingFocusIDs  []string               `json:"remainingFocusIds,omitempty"`
	CompletedFocusIDs  []string               `json:"completedFocusIds,omitempty"`
	Coverage           []RuntimeAuditCoverage `json:"coverage,omitempty"`
	// CoverageBindings is the complete immutable activation declaration across
	// current, remaining and completed focuses. It is audit-only: the
	// Orchestrator SM-2 reducer remains the sole coverage-credit authority.
	// Keeping the full set is required to prove a completed Run did not lose the
	// binding lineage of an earlier focus.
	CoverageBindings  []RuntimeAuditCoverageBinding             `json:"coverageBindings,omitempty"`
	AcceptedEvidence  []projectioncontract.RuntimeAuditEvidence `json:"acceptedEvidence,omitempty"`
	RejectedEvidence  []projectioncontract.RuntimeAuditEvidence `json:"rejectedEvidence,omitempty"`
	Grounding         RuntimeAuditGrounding                     `json:"grounding"`
	Authority         RuntimeAuditAuthority                     `json:"authority"`
	CandidateAttempts []RuntimeAuditCandidateAttempt            `json:"candidateAttempts,omitempty"`
	Dispatches        []RuntimeAuditDispatch                    `json:"dispatches,omitempty"`
	OperationReceipts []RuntimeAuditOperationReceipt            `json:"operationReceipts,omitempty"`
	Repair            RuntimeAuditRepair                        `json:"repair"`
	// Failure is the current persisted Runtime failure fact.  It is read-only
	// audit data; its original diagnostic is preserved separately from stable
	// categorization so clients cannot mistake diagnostic text for authority.
	Failure *RuntimeAuditFailure `json:"failure,omitempty"`
	// RepairDirective is a separate projection of the persisted canonical
	// authority. It deliberately is not inferred from Assessment observations.
	RepairDirective *RuntimeRepairDirective  `json:"repairDirective,omitempty"`
	Assessments     []RuntimeAuditAssessment `json:"assessments,omitempty"`
	Terminal        *RuntimeAuditTerminal    `json:"terminal,omitempty"`
}

type RuntimeAuditFocus struct {
	ID                  string                        `json:"id,omitempty"`
	SliceID             string                        `json:"sliceId,omitempty"`
	CoverageWorkItemIDs []string                      `json:"coverageWorkItemIds,omitempty"`
	TargetRefs          []string                      `json:"targetRefs,omitempty"`
	TargetRole          string                        `json:"targetRole,omitempty"`
	RequireAssessment   bool                          `json:"requireAssessment,omitempty"`
	CoverageBindings    []RuntimeAuditCoverageBinding `json:"coverageBindings,omitempty"`
}

// RuntimeAuditCoverageBinding is a read-only rendering of the immutable
// activation declaration.  It contains no coverage state or action: readers
// correlate this declaration with evidence.BindingRefs and owner receipts.
type RuntimeAuditCoverageBinding struct {
	ID                       string                      `json:"id,omitempty"`
	CoverageWorkItemID       string                      `json:"coverageWorkItemId,omitempty"`
	FocusID                  string                      `json:"focusId,omitempty"`
	DefinitionRevision       string                      `json:"definitionRevision,omitempty"`
	Hash                     string                      `json:"hash,omitempty"`
	Proofs                   []RuntimeAuditCoverageProof `json:"proofs,omitempty"`
	TargetRefs               []string                    `json:"targetRefs,omitempty"`
	TargetSelectorRef        string                      `json:"targetSelectorRef,omitempty"`
	TargetSelectorOwnerRef   string                      `json:"targetSelectorOwnerRef,omitempty"`
	TargetSelectorSubjectRef string                      `json:"targetSelectorSubjectRef,omitempty"`
	ProspectiveTargetRef     string                      `json:"prospectiveTargetRef,omitempty"`
	DeclarationRef           string                      `json:"declarationRef,omitempty"`
	PrerequisiteRefs         []string                    `json:"prerequisiteRefs,omitempty"`
}

type RuntimeAuditCoverageProof struct {
	EvidenceKind             string   `json:"evidenceKind,omitempty"`
	RequiredEffectScopes     []string `json:"requiredEffectScopes,omitempty"`
	AllowedOperationKinds    []string `json:"allowedOperationKinds,omitempty"`
	AllowedOperationVariants []string `json:"allowedOperationVariants,omitempty"`
	ValidationRecipeID       string   `json:"validationRecipeId,omitempty"`
	RequiredCheckRefs        []string `json:"requiredCheckRefs,omitempty"`
	MetricConstraints        []string `json:"metricConstraints,omitempty"`
	ArtifactRefs             []string `json:"artifactRefs,omitempty"`
}

type RuntimeAuditCoverage struct {
	ID                     string   `json:"id,omitempty"`
	SliceID                string   `json:"sliceId,omitempty"`
	State                  string   `json:"state,omitempty"`
	EvidenceRefs           []string `json:"evidenceRefs,omitempty"`
	LastAssessmentID       string   `json:"lastAssessmentId,omitempty"`
	ReopenedByEvidenceRefs []string `json:"reopenedByEvidenceRefs,omitempty"`
}

type RuntimeAuditGrounding struct {
	TargetSelection                 *RuntimeAuditTargetSelection          `json:"targetSelection,omitempty"`
	FailureTarget                   *RuntimeAuditFailureTarget            `json:"failureTarget,omitempty"`
	ProviderCeiling                 *RuntimeAuditProviderCeiling          `json:"providerCeiling,omitempty"`
	TargetSnapshots                 []RuntimeAuditTargetSnapshot          `json:"targetSnapshots,omitempty"`
	ActivePrerequisites             []RuntimeAuditPrerequisite            `json:"activePrerequisites,omitempty"`
	RetiredPrerequisites            []RuntimeAuditPrerequisite            `json:"retiredPrerequisites,omitempty"`
	PrerequisiteObservations        []RuntimeAuditPrerequisiteObservation `json:"prerequisiteObservations,omitempty"`
	RetiredPrerequisiteObservations []RuntimeAuditPrerequisiteObservation `json:"retiredPrerequisiteObservations,omitempty"`
	OperationAdmissibilities        []RuntimeAuditOperationAdmissibility  `json:"operationAdmissibilities,omitempty"`
}

// RuntimeAuditPrerequisite exposes prerequisite lifecycle binding without
// exposing owner-private fact content.  The projection remains read-only.
type RuntimeAuditPrerequisite struct {
	ID            string `json:"id,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Scope         string `json:"scope,omitempty"`
	FocusID       string `json:"focusId,omitempty"`
	OwnerModuleID string `json:"ownerModuleId,omitempty"`
}

// RuntimeAuditProviderCeiling exposes the immutable model capability sealed
// into this Run. It is metadata only and intentionally carries no provider
// credentials, endpoint or request content.
type RuntimeAuditProviderCeiling struct {
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

type RuntimeAuditTargetSelection struct {
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

type RuntimeAuditFailureTarget struct {
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

type RuntimeAuditTargetSnapshot struct {
	TargetRef    string `json:"targetRef,omitempty"`
	ContentHash  string `json:"contentHash,omitempty"`
	ContentState string `json:"contentState,omitempty"`
	Size         int64  `json:"size,omitempty"`
	ObservedAt   string `json:"observedAt,omitempty"`
}

type RuntimeAuditPrerequisiteObservation struct {
	PrerequisiteID string                            `json:"prerequisiteId,omitempty"`
	Kind           string                            `json:"kind,omitempty"`
	OwnerModuleID  string                            `json:"ownerModuleId,omitempty"`
	ObservationID  string                            `json:"observationId,omitempty"`
	ContentHash    string                            `json:"contentHash,omitempty"`
	Dependencies   []RuntimeAuditFreshnessDependency `json:"dependencies,omitempty"`
}

type RuntimeAuditFreshnessDependency struct {
	Domain        string `json:"domain,omitempty"`
	OwnerModuleID string `json:"ownerModuleId,omitempty"`
	SubjectRef    string `json:"subjectRef,omitempty"`
	Revision      string `json:"revision,omitempty"`
}

type RuntimeAuditOperationAdmissibility struct {
	PrerequisiteID     string   `json:"prerequisiteId,omitempty"`
	SnapshotID         string   `json:"snapshotId,omitempty"`
	AcquisitionBinding string   `json:"acquisitionBinding,omitempty"`
	OwnerModuleID      string   `json:"ownerModuleId,omitempty"`
	OperationKinds     []string `json:"operationKinds,omitempty"`
	TargetRefs         []string `json:"targetRefs,omitempty"`
	SelectorRefs       []string `json:"selectorRefs,omitempty"`
	ObservedRevision   string   `json:"observedRevision,omitempty"`
}

type RuntimeAuditAuthority struct {
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

// RuntimeAuditOperationReceipt is the bounded projection of a persisted
// capability owner receipt. Feedback is copied as structured metadata only;
// it cannot alter Runtime state or issue a command.
type RuntimeAuditOperationReceipt struct {
	ExecutionKey        string                                     `json:"executionKey,omitempty"`
	Fence               uint64                                     `json:"fence,omitempty"`
	OperationID         string                                     `json:"operationId,omitempty"`
	Outcome             string                                     `json:"outcome,omitempty"`
	ObservedEffectScope string                                     `json:"observedEffectScope,omitempty"`
	Changed             bool                                       `json:"changed,omitempty"`
	FeedbackDomain      string                                     `json:"feedbackDomain,omitempty"`
	StableCategory      string                                     `json:"stableCategory,omitempty"`
	OriginalDiagnostic  *runtimecontract.RuntimeOriginalDiagnostic `json:"originalDiagnostic,omitempty"`
	TargetRefs          []string                                   `json:"targetRefs,omitempty"`
	EvidenceRefs        []string                                   `json:"evidenceRefs,omitempty"`
	EvidenceTags        []string                                   `json:"evidenceTags,omitempty"`
	ValidationRecipeID  string                                     `json:"validationRecipeId,omitempty"`
	ExecutedChecks      []string                                   `json:"executedChecks,omitempty"`
	PassedChecks        []string                                   `json:"passedChecks,omitempty"`
	FailedChecks        []string                                   `json:"failedChecks,omitempty"`
	SkippedChecks       []string                                   `json:"skippedChecks,omitempty"`
}

type RuntimeAuditCandidateAttempt struct {
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

type RuntimeAuditDispatch struct {
	ID                  string `json:"id,omitempty"`
	AuthoritySnapshotID string `json:"authoritySnapshotId,omitempty"`
	State               string `json:"state,omitempty"`
	CandidateHash       string `json:"candidateHash,omitempty"`
	CompletedOperations int    `json:"completedOperations,omitempty"`
	// Operations is the sealed SM-3 operation identity needed to audit the
	// bindingRef -> receipt chain. It is a projection of the persisted dispatch,
	// not an executable command or a second operation authority.
	Operations []RuntimeAuditSealedOperation `json:"operations,omitempty"`
}

// RuntimeAuditSealedOperation deliberately exposes only the bounded
// authorization identity used by C12 audit verification. Candidate parameters
// and owner-private payloads remain excluded.
type RuntimeAuditSealedOperation struct {
	ID          string   `json:"id,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	BindingRefs []string `json:"bindingRefs,omitempty"`
	TargetRefs  []string `json:"targetRefs,omitempty"`
}

type RuntimeAuditRepair struct {
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

// RuntimeAuditFailure projects a bounded stable failure classification and
// the producer-authored safe original diagnostic.  Neither field drives an
// action: RepairDirective remains the only projected repair authority.
type RuntimeAuditFailure struct {
	Class              string                                     `json:"class,omitempty"`
	Code               string                                     `json:"code,omitempty"`
	Signature          string                                     `json:"signature,omitempty"`
	Message            string                                     `json:"message,omitempty"`
	TargetRefs         []string                                   `json:"targetRefs,omitempty"`
	OriginalDiagnostic *runtimecontract.RuntimeOriginalDiagnostic `json:"originalDiagnostic,omitempty"`
}

type RuntimeAuditAssessment struct {
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
	CanonicalDisposition string                       `json:"canonicalDisposition,omitempty"`
	CoverageWorkItemIDs  []string                     `json:"coverageWorkItemIds,omitempty"`
	Summary              string                       `json:"summary,omitempty"`
	EvidenceRefs         []string                     `json:"evidenceRefs,omitempty"`
	ArtifactRefs         []string                     `json:"artifactRefs,omitempty"`
	Items                []RuntimeAuditAssessmentItem `json:"items,omitempty"`
	Gap                  *RuntimeAuditAssessmentGap   `json:"gap,omitempty"`
}

type RuntimeAuditAssessmentItem struct {
	CoverageWorkItemID string   `json:"coverageWorkItemId,omitempty"`
	Satisfied          bool     `json:"satisfied"`
	Reason             string   `json:"reason,omitempty"`
	EvidenceRefs       []string `json:"evidenceRefs,omitempty"`
	ArtifactRefs       []string `json:"artifactRefs,omitempty"`
}

// RuntimeAuditAssessmentGap preserves the producer's safe original
// diagnostic. It remains distinct from the canonical RepairDirective.
type RuntimeAuditAssessmentGap struct {
	CoverageWorkItemIDs  []string `json:"coverageWorkItemIds,omitempty"`
	MissingEvidenceKinds []string `json:"missingEvidenceKinds,omitempty"`
	TargetRefs           []string `json:"targetRefs,omitempty"`
	EvidenceRefs         []string `json:"evidenceRefs,omitempty"`
	Reason               string   `json:"reason,omitempty"`
	FailureClass         string   `json:"failureClass,omitempty"`
	FailureCode          string   `json:"failureCode,omitempty"`
	FailureSignature     string   `json:"failureSignature,omitempty"`
}

type RuntimeAuditTerminal struct {
	Code                     string `json:"code,omitempty"`
	Summary                  string `json:"summary,omitempty"`
	AdministrativeKind       string `json:"administrativeKind,omitempty"`
	AdministrativeRevision   uint64 `json:"administrativeRevision,omitempty"`
	TerminalDeliveryAttempts int    `json:"terminalDeliveryAttempts,omitempty"`
}
