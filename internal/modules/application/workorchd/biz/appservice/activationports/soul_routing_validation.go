package activationports

import (
	"context"
	"sort"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func ValidateSoulRoutingCapabilitySelection(ctx context.Context, hub magicevent.Hub, routing appmodelcore.SoulRouting, skillIDs []string, mcpServerIDs []string, capabilityIDs []string, capabilityCategories []string, toolKinds []string) error {
	if routing.Status != "resolved" {
		return nil
	}
	execution := routing.Bindings[appmodelcore.SoulBindingStageExecution]
	soulID := common.SoulID(strings.TrimSpace(execution.SoulID))
	if soulID == "" {
		soulID = common.SoulID(strings.TrimSpace(routing.EffectiveDefaultSoulID))
	}
	return contextregistryports.ValidateSoulCapabilitySelection(ctx, hub, soulID, skillIDs, mcpServerIDs, capabilityIDs, capabilityCategories, toolKinds)
}

func workspaceExecutionSoul(defaultSoul *soulmodel.Soul, stageSouls map[string]*soulmodel.Soul) *soulmodel.Soul {
	if value := stageSouls[appmodelcore.SoulBindingStageExecution]; value != nil {
		return value
	}
	return defaultSoul
}

func validateSelectedMCPCapabilityOwnership(selection WorkspaceMCPSelectionResult) error {
	selectedServers := make(map[common.MCPServerID]struct{}, len(selection.MCPServerIDs))
	for _, raw := range selection.MCPServerIDs {
		id := common.MCPServerID(strings.TrimSpace(raw))
		if id == "" {
			return common.ErrInvalidArgument
		}
		selectedServers[id] = struct{}{}
	}
	expectedCapabilities := make(map[common.MCPCapabilityID]struct{}, len(selection.CapabilityIDs))
	for _, raw := range selection.CapabilityIDs {
		id := common.MCPCapabilityID(strings.TrimSpace(raw))
		if id == "" {
			return common.ErrInvalidArgument
		}
		expectedCapabilities[id] = struct{}{}
	}
	if len(selection.Capabilities) != len(expectedCapabilities) {
		return common.ErrConflict
	}
	seen := make(map[common.MCPCapabilityID]struct{}, len(selection.Capabilities))
	for _, capability := range selection.Capabilities {
		if capability == nil || capability.ID == "" || capability.ServerID == "" || strings.TrimSpace(capability.Category) == "" {
			return common.ErrConflict
		}
		if _, ok := expectedCapabilities[capability.ID]; !ok {
			return common.ErrConflict
		}
		if _, duplicate := seen[capability.ID]; duplicate {
			return common.ErrConflict
		}
		seen[capability.ID] = struct{}{}
		if _, ok := selectedServers[capability.ServerID]; !ok {
			return common.ErrConflict
		}
	}
	return nil
}

func runtimeMCPCapabilityToolKinds(items []*mcpmodel.Capability) []string {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		kind := runtimecontract.SealMCPCapabilityEffect(runtimecontract.RuntimeMCPCapabilityContext{Kind: item.Kind}).OperationKind
		if kind != "" {
			set[kind] = struct{}{}
		}
	}
	ret := make([]string, 0, len(set))
	for value := range set {
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
}
