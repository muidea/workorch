package runtime

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeCWIReopenTrigger is an Orchestrator-private reduction input. It is
// created only after an accepted receipt or a typed reconciliation batch has
// passed its own owner contract checks.
type runtimeCWIReopenTrigger struct {
	CoverageWorkItemID common.CoverageWorkItemID
	TriggerRef         string
	Reason             string
}

func requireCurrentRuntimeCWILedger(facts runtimecontract.RuntimeFacts) error {
	if facts.CWILedgerSchemaVersion != runtimecontract.RuntimeCWILedgerSchemaCurrent {
		return fmt.Errorf("runtime CWI transition requires the current ledger schema")
	}
	return nil
}

func validateCurrentCWIBindingFence(facts runtimecontract.RuntimeFacts) error {
	return validateCurrentCWIBindingFenceForStates(facts, runtimecontract.RuntimeCWIConvergenceStateBound)
}

// validateCurrentCWIBindingFenceForEvidenceCommit accepts a CWI that was
// already durably settled by an earlier receipt in the same active dispatch.
// SM-3 must still commit the remaining sealed operations before SM-2 advances
// the focus projection. Candidate request and dispatch seal intentionally use
// the stricter bound-only fence above.
func validateCurrentCWIBindingFenceForEvidenceCommit(facts runtimecontract.RuntimeFacts) error {
	return validateCurrentCWIBindingFenceForStates(
		facts,
		runtimecontract.RuntimeCWIConvergenceStateBound,
		runtimecontract.RuntimeCWIConvergenceStateSettled,
	)
}

func validateCurrentCWIBindingFenceForStates(facts runtimecontract.RuntimeFacts, allowed ...runtimecontract.RuntimeCWIConvergenceState) error {
	allowedStates := make(map[runtimecontract.RuntimeCWIConvergenceState]struct{}, len(allowed))
	for _, state := range allowed {
		allowedStates[state] = struct{}{}
	}
	for _, coverageID := range facts.Focus.Normalized().CoverageWorkItemIDs {
		item, found := currentRuntimeCWIBindingForFocus(facts, coverageID)
		if !found {
			return fmt.Errorf("runtime CWI %q has no current bound binding", coverageID)
		}
		if _, permitted := allowedStates[item.CWIConvergenceState]; !permitted {
			return fmt.Errorf("runtime CWI %q has no current allowed binding state", coverageID)
		}
	}
	return nil
}

func runtimeCoverageByID(values []runtimecontract.RuntimeCoverageItemFact) map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItemFact {
	ret := make(map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItemFact, len(values))
	for _, item := range values {
		if item.ID != "" {
			ret[item.ID] = item
		}
	}
	return ret
}

func coverageBindingForRuntimeCWI(focus runtimecontract.CoverageFocus, coverageID common.CoverageWorkItemID) (runtimecontract.CoverageEvidenceBinding, bool) {
	for _, binding := range focus.Normalized().CoverageEvidenceBindings {
		if binding.CoverageWorkItemID == coverageID {
			return binding.Clone(), true
		}
	}
	return runtimecontract.CoverageEvidenceBinding{}, false
}

// currentRuntimeCWIBindingForFocus is the one read-only canonical binding
// fence shared by SM-3 admission and SM-2 evidence attribution. It only reads
// the persisted Orchestrator aggregate; it never derives or repairs a binding.
func currentRuntimeCWIBindingForFocus(facts runtimecontract.RuntimeFacts, coverageID common.CoverageWorkItemID) (runtimecontract.RuntimeCoverageItemFact, bool) {
	facts = facts.Normalized()
	coverageID = common.CoverageWorkItemID(strings.TrimSpace(string(coverageID)))
	if facts.CWILedgerSchemaVersion != runtimecontract.RuntimeCWILedgerSchemaCurrent || coverageID == "" {
		return runtimecontract.RuntimeCoverageItemFact{}, false
	}
	item, found := runtimeCoverageByID(facts.Coverage)[coverageID]
	if !found || item.CurrentBindingRef == nil {
		return runtimecontract.RuntimeCoverageItemFact{}, false
	}
	current := item.CurrentBindingRef.Normalized()
	if err := current.Validate(); err != nil {
		return runtimecontract.RuntimeCoverageItemFact{}, false
	}
	binding, found := coverageBindingForRuntimeCWI(facts.Focus, coverageID)
	if !found {
		return runtimecontract.RuntimeCoverageItemFact{}, false
	}
	wanted := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(binding, current.SemanticManifestHash)
	if !runtimecontract.SameRuntimeCWIBindingRef(current, wanted) {
		return runtimecontract.RuntimeCoverageItemFact{}, false
	}
	return item, true
}

