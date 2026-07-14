package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type CoverageWorkItemRepository struct {
	mu    sync.RWMutex
	items map[common.CoverageWorkItemID]*orchestratormodel.CoverageWorkItem
}

func NewCoverageWorkItemRepository() *CoverageWorkItemRepository {
	return &CoverageWorkItemRepository{items: map[common.CoverageWorkItemID]*orchestratormodel.CoverageWorkItem{}}
}

func (r *CoverageWorkItemRepository) Create(_ context.Context, item *orchestratormodel.CoverageWorkItem) error {
	if !validCoverageWorkItem(item) {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	if item.IsActive() && r.hasActiveSourceConflictLocked(item, item.ID) {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneCoverageWorkItem(item)
	return nil
}

func (r *CoverageWorkItemRepository) Get(_ context.Context, id common.CoverageWorkItemID) (*orchestratormodel.CoverageWorkItem, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneCoverageWorkItem(item), nil
}

func (r *CoverageWorkItemRepository) ListByRun(_ context.Context, runID common.RunID) ([]*orchestratormodel.CoverageWorkItem, error) {
	if runID == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	ret := make([]*orchestratormodel.CoverageWorkItem, 0)
	for _, item := range r.items {
		if item.RunID == runID {
			ret = append(ret, cloneCoverageWorkItem(item))
		}
	}
	sortCoverageWorkItems(ret)
	return nilIfEmptyCoverageWorkItems(ret), nil
}

func (r *CoverageWorkItemRepository) ListActiveByRun(ctx context.Context, runID common.RunID) ([]*orchestratormodel.CoverageWorkItem, error) {
	items, err := r.ListByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	ret := items[:0]
	for _, item := range items {
		if item.IsActive() {
			ret = append(ret, item)
		}
	}
	return nilIfEmptyCoverageWorkItems(ret), nil
}

func (r *CoverageWorkItemRepository) Update(_ context.Context, item *orchestratormodel.CoverageWorkItem) error {
	if !validCoverageWorkItem(item) {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	if item.IsActive() && r.hasActiveSourceConflictLocked(item, item.ID) {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneCoverageWorkItem(item)
	return nil
}

func (r *CoverageWorkItemRepository) Delete(_ context.Context, id common.CoverageWorkItemID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *CoverageWorkItemRepository) hasActiveSourceConflictLocked(item *orchestratormodel.CoverageWorkItem, ignore common.CoverageWorkItemID) bool {
	for id, existing := range r.items {
		if id == ignore || !existing.IsActive() {
			continue
		}
		if existing.RunID == item.RunID &&
			existing.DefinitionRevision == item.DefinitionRevision &&
			existing.NormalizedSourceKey == item.NormalizedSourceKey {
			return true
		}
	}
	return false
}

func validCoverageWorkItem(item *orchestratormodel.CoverageWorkItem) bool {
	return item != nil &&
		item.ID != "" &&
		item.WorkspaceID != "" &&
		item.RunID != "" &&
		item.DefinitionRevision != "" &&
		item.NormalizedSourceKey != ""
}

func nilIfEmptyCoverageWorkItems(items []*orchestratormodel.CoverageWorkItem) []*orchestratormodel.CoverageWorkItem {
	if len(items) == 0 {
		return nil
	}
	return items
}

func sortCoverageWorkItems(items []*orchestratormodel.CoverageWorkItem) {
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		if left.RunID != right.RunID {
			return left.RunID < right.RunID
		}
		if left.DefinitionRevision != right.DefinitionRevision {
			return left.DefinitionRevision < right.DefinitionRevision
		}
		if left.NormalizedSourceKey != right.NormalizedSourceKey {
			return left.NormalizedSourceKey < right.NormalizedSourceKey
		}
		return left.ID < right.ID
	})
}

func cloneCoverageWorkItem(item *orchestratormodel.CoverageWorkItem) *orchestratormodel.CoverageWorkItem {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.RequiredEvidenceKinds = append([]string(nil), item.RequiredEvidenceKinds...)
	cloned.AcceptedEvidenceRefs = append([]string(nil), item.AcceptedEvidenceRefs...)
	cloned.RejectedEvidenceRefs = append([]string(nil), item.RejectedEvidenceRefs...)
	cloned.Metadata = item.Metadata.Clone()
	return &cloned
}
