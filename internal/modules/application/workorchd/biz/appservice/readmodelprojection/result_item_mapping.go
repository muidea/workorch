package readmodelprojection

import (
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mapResult(item *resultmodel.Result) projectioncontract.Result {
	return projectioncontract.Result{
		ID:            string(item.ID),
		WorkspaceID:   string(item.WorkspaceID),
		RunID:         string(item.RunID),
		GoalID:        string(item.GoalID),
		DoDID:         string(item.DoDID),
		State:         string(item.State),
		Title:         item.Title,
		Summary:       item.Summary,
		SourceType:    item.SourceType,
		SourceRef:     item.SourceRef,
		EvidenceRefs:  cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs:  resultArtifactRefs(item),
		DoDAssessment: mapDoDAssessment(item.DoDAssessment),
		SummaryDetail: mapResultSummary(item.SummaryDetail),
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}
}
