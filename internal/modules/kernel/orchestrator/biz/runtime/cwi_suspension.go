package runtime

import (
	"fmt"
	"sort"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeCWIRevalidationSourceUnavailable = "cwi_revalidation_source_unavailable"

// suspendRuntimeSuccessorForStalePredecessor is an SM-2 safe-window projection.
// The private receipt/observation CAS is its only producer; external callers
// cannot select a predecessor or supply a frozen focus. Unsupported sources
// become an explicit failed outcome, never another effect/candidate request.
func suspendRuntimeSuccessorForStalePredecessor(a *RuntimeAggregate) (bool, error) {
	if a == nil {
		return false, common.ErrInvalidArgument
	}
	if a.ActiveDispatch != nil || runtimeApplicabilityHasUnsettledProvider(*a) || a.Termination != nil || a.TerminalMaterialSeal != nil || isTerminalConvergenceState(a.Decision.State) {
		return false, nil
	}
	unavailable := func(reason string) (bool, error) {
		failure := runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateInfeasible, Code: runtimeCWIRevalidationSourceUnavailable, Message: reason}
		if a.Facts.Failure != nil {
			failure = a.Facts.Failure.Clone()
			failure.Class, failure.Code, failure.Message = runtimecontract.FailureClassCandidateInfeasible, runtimeCWIRevalidationSourceUnavailable, reason
		}
		if err := replaceRuntimeFailureAuthority(&a.Facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
			return false, err
		}
		a.Facts.Resources.TargetSelection, a.Facts.Resources.TargetSnapshots = nil, nil
		return true, nil
	}
	rows := runtimeCoverageByID(a.Facts.Coverage)
	currentID := common.CoverageWorkItemID("")
	active := -1
	for i, pause := range a.Facts.CWISuspensions {
		if pause.Active() {
			if len(pause.FrozenFocus.CoverageWorkItemIDs) != 1 {
				return false, common.ErrConflict
			}
			active = i
			currentID = pause.FrozenFocus.CoverageWorkItemIDs[0]
			break
		}
	}
	if active < 0 {
		if len(a.Facts.Focus.CoverageWorkItemIDs) != 1 {
			return false, nil
		}
		currentID = a.Facts.Focus.CoverageWorkItemIDs[0]
	}
	current := rows[currentID]
	consumers, err := runtimeSuspensionConsumers(a.Facts, a.Reconciliation, current, a.StateRevision+1)
	if err != nil {
		return unavailable(err.Error())
	}
	if active >= 0 {
		pause := &a.Facts.CWISuspensions[active]
		additions := runtimeSuspensionNewConsumers(pause.Consumers, consumers)
		if len(additions) == 0 {
			return false, nil
		}
		pause.Consumers = append(pause.Consumers, additions...)
		// New observations can arrive one at a time. Reuse the ordinary
		// eligibility projection after joining their exact sources; no nested
		// pause or second queue is introduced.
		projected, err := projectRuntimeCWIExecutionFocus(a.Facts, nil, nil)
		if err != nil {
			return false, err
		}
		a.Facts = projected
		return true, nil
	}
	if len(consumers) == 0 {
		return false, nil
	}
	// Select by the existing Definition/eligibility rule, never dependency
	// enumeration order or a pause-local scheduler.
	var predecessor common.CoverageWorkItemID
	for _, id := range runtimecontract.RuntimeCWIsEligibleForPlanning(a.Facts) {
		for _, consumer := range consumers {
			if consumer.PredecessorID == id {
				predecessor = id
				break
			}
		}
		if predecessor != "" {
			break
		}
	}
	if current.CurrentBindingRef == nil {
		canYield, reason, yieldErr := runtimeCWICurrentFocusCanYieldWithoutSuspension(*a, current)
		if yieldErr != nil {
			return false, yieldErr
		}
		if !canYield {
			return unavailable(reason)
		}
	} else if current.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound {
		return unavailable("stale predecessor revalidation cannot freeze a successor without its original current binding")
	}
	unboundYield := current.CurrentBindingRef == nil
	if a.Facts.Repair.Exhausted {
		return unavailable("CWI suspension cannot reset exhausted repair budget")
	}
	eligible := false
	for _, id := range runtimecontract.RuntimeCWIsEligibleForPlanning(a.Facts) {
		eligible = eligible || id == predecessor
	}
	if !eligible {
		return unavailable("exact stale predecessor is not currently eligible; nested suspension is unavailable")
	}
	var effectID common.EvidenceRecordID
	if !unboundYield {
		for _, fact := range a.Facts.Evidence.Accepted {
			if fact.Kind != runtimecontract.EvidenceKindStateChange || !runtimecontract.EvidenceAppliesToCoverageWorkItem(a.Facts.Focus, fact, currentID) {
				continue
			}
			for _, dispatch := range a.CompletedDispatches {
				if dispatch.ID != fact.DispatchID {
					continue
				}
				for _, execution := range dispatch.OperationExecutions {
					if execution.OperationID == fact.OperationID && execution.State == runtimecontract.OperationExecutionEvidenceCommitted && execution.EvidenceFactID == fact.ID && execution.EvidenceFactHash == runtimecontract.StableRuntimeEvidenceFactHash(fact) && runtimecontract.RuntimeEffectIsDurable(execution.ExpectedEffectScope) {
						effectID = fact.ID
					}
				}
			}
		}
		if effectID == "" {
			for _, fact := range a.Facts.Evidence.Accepted {
				if fact.Kind == runtimecontract.EvidenceKindStateChange && runtimecontract.EvidenceAppliesToCoverageWorkItem(a.Facts.Focus, fact, currentID) {
					return unavailable("CWI suspension cannot drop an unsettled successor effect")
				}
			}
		}
	}
	for _, dispatch := range a.CompletedDispatches {
		for _, execution := range dispatch.OperationExecutions {
			if execution.State == runtimecontract.OperationExecutionOutcomeUnknown {
				return unavailable("CWI suspension cannot cross unknown execution outcome")
			}
		}
	}
	remaining := make([]runtimecontract.CoverageFocus, 0, len(a.Facts.RemainingFocuses))
	var next runtimecontract.CoverageFocus
	for _, focus := range a.Facts.RemainingFocuses {
		if len(focus.CoverageWorkItemIDs) == 1 && focus.CoverageWorkItemIDs[0] == predecessor {
			if next.ID != "" {
				return false, fmt.Errorf("%w: duplicate suspension predecessor cursor", common.ErrConflict)
			}
			next = focus
		} else {
			remaining = append(remaining, focus)
		}
	}
	if next.ID == "" {
		return false, fmt.Errorf("%w: missing suspension predecessor cursor", common.ErrConflict)
	}
	if unboundYield {
		previousFocus := a.Facts.Focus.Normalized()
		facts := a.Facts.Normalized()
		clearRuntimeSuspendedFocusAuthority(&facts)
		remaining = append([]runtimecontract.CoverageFocus{previousFocus}, remaining...)
		projected, err := installRuntimeCWIFocusProjection(facts, next, remaining, facts.CompletedFocuses)
		if err != nil {
			return false, err
		}
		a.Facts = projected
		if a.ImplementationWork.ScopePreview != nil {
			a.ImplementationWork.ScopePreview = nil
		}
		return true, nil
	}
	pause := (runtimecontract.RuntimeCWISuspension{ProtocolRef: runtimecontract.RuntimeCWISuspensionV1, SuspendedAtStateRevision: a.StateRevision + 1, SuspendedEvidenceRevision: a.Facts.EvidenceRevision,
		FrozenFocus: a.Facts.Focus, FrozenBinding: *current.CurrentBindingRef, EffectEvidenceID: effectID, Consumers: consumers,
		Repair: a.Facts.Repair, RepairDirective: a.Facts.RepairDirective, Failure: a.Facts.Failure}).CloneRaw()
	pause.FrozenPrerequisites, pause.FrozenObservations = runtimeSuspensionFocusPrerequisites(a.Facts)
	facts := a.Facts.Normalized()
	facts.CWISuspensions = append(facts.CWISuspensions, pause)
	clearRuntimeSuspendedFocusAuthority(&facts)
	remaining = append([]runtimecontract.CoverageFocus{pause.FrozenFocus}, remaining...)
	projected, err := installRuntimeCWIFocusProjection(facts, next, remaining, facts.CompletedFocuses)
	if err != nil {
		return false, err
	}
	a.Facts = projected
	return true, nil
}