func runtimeCWIPendingFocus(source runtimecontract.CoverageFocus, coverageID common.CoverageWorkItemID) runtimecontract.CoverageFocus {
	value := source.Normalized()
	value.ID = "semantic-pending/" + string(coverageID)
	value.CoverageWorkItemIDs = []common.CoverageWorkItemID{coverageID}
	value.CoverageEvidenceBindings = nil
	return value.Normalized()
}

func runtimeCWIWithCoverage(focus runtimecontract.CoverageFocus, coverageIDs []common.CoverageWorkItemID) runtimecontract.CoverageFocus {
	allowed := make(map[common.CoverageWorkItemID]struct{}, len(coverageIDs))
	for _, coverageID := range coverageIDs {
		allowed[coverageID] = struct{}{}
	}
	value := focus.Normalized()
	value.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), coverageIDs...)
	bindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(value.CoverageEvidenceBindings))
	for _, binding := range value.CoverageEvidenceBindings {
		if _, found := allowed[binding.CoverageWorkItemID]; found {
			bindings = append(bindings, binding.Clone())
		}
	}
	value.CoverageEvidenceBindings = bindings
	return value.Normalized()
}

// replaceRuntimeCWIFocusProjection replaces exact CWI queue members by their
// stable pending projection. The local maps are invocation-scoped only; the
// persisted authority remains the Coverage row and the focus queue.
func replaceRuntimeCWIFocusProjection(facts runtimecontract.RuntimeFacts, replacements []common.CoverageWorkItemID) (runtimecontract.RuntimeFacts, error) {
	if len(replacements) == 0 {
		return facts, nil
	}
	wanted := make(map[common.CoverageWorkItemID]struct{}, len(replacements))
	for _, coverageID := range replacements {
		if coverageID != "" {
			wanted[coverageID] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return facts, nil
	}

	split := func(focus runtimecontract.CoverageFocus) ([]runtimecontract.CoverageFocus, []runtimecontract.CoverageFocus) {
		focus = focus.Normalized()
		retainedIDs := make([]common.CoverageWorkItemID, 0, len(focus.CoverageWorkItemIDs))
		pending := make([]runtimecontract.CoverageFocus, 0)
		for _, coverageID := range focus.CoverageWorkItemIDs {
			if _, replace := wanted[coverageID]; replace {
				pending = append(pending, runtimeCWIPendingFocus(focus, coverageID))
				continue
			}
			retainedIDs = append(retainedIDs, coverageID)
		}
		retained := make([]runtimecontract.CoverageFocus, 0, 1)
		if len(retainedIDs) != 0 {
			retained = append(retained, runtimeCWIWithCoverage(focus, retainedIDs))
		}
		return retained, pending
	}

	currentRetained, currentPending := split(facts.Focus)
	if len(currentRetained) == 0 && len(currentPending) == 0 {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI focus projection has no current focus")
	}
	currentQueue := append(currentRetained, currentPending...)
	if len(currentQueue) == 0 {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI focus projection has no current queue")
	}

	remaining := make([]runtimecontract.CoverageFocus, 0, len(currentQueue)-1+len(facts.RemainingFocuses))
	remaining = append(remaining, currentQueue[1:]...)
	for _, focus := range facts.RemainingFocuses {
		retained, pending := split(focus)
		remaining = append(remaining, retained...)
		remaining = append(remaining, pending...)
	}

	completed := make([]runtimecontract.CoverageFocus, 0, len(facts.CompletedFocuses))
	requeued := make([]runtimecontract.CoverageFocus, 0)
	for _, focus := range facts.CompletedFocuses {
		retained, pending := split(focus)
		completed = append(completed, retained...)
		requeued = append(requeued, pending...)
	}
	// Completed CWI become executable again before the already pending queue.
	remaining = append(requeued, remaining...)

	facts.Focus = currentQueue[0]
	facts.RemainingFocuses = remaining
	facts.CompletedFocuses = completed
	return projectRuntimeCWIFocusIDs(facts)
}

func projectRuntimeCWIFocusIDs(facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeFacts, error) {
	byCoverage := make(map[common.CoverageWorkItemID]string, len(facts.Coverage))
	for _, focus := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		focus = focus.Normalized()
		if focus.ID == "" {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI projection has an empty focus")
		}
		for _, coverageID := range focus.CoverageWorkItemIDs {
			if previous, duplicate := byCoverage[coverageID]; duplicate {
				return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI %q appears in focus %q and %q", coverageID, previous, focus.ID)
			}
			byCoverage[coverageID] = focus.ID
		}
	}
	for index := range facts.Coverage {
		focusID, found := byCoverage[facts.Coverage[index].ID]
		if !found {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI %q has no focus projection", facts.Coverage[index].ID)
		}
		facts.Coverage[index].FocusID = focusID
	}
	facts.Coverage = runtimecontract.NormalizeRuntimeCoverageFacts(facts.Coverage)
	// Focus projection is an SM-2 mutation path in its own right: semantic
	// sealing, binding retirement, and CWI reopen all arrive here before a
	// later reduction. Rebind repair state now so no caller can persist a new
	// focus paired with the predecessor's repair budget or directive.
	return rebindRepairForCurrentFocus(facts), nil
}

