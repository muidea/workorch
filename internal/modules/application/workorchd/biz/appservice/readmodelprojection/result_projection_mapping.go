package readmodelprojection

import (
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: result_item_mapping.go

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

// source: result_mapping.go

func mapResults(items []*resultmodel.Result) []projectioncontract.Result {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.Result, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, mapResult(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: result_single_projection.go

func projectResult(item *resultmodel.Result) projectioncontract.Result {
	if item == nil {
		return projectioncontract.Result{}
	}
	return projectioncontract.Result{
		ID:           string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		GoalID:       string(item.GoalID),
		DoDID:        string(item.DoDID),
		State:        string(item.State),
		Title:        item.Title,
		Summary:      item.Summary,
		SourceType:   item.SourceType,
		SourceRef:    item.SourceRef,
		EvidenceRefs: cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs: resultArtifactRefs(item),
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}
}
