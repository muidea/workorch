package workorchprojectioncontract

import "github.com/muidea/workorch/internal/pkg/workorchresultcontract"

type Result struct {
	ID            string                                `json:"id"`
	WorkspaceID   string                                `json:"workspaceId"`
	RunID         string                                `json:"runId"`
	GoalID        string                                `json:"goalId,omitempty"`
	DoDID         string                                `json:"doDId,omitempty"`
	State         string                                `json:"state"`
	Title         string                                `json:"title,omitempty"`
	Summary       string                                `json:"summary,omitempty"`
	SourceType    string                                `json:"sourceType,omitempty"`
	SourceRef     string                                `json:"sourceRef,omitempty"`
	EvidenceRefs  []string                              `json:"evidenceRefs,omitempty"`
	ArtifactRefs  []string                              `json:"artifactRefs,omitempty"`
	DoDAssessment *workorchresultcontract.DoDAssessment `json:"doDAssessment,omitempty"`
	SummaryDetail *workorchresultcontract.ResultSummary `json:"summaryDetail,omitempty"`
	CreatedAt     string                                `json:"createdAt,omitempty"`
	UpdatedAt     string                                `json:"updatedAt,omitempty"`
}
