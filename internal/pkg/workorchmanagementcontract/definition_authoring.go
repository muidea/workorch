package workorchmanagementcontract

type DefinitionAuthoringDiagnosticMetadata struct {
	ParseError       string `json:"parseError,omitempty"`
	ParseErrorSHA256 string `json:"parseErrorSHA256,omitempty"`
	ParseErrorBytes  uint64 `json:"parseErrorBytes"`
	ParseErrorSaved  bool   `json:"parseErrorSaved"`
	PayloadSHA256    string `json:"payloadSHA256"`
	PayloadBytes     uint64 `json:"payloadBytes"`
	PayloadSaved     bool   `json:"payloadSaved"`
	NotSavedReason   string `json:"notSavedReason,omitempty"`
}

type DefinitionAuthoringDiagnosticView struct {
	WorkspaceID      string `json:"workspaceId"`
	PermitID         string `json:"permitId"`
	Phase            string `json:"phase"`
	FailureCode      string `json:"failureCode"`
	ParseError       string `json:"parseError,omitempty"`
	ParseErrorSHA256 string `json:"parseErrorSHA256,omitempty"`
	ParseErrorBytes  uint64 `json:"parseErrorBytes"`
	ParseErrorSaved  bool   `json:"parseErrorSaved"`
	PayloadBase64    string `json:"payloadBase64,omitempty"`
	PayloadSHA256    string `json:"payloadSHA256"`
	PayloadBytes     uint64 `json:"payloadBytes"`
	PayloadSaved     bool   `json:"payloadSaved"`
	NotSavedReason   string `json:"notSavedReason,omitempty"`
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
	GoalText           string                            `json:"goalText"`
	Items              []DefinitionAuthoringItem         `json:"items"`
	Resources          []DefinitionAuthoringResource     `json:"resources,omitempty"`
	Verifications      []DefinitionAuthoringVerification `json:"verifications,omitempty"`
	Reviews            []DefinitionAuthoringReview       `json:"reviews,omitempty"`
	ProposalHash       string                            `json:"proposalHash"`
	SpecificationHash  string                            `json:"specificationHash,omitempty"`
	DerivationPermitID string                            `json:"derivationPermitId"`
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
	CompilationAccepted    bool                              `json:"compilationAccepted,omitempty"`
	WorkspaceID            string                            `json:"workspaceId"`
	WorkspaceRevision      uint64                            `json:"workspaceRevision"`
	State                  string                            `json:"state"`
	Input                  *DefinitionAuthoringInput         `json:"input,omitempty"`
	InputRevision          uint64                            `json:"inputRevision,omitempty"`
	InputHash              string                            `json:"inputHash,omitempty"`
	TemplateID             string                            `json:"templateId,omitempty"`
	TemplateCurrent        bool                              `json:"templateCurrent"`
	Resources              []DefinitionAuthoringResource     `json:"resources,omitempty"`
	DerivationPermitID     string                            `json:"derivationPermitId,omitempty"`
	Requests               []DefinitionAuthoringRequestState `json:"requests,omitempty"`
	Proposal               *DefinitionAuthoringProposal      `json:"proposal,omitempty"`
	ConfirmationPhase      string                            `json:"confirmationPhase,omitempty"`
	ConfirmationRecovery   *DefinitionAuthoringConfirm       `json:"confirmationRecovery,omitempty"`
	CandidateHash          string                            `json:"candidateHash,omitempty"`
	AdmissionDisposition   string                            `json:"admissionDisposition,omitempty"`
	AdmissionViolationCode string                            `json:"admissionViolationCode,omitempty"`
	AdmissionOffendingRefs []string                          `json:"admissionOffendingRefs,omitempty"`
	BlockingTicketID       string                            `json:"blockingTicketId,omitempty"`
	StartupInputHash       string                            `json:"startupInputHash,omitempty"`
	ActivationState        string                            `json:"activationState,omitempty"`
}
