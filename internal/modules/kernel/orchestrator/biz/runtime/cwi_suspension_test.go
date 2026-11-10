package runtime

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// A detached projection fixture, not an owner/CAS/receipt producer. Real
// persisted-source and recovery admission is exercised by the biz mainline.
func runtimeSuspensionProjectionFixture(t *testing.T) RuntimeAggregate {
	t.Helper()
	a := stateChangeAggregate(t)
	id := a.Facts.Focus.CoverageWorkItemIDs[0]
	old := runtimecontract.RuntimeCWIBindingRef{BindingID: "old-source", BindingHash: "old-hash", FocusID: "old-source-focus", SemanticManifestHash: "old-generation"}
	cause := runtimecontract.RuntimeCWIReopenLineage{PreviousBindingRef: old, TriggerRef: "actual-stale-source", Reason: "owner dependency observation no longer matches original evidence applicability"}
	predecessor := runtimecontract.RuntimeCoverageItemFact{ID: "ordinary-source", State: runtimecontract.RuntimeCoverageItemStateOpen, CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStateReopened, ReopenLineage: []runtimecontract.RuntimeCWIReopenLineage{cause}, FocusID: "semantic-pending/ordinary-source"}
	a.Facts.DefinitionFocuses = append(a.Facts.DefinitionFocuses, runtimecontract.RuntimeDefinitionFocus{ID: "ordinary-source-definition", CoverageWorkItemIDs: []common.CoverageWorkItemID{predecessor.ID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, RequireAssessment: true})
	a.Facts.DefinitionSeed.CoverageItems = append(a.Facts.DefinitionSeed.CoverageItems, runtimecontract.RuntimeCoverageItem{ID: predecessor.ID, SliceID: a.Facts.Focus.SliceID})
	for i := range a.Facts.Coverage {
		if a.Facts.Coverage[i].ID == id {
			a.Facts.Coverage[i].PrerequisiteCoverageWorkItemIDs = []common.CoverageWorkItemID{predecessor.ID}
		}
	}
	a.Facts.Coverage = append(a.Facts.Coverage, predecessor)
	preFocus := runtimeCWIPendingFocus(a.Facts.Focus, predecessor.ID)
	preFocus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	a.Facts.RemainingFocuses = append([]runtimecontract.CoverageFocus{preFocus}, a.Facts.RemainingFocuses...)
	a.Facts.EvidenceApplicabilityObservations = []runtimecontract.RuntimeEvidenceApplicabilityObservation{{Request: runtimecontract.RuntimeEvidenceApplicabilityRequest{RequestID: cause.TriggerRef}}}
	a.Reconciliation.Request = &runtimecontract.RuntimeCWIReconciliationRequest{RequestID: "original-request", WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, DefinitionRevision: a.Facts.DefinitionRevision, Generation: 1, Bindings: []runtimecontract.RuntimeCWIReconciliationBindingRef{{CoverageWorkItemID: predecessor.ID, BindingID: old.BindingID, BindingHash: old.BindingHash}}}
	a.Reconciliation.Batch = &runtimecontract.RuntimeCWIReconciliationBatch{Hash: "original-batch"}
	a.Reconciliation.Reopened = []runtimecontract.RuntimeCWIReconciliationReopenedLineage{{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, RequestID: "original-request", Generation: 1, BatchHash: "original-batch", CoverageWorkItemID: predecessor.ID, BindingID: old.BindingID, ObservationRef: cause.TriggerRef}}
	a.Reconciliation.Results = []runtimecontract.RuntimeCWIReconciliationResult{{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, RequestID: "original-request", Generation: 1, BatchHash: "original-batch", CoverageItemID: predecessor.ID, BindingID: old.BindingID, ObservationRef: cause.TriggerRef, Disposition: runtimecontract.RuntimeCWIReconciliationDispositionReopen}}

	ref := a.Facts.Focus.CoverageEvidenceBindings[0].ID
	fact := runtimecontract.EvidenceFact{ID: "real-effect", Kind: runtimecontract.EvidenceKindStateChange, BindingRefs: []string{ref}, CoverageWorkItemIDs: []common.CoverageWorkItemID{id}, DispatchID: "completed-effect", OperationID: "effect"}
	a.Facts.Evidence.Accepted = append(a.Facts.Evidence.Accepted, fact)
	a.CompletedDispatches = append(a.CompletedDispatches, runtimecontract.RuntimeDispatchRecord{ID: fact.DispatchID, OperationExecutions: []runtimecontract.RuntimeOperationExecutionRecord{{OperationID: fact.OperationID, State: runtimecontract.OperationExecutionEvidenceCommitted, EvidenceFactID: fact.ID, EvidenceFactHash: runtimecontract.StableRuntimeEvidenceFactHash(fact), ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable}}})
	a.Facts.Repair = runtimecontract.RuntimeRepairLedger{FocusID: a.Facts.Focus.ID, SliceID: a.Facts.Focus.SliceID, RepairAttempts: 2, ValidationOnlyRetries: 1, NoProgressCount: 1, SignatureCounts: []runtimecontract.RuntimeFailureSignatureCount{{Signature: "original", Count: 1}}}
	return a
}

func TestCWISuspensionDetachedProjectionPreservesFrozenBindingAndRepairDebt(t *testing.T) {
	before := runtimeSuspensionProjectionFixture(t)
	a := before.CloneRaw()
	changed, err := suspendRuntimeSuccessorForStalePredecessor(&a)
	if err != nil || !changed || len(a.Facts.CWISuspensions) != 1 {
		t.Fatalf("pause: %v changed=%t", err, changed)
	}
	pause := a.Facts.CWISuspensions[0]
	probe := a.CloneRaw().Facts
	probe.Coverage = append(probe.Coverage, runtimecontract.RuntimeCoverageItemFact{ID: "unrelated-ready", CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStateUnbound})
	if got := runtimecontract.RuntimeCWIsEligibleForPlanning(probe); !reflect.DeepEqual(got, []common.CoverageWorkItemID{pause.Consumers[0].PredecessorID}) {
		t.Fatalf("pause selected unrelated source: %v", got)
	}
	if a.Facts.Focus.CoverageWorkItemIDs[0] != pause.Consumers[0].PredecessorID || !reflect.DeepEqual(pause.FrozenFocus, before.Facts.Focus) || !reflect.DeepEqual(pause.Repair, before.Facts.Repair) || !reflect.DeepEqual(a.OperationReceipts, before.OperationReceipts) || !reflect.DeepEqual(a.ExecutionBudget, before.ExecutionBudget) {
		t.Fatal("pause lost original source/repair/effect/budget")
	}
	saved := a.CloneRaw()
	if changed, err := suspendRuntimeSuccessorForStalePredecessor(&a); err != nil || changed || !reflect.DeepEqual(a, saved) {
		t.Fatal("duplicate pause mutated source")
	}
	if _, resumed, err := resumeRuntimeSuspendedSuccessor(a.Facts); err != nil || resumed {
		t.Fatal("reopened predecessor resumed without ordinary settlement")
	}
	ref := runtimecontract.RuntimeCWIBindingRef{BindingID: "new-ordinary-proof", BindingHash: "new-hash", FocusID: a.Facts.Focus.ID, SemanticManifestHash: "new-generation"}
	for i := range a.Facts.Coverage {
		if a.Facts.Coverage[i].ID == pause.Consumers[0].PredecessorID {
			row := &a.Facts.Coverage[i]
			row.State = runtimecontract.RuntimeCoverageItemStateSatisfied
			row.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateSettled
			row.CurrentBindingRef = &ref
			row.LastAssessmentID = "actual-ordinary-assessment"
		}
	}
	restored, resumed, err := resumeRuntimeSuspendedSuccessor(a.Facts)
	if err != nil || !resumed || !reflect.DeepEqual(restored.Focus, before.Facts.Focus) || !reflect.DeepEqual(restored.Repair, before.Facts.Repair) {
		t.Fatalf("resume: %v resumed=%t", err, resumed)
	}
	if restored.Resources.TargetSelection != nil || len(restored.Resources.TargetSnapshots) != 0 {
		t.Fatal("resume restored stale owner qualification")
	}
	if _, again, err := resumeRuntimeSuspendedSuccessor(restored); err != nil || again {
		t.Fatal("duplicate resume repeated transition")
	}
	for _, scenario := range []string{"binding", "source", "missing_queue", "duplicate_queue"} {
		t.Run(scenario, func(t *testing.T) {
			facts := a.CloneRaw().Facts
			switch scenario {
			case "binding":
				for i := range facts.Coverage {
					if facts.Coverage[i].ID != pause.Consumers[0].PredecessorID {
						facts.Coverage[i].CurrentBindingRef = nil
					}
				}
			case "source":
				for i := range facts.Coverage {
					if facts.Coverage[i].ID == pause.Consumers[0].PredecessorID {
						facts.Coverage[i].ReopenLineage[0].TriggerRef = "drift"
					}
				}
			case "missing_queue":
				facts.RemainingFocuses = nil
			case "duplicate_queue":
				facts.RemainingFocuses = append(facts.RemainingFocuses, pause.FrozenFocus)
			}
			original := runtimecontract.CloneRuntimeCWISuspensions(facts.CWISuspensions)
			if _, _, err := resumeRuntimeSuspendedSuccessor(facts); err == nil || !reflect.DeepEqual(original, facts.CWISuspensions) {
				t.Fatal("drift resumed or changed source")
			}
		})
	}
}

func TestCWISuspensionSafeWindowGuardsDoNotDropInFlightResponsibility(t *testing.T) {
	for _, scenario := range []string{"dispatch", "submitted", "terminal", "material", "nested"} {
		t.Run(scenario, func(t *testing.T) {
			a := runtimeSuspensionProjectionFixture(t)
			switch scenario {
			case "dispatch":
				a.ActiveDispatch = &runtimecontract.RuntimeDispatchRecord{ID: "in-flight"}
			case "submitted":
				a.CandidateRequestSessions = append(a.CandidateRequestSessions, runtimecontract.RuntimeCandidateRequestSession{ProviderSubmitted: true, State: runtimecontract.RuntimeCandidateRequestSessionPrepared})
			case "terminal":
				a.Decision.State = runtimecontract.ConvergenceStateFailed
			case "material":
				a.TerminalMaterialSeal = &runtimecontract.RuntimeTerminalMaterialSeal{}
			case "nested":
				if changed, err := suspendRuntimeSuccessorForStalePredecessor(&a); err != nil || !changed {
					t.Fatal(err)
				}
			}
			before := a.CloneRaw()
			if changed, err := suspendRuntimeSuccessorForStalePredecessor(&a); err != nil || changed || !reflect.DeepEqual(before, a) {
				t.Fatal("unsafe window changed authority")
			}
		})
	}
}

func TestCWISuspensionMissingOrdinarySourceUnknownAndBudgetFailClosed(t *testing.T) {
	for _, scenario := range []string{"undeclared", "missing_consumer", "missing_owner_source", "foreign_owner_source", "missing_owner_result", "unknown", "no_binding", "exhausted"} {
		t.Run(scenario, func(t *testing.T) {
			a := runtimeSuspensionProjectionFixture(t)
			switch scenario {
			case "undeclared":
				a.Facts.DefinitionFocuses[len(a.Facts.DefinitionFocuses)-1].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
			case "foreign_owner_source":
				a.Reconciliation.Reopened[0].WorkspaceID = "foreign"
			case "missing_owner_result":
				a.Reconciliation.Results = nil
			case "unknown":
				a.CompletedDispatches[0].OperationExecutions[0].State = runtimecontract.OperationExecutionOutcomeUnknown
			case "missing_consumer":
				a.Facts.Coverage[len(a.Facts.Coverage)-1].HistoricalBehavior = &runtimecontract.RuntimeHistoricalBehaviorCompletion{}
			case "missing_owner_source":
				a.Reconciliation.Reopened = nil
				a.Facts.EvidenceApplicabilityObservations = nil
			case "no_binding":
				a.Facts.Coverage[0].CurrentBindingRef = nil
			case "exhausted":
				a.Facts.Repair.Exhausted = true
			}
			before := a.CloneRaw()
			changed, err := suspendRuntimeSuccessorForStalePredecessor(&a)
			if err != nil || !changed || a.Facts.Failure == nil || a.Facts.Failure.Code != runtimeCWIRevalidationSourceUnavailable {
				t.Fatalf("unsupported source: %v changed=%t", err, changed)
			}
			if !reflect.DeepEqual(a.ExecutionBudget, before.ExecutionBudget) || !reflect.DeepEqual(a.OperationReceipts, before.OperationReceipts) || !reflect.DeepEqual(a.Facts.Repair, before.Facts.Repair) {
				t.Fatal("unavailable source reset budget/receipt/debt")
			}
		})
	}
}

func runtimeSecondSuspensionConsumer(a *RuntimeAggregate) common.CoverageWorkItemID {
	original := a.Facts.Coverage[len(a.Facts.Coverage)-1]
	row := original
	row.ID, row.FocusID = "ordinary-second", "semantic-pending/ordinary-second"
	row.ReopenLineage = append([]runtimecontract.RuntimeCWIReopenLineage(nil), original.ReopenLineage...)
	row.ReopenLineage[0].TriggerRef += "/second"
	row.ReopenLineage[0].PreviousBindingRef.BindingID += "/second"
	a.Facts.Coverage[0].PrerequisiteCoverageWorkItemIDs = append(a.Facts.Coverage[0].PrerequisiteCoverageWorkItemIDs, row.ID)
	a.Facts.Coverage = append(a.Facts.Coverage, row)
	a.Facts.DefinitionSeed.CoverageItems = append(a.Facts.DefinitionSeed.CoverageItems, runtimecontract.RuntimeCoverageItem{ID: row.ID, SliceID: a.Facts.Focus.SliceID})
	d := a.Facts.DefinitionFocuses[len(a.Facts.DefinitionFocuses)-1]
	d.ID, d.CoverageWorkItemIDs = "second-source-definition", []common.CoverageWorkItemID{row.ID}
	a.Facts.DefinitionFocuses = append(a.Facts.DefinitionFocuses, d)
	focus := runtimeCWIPendingFocus(a.Facts.Focus, row.ID)
	focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	a.Facts.RemainingFocuses = append(a.Facts.RemainingFocuses, focus)
	binding := a.Reconciliation.Request.Bindings[0]
	binding.CoverageWorkItemID, binding.BindingID = row.ID, row.ReopenLineage[0].PreviousBindingRef.BindingID
	a.Reconciliation.Request.Bindings = append(a.Reconciliation.Request.Bindings, binding)
	lineage := a.Reconciliation.Reopened[0]
	lineage.CoverageWorkItemID, lineage.BindingID, lineage.ObservationRef = row.ID, binding.BindingID, row.ReopenLineage[0].TriggerRef
	a.Reconciliation.Reopened = append(a.Reconciliation.Reopened, lineage)
	result := a.Reconciliation.Results[0]
	result.CoverageItemID, result.BindingID, result.ObservationRef = row.ID, binding.BindingID, lineage.ObservationRef
	a.Reconciliation.Results = append(a.Reconciliation.Results, result)
	return row.ID
}

func TestCWISuspensionJoinsAllSourcesAndKeepsConsumerProofsImmutable(t *testing.T) {
	previous := runtimeSuspensionProjectionFixture(t)
	second := runtimeSecondSuspensionConsumer(&previous)
	// The second dependency has not yet been observed as stale. The same
	// declared edge will later acquire its own formal invalidation source.
	row := &previous.Facts.Coverage[len(previous.Facts.Coverage)-1]
	row.State, row.CWIConvergenceState = runtimecontract.RuntimeCoverageItemStateSatisfied, runtimecontract.RuntimeCWIConvergenceStateSettled
	old := row.ReopenLineage[0].PreviousBindingRef
	row.CurrentBindingRef = &old
	a := previous.CloneRaw()
	if changed, err := suspendRuntimeSuccessorForStalePredecessor(&a); err != nil || !changed {
		t.Fatal(err)
	}
	a.StateRevision++
	if len(a.Facts.CWISuspensions[0].Consumers) != 1 {
		t.Fatal("unobserved source was treated as stale")
	}
	before := a.CloneRaw()
	row = &a.Facts.Coverage[runtimeCWICoverageIndex(a.Facts.Coverage, second)]
	row.State, row.CWIConvergenceState, row.CurrentBindingRef = runtimecontract.RuntimeCoverageItemStateOpen, runtimecontract.RuntimeCWIConvergenceStateReopened, nil
	if changed, err := suspendRuntimeSuccessorForStalePredecessor(&a); err != nil || !changed {
		t.Fatal("late source did not join original pause", err)
	}
	a.StateRevision++
	if len(a.Facts.CWISuspensions) != 1 || len(a.Facts.CWISuspensions[0].Consumers) != 2 || a.Facts.CWISuspensions[0].Consumers[1].PredecessorID != second {
		t.Fatal("late source lost or nested")
	}
	if err := validateRuntimeCWISuspensionTransition(before, a); err != nil {
		t.Fatal(err)
	}
	saved := a.CloneRaw()
	if changed, err := suspendRuntimeSuccessorForStalePredecessor(&a); err != nil || changed || !reflect.DeepEqual(saved, a) {
		t.Fatal("duplicate source mutated pause")
	}
	for _, kind := range []string{"omitted", "scope", "prior_proof", "budget", "dispatch", "submitted"} {
		t.Run(kind, func(t *testing.T) {
			from, to := before.CloneRaw(), saved.CloneRaw()
			switch kind {
			case "omitted":
				to.Facts.CWISuspensions[0].Consumers = to.Facts.CWISuspensions[0].Consumers[1:]
			case "scope":
				to.Facts.CWISuspensions[0].Consumers[1].PredecessorID = "foreign"
			case "prior_proof":
				to.Facts.CWISuspensions[0].Consumers[0].ResumeAssessmentID = "forged"
			case "budget":
				to.ExecutionBudget.Policy.PolicyHash += "drift"
			case "dispatch":
				from.ActiveDispatch = &runtimecontract.RuntimeDispatchRecord{ID: "unsettled"}
			case "submitted":
				from.CandidateRequestSessions = append(from.CandidateRequestSessions, runtimecontract.RuntimeCandidateRequestSession{State: runtimecontract.RuntimeCandidateRequestSessionPrepared, ProviderSubmitted: true})
			}
			if validateRuntimeCWISuspensionTransition(from, to) == nil {
				t.Fatal("illegal consumer append accepted")
			}
		})
	}
}

func TestCWISuspensionAdmissionRejectsIncompleteConsumerSet(t *testing.T) {
	previous := runtimeSuspensionProjectionFixture(t)
	runtimeSecondSuspensionConsumer(&previous)
	next := previous.CloneRaw()
	if changed, err := suspendRuntimeSuccessorForStalePredecessor(&next); err != nil || !changed {
		t.Fatal(err)
	}
	next.StateRevision++
	if err := validateRuntimeCWISuspensionTransition(previous, next); err != nil {
		t.Fatal(err)
	}
	next.Facts.CWISuspensions[0].Consumers = next.Facts.CWISuspensions[0].Consumers[:1]
	if validateRuntimeCWISuspensionTransition(previous, next) == nil {
		t.Fatal("only first consumer admitted as complete set")
	}
}
