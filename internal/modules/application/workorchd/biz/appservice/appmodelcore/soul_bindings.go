package appmodelcore

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func WorkspaceSoulBindingsFromModel(workspaceID common.WorkspaceID, defaultSoulID common.SoulID, bindings map[string]common.SoulID) workorchmanagementcontract.WorkspaceSoulBindings {
	return workorchmanagementcontract.WorkspaceSoulBindings{
		WorkspaceID:     string(workspaceID),
		DefaultSoulID:   string(defaultSoulID),
		Bindings:        MapSoulBindingIDs(bindings),
		PriorityPolicy:  SoulRoutingPriorityPolicy(),
		AvailableStages: SoulBindingStages(),
	}
}
