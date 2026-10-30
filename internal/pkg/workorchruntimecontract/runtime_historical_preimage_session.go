package workorchruntimecontract

// A private, single consumer-call index. A successful full preimage traversal
// is retained only with a detached exact raw ledger; never match on hash alone.
type runtimeHistoricalPreimageSession struct {
	semantic          *runtimeSemanticValidationSession
	ledger            *RuntimeSemanticPlanningLedger
	entries           []runtimeAssessmentPurposeBindingPreimage
	reviews           []runtimeHistoricalInputReviewPreimage
	lineageOperations []RuntimeStateRecord
	// Only the owner-private transaction enables complete consumer subresults.
	// Ordinary public consumers keep their independent full-validation path.
	sourceCompiler *runtimeHistoricalSourceCompiler
}

// runtime-dto-source-ledger-compiler/v1 connects complete nested consumers to
// existing pure semantic gates only. A missing context remains an independent
// cold invocation, never a success credential. No retained type or cap changes.
func (s *runtimeHistoricalPreimageSession) semanticSession() *runtimeSemanticValidationSession {
	if s == nil {
		return newRuntimeSemanticValidationSession()
	}
	if s.semantic == nil {
		s.semantic = newRuntimeSemanticValidationSession()
	}
	return s.semantic
}

// This projection contains every raw field read by the complete operation
// lineage gate, not a hash or a caller-supplied validation flag.
func runtimeLineageOperationSourceRecord(s RuntimeLineageAssertionSources) RuntimeStateRecord {
	return RuntimeStateRecord{
		Facts:          RuntimeFacts{WorkspaceID: s.Facts.WorkspaceID, RunID: s.Facts.RunID, Evidence: s.Facts.Evidence},
		ActiveDispatch: s.Active, CompletedDispatches: s.Completed,
		CandidateAttempts: s.Attempts, CandidateRequestSessions: s.Sessions,
		OperationGovernanceBindings: s.Governance, GovernanceResolutions: s.Resolutions,
		OperationReceipts: s.Receipts,
	}
}

func runtimeValidateLineageOperationsInSession(sources RuntimeLineageAssertionSources, session *runtimeHistoricalPreimageSession) error {
	if session == nil {
		return runtimeValidateLineageOperations(sources)
	}
	raw := runtimeLineageOperationSourceRecord(sources)
	for _, prior := range session.lineageOperations {
		if runtimeRawEqual(prior, raw) {
			return nil
		}
	}
	if err := runtimeValidateLineageOperationsInSemanticSession(sources, session.semanticSession()); err != nil {
		return err
	}
	session.lineageOperations = append(session.lineageOperations, raw.Clone())
	return nil
}

func (s *runtimeHistoricalPreimageSession) preimages(ledger RuntimeSemanticPlanningLedger) ([]runtimeAssessmentPurposeBindingPreimage, error) {
	if s == nil {
		return runtimeAssessmentPurposePreimagesFromSemanticLedger(ledger, false)
	}
	if s.ledger != nil && runtimeRawEqual(*s.ledger, ledger) {
		return s.entries, nil
	}
	if s.semantic == nil {
		s.semantic = newRuntimeSemanticValidationSession()
	}
	entries, err := runtimeAssessmentPurposePreimagesFromSemanticLedgerInSession(ledger, false, s.semantic)
	if err != nil {
		return nil, err
	}
	raw := cloneRuntimeSemanticPlanningLedgerRaw(ledger)
	s.ledger, s.entries = &raw, entries
	// These products depend on the complete ledger admitted above. A new raw
	// ledger invalidates them, even if its declared hash/IDs did not change.
	s.reviews = nil
	return entries, nil
}

type runtimeHistoricalInputReviewPreimage struct {
	binding CoverageEvidenceBinding
	result  RuntimeValidationProofRequirementBinding
}