// projectRuntimeCWISealedFocuses installs a semantic batch into the existing
// CWI projection without rebuilding settled or already-bound queue members.
// The provider-authored batch order is retained only for eligible CWI; all
// other rows keep their prior owner-local projection and lineage.
func projectRuntimeCWISealedFocuses(
	facts runtimecontract.RuntimeFacts,
	sealed []runtimecontract.CoverageFocus,
	pending []runtimecontract.CoverageFocus,
) (runtimecontract.RuntimeFacts, error) {
	sealedByCoverage := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(sealed))
	pendingByCoverage := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(pending))
	ordered := make([]runtimecontract.CoverageFocus, 0, len(sealed)+len(pending))
	for _, source := range sealed {
		focus := source.Normalized()
		if len(focus.CoverageWorkItemIDs) != 1 || len(focus.CoverageEvidenceBindings) != 1 {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI sealed focus %q must contain exactly one coverage binding", focus.ID)
		}
		coverageID := focus.CoverageWorkItemIDs[0]
		binding, found := coverageBindingForRuntimeCWI(focus, coverageID)
		if !found || binding.ID == "" || binding.FocusID != focus.ID {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI sealed focus %q has no exact binding", focus.ID)
		}
		if _, duplicate := sealedByCoverage[coverageID]; duplicate {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI sealed projection duplicates coverage %q", coverageID)
		}
		sealedByCoverage[coverageID] = focus
		ordered = append(ordered, focus)
	}
	for _, source := range pending {
		focus := source.Normalized()
		if len(focus.CoverageWorkItemIDs) != 1 || len(focus.CoverageEvidenceBindings) != 0 ||
			focus.ID != "semantic-pending/"+string(focus.CoverageWorkItemIDs[0]) {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI pending focus %q is not canonical", focus.ID)
		}
		coverageID := focus.CoverageWorkItemIDs[0]
		if _, alreadySealed := sealedByCoverage[coverageID]; alreadySealed {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI projection repeats sealed coverage %q", coverageID)
		}
		if _, duplicate := pendingByCoverage[coverageID]; duplicate {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI pending projection duplicates coverage %q", coverageID)
		}
		pendingByCoverage[coverageID] = focus
		ordered = append(ordered, focus)
	}
	if len(ordered) == 0 {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI semantic projection has no eligible focus")
	}

	active := append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...)
	retained := make([]runtimecontract.CoverageFocus, 0, len(active))
	for _, source := range active {
		focus := source.Normalized()
		ids := make([]common.CoverageWorkItemID, 0, len(focus.CoverageWorkItemIDs))
		for _, coverageID := range focus.CoverageWorkItemIDs {
			if _, replace := sealedByCoverage[coverageID]; replace {
				continue
			}
			if _, replace := pendingByCoverage[coverageID]; replace {
				continue
			}
			ids = append(ids, coverageID)
		}
		if len(ids) != 0 {
			retained = append(retained, runtimeCWIWithCoverage(focus, ids))
		}
	}
	// The active CWI has to be part of the newly sealed batch. Otherwise a
	// semantic batch could silently move a different CWI ahead of the current
	// SM-2 authority cursor.
	activeCoverage := facts.Focus.Normalized().CoverageWorkItemIDs
	containsCurrent := false
	for _, coverageID := range activeCoverage {
		if _, found := sealedByCoverage[coverageID]; found {
			containsCurrent = true
			break
		}
	}
	if !containsCurrent {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI semantic batch does not bind the current focus")
	}

	facts.Focus = ordered[0]
	facts.RemainingFocuses = append(append([]runtimecontract.CoverageFocus(nil), ordered[1:]...), retained...)
	return projectRuntimeCWIFocusIDs(facts)
}

