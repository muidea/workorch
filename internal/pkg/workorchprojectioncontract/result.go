package workorchprojectioncontract

import (
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type Result struct {
	OriginalDiagnostic *runtimecontract.RuntimeOriginalDiagnostic `json:"originalDiagnostic,omitempty"`
	RuntimeCloseout    *runtimecontract.RuntimeResultCloseout     `json:"runtimeCloseout,omitempty"`
	ID                 string                                     `json:"id"`
	WorkspaceID        string                                     `json:"workspaceId"`
	RunID              string                                     `json:"runId"`
	GoalID             string                                     `json:"goalId,omitempty"`
	DoDID              string                                     `json:"doDId,omitempty"`
	State              string                                     `json:"state"`
	Title              string                                     `json:"title,omitempty"`
	Summary            string                                     `json:"summary,omitempty"`
	SourceType         string                                     `json:"sourceType,omitempty"`
	SourceRef          string                                     `json:"sourceRef,omitempty"`
	EvidenceRefs       []string                                   `json:"evidenceRefs,omitempty"`
	ArtifactRefs       []string                                   `json:"artifactRefs,omitempty"`
	DoDAssessment      *workorchresultcontract.DoDAssessment      `json:"doDAssessment,omitempty"`
	SummaryDetail      *workorchresultcontract.ResultSummary      `json:"summaryDetail,omitempty"`
	CreatedAt          string                                     `json:"createdAt,omitempty"`
	UpdatedAt          string                                     `json:"updatedAt,omitempty"`
}

func CloneResult(item Result) Result {
	item.OriginalDiagnostic = item.OriginalDiagnostic.CloneOriginal()
	if item.RuntimeCloseout != nil {
		closeout := item.RuntimeCloseout.CloneRaw()
		item.RuntimeCloseout = &closeout
	}
	item.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
	item.ArtifactRefs = append([]string(nil), item.ArtifactRefs...)
	if item.DoDAssessment != nil {
		cloned := item.DoDAssessment.Clone()
		item.DoDAssessment = &cloned
	}
	if item.SummaryDetail != nil {
		cloned := item.SummaryDetail.Clone()
		item.SummaryDetail = &cloned
	}
	return item
}

func CloneResults(items []Result) []Result {
	if items == nil {
		return nil
	}
	ret := make([]Result, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneResult(item))
	}
	return ret
}
