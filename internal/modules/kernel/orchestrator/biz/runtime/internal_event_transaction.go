package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// RuntimeInternalTransactionPayload contains semantic-plan, provider-request,
// candidate, sealed-dispatch and receipt transaction facts. Whole-document
// request recovery remains an SM-4 decision, but it mutates this same persisted
// request transaction rather than a delivery ledger. This payload cannot carry
// activation or terminal-delivery state.
type RuntimeInternalTransactionPayload struct {
	SemanticManifest                   *runtimecontract.RuntimeSemanticPlanManifest
	SemanticDocument                   *runtimecontract.RuntimeSemanticPlanDocument
	SemanticPlanBatch                  *runtimecontract.RuntimeSemanticPlanBatch
	SemanticPlanBatchNextState         runtimecontract.RuntimeSemanticPlanBatchState
	SemanticPlanBatchFeasibilityResult *runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult
	SemanticPlanBatchDocument          *runtimecontract.RuntimeSemanticPlanDocument
	SemanticPlanMergedDocument         *runtimecontract.RuntimeSemanticPlanDocument
	SemanticDiagnostic                 *runtimecontract.RuntimeOriginalDiagnostic

	StablePrefixHash string
	DocumentKind     runtimecontract.RuntimeProviderDocumentKind
	ManifestHash     string
	CorrectionHash   string
	LogicalRequestID string
	Sequence         int
	PhysicalCallID   string
	// ProviderRequestTerminal is the single owner-private coordination fact
	// that atomically closes the SM-3 provider request and applies its already
	// normalized SM-2 successor failure. It never crosses EventHub.
	ProviderRequestTerminal *RuntimeProviderRequestTerminalFact
	// ProviderRequestRecovery is the single owner-private coordination fact
	// that atomically settles the physical call, advances SM-3 recovery,
	// consumes the SM-4 transport guard when applicable, and applies an
	// exhausted SM-2 successor. It never crosses EventHub.
	ProviderRequestRecovery *RuntimeProviderRequestRecoveryFact
	// ProviderDocumentFraming is present on CandidateDocumentObserved only for
	// an empty/invalid physical response. The LLM owner supplied this bounded
	// fact; the Orchestrator-private transition settles it without carrying the
	// raw provider body across another boundary.
	ProviderDocumentFraming *runtimecontract.RuntimeProviderDocumentFraming
	// ProviderBudgetSettlement is the exact Orchestrator-private settlement
	// derived from the LLM audit fact. It is reduced in the same aggregate CAS as
	// the request outcome and never crosses EventHub.
	ProviderBudgetSettlement *RuntimeProviderBudgetSettlement
	ExpectedDocument         *runtimecontract.RuntimeExpectedDocumentContract

	Attempt              int
	Manifest             *runtimecontract.RuntimeCandidateContractManifest
	Candidate            *runtimecontract.RuntimeCandidate
	CandidateFailure     *runtimecontract.RuntimeCandidateFailure
	CandidateFeasibility *runtimecontract.RuntimeCandidateFeasibilityRecord
	PrerequisiteRequest  *runtimecontract.RuntimeOperationPrerequisiteRequest
	Stale                *runtimecontract.RuntimePrerequisiteStale

	OperationReceipt *runtimecontract.RuntimeOperationReceipt
	// OperationReceiptOwnerSource is the exact EventHub producer identity
	// observed by the Orchestrator adapter. It is admitted only with
	// OperationReceiptObserved and is checked against the sealed operation
	// descriptor in the same aggregate load/CAS transaction as the receipt.
	OperationReceiptOwnerSource string
	DispatchID                  string
	Fence                       uint64
	Executor                    RuntimeOperationExecutor
	// Recovery authorization is produced only after the independent SM-4
	// RecoveryBudgetConsumed event commits.  The attempt is an exact persisted
	// counter fence, not a request to mutate the budget from the SM-3 handler.
	RecoveryOperationID   string
	RecoveryBudgetAttempt int
	GovernanceResolution  *runtimecontract.RuntimeGovernanceResolution
	// GovernanceContinuationDriveLease is an SM-4 successor template sealed in
	// the same owner-private CAS as a successful governance continuation. The
	// reducer owns revision/generation/state; the caller supplies only the exact
	// scope/signature/due time. It never crosses EventHub.
	GovernanceContinuationDriveLease *runtimecontract.RuntimeDriveLease
	DefinitionClarificationRequest   *runtimecontract.RuntimeDefinitionClarificationRequestRecord
}

