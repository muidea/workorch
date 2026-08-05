package workorchprojectioncontract

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// RuntimeAudit is the complete metadata-only projection of the persisted
// Runtime aggregate. It preserves identity, revision, freshness, grounding,
// dispatch and terminal lineage without exposing executable parameters or
// owner-private prerequisite content.
type RuntimeAudit struct {
	WorkspaceID        string                         `json:"workspaceId,omitempty"`
	RunID              string                         `json:"runId,omitempty"`
	SchemaVersion      uint32                         `json:"schemaVersion,omitempty"`
	StateRevision      uint64                         `json:"stateRevision,omitempty"`
	EvidenceSequence   uint64                         `json:"evidenceSequence,omitempty"`
	DefinitionRevision string                         `json:"definitionRevision,omitempty"`
	EvidenceRevision   string                         `json:"evidenceRevision,omitempty"`
	Focus              RuntimeAuditFocus              `json:"focus"`
	RemainingFocusIDs  []string                       `json:"remainingFocusIds,omitempty"`
	CompletedFocusIDs  []string                       `json:"completedFocusIds,omitempty"`
	Coverage           []RuntimeAuditCoverage         `json:"coverage,omitempty"`
	CoverageBindings   []RuntimeAuditCoverageBinding  `json:"coverageBindings,omitempty"`
	AcceptedEvidence   []RuntimeAuditEvidence         `json:"acceptedEvidence,omitempty"`
	RejectedEvidence   []RuntimeAuditEvidence         `json:"rejectedEvidence,omitempty"`
	Grounding          RuntimeAuditGrounding          `json:"grounding"`
	Authority          RuntimeAuditAuthority          `json:"authority"`
	CandidateAttempts  []RuntimeAuditCandidateAttempt `json:"candidateAttempts,omitempty"`
	Dispatches         []RuntimeAuditDispatch         `json:"dispatches,omitempty"`
	OperationReceipts  []RuntimeAuditOperationReceipt `json:"operationReceipts,omitempty"`
	Repair             RuntimeAuditRepair             `json:"repair"`
	Failure            *RuntimeAuditFailure           `json:"failure,omitempty"`
	RepairDirective    *RuntimeRepairDirective        `json:"repairDirective,omitempty"`
	Assessments        []RuntimeAuditAssessment       `json:"assessments,omitempty"`
	Terminal           *RuntimeAuditTerminal          `json:"terminal,omitempty"`
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

type RuntimeAuditPrerequisite struct {
	ID            string `json:"id,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Scope         string `json:"scope,omitempty"`
	FocusID       string `json:"focusId,omitempty"`
	OwnerModuleID string `json:"ownerModuleId,omitempty"`
}

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

type RuntimeAuditTerminal struct {
	Code                     string `json:"code,omitempty"`
	Summary                  string `json:"summary,omitempty"`
	AdministrativeKind       string `json:"administrativeKind,omitempty"`
	AdministrativeRevision   uint64 `json:"administrativeRevision,omitempty"`
	TerminalDeliveryAttempts int    `json:"terminalDeliveryAttempts,omitempty"`
}

// RuntimeAuditCoverageBinding is immutable activation-declaration metadata.
// It contains no coverage state and grants no operation authority.
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

// RuntimeAuditDispatch contains only sealed SM-3 identity required by audit
// readers. It is not executable and carries no operation parameters.
type RuntimeAuditDispatch struct {
	ID                  string                        `json:"id,omitempty"`
	AuthoritySnapshotID string                        `json:"authoritySnapshotId,omitempty"`
	State               string                        `json:"state,omitempty"`
	CandidateHash       string                        `json:"candidateHash,omitempty"`
	CompletedOperations int                           `json:"completedOperations,omitempty"`
	Operations          []RuntimeAuditSealedOperation `json:"operations,omitempty"`
}

type RuntimeAuditSealedOperation struct {
	ID          string   `json:"id,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	BindingRefs []string `json:"bindingRefs,omitempty"`
	TargetRefs  []string `json:"targetRefs,omitempty"`
}

// RuntimeAuditOperationReceipt is structured, already-judged capability-owner
// feedback. It cannot issue a command or mutate Runtime state.
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

type RuntimeAuditRepair struct {
	FocusID                       string   `json:"focusId,omitempty"`
	SliceID                       string   `json:"sliceId,omitempty"`
	RepairAttempts                int      `json:"repairAttempts"`
	ValidationOnlyRetries         int      `json:"validationOnlyRetries"`
	NoProgressCount               int      `json:"noProgressCount"`
	NoProgressOperationShapeCount int      `json:"noProgressOperationShapeCount,omitempty"`
	ActiveGapCoverageWorkItemIDs  []string `json:"activeGapCoverageWorkItemIds,omitempty"`
	ActiveGapFailureClass         string   `json:"activeGapFailureClass,omitempty"`
	ActiveGapFailureCode          string   `json:"activeGapFailureCode,omitempty"`
	ActiveGapFailureSignature     string   `json:"activeGapFailureSignature,omitempty"`
	LastProgressRevision          string   `json:"lastProgressRevision,omitempty"`
	Exhausted                     bool     `json:"exhausted,omitempty"`
	TerminalDeliveryAttempts      int      `json:"terminalDeliveryAttempts"`
}

// RuntimeAuditAssessment preserves the producer observation independently of
// canonical repair authority.
type RuntimeAuditAssessment struct {
	ID                   string                       `json:"id,omitempty"`
	FocusID              string                       `json:"focusId,omitempty"`
	SliceID              string                       `json:"sliceId,omitempty"`
	Scope                string                       `json:"scope,omitempty"`
	RequirementID        string                       `json:"requirementId,omitempty"`
	RequirementText      string                       `json:"requirementText,omitempty"`
	ClaimedDisposition   string                       `json:"claimedDisposition,omitempty"`
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

// RuntimeAuditFailure keeps the stable classification and safe producer
// diagnostic separate from the canonical repair directive.
type RuntimeAuditFailure struct {
	Class              string                                     `json:"class,omitempty"`
	Code               string                                     `json:"code,omitempty"`
	Signature          string                                     `json:"signature,omitempty"`
	Message            string                                     `json:"message,omitempty"`
	TargetRefs         []string                                   `json:"targetRefs,omitempty"`
	OriginalDiagnostic *runtimecontract.RuntimeOriginalDiagnostic `json:"originalDiagnostic,omitempty"`
}

func CloneRuntimeAudit(item *RuntimeAudit) *RuntimeAudit {
	if item == nil {
		return nil
	}
	ret := *item
	ret.RemainingFocusIDs = append([]string(nil), item.RemainingFocusIDs...)
	ret.CompletedFocusIDs = append([]string(nil), item.CompletedFocusIDs...)
	ret.Focus.CoverageWorkItemIDs = append([]string(nil), item.Focus.CoverageWorkItemIDs...)
	ret.Focus.TargetRefs = append([]string(nil), item.Focus.TargetRefs...)
	ret.Focus.CoverageBindings = CloneRuntimeAuditCoverageBindings(item.Focus.CoverageBindings)
	ret.Coverage = cloneRuntimeAuditCoverage(item.Coverage)
	ret.CoverageBindings = CloneRuntimeAuditCoverageBindings(item.CoverageBindings)
	ret.AcceptedEvidence = CloneRuntimeAuditEvidence(item.AcceptedEvidence)
	ret.RejectedEvidence = CloneRuntimeAuditEvidence(item.RejectedEvidence)
	ret.Grounding = cloneRuntimeAuditGrounding(item.Grounding)
	ret.CandidateAttempts = append([]RuntimeAuditCandidateAttempt(nil), item.CandidateAttempts...)
	ret.Dispatches = cloneRuntimeAuditDispatches(item.Dispatches)
	ret.OperationReceipts = cloneRuntimeAuditOperationReceipts(item.OperationReceipts)
	ret.Repair.ActiveGapCoverageWorkItemIDs = append([]string(nil), item.Repair.ActiveGapCoverageWorkItemIDs...)
	if item.Failure != nil {
		failure := *item.Failure
		failure.TargetRefs = append([]string(nil), item.Failure.TargetRefs...)
		failure.OriginalDiagnostic = item.Failure.OriginalDiagnostic.CloneOriginal()
		ret.Failure = &failure
	}
	ret.RepairDirective = cloneRuntimeRepairDirective(item.RepairDirective)
	ret.Assessments = CloneRuntimeAuditAssessments(item.Assessments)
	if item.Terminal != nil {
		terminal := *item.Terminal
		ret.Terminal = &terminal
	}
	return &ret
}

func cloneRuntimeAuditCoverage(items []RuntimeAuditCoverage) []RuntimeAuditCoverage {
	if items == nil {
		return nil
	}
	ret := make([]RuntimeAuditCoverage, 0, len(items))
	for _, item := range items {
		item.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
		item.ReopenedByEvidenceRefs = append([]string(nil), item.ReopenedByEvidenceRefs...)
		ret = append(ret, item)
	}
	return ret
}

func cloneRuntimeAuditGrounding(item RuntimeAuditGrounding) RuntimeAuditGrounding {
	if item.TargetSelection != nil {
		value := *item.TargetSelection
		value.SelectorCandidateRefs = append([]string(nil), item.TargetSelection.SelectorCandidateRefs...)
		value.TargetRefs = append([]string(nil), item.TargetSelection.TargetRefs...)
		item.TargetSelection = &value
	}
	if item.FailureTarget != nil {
		value := *item.FailureTarget
		value.TargetRefs = append([]string(nil), item.FailureTarget.TargetRefs...)
		value.EvidenceRefs = append([]string(nil), item.FailureTarget.EvidenceRefs...)
		item.FailureTarget = &value
	}
	if item.ProviderCeiling != nil {
		value := *item.ProviderCeiling
		item.ProviderCeiling = &value
	}
	item.TargetSnapshots = append([]RuntimeAuditTargetSnapshot(nil), item.TargetSnapshots...)
	item.ActivePrerequisites = append([]RuntimeAuditPrerequisite(nil), item.ActivePrerequisites...)
	item.RetiredPrerequisites = append([]RuntimeAuditPrerequisite(nil), item.RetiredPrerequisites...)
	item.PrerequisiteObservations = cloneRuntimeAuditPrerequisiteObservations(item.PrerequisiteObservations)
	item.RetiredPrerequisiteObservations = cloneRuntimeAuditPrerequisiteObservations(item.RetiredPrerequisiteObservations)
	item.OperationAdmissibilities = cloneRuntimeAuditOperationAdmissibilities(item.OperationAdmissibilities)
	return item
}

func cloneRuntimeAuditPrerequisiteObservations(items []RuntimeAuditPrerequisiteObservation) []RuntimeAuditPrerequisiteObservation {
	if items == nil {
		return nil
	}
	ret := make([]RuntimeAuditPrerequisiteObservation, 0, len(items))
	for _, item := range items {
		item.Dependencies = append([]RuntimeAuditFreshnessDependency(nil), item.Dependencies...)
		ret = append(ret, item)
	}
	return ret
}

func cloneRuntimeAuditOperationAdmissibilities(items []RuntimeAuditOperationAdmissibility) []RuntimeAuditOperationAdmissibility {
	if items == nil {
		return nil
	}
	ret := make([]RuntimeAuditOperationAdmissibility, 0, len(items))
	for _, item := range items {
		item.OperationKinds = append([]string(nil), item.OperationKinds...)
		item.TargetRefs = append([]string(nil), item.TargetRefs...)
		item.SelectorRefs = append([]string(nil), item.SelectorRefs...)
		ret = append(ret, item)
	}
	return ret
}

func cloneRuntimeAuditDispatches(items []RuntimeAuditDispatch) []RuntimeAuditDispatch {
	if items == nil {
		return nil
	}
	ret := make([]RuntimeAuditDispatch, 0, len(items))
	for _, item := range items {
		if item.Operations != nil {
			operations := make([]RuntimeAuditSealedOperation, 0, len(item.Operations))
			for _, operation := range item.Operations {
				operation.BindingRefs = append([]string(nil), operation.BindingRefs...)
				operation.TargetRefs = append([]string(nil), operation.TargetRefs...)
				operations = append(operations, operation)
			}
			item.Operations = operations
		}
		ret = append(ret, item)
	}
	return ret
}

func cloneRuntimeAuditOperationReceipts(items []RuntimeAuditOperationReceipt) []RuntimeAuditOperationReceipt {
	if items == nil {
		return nil
	}
	ret := make([]RuntimeAuditOperationReceipt, 0, len(items))
	for _, item := range items {
		item.OriginalDiagnostic = item.OriginalDiagnostic.CloneOriginal()
		item.TargetRefs = append([]string(nil), item.TargetRefs...)
		item.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
		item.EvidenceTags = append([]string(nil), item.EvidenceTags...)
		item.ExecutedChecks = append([]string(nil), item.ExecutedChecks...)
		item.PassedChecks = append([]string(nil), item.PassedChecks...)
		item.FailedChecks = append([]string(nil), item.FailedChecks...)
		item.SkippedChecks = append([]string(nil), item.SkippedChecks...)
		ret = append(ret, item)
	}
	return ret
}

func CloneRuntimeAuditCoverageBindings(values []RuntimeAuditCoverageBinding) []RuntimeAuditCoverageBinding {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeAuditCoverageBinding, len(values))
	for index, value := range values {
		value.TargetRefs = append([]string(nil), value.TargetRefs...)
		value.PrerequisiteRefs = append([]string(nil), value.PrerequisiteRefs...)
		value.Proofs = cloneRuntimeAuditCoverageProofs(value.Proofs)
		ret[index] = value
	}
	return ret
}

func cloneRuntimeAuditCoverageProofs(values []RuntimeAuditCoverageProof) []RuntimeAuditCoverageProof {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeAuditCoverageProof, len(values))
	for index, value := range values {
		value.RequiredEffectScopes = append([]string(nil), value.RequiredEffectScopes...)
		value.AllowedOperationKinds = append([]string(nil), value.AllowedOperationKinds...)
		value.AllowedOperationVariants = append([]string(nil), value.AllowedOperationVariants...)
		value.RequiredCheckRefs = append([]string(nil), value.RequiredCheckRefs...)
		value.MetricConstraints = append([]string(nil), value.MetricConstraints...)
		value.ArtifactRefs = append([]string(nil), value.ArtifactRefs...)
		ret[index] = value
	}
	return ret
}

