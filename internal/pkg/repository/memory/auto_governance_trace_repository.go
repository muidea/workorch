package memory

import (
	"context"
	"sort"
	"strings"
	"sync"

	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type AutoGovernanceTraceRepository struct {
	mu    sync.RWMutex
	items map[common.AutoGovernanceTraceID]*governancemodel.AutoGovernanceTrace
}

func NewAutoGovernanceTraceRepository() *AutoGovernanceTraceRepository {
	return &AutoGovernanceTraceRepository{items: map[common.AutoGovernanceTraceID]*governancemodel.AutoGovernanceTrace{}}
}

func (r *AutoGovernanceTraceRepository) Create(_ context.Context, item *governancemodel.AutoGovernanceTrace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" || strings.TrimSpace(item.ItemType) == "" || strings.TrimSpace(item.ItemID) == "" {
		return common.ErrInvalidArgument
	}
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneAutoGovernanceTrace(item)
	return nil
}

func (r *AutoGovernanceTraceRepository) Upsert(_ context.Context, item *governancemodel.AutoGovernanceTrace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" || strings.TrimSpace(item.ItemType) == "" || strings.TrimSpace(item.ItemID) == "" {
		return common.ErrInvalidArgument
	}
	r.items[item.ID] = cloneAutoGovernanceTrace(item)
	return nil
}

func (r *AutoGovernanceTraceRepository) Get(_ context.Context, id common.AutoGovernanceTraceID) (*governancemodel.AutoGovernanceTrace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneAutoGovernanceTrace(item), nil
}

func (r *AutoGovernanceTraceRepository) List(_ context.Context, query repository.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ret := make([]*governancemodel.AutoGovernanceTrace, 0, len(r.items))
	for _, item := range r.items {
		if !autoGovernanceTraceMatches(item, query) {
			continue
		}
		ret = append(ret, cloneAutoGovernanceTrace(item))
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
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func cloneAutoGovernanceTrace(item *governancemodel.AutoGovernanceTrace) *governancemodel.AutoGovernanceTrace {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
	return &cloned
}

func autoGovernanceTraceMatches(item *governancemodel.AutoGovernanceTrace, query repository.AutoGovernanceTraceQuery) bool {
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
