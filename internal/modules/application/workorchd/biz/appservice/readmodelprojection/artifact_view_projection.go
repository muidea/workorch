package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func ProjectArtifactView(item projectioncontract.Artifact) projectioncontract.Artifact {
	return projectioncontract.Artifact{
		ID:                 item.ID,
		WorkspaceID:        item.WorkspaceID,
		RunID:              item.RunID,
		ResultID:           item.ResultID,
		StepID:             item.StepID,
		ToolCallID:         item.ToolCallID,
		PatchID:            item.PatchID,
		ValidationResultID: item.ValidationResultID,
		ArtifactType:       item.ArtifactType,
		Category:           item.Category,
		Name:               item.Name,
		Summary:            item.Summary,
		URI:                item.URI,
		ContentType:        item.ContentType,
		State:              item.State,
		EvidenceRefs:       cloneReadModelStringSlice(item.EvidenceRefs),
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}
