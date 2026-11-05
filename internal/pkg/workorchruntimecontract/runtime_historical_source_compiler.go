package workorchruntimecontract

// Complete pure consumer results live only in one owner-private transaction.
// A detached full source superset, not a digest or normalized projection, is
// compared at every gate. This execution guard does not certify historical
// behavior or acceptance results.
type runtimeHistoricalSourcePreimage struct {
	raw       RuntimeLineageAssertionSources
	execution []runtimeExecutionLineagePreimage
}

type runtimeExecutionLineagePreimage struct {
	binding  CoverageEvidenceBinding
	revision string
}

type runtimeHistoricalSourceCompiler struct {
	images []runtimeHistoricalSourcePreimage
}

func (c *runtimeHistoricalSourceCompiler) index(s RuntimeLineageAssertionSources) int {
	for i := range c.images {
		if runtimeRawEqual(c.images[i].raw, s) {
			return i
		}
	}
	return -1
}

// Only successful complete guard computations on identical full inputs may
// share work inside one lexical owner transaction. No false result, receipt,
// assessment credit or result survives that transaction.
func (c *runtimeHistoricalSourceCompiler) executionSatisfied(binding CoverageEvidenceBinding, s RuntimeLineageAssertionSources, revision string, body func() bool) bool {
	if c == nil {
		return body()
	}
	index := c.index(s)
	if index >= 0 {
		for _, entry := range c.images[index].execution {
			if entry.revision == revision && runtimeRawEqual(entry.binding, binding) {
				return true
			}
		}
	}
	if !body() {
		return false
	}
	// A nested consumer may have registered the same complete source while
	// computing the guard. Reuse that detached graph rather than cloning twice.
	if index < 0 {
		index = c.index(s)
	}
	if index < 0 {
		c.images = append(c.images, runtimeHistoricalSourcePreimage{raw: cloneRuntimeLineageSourcesRaw(s)})
		index = len(c.images) - 1
	}
	c.images[index].execution = append(c.images[index].execution, runtimeExecutionLineagePreimage{binding: cloneCoverageEvidenceBindingRaw(binding), revision: revision})
	return true
}

func cloneRuntimeLineageSourcesRaw(s RuntimeLineageAssertionSources) RuntimeLineageAssertionSources {
	// Reuse the persistence raw copier, which preserves nil/empty and every
	// nested original byte without normalization. No record is validated here.
	r := (RuntimeStateRecord{
		Definition: s.Definition, Facts: s.Facts, ImplementationWork: s.Work,
		SemanticPlanning: s.Semantic, ActiveDispatch: s.Active, CompletedDispatches: s.Completed,
		OperationReceipts: s.Receipts, OperationGovernanceBindings: s.Governance,
		ExecutionBudget: s.Budget, EvidenceSequence: s.EvidenceSequence,
		GovernanceResolutions: s.Resolutions, CandidateAttempts: s.Attempts,
		CandidateRequestSessions: s.Sessions, ProcessedEventIDs: s.ProcessedEventIDs,
		CoverageProofLineage: s.ProofLineage, RepairSurfaceConfirmations: s.Confirmations,
		Reconciliation: s.Reconciliation,
	}).Clone()
	return RuntimeLineageAssertionSources{
		Definition: r.Definition, Facts: r.Facts, Work: r.ImplementationWork,
		Semantic: r.SemanticPlanning, Active: r.ActiveDispatch, Completed: r.CompletedDispatches,
		Receipts: r.OperationReceipts, Governance: r.OperationGovernanceBindings,
		Budget: r.ExecutionBudget, EvidenceSequence: r.EvidenceSequence,
		Resolutions: r.GovernanceResolutions, Attempts: r.CandidateAttempts,
		Sessions: r.CandidateRequestSessions, ProcessedEventIDs: r.ProcessedEventIDs,
		ProofLineage: r.CoverageProofLineage, Confirmations: r.RepairSurfaceConfirmations,
		Reconciliation: r.Reconciliation,
	}
}
