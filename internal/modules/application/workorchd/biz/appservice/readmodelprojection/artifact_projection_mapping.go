package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: artifact_item_mapping.go

func artifactFromModel(item *resultmodel.Artifact) projectioncontract.Artifact {
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
		Metadata:           cloneReadModelMap(item.Metadata.Map()),
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}

// source: artifact_mapping.go

func mapArtifacts(items []*resultmodel.Artifact) []projectioncontract.Artifact {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.Artifact, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, artifactFromModel(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: artifact_projection.go

func projectArtifact(item *resultmodel.Artifact) projectioncontract.Artifact {
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