// runtimeCWICurrentFocusCanYieldWithoutSuspension determines whether an unbound
// current focus holds no binding fence, repair debt, active failure/directive,
// semantic generation, or unsettled implementation-work commitment, so SM-2 can
// yield focus directly to a reopened declared ordinary predecessor without
// manufacturing a RuntimeCWISuspension.
func runtimeCWICurrentFocusCanYieldWithoutSuspension(a RuntimeAggregate, current runtimecontract.RuntimeCoverageItemFact) (bool, string, error) {
	switch current.CWIConvergenceState {
	case runtimecontract.RuntimeCWIConvergenceStateUnbound,
		runtimecontract.RuntimeCWIConvergenceStatePlanning,
		runtimecontract.RuntimeCWIConvergenceStateReopened:
	default:
		return false, "stale predecessor revalidation cannot freeze a successor without its original current binding", nil
	}
	if len(a.Facts.Focus.CoverageEvidenceBindings) != 0 {
		return false, "stale predecessor revalidation cannot freeze a successor without its original current binding", nil
	}
	for _, fact := range a.Facts.Evidence.Accepted {
		if fact.Kind == runtimecontract.EvidenceKindStateChange && runtimecontract.EvidenceAppliesToCoverageWorkItem(a.Facts.Focus, fact, current.ID) {
			return false, "CWI suspension cannot drop an unsettled successor effect", nil
		}
	}
	if a.Facts.Repair.Exhausted {
		return false, "CWI suspension cannot reset exhausted repair budget", nil
	}
	if a.Facts.Repair.RepairAttempts != 0 ||
		a.Facts.Repair.ValidationOnlyRetries != 0 ||
		a.Facts.Repair.NoProgressCount != 0 ||
		len(a.Facts.Repair.SignatureCounts) != 0 ||
		len(a.Facts.Repair.NoProgressOperationShapes) != 0 ||
		a.Facts.Repair.ActiveGap != nil {
		return false, "unbound successor with repair debt cannot yield without its original current binding", nil
	}
	if a.Facts.RepairDirective != nil || a.Facts.Failure != nil || a.SemanticPlanning.FailureInputReview != nil {
		return false, "unbound successor with active failure or repair authority cannot yield without its original current binding", nil
	}
	if (a.SemanticPlanning.Phase != "" && a.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired) || a.SemanticPlanning.Manifest != nil {
		return false, "unbound successor cannot yield across an active semantic planning generation", nil
	}
	reservation, err := CurrentRuntimeImplementationWorkReservation(a)
	if err != nil {
		return false, "", err
	}
	if reservation != nil {
		return false, "unbound successor cannot yield with an unsettled implementation work reservation", nil
	}
	for _, round := range a.ImplementationWork.Rounds {
		if round.CoverageWorkItemID == current.ID && (round.CompletedReceiptSetHash == "" || round.FailedReceiptHash != "") {
			return false, "unbound successor cannot yield with an unsettled or failed implementation work round", nil
		}
	}
	if a.ImplementationWork.ScopePreview != nil && a.ImplementationWork.ScopePreview.Request.CoverageWorkItemID != current.ID {
		return false, "unbound successor cannot yield across another CWI scope preview", nil
	}
	return true, "", nil
}

