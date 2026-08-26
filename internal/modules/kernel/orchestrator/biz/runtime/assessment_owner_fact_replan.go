package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	runtimeCWIReopenReasonOwnerFactPostcondition      = "assessment_owner_fact_postcondition_unmet"
	runtimeCWIReopenReasonOwnerFactAcquisitionSettled = "assessment_owner_fact_acquisition_settled"
	runtimeOwnerFactAcquisitionAttemptSignaturePrefix = "owner_fact_acquisition_attempt/"
)

// currentRuntimeSemanticOwnerFactAcquisition identifies the intermediate
// obligation sealed for the current binding. The immutable Definition proof
// remains in DefinitionFocuses; this value only describes the current semantic
// generation and therefore cannot grant terminal coverage.
//
// A durable receipt may retire the semantic generation before its assessment
// is observed. In that case CurrentBindingRef remains the exact proof identity
// for the already-accepted evidence, while the manifest moves to immutable
// semantic history. A retired manifest is consulted here only to recover that
// binding's purpose lineage. It is never returned as candidate, dispatch, or
// operation authority.
func currentRuntimeSemanticOwnerFactAcquisition(aggregate RuntimeAggregate) (runtimecontract.RuntimeSemanticOwnerFactAcquisition, bool, error) {
	ledger := aggregate.SemanticPlanning.Normalized()
	focus := aggregate.Facts.Focus.Normalized()
	if len(focus.CoverageWorkItemIDs) != 1 {
		return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, fmt.Errorf("%w: assessment purpose resolution requires one current coverage item", common.ErrConflict)
	}
	coverageID := focus.CoverageWorkItemIDs[0]
	index := runtimeCWICoverageIndex(aggregate.Facts.Coverage, coverageID)
	if index < 0 || aggregate.Facts.Coverage[index].CurrentBindingRef == nil {
		return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, fmt.Errorf("%w: current assessment CWI has no binding purpose lineage", common.ErrConflict)
	}
	ref := aggregate.Facts.Coverage[index].CurrentBindingRef.Clone()
	if err := ref.Validate(); err != nil {
		return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, fmt.Errorf("%w: current assessment CWI binding: %v", common.ErrConflict, err)
	}
	if ref.FocusID != focus.ID {
		return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, fmt.Errorf("%w: current assessment CWI binding focus does not match current focus", common.ErrConflict)
	}
	binding, bindingFound := coverageEvidenceBindingForCoverage(focus, coverageID)
	if !bindingFound || binding.ID != ref.BindingID || binding.Hash != ref.BindingHash || binding.FocusID != ref.FocusID {
		return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, fmt.Errorf("%w: current assessment CWI binding identity conflicts with the sealed focus binding", common.ErrConflict)
	}

	var (
		resolvedPurpose runtimecontract.RuntimeSemanticEvidenceObligationPurpose
		acquisition     runtimecontract.RuntimeSemanticOwnerFactAcquisition
		manifestMatches int
	)
	resolveManifest := func(manifest runtimecontract.RuntimeSemanticPlanManifest) error {
		manifest = manifest.Normalized()
		if manifest.Hash != ref.SemanticManifestHash {
			return nil
		}
		if err := manifest.Validate(); err != nil {
			return fmt.Errorf("%w: current assessment binding manifest is invalid: %v", common.ErrConflict, err)
		}
		var matched *runtimecontract.RuntimeSemanticProofObligation
		for obligationIndex := range manifest.ProofObligations {
			obligation := manifest.ProofObligations[obligationIndex].Clone()
			if obligation.CoverageWorkItemID != coverageID {
				continue
			}
			if matched != nil {
				return fmt.Errorf("%w: current assessment binding manifest has conflicting CWI proof obligations", common.ErrConflict)
			}
			matched = &obligation
		}
		if matched == nil {
			return fmt.Errorf("%w: current assessment binding manifest has no CWI proof obligation", common.ErrConflict)
		}
		evidenceObligation, err := runtimecontract.ResolveRuntimeSemanticEvidenceObligation(*matched)
		if err != nil {
			return fmt.Errorf("%w: current assessment binding evidence obligation: %v", common.ErrConflict, err)
		}
		if manifestMatches != 0 && resolvedPurpose != evidenceObligation.Purpose {
			return fmt.Errorf("%w: current assessment binding manifest purpose conflicts across semantic lineage", common.ErrConflict)
		}
		manifestMatches++
		resolvedPurpose = evidenceObligation.Purpose
		if evidenceObligation.Purpose == runtimecontract.RuntimeSemanticEvidenceObligationOwnerFactAcquisition {
			if matched.OwnerFactAcquisition == nil {
				return fmt.Errorf("%w: semantic owner-fact acquisition has no acquisition contract", common.ErrConflict)
			}
			acquisition = matched.OwnerFactAcquisition.Clone()
		}
		return nil
	}

	// Only a currently sealed manifest may contribute live purpose lineage.
	// Other active phases are deliberately ignored even if a rollback happens
	// to reproduce the same manifest hash.
	if ledger.Phase == runtimecontract.RuntimeSemanticPlanningSealed && ledger.Manifest != nil {
		if err := resolveManifest(*ledger.Manifest); err != nil {
			return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, err
		}
	}
	for _, generation := range ledger.History {
		if generation.Snapshot.Manifest == nil || strings.TrimSpace(generation.Snapshot.Manifest.Hash) != ref.SemanticManifestHash {
			continue
		}
		if err := generation.Validate(); err != nil {
			return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, fmt.Errorf("%w: current assessment binding retired generation is invalid: %v", common.ErrConflict, err)
		}
		if generation.Snapshot.Phase != runtimecontract.RuntimeSemanticPlanningSealed {
			return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, fmt.Errorf("%w: current assessment binding retired generation was not sealed", common.ErrConflict)
		}
		if err := resolveManifest(*generation.Snapshot.Manifest); err != nil {
			return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, err
		}
	}
	if manifestMatches == 0 {
		return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, fmt.Errorf("%w: current assessment CWI binding manifest is absent from sealed semantic lineage", common.ErrConflict)
	}
	switch resolvedPurpose {
	case runtimecontract.RuntimeSemanticEvidenceObligationDefinitionProof:
		return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, nil
	case runtimecontract.RuntimeSemanticEvidenceObligationOwnerFactAcquisition:
		if err := acquisition.Validate(); err != nil {
			return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, fmt.Errorf("%w: current assessment acquisition purpose: %v", common.ErrConflict, err)
		}
		return acquisition, true, nil
	default:
		return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, fmt.Errorf("%w: current assessment binding has unsupported evidence purpose %q", common.ErrConflict, resolvedPurpose)
	}
}

