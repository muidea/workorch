package memory

import (
	"context"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	memoryservice "github.com/muidea/workorch/internal/modules/kernel/memory/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service struct {
	items repository.MemoryItemRepository
}

func NewService(items repository.MemoryItemRepository) *Service {
	return &Service{items: items}
}

func (s *Service) CreateMemoryItem(ctx context.Context, cmd memoryservice.CreateMemoryItemCommand) error {
	if err := validateMemoryCommand(cmd); err != nil {
		return err
	}
	return s.items.Create(ctx, memoryItemFromCommand(cmd))
}

func (s *Service) UpdateMemoryItem(ctx context.Context, cmd memoryservice.UpdateMemoryItemCommand) error {
	if err := validateMemoryCommand(cmd); err != nil {
		return err
	}
	return s.items.Update(ctx, memoryItemFromCommand(cmd))
}

func (s *Service) GetMemoryItem(ctx context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.items.Get(ctx, id)
}

func (s *Service) DeleteMemoryItem(ctx context.Context, id common.MemoryID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	return s.items.Delete(ctx, id)
}

func (s *Service) ListMemoryByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.items.ListByWorkspaceID(ctx, workspaceID)
}

func (s *Service) SearchMemory(ctx context.Context, query repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	if query.WorkspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.items.Search(ctx, query)
}

func validateMemoryCommand(cmd memoryservice.CreateMemoryItemCommand) error {
	if cmd.MemoryID == "" || cmd.WorkspaceID == "" || cmd.SourceType == "" || cmd.MemoryType == "" || cmd.Summary == "" {
		return common.ErrInvalidArgument
	}
	if !modulecommon.IsSupportedSourceType(cmd.SourceType) || !modulecommon.IsSupportedMemoryType(cmd.MemoryType) {
		return common.ErrInvalidArgument
	}
	if !modulecommon.IsWritableMemoryTypeForSource(cmd.SourceType, cmd.MemoryType) {
		return common.ErrInvalidArgument
	}
	if hasBlank(cmd.Tags) {
		return common.ErrInvalidArgument
	}
	if cmd.SourceType != modulecommon.MemorySourceUser && cmd.SourceRef == "" {
		return common.ErrInvalidArgument
	}
	if cmd.SourceType == modulecommon.MemorySourceUser &&
		cmd.SourceRef == "" &&
		cmd.MemoryType != modulecommon.MemoryTypeProjectContext &&
		cmd.MemoryType != modulecommon.MemoryTypeUserPreference {
		return common.ErrInvalidArgument
	}
	if modulecommon.ContainsSensitiveMemoryText(cmd.Summary, cmd.Content, cmd.EmbeddingText, strings.Join(cmd.Tags, " ")) {
		return common.ErrInvalidArgument
	}
	return nil
}

func memoryItemFromCommand(cmd memoryservice.CreateMemoryItemCommand) *memorymodel.MemoryItem {
	return &memorymodel.MemoryItem{
		ID:                  cmd.MemoryID,
		WorkspaceID:         cmd.WorkspaceID,
		RunID:               cmd.RunID,
		SourceType:          cmd.SourceType,
		SourceRef:           cmd.SourceRef,
		MemoryType:          cmd.MemoryType,
		Content:             cmd.Content,
		Summary:             cmd.Summary,
		Tags:                cloneStrings(cmd.Tags),
		Importance:          cmd.Importance,
		Confidence:          cmd.Confidence,
		Visibility:          cmd.Visibility,
		EmbeddingProviderID: cmd.EmbeddingProviderID,
		EmbeddingModel:      cmd.EmbeddingModel,
		EmbeddingText:       cmd.EmbeddingText,
		Vector:              cloneFloats(cmd.Vector),
	}
}

func hasBlank(values []string) bool {
	for _, value := range values {
		if value == "" {
			return true
		}
	}
	return false
}

func cloneStrings(input []string) []string {
	if input == nil {
		return nil
	}
	return append([]string(nil), input...)
}

func cloneFloats(input []float64) []float64 {
	if input == nil {
		return nil
	}
	return append([]float64(nil), input...)
}
