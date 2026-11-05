package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
)

// ValidateRuntimeHistoricalBehaviorCompletions reads committed SM-2 products.
// Original semantic compilation and receipt adjudication belong to their
// producers. This consumer has no semantic, provider or recovery graph input.
func ValidateRuntimeHistoricalBehaviorCompletions(facts RuntimeFacts, completed []RuntimeDispatchRecord) error {
	return runtimeProcessingRegion("historical_behavior", func() error {
		for _, row := range facts.Coverage {
			if row.HistoricalBehavior == nil {
				if len(row.HistoricalAcceptanceProofs) != 0 {
					return fmt.Errorf("historical acceptance lacks original behavior completion")
				}
				continue
			}
			if err := validateRuntimeHistoricalBehaviorResult(facts, completed, row); err != nil {
				return err
			}
			if err := validateRuntimeHistoricalAcceptanceResults(facts, completed, row); err != nil {
				return err
			}
		}
		return nil
	})
}

func committedHistoricalDispatch(facts RuntimeFacts, completed []RuntimeDispatchRecord, id string) (RuntimeDispatchRecord, error) {
	var result RuntimeDispatchRecord
	matches := 0
	for _, dispatch := range completed {
		if dispatch.ID == id {
			result, matches = dispatch, matches+1
		}
	}
	if id == "" || matches != 1 || result.State != RuntimeDispatchStateCommitted ||
		result.WorkspaceID != facts.WorkspaceID || result.RunID != facts.RunID || result.Transaction.DefinitionRevision != facts.DefinitionRevision {
		return RuntimeDispatchRecord{}, fmt.Errorf("historical result has no exact committed source dispatch %q", id)
	}
	return result, nil
}

func validateRuntimeHistoricalBehaviorResult(facts RuntimeFacts, completed []RuntimeDispatchRecord, row RuntimeCoverageItemFact) error {
	value := row.HistoricalBehavior
	if value == nil || value.Source.Validate(row.ID) != nil || value.DispatchID == "" || value.BindingRef.Validate() != nil || !reflect.DeepEqual(value.BindingRef, value.BindingRef.Normalized()) ||
		row.CurrentBindingRef == nil || !SameRuntimeCWIBindingRef(value.BindingRef, *row.CurrentBindingRef) ||
		(row.State != RuntimeCoverageItemStateOpen || row.CWIConvergenceState != RuntimeCWIConvergenceStateBound) &&
			(row.State != RuntimeCoverageItemStateSatisfied || row.CWIConvergenceState != RuntimeCWIConvergenceStateSettled) {
		return fmt.Errorf("historical behavior committed result identity mismatch")
	}
	matches := 0
	for _, item := range facts.DefinitionSeed.CoverageItems {
		if item.ID == row.ID && sameRuntimeHistoricalAcceptanceSource(item.HistoricalAcceptance, &value.Source) {
			matches++
		}
	}
	if matches != 1 || facts.DefinitionSeed.SourceObligations == nil || value.Source.InventoryHash != facts.DefinitionSeed.SourceObligations.Hash {
		return fmt.Errorf("historical behavior committed result definition source mismatch")
	}
	dispatch, err := committedHistoricalDispatch(facts, completed, value.DispatchID)
	if err != nil {
		return err
	}
	binding, found := assessmentBindingForCoverage(dispatch.CoverageBindings, row.ID)
	if !found || !SameRuntimeCWIBindingRef(value.BindingRef, RuntimeCWIBindingRefForCoverageBinding(binding, value.BindingRef.SemanticManifestHash)) ||
		!sameRuntimeHistoricalAcceptanceSource(binding.HistoricalAcceptance, &value.Source) || dispatch.Transaction.FocusID != row.FocusID ||
		dispatch.Transaction.State != ConvergenceStateNeedStateChange || dispatch.RepairCommitment != nil || dispatch.ImplementationWorkCommitment != nil {
		return fmt.Errorf("historical behavior committed binding source mismatch")
	}
	// Resolve exact evidence identities through the original execution ledger.
	// Do not re-evaluate the original effect, semantic feasibility or all-of.
	refs := make([]string, 0)
	for _, operation := range runtimeCandidateAdmissionOperations(dispatch.Candidate) {
		if !slices.Contains(operation.BindingRefs, binding.ID) {
			continue
		}
		var execution RuntimeOperationExecutionRecord
		count := 0
		for _, entry := range dispatch.OperationExecutions {
			if entry.OperationID == operation.ID {
				execution, count = entry, count+1
			}
		}
		if count != 1 || execution.State != OperationExecutionEvidenceCommitted || !slices.Contains(dispatch.CompletedOperationIDs, operation.ID) {
			return fmt.Errorf("historical behavior committed execution source is missing")
		}
		count = 0
		for _, fact := range facts.Evidence.Accepted {
			if fact.ID == execution.EvidenceFactID && fact.DispatchID == dispatch.ID && fact.OperationID == operation.ID &&
				fact.FocusID == row.FocusID && fact.Kind == EvidenceKindStateChange && slices.Contains(fact.CoverageWorkItemIDs, row.ID) &&
				slices.Contains(fact.BindingRefs, binding.ID) && StableRuntimeEvidenceFactHash(fact) == execution.EvidenceFactHash {
				count++
			}
		}
		if count != 1 {
			return fmt.Errorf("historical behavior committed evidence source integrity mismatch")
		}
		refs = append(refs, string(execution.EvidenceFactID))
	}
	slices.Sort(refs)
	if len(refs) == 0 || !slices.Equal(refs, value.EvidenceRefs) {
		return fmt.Errorf("historical behavior committed evidence membership mismatch")
	}
	return nil
}

