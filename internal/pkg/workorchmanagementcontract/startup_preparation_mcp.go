package workorchmanagementcontract

// Public shape validation only. Workspace independently derives these codes
// from complete private archives; the wire cannot establish owner readiness.
func isPreparationMCPSelectionReason(reason string) bool {
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

func preparationMCPReasonRequiresEvidence(reason string) bool {
	return reason == "mcp_registry_observed_connection_pending" || isPreparationMCPSelectionReason(reason)
}
