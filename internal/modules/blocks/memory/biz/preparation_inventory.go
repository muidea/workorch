package memory

import (
	"context"
	"slices"
	"sort"
	"time"

	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Called under the existing Workspace mutation lock and active-deletion fence.
// A single repository read, never paginated or filtered into a false empty set.
func (m *Biz) observePreparationInventory(ctx context.Context, c memoryevents.ObservePreparationInventoryCommand) (memoryevents.ObservePreparationInventoryResult, error) {
	if c.Validate() != nil {
		return memoryevents.ObservePreparationInventoryResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return memoryevents.ObservePreparationInventoryResult{}, err
	}
	if _, err := common.BindRuntimeWorkspaceContext(ctx, c.WorkspaceID); err != nil {
		return memoryevents.ObservePreparationInventoryResult{}, err
	}
	items, err := m.service.items.ListByWorkspaceID(ctx, c.WorkspaceID)
	if err != nil {
		return memoryevents.ObservePreparationInventoryResult{}, err
	}
	if len(items) > memorymodel.PreparationInventoryMaxItems {
		return memoryevents.ObservePreparationInventoryResult{}, common.ErrConflict
	}
	// Reject invalid scope/nil before sorting. Never silently drop corrupt rows.
	for _, item := range items {
		if item == nil || item.WorkspaceID != c.WorkspaceID {
			return memoryevents.ObservePreparationInventoryResult{}, common.ErrConflict
		}
	}
	v := memorymodel.PreparationInventory{Version: memorymodel.PreparationInventoryVersion, WorkspaceID: c.WorkspaceID, OwnerEpoch: m.preparationEpoch, ObservedAt: time.Now().UTC(), Items: slices.Clone(items)}
	if v.Items == nil {
		v.Items = []*memorymodel.MemoryItem{}
	}
	sort.Slice(v.Items, func(i, j int) bool { return v.Items[i].ID < v.Items[j].ID })
	if err := v.ValidateFor(c.WorkspaceID); err != nil {
		return memoryevents.ObservePreparationInventoryResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return memoryevents.ObservePreparationInventoryResult{}, err
	}
	return memoryevents.ObservePreparationInventoryResult{Inventory: v.Clone()}, nil
}
