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
	ref := a.Facts.Focus.CoverageEvidenceBindings[0].ID
	fact := runtimecontract.EvidenceFact{ID: "real-effect", Kind: runtimecontract.EvidenceKindStateChange, BindingRefs: []string{ref}, CoverageWorkItemIDs: []common.CoverageWorkItemID{id}, DispatchID: "completed-effect", OperationID: "effect"}
	a.Facts.Evidence.Accepted = append(a.Facts.Evidence.Accepted, fact)
	a.CompletedDispatches = append(a.CompletedDispatches, SealedDispatch{ID: fact.DispatchID, OperationExecutions: []runtimecontract.RuntimeOperationExecutionRecord{{OperationID: fact.OperationID, State: runtimecontract.OperationExecutionEvidenceCommitted, EvidenceFactID: fact.ID, EvidenceFactHash: runtimecontract.StableRuntimeEvidenceFactHash(fact), ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable}}})
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
	if got := runtimecontract.RuntimeCWIsEligibleForPlanning(probe); !reflect.DeepEqual(got, []common.CoverageWorkItemID{pause.PredecessorID}) {
		t.Fatalf("pause selected unrelated source: %v", got)
	}
	if a.Facts.Focus.CoverageWorkItemIDs[0] != pause.PredecessorID || !reflect.DeepEqual(pause.FrozenFocus, before.Facts.Focus) || !reflect.DeepEqual(pause.Repair, before.Facts.Repair) || !reflect.DeepEqual(a.OperationReceipts, before.OperationReceipts) || !reflect.DeepEqual(a.ExecutionBudget, before.ExecutionBudget) {
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
		if a.Facts.Coverage[i].ID == pause.PredecessorID {
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
					if facts.Coverage[i].ID != pause.PredecessorID {
						facts.Coverage[i].CurrentBindingRef = nil
					}
				}
			case "source":
				for i := range facts.Coverage {
					if facts.Coverage[i].ID == pause.PredecessorID {
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
				a.ActiveDispatch = &SealedDispatch{ID: "in-flight"}
			case "submitted":
				a.CandidateRequestSessions = append(a.CandidateRequestSessions, runtimecontract.RuntimeCandidateRequestSession{ProviderSubmitted: true, State: runtimecontract.RuntimeCandidateRequestSessionPrepared})
			case "terminal":
				a.Decision.State = runtimecontract.ConvergenceStateFailed
			case "material":
				a.TerminalMaterialSeal = &runtimecontract.RuntimeTerminalMaterialSeal{}
			case "nested":
				a.Facts.CWISuspensions = []runtimecontract.RuntimeCWISuspension{{ProtocolRef: runtimecontract.RuntimeCWISuspensionV1}}
			}
			before := a.CloneRaw()
			if changed, err := suspendRuntimeSuccessorForStalePredecessor(&a); err != nil || changed || !reflect.DeepEqual(before, a) {
				t.Fatal("unsafe window changed authority")
			}
		})
	}
}

func TestCWISuspensionMissingOrdinarySourceUnknownAndCapacityFailClosed(t *testing.T) {
	for _, scenario := range []string{"undeclared", "unknown", "capacity", "no_effect", "no_binding", "exhausted"} {
		t.Run(scenario, func(t *testing.T) {
			a := runtimeSuspensionProjectionFixture(t)
			switch scenario {
			case "undeclared":
				a.Facts.DefinitionFocuses[len(a.Facts.DefinitionFocuses)-1].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
			case "unknown":
				a.CompletedDispatches[0].OperationExecutions[0].State = runtimecontract.OperationExecutionOutcomeUnknown
			case "capacity":
				for range runtimecontract.RuntimeCWISuspensionLimit {
					a.Facts.CWISuspensions = append(a.Facts.CWISuspensions, runtimecontract.RuntimeCWISuspension{ResumeBinding: &runtimecontract.RuntimeCWIBindingRef{}})
				}
			case "no_effect":
				a.Facts.Evidence.Accepted = nil
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