// Resolve only current, affected acceptance edges at the formal observation
// ingress. A shared consumer is included once; a later genuine reopen appends
// its new source without rewriting an earlier proof.
func runtimeSuspensionConsumers(facts runtimecontract.RuntimeFacts, reconciliation runtimecontract.RuntimeCWIReconciliationLedger, current runtimecontract.RuntimeCoverageItemFact, revision uint64) ([]runtimecontract.RuntimeCWISuspendedConsumer, error) {
	rows := runtimeCoverageByID(facts.Coverage)
	var consumers []runtimecontract.RuntimeCWISuspendedConsumer
	seen := make(map[common.CoverageWorkItemID]bool)
	for _, id := range current.PrerequisiteCoverageWorkItemIDs {
		dependency, found := rows[id]
		if !found {
			return nil, fmt.Errorf("suspension prerequisite %q missing", id)
		}
		if runtimecontract.RuntimeCWIPrerequisiteReadyForConsumer(facts, dependency, current.ID) {
			continue
		}
		row, supported := runtimecontract.RuntimeCWIRevalidationConsumer(facts, dependency)
		if !supported {
			return nil, fmt.Errorf("dependency %q has no reopened declared acceptance consumer", dependency.ID)
		}
		cause := row.ReopenLineage[len(row.ReopenLineage)-1]
		if !runtimecontract.RuntimeCWIReopenHasOwnerSource(facts, reconciliation, row.ID, cause, revision) {
			return nil, fmt.Errorf("acceptance consumer %q has no exact owner invalidation source", row.ID)
		}
		if !runtimecontract.RuntimeCWISuspensionOrdinaryPredecessorSupported(facts, row.ID) {
			return nil, fmt.Errorf("consumer %q has no declared validation-only current acceptance source; historical effects cannot be replayed", row.ID)
		}
		if !seen[row.ID] {
			consumers = append(consumers, runtimecontract.RuntimeCWISuspendedConsumer{PredecessorID: row.ID, ReopenSource: cause, RecordedAtStateRevision: revision})
			seen[row.ID] = true
		}
	}
	sort.Slice(consumers, func(i, j int) bool { return consumers[i].PredecessorID < consumers[j].PredecessorID })
	return consumers, nil
}

