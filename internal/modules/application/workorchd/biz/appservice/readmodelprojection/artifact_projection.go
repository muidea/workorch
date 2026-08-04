package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
)

func ProjectArtifact(item *resultmodel.Artifact) projectioncontract.Artifact {
	if item == nil {
		return projectioncontract.Artifact{}
	}
	return projectioncontract.Artifact{
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
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}
