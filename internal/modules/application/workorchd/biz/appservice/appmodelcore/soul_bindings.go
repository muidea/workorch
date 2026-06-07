package appmodelcore

import "github.com/muidea/workorch/internal/pkg/common"

type WorkspaceSoulBindings struct {
	WorkspaceID     string            `json:"workspaceId,omitempty"`
	DefaultSoulID   string            `json:"defaultSoulId,omitempty"`
	Bindings        map[string]string `json:"bindings,omitempty"`
	PriorityPolicy  []string          `json:"priorityPolicy,omitempty"`
	AvailableStages []string          `json:"availableStages,omitempty"`
}

func WorkspaceSoulBindingsFromModel(workspaceID common.WorkspaceID, defaultSoulID common.SoulID, bindings map[string]common.SoulID) WorkspaceSoulBindings {
	return WorkspaceSoulBindings{
		WorkspaceID:     string(workspaceID),
		DefaultSoulID:   string(defaultSoulID),
		Bindings:        MapSoulBindingIDs(bindings),
		PriorityPolicy:  SoulRoutingPriorityPolicy(),
		AvailableStages: SoulBindingStages(),
	}
}
