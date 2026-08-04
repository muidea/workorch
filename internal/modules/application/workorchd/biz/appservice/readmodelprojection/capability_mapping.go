package readmodelprojection

import (
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func RuntimeCapabilityFromModel(item *mcpmodel.Capability) projectioncontract.RuntimeCapability {
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
		Status:         item.Status,
	}
}
