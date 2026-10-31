package runtime

import (
	"fmt"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"slices"
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
	rounds := 0
	for _, r := range a.ImplementationWork.Rounds {
		if r.CoverageWorkItemID == v.Binding.CoverageWorkItemID {
			if r.CompletedReceiptSetHash == "" || r.FailedReceiptHash != "" {
				return a, false, nil
			}
			rounds++
		}
	}
	if rounds >= policy.MaxRoundsPerCWI {
		return a, false, nil
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
	snapshot := ledger.WithoutHistory().Clone()
	g := runtimecontract.RuntimeSemanticPlanningGeneration{Reason: runtimecontract.RuntimeSemanticPlanningRetirementInputRevision, RetiredAtStateRevision: a.StateRevision + 1, Snapshot: snapshot}
	if err := g.Validate(); err != nil {
		return a, false, err
	}
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