func CloneRuntimeAuditAssessments(values []RuntimeAuditAssessment) []RuntimeAuditAssessment {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeAuditAssessment, len(values))
	for index, value := range values {
		value.CoverageWorkItemIDs = append([]string(nil), value.CoverageWorkItemIDs...)
		value.EvidenceRefs = append([]string(nil), value.EvidenceRefs...)
		value.ArtifactRefs = append([]string(nil), value.ArtifactRefs...)
		value.Items = cloneRuntimeAuditAssessmentItems(value.Items)
		if value.Gap != nil {
			gap := *value.Gap
			gap.CoverageWorkItemIDs = append([]string(nil), gap.CoverageWorkItemIDs...)
			gap.MissingEvidenceKinds = append([]string(nil), gap.MissingEvidenceKinds...)
			gap.TargetRefs = append([]string(nil), gap.TargetRefs...)
			gap.EvidenceRefs = append([]string(nil), gap.EvidenceRefs...)
			value.Gap = &gap
		}
		ret[index] = value
	}
	return ret
}

func cloneRuntimeAuditAssessmentItems(values []RuntimeAuditAssessmentItem) []RuntimeAuditAssessmentItem {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeAuditAssessmentItem, len(values))
	for index, value := range values {
		value.EvidenceRefs = append([]string(nil), value.EvidenceRefs...)
		value.ArtifactRefs = append([]string(nil), value.ArtifactRefs...)
		ret[index] = value
	}
	return ret
}

// RuntimeAuditEvidence is the immutable, owner-neutral projection of one
// already-judged Runtime evidence fact. It carries no reducer instruction or
// evidence-admission authority.
type RuntimeAuditEvidence struct {
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

func CloneRuntimeAuditEvidence(values []RuntimeAuditEvidence) []RuntimeAuditEvidence {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeAuditEvidence, 0, len(values))
	for _, value := range values {
		value.TargetRefs = append([]string(nil), value.TargetRefs...)
		value.BindingRefs = append([]string(nil), value.BindingRefs...)
		value.CoverageWorkItemIDs = append([]string(nil), value.CoverageWorkItemIDs...)
		value.EvidenceRefs = append([]string(nil), value.EvidenceRefs...)
		value.EvidenceTags = append([]string(nil), value.EvidenceTags...)
		value.ExecutedChecks = append([]string(nil), value.ExecutedChecks...)
		value.PassedChecks = append([]string(nil), value.PassedChecks...)
		value.FailedChecks = append([]string(nil), value.FailedChecks...)
		value.SkippedChecks = append([]string(nil), value.SkippedChecks...)
		ret = append(ret, value)
	}
	return ret
}
