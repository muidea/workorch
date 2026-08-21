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

	StablePrefixHash          string
	DocumentKind              runtimecontract.RuntimeProviderDocumentKind
	ManifestHash              string
	CorrectionHash            string
	LogicalRequestID          string
	Sequence                  int
	PhysicalCallID            string
	RequestOutcome            *runtimecontract.RuntimeCandidateRequestOutcome
	ProviderDocumentOutcome   *runtimecontract.RuntimeProviderDocumentOutcome
	ProviderBudgetObservation *runtimecontract.RuntimeProviderBudgetObservation
	ExpectedDocument          *runtimecontract.RuntimeExpectedDocumentContract

	Attempt              int
	Manifest             *runtimecontract.RuntimeCandidateContractManifest
	Candidate            *runtimecontract.RuntimeCandidate
	CandidateFailure     *runtimecontract.RuntimeCandidateFailure
	CandidateFeasibility *runtimecontract.RuntimeCandidateFeasibilityRecord
	PrerequisiteRequest  *runtimecontract.RuntimeOperationPrerequisiteRequest
	Stale                *runtimecontract.RuntimePrerequisiteStale

	OperationReceipt *runtimecontract.RuntimeOperationReceipt
	DispatchID       string
	Fence            uint64
	Executor         RuntimeOperationExecutor
	// Recovery authorization is produced only after the independent SM-4
	// RecoveryBudgetConsumed event commits.  The attempt is an exact persisted
	// counter fence, not a request to mutate the budget from the SM-3 handler.
	RecoveryOperationID            string
	RecoveryBudgetAttempt          int
	GovernanceResolution           *runtimecontract.RuntimeGovernanceResolution
	DefinitionClarificationRequest *runtimecontract.RuntimeDefinitionClarificationRequestRecord
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
	RuntimeInternalEventCandidateRequestOutcomeObserved        RuntimeInternalEventKind = "candidate_request_outcome_observed"
	RuntimeInternalEventCandidateRequestRecoveryRequired       RuntimeInternalEventKind = "candidate_request_recovery_required"
	RuntimeInternalEventCandidateDocumentObserved              RuntimeInternalEventKind = "candidate_document_observed"
	RuntimeInternalEventCandidateRequestStaleObserved          RuntimeInternalEventKind = "candidate_request_stale_observed"
	RuntimeInternalEventCandidateRequestTerminated             RuntimeInternalEventKind = "candidate_request_terminated"
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
