package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeHistoricalAcceptanceProof is the detached result of replaying one
// original ordinary assessment. It grants no credit: only an SM-2 private
// transaction may install a settlement after verifying current applicability.
// Historical behavior and ordinary acceptance retain distinct sources.
type RuntimeHistoricalAcceptanceProof struct {
	Source                 RuntimeHistoricalAcceptanceSource
	HistoricalBindingRef   RuntimeCWIBindingRef
	BehaviorDispatchID     string
	BehaviorEvidenceRefs   []string
	AcceptanceBindingRef   RuntimeCWIBindingRef
	AssessmentDispatchID   string
	AcceptanceEvidenceRefs []string
}

const RuntimeHistoricalAcceptanceProofLimit = 128

func cloneRuntimeHistoricalAcceptanceProofs(values []RuntimeHistoricalAcceptanceProof) []RuntimeHistoricalAcceptanceProof {
	if values == nil {
		return nil
	}
	ret := make([]RuntimeHistoricalAcceptanceProof, len(values))
	for index, value := range values {
		ret[index] = value.Clone()
	}
	return ret
}

func (p RuntimeHistoricalAcceptanceProof) Clone() RuntimeHistoricalAcceptanceProof {
	p.BehaviorEvidenceRefs = slices.Clone(p.BehaviorEvidenceRefs)
	p.AcceptanceEvidenceRefs = slices.Clone(p.AcceptanceEvidenceRefs)
	return p
}

// CompileRuntimeHistoricalAcceptanceProof independently replays the original
// Definition endpoints, accepted semantic bindings, complete historical owner
// effects and the ordinary consumer's immutable assessment cutoff. A producer
// claim, current row status or recomputed local hash cannot supply this proof.
func CompileRuntimeHistoricalAcceptanceProof(sources RuntimeLineageAssertionSources, historicalID common.CoverageWorkItemID, assessmentID string) (RuntimeHistoricalAcceptanceProof, error) {
	return compileRuntimeHistoricalAcceptanceInSession(sources, historicalID, assessmentID, &runtimeHistoricalPreimageSession{})
}

