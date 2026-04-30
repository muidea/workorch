package result

import "github.com/muidea/workorch/internal/pkg/common"

type Result struct {
	ID            common.ResultID
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID
	GoalID        common.GoalID
	DoDID         common.DoDID
	State         common.ResultState
	Title         string
	Summary       string
	SourceType    string
	SourceRef     string
	EvidenceRefs  []string
	ArtifactRefs  []common.ArtifactID
	DoDAssessment DoDAssessment
	SummaryDetail ResultSummary
	CreatedAt     string
	UpdatedAt     string
}

type ResultSummary struct {
	GoalOutcome           string
	DoDItems              []DoDAssessmentItem
	ImplementationSummary string
	VerificationSummary   string
	EvidenceRefs          []string
	ArtifactRefs          []common.ArtifactID
	KnownLimitations      []string
	ResidualRisks         []string
	NextActions           []string
}

type DoDAssessment struct {
	Satisfied bool
	Items     []DoDAssessmentItem
	Summary   string
}

type DoDAssessmentItem struct {
	Item          string
	State         string
	EvidenceRefs  []string
	FailureReason string
	Confidence    float64
	SourceNote    string
}
