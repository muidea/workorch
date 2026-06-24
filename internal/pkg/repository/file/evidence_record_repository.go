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

type EvidenceRecordRepository struct {
	mu   sync.RWMutex
	path string
}

func NewEvidenceRecordRepository(path string) *EvidenceRecordRepository {
	return &EvidenceRecordRepository{path: path}
}

func (r *EvidenceRecordRepository) Create(_ context.Context, item *orchestratormodel.EvidenceRecord) error {
	if !validEvidenceRecord(item) {
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
	return writeJSONFileAtomic(path, cloneEvidenceRecord(item))
}

func (r *EvidenceRecordRepository) Get(_ context.Context, id common.EvidenceRecordID) (*orchestratormodel.EvidenceRecord, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	path, err := r.itemPath(id)
	if err != nil {
		return nil, err
	}
	item, err := readJSONItemFile[orchestratormodel.EvidenceRecord](path)
	if err != nil {
		return nil, err
	}
	return cloneEvidenceRecord(&item), nil
}

func (r *EvidenceRecordRepository) ListByRun(_ context.Context, runID common.RunID) ([]*orchestratormodel.EvidenceRecord, error) {
	if runID == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listLocked(func(item *orchestratormodel.EvidenceRecord) bool {
		return item.RunID == runID
	})
}

func (r *EvidenceRecordRepository) ListByStep(_ context.Context, stepID common.StepID) ([]*orchestratormodel.EvidenceRecord, error) {
	if stepID == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listLocked(func(item *orchestratormodel.EvidenceRecord) bool {
		return item.StepID == stepID
	})
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

func (r *EvidenceRecordRepository) Update(_ context.Context, item *orchestratormodel.EvidenceRecord) error {
	if !validEvidenceRecord(item) {
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
	return writeJSONFileAtomic(path, cloneEvidenceRecord(item))
}

func (r *EvidenceRecordRepository) Delete(_ context.Context, id common.EvidenceRecordID) error {
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

func (r *EvidenceRecordRepository) itemPath(id common.EvidenceRecordID) (string, error) {
	value := strings.TrimSpace(string(id))
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\`) {
		return "", common.ErrInvalidArgument
	}
	return filepath.Join(r.path, value+".json"), nil
}

func (r *EvidenceRecordRepository) listLocked(matches func(*orchestratormodel.EvidenceRecord) bool) ([]*orchestratormodel.EvidenceRecord, error) {
	paths, err := repositoryItemPaths(r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*orchestratormodel.EvidenceRecord, 0, len(paths))
	for _, path := range paths {
		item, err := readJSONItemFile[orchestratormodel.EvidenceRecord](path)
		if err != nil {
			return nil, err
		}
		if matches == nil || matches(&item) {
			ret = append(ret, cloneEvidenceRecord(&item))
		}
	}
	sortEvidenceRecords(ret)
	return nilIfEmptyEvidenceRecords(ret), nil
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
	cloned.Metadata = cloneMap(item.Metadata)
	return &cloned
}