// SM-3 facts advance the provider/candidate/sealed-dispatch transaction. They
// cannot select the next convergence action.
const (
	RuntimeInternalEventSemanticPlanRequestStarted             RuntimeInternalEventKind = "semantic_plan_request_started"
	RuntimeInternalEventSemanticPlanEvaluated                  RuntimeInternalEventKind = "semantic_plan_evaluated"
	RuntimeInternalEventSemanticPlanCorrectionRequired         RuntimeInternalEventKind = "semantic_plan_correction_required"
	RuntimeInternalEventCandidateRequestEnsured                RuntimeInternalEventKind = "candidate_request_ensured"
	RuntimeInternalEventSemanticPlanBatchCompiled              RuntimeInternalEventKind = "semantic_plan_batch_compiled"
	RuntimeInternalEventSemanticPlanBatchAdvanced              RuntimeInternalEventKind = "semantic_plan_batch_advanced"
	RuntimeInternalEventSemanticPlanBatchFeasibilityObserved   RuntimeInternalEventKind = "semantic_plan_batch_feasibility_observed"
	RuntimeInternalEventSemanticPlanBatchDocumentObserved      RuntimeInternalEventKind = "semantic_plan_batch_document_observed"
	RuntimeInternalEventSemanticPlanBatchDocumentsMerged       RuntimeInternalEventKind = "semantic_plan_batch_documents_merged"
	RuntimeInternalEventCandidateRequestSubmitted              RuntimeInternalEventKind = "candidate_request_submitted"
	RuntimeInternalEventProviderRequestRecoveryCoordinated     RuntimeInternalEventKind = "provider_request_recovery_coordinated"
	RuntimeInternalEventCandidateDocumentObserved              RuntimeInternalEventKind = "candidate_document_observed"
	RuntimeInternalEventCandidateRequestStaleObserved          RuntimeInternalEventKind = "candidate_request_stale_observed"
	RuntimeInternalEventProviderRequestTerminalCoordinated     RuntimeInternalEventKind = "provider_request_terminal_coordinated"
	RuntimeInternalEventCandidateAttemptBegun                  RuntimeInternalEventKind = "candidate_attempt_begun"
	RuntimeInternalEventCandidateDocumentRejected              RuntimeInternalEventKind = "candidate_document_rejected"
	RuntimeInternalEventCandidateProposalObserved              RuntimeInternalEventKind = "candidate_proposal_observed"
	RuntimeInternalEventCandidateFeasibilityObserved           RuntimeInternalEventKind = "candidate_feasibility_observed"
	RuntimeInternalEventCandidatePrerequisiteStaleObserved     RuntimeInternalEventKind = "candidate_prerequisite_stale_observed"
	RuntimeInternalEventCandidatePrerequisiteObserved          RuntimeInternalEventKind = "candidate_prerequisite_observed"
	RuntimeInternalEventCandidateSealed                        RuntimeInternalEventKind = "candidate_sealed"
	RuntimeInternalEventGovernanceDispatchSealed               RuntimeInternalEventKind = "governance_dispatch_sealed"
	RuntimeInternalEventCandidateRejected                      RuntimeInternalEventKind = "candidate_rejected"
	RuntimeInternalEventGovernanceResolutionObserved           RuntimeInternalEventKind = "governance_resolution_observed"
	RuntimeInternalEventOperationReceiptObserved               RuntimeInternalEventKind = "operation_receipt_observed"
	RuntimeInternalEventDispatchExecutionRequested             RuntimeInternalEventKind = "dispatch_execution_requested"
	RuntimeInternalEventDefinitionClarificationDispatchSealed  RuntimeInternalEventKind = "definition_clarification_dispatch_sealed"
	RuntimeInternalEventDefinitionClarificationRequestObserved RuntimeInternalEventKind = "definition_clarification_request_observed"
)
