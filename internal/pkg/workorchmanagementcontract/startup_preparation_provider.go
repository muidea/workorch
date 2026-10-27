package workorchmanagementcontract

// Public shape only, not an owner-issued permission or readiness verdict.
func isPreparationProviderSelectionReason(reason string) bool {
	switch reason {
	case "provider_selection_budget_observed_authorization_pending", "provider_selection_budget_unavailable", "provider_selection_budget_source_conflict", "provider_selection_authorization_pending", "provider_selection_source_binding_conflict", "provider_selection_configuration_source_unavailable", "provider_selection_invalid", "provider_selection_model_missing", "provider_selection_capability_incomplete", "provider_selection_operation_unsupported", "provider_selection_reasoning_unsupported":
		return true
	}
	return false
}
