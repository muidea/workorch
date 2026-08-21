package runtime

import (
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RuntimeInternalAuthorityPayload contains only SM-2 authority/focus facts.
// It cannot carry an executor, request transport state or delivery ledger.
type RuntimeInternalAuthorityPayload struct {
	ReconciliationBatch *runtimecontract.RuntimeCWIReconciliationBatch
	DefinitionGap       *runtimecontract.RuntimeDefinitionGapObservation

	SemanticFeasibility *runtimecontract.RuntimeSemanticProofFeasibility
	SemanticDiagnostic  *runtimecontract.RuntimeOriginalDiagnostic
	// SemanticFailureCode is an Orchestrator-selected bounded terminal code.
	// It remains separate from SemanticDiagnostic so the producer-authored
	// diagnostic can be persisted byte-for-byte.
	SemanticFailureCode       string
	SemanticBatchID           string
	SemanticRefreshGeneration int
	ManifestHash              string
	LogicalRequestID          string
	CWIBindingRetirement      *RuntimeCWIBindingRetirementFact
	SemanticCatalogScopeGap   *RuntimeSemanticCatalogScopeGap
	SemanticCoverageOrder     []common.CoverageWorkItemID
	SemanticOrderManifestHash string

	PrerequisiteResult              *PrerequisiteResult
	RecoverableWaitProbeObservation *runtimecontract.RuntimeRecoverableWaitProbeObservation
	Stale                           *runtimecontract.RuntimePrerequisiteStale
	FeasibilityFact                 *runtimecontract.RuntimeFeasibilityFact
	FailureFact                     *runtimecontract.FailureFact
	CloseActiveDispatch             bool

	Attempt                   int
	OwnerFeasibility          *runtimecontract.RuntimeOwnerFeasibilityObservation
	GovernanceRequest         *runtimecontract.RuntimeGovernanceRequestRecord
	RepairSurfaceConfirmation *runtimecontract.RuntimeRepairSurfaceConfirmation
}

// SM-2 facts are observations used by authority/focus convergence. They never
// execute operations or project another owner's state.
const (
	RuntimeInternalEventReconcileAuthority          RuntimeInternalEventKind = "reconcile_authority"
	RuntimeInternalEventReconciliationBatchObserved RuntimeInternalEventKind = "reconciliation_batch_observed"
	RuntimeInternalEventSemanticContractRequired    RuntimeInternalEventKind = "semantic_contract_required"
	// SemanticCatalogInvalidated is an Orchestrator-private SM-2 fact
	// transition. It retires a manifest whose owner observation is no longer
	// current; it never carries a destination state or provider repair choice.
	RuntimeInternalEventSemanticCatalogInvalidated           RuntimeInternalEventKind = "semantic_catalog_invalidated"
	RuntimeInternalEventSemanticCatalogScopeInvalidated      RuntimeInternalEventKind = "semantic_catalog_scope_invalidated"
	RuntimeInternalEventCWIBindingRetired                    RuntimeInternalEventKind = "cwi_binding_retired"
	RuntimeInternalEventEligibleFocusProjected               RuntimeInternalEventKind = "eligible_focus_projected"
	RuntimeInternalEventSemanticFeasibilityRequired          RuntimeInternalEventKind = "semantic_feasibility_required"
	RuntimeInternalEventSemanticFeasibilityObserved          RuntimeInternalEventKind = "semantic_feasibility_observed"
	RuntimeInternalEventSemanticContractSealed               RuntimeInternalEventKind = "semantic_contract_sealed"
	RuntimeInternalEventSemanticContractFailed               RuntimeInternalEventKind = "semantic_contract_failed"
	RuntimeInternalEventPrerequisiteObserved                 RuntimeInternalEventKind = "prerequisite_observed"
	RuntimeInternalEventPrerequisitesReadOnlyStaleObserved   RuntimeInternalEventKind = "prerequisites_read_only_stale_observed"
	RuntimeInternalEventFeasibilityObserved                  RuntimeInternalEventKind = "feasibility_observed"
	RuntimeInternalEventFailureObserved                      RuntimeInternalEventKind = "failure_observed"
	RuntimeInternalEventOwnerFeasibilityObserved             RuntimeInternalEventKind = "owner_feasibility_observed"
	RuntimeInternalEventSemanticBindingContradictionObserved RuntimeInternalEventKind = "semantic_binding_contradiction_observed"
	RuntimeInternalEventAssessmentObserved                   RuntimeInternalEventKind = "assessment_observed"
	RuntimeInternalEventGovernanceRequestObserved            RuntimeInternalEventKind = "governance_request_observed"
	RuntimeInternalEventValidationFailureObserved            RuntimeInternalEventKind = "validation_failure_observed"
	RuntimeInternalEventRepairSurfaceConfirmationObserved    RuntimeInternalEventKind = "repair_surface_confirmation_observed"
	RuntimeInternalEventDefinitionGapObserved                RuntimeInternalEventKind = "runtime_definition_gap_observed"
)
