package biz

import (
	"context"
	"fmt"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// Inventory only: never enter activation's semantic recall or list fallback.
func (b *Biz) ObservePreparationMemoryInventory(ctx context.Context, id common.WorkspaceID) (memorymodel.PreparationInventory, error) {
	c := memoryevents.ObservePreparationInventoryCommand{WorkspaceID: id}
	if b == nil || b.EventHub() == nil || ctx == nil || c.Validate() != nil {
		return memorymodel.PreparationInventory{}, common.ErrInvalidArgument
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, id)
	if err != nil {
		return memorymodel.PreparationInventory{}, err
	}
	bounded, cancel := context.WithTimeout(bound, 2*time.Second)
	defer cancel()
	if err := bounded.Err(); err != nil {
		return memorymodel.PreparationInventory{}, err
	}
	started := time.Now().UTC()
	ev := magicevent.NewEventWithContext(memoryevents.TopicObservePreparationInventory, ModuleID, memoryevents.CommandDestination, magicevent.NewHeader(), bounded, c)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(memoryevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(id)}))
	r := b.SendEvent(ev)
	if r == nil {
		return memorymodel.PreparationInventory{}, common.ErrConflict
	}
	if err := r.Error(); err != nil {
		return memorymodel.PreparationInventory{}, err
	}
	// Keep the concrete *def.Error return; assigning its nil value to the
	// earlier error interface would box nil and falsely reject a valid read.
	data, getErr := r.Get()
	v, ok := data.(memoryevents.ObservePreparationInventoryResult)
	if getErr != nil || !ok {
		return memorymodel.PreparationInventory{}, fmt.Errorf("%w: invalid preparation inventory result type", common.ErrConflict)
	}
	if v.ValidateFor(c) != nil {
		return memorymodel.PreparationInventory{}, fmt.Errorf("%w: invalid preparation inventory value", common.ErrConflict)
	}
	if v.Inventory.ObservedAt.Before(started) || v.Inventory.ObservedAt.After(time.Now().UTC()) {
		return memorymodel.PreparationInventory{}, fmt.Errorf("%w: preparation inventory outside observation interval", common.ErrConflict)
	}
	if err := bounded.Err(); err != nil {
		return memorymodel.PreparationInventory{}, err
	}
	return v.Inventory.Clone(), nil
}
