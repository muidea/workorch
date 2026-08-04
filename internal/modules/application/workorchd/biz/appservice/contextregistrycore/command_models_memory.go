package contextregistrycore

import (
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func MemoryItemFromCommand(memoryID string, workspaceID string, cmd workorchmanagementcontract.MemoryMutation) *memorymodel.MemoryItem {
	return &memorymodel.MemoryItem{
		ID:                  common.MemoryID(memoryID),
		WorkspaceID:         common.WorkspaceID(workspaceID),
		RunID:               common.RunID(cmd.RunID),
		SourceType:          cmd.SourceType,
		SourceRef:           cmd.SourceRef,
		MemoryType:          cmd.MemoryType,
		Content:             cmd.Content,
		Summary:             cmd.Summary,
		Tags:                CloneStrings(cmd.Tags),
		Importance:          cmd.Importance,
		Confidence:          cmd.Confidence,
		Visibility:          cmd.Visibility,
		EmbeddingProviderID: cmd.EmbeddingProviderID,
		EmbeddingModel:      cmd.EmbeddingModel,
		EmbeddingText:       cmd.EmbeddingText,
		Vector:              CloneFloats(cmd.Vector),
	}
}

func MemoryItemFromMergedCommand(memoryID string, workspaceID common.WorkspaceID, cmd workorchmanagementcontract.MemoryMutation) *memorymodel.MemoryItem {
	return &memorymodel.MemoryItem{
		ID:                  common.MemoryID(memoryID),
		WorkspaceID:         workspaceID,
		RunID:               common.RunID(cmd.RunID),
		SourceType:          cmd.SourceType,
		SourceRef:           cmd.SourceRef,
		MemoryType:          cmd.MemoryType,
		Content:             cmd.Content,
		Summary:             cmd.Summary,
		Tags:                CloneStrings(cmd.Tags),
		Importance:          cmd.Importance,
		Confidence:          cmd.Confidence,
		Visibility:          cmd.Visibility,
		EmbeddingProviderID: cmd.EmbeddingProviderID,
		EmbeddingModel:      cmd.EmbeddingModel,
		EmbeddingText:       cmd.EmbeddingText,
		Vector:              CloneFloats(cmd.Vector),
	}
}