// retainRuntimeOwnerFactAcquisitionCoverageOpen prevents current-generation
// state-change evidence from being misclassified as terminal Definition proof.
// Evidence and assessment lineage remain attached to the row; only terminal
// credit is withheld until the successor generation proves the original kinds.
func retainRuntimeOwnerFactAcquisitionCoverageOpen(coverage []runtimecontract.RuntimeCoverageItemFact, focus runtimecontract.CoverageFocus) ([]runtimecontract.RuntimeCoverageItemFact, error) {
	focus = focus.Normalized()
	if len(focus.CoverageWorkItemIDs) != 1 {
		return nil, fmt.Errorf("%w: owner-fact acquisition settlement requires one current coverage item", common.ErrConflict)
	}
	coverageID := focus.CoverageWorkItemIDs[0]
	ret := append([]runtimecontract.RuntimeCoverageItemFact(nil), coverage...)
	found := false
	for index := range ret {
		if ret[index].ID != coverageID {
			continue
		}
		found = true
		if ret[index].State != runtimecontract.RuntimeCoverageItemStateSatisfied || ret[index].CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound {
			return nil, fmt.Errorf("%w: owner-fact acquisition did not establish its current bound proof", common.ErrConflict)
		}
		ret[index].State = runtimecontract.RuntimeCoverageItemStateOpen
	}
	if !found {
		return nil, fmt.Errorf("%w: owner-fact acquisition references unknown coverage %q", common.ErrConflict, coverageID)
	}
	return runtimecontract.NormalizeRuntimeCoverageFacts(ret), nil
}

// settleRuntimeOwnerFactAcquisition is an SM-2-local transition invoked only
// by the owner-private AssessmentObserved route. It records the one bounded
// acquisition attempt, retires that semantic generation, and reopens the same
// CWI for its still-immutable terminal Definition proof.
func settleRuntimeOwnerFactAcquisition(aggregate *RuntimeAggregate, assessmentID string, acquisition runtimecontract.RuntimeSemanticOwnerFactAcquisition) (bool, error) {
	if aggregate == nil || strings.TrimSpace(assessmentID) == "" {
		return false, common.ErrInvalidArgument
	}
	acquisition = acquisition.Normalized()
	if err := acquisition.Validate(); err != nil {
		return false, err
	}
	if aggregate.ActiveDispatch != nil {
		return false, fmt.Errorf("%w: owner-fact acquisition settlement cannot retain an active dispatch", common.ErrConflict)
	}
	if !canonicalAssessmentProofCompleteWithoutOwnerFacts(*aggregate) ||
		len(runtimecontract.RuntimeMissingOwnerFactRequirements(aggregate.Facts.Focus.RequiredOwnerFacts, aggregate.Facts.Resources.OperationAdmissibilities)) != 0 {
		return false, fmt.Errorf("%w: owner-fact acquisition settlement requires proof and the re-observed owner fact", common.ErrConflict)
	}
	if runtimeOwnerFactAcquisitionAttemptRecorded(aggregate.Facts, acquisition.Requirement, acquisition.AcquisitionOperationKind) {
		return false, fmt.Errorf("%w: owner-fact acquisition attempt was already consumed", common.ErrConflict)
	}
	carriedRepair, recorded := recordAcceptedOwnerFactAcquisitionShapes(*aggregate, aggregate.Facts.Repair, acquisition)
	if !recorded {
		return false, fmt.Errorf("%w: owner-fact acquisition settlement has no accepted acquisition operation", common.ErrConflict)
	}
	return reopenRuntimeCWIForOwnerFactAssessment(aggregate, assessmentID, runtimeCWIReopenReasonOwnerFactAcquisitionSettled, carriedRepair)
}

