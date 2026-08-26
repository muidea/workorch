package memory

import (
	"context"
	"errors"
	"math"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type service struct {
	items  repository.MemoryItemRepository
	traces repository.MemoryRecallTraceRepository
}

func newService(items repository.MemoryItemRepository) *service {
	return &service{items: items}
}

func newServiceWithRecallTraces(items repository.MemoryItemRepository, traces repository.MemoryRecallTraceRepository) *service {
	return &service{items: items, traces: traces}
}

func (s *service) createMemoryItem(ctx context.Context, cmd memoryevents.CreateMemoryCommand) error {
	if err := validateMemoryCommand(cmd); err != nil {
		return err
	}
	return s.items.Create(ctx, memoryItemFromCommand(cmd))
}

func (s *service) updateMemoryItem(ctx context.Context, cmd memoryevents.CreateMemoryCommand) error {
	if err := validateMemoryCommand(cmd); err != nil {
		return err
	}
	return s.items.Update(ctx, memoryItemFromCommand(cmd))
}

func (s *service) getMemoryItem(ctx context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.items.Get(ctx, id)
}

func (s *service) getMemoryItemScoped(ctx context.Context, workspaceID common.WorkspaceID, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	if workspaceID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	item, err := s.getMemoryItem(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.WorkspaceID != workspaceID {
		return nil, common.ErrConflict
	}
	return item, nil
}

func (s *service) deleteMemoryItem(ctx context.Context, id common.MemoryID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	return s.items.Delete(ctx, id)
}

func (s *service) listMemoryByWorkspace(ctx context.Context, workspaceID common.WorkspaceID, cursorAfterID common.MemoryID, limit int) ([]*memorymodel.MemoryItem, common.MemoryID, error) {
	if workspaceID == "" {
		return nil, "", common.ErrInvalidArgument
	}
	limit = normalizedLimit(limit, modulecommon.MemoryListDefaultLimit, modulecommon.MemoryListMaxLimit)
	items, err := s.items.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, "", err
	}
	page := make([]*memorymodel.MemoryItem, 0, min(limit+1, len(items)))
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID || (cursorAfterID != "" && string(item.ID) <= string(cursorAfterID)) {
			continue
		}
		page = append(page, item)
		if len(page) > limit {
			break
		}
	}
	var nextCursor common.MemoryID
	if len(page) > limit {
		nextCursor = page[limit-1].ID
		page = page[:limit]
	}
	return memorymodel.CloneMemoryItems(page), nextCursor, nil
}

func (s *service) searchMemory(ctx context.Context, query workorchquerycontract.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	query = query.Normalized()
	query.Limit = normalizedLimit(query.Limit, modulecommon.MemorySearchDefaultLimit, modulecommon.MemorySearchMaxLimit)
	if query.WorkspaceID == "" || len(query.Vector) > modulecommon.MemoryVectorMaxDimension || math.IsNaN(query.MinScore) || math.IsInf(query.MinScore, 0) || query.MinScore < -1 || query.MinScore > 1 {
		return nil, common.ErrInvalidArgument
	}
	for _, value := range query.Vector {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, common.ErrInvalidArgument
		}
	}
	for _, memoryType := range query.Types {
		if !modulecommon.IsSupportedMemoryType(memoryType) {
			return nil, common.ErrInvalidArgument
		}
	}
	items, err := s.items.Search(ctx, query.Clone())
	if err != nil {
		return nil, err
	}
	if len(items) > query.Limit {
		items = items[:query.Limit]
	}
	return memorymodel.CloneMemoryItems(items), nil
}

func (s *service) recordMemoryRecallTrace(ctx context.Context, trace *memorymodel.MemoryRecallTrace) (memoryevents.MemoryRecallTraceWriteDisposition, error) {
	if s == nil || s.traces == nil || trace == nil || trace.ID == "" || trace.WorkspaceID == "" || trace.RunID == "" {
		return "", common.ErrInvalidArgument
	}
	if err := s.traces.Create(ctx, memorymodel.CloneMemoryRecallTrace(trace)); err == nil {
		return memoryevents.MemoryRecallTraceRecorded, nil
	} else if !errors.Is(err, common.ErrConflict) {
		return "", err
	}
	existing, err := s.traces.Get(ctx, trace.ID)
	if err != nil {
		return "", err
	}
	if !memorymodel.SameMemoryRecallTrace(existing, trace) {
		return "", common.ErrConflict
	}
	return memoryevents.MemoryRecallTraceDuplicate, nil
}

func (s *service) loadMemoryRecallTrace(ctx context.Context, workspaceID common.WorkspaceID, id common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error) {
	if s == nil || s.traces == nil || workspaceID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	item, err := s.traces.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil || item.ID != id || item.WorkspaceID != workspaceID {
		return nil, common.ErrConflict
	}
	return memorymodel.CloneMemoryRecallTrace(item), nil
}

func (s *service) listMemoryRecallTraces(ctx context.Context, query workorchquerycontract.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, common.MemoryRecallTraceID, error) {
	query = query.Normalized()
	if s == nil || s.traces == nil || query.WorkspaceID == "" {
		return nil, "", common.ErrInvalidArgument
	}
	query.Limit = normalizedLimit(query.Limit, modulecommon.MemoryRecallListDefaultLimit, modulecommon.MemoryRecallListMaxLimit)
	repositoryQuery := query
	repositoryQuery.Limit = query.Limit + 1
	items, err := s.traces.List(ctx, repositoryQuery)
	if err != nil {
		return nil, "", err
	}
	var nextCursor common.MemoryRecallTraceID
	if len(items) > query.Limit {
		nextCursor = items[query.Limit-1].ID
		items = items[:query.Limit]
	}
	ret := make([]*memorymodel.MemoryRecallTrace, 0, len(items))
	for _, item := range items {
		if item == nil || item.WorkspaceID != query.WorkspaceID {
			return nil, "", common.ErrConflict
		}
		ret = append(ret, memorymodel.CloneMemoryRecallTrace(item))
	}
	return ret, nextCursor, nil
}

func normalizedLimit(value, defaultLimit, maxLimit int) int {
	if value <= 0 {
		return defaultLimit
	}
	if value > maxLimit {
		return maxLimit
	}
	return value
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
	if len(cmd.Vector) > modulecommon.MemoryVectorMaxDimension {
		return common.ErrInvalidArgument
	}
	for _, value := range cmd.Vector {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return common.ErrInvalidArgument
		}
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
