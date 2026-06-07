package readmodelcore

type Result struct {
	ID            string         `json:"id"`
	WorkspaceID   string         `json:"workspaceId"`
	RunID         string         `json:"runId"`
	GoalID        string         `json:"goalId,omitempty"`
	DoDID         string         `json:"doDId,omitempty"`
	State         string         `json:"state"`
	Title         string         `json:"title,omitempty"`
	Summary       string         `json:"summary,omitempty"`
	SourceType    string         `json:"sourceType,omitempty"`
	SourceRef     string         `json:"sourceRef,omitempty"`
	EvidenceRefs  []string       `json:"evidenceRefs,omitempty"`
	ArtifactRefs  []string       `json:"artifactRefs,omitempty"`
	DoDAssessment *DoDAssessment `json:"doDAssessment,omitempty"`
	SummaryDetail *ResultSummary `json:"summaryDetail,omitempty"`
	CreatedAt     string         `json:"createdAt,omitempty"`
	UpdatedAt     string         `json:"updatedAt,omitempty"`
}

type ResultSummary struct {
	GoalOutcome           string              `json:"goalOutcome,omitempty"`
	DoDItems              []DoDAssessmentItem `json:"doDItems,omitempty"`
	ImplementationSummary string              `json:"implementationSummary,omitempty"`
	VerificationSummary   string              `json:"verificationSummary,omitempty"`
	EvidenceRefs          []string            `json:"evidenceRefs,omitempty"`
	ArtifactRefs          []string            `json:"artifactRefs,omitempty"`
	KnownLimitations      []string            `json:"knownLimitations,omitempty"`
	ResidualRisks         []string            `json:"residualRisks,omitempty"`
	NextActions           []string            `json:"nextActions,omitempty"`
}