func compileRuntimeHistoricalAcceptanceInSession(sources RuntimeLineageAssertionSources, historicalID common.CoverageWorkItemID, assessmentID string, session *runtimeHistoricalPreimageSession) (RuntimeHistoricalAcceptanceProof, error) {
	invalid := func() (RuntimeHistoricalAcceptanceProof, error) {
		return RuntimeHistoricalAcceptanceProof{}, fmt.Errorf("historical acceptance requires exact confirmed endpoints, complete behavior and independent ordinary all-of assessment")
	}
	endpoints, err := ResolveRuntimeEvidencePurposeEndpoints(sources.Definition, sources.Facts.Activation.ExecutionContract, sources.Facts.DefinitionSeed, historicalID)
	if err != nil {
		return RuntimeHistoricalAcceptanceProof{}, err
	}
	var history RuntimeCoverageItemFact
	matches := 0
	for _, row := range sources.Facts.Coverage {
		if row.ID == historicalID {
			history, matches = row, matches+1
		}
	}
	if matches != 1 || history.HistoricalBehavior == nil {
		return invalid()
	}
	behavior, err := compileRuntimeHistoricalBehaviorInSession(sources, historicalID, history.HistoricalBehavior.DispatchID, false, session)
	if err != nil || !reflect.DeepEqual(behavior, history.HistoricalBehavior) {
		return RuntimeHistoricalAcceptanceProof{}, fmt.Errorf("historical acceptance original behavior completion mismatch: %v", err)
	}
	var dispatch RuntimeDispatchRecord
	matches = 0
	for _, value := range sources.Completed {
		if value.ID == assessmentID {
			dispatch, matches = value, matches+1
		}
	}
	if matches != 1 || dispatch.State != RuntimeDispatchStateCommitted || dispatch.WorkspaceID != sources.Facts.WorkspaceID || dispatch.RunID != sources.Facts.RunID ||
		dispatch.Transaction.DefinitionRevision != sources.Facts.DefinitionRevision || dispatch.Transaction.State != ConvergenceStateNeedAssessment ||
		dispatch.AssessmentAuthorityInput == nil || dispatch.AssessmentAuthorityInput.Purpose != RuntimeAssessmentPurposeDefinitionProof || dispatch.Candidate.Assessment == nil ||
		dispatch.RepairCommitment != nil || dispatch.ImplementationWorkCommitment != nil ||
		!slices.Equal(dispatch.Candidate.Assessment.CoverageWorkItemIDs, []common.CoverageWorkItemID{endpoints.Acceptance.ID}) {
		return invalid()
	}
	var assessment RuntimeAssessmentFact
	matches = 0
	for _, value := range sources.Facts.Assessments {
		if value.ID == assessmentID {
			assessment, matches = value, matches+1
		}
	}
	if matches != 1 || assessment.Disposition != AssessmentDispositionSatisfied {
		return invalid()
	}
	if err := ValidateRuntimeAssessmentAuthoritySources(sources.Facts, nil, []RuntimeDispatchRecord{dispatch}); err != nil {
		return RuntimeHistoricalAcceptanceProof{}, fmt.Errorf("historical acceptance original assessment authority: %w", err)
	}
	if err := ValidateRuntimeAssessmentApplicabilityGenerations(sources.Reconciliation, nil, []RuntimeDispatchRecord{dispatch}); err != nil {
		return RuntimeHistoricalAcceptanceProof{}, err
	}
	for _, observation := range dispatch.AssessmentAuthorityInput.ApplicabilityObservations {
		if err := ValidateRuntimeEvidenceApplicabilityObservationSource(observation, sources.Facts, sources.Receipts); err != nil {
			return RuntimeHistoricalAcceptanceProof{}, fmt.Errorf("historical acceptance original observation receipt: %w", err)
		}
	}
	if err := validateRuntimeAssessmentAdjudicationInSession(dispatch, assessment, sources.Facts, sources.Completed, sources.ProofLineage, sources, session); err != nil {
		return RuntimeHistoricalAcceptanceProof{}, fmt.Errorf("historical acceptance original ordinary adjudication: %w", err)
	}
	binding, found := assessmentBindingForCoverage(dispatch.CoverageBindings, endpoints.Acceptance.ID)
	if !found || binding.Validate() != nil || binding.HistoricalAcceptance != nil ||
		binding.DeclarationRef != CanonicalRuntimeDefinitionEvidenceDeclarationRef(endpoints.Acceptance.SourceScope, endpoints.Acceptance.SourceID) {
		return invalid()
	}
	entries, err := session.preimages(sources.Semantic)
	if err != nil {
		return invalid()
	}
	var original runtimeAssessmentPurposeBindingPreimage
	matches = 0
	for _, entry := range entries {
		if RuntimeCoverageEvidenceBindingProjectionsEqual(entry.bindings, dispatch.CoverageBindings) {
			original, matches = entry, matches+1
		}
	}
	if matches != 1 || original.obligation.HistoricalAcceptance != nil || original.obligation.ValidationProofRequirement == nil ||
		!sameRuntimeSourceObligationChecks(endpoints.Acceptance.SourceObligationChecks, original.obligation.ValidationProofRequirement.SourceObligationChecks) {
		return RuntimeHistoricalAcceptanceProof{}, fmt.Errorf("historical acceptance original semantic all-of check source mismatch")
	}
	preimage, err := original.assessmentPurposeInSession(session.semanticSession())
	if err != nil || preimage.Purpose != RuntimeAssessmentPurposeDefinitionProof ||
		validateRuntimeHistoricalSemanticStartupSource(sources, preimage.SemanticManifestHash) != nil {
		return invalid()
	}
	proof, found := CoverageEvidenceProofForKind(binding, EvidenceKindValidation)
	if !found || proof.ValidationProofRequirementHash != original.obligation.ValidationProofRequirement.Hash {
		return invalid()
	}
	for _, check := range endpoints.Acceptance.SourceObligationChecks {
		if !slices.Contains(proof.RequiredCheckRefs, check.CheckRef) {
			return invalid()
		}
	}
	// The ordinary assessment must follow every original effect. In particular,
	// future behavior cannot be borrowed by an older same-revision observation.
	for _, ref := range behavior.EvidenceRefs {
		matches = 0
		for _, fact := range sources.Facts.Evidence.Accepted {
			if string(fact.ID) == ref && fact.Revision != "" && !EvidenceRevisionAfter(fact.Revision, dispatch.Transaction.EvidenceRevision) {
				matches++
			}
		}
		if matches != 1 {
			return invalid()
		}
	}
	refs := make([]string, 0)
	for _, selected := range dispatch.AssessmentAdjudication.Proofs {
		if selected.CoverageWorkItemID != endpoints.Acceptance.ID || selected.EvidenceKind != EvidenceKindValidation {
			continue
		}
		var fact EvidenceFact
		matches = 0
		for _, value := range sources.Facts.Evidence.Accepted {
			if value.ID == selected.EvidenceFactID && StableRuntimeEvidenceFactHash(value) == selected.EvidenceFactHash {
				fact, matches = value, matches+1
			}
		}
		if matches != 1 || fact.Validation == nil || fact.OwnerRevisionSet == nil || fact.ApplicabilityDependencies == nil || RuntimeEvidenceIsHistoricalEffect(fact) || assessmentEvidenceWeak(fact) ||
			!RuntimeValidationObservationSatisfiesCoverageProof(proof, *fact.Validation) ||
			!RuntimeEvidenceHasFreshProofApplicability(fact, dispatch.AssessmentAuthorityInput.ApplicabilityObservations, dispatch.Transaction.EvidenceRevision, dispatch.AssessmentAuthorityInput.ApplicabilityReconciliationRequestHash) {
			return invalid()
		}
		var operationDispatch RuntimeDispatchRecord
		matches = 0
		for _, value := range sources.Completed {
			if value.ID == fact.DispatchID {
				operationDispatch, matches = value, matches+1
			}
		}
		operation, found := assessmentOperationForEvidence(sources.Completed, fact)
		if matches != 1 || !found || operationDispatch.State != RuntimeDispatchStateCommitted {
			return invalid()
		}
		ownerFact, receipt, valid := runtimeLineageOperationFact(operationDispatch, operation, sources, true)
		if !valid || !reflect.DeepEqual(ownerFact, fact) {
			return RuntimeHistoricalAcceptanceProof{}, fmt.Errorf("historical acceptance original ordinary owner receipt lineage mismatch")
		}
		if err := MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindValidation, operation, receipt); err != nil {
			return RuntimeHistoricalAcceptanceProof{}, fmt.Errorf("historical acceptance ordinary all-of receipt: %w", err)
		}
		refs = append(refs, string(fact.ID))
	}
	if len(refs) == 0 {
		return invalid()
	}
	slices.Sort(refs)
	return RuntimeHistoricalAcceptanceProof{Source: behavior.Source, HistoricalBindingRef: behavior.BindingRef,
		BehaviorDispatchID: behavior.DispatchID, BehaviorEvidenceRefs: slices.Clone(behavior.EvidenceRefs),
		AcceptanceBindingRef: RuntimeCWIBindingRefForCoverageBinding(binding, preimage.SemanticManifestHash),
		AssessmentDispatchID: dispatch.ID, AcceptanceEvidenceRefs: refs}, nil
}