func appendRuntimeCWIBindingLineage(values []runtimecontract.RuntimeCWIBindingRef, ref runtimecontract.RuntimeCWIBindingRef) ([]runtimecontract.RuntimeCWIBindingRef, error) {
	ref = ref.Normalized()
	for _, existing := range values {
		if existing.BindingID != ref.BindingID {
			continue
		}
		if runtimecontract.SameRuntimeCWIBindingRef(existing, ref) {
			return values, nil
		}
		return nil, fmt.Errorf("runtime CWI binding lineage reuses binding ID %q with a different identity", ref.BindingID)
	}
	if len(values) >= runtimecontract.RuntimeCWIBindingLineageLimit {
		return nil, fmt.Errorf("runtime CWI binding lineage exceeds bounded limit %d", runtimecontract.RuntimeCWIBindingLineageLimit)
	}
	return append(values, ref), nil
}

// sealRuntimeCWIBindings turns one complete, owner-feasible semantic batch
// into the sole current binding for its eligible CWI rows. It does not invoke
// a capability, select an operation, or write storage.
func sealRuntimeCWIBindings(
	facts runtimecontract.RuntimeFacts,
	semanticManifestHash string,
	focuses []runtimecontract.CoverageFocus,
	pending []runtimecontract.CoverageFocus,
) (runtimecontract.RuntimeFacts, error) {
	if err := requireCurrentRuntimeCWILedger(facts); err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	semanticManifestHash = strings.TrimSpace(semanticManifestHash)
	if semanticManifestHash == "" || len(focuses) == 0 {
		return runtimecontract.RuntimeFacts{}, common.ErrInvalidArgument
	}
	byID := make(map[common.CoverageWorkItemID]int, len(facts.Coverage))
	for index, item := range facts.Coverage {
		if item.ID == "" {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI semantic seal has an empty coverage item")
		}
		byID[item.ID] = index
	}
	selected := make(map[common.CoverageWorkItemID]runtimecontract.CoverageEvidenceBinding)
	for _, focus := range focuses {
		focus = focus.Normalized()
		for _, binding := range focus.CoverageEvidenceBindings {
			binding = binding.Normalized()
			if _, found := byID[binding.CoverageWorkItemID]; !found {
				return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI semantic seal references unknown coverage %q", binding.CoverageWorkItemID)
			}
			if _, duplicate := selected[binding.CoverageWorkItemID]; duplicate {
				return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI semantic seal duplicates coverage %q", binding.CoverageWorkItemID)
			}
			if _, found := coverageBindingForRuntimeCWI(focus, binding.CoverageWorkItemID); !found {
				return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI semantic seal binding %q is outside its focus", binding.ID)
			}
			selected[binding.CoverageWorkItemID] = binding
		}
	}
	if len(selected) == 0 {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI semantic seal has no binding")
	}
	for _, coverageID := range facts.Focus.CoverageWorkItemIDs {
		item := facts.Coverage[byID[coverageID]]
		if item.CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStateUnbound || item.CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStatePlanning || item.CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStateReopened {
			if _, found := selected[coverageID]; !found {
				return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI semantic seal omits active coverage %q", coverageID)
			}
		}
	}

	for coverageID, binding := range selected {
		index := byID[coverageID]
		item := &facts.Coverage[index]
		switch item.CWIConvergenceState {
		case runtimecontract.RuntimeCWIConvergenceStateUnbound, runtimecontract.RuntimeCWIConvergenceStatePlanning, runtimecontract.RuntimeCWIConvergenceStateReopened:
		default:
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI %q cannot receive a semantic binding from state %q", coverageID, item.CWIConvergenceState)
		}
		if item.CurrentBindingRef != nil {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI %q already has a current binding", coverageID)
		}
		ref := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(binding, semanticManifestHash)
		lineage, err := appendRuntimeCWIBindingLineage(item.BindingLineage, ref)
		if err != nil {
			return runtimecontract.RuntimeFacts{}, err
		}
		item.BindingLineage = lineage
		item.CurrentBindingRef = &ref
		item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateBound
		if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			item.State = runtimecontract.RuntimeCoverageItemStateOpen
		}
	}
	for index := range facts.Coverage {
		item := &facts.Coverage[index]
		if _, sealed := selected[item.ID]; sealed || item.CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStateSettled {
			continue
		}
		if item.CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStateBound {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI %q retains an unrelated current binding", item.ID)
		}
		item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStatePlanning
		item.CurrentBindingRef = nil
	}

	return projectRuntimeCWISealedFocuses(facts, focuses, pending)
}

