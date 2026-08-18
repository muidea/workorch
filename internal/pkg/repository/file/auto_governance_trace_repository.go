package file

import (
	"context"
	"sort"
	"strings"
	"sync"

	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type AutoGovernanceTraceRepository struct {
	mu   sync.RWMutex
	path string
}

func NewAutoGovernanceTraceRepository(path string) *AutoGovernanceTraceRepository {
	return &AutoGovernanceTraceRepository{path: path}
}

func (r *AutoGovernanceTraceRepository) Create(_ context.Context, item *governancemodel.AutoGovernanceTrace) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" || strings.TrimSpace(item.ItemType) == "" || strings.TrimSpace(item.ItemID) == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	items, err := loadMap[common.AutoGovernanceTraceID, governancemodel.AutoGovernanceTrace](r.path)
	if err != nil {
		return err
	}
	if _, ok := items[item.ID]; ok {
		return common.ErrConflict
	}
	items[item.ID] = *cloneAutoGovernanceTrace(item)
	return saveMap(r.path, items)
}

func (r *AutoGovernanceTraceRepository) Upsert(_ context.Context, item *governancemodel.AutoGovernanceTrace) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" || strings.TrimSpace(item.ItemType) == "" || strings.TrimSpace(item.ItemID) == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	items, err := loadMap[common.AutoGovernanceTraceID, governancemodel.AutoGovernanceTrace](r.path)
	if err != nil {
		return err
	}
	if existing, ok := items[item.ID]; ok && !sameAutoGovernanceTraceIdentity(&existing, item) {
		return common.ErrConflict
	}
	items[item.ID] = *cloneAutoGovernanceTrace(item)
	return saveMap(r.path, items)
}

func sameAutoGovernanceTraceIdentity(left *governancemodel.AutoGovernanceTrace, right *governancemodel.AutoGovernanceTrace) bool {
	return left != nil && right != nil &&
		left.ID == right.ID &&
		left.WorkspaceID == right.WorkspaceID &&
		left.RunID == right.RunID &&
		left.StepID == right.StepID &&
		strings.EqualFold(strings.TrimSpace(left.ItemType), strings.TrimSpace(right.ItemType)) &&
		strings.TrimSpace(left.ItemID) == strings.TrimSpace(right.ItemID)
}

func (r *AutoGovernanceTraceRepository) Get(_ context.Context, id common.AutoGovernanceTraceID) (*governancemodel.AutoGovernanceTrace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items, err := loadMap[common.AutoGovernanceTraceID, governancemodel.AutoGovernanceTrace](r.path)
	if err != nil {
		return nil, err
	}
	item, ok := items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneAutoGovernanceTrace(&item), nil
}

func (r *AutoGovernanceTraceRepository) List(_ context.Context, query workorchquerycontract.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items, err := loadMap[common.AutoGovernanceTraceID, governancemodel.AutoGovernanceTrace](r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]*governancemodel.AutoGovernanceTrace, 0, len(items))
	for _, item := range items {
		if !autoGovernanceTraceMatches(&item, query) {
			continue
		}
		ret = append(ret, cloneAutoGovernanceTrace(&item))
	}
	sortAutoGovernanceTraces(ret)
	if query.Limit > 0 && len(ret) > query.Limit {
		ret = ret[:query.Limit]
	}
	return ret, nil
}

func (r *AutoGovernanceTraceRepository) Delete(_ context.Context, id common.AutoGovernanceTraceID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deleteMapItem[common.AutoGovernanceTraceID, governancemodel.AutoGovernanceTrace](r.path, id)
}

func cloneAutoGovernanceTrace(item *governancemodel.AutoGovernanceTrace) *governancemodel.AutoGovernanceTrace {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
	return &cloned
}

func autoGovernanceTraceMatches(item *governancemodel.AutoGovernanceTrace, query workorchquerycontract.AutoGovernanceTraceQuery) bool {
	if item == nil {
		return false
	}
	if query.WorkspaceID != "" && item.WorkspaceID != query.WorkspaceID {
		return false
	}
	if query.RunID != "" && item.RunID != query.RunID {
		return false
	}
	if query.StepID != "" && item.StepID != query.StepID {
		return false
	}
	if query.ItemType != "" && !strings.EqualFold(item.ItemType, query.ItemType) {
		return false
	}
	if query.ItemID != "" && item.ItemID != query.ItemID {
		return false
	}
	if query.Status != "" && !strings.EqualFold(item.Status, query.Status) {
		return false
	}
	return true
}

func sortAutoGovernanceTraces(items []*governancemodel.AutoGovernanceTrace) {
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		if left == nil || right == nil {
			return left != nil
		}
		if left.OccurredAt != right.OccurredAt {
			return left.OccurredAt < right.OccurredAt
		}
		return left.ID < right.ID
	})
}
