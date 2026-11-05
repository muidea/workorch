package runtime

import (
	"fmt"
	"slices"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Only the SM-2 private review-classification event may retire this report.
// It reserves no effect, creates no tool failure and grants no CWI credit.
func retireRuntimeInputReviewForRevision(a RuntimeAggregate) (RuntimeAggregate, bool, error) {
	v, err := runtimecontract.CompileRuntimeValidationInputRevision(a.SemanticPlanning)
	if err != nil {
		return a, false, nil
	} // Unsupported/ambiguous review retains original explicit failure.
	var policy *runtimecontract.RuntimeImplementationWorkPolicy
	for _, f := range a.Facts.DefinitionFocuses {
		if len(f.CoverageWorkItemIDs) == 1 && f.CoverageWorkItemIDs[0] == v.Binding.CoverageWorkItemID {
			policy = f.ImplementationWorkPolicy
		}
	}
	if policy == nil || policy.InputRevisionProtocolRef != runtimecontract.RuntimeValidationInputRevisionProtocolV1 || policy.OwnerModuleID != v.OwnerModuleID {
		return a, false, nil
	}
	if a.ActiveDispatch != nil || a.Facts.Failure != nil || a.Facts.RepairDirective != nil || a.ImplementationWork.ScopePreview != nil || a.Termination != nil || len(a.Facts.Focus.CoverageWorkItemIDs) != 1 || a.Facts.Focus.CoverageWorkItemIDs[0] != v.Binding.CoverageWorkItemID || !slices.Contains(runtimecontract.RuntimeCWIsEligibleForPlanning(a.Facts), v.Binding.CoverageWorkItemID) {
		return a, false, nil
	}
	for _, r := range a.ImplementationWork.Rounds {
		if r.CoverageWorkItemID == v.Binding.CoverageWorkItemID {
			if r.CompletedReceiptSetHash == "" || r.FailedReceiptHash != "" {
				return a, false, nil
			}
		}
	}
	if pending, e := runtimecontract.PendingRuntimeValidationInputRevision(a.SemanticPlanning, a.ImplementationWork, v.Binding.CoverageWorkItemID); e != nil || pending != nil {
		return a, false, nil
	}
	for _, session := range a.CandidateRequestSessions {
		if session.ProviderSubmitted && session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete && session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal {
			return a, false, nil
		}
	}
	ledger := a.SemanticPlanning
	snapshot := ledger.WithoutHistory().CloneRaw()
	g := runtimecontract.RuntimeSemanticPlanningGeneration{Reason: runtimecontract.RuntimeSemanticPlanningRetirementInputRevision, RetiredAtStateRevision: a.StateRevision + 1, Snapshot: snapshot}
	if err := retireSemanticPlanningRequestSessions(&a, ledger, runtimecontract.RuntimeValidationInputRevisionRetirementOutcome); err != nil {
		return a, false, err
	}
	a.SemanticPlanning = runtimecontract.RuntimeSemanticPlanningLedger{SchemaVersion: ledger.SchemaVersion, Phase: runtimecontract.RuntimeSemanticPlanningRequired, DefinitionSeed: ledger.DefinitionSeed.Clone(), CatalogRefreshAttempts: ledger.CatalogRefreshAttempts, FeasibilityGaps: ledger.FeasibilityGaps, History: ledger.History, HistoryArchive: ledger.HistoryArchive}
	if appended, err := a.SemanticPlanning.AppendHistory(g); err != nil {
		return a, false, err
	} else {
		a.SemanticPlanning = appended
	}
	facts, decision, err := reduceRuntimeAggregateWithDirective(a, a.Facts, &a.Decision.AuthoritySnapshot)
	if err != nil {
		return a, false, fmt.Errorf("reduce input revision: %w", err)
	}
	a.Facts, a.Decision = facts, decision
	return a, true, nil
}

// The prerequisite-observed private SM-2 event may retain a structural input
// source in the same maintenance history. No semantic report/request is invented.
func retainRuntimeOwnerInputRevision(a RuntimeAggregate) (RuntimeAggregate, bool, error) {
	if len(a.Facts.Prerequisites.Pending) != 0 || a.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || a.ActiveDispatch != nil || a.Termination != nil || a.Facts.Failure != nil || a.Facts.RepairDirective != nil || a.ImplementationWork.ScopePreview != nil {
		return a, false, nil
	}
	v, err := runtimecontract.CompileRuntimeOwnerInputRevision(a.Facts)
	if err != nil {
		return a, false, nil
	}
	permitted := false
	for _, f := range a.Facts.DefinitionFocuses {
		if len(f.CoverageWorkItemIDs) == 1 && f.CoverageWorkItemIDs[0] == v.Binding.CoverageWorkItemID && f.ImplementationWorkPolicy != nil {
			p := f.ImplementationWorkPolicy
			permitted = p.InputRevisionProtocolRef == runtimecontract.RuntimeValidationInputRevisionProtocolV1 && p.OwnerModuleID == v.OwnerModuleID
		}
	}
	if !runtimeOwnerInputRevisionCoversUnavailable(a.Facts, v) || !permitted || !slices.Contains(runtimecontract.RuntimeCWIsEligibleForPlanning(a.Facts), v.Binding.CoverageWorkItemID) {
		return a, false, nil
	}
	for _, r := range a.ImplementationWork.Rounds {
		if r.CoverageWorkItemID == v.Binding.CoverageWorkItemID && (r.CompletedReceiptSetHash == "" || r.FailedReceiptHash != "") {
			return a, false, nil
		}
	}
	for _, session := range a.CandidateRequestSessions {
		if session.ProviderSubmitted && session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete && session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal {
			return a, false, nil
		}
	}
	if pending, e := runtimecontract.PendingRuntimeValidationInputRevision(a.SemanticPlanning, a.ImplementationWork, v.Binding.CoverageWorkItemID); e != nil || pending != nil {
		return a, false, nil
	}
	for _, g := range a.SemanticPlanning.AllHistory() {
		if g.Reason != runtimecontract.RuntimeSemanticPlanningRetirementInputRevision {
			continue
		}
		previous, e := runtimecontract.CompileRuntimeValidationInputRevisionGeneration(g)
		if e != nil {
			return a, false, e
		}
		if runtimecontract.RuntimeInputRevisionSamePhysicalState(previous, v) {
			return a, false, nil
		}
	}
	g := runtimecontract.RuntimeSemanticPlanningGeneration{Reason: runtimecontract.RuntimeSemanticPlanningRetirementInputRevision, RetiredAtStateRevision: a.StateRevision + 1, OwnerInputRevision: v, Snapshot: a.SemanticPlanning.WithoutHistory().CloneRaw()}
	ledger, err := a.SemanticPlanning.AppendHistory(g)
	if err != nil {
		return a, false, err
	}
	a.SemanticPlanning = ledger
	return a, true, nil
}

func pendingRuntimeOwnerInputRevision(a *RuntimeAggregate, facts runtimecontract.RuntimeFacts) bool {
	if a == nil || len(facts.Focus.CoverageWorkItemIDs) != 1 {
		return false
	}
	v, err := runtimecontract.PendingRuntimeValidationInputRevision(a.SemanticPlanning, a.ImplementationWork, facts.Focus.CoverageWorkItemIDs[0])
	if err != nil || v == nil || v.OwnerObservation == nil {
		return false
	}
	current, err := runtimecontract.CompileRuntimeOwnerInputRevision(facts)
	return err == nil && runtimeOwnerInputRevisionCoversUnavailable(facts, v) && runtimecontract.RuntimeInputRevisionSamePhysicalState(current, v) && current.OwnerObservation.Observation.ObservationID == v.OwnerObservation.Observation.ObservationID
}

// A maintenance source cannot hide another genuine unavailable obligation.
func runtimeOwnerInputRevisionCoversUnavailable(f runtimecontract.RuntimeFacts, v *runtimecontract.RuntimeValidationInputRevision) bool {
	covered := false
	for _, r := range runtimecontract.RuntimeMissingOwnerFactRequirements(currentFocusOwnerFactRequirements(f), f.Resources.OperationAdmissibilities) {
		if runtimecontract.RuntimeOwnerFactHasUnavailableValidationProof(r, f.Resources.OperationAdmissibilities) {
			if v == nil || v.OwnerObservation == nil || r.OwnerModuleID != v.OwnerObservation.Catalog.OwnerModuleID || r.ValidationProofRequirementHash != v.Binding.RequirementHash {
				return false
			}
			covered = true
		}
	}
	return covered
}
