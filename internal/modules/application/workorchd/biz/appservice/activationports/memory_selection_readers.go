package activationports

import (
	"context"
	"sync"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func selectedExplicitMemoryItems(ctx context.Context, opts MemorySelectionOptions) ([]*memorymodel.MemoryItem, *MemorySelectionEvidence, error) {
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
		if activationcore.MemoryVisibleToLLM(r.item) {
			ret = append(ret, r.item)
		}
	}
	status := "selected"
	if len(ret) == 0 && len(results) > 0 {
		status = "none_selected"
	}
	return ret, &MemorySelectionEvidence{
		Policy:             "explicit_memory_selection",
		Status:             status,
		RetrievedMemoryIDs: activationcore.MemoryItemIDs(ret),
	}, nil
}

func selectedWorkspaceListMemoryItems(ctx context.Context, opts MemorySelectionOptions) ([]*memorymodel.MemoryItem, *MemorySelectionEvidence, error) {
	items, err := ownerreadports.ListMemoryItemsByWorkspaceOwnerCommand(ctx, opts.Hub, opts.WorkspaceID)
	if err != nil {
		return nil, nil, err
	}
	selected := activationcore.VisibleMemoryItems(items, 8)
	status := "selected"
	if len(selected) == 0 && len(items) > 0 {
		status = "none_selected"
	}
	return selected, &MemorySelectionEvidence{
		Policy:             "deterministic_workspace_list",
		Status:             status,
		CandidateMemoryIDs: activationcore.MemoryItemIDs(items),
		ExcludedMemoryIDs:  activationcore.MemoryItemIDs(activationcore.ExcludedMemoryItems(items, selected)),
		RetrievedMemoryIDs: activationcore.MemoryItemIDs(selected),
	}, nil
}