func canonicalRuntimeCWIReconciliationContract(facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeCWIReconciliationContract, error) {
	if err := requireCurrentRuntimeCWILedger(facts); err != nil {
		return runtimecontract.RuntimeCWIReconciliationContract{}, err
	}
	coverage := runtimeCoverageByID(facts.Coverage)
	focuses := make([]runtimecontract.CoverageFocus, 0, 1+len(facts.RemainingFocuses)+len(facts.CompletedFocuses))
	for _, source := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		source = source.Normalized()
		coverageIDs := make([]common.CoverageWorkItemID, 0, len(source.CoverageWorkItemIDs))
		bindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(source.CoverageEvidenceBindings))
		for _, coverageID := range source.CoverageWorkItemIDs {
			item, found := coverage[coverageID]
			if !found || item.CurrentBindingRef == nil {
				continue
			}
			binding, found := coverageBindingForRuntimeCWI(source, coverageID)
			if !found {
				return runtimecontract.RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime CWI %q current binding has no focus declaration", coverageID)
			}
			ref := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(binding, item.CurrentBindingRef.SemanticManifestHash)
			if !runtimecontract.SameRuntimeCWIBindingRef(*item.CurrentBindingRef, ref) {
				return runtimecontract.RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime CWI %q focus binding differs from its canonical binding", coverageID)
			}
			coverageIDs = append(coverageIDs, coverageID)
			bindings = append(bindings, binding)
		}
		if len(coverageIDs) == 0 {
			continue
		}
		source.CoverageWorkItemIDs = coverageIDs
		source.CoverageEvidenceBindings = bindings
		focuses = append(focuses, source)
	}
	if len(focuses) == 0 {
		return runtimecontract.RuntimeCWIReconciliationContract{}, nil
	}
	return runtimecontract.CompileRuntimeCWIReconciliationContract(facts.DefinitionRevision, focuses)
}

func settleRuntimeCWIsForCurrentFocus(facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeFacts, error) {
	if err := requireCurrentRuntimeCWILedger(facts); err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	if err := validateCurrentCWIBindingFenceForEvidenceCommit(facts); err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	current := make(map[common.CoverageWorkItemID]struct{}, len(facts.Focus.CoverageWorkItemIDs))
	for _, coverageID := range facts.Focus.CoverageWorkItemIDs {
		current[coverageID] = struct{}{}
	}
	for index := range facts.Coverage {
		item := &facts.Coverage[index]
		if _, found := current[item.ID]; !found || item.State != runtimecontract.RuntimeCoverageItemStateSatisfied {
			continue
		}
		if item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound && item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateSettled {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime CWI %q cannot settle from state %q", item.ID, item.CWIConvergenceState)
		}
		item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateSettled
	}
	return facts.Normalized(), nil
}

