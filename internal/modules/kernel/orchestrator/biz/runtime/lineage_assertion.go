package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

func runtimeLineageAssertionSources(aggregate RuntimeAggregate) runtimecontract.RuntimeLineageAssertionSources {
	var active *runtimecontract.RuntimeDispatchRecord
	if aggregate.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*aggregate.ActiveDispatch)
		active = &value
	}
	return runtimeLineageAssertionSourcesWithDispatches(aggregate, active, sealedDispatchesToRuntimeRecords(aggregate.CompletedDispatches))
}

func runtimeLineageAssertionSourcesWithDispatches(aggregate RuntimeAggregate, active *runtimecontract.RuntimeDispatchRecord, completed []runtimecontract.RuntimeDispatchRecord) runtimecontract.RuntimeLineageAssertionSources {
	return runtimecontract.RuntimeLineageAssertionSources{
		Definition: aggregate.Definition, Facts: aggregate.Facts, Work: aggregate.ImplementationWork,
		Semantic: aggregate.SemanticPlanning, Active: active,
		Completed: completed,
		Receipts:  aggregate.OperationReceipts, Governance: aggregate.OperationGovernanceBindings,
		Budget: aggregate.ExecutionBudget, EvidenceSequence: aggregate.EvidenceSequence,
		Resolutions: aggregate.GovernanceResolutions, Attempts: aggregate.CandidateAttempts,
		Sessions: aggregate.CandidateRequestSessions, ProcessedEventIDs: aggregate.ProcessedEventIDs,
		ProofLineage: aggregate.CoverageProofLineage, Confirmations: aggregate.RepairSurfaceConfirmations,
		Reconciliation: aggregate.Reconciliation,
	}
}

// runtimeAssessmentLineageCutoff 固定原 assessment seal；非 assessment 查询沿用当前序列。
func runtimeAssessmentLineageCutoff(aggregate RuntimeAggregate) string {
	if aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.Candidate.Assessment != nil {
		return aggregate.ActiveDispatch.Transaction.EvidenceRevision
	}
	return aggregate.Facts.EvidenceRevision
}
