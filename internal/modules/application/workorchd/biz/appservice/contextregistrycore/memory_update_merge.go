package contextregistrycore

import (
	"strings"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
)

func MergeMemoryUpdateCommand(existing *memorymodel.MemoryItem, cmd UpdateMemoryItemCommand) UpdateMemoryItemCommand {
	if existing == nil {
		return cmd
	}
	ret := UpdateMemoryItemCommand{
		MemoryID:            string(existing.ID),
		RunID:               string(existing.RunID),
		SourceType:          existing.SourceType,
		SourceRef:           existing.SourceRef,
		MemoryType:          existing.MemoryType,
		Content:             existing.Content,
		Summary:             existing.Summary,
		Tags:                CloneStrings(existing.Tags),
		Importance:          existing.Importance,
		Confidence:          existing.Confidence,
		Visibility:          existing.Visibility,
		EmbeddingProviderID: existing.EmbeddingProviderID,
		EmbeddingModel:      existing.EmbeddingModel,
		EmbeddingText:       existing.EmbeddingText,
		Vector:              CloneFloats(existing.Vector),
	}
	if strings.TrimSpace(cmd.RunID) != "" {
		ret.RunID = strings.TrimSpace(cmd.RunID)
	}
	if strings.TrimSpace(cmd.SourceType) != "" {
		ret.SourceType = strings.TrimSpace(cmd.SourceType)
	}
	if strings.TrimSpace(cmd.SourceRef) != "" {
		ret.SourceRef = strings.TrimSpace(cmd.SourceRef)
	}
	if strings.TrimSpace(cmd.MemoryType) != "" {
		ret.MemoryType = strings.TrimSpace(cmd.MemoryType)
	}
	if cmd.Content != "" {
		ret.Content = cmd.Content
	}
	if strings.TrimSpace(cmd.Summary) != "" {
		ret.Summary = strings.TrimSpace(cmd.Summary)
	}
	if cmd.Tags != nil {
		ret.Tags = CloneStrings(cmd.Tags)
	}
	if cmd.Importance != 0 {
		ret.Importance = cmd.Importance
	}
	if cmd.Confidence != 0 {
		ret.Confidence = cmd.Confidence
	}
	if strings.TrimSpace(cmd.Visibility) != "" {
		ret.Visibility = strings.TrimSpace(cmd.Visibility)
	}
	if strings.TrimSpace(cmd.EmbeddingProviderID) != "" {
		ret.EmbeddingProviderID = strings.TrimSpace(cmd.EmbeddingProviderID)
	}
	if strings.TrimSpace(cmd.EmbeddingModel) != "" {
		ret.EmbeddingModel = strings.TrimSpace(cmd.EmbeddingModel)
	}
	if strings.TrimSpace(cmd.EmbeddingText) != "" {
		ret.EmbeddingText = strings.TrimSpace(cmd.EmbeddingText)
	}
	if cmd.Vector != nil {
		ret.Vector = CloneFloats(cmd.Vector)
	}
	return ret
}
