package events

import (
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const TopicObservePreparationInventory = "/memory/startup-preparation/inventory/query"

type ObservePreparationInventoryCommand struct{ WorkspaceID common.WorkspaceID }

func (c ObservePreparationInventoryCommand) Validate() error {
	if !memorymodel.ValidPreparationWorkspaceID(c.WorkspaceID) {
		return common.ErrInvalidArgument
	}
	return nil
}

type ObservePreparationInventoryResult struct {
	Inventory memorymodel.PreparationInventory
}

func (r ObservePreparationInventoryResult) ValidateFor(c ObservePreparationInventoryCommand) error {
	if c.Validate() != nil {
		return common.ErrConflict
	}
	return r.Inventory.ValidateFor(c.WorkspaceID)
}
