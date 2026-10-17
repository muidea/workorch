package workorchmanagementcontract

type DefinitionProviderObservationMetadata struct {
	Observed      bool   `json:"observed"`
	Complete      bool   `json:"complete"`
	Saved         bool   `json:"saved"`
	ObservedBytes uint64 `json:"observedBytes"`
	SHA256        string `json:"sha256,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type DefinitionProviderObservationView struct {
	DefinitionProviderObservationMetadata
	ContentBase64 string `json:"contentBase64"`
}

type DefinitionAuthoringSuccessAuditView struct {
	WorkspaceID   string                            `json:"workspaceId"`
	PermitID      string                            `json:"permitId"`
	CanonicalHash string                            `json:"canonicalHash"`
	Document      DefinitionProviderObservationView `json:"document"`
	Wire          DefinitionProviderObservationView `json:"wire"`
}

type DefinitionAuthoringDiagnosticMetadata struct {
	ParseError       string                                 `json:"parseError,omitempty"`
	ParseErrorSHA256 string                                 `json:"parseErrorSHA256,omitempty"`
	ParseErrorBytes  uint64                                 `json:"parseErrorBytes"`
	ParseErrorSaved  bool                                   `json:"parseErrorSaved"`
	PayloadSHA256    string                                 `json:"payloadSHA256"`
	PayloadBytes     uint64                                 `json:"payloadBytes"`
	PayloadSaved     bool                                   `json:"payloadSaved"`
	NotSavedReason   string                                 `json:"notSavedReason,omitempty"`
	ProviderCode     string                                 `json:"providerCode,omitempty"`
	ProviderDocument *DefinitionProviderObservationMetadata `json:"providerDocument,omitempty"`
	ProviderWire     *DefinitionProviderObservationMetadata `json:"providerWire,omitempty"`
	ProviderCause    *DefinitionProviderObservationMetadata `json:"providerCause,omitempty"`
}

type DefinitionAuthoringPreparationDiagnosticMetadata struct {
	DiagnosticID              string `json:"diagnosticId"`
	WorkspaceID               string `json:"workspaceId"`
	InputHash                 string `json:"inputHash"`
	TemplateSnapshotHash      string `json:"templateSnapshotHash"`
	ObservedWorkspaceRevision uint64 `json:"observedWorkspaceRevision"`
	Stage                     string `json:"stage"`
	TargetRequestID           string `json:"targetRequestId"`
	DerivationPermitID        string `json:"derivationPermitId,omitempty"`
	ReviewKind                string `json:"reviewKind,omitempty"`
	ProposalHash              string `json:"proposalHash,omitempty"`
	PhaseCode                 string `json:"phaseCode"`
	ResourceRef               string `json:"resourceRef,omitempty"`
	ProviderBindingHash       string `json:"providerBindingHash,omitempty"`
	CauseSHA256               string `json:"causeSHA256"`
	CauseBytes                uint64 `json:"causeBytes"`
	CauseSaved                bool   `json:"causeSaved"`
	NotSavedReason            string `json:"notSavedReason,omitempty"`
	Current                   bool   `json:"current"`
}

type DefinitionAuthoringPreparationDiagnosticView struct {
	DefinitionAuthoringPreparationDiagnosticMetadata
	CauseBase64 string `json:"causeBase64,omitempty"`
}

type DefinitionAuthoringDiagnosticView struct {
	WorkspaceID      string                             `json:"workspaceId"`
	PermitID         string                             `json:"permitId"`
	Phase            string                             `json:"phase"`
	FailureCode      string                             `json:"failureCode"`
	ParseError       string                             `json:"parseError,omitempty"`
	ParseErrorSHA256 string                             `json:"parseErrorSHA256,omitempty"`
	ParseErrorBytes  uint64                             `json:"parseErrorBytes"`
	ParseErrorSaved  bool                               `json:"parseErrorSaved"`
	PayloadBase64    string                             `json:"payloadBase64"`
	PayloadSHA256    string                             `json:"payloadSHA256"`
	PayloadBytes     uint64                             `json:"payloadBytes"`
	PayloadSaved     bool                               `json:"payloadSaved"`
	NotSavedReason   string                             `json:"notSavedReason,omitempty"`
	ProviderCode     string                             `json:"providerCode,omitempty"`
	ProviderDocument *DefinitionProviderObservationView `json:"providerDocument,omitempty"`
	ProviderWire     *DefinitionProviderObservationView `json:"providerWire,omitempty"`
	ProviderCause    *DefinitionProviderObservationView `json:"providerCause,omitempty"`
}

// DefinitionAuthoringInput contains user-controlled Goal-only creation input.
// Workspace and revision are assigned by the application and Workspace owner.
type DefinitionAuthoringInput struct {
	GoalText               string   `json:"goalText"`
	AcceptanceRequirements []string `json:"acceptanceRequirements,omitempty"`
	ResourceRefs           []string `json:"resourceRefs,omitempty"`
}

type DefinitionAuthoringCreate struct {
	RequestID            string                   `json:"requestId"`
	Title                string                   `json:"title"`
	Description          string                   `json:"description,omitempty"`
	TemplateID           string                   `json:"templateId"`
	DeliverableDirectory string                   `json:"deliverableDirectory,omitempty"`
	Input                DefinitionAuthoringInput `json:"input"`
}

type DefinitionAuthoringSave struct {
	RequestID                 string                   `json:"requestId"`
	TemplateID                string                   `json:"templateId"`
	DeliverableDirectory      *string                  `json:"deliverableDirectory,omitempty"`
	Input                     DefinitionAuthoringInput `json:"input"`
	ExpectedWorkspaceRevision uint64                   `json:"expectedWorkspaceRevision"`
	ExpectedInputHash         string                   `json:"expectedInputHash,omitempty"`
}

type DefinitionAuthoringConfirm struct {
	InputHash                 string `json:"inputHash"`
	DerivationPermitID        string `json:"derivationPermitId"`
	ProposalHash              string `json:"proposalHash"`
	SpecificationHash         string `json:"specificationHash"`
	ExpectedWorkspaceRevision uint64 `json:"expectedWorkspaceRevision"`
}

type DefinitionAuthoringResource struct {
	ID           string `json:"id"`
	Ref          string `json:"ref"`
	SnapshotHash string `json:"snapshotHash"`
}

type DefinitionAuthoringItem struct {
	Ref        string `json:"ref"`
	Text       string `json:"text"`
	SourceKind string `json:"sourceKind"`
}

type DefinitionAuthoringVerification struct {
	ID                    string `json:"id"`
	ObligationID          string `json:"obligationId"`
	Declaration           string `json:"declaration"`
	Status                string `json:"status"`
	ProfileID             string `json:"profileId,omitempty"`
	EvidenceRequirementID string `json:"evidenceRequirementId,omitempty"`
	Scope                 string `json:"scope,omitempty"`
}

type DefinitionAuthoringReview struct {
	Kind              string   `json:"kind"`
	Status            string   `json:"status"`
	Reason            string   `json:"reason,omitempty"`
	EvidenceRefs      []string `json:"evidenceRefs,omitempty"`
	RequirementIDs    []string `json:"requirementIds,omitempty"`
	SpecificationHash string   `json:"specificationHash"`
}

type DefinitionAuthoringProposal struct {
	GoalText                string                                      `json:"goalText"`
	Items                   []DefinitionAuthoringItem                   `json:"items"`
	Resources               []DefinitionAuthoringResource               `json:"resources,omitempty"`
	Verifications           []DefinitionAuthoringVerification           `json:"verifications,omitempty"`
	Reviews                 []DefinitionAuthoringReview                 `json:"reviews,omitempty"`
	ExecutionProofMappings  []DefinitionAuthoringExecutionProofMapping  `json:"executionProofMappings,omitempty"`
	EvidencePurposeMappings []DefinitionAuthoringEvidencePurposeMapping `json:"evidencePurposeMappings,omitempty"`
	ProposalHash            string                                      `json:"proposalHash"`
	SpecificationHash       string                                      `json:"specificationHash,omitempty"`
	DerivationPermitID      string                                      `json:"derivationPermitId"`
}

type DefinitionAuthoringExecutionProofMapping struct {
	ConsumerGoalText string `json:"consumerGoalText,omitempty"`
	ContractRef      string `json:"contractRef"`
	ConsumerItemRef  string `json:"consumerItemRef"`
	RuleRef          string `json:"ruleRef"`
	SourceItemRef    string `json:"sourceItemRef"`
}

type DefinitionAuthoringEvidencePurposeMapping struct {
	ContractRef             string `json:"contractRef"`
	HistoricalItemRef       string `json:"historicalItemRef"`
	AcceptanceItemRef       string `json:"acceptanceItemRef"`
	HistoricalProfileID     string `json:"historicalProfileId,omitempty"`
	HistoricalDeliverableID string `json:"historicalDeliverableId,omitempty"`
	HistoricalProfileHash   string `json:"historicalProfileHash,omitempty"`
	HistoricalText          string `json:"historicalText,omitempty"`
}

type DefinitionAuthoringRequestState struct {
	PermitID    string                                 `json:"permitId"`
	Phase       string                                 `json:"phase"`
	ReviewKind  string                                 `json:"reviewKind,omitempty"`
	State       string                                 `json:"state"`
	FailureCode string                                 `json:"failureCode,omitempty"`
	Diagnostic  *DefinitionAuthoringDiagnosticMetadata `json:"diagnostic,omitempty"`
}

// DefinitionAuthoringView is a bounded management read model, not a submitted
// owner record. In particular, File snapshot content never crosses this API.
type DefinitionAuthoringView struct {
	Title                  string                                             `json:"title,omitempty"`
	Description            string                                             `json:"description,omitempty"`
	DeliverableDirectory   string                                             `json:"deliverableDirectory,omitempty"`
	Creation               *DefinitionAuthoringCreationStatus                 `json:"creation,omitempty"`
	CompilationAccepted    bool                                               `json:"compilationAccepted,omitempty"`
	WorkspaceID            string                                             `json:"workspaceId"`
	WorkspaceRevision      uint64                                             `json:"workspaceRevision"`
	State                  string                                             `json:"state"`
	Input                  *DefinitionAuthoringInput                          `json:"input,omitempty"`
	InputRevision          uint64                                             `json:"inputRevision,omitempty"`
	InputHash              string                                             `json:"inputHash,omitempty"`
	TemplateID             string                                             `json:"templateId,omitempty"`
	TemplateCurrent        bool                                               `json:"templateCurrent"`
	Resources              []DefinitionAuthoringResource                      `json:"resources,omitempty"`
	DerivationPermitID     string                                             `json:"derivationPermitId,omitempty"`
	Requests               []DefinitionAuthoringRequestState                  `json:"requests,omitempty"`
	PreparationDiagnostics []DefinitionAuthoringPreparationDiagnosticMetadata `json:"preparationDiagnostics,omitempty"`
	Proposal               *DefinitionAuthoringProposal                       `json:"proposal,omitempty"`
	ConfirmationPhase      string                                             `json:"confirmationPhase,omitempty"`
	ConfirmationRecovery   *DefinitionAuthoringConfirm                        `json:"confirmationRecovery,omitempty"`
	CandidateHash          string                                             `json:"candidateHash,omitempty"`
	AdmissionDisposition   string                                             `json:"admissionDisposition,omitempty"`
	AdmissionViolationCode string                                             `json:"admissionViolationCode,omitempty"`
	AdmissionOffendingRefs []string                                           `json:"admissionOffendingRefs,omitempty"`
	BlockingTicketID       string                                             `json:"blockingTicketId,omitempty"`
	StartupInputHash       string                                             `json:"startupInputHash,omitempty"`
	ActivationState        string                                             `json:"activationState,omitempty"`
}

// Creation status is an owner-backed historical fact, never execution readiness.
type DefinitionAuthoringCreationStatus struct {
	RequestID         string                     `json:"requestId"`
	InputHash         string                     `json:"inputHash"`
	DirectoryPrepared bool                       `json:"directoryPrepared"`
	Replay            *DefinitionAuthoringCreate `json:"replay,omitempty"`
}
