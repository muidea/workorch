package preparationcompiler

import (
	"github.com/muidea/workorch/internal/pkg/runtimeprofiledecode"
	"strings"
)

// This projection matches activationcore.WorkspaceExecutionContractForTemplate.
// Merge detaches the inputs; filtering preserves the activation defaults.
func executionContractHints(workspace, template map[string]any) map[string]any {
	hints := runtimeprofiledecode.MergeTemplateWorkspaceContextHintsForExecutionContract(template, workspace)
	out := map[string]any{"stateChangePolicy": "allowed_but_not_required"}
	if value, ok := hints["stateChangePolicy"].(string); ok && strings.TrimSpace(value) != "" {
		out["stateChangePolicy"] = strings.TrimSpace(value)
	}
	for _, key := range []string{"profileContracts", "runtimeRepair", "acceptanceSlicePolicy", "templateStagePolicies"} {
		if value, ok := hints[key].(map[string]any); ok && len(value) > 0 {
			out[key] = value
		}
	}
	for _, key := range []string{"capabilityUsagePolicies", "validationPolicies"} {
		if value, ok := hints[key].([]any); ok && len(value) > 0 {
			out[key] = value
		}
	}
	return out
}
