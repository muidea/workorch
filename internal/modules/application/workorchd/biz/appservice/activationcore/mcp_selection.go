package activationcore

import (
	"slices"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type MCPSelectionContext struct {
	RequestedMCPServerIDs  []string
	DefaultMCPServerIDs    []string
	RequiredMCPServerIDs   []string
	AutoBoundMCPServerIDs  []string
	SelectedMCPServerIDs   []string
	RequestedCapabilityIDs []string
	DefaultCapabilityIDs   []string
	RequiredCapabilityIDs  []string
	AutoBoundCapabilityIDs []string
	SelectedCapabilityIDs  []string
}

func WorkspaceMCPSelectionContext(requestedMCPServerIDs []string, defaultMCPServerIDs []string, requestedCapabilityIDs []string, defaultCapabilityIDs []string, skills []workorchmanagementcontract.Skill) MCPSelectionContext {
	ret := MCPSelectionContext{
		RequestedMCPServerIDs:  NormalizeStringIDs(requestedMCPServerIDs),
		DefaultMCPServerIDs:    contextregistrycore.CloneStrings(defaultMCPServerIDs),
		RequiredMCPServerIDs:   RequiredMCPServerRefs(skills),
		RequestedCapabilityIDs: NormalizeStringIDs(requestedCapabilityIDs),
		DefaultCapabilityIDs:   contextregistrycore.CloneStrings(defaultCapabilityIDs),
		RequiredCapabilityIDs:  RequiredCapabilityRefs(skills),
	}
	ret.SelectedMCPServerIDs = AppendDefaultIDs(ret.RequestedMCPServerIDs, ret.DefaultMCPServerIDs)
	for _, id := range ret.RequiredMCPServerIDs {
		if !slices.Contains(ret.SelectedMCPServerIDs, id) {
			ret.SelectedMCPServerIDs = append(ret.SelectedMCPServerIDs, id)
			ret.AutoBoundMCPServerIDs = append(ret.AutoBoundMCPServerIDs, id)
		}
	}
	ret.SelectedCapabilityIDs = AppendDefaultIDs(ret.RequestedCapabilityIDs, ret.DefaultCapabilityIDs)
	for _, id := range ret.RequiredCapabilityIDs {
		if !slices.Contains(ret.SelectedCapabilityIDs, id) {
			ret.SelectedCapabilityIDs = append(ret.SelectedCapabilityIDs, id)
			ret.AutoBoundCapabilityIDs = append(ret.AutoBoundCapabilityIDs, id)
		}
	}
	return ret
}

func (c MCPSelectionContext) Context() map[string]any {
	return map[string]any{
		"policy":                 "template_default_and_skill_required_mcp_auto_bind",
		"requestedMCPServerIds":  c.RequestedMCPServerIDs,
		"defaultMCPServerIds":    c.DefaultMCPServerIDs,
		"requiredMCPServerIds":   c.RequiredMCPServerIDs,
		"autoBoundMCPServerIds":  c.AutoBoundMCPServerIDs,
		"selectedMCPServerIds":   c.SelectedMCPServerIDs,
		"requestedCapabilityIds": c.RequestedCapabilityIDs,
		"defaultCapabilityIds":   c.DefaultCapabilityIDs,
		"requiredCapabilityIds":  c.RequiredCapabilityIDs,
		"autoBoundCapabilityIds": c.AutoBoundCapabilityIDs,
		"selectedCapabilityIds":  c.SelectedCapabilityIDs,
	}
}
