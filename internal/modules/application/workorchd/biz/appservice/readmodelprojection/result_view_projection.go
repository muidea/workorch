package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ProjectResultView(item readmodelcore.Result) readmodelcore.Result {
	return readmodelcore.Result{
		ID:           item.ID,
		WorkspaceID:  item.WorkspaceID,
		RunID:        item.RunID,
		GoalID:       item.GoalID,
		DoDID:        item.DoDID,
		State:        item.State,
		Title:        item.Title,
		Summary:      item.Summary,
		SourceType:   item.SourceType,
		SourceRef:    item.SourceRef,
		EvidenceRefs: cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs: cloneReadModelStringSlice(item.ArtifactRefs),
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}
}
