package workorchruntimecontract

import "fmt"

// runtimeTerminalCurrentCoverageBinding 只读取 exact current ledger ref；不重建或补封存 binding。
func runtimeTerminalCurrentCoverageBinding(facts RuntimeFacts, coverage RuntimeCoverageItemFact) (CoverageEvidenceBinding, bool) {
	if coverage.CurrentBindingRef == nil {
		return CoverageEvidenceBinding{}, false
	}
	current := coverage.CurrentBindingRef
	if current.Validate() != nil {
		return CoverageEvidenceBinding{}, false
	}
	var binding CoverageEvidenceBinding
	matches := 0
	focuses := append([]CoverageFocus{facts.Focus}, facts.RemainingFocuses...)
	focuses = append(focuses, facts.CompletedFocuses...)
	for _, focus := range focuses {
		for _, value := range focus.CoverageEvidenceBindings {
			if value.CoverageWorkItemID == coverage.ID && value.ID == current.BindingID && value.Hash == current.BindingHash && value.FocusID == current.FocusID && value.DefinitionRevision == facts.DefinitionRevision {
				binding = value
				matches++
			}
		}
	}
	return binding, matches == 1
}

// runtimeTerminalEvidenceMatchesCurrentProof 区分当前证明与历史 accepted 审计，不授予 CWI credit。
func runtimeTerminalEvidenceMatchesCurrentProof(source RuntimeStateRecord, evidence EvidenceFact, binding CoverageEvidenceBinding) bool {
	if !RuntimeEvidenceHasFreshProofApplicability(evidence, source.Facts.EvidenceApplicabilityObservations, source.Facts.EvidenceRevision, source.Reconciliation.RequestHash) {
		return false
	}
	return runtimeTerminalEvidenceMatchesProofSource(source, evidence, binding)
}

func runtimeTerminalEvidenceMatchesProofSource(source RuntimeStateRecord, evidence EvidenceFact, binding CoverageEvidenceBinding) bool {
	if assessmentEvidenceWeak(evidence) || !coverageWorkItemIDInAssessment(evidence.CoverageWorkItemIDs, binding.CoverageWorkItemID) {
		return false
	}
	proof, found := CoverageEvidenceProofForKind(binding, evidence.Kind)
	if !found {
		return false
	}
	if evidence.Kind == EvidenceKindValidation && (evidence.Validation == nil || !RuntimeValidationObservationSatisfiesCoverageProof(proof, *evidence.Validation)) {
		return false
	}
	for _, metric := range proof.MetricConstraints {
		if !containsString(evidence.EvidenceTags, metric) {
			return false
		}
	}
	for _, artifact := range proof.ArtifactRefs {
		if !containsString(evidence.EvidenceRefs, artifact) {
			return false
		}
	}
	var original RuntimeCoverageProofLineage
	matches := 0
	for _, row := range source.CoverageProofLineage {
		if row.Kind == RuntimeCoverageProofLineageKindSource && row.SourceEvidenceID == evidence.ID && row.CoverageWorkItemID == binding.CoverageWorkItemID && row.EvidenceKind == evidence.Kind {
			if row.WorkspaceID != source.Facts.WorkspaceID || row.RunID != source.Facts.RunID {
				return false
			}
			original = row
			matches++
		}
	}
	if matches == 0 {
		// 未使用 rollover 时也反查原 completed dispatch，不能由当前 binding 自证来源。
		if !containsString(evidence.BindingRefs, binding.ID) {
			return false
		}
		for _, dispatch := range source.CompletedDispatches {
			if dispatch.ID != evidence.DispatchID || dispatch.WorkspaceID != source.Facts.WorkspaceID || dispatch.RunID != source.Facts.RunID {
				continue
			}
			for _, original := range dispatch.CoverageBindings {
				if original.ID == binding.ID && original.Hash == binding.Hash && original.CoverageWorkItemID == binding.CoverageWorkItemID {
					return true
				}
			}
		}
		return false
	}
	if matches != 1 || !containsString(evidence.BindingRefs, original.SourceBindingRef) || original.SourceDispatchID != evidence.DispatchID || original.SourceOperationID != evidence.OperationID {
		return false
	}
	currentIdentity, err := DeriveCoverageEvidenceProofIdentity(binding, evidence.Kind)
	if err != nil {
		return false
	}
	if original.SourceBindingRef == binding.ID && original.SourceBindingHash == binding.Hash {
		return SameCoverageEvidenceProofIdentity(original.SourceProofIdentity, currentIdentity)
	}
	resolutionMatches := 0
	equivalent := false
	for _, row := range source.CoverageProofLineage {
		if row.Kind != RuntimeCoverageProofLineageKindResolution || row.SourceLineageID != original.ID || row.CurrentBindingRef != binding.ID || row.CurrentBindingHash != binding.Hash {
			continue
		}
		resolutionMatches++
		equivalent = row.WorkspaceID == source.Facts.WorkspaceID && row.RunID == source.Facts.RunID && row.CoverageWorkItemID == binding.CoverageWorkItemID && row.EvidenceKind == evidence.Kind && row.Disposition == RuntimeCoverageProofLineageDispositionEquivalent && SameCoverageEvidenceProofIdentity(original.SourceProofIdentity, currentIdentity) && SameCoverageEvidenceProofIdentity(row.CurrentProofIdentity, currentIdentity)
	}
	return resolutionMatches == 1 && equivalent
}

// Success cannot silently omit a versioned proof's freshness obligation. This
// is a material gate, not new SM-2 credit or an owner query. Failed/admin results
// still retain and deliver their original history.
func validateRuntimeTerminalEvidenceApplicability(source RuntimeStateRecord) error {
	for _, coverage := range source.Facts.Coverage {
		binding, found := runtimeTerminalCurrentCoverageBinding(source.Facts, coverage)
		if !found || coverage.State != RuntimeCoverageItemStateSatisfied {
			continue
		}
		if coverage.HistoricalBehavior != nil {
			if _, _, valid := runtimeTerminalHistoricalAcceptance(source, coverage); !valid {
				return fmt.Errorf("terminal historical effect %q lacks exact original settlement and fresh ordinary consumer", coverage.ID)
			}
			continue
		}
		var selected []EvidenceFact
		for _, fact := range source.Facts.Evidence.Accepted {
			if runtimeTerminalEvidenceMatchesCurrentProof(source, fact, binding) {
				selected = append(selected, fact)
			}
		}
		if !RuntimeHistoricalEffectsHaveCurrentAcceptanceProof(selected) {
			return fmt.Errorf("terminal historical effect %q lacks independent current ordinary acceptance proof", coverage.ID)
		}
		for _, proof := range binding.Proofs {
			versioned, fresh := false, false
			for _, fact := range source.Facts.Evidence.Accepted {
				if fact.Kind != proof.EvidenceKind || !runtimeTerminalEvidenceMatchesProofSource(source, fact, binding) {
					continue
				}
				versioned = versioned || fact.ApplicabilityDependencies != nil
				fresh = fresh || RuntimeEvidenceHasFreshProofApplicability(fact, source.Facts.EvidenceApplicabilityObservations, source.Facts.EvidenceRevision, source.Reconciliation.RequestHash)
			}
			if versioned && !fresh {
				return fmt.Errorf("terminal current proof %q/%q lacks exact-generation applicability observation", coverage.ID, proof.EvidenceKind)
			}
		}
	}
	return nil
}
