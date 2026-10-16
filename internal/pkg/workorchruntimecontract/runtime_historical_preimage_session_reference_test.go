package workorchruntimecontract

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	"reflect"
	"slices"
)

// Original complete historical consumers before local preimage-index reuse.
func referenceHistoricalBehaviorBeforePreimageSession(sources RuntimeLineageAssertionSources, historicalID common.CoverageWorkItemID, dispatchID string, requireOpen bool) (*RuntimeHistoricalBehaviorCompletion, error) {
	invalid := func() (*RuntimeHistoricalBehaviorCompletion, error) {
		return nil, fmt.Errorf("historical behavior requires original confirmed mapping and complete exact accepted effect lineage")
	}
	endpoints, err := ResolveRuntimeEvidencePurposeEndpoints(sources.Definition, sources.Facts.Activation.ExecutionContract, sources.Facts.DefinitionSeed, historicalID)
	if err != nil {
		return nil, err
	}
	var row RuntimeCoverageItemFact
	rows := 0
	for _, candidate := range sources.Facts.Coverage {
		if candidate.ID == historicalID {
			row, rows = candidate, rows+1
		}
	}
	if rows != 1 || row.CurrentBindingRef == nil {
		return invalid()
	}
	open := row.CWIConvergenceState == RuntimeCWIConvergenceStateBound && row.State == RuntimeCoverageItemStateOpen
	settled := row.CWIConvergenceState == RuntimeCWIConvergenceStateSettled && row.State == RuntimeCoverageItemStateSatisfied
	if !open && (requireOpen || !settled) {
		return invalid()
	}
	var dispatch RuntimeDispatchRecord
	matches := 0
	for _, candidate := range sources.Completed {
		if candidate.ID == dispatchID {
			dispatch, matches = candidate, matches+1
		}
	}
	operations := runtimeCandidateAdmissionOperations(dispatch.Candidate)
	if matches != 1 || dispatch.ID == "" || dispatch.State != RuntimeDispatchStateCommitted ||
		dispatch.WorkspaceID != sources.Facts.WorkspaceID || dispatch.RunID != sources.Facts.RunID ||
		dispatch.Transaction.DefinitionRevision != sources.Facts.DefinitionRevision || dispatch.Transaction.State != ConvergenceStateNeedStateChange ||
		dispatch.Transaction.FocusID != row.FocusID || dispatch.RepairCommitment != nil || dispatch.ImplementationWorkCommitment != nil ||
		len(operations) == 0 || len(dispatch.CompletedOperationIDs) != len(operations) {
		return invalid()
	}
	var binding CoverageEvidenceBinding
	matches = 0
	for _, candidate := range dispatch.CoverageBindings {
		if candidate.CoverageWorkItemID == historicalID {
			binding, matches = candidate, matches+1
		}
	}
	if matches != 1 || binding.Validate() != nil || binding.HistoricalAcceptance == nil ||
		binding.DeclarationRef != CanonicalRuntimeDefinitionEvidenceDeclarationRef(endpoints.Historical.SourceScope, endpoints.Historical.SourceID) ||
		!sameRuntimeHistoricalAcceptanceSource(binding.HistoricalAcceptance, endpoints.Historical.HistoricalAcceptance) ||
		!SameRuntimeCWIBindingRef(*row.CurrentBindingRef, RuntimeCWIBindingRefForCoverageBinding(binding, row.CurrentBindingRef.SemanticManifestHash)) ||
		len(binding.Proofs) != 1 || binding.Proofs[0].EvidenceKind != EvidenceKindStateChange {
		return invalid()
	}
	// Binding hashes and dispatch-local declarations cannot certify their own
	// provenance. Replay the accepted provider document and receiving-owner
	// feasibility from the original semantic generation, including after it
	// has been retired by the effect's catalog invalidation.
	preimage, err := ResolveRuntimeAssessmentPurposePreimage(sources.Semantic, dispatch.CoverageBindings)
	if err != nil || preimage.Purpose != RuntimeAssessmentPurposeDefinitionProof ||
		preimage.SemanticManifestHash != row.CurrentBindingRef.SemanticManifestHash ||
		validateRuntimeHistoricalSemanticStartupSource(sources, preimage.SemanticManifestHash) != nil {
		return invalid()
	}
	refs := make([]string, 0, len(operations))
	targets := make([]string, 0)
	seenOperations := make(map[string]bool, len(operations))
	for _, operation := range operations {
		if seenOperations[operation.ID] || !slices.Contains(dispatch.CompletedOperationIDs, operation.ID) {
			return invalid()
		}
		seenOperations[operation.ID] = true
		if !slices.Contains(operation.BindingRefs, binding.ID) {
			continue
		}
		fact, receipt, valid := runtimeLineageOperationFact(dispatch, operation, sources, true)
		if !valid || fact.FocusID != row.FocusID || fact.Kind != EvidenceKindStateChange ||
			!slices.Contains(fact.CoverageWorkItemIDs, historicalID) || !RuntimeEvidenceIsHistoricalEffect(fact) ||
			ValidateRuntimeHistoricalEffectReceiptPurpose(dispatch, operation, receipt.Evidence.ApplicabilityDependencies) != nil ||
			MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindStateChange, operation, receipt) != nil {
			return invalid()
		}
		refs = append(refs, string(fact.ID))
		targets = append(targets, operation.TargetRefs...)
	}
	if len(refs) == 0 {
		return invalid()
	}
	// Every sealed prospective member must occur in this one dispatch. The
	// normal receipt matcher above still validates each singleton target.
	for _, target := range binding.ProspectiveTargetRefs {
		if count := slices.Index(targets, target); count < 0 {
			return invalid()
		}
		members := 0
		for _, actual := range targets {
			if actual == target {
				members++
			}
		}
		if members != 1 {
			return invalid()
		}
	}
	slices.Sort(refs)
	return &RuntimeHistoricalBehaviorCompletion{Source: *binding.HistoricalAcceptance, BindingRef: *row.CurrentBindingRef, DispatchID: dispatch.ID, EvidenceRefs: refs}, nil
}

