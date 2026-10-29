package activationports

import (
	"context"
	"github.com/muidea/workorch/internal/pkg/runtimeprojection"
	"sync"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func selectedExplicitMemoryItems(ctx context.Context, opts MemorySelectionOptions) ([]*memorymodel.MemoryItem, *runtimeprojection.MemorySelectionEvidence, error) {
	if opts.WorkspaceID == "" || opts.Hub == nil || len(opts.IDs) > activationcore.RuntimeMemorySelectionCandidateLimit {
		return nil, nil, common.ErrInvalidArgument
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, opts.WorkspaceID)
	type memResult struct {
		item *memorymodel.MemoryItem
		err  error
	}
	results := make([]memResult, len(opts.IDs))
	var wg sync.WaitGroup
	for i, id := range opts.IDs {
		wg.Add(1)
		go func(idx int, mid string) {
			defer wg.Done()
			results[idx].item, results[idx].err = ownerreadports.ReadMemoryItemByOwnerCommand(ctx, opts.Hub, common.MemoryID(mid))
		}(i, id)
	}
	wg.Wait()
	ret := make([]*memorymodel.MemoryItem, 0, len(opts.IDs))
	for _, r := range results {
		if r.err != nil {
			return nil, nil, r.err
		}
		if r.item.WorkspaceID != opts.WorkspaceID {
			return nil, nil, common.ErrConflict
		}
		ret = append(ret, r.item)
	}
	return applyRuntimeMemorySelectionBudget(ret, &runtimeprojection.MemorySelectionEvidence{
		Policy: "explicit_memory_selection", RefreshSource: "explicit_ids",
	})
}

func selectedWorkspaceListMemoryItems(ctx context.Context, opts MemorySelectionOptions) ([]*memorymodel.MemoryItem, *runtimeprojection.MemorySelectionEvidence, error) {
	items, err := ownerreadports.ListMemoryItemsByWorkspaceOwnerCommand(ctx, opts.Hub, opts.WorkspaceID)
	if err != nil {
		return nil, nil, err
	}
	return applyRuntimeMemorySelectionBudget(items, &runtimeprojection.MemorySelectionEvidence{
		Policy: "deterministic_workspace_list", RefreshSource: "workspace_inventory",
	})
}
