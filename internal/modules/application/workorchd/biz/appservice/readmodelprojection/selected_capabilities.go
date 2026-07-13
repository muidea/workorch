package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func SelectedRuntimeCapabilitiesFromRun(run *runmodel.Run, catalog map[string]readmodelcore.RuntimeCapability) []readmodelcore.RuntimeCapability {
	if run == nil || run.RuntimeBootstrap == nil {
		return nil
	}
	activation := run.RuntimeBootstrap.Activation.Normalized()
	ret := make([]readmodelcore.RuntimeCapability, 0, len(activation.MCPCapabilities))
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
			item = readmodelcore.RuntimeCapability{
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
