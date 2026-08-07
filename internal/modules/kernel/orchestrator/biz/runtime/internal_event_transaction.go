package runtime

// SM-3 facts advance the provider/candidate/sealed-dispatch transaction. They
// cannot select the next convergence action.
const (
	RuntimeInternalEventSemanticPlanRequestStarted           RuntimeInternalEventKind = "semantic_plan_request_started"
	RuntimeInternalEventSemanticPlanEvaluated                RuntimeInternalEventKind = "semantic_plan_evaluated"
	RuntimeInternalEventSemanticPlanCorrectionRequired       RuntimeInternalEventKind = "semantic_plan_correction_required"
	RuntimeInternalEventCandidateRequestEnsured              RuntimeInternalEventKind = "candidate_request_ensured"
	RuntimeInternalEventSemanticPlanBatchCompiled            RuntimeInternalEventKind = "semantic_plan_batch_compiled"
	RuntimeInternalEventSemanticPlanBatchAdvanced            RuntimeInternalEventKind = "semantic_plan_batch_advanced"
	RuntimeInternalEventSemanticPlanBatchFeasibilityObserved RuntimeInternalEventKind = "semantic_plan_batch_feasibility_observed"
	RuntimeInternalEventSemanticPlanBatchDocumentObserved    RuntimeInternalEventKind = "semantic_plan_batch_document_observed"
	RuntimeInternalEventSemanticPlanBatchDocumentsMerged     RuntimeInternalEventKind = "semantic_plan_batch_documents_merged"
	RuntimeInternalEventSemanticPlanBatchFeasibilitiesMerged RuntimeInternalEventKind = "semantic_plan_batch_feasibilities_merged"
	RuntimeInternalEventCandidateRequestSubmitted            RuntimeInternalEventKind = "candidate_request_submitted"
	RuntimeInternalEventCandidateRequestFragmentObserved     RuntimeInternalEventKind = "candidate_request_fragment_observed"
	RuntimeInternalEventCandidateRequestOutcomeObserved      RuntimeInternalEventKind = "candidate_request_outcome_observed"
	RuntimeInternalEventCandidateDocumentObserved            RuntimeInternalEventKind = "candidate_document_observed"
	RuntimeInternalEventCandidateRequestStaleObserved        RuntimeInternalEventKind = "candidate_request_stale_observed"
	RuntimeInternalEventCandidateRequestTerminated           RuntimeInternalEventKind = "candidate_request_terminated"
	RuntimeInternalEventCandidateAttemptBegun                RuntimeInternalEventKind = "candidate_attempt_begun"
	RuntimeInternalEventCandidateDocumentRejected            RuntimeInternalEventKind = "candidate_document_rejected"
	RuntimeInternalEventCandidateProposalObserved            RuntimeInternalEventKind = "candidate_proposal_observed"
	RuntimeInternalEventCandidateFeasibilityObserved         RuntimeInternalEventKind = "candidate_feasibility_observed"
	RuntimeInternalEventCandidatePrerequisiteStaleObserved   RuntimeInternalEventKind = "candidate_prerequisite_stale_observed"
	RuntimeInternalEventCandidatePrerequisiteObserved        RuntimeInternalEventKind = "candidate_prerequisite_observed"
	RuntimeInternalEventCandidateSealed                      RuntimeInternalEventKind = "candidate_sealed"
	RuntimeInternalEventGovernanceDispatchSealed             RuntimeInternalEventKind = "governance_dispatch_sealed"
	RuntimeInternalEventCandidateRejected                    RuntimeInternalEventKind = "candidate_rejected"
	RuntimeInternalEventGovernanceResolutionObserved         RuntimeInternalEventKind = "governance_resolution_observed"
	RuntimeInternalEventOperationReceiptObserved             RuntimeInternalEventKind = "operation_receipt_observed"
	RuntimeInternalEventDispatchExecutionRequested           RuntimeInternalEventKind = "dispatch_execution_requested"
)
