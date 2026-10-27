package workspace

const PreparationMCPSelectionEvaluationVersion = "workorch-preparation-mcp-selection-evaluation/v1"

// BlockedReason consumes a historical compilation, not connection evidence.
// Empty selection has no implicit exemption. Unknown versions/codes cannot
// produce a public classification or preserve a caller-authored verdict.
func (c PreparationMCPSelectionCompilation) BlockedReason() string {
	if c.Version != PreparationMCPSelectionVersion {
		return ""
	}
	if c.FailureCode == "" {
		if len(c.SelectedServerIDs) == 0 && len(c.SelectedCapabilityIDs) == 0 {
			return "mcp_selection_empty_policy_pending"
		}
		return "mcp_selection_compiled_connection_pending"
	}
	switch c.FailureCode {
	case "source_binding_conflict", "configuration_source_unavailable", "configuration_declaration_invalid",
		"selection_limit", "skill_source_incomplete", "skill_requirement_invalid", "server_disabled",
		"server_missing", "capability_disabled", "capability_server_not_selected", "capability_not_allowed", "capability_missing":
		return "mcp_selection_" + c.FailureCode
	}
	return ""
}

// Finite, non-sensitive wire codes only; never accept an arbitrary prefix.
func IsPreparationMCPSelectionReason(reason string) bool {
	switch reason {
	case "mcp_selection_empty_policy_pending", "mcp_selection_compiled_connection_pending",
		"mcp_selection_source_binding_conflict", "mcp_selection_configuration_source_unavailable",
		"mcp_selection_configuration_declaration_invalid", "mcp_selection_selection_limit",
		"mcp_selection_skill_source_incomplete", "mcp_selection_skill_requirement_invalid",
		"mcp_selection_server_disabled", "mcp_selection_server_missing",
		"mcp_selection_capability_disabled", "mcp_selection_capability_server_not_selected",
		"mcp_selection_capability_not_allowed", "mcp_selection_capability_missing":
		return true
	}
	return false
}

func PreparationMCPReasonRequiresEvidence(reason string) bool {
	return reason == "mcp_registry_observed_connection_pending" || IsPreparationMCPSelectionReason(reason)
}

func (e WorkspacePreparationMCPRegistryEvidence) BlockedReason() string {
	if e.SelectionEvaluationVersion == "" {
		return "mcp_registry_observed_connection_pending"
	}
	if e.SelectionEvaluationVersion == PreparationMCPSelectionEvaluationVersion && e.Selection != nil {
		return e.Selection.BlockedReason()
	}
	return ""
}