// advanceRuntimeCWIFocus is the sole production wrapper around the focus-queue
// helper. Current-schema aggregates must first prove that all current CWI have
// exact binding-matched credit, then mark those rows settled, before moving the
// focus projection.
func advanceRuntimeCWIFocus(facts runtimecontract.RuntimeFacts, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) (runtimecontract.RuntimeFacts, error) {
	if err := requireCurrentRuntimeCWILedger(facts); err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	if len(missingEvidenceKinds(facts, proofLineages...)) != 0 || !currentFocusCoverageSatisfied(facts, proofLineages...) {
		return facts, nil
	}
	settled, err := settleRuntimeCWIsForCurrentFocus(facts)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	if err := runtimecontract.ValidateRuntimeCWILedger(settled); err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	advanced := advanceSatisfiedCoverageFocus(settled, proofLineages...)
	if err := runtimecontract.ValidateRuntimeCWILedger(advanced); err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	return advanced, nil
}

func reopenRuntimeCWIs(facts runtimecontract.RuntimeFacts, triggers []runtimeCWIReopenTrigger) (runtimecontract.RuntimeFacts, []common.CoverageWorkItemID, error) {
	if err := requireCurrentRuntimeCWILedger(facts); err != nil {
		return runtimecontract.RuntimeFacts{}, nil, err
	}
	if len(triggers) == 0 {
		return facts, nil, nil
	}
	sort.SliceStable(triggers, func(i, j int) bool {
		return string(triggers[i].CoverageWorkItemID) < string(triggers[j].CoverageWorkItemID)
	})
	byID := make(map[common.CoverageWorkItemID]int, len(facts.Coverage))
	for index, item := range facts.Coverage {
		byID[item.ID] = index
	}
	reopened := make([]common.CoverageWorkItemID, 0, len(triggers))
	seen := make(map[common.CoverageWorkItemID]struct{}, len(triggers))
	for _, trigger := range triggers {
		coverageID := trigger.CoverageWorkItemID
		if coverageID == "" {
			continue
		}
		if _, duplicate := seen[coverageID]; duplicate {
			continue
		}
		seen[coverageID] = struct{}{}
		index, found := byID[coverageID]
		if !found {
			return runtimecontract.RuntimeFacts{}, nil, fmt.Errorf("runtime CWI reopen references unknown coverage %q", coverageID)
		}
		item := &facts.Coverage[index]
		if item.CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStateReopened {
			continue
		}
		// A declared dependency can be observed before its dependent CWI has
		// settled. That is not a second reopen: the item is already open or
		// still executing, so this trigger is a deterministic no-op.
		if item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateSettled || item.CurrentBindingRef == nil || item.State != runtimecontract.RuntimeCoverageItemStateSatisfied {
			continue
		}
		previous := item.CurrentBindingRef.Clone()
		lineage := runtimecontract.RuntimeCWIReopenLineage{
			PreviousBindingRef: previous,
			TriggerRef:         strings.TrimSpace(trigger.TriggerRef),
			Reason:             strings.TrimSpace(trigger.Reason),
		}
		if err := lineage.Validate(); err != nil {
			return runtimecontract.RuntimeFacts{}, nil, err
		}
		if len(item.ReopenLineage) >= runtimecontract.RuntimeCWIReopenLineageLimit {
			return runtimecontract.RuntimeFacts{}, nil, fmt.Errorf("runtime CWI reopen lineage exceeds bounded limit %d", runtimecontract.RuntimeCWIReopenLineageLimit)
		}
		item.ReopenLineage = append(item.ReopenLineage, lineage)
		item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateReopened
		item.CurrentBindingRef = nil
		item.State = runtimecontract.RuntimeCoverageItemStateOpen
		item.LastAssessmentID = ""
		item.ReopenedByEvidenceRefs = append(item.ReopenedByEvidenceRefs, lineage.TriggerRef)
		reopened = append(reopened, coverageID)
	}
	if len(reopened) == 0 {
		return facts, nil, nil
	}
	updated, err := replaceRuntimeCWIFocusProjection(facts, reopened)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, nil, err
	}
	return updated, reopened, nil
}