func runtimeSuspensionNewConsumers(previous, observed []runtimecontract.RuntimeCWISuspendedConsumer) []runtimecontract.RuntimeCWISuspendedConsumer {
	var additions []runtimecontract.RuntimeCWISuspendedConsumer
	for _, c := range observed {
		found := false
		for _, old := range previous {
			found = found || old.PredecessorID == c.PredecessorID && runtimecontract.RuntimeRawEqual(old.ReopenSource, c.ReopenSource)
		}
		if !found {
			additions = append(additions, c)
		}
	}
	return additions
}

func clearRuntimeSuspendedFocusAuthority(facts *runtimecontract.RuntimeFacts) {
	retireFocusScopedPrerequisites(facts, facts.Focus.ID)
	facts.Failure, facts.Feasibility, facts.RepairDirective = nil, nil, nil
	facts.Resources.TargetSelection, facts.Resources.TargetSnapshots, facts.Resources.FailureTarget = nil, nil, nil
}

// resumeRuntimeSuspendedSuccessor runs only after ordinary current settlement
// and fresh applicability sampling. It restores the original successor, not
// its owner catalogs/current qualification, and preserves its original repair
// debt. The completed predecessor remains in the canonical completed queue.
func resumeRuntimeSuspendedSuccessor(facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeFacts, bool, error) {
	facts = facts.Normalized()
	for index, pause := range facts.CWISuspensions {
		if !pause.Active() {
			continue
		}
		rows := runtimeCoverageByID(facts.Coverage)
		if len(facts.Focus.CoverageWorkItemIDs) != 1 {
			return runtimecontract.RuntimeFacts{}, false, common.ErrConflict
		}
		id := facts.Focus.CoverageWorkItemIDs[0]
		consumerIndex := -1
		for j, c := range pause.Consumers {
			if c.PredecessorID == id && c.ResumeBinding == nil {
				consumerIndex = j
			}
		}
		if consumerIndex < 0 {
			return runtimecontract.RuntimeFacts{}, false, fmt.Errorf("%w: suspension resume cursor drift", common.ErrConflict)
		}
		consumer := pause.Consumers[consumerIndex]
		predecessor := rows[id]
		if predecessor.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateSettled || predecessor.State != runtimecontract.RuntimeCoverageItemStateSatisfied || predecessor.CurrentBindingRef == nil || predecessor.LastAssessmentID == "" {
			return facts, false, nil
		}
		if len(predecessor.ReopenLineage) == 0 || !runtimecontract.RuntimeRawEqual(predecessor.ReopenLineage[len(predecessor.ReopenLineage)-1], consumer.ReopenSource) || runtimecontract.SameRuntimeCWIBindingRef(*predecessor.CurrentBindingRef, consumer.ReopenSource.PreviousBindingRef) {
			return runtimecontract.RuntimeFacts{}, false, fmt.Errorf("%w: suspension resume source drift", common.ErrConflict)
		}
		current := rows[pause.FrozenFocus.CoverageWorkItemIDs[0]]
		if current.CurrentBindingRef == nil || !runtimecontract.SameRuntimeCWIBindingRef(*current.CurrentBindingRef, pause.FrozenBinding) {
			return runtimecontract.RuntimeFacts{}, false, fmt.Errorf("%w: suspension frozen binding drift", common.ErrConflict)
		}
		ref := predecessor.CurrentBindingRef.Clone()
		c := &facts.CWISuspensions[index].Consumers[consumerIndex]
		c.ResumeBinding, c.ResumeAssessmentID = &ref, predecessor.LastAssessmentID
		c.ResumeEvidenceRevision, c.ResumeObservationCount = facts.EvidenceRevision, len(facts.EvidenceApplicabilityObservations)
		pending := false
		for _, other := range facts.CWISuspensions[index].Consumers {
			pending = pending || other.ResumeBinding == nil
		}
		if pending {
			eligible := runtimecontract.RuntimeCWIsEligibleForPlanning(facts)
			if len(eligible) == 0 {
				return runtimecontract.RuntimeFacts{}, false, fmt.Errorf("%w: suspension has no eligible unresolved consumer", common.ErrConflict)
			}
			var next runtimecontract.CoverageFocus
			var remaining []runtimecontract.CoverageFocus
			for _, f := range facts.RemainingFocuses {
				if len(f.CoverageWorkItemIDs) == 1 && f.CoverageWorkItemIDs[0] == eligible[0] {
					if next.ID != "" {
						return runtimecontract.RuntimeFacts{}, false, common.ErrConflict
					}
					next = f
				} else {
					remaining = append(remaining, f)
				}
			}
			if next.ID == "" {
				return runtimecontract.RuntimeFacts{}, false, common.ErrConflict
			}
			completed := append(append([]runtimecontract.CoverageFocus(nil), facts.CompletedFocuses...), facts.Focus)
			clearRuntimeSuspendedFocusAuthority(&facts)
			projected, err := installRuntimeCWIFocusProjection(facts, next, remaining, completed)
			return projected, err == nil, err
		}
		for _, prerequisiteID := range current.PrerequisiteCoverageWorkItemIDs {
			if !runtimecontract.RuntimeCWIPrerequisiteReadyForConsumer(facts, rows[prerequisiteID], current.ID) {
				return runtimecontract.RuntimeFacts{}, false, fmt.Errorf("%w: completed suspension consumers have new unresolved prerequisite facts", common.ErrConflict)
			}
		}
		remaining := make([]runtimecontract.CoverageFocus, 0, len(facts.RemainingFocuses))
		found := 0
		for _, focus := range facts.RemainingFocuses {
			if focus.ID == pause.FrozenFocus.ID {
				if !runtimecontract.RuntimeRawEqual(focus, pause.FrozenFocus) {
					return runtimecontract.RuntimeFacts{}, false, common.ErrConflict
				}
				found++
			} else {
				remaining = append(remaining, focus)
			}
		}
		if found != 1 {
			return runtimecontract.RuntimeFacts{}, false, fmt.Errorf("%w: suspension frozen queue missing/duplicate", common.ErrConflict)
		}
		completed := append(append([]runtimecontract.CoverageFocus(nil), facts.CompletedFocuses...), facts.Focus)
		clearRuntimeSuspendedFocusAuthority(&facts)
		// Remove only these exact archived identities from the active retirement
		// index. Their original bodies remain in the immutable pause; no old
		// observation is restored as current, so ordinary preflight reacquires.
		archived := make(map[string]bool)
		for _, prerequisite := range pause.FrozenPrerequisites {
			archived[prerequisite.ID] = true
		}
		retired := facts.Prerequisites.Retired[:0]
		for _, prerequisite := range facts.Prerequisites.Retired {
			if !archived[prerequisite.ID] {
				retired = append(retired, prerequisite)
			}
		}
		facts.Prerequisites.Retired = retired
		observations := facts.Resources.RetiredPrerequisiteObservations[:0]
		for _, observation := range facts.Resources.RetiredPrerequisiteObservations {
			if !archived[observation.PrerequisiteID] {
				observations = append(observations, observation)
			}
		}
		facts.Resources.RetiredPrerequisiteObservations = observations
		facts.CWISuspensions[index].ResumedEvidenceRevision = facts.EvidenceRevision
		projected, err := installRuntimeCWIFocusProjection(facts, pause.FrozenFocus, remaining, completed)
		if err != nil {
			return runtimecontract.RuntimeFacts{}, false, err
		}
		projected.Repair, projected.RepairDirective, projected.Failure = pause.Repair, pause.RepairDirective, pause.Failure
		return projected.Normalized(), true, nil
	}
	return facts, false, nil
}