func recordAcceptedOwnerFactAcquisitionShapes(aggregate RuntimeAggregate, ledger runtimecontract.RuntimeRepairLedger, acquisition runtimecontract.RuntimeSemanticOwnerFactAcquisition) (runtimecontract.RuntimeRepairLedger, bool) {
	recorded := false
	signature := runtimeOwnerFactAcquisitionFailureSignature(acquisition.Requirement)
	for _, evidence := range aggregate.Facts.Evidence.Accepted {
		if evidence.Kind != runtimecontract.EvidenceKindStateChange ||
			!runtimeEvidenceAppliesToCurrentCoverageWithLineage(aggregate.Facts, evidence, aggregate.CoverageProofLineage) {
			continue
		}
		operation, found := runtimeOperationForEvidence(aggregate, evidence)
		if !found || runtimecontract.CanonicalRuntimeBlockKind(operation.Kind) != acquisition.AcquisitionOperationKind {
			continue
		}
		ledger = recordNoProgressOperationShape(ledger, runtimecontract.RuntimeRejectedOperationShape{
			Operation:        operation,
			EvidenceID:       evidence.ID,
			FailureClass:     runtimecontract.FailureClassCoverageAssessmentIncomplete,
			FailureCode:      runtimeCWIReopenReasonOwnerFactAcquisitionSettled,
			FailureSignature: signature,
			FailureMessage:   "accepted operation settled the bounded owner-fact acquisition but is not terminal Definition proof",
		})
		recorded = true
	}
	return ledger, recorded
}

// replanRuntimeCWIForUnmetOwnerFacts is an SM-2-local transition after an
// assessment dispatch has been committed. The capability owner has already
// published its current catalog; this reducer only records that the sealed
// proof did not establish the profile-declared postcondition and reopens the
// same CWI for a new semantic generation. No capability or Run state is
// mutated here.
func replanRuntimeCWIForUnmetOwnerFacts(aggregate *RuntimeAggregate, assessmentID string) (bool, error) {
	if aggregate == nil || strings.TrimSpace(assessmentID) == "" {
		return false, common.ErrInvalidArgument
	}
	if aggregate.ActiveDispatch != nil {
		return false, fmt.Errorf("%w: owner-fact CWI replan cannot retain an active dispatch", common.ErrConflict)
	}
	facts := aggregate.Facts.Normalized()
	if err := requireCurrentRuntimeCWILedger(facts); err != nil {
		return false, err
	}
	if !canonicalAssessmentProofCompleteWithoutOwnerFacts(*aggregate) {
		return false, nil
	}
	if len(runtimecontract.RuntimeMissingOwnerFactRequirements(facts.Focus.RequiredOwnerFacts, facts.Resources.OperationAdmissibilities)) == 0 {
		return false, nil
	}
	acquisition, acquisitionReady := runtimecontract.ResolveRuntimeOwnerFactAcquisition(facts.Focus.RequiredOwnerFacts, facts.Focus.OwnerFactConvergences, facts.Resources.OperationAdmissibilities)
	if !acquisitionReady || runtimeOwnerFactReplanBudgetExhausted(facts, acquisition) {
		return false, nil
	}
	// Account the operation that produced the just-committed assessment before
	// deciding whether another semantic generation is authorized. The sealed
	// max is an acquisition-attempt bound, not a count of provider replans.
	carriedRepair := recordAcceptedNoProgressShapes(*aggregate, facts.Repair, runtimeOwnerFactAcquisitionFailureSignature(acquisition.Requirement))
	factsWithCurrentAttempt := facts
	factsWithCurrentAttempt.Repair = carriedRepair
	if runtimeOwnerFactReplanBudgetExhausted(factsWithCurrentAttempt, acquisition) {
		aggregate.Facts.Repair = carriedRepair
		return false, nil
	}
	return reopenRuntimeCWIForOwnerFactAssessment(aggregate, assessmentID, runtimeCWIReopenReasonOwnerFactPostcondition, carriedRepair)
}

