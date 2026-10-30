package workorchruntimecontract

// Complete pure consumer results live only in one owner-private transaction.
// A detached full source superset, not a digest or normalized projection, is
// compared at every gate. The two consumers cannot certify each other.
type runtimeHistoricalSourcePreimage struct {
	raw                     RuntimeLineageAssertionSources
	behavior, adjudications bool
}

type runtimeHistoricalSourceCompiler struct {
	images []runtimeHistoricalSourcePreimage
}

func (c *runtimeHistoricalSourceCompiler) validate(s RuntimeLineageAssertionSources, adjudications bool, body func() error) error {
	if c == nil {
		return body()
	}
	index := -1
	for i := range c.images {
		if runtimeRawEqual(c.images[i].raw, s) {
			index = i
			if adjudications && c.images[i].adjudications || !adjudications && c.images[i].behavior {
				return nil
			}
			break
		}
	}
	if err := body(); err != nil {
		return err
	}
	if index < 0 {
		c.images = append(c.images, runtimeHistoricalSourcePreimage{raw: cloneRuntimeLineageSourcesRaw(s)})
		index = len(c.images) - 1
	}
	if adjudications {
		c.images[index].adjudications = true
	} else {
		c.images[index].behavior = true
	}
	return nil
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
