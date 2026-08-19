package readmodelprojection

import (
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: capability_catalog.go

func runtimeCapabilityCatalogFromItems(items []*mcpmodel.Capability) map[string]projectioncontract.RuntimeCapability {
	if items == nil {
		return nil
	}
	ret := make(map[string]projectioncontract.RuntimeCapability, len(items))
	for _, item := range items {
		capability := runtimeCapabilityFromModel(item)
		if capability.ID == "" {
			continue
		}
		ret[capability.ID] = capability
	}
	return ret
}

// source: capability_mapping.go

func runtimeCapabilityFromModel(item *mcpmodel.Capability) projectioncontract.RuntimeCapability {
	if item == nil {
		return projectioncontract.RuntimeCapability{}
	}
	return projectioncontract.RuntimeCapability{
		ID:             string(item.ID),
		ServerID:       string(item.ServerID),
		Kind:           item.Kind,
		Name:           item.Name,
		DisplayName:    firstNonEmptyTimelineString(item.DisplayName, item.Name, string(item.ID)),
		Description:    item.Description,
		Category:       item.Category,
		Tags:           cloneReadModelStringSlice(item.Tags),
		RiskLevel:      item.RiskLevel,
		Enabled:        item.Enabled,
		ApprovalPolicy: item.ApprovalPolicy,
		Status:         availabilityStatus(item),
	}
}

func availabilityStatus(item *mcpmodel.Capability) string {
	if item != nil && item.Enabled {
		return "enabled"
	}
	return "disabled"
}
