package memory

import (
	"context"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type Service struct {
	items  repository.MemoryItemRepository
	traces repository.MemoryRecallTraceRepository
}

func NewService(items repository.MemoryItemRepository) *Service {
	return &Service{items: items}
}

func NewServiceWithRecallTraces(items repository.MemoryItemRepository, traces repository.MemoryRecallTraceRepository) *Service {
	return &Service{items: items, traces: traces}
}

func (s *Service) CreateMemoryItem(ctx context.Context, cmd memoryevents.CreateMemoryCommand) error {
	if err := validateMemoryCommand(cmd); err != nil {
		return err
	}
	return s.items.Create(ctx, memoryItemFromCommand(cmd))
}

func (s *Service) UpdateMemoryItem(ctx context.Context, cmd memoryevents.CreateMemoryCommand) error {
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

func (s *Service) SearchMemory(ctx context.Context, query workorchquerycontract.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	if query.WorkspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.items.Search(ctx, query)
}

func (s *Service) UpsertMemoryRecallTrace(ctx context.Context, trace *memorymodel.MemoryRecallTrace) error {
	if s == nil || s.traces == nil || trace == nil || trace.ID == "" || trace.WorkspaceID == "" || trace.RunID == "" {
		return common.ErrInvalidArgument
	}
	return s.traces.Upsert(ctx, trace)
}

func (s *Service) GetMemoryRecallTrace(ctx context.Context, id common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error) {
	if s == nil || s.traces == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.traces.Get(ctx, id)
}

func (s *Service) ListMemoryRecallTraces(ctx context.Context, query workorchquerycontract.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	if s == nil || s.traces == nil {
		return nil, common.ErrInvalidArgument
	}
	return s.traces.List(ctx, query)
}

func (s *Service) DeleteMemoryRecallTrace(ctx context.Context, id common.MemoryRecallTraceID) error {
	if s == nil || s.traces == nil || id == "" {
		return common.ErrInvalidArgument
	}
	return s.traces.Delete(ctx, id)
}

func validateMemoryCommand(cmd memoryevents.CreateMemoryCommand) error {
	if cmd.MemoryID == "" ||
		cmd.WorkspaceID == "" ||
		strings.TrimSpace(cmd.SourceType) == "" ||
		strings.TrimSpace(cmd.MemoryType) == "" ||
		strings.TrimSpace(cmd.Summary) == "" {
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
	if cmd.SourceType != modulecommon.MemorySourceUser && strings.TrimSpace(cmd.SourceRef) == "" {
		return common.ErrInvalidArgument
	}
	if cmd.SourceType == modulecommon.MemorySourceUser &&
		strings.TrimSpace(cmd.SourceRef) == "" &&
		cmd.MemoryType != modulecommon.MemoryTypeProjectContext &&
		cmd.MemoryType != modulecommon.MemoryTypeUserPreference {
		return common.ErrInvalidArgument
	}
	if modulecommon.ContainsSensitiveMemoryText(cmd.Summary, cmd.Content, cmd.EmbeddingText, strings.Join(cmd.Tags, " ")) {
		return common.ErrInvalidArgument
	}
	return nil
}

func memoryItemFromCommand(cmd memoryevents.CreateMemoryCommand) *memorymodel.MemoryItem {
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
		if strings.TrimSpace(value) == "" {
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
