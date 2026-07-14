package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type EvidenceRecordRepository struct {
	mu    sync.RWMutex
	items map[string]*orchestratormodel.EvidenceRecord
}

func NewEvidenceRecordRepository() *EvidenceRecordRepository {
	return &EvidenceRecordRepository{items: map[string]*orchestratormodel.EvidenceRecord{}}
}

func (r *EvidenceRecordRepository) Create(_ context.Context, item *orchestratormodel.EvidenceRecord) error {
	if !validEvidenceRecord(item) {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if _, ok := r.items[key]; ok {
		return common.ErrConflict
	}
	r.items[key] = cloneEvidenceRecord(item)
	return nil
}

func (r *EvidenceRecordRepository) Get(ctx context.Context, id common.EvidenceRecordID) (*orchestratormodel.EvidenceRecord, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *orchestratormodel.EvidenceRecord) string {
		return string(item.ID)
	})
	if err != nil {
		return nil, err
	}
	return cloneEvidenceRecord(r.items[key]), nil
}

func (r *EvidenceRecordRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*orchestratormodel.EvidenceRecord, error) {
	if runID == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	matches, conflict := runScopedListHasConflict(ctx, r.items, runID, func(item *orchestratormodel.EvidenceRecord) common.RunID {
		return item.RunID
	}, func(item *orchestratormodel.EvidenceRecord) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	return sortedEvidenceRecordSlice(matches), nil
}

func (r *EvidenceRecordRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*orchestratormodel.EvidenceRecord, error) {
	if stepID == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	matches, conflict := stepScopedListHasConflict(ctx, r.items, stepID, func(item *orchestratormodel.EvidenceRecord) common.StepID {
		return item.StepID
	}, func(item *orchestratormodel.EvidenceRecord) common.WorkspaceID {
		return item.WorkspaceID
	})
	if conflict {
		return nil, common.ErrConflict
	}
	return sortedEvidenceRecordSlice(matches), nil
}

func (r *EvidenceRecordRepository) ListBySourceRef(ctx context.Context, runID common.RunID, sourceRef string) ([]*orchestratormodel.EvidenceRecord, error) {
	items, err := r.ListByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	ret := items[:0]
	for _, item := range items {
		if item.SourceRef == sourceRef {
			ret = append(ret, item)
		}
	}
	return nilIfEmptyEvidenceRecords(ret), nil
}

func (r *EvidenceRecordRepository) Update(ctx context.Context, item *orchestratormodel.EvidenceRecord) error {
	if !validEvidenceRecord(item) {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := workspaceScopedKey(item.WorkspaceID, string(item.ID))
	if item.WorkspaceID == "" {
		found, err := findScopedMemoryKey(ctx, r.items, string(item.ID), func(current *orchestratormodel.EvidenceRecord) string {
			return string(current.ID)
		})
		if err != nil {
			return err
		}
		key = found
		item.WorkspaceID = r.items[key].WorkspaceID
	}
	if _, ok := r.items[key]; !ok {
		return common.ErrNotFound
	}
	r.items[key] = cloneEvidenceRecord(item)
	return nil
}

func (r *EvidenceRecordRepository) Delete(ctx context.Context, id common.EvidenceRecordID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := findScopedMemoryKey(ctx, r.items, string(id), func(item *orchestratormodel.EvidenceRecord) string {
		return string(item.ID)
	})
	if err != nil {
		return err
	}
	delete(r.items, key)
	return nil
}

func validEvidenceRecord(item *orchestratormodel.EvidenceRecord) bool {
	return item != nil &&
		item.ID != "" &&
		item.WorkspaceID != "" &&
		item.RunID != "" &&
		item.Kind != "" &&
		item.Status != "" &&
		item.SourceRef != ""
}

func sortedEvidenceRecordSlice(items []*orchestratormodel.EvidenceRecord) []*orchestratormodel.EvidenceRecord {
	ret := make([]*orchestratormodel.EvidenceRecord, 0, len(items))
	for _, item := range items {
		ret = append(ret, cloneEvidenceRecord(item))
	}
	sortEvidenceRecords(ret)
	return nilIfEmptyEvidenceRecords(ret)
}

func nilIfEmptyEvidenceRecords(items []*orchestratormodel.EvidenceRecord) []*orchestratormodel.EvidenceRecord {
	if len(items) == 0 {
		return nil
	}
	return items
}

func sortEvidenceRecords(items []*orchestratormodel.EvidenceRecord) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].RunID != items[j].RunID {
			return string(items[i].RunID) < string(items[j].RunID)
		}
		if items[i].StepID != items[j].StepID {
			return string(items[i].StepID) < string(items[j].StepID)
		}
		if items[i].CreatedAt != items[j].CreatedAt {
			return items[i].CreatedAt < items[j].CreatedAt
		}
		return string(items[i].ID) < string(items[j].ID)
	})
}

func cloneEvidenceRecord(item *orchestratormodel.EvidenceRecord) *orchestratormodel.EvidenceRecord {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.CoverageItemRefs = append([]string(nil), item.CoverageItemRefs...)
	cloned.Supersedes = append([]common.EvidenceRecordID(nil), item.Supersedes...)
	cloned.Metadata = item.Metadata.Clone()
	return &cloned
}
