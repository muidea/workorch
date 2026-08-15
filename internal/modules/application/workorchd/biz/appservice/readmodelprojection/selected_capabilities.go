package readmodelprojection

import (
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func selectedRuntimeCapabilitiesFromRun(run *runmodel.Run, catalog map[string]projectioncontract.RuntimeCapability) []projectioncontract.RuntimeCapability {
	if run == nil || run.RuntimeBootstrap == nil {
		return nil
	}
	activation := run.RuntimeBootstrap.Activation.Normalized()
	ret := make([]projectioncontract.RuntimeCapability, 0, len(activation.MCPCapabilities))
	seen := map[string]struct{}{}
	for _, value := range activation.MCPCapabilities {
		id := string(value.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		item, ok := catalog[id]
		if !ok {
			item = projectioncontract.RuntimeCapability{
				ID: id, ServerID: string(value.ServerID), Kind: value.Kind, Name: value.Name,
				DisplayName: firstNonEmptyTimelineString(value.DisplayName, value.Name, id), Description: value.Description,
				Category: value.Category, Tags: cloneReadModelStringSlice(value.Tags), RiskLevel: value.RiskLevel,
				Enabled: value.Enabled, ApprovalPolicy: value.ApprovalPolicy, Status: value.Status,
			}
		}
		ret = append(ret, item)
		seen[id] = struct{}{}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