// applyRuntimeCWIReopen is the single SM-2 mutation helper for receipt- and
// reconciliation-driven CWI reopening. It changes the canonical Coverage
// ledger, its focus projection, and the current reconciliation contract in
// one owner-local aggregate transaction. Callers retain responsibility for
// recording their own SM-4 batch/result facts before superseding an active
// generation, so no transport history is discarded.
func applyRuntimeCWIReopen(aggregate *RuntimeAggregate, triggers []runtimeCWIReopenTrigger) ([]common.CoverageWorkItemID, error) {
	if aggregate == nil {
		return nil, common.ErrInvalidArgument
	}
	updated, reopened, err := reopenRuntimeCWIs(aggregate.Facts, triggers)
	if err != nil || len(reopened) == 0 {
		return reopened, err
	}
	contract, err := canonicalRuntimeCWIReconciliationContract(updated)
	if err != nil {
		return nil, err
	}
	aggregate.Facts = updated
	aggregate.Facts.ReconciliationContract = contract
	return reopened, nil
}

func retireRuntimeCWIBindingsForManifest(facts runtimecontract.RuntimeFacts, semanticManifestHash string) (runtimecontract.RuntimeFacts, []common.CoverageWorkItemID, error) {
	if err := requireCurrentRuntimeCWILedger(facts); err != nil {
		return runtimecontract.RuntimeFacts{}, nil, err
	}
	semanticManifestHash = strings.TrimSpace(semanticManifestHash)
	if semanticManifestHash == "" {
		return runtimecontract.RuntimeFacts{}, nil, common.ErrInvalidArgument
	}
	retired := make([]common.CoverageWorkItemID, 0)
	for index := range facts.Coverage {
		item := &facts.Coverage[index]
		if item.CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStateSettled || item.CurrentBindingRef == nil || item.CurrentBindingRef.SemanticManifestHash != semanticManifestHash {
			continue
		}
		item.CurrentBindingRef = nil
		item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStatePlanning
		if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			item.State = runtimecontract.RuntimeCoverageItemStateOpen
		}
		retired = append(retired, item.ID)
	}
	if len(retired) == 0 {
		return facts, nil, nil
	}
	updated, err := replaceRuntimeCWIFocusProjection(facts, retired)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, nil, err
	}
	return updated, retired, nil
}

func supersedeActiveReconciliationForCurrentContract(aggregate *RuntimeAggregate) (bool, error) {
	if aggregate == nil || aggregate.Reconciliation.Request == nil {
		return false, nil
	}
	request := aggregate.Reconciliation.Request.Clone()
	contract := aggregate.Facts.ReconciliationContract.Normalized()
	if contract.SchemaVersion != "" && request.ValidateAgainstContract(contract) == nil {
		return false, nil
	}
	ledger := aggregate.Reconciliation.Clone()
	if len(ledger.History) >= runtimecontract.RuntimeCWIReconciliationHistoryLimit {
		return false, fmt.Errorf("runtime reconciliation history exceeds bounded limit %d", runtimecontract.RuntimeCWIReconciliationHistoryLimit)
	}
	previous := runtimecontract.RuntimeCWIReconciliationGeneration{
		Superseded:  true,
		Request:     request,
		RequestHash: ledger.RequestHash,
		Batch:       ledger.Batch,
		Results:     append([]runtimecontract.RuntimeCWIReconciliationResult(nil), ledger.Results...),
		Reopened:    append([]runtimecontract.RuntimeCWIReconciliationReopenedLineage(nil), ledger.Reopened...),
	}
	if err := previous.Validate(); err != nil {
		return false, err
	}
	ledger.History = append(ledger.History, previous)
	ledger.Request = nil
	ledger.RequestHash = ""
	ledger.Batch = nil
	ledger.Results = nil
	ledger.Reopened = nil
	aggregate.Reconciliation = ledger.Normalized()
	if err := aggregate.Reconciliation.Validate(); err != nil {
		return false, err
	}
	return true, nil
}