func validateRuntimeCWISuspensionTransition(previous, next RuntimeAggregate) error {
	before, after := previous.Facts.CWISuspensions, next.Facts.CWISuspensions
	if len(after) < len(before) || len(after) > len(before)+1 {
		return fmt.Errorf("%w: CWI suspension history is append-only", common.ErrConflict)
	}
	for i, original := range before {
		if runtimecontract.RuntimeRawEqual(original, after[i]) {
			continue
		}
		if !original.Active() || len(original.Consumers) > len(after[i].Consumers) {
			return fmt.Errorf("%w: CWI suspension source cannot be rewritten", common.ErrConflict)
		}
		if previous.ActiveDispatch != nil || runtimeApplicabilityHasUnsettledProvider(previous) || previous.Termination != nil || previous.TerminalMaterialSeal != nil || isTerminalConvergenceState(previous.Decision.State) {
			return fmt.Errorf("%w: CWI suspension change crossed in-flight responsibility", common.ErrConflict)
		}
		if len(original.Consumers) < len(after[i].Consumers) {
			rows := runtimeCoverageByID(next.Facts.Coverage)
			observed, err := runtimeSuspensionConsumers(next.Facts, next.Reconciliation, rows[original.FrozenFocus.CoverageWorkItemIDs[0]], next.StateRevision)
			if err != nil {
				return err
			}
			copy := after[i].CloneRaw()
			additions := runtimeSuspensionNewConsumers(original.Consumers, observed)
			copy.Consumers = copy.Consumers[:len(original.Consumers)]
			if !runtimecontract.RuntimeRawEqual(copy, original) || !runtimecontract.RuntimeRawEqual(after[i].Consumers[len(original.Consumers):], additions) {
				return fmt.Errorf("%w: suspension consumer append has no exact current owner source", common.ErrConflict)
			}
			if !runtimecontract.RuntimeRawEqual(previous.CompletedDispatches, next.CompletedDispatches) || !runtimecontract.RuntimeRawEqual(previous.OperationReceipts, next.OperationReceipts) || !runtimecontract.RuntimeRawEqual(previous.ExecutionBudget, next.ExecutionBudget) {
				return fmt.Errorf("%w: consumer append cannot change effects/receipts/budget", common.ErrConflict)
			}
			continue
		}
		copy := after[i].CloneRaw()
		copy.ResumedEvidenceRevision = original.ResumedEvidenceRevision
		changed := 0
		for j, old := range original.Consumers {
			current := after[i].Consumers[j]
			if runtimecontract.RuntimeRawEqual(old, current) {
				continue
			}
			if old.ResumeBinding != nil || current.ResumeBinding == nil || len(previous.Facts.Focus.CoverageWorkItemIDs) != 1 || previous.Facts.Focus.CoverageWorkItemIDs[0] != old.PredecessorID {
				return fmt.Errorf("%w: CWI consumer proof may be committed once for the exact current consumer", common.ErrConflict)
			}
			copy.Consumers[j].ResumeBinding, copy.Consumers[j].ResumeAssessmentID, copy.Consumers[j].ResumeEvidenceRevision, copy.Consumers[j].ResumeObservationCount = nil, "", "", 0
			changed++
		}
		if changed != 1 || !runtimecontract.RuntimeRawEqual(copy, original) {
			return fmt.Errorf("%w: CWI resume rewrote original source", common.ErrConflict)
		}
		if previous.ActiveDispatch != nil || runtimeApplicabilityHasUnsettledProvider(previous) || previous.Termination != nil || previous.TerminalMaterialSeal != nil || isTerminalConvergenceState(previous.Decision.State) {
			return fmt.Errorf("%w: CWI resume crossed in-flight responsibility", common.ErrConflict)
		}
		if !after[i].Active() {
			if after[i].ResumedEvidenceRevision != next.Facts.EvidenceRevision || !runtimecontract.RuntimeRawEqual(next.Facts.Focus, original.FrozenFocus) || !runtimecontract.RuntimeRawEqual(next.Facts.Repair, original.Repair) || !runtimecontract.RuntimeRawEqual(next.Facts.RepairDirective, original.RepairDirective) || !runtimecontract.RuntimeRawEqual(next.Facts.Failure, original.Failure) {
				return fmt.Errorf("%w: CWI resume lost frozen focus/repair budget", common.ErrConflict)
			}
		} else {
			projection := previous.Facts.Normalized()
			projection.CWISuspensions[i] = after[i].CloneRaw()
			eligible := runtimecontract.RuntimeCWIsEligibleForPlanning(projection)
			allowed := len(eligible) != 0 && len(next.Facts.Focus.CoverageWorkItemIDs) == 1 && next.Facts.Focus.CoverageWorkItemIDs[0] == eligible[0]
			if !allowed {
				return fmt.Errorf("%w: CWI continuation did not use canonical eligibility", common.ErrConflict)
			}
		}

	}
	if len(after) != len(before) {
		if previous.ActiveDispatch != nil || runtimeApplicabilityHasUnsettledProvider(previous) || previous.Termination != nil || previous.TerminalMaterialSeal != nil || isTerminalConvergenceState(previous.Decision.State) {
			return fmt.Errorf("%w: CWI suspension crossed original unsafe window", common.ErrConflict)
		}
		added := after[len(before)]
		if !added.Active() {
			return fmt.Errorf("%w: CWI suspension cannot append a pre-resumed source", common.ErrConflict)
		}
		for _, consumer := range added.Consumers {
			if consumer.ResumeBinding != nil || consumer.ResumeAssessmentID != "" || consumer.ResumeEvidenceRevision != "" || consumer.ResumeObservationCount != 0 {
				return fmt.Errorf("%w: appended consumer is already completed", common.ErrConflict)
			}
		}
		rows := runtimeCoverageByID(next.Facts.Coverage)
		expected, err := runtimeSuspensionConsumers(next.Facts, next.Reconciliation, rows[added.FrozenFocus.CoverageWorkItemIDs[0]], next.StateRevision)
		if err != nil {
			return err
		}
		if !runtimecontract.RuntimeRawEqual(added.Consumers, expected) {
			return fmt.Errorf("%w: suspension omitted or changed an affected consumer", common.ErrConflict)
		}
		prerequisites, observations := runtimeSuspensionFocusPrerequisites(previous.Facts)
		if !runtimecontract.RuntimeRawEqual(added.FrozenPrerequisites, prerequisites) || !runtimecontract.RuntimeRawEqual(added.FrozenObservations, observations) {
			return fmt.Errorf("%w: suspension prerequisite archive differs from original owner preimage", common.ErrConflict)
		}
		if added.SuspendedAtStateRevision != next.StateRevision || added.SuspendedEvidenceRevision != previous.Facts.EvidenceRevision || !runtimecontract.RuntimeRawEqual(added.FrozenFocus, previous.Facts.Focus) || !runtimecontract.RuntimeRawEqual(added.Repair, previous.Facts.Repair) || !runtimecontract.RuntimeRawEqual(added.RepairDirective, previous.Facts.RepairDirective) || !runtimecontract.RuntimeRawEqual(added.Failure, previous.Facts.Failure) {
			return fmt.Errorf("%w: CWI suspension has no exact original focus/repair preimage", common.ErrConflict)
		}
		if !runtimecontract.RuntimeRawEqual(previous.CompletedDispatches, next.CompletedDispatches) || !runtimecontract.RuntimeRawEqual(previous.OperationReceipts, next.OperationReceipts) || !runtimecontract.RuntimeRawEqual(previous.ExecutionBudget, next.ExecutionBudget) {
			return fmt.Errorf("%w: CWI suspension cannot change effects/receipts/budget", common.ErrConflict)
		}
	}
	return nil
}

func runtimeSuspensionFocusPrerequisites(facts runtimecontract.RuntimeFacts) ([]runtimecontract.RuntimePrerequisite, []runtimecontract.RuntimePrerequisiteObservation) {
	// Normalized constructs detached owner facts; original body integrity is
	// checked by the enclosing aggregate before any pause transaction.
	facts = facts.Normalized()
	var prerequisites []runtimecontract.RuntimePrerequisite
	ids := make(map[string]bool)
	for _, prerequisite := range append(facts.Prerequisites.Pending, facts.Prerequisites.Satisfied...) {
		if prerequisite.Scope == runtimecontract.RuntimePrerequisiteScopeFocus && prerequisite.FocusID == facts.Focus.ID {
			prerequisites = append(prerequisites, prerequisite)
			ids[prerequisite.ID] = true
		}
	}
	var observations []runtimecontract.RuntimePrerequisiteObservation
	for _, observation := range facts.Resources.PrerequisiteObservations {
		if ids[observation.PrerequisiteID] {
			observations = append(observations, observation)
		}
	}
	return prerequisites, observations
}