// Immutable historical settlements are replayed against their original
// assessment, not today's observations. Active credit additionally requires
// the exact consumer's current settled binding; terminal gates separately
// enforce this revision/generation's ordinary current-read observations.
func validateRuntimeHistoricalAcceptanceProofs(sources RuntimeLineageAssertionSources, row RuntimeCoverageItemFact) error {
	return validateRuntimeHistoricalAcceptanceProofsInSession(sources, row, &runtimeHistoricalPreimageSession{})
}

func validateRuntimeHistoricalAcceptanceProofsInSession(sources RuntimeLineageAssertionSources, row RuntimeCoverageItemFact, session *runtimeHistoricalPreimageSession) error {
	if len(row.HistoricalAcceptanceProofs) > RuntimeHistoricalAcceptanceProofLimit {
		return fmt.Errorf("historical acceptance proof history exceeds bound")
	}
	seen := make(map[string]bool, len(row.HistoricalAcceptanceProofs))
	lastPosition := -1
	for _, value := range row.HistoricalAcceptanceProofs {
		if seen[value.AssessmentDispatchID] {
			return fmt.Errorf("historical acceptance repeats original assessment")
		}
		seen[value.AssessmentDispatchID] = true
		expected, err := compileRuntimeHistoricalAcceptanceInSession(sources, row.ID, value.AssessmentDispatchID, session)
		if err != nil || !reflect.DeepEqual(expected, value) {
			return fmt.Errorf("historical acceptance differs from original all-of assessment: %v", err)
		}
		position := -1
		for index, dispatch := range sources.Completed {
			if dispatch.ID == value.AssessmentDispatchID {
				position = index
			}
		}
		if position <= lastPosition {
			return fmt.Errorf("historical acceptance proof history is not append ordered")
		}
		lastPosition = position
	}
	if row.State != RuntimeCoverageItemStateSatisfied {
		return nil
	}
	if len(row.HistoricalAcceptanceProofs) == 0 {
		return fmt.Errorf("historical effect cannot grant terminal credit without independent acceptance")
	}
	current := row.HistoricalAcceptanceProofs[len(row.HistoricalAcceptanceProofs)-1]
	if row.LastAssessmentID != current.AssessmentDispatchID || !slices.Equal(row.EvidenceRefs, normalizeStrings(append(slices.Clone(current.BehaviorEvidenceRefs), current.AcceptanceEvidenceRefs...))) {
		return fmt.Errorf("historical terminal credit differs from exact all-of settlement")
	}
	matches := 0
	for _, consumer := range sources.Facts.Coverage {
		if consumer.ID != current.Source.AcceptanceCoverageWorkItemID {
			continue
		}
		if consumer.State != RuntimeCoverageItemStateSatisfied || consumer.CWIConvergenceState != RuntimeCWIConvergenceStateSettled ||
			consumer.CurrentBindingRef == nil || !SameRuntimeCWIBindingRef(*consumer.CurrentBindingRef, current.AcceptanceBindingRef) || consumer.LastAssessmentID != current.AssessmentDispatchID {
			return fmt.Errorf("historical terminal credit has no exact current ordinary consumer settlement")
		}
		matches++
	}
	if matches != 1 {
		return fmt.Errorf("historical terminal credit has ambiguous ordinary consumer")
	}
	return nil
}
