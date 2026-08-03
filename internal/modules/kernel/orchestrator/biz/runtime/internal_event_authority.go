package runtime

// SM-2 facts are observations used by authority/focus convergence. They never
// execute operations or project another owner's state.
const (
	RuntimeInternalEventReconcileAuthority                 RuntimeInternalEventKind = "reconcile_authority"
	RuntimeInternalEventReconciliationBatchObserved        RuntimeInternalEventKind = "reconciliation_batch_observed"
	RuntimeInternalEventSemanticContractRequired           RuntimeInternalEventKind = "semantic_contract_required"
	RuntimeInternalEventSemanticFeasibilityRequired        RuntimeInternalEventKind = "semantic_feasibility_required"
	RuntimeInternalEventSemanticFeasibilityObserved        RuntimeInternalEventKind = "semantic_feasibility_observed"
	RuntimeInternalEventSemanticContractSealed             RuntimeInternalEventKind = "semantic_contract_sealed"
	RuntimeInternalEventSemanticContractFailed             RuntimeInternalEventKind = "semantic_contract_failed"
	RuntimeInternalEventPrerequisiteObserved               RuntimeInternalEventKind = "prerequisite_observed"
	RuntimeInternalEventPrerequisitesReadOnlyStaleObserved RuntimeInternalEventKind = "prerequisites_read_only_stale_observed"
	RuntimeInternalEventFeasibilityObserved                RuntimeInternalEventKind = "feasibility_observed"
	RuntimeInternalEventFailureObserved                    RuntimeInternalEventKind = "failure_observed"
	RuntimeInternalEventOwnerFeasibilityObserved           RuntimeInternalEventKind = "owner_feasibility_observed"
	RuntimeInternalEventAssessmentObserved                 RuntimeInternalEventKind = "assessment_observed"
	RuntimeInternalEventGovernanceRequestObserved          RuntimeInternalEventKind = "governance_request_observed"
	RuntimeInternalEventValidationFailureObserved          RuntimeInternalEventKind = "validation_failure_observed"
	RuntimeInternalEventRepairSurfaceConfirmationObserved  RuntimeInternalEventKind = "repair_surface_confirmation_observed"
)