func validateRuntimeHistoricalAcceptanceResult(facts RuntimeFacts, completed []RuntimeDispatchRecord, row RuntimeCoverageItemFact, value RuntimeHistoricalAcceptanceProof) error {
	behavior := row.HistoricalBehavior
	if behavior == nil || !reflect.DeepEqual(value.Source, behavior.Source) || value.HistoricalBindingRef != behavior.BindingRef ||
		value.AcceptanceBindingRef.Validate() != nil || !reflect.DeepEqual(value.AcceptanceBindingRef, value.AcceptanceBindingRef.Normalized()) ||
		value.BehaviorDispatchID != behavior.DispatchID || !slices.Equal(value.BehaviorEvidenceRefs, behavior.EvidenceRefs) {
		return fmt.Errorf("historical acceptance committed behavior reference mismatch")
	}
	dispatch, err := committedHistoricalDispatch(facts, completed, value.AssessmentDispatchID)
	if err != nil {
		return err
	}
	if dispatch.Candidate.Assessment == nil || dispatch.AssessmentAuthorityInput == nil ||
		dispatch.AssessmentAuthorityInput.Purpose != RuntimeAssessmentPurposeDefinitionProof ||
		!slices.Equal(dispatch.Candidate.Assessment.CoverageWorkItemIDs, []common.CoverageWorkItemID{value.Source.AcceptanceCoverageWorkItemID}) {
		return fmt.Errorf("historical acceptance committed assessment identity mismatch")
	}
	var fact RuntimeAssessmentFact
	matches := 0
	for _, original := range facts.Assessments {
		if original.ID == value.AssessmentDispatchID {
			fact, matches = original, matches+1
		}
	}
	if matches != 1 || fact.Disposition != AssessmentDispositionSatisfied {
		return fmt.Errorf("historical acceptance has no original satisfied assessment")
	}
	if err := ValidateRuntimeAssessmentAdjudication(dispatch, fact, facts); err != nil {
		return err
	}
	binding, found := assessmentBindingForCoverage(dispatch.CoverageBindings, value.Source.AcceptanceCoverageWorkItemID)
	if !found || !SameRuntimeCWIBindingRef(value.AcceptanceBindingRef, RuntimeCWIBindingRefForCoverageBinding(binding, value.AcceptanceBindingRef.SemanticManifestHash)) ||
		binding.HistoricalAcceptance != nil {
		return fmt.Errorf("historical acceptance committed binding reference mismatch")
	}
	matches = 0
	for _, item := range facts.DefinitionSeed.CoverageItems {
		if item.ID == value.Source.AcceptanceCoverageWorkItemID && binding.DeclarationRef == CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID) {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("historical acceptance committed declaration source mismatch")
	}
	matches = 0
	for _, consumer := range facts.Coverage {
		if consumer.ID == value.Source.AcceptanceCoverageWorkItemID && slices.Contains(consumer.BindingLineage, value.AcceptanceBindingRef) {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("historical acceptance committed binding lineage is missing")
	}
	refs := make([]string, 0)
	for _, selected := range dispatch.AssessmentAdjudication.Proofs {
		if selected.CoverageWorkItemID == value.Source.AcceptanceCoverageWorkItemID && selected.EvidenceKind == EvidenceKindValidation {
			refs = append(refs, string(selected.EvidenceFactID))
		}
	}
	slices.Sort(refs)
	if len(refs) == 0 || !slices.Equal(refs, value.AcceptanceEvidenceRefs) {
		return fmt.Errorf("historical acceptance committed evidence membership mismatch")
	}
	return nil
}