func referenceHistoricalCompletionsBeforePreimageSession(sources RuntimeLineageAssertionSources) error {
	for _, row := range sources.Facts.Coverage {
		if row.HistoricalBehavior == nil {
			continue
		}
		expected, err := referenceHistoricalBehaviorBeforePreimageSession(sources, row.ID, row.HistoricalBehavior.DispatchID, false)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(row.HistoricalBehavior, expected) {
			return fmt.Errorf("historical behavior differs from original complete effect lineage")
		}
		if err := referenceHistoricalProofsBeforePreimageSession(sources, row); err != nil {
			return err
		}
	}
	for _, row := range sources.Facts.Coverage {
		if row.HistoricalBehavior == nil && len(row.HistoricalAcceptanceProofs) != 0 {
			return fmt.Errorf("historical acceptance lacks original behavior completion")
		}
	}
	return nil
}

// RuntimeCWIPrerequisiteReadyForConsumer preserves ordinary settled admission.
func referenceHistoricalAcceptanceBeforePreimageSession(sources RuntimeLineageAssertionSources, historicalID common.CoverageWorkItemID, assessmentID string) (RuntimeHistoricalAcceptanceProof, error) {
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
	behavior, err := referenceHistoricalBehaviorBeforePreimageSession(sources, historicalID, history.HistoricalBehavior.DispatchID, false)
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
	if err := ValidateRuntimeAssessmentAdjudication(dispatch, assessment, sources.Facts, sources.Completed, sources.ProofLineage, sources); err != nil {
		return RuntimeHistoricalAcceptanceProof{}, fmt.Errorf("historical acceptance original ordinary adjudication: %w", err)
	}
	binding, found := assessmentBindingForCoverage(dispatch.CoverageBindings, endpoints.Acceptance.ID)
	if !found || binding.Validate() != nil || binding.HistoricalAcceptance != nil ||
		binding.DeclarationRef != CanonicalRuntimeDefinitionEvidenceDeclarationRef(endpoints.Acceptance.SourceScope, endpoints.Acceptance.SourceID) {
		return invalid()
	}
	entries, err := runtimeAssessmentPurposePreimagesFromSemanticLedger(sources.Semantic, false)
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
	preimage, err := original.assessmentPurpose()
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
func referenceHistoricalProofsBeforePreimageSession(sources RuntimeLineageAssertionSources, row RuntimeCoverageItemFact) error {
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
		expected, err := referenceHistoricalAcceptanceBeforePreimageSession(sources, row.ID, value.AssessmentDispatchID)
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
