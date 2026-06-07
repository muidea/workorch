package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
)

func artifactFromModel(item *resultmodel.Artifact) readmodelcore.Artifact {
	return readmodelcore.Artifact{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		RunID:              string(item.RunID),
		ResultID:           string(item.ResultID),
		StepID:             string(item.StepID),
		ToolCallID:         string(item.ToolCallID),
		PatchID:            string(item.PatchID),
		ValidationResultID: string(item.ValidationResultID),
		ArtifactType:       item.ArtifactType,
		Category:           item.Category,
		Name:               item.Name,
		Summary:            item.Summary,
		URI:                item.URI,
		ContentType:        item.ContentType,
		State:              string(item.State),
		EvidenceRefs:       cloneReadModelStringSlice(item.EvidenceRefs),
		Metadata:           cloneReadModelMap(item.Metadata),
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}
