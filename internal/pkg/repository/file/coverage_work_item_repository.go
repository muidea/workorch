package file

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type CoverageWorkItemRepository struct {
	mu   sync.RWMutex
	path string
}

func NewCoverageWorkItemRepository(path string) *CoverageWorkItemRepository {
	return &CoverageWorkItemRepository{path: path}
}

func (r *CoverageWorkItemRepository) Create(_ context.Context, item *orchestratormodel.CoverageWorkItem) error {
	if !validCoverageWorkItem(item) {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	path, err := r.itemPath(item.ID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if item.IsActive() {
		conflict, err := r.hasActiveSourceConflictLocked(item, item.ID)
		if err != nil {
			return err
		}
		if conflict {
			return common.ErrConflict
		}
	}
	return writeJSONFileAtomic(path, cloneCoverageWorkItem(item))
}

func (r *CoverageWorkItemRepository) Get(_ context.Context, id common.CoverageWorkItemID) (*orchestratormodel.CoverageWorkItem, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	path, err := r.itemPath(id)
	if err != nil {
		return nil, err
	}
	item, err := readJSONItemFile[orchestratormodel.CoverageWorkItem](path)
	if err != nil {
		return nil, err
	}
	return cloneCoverageWorkItem(&item), nil
}

func (r *CoverageWorkItemRepository) ListByRun(_ context.Context, runID common.RunID) ([]*orchestratormodel.CoverageWorkItem, error) {
	if runID == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listLocked(func(item *orchestratormodel.CoverageWorkItem) bool {
		return item.RunID == runID
	})
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
	path, err := r.itemPath(item.ID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	if item.IsActive() {
		conflict, err := r.hasActiveSourceConflictLocked(item, item.ID)
		if err != nil {
			return err
		}
		if conflict {
			return common.ErrConflict
		}
	}
	return writeJSONFileAtomic(path, cloneCoverageWorkItem(item))
}

func (r *CoverageWorkItemRepository) Delete(_ context.Context, id common.CoverageWorkItemID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	path, err := r.itemPath(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return os.Remove(path)
}

func (r *CoverageWorkItemRepository) itemPath(id common.CoverageWorkItemID) (string, error) {
	value := strings.TrimSpace(string(id))
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\`) {
		return "", common.ErrInvalidArgument
	}
	return filepath.Join(r.path, value+".json"), nil
}

func (r *CoverageWorkItemRepository) listLocked(matches func(*orchestratormodel.CoverageWorkItem) bool) ([]*orchestratormodel.CoverageWorkItem, error) {
	paths, err := repositoryItemPaths(r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*orchestratormodel.CoverageWorkItem, 0, len(paths))
	for _, path := range paths {
		item, err := readJSONItemFile[orchestratormodel.CoverageWorkItem](path)
		if err != nil {
			return nil, err
		}
		if matches(&item) {
			ret = append(ret, cloneCoverageWorkItem(&item))
		}
	}
	sortCoverageWorkItems(ret)
	return nilIfEmptyCoverageWorkItems(ret), nil
}

func (r *CoverageWorkItemRepository) hasActiveSourceConflictLocked(item *orchestratormodel.CoverageWorkItem, ignore common.CoverageWorkItemID) (bool, error) {
	items, err := r.listLocked(func(existing *orchestratormodel.CoverageWorkItem) bool {
		return existing.ID != ignore &&
			existing.IsActive() &&
			existing.RunID == item.RunID &&
			existing.DefinitionRevision == item.DefinitionRevision &&
			existing.NormalizedSourceKey == item.NormalizedSourceKey
	})
	return len(items) > 0, err
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