func reopenRuntimeCWIForOwnerFactAssessment(aggregate *RuntimeAggregate, assessmentID, reason string, carriedRepair runtimecontract.RuntimeRepairLedger) (bool, error) {
	facts := aggregate.Facts.Normalized()
	if len(facts.Focus.CoverageWorkItemIDs) != 1 {
		return false, fmt.Errorf("%w: owner-fact CWI replan requires one current coverage item", common.ErrConflict)
	}
	coverageID := facts.Focus.CoverageWorkItemIDs[0]
	index := runtimeCWICoverageIndex(facts.Coverage, coverageID)
	if index < 0 {
		return false, fmt.Errorf("%w: owner-fact CWI replan references unknown coverage %q", common.ErrConflict, coverageID)
	}
	item := &facts.Coverage[index]
	if item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound || item.CurrentBindingRef == nil ||
		item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
		return false, fmt.Errorf("%w: owner-fact CWI replan requires a current unsatisfied binding", common.ErrConflict)
	}
	lineage := runtimecontract.RuntimeCWIReopenLineage{
		PreviousBindingRef: item.CurrentBindingRef.Clone(),
		TriggerRef:         strings.TrimSpace(assessmentID),
		Reason:             strings.TrimSpace(reason),
	}.Normalized()
	if err := lineage.Validate(); err != nil {
		return false, err
	}
	if len(item.ReopenLineage) >= runtimecontract.RuntimeCWIReopenLineageLimit {
		return false, fmt.Errorf("runtime CWI reopen lineage exceeds bounded limit %d", runtimecontract.RuntimeCWIReopenLineageLimit)
	}
	// The owner-fact postcondition proves that an accepted state-change shape did
	// not close this CWI. Preserve only those exact shapes across the successor
	// semantic focus; attempts, gap text and directives remain focus-local and
	// are reset by the canonical projection transition.
	item.ReopenLineage = append(item.ReopenLineage, lineage)
	item.CurrentBindingRef = nil
	item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateReopened
	item.State = runtimecontract.RuntimeCoverageItemStateOpen

	updated, err := replaceRuntimeCWIFocusProjection(facts, []common.CoverageWorkItemID{coverageID})
	if err != nil {
		return false, err
	}
	updated.Repair.FocusID = updated.Focus.ID
	updated.Repair.SliceID = updated.Focus.SliceID
	updated.Repair.NoProgressOperationShapes = append([]runtimecontract.RuntimeRejectedOperationShape(nil), carriedRepair.NoProgressOperationShapes...)
	updated.Repair = updated.Repair.Normalized()
	contract, err := canonicalRuntimeCWIReconciliationContract(updated)
	if err != nil {
		return false, err
	}
	updated.ReconciliationContract = contract
	aggregate.Facts = updated
	if _, err := supersedeActiveReconciliationForCurrentContract(aggregate); err != nil {
		return false, err
	}
	if err := retireSemanticPlanningGenerationForCWIReopen(aggregate, []common.CoverageWorkItemID{coverageID}); err != nil {
		return false, err
	}
	return true, nil
}

func runtimeOwnerFactReplanBudgetExhausted(facts runtimecontract.RuntimeFacts, acquisition runtimecontract.RuntimeOwnerFactConvergence) bool {
	facts = facts.Normalized()
	acquisition = acquisition.Normalized()
	if acquisition.Validate() != nil || acquisition.RequirePreexisting || acquisition.MaxAcquisitionAttempts != 1 || len(facts.Focus.CoverageWorkItemIDs) != 1 {
		return true
	}
	return runtimeOwnerFactAcquisitionAttemptRecorded(facts, acquisition.Requirement, acquisition.AcquisitionOperationKind)
}

func runtimeOwnerFactAcquisitionAttemptRecorded(facts runtimecontract.RuntimeFacts, requirement runtimecontract.RuntimeOwnerFactRequirement, operationKind string) bool {
	facts = facts.Normalized()
	signature := runtimeOwnerFactAcquisitionFailureSignature(requirement)
	operationKind = runtimecontract.CanonicalRuntimeBlockKind(operationKind)
	for _, shape := range facts.Repair.NoProgressOperationShapes {
		if runtimecontract.CanonicalRuntimeBlockKind(shape.Operation.Kind) == operationKind && strings.TrimSpace(shape.FailureSignature) == signature {
			return true
		}
	}
	return false
}

func runtimeOwnerFactAcquisitionFailureSignature(requirement runtimecontract.RuntimeOwnerFactRequirement) string {
	return runtimeOwnerFactAcquisitionAttemptSignaturePrefix + runtimecontract.RuntimeOwnerFactRequirementRef(requirement)
}
