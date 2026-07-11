package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
)

func ProjectResult(item *resultmodel.Result) readmodelcore.Result {
	if item == nil {
		return readmodelcore.Result{}
	}
	return readmodelcore.Result{
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
