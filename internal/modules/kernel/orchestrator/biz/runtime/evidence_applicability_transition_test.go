package runtime

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestEvidenceApplicabilityResumeGenerationRequiresFreshObservationWithoutSemanticDrift(t *testing.T) {
	before, observation := applicabilityTransitionFixture(t)
	repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &before}}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := controller.ApplyInternalEvent(ctx, applicabilityInternalEvent(observation))
	if err != nil {
		t.Fatal(err)
	}
	if _, required, err := NextRuntimeEvidenceApplicabilityRequest(first.Aggregate); err != nil || required {
		t.Fatalf("same generation requery: %v", err)
	}
	contract := first.Aggregate.Facts.ReconciliationContract
	request := runtimecontract.RuntimeCWIReconciliationRequest{RequestID: "resume-generation-1", WorkspaceID: before.Facts.WorkspaceID, RunID: before.Facts.RunID, Scope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, DefinitionRevision: contract.DefinitionRevision, BindingSetHash: contract.BindingSetHash, Bindings: contract.Bindings, WorkspaceBaselineRevision: "actual-tree-1", WorkspaceBaselineRef: "workspace-tree/actual-tree-1", Generation: 1}
	for generation := uint64(1); generation <= 2; generation++ {
		request.Generation = generation
		if generation == 2 {
			request.RequestID = "resume-generation-2"
			request.WorkspaceBaselineRevision = "actual-tree-2"
			request.WorkspaceBaselineRef = "workspace-tree/actual-tree-2"
		}
		advanced, changed, err := controller.ensureReconciliationRequest(ctx, request)
		if err != nil || !changed {
			t.Fatalf("new reconciliation generation: %v", err)
		}
		// New controller + raw JSON roundtrip: no process-local query cache.
		reloaded := runtimeTestEvidenceAggregateRoundTrip(t, advanced)
		repository.aggregate = &reloaded
		controller, err = NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		fresh, required, err := NextRuntimeEvidenceApplicabilityRequest(reloaded)
		if err != nil || !required || fresh.ReconciliationRequestHash == "" || fresh.ReconciliationRequestHash != advanced.Reconciliation.RequestHash || fresh.RequestID == observation.Request.RequestID || fresh.EvidenceRevision != observation.Request.EvidenceRevision {
			t.Fatalf("fresh generation reused old observation: required=%v err=%v", required, err)
		}
		fact := reloaded.Facts.Evidence.Accepted[0]
		if runtimecontract.RuntimeEvidenceHasFreshApplicability(fact, reloaded.Facts.EvidenceApplicabilityObservations, reloaded.Facts.EvidenceRevision, reloaded.Reconciliation.RequestHash) {
			t.Fatal("pure consumer accepted old-generation observation")
		}
		_, decision, err := reduceRuntimeAggregateWithDirective(reloaded, reloaded.Facts, &reloaded.Decision.AuthoritySnapshot)
		if err != nil || decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(decision.Prerequisites) != 1 || decision.Prerequisites[0].Kind != runtimecontract.RuntimeEvidenceApplicabilityPrerequisiteV1 || len(decision.RequiredOutput.AcceptedKinds) != 0 {
			t.Fatalf("new generation invented an executable proof gap or sealed success: state=%s err=%v", decision.State, err)
		}
		if err := validateCandidateAttemptBeginAuthority(reloaded); err == nil {
			t.Fatal("assessment candidate began before fresh current-generation query")
		}
		semanticHash := runtimecontract.StableRuntimeEvidenceApplicabilitySemanticHash(reloaded.Facts.EvidenceApplicabilityObservations)
		duplicateResult := observation.CloneRaw()
		duplicateResult.Request = fresh
		duplicateResult.RecordedAtStateRevision = fresh.IssuedAtStateRevision + 1
		result, err := controller.ApplyInternalEvent(ctx, applicabilityInternalEvent(duplicateResult))
		if err != nil {
			t.Fatal(err)
		}
		if runtimecontract.StableRuntimeEvidenceApplicabilitySemanticHash(result.Aggregate.Facts.EvidenceApplicabilityObservations) != semanticHash || !reflect.DeepEqual(result.Aggregate.ExecutionBudget, before.ExecutionBudget) || !reflect.DeepEqual(result.Aggregate.OperationReceipts, before.OperationReceipts) {
			t.Fatal("identical fresh query changed semantics/effect/budget")
		}
		if _, required, err := NextRuntimeEvidenceApplicabilityRequest(result.Aggregate); err != nil || required {
			t.Fatalf("committed generation repeated query: %v", err)
		}
		if !runtimecontract.RuntimeEvidenceHasFreshApplicability(fact, result.Aggregate.Facts.EvidenceApplicabilityObservations, result.Aggregate.Facts.EvidenceRevision, result.Aggregate.Reconciliation.RequestHash) {
			t.Fatal("fresh matching owner result was not consumable")
		}
	}
}

func applicabilityTransitionFixture(t *testing.T) (RuntimeAggregate, runtimecontract.RuntimeEvidenceApplicabilityObservation) {
	t.Helper()
	reads := (runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: "definition-v2", Entries: []runtimecontract.RuntimeOwnerRevisionRef{{OwnerKind: "capability", OwnerInstanceID: "typed-owner/ws-v2", ResourceKind: "workspace_target", ResourceID: "target", Revision: "historical-input", DependencyHash: "read-selector"}}}).WithHash()
	current := *runtimecontract.CloneRuntimeOwnerRevisionSet(&reads)
	current.Entries[0].ResourceKind = "workspace_target_content"
	current.Entries[0].Revision, current.Entries[0].DependencyHash = "postcondition", "postcondition-selector"
	current = current.WithHash()
	proof := &runtimecontract.RuntimeEvidenceApplicabilityDependencies{ProtocolRef: runtimecontract.RuntimeEvidenceApplicabilityDependenciesProtocol, OwnerRevisionSet: current}
	_, prepared, _ := prepareOwnerFactAssessmentDependencyBoundary(t, true, nil, &reads, proof, false)
	aggregate := prepared.aggregate.CloneRaw()
	if aggregate.ActiveDispatch != nil {
		t.Fatal("fixture must stop before assessment candidate")
	}
	request, err := CompileRuntimeEvidenceApplicabilityRequest(aggregate, aggregate.Facts.Evidence.Accepted[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	return aggregate, runtimecontract.RuntimeEvidenceApplicabilityObservation{Request: request, RecordedAtStateRevision: request.IssuedAtStateRevision + 1, CurrentDependencies: &current}
}

func applicabilityInternalEvent(observation runtimecontract.RuntimeEvidenceApplicabilityObservation) RuntimeInternalEvent {
	return RuntimeInternalEvent{Kind: RuntimeInternalEventEvidenceApplicabilityObserved, Scope: RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"}, Authority: &RuntimeInternalAuthorityPayload{EvidenceApplicability: &observation}}
}

func TestApplicabilityRevisionFenceIncludesCompletedFocusBeforeCurrentClosure(t *testing.T) {
	aggregate, observation := applicabilityTransitionFixture(t)
	facts := aggregate.CloneRaw().Facts
	// Typed reducer fixture only: a later focus cannot hide an earlier current
	// proof. The production mainline independently exercises actual owners/CAS.
	facts.CompletedFocuses = append(facts.CompletedFocuses, facts.Focus)
	facts.Focus = runtimecontract.CoverageFocus{ID: "later-focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{"later-cwi"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}}
	if !runtimeApplicabilityRevisionPending(facts, aggregate.CoverageProofLineage) {
		t.Fatal("completed-focus current proof escaped new-revision sampling fence")
	}
	decision, pending, err := runtimeApplicabilityPrerequisiteDecision(runtimecontract.RuntimeDecision{Focus: facts.Focus}, facts, &aggregate, aggregate.CoverageProofLineage)
	if err != nil || !pending || decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || decision.Terminal != nil || len(decision.RequiredOutput.AcceptedKinds) != 0 || decision.Prerequisites[0].SubjectRef != string(facts.Evidence.Accepted[0].ID) {
		t.Fatalf("earlier proof read became an unrelated executable target gap: state=%s pending=%v err=%v", decision.State, pending, err)
	}
	// A projection is not an authorable/bootstrap requirement or a generic
	// owner-fact acquisition. Only the original current-read transaction may
	// resolve it; it does not fabricate a new resource observation.
	if runtimecontract.RuntimePrerequisiteKindSupported(decision.Prerequisites[0].Kind) {
		t.Fatal("decision-only current-read debt entered generic acquisition inventory")
	}
	facts.Focus.RequireAssessment = false
	if _, pending, err := runtimeApplicabilityPrerequisiteDecision(runtimecontract.RuntimeDecision{Focus: facts.Focus}, facts, &aggregate, aggregate.CoverageProofLineage); err != nil || !pending {
		t.Fatal("read debt vanished without an assessment/verification-policy guard")
	}
	facts.EvidenceApplicabilityObservations = []runtimecontract.RuntimeEvidenceApplicabilityObservation{observation}
	if runtimeApplicabilityRevisionPending(facts, aggregate.CoverageProofLineage) {
		t.Fatal("exact observed completed-focus source remained pending")
	}
	facts.EvidenceRevision += "1"
	if !runtimeApplicabilityRevisionPending(facts, aggregate.CoverageProofLineage) {
		t.Fatal("old completed-focus observation became fresh for a later revision")
	}
	facts.CompletedFocuses = nil
	if runtimeApplicabilityRevisionPending(facts, aggregate.CoverageProofLineage) {
		t.Fatal("historical source without current binding blocked unrelated proof")
	}
}

func TestEvidenceApplicabilityObservationIsRevisionFencedDetachedAndIdempotent(t *testing.T) {
	before, observation := applicabilityTransitionFixture(t)
	repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &before}}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := controller.ApplyInternalEvent(context.Background(), applicabilityInternalEvent(observation))
	if err != nil {
		t.Fatal(err)
	}
	if result.Aggregate.StateRevision != before.StateRevision+1 || repository.saves != 1 || len(result.Aggregate.Facts.EvidenceApplicabilityObservations) != 1 {
		t.Fatal("owner observation was not committed exactly once")
	}
	if !reflect.DeepEqual(before.OperationReceipts, result.Aggregate.OperationReceipts) || !reflect.DeepEqual(before.Facts.Evidence, result.Aggregate.Facts.Evidence) || !reflect.DeepEqual(before.ExecutionBudget, result.Aggregate.ExecutionBudget) || !reflect.DeepEqual(before.Facts.Coverage, result.Aggregate.Facts.Coverage) {
		t.Fatal("matching observation changed original receipt, effect, budget or credit")
	}
	for _, copy := range []RuntimeAggregate{result.Aggregate.CloneRaw(), runtimeTestEvidenceAggregateRoundTrip(t, result.Aggregate)} {
		if err := copy.Validate(); err != nil {
			t.Fatal(err)
		}
		copy.Facts.EvidenceApplicabilityObservations[0].CurrentDependencies.Entries[0].Revision = "caller-change"
		if result.Aggregate.Facts.EvidenceApplicabilityObservations[0].CurrentDependencies.Entries[0].Revision != "postcondition" {
			t.Fatal("observation clone/reload shared original current bytes")
		}
	}
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := restarted.ApplyInternalEvent(context.Background(), applicabilityInternalEvent(observation))
	if err != nil || repository.saves != 1 || !reflect.DeepEqual(replay.Aggregate, result.Aggregate) {
		t.Fatalf("exact duplicate changed persisted source: %v", err)
	}
	changed := observation.CloneRaw()
	changed.CurrentDependencies.Entries[0].Revision = "different-response"
	*changed.CurrentDependencies = changed.CurrentDependencies.WithHash()
	if _, err := restarted.ApplyInternalEvent(context.Background(), applicabilityInternalEvent(changed)); err == nil || repository.saves != 1 {
		t.Fatal("changed duplicate bypassed original observation")
	}
}

func TestApplicabilityReadDebtRejectsForgedProjectionAndGenericAcquisition(t *testing.T) {
	before, _ := applicabilityTransitionFixture(t)
	if before.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(before.Decision.Prerequisites) != 1 || before.Validate() != nil {
		t.Fatal("fixture lacks valid original current-read debt")
	}
	for _, field := range []string{"identity", "source", "owner", "operation-authority", "generic-facts"} {
		t.Run(field, func(t *testing.T) {
			forged := before.CloneRaw()
			switch field {
			case "identity":
				forged.Decision.Prerequisites[0].ID = "caller-chosen-read-debt"
				forged.Decision.RequiredOutput.PrerequisiteIDs = []string{forged.Decision.Prerequisites[0].ID}
			case "source":
				forged.Decision.Prerequisites[0].SubjectRef = "unobserved-source"
			case "owner":
				forged.Decision.Prerequisites[0].OwnerModuleID = "caller-owner"
			case "operation-authority":
				forged.Decision.RequiredOutput.AcceptedKinds = []string{runtimecontract.StepKindFileWrite}
			case "generic-facts":
				forged.Facts.Prerequisites.Pending = append(forged.Facts.Prerequisites.Pending, forged.Decision.Prerequisites[0])
			}
			// Coordinated snapshot/hash copies cannot substitute for original
			// source admission or turn the projection into owner acquisition.
			sealed, err := runtimecontract.SealRuntimeDecision(forged.Facts, forged.Decision, 0)
			if err == nil {
				forged.Decision = sealed
				if forged.Validate() == nil {
					t.Fatal("forged read-debt projection crossed full aggregate gate")
				}
			}
		})
	}
	// Query/seal guards do not spend another attempt while a read is owed.
	repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &before}}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "premature-read-debt"); err == nil || repository.saves != 0 || !reflect.DeepEqual(*repository.aggregate, before) {
		t.Fatal("read-only prerequisite debt admitted candidate/provider authority or spent budget")
	}
}

func TestEvidenceApplicabilityObservationRejectsStaleAndForgedSourcesWithoutCAS(t *testing.T) {
	for _, name := range []string{"late", "evidence", "receipt", "dispatch", "operation", "raw", "identity", "definition", "missing_pair", "envelope"} {
		t.Run(name, func(t *testing.T) {
			before, observation := applicabilityTransitionFixture(t)
			repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &before}}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "late":
				observation.Request.IssuedAtStateRevision--
				observation.RecordedAtStateRevision--
			case "evidence":
				observation.Request.SourceEvidenceHash = "forged"
			case "receipt":
				observation.Request.SourceReceiptHash = "forged"
			case "dispatch":
				observation.Request.DispatchID = "other"
			case "operation":
				observation.Request.OperationID = "other"
			case "raw":
				observation.CurrentDependencies.Entries[0].Revision += " "
			case "identity":
				observation.CurrentDependencies.Entries[0].ResourceID = "other"
				*observation.CurrentDependencies = observation.CurrentDependencies.WithHash()
			case "definition":
				observation.CurrentDependencies.DefinitionRevision = "other"
				*observation.CurrentDependencies = observation.CurrentDependencies.WithHash()
			case "missing_pair":
				observation.Missing = true
			}
			observation.Request = observation.Request.WithID()
			event := applicabilityInternalEvent(observation)
			if name == "envelope" {
				event.Authority.CloseActiveDispatch = true
			}
			if _, err := controller.ApplyInternalEvent(context.Background(), event); err == nil || repository.saves != 0 || !reflect.DeepEqual(*repository.aggregate, before) {
				t.Fatal("stale/forged owner observation changed aggregate")
			}
		})
	}
}

func TestEvidenceApplicabilityObservationCASFailureAndResponseLossPreserveOriginal(t *testing.T) {
	for _, afterCommit := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_cas", true: "after_cas_response_loss"}[afterCommit], func(t *testing.T) {
			before, observation := applicabilityTransitionFixture(t)
			repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &before}, failBefore: !afterCommit, failAfter: afterCommit}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			event := applicabilityInternalEvent(observation)
			if _, err := controller.ApplyInternalEvent(context.Background(), event); err == nil {
				t.Fatal("CAS fault was lost")
			}
			persisted, err := repository.Load(context.Background(), "ws-v2", "run-v2")
			if err != nil || persisted.Validate() != nil || (!afterCommit && !reflect.DeepEqual(before, persisted)) {
				t.Fatalf("fault damaged original aggregate: %v", err)
			}
			restarted, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			result, err := restarted.ApplyInternalEvent(context.Background(), event)
			if err != nil || repository.saves != 1 || len(result.Aggregate.Facts.EvidenceApplicabilityObservations) != 1 || result.Aggregate.StateRevision != before.StateRevision+1 {
				t.Fatalf("restart did not retain exact single observation CAS: %v", err)
			}
		})
	}
}

func TestEvidenceApplicabilityStaleObservationReopensOnlyOriginalCredit(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "changed_bytes", true: "missing_resource"}[missing], func(t *testing.T) {
			before, observation := applicabilityTransitionFixture(t)
			// Explicit already-credited CWI fixture, as in reconciliation tests.
			// This is not a producer of acceptance or an end-to-end completion oracle.
			before.Facts.Coverage[0].State = runtimecontract.RuntimeCoverageItemStateSatisfied
			before.Facts.Coverage[0].CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateSettled
			if err := before.Validate(); err != nil {
				t.Fatalf("credited source fixture: %v", err)
			}
			if missing {
				observation.Missing, observation.CurrentDependencies = true, nil
			} else {
				observation.CurrentDependencies.Entries[0].Revision = "later-bytes"
				*observation.CurrentDependencies = observation.CurrentDependencies.WithHash()
			}
			repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &before}}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			result, err := controller.ApplyInternalEvent(context.Background(), applicabilityInternalEvent(observation))
			if err != nil {
				t.Fatal(err)
			}
			item := result.Aggregate.Facts.Coverage[0]
			if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied || item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateReopened || item.CurrentBindingRef != nil || len(item.ReopenLineage) != 1 || item.ReopenLineage[0].TriggerRef != observation.Request.RequestID {
				t.Fatal("stale observation retained credit or lost exact reopen provenance")
			}
			if !runtimecontract.RuntimeEvidenceHasStaleApplicability(before.Facts.Evidence.Accepted[0], result.Aggregate.Facts.EvidenceApplicabilityObservations, result.Aggregate.Facts.EvidenceRevision) {
				t.Fatal("original evidence remained current after its precise dependency changed")
			}
			if !reflect.DeepEqual(before.OperationReceipts, result.Aggregate.OperationReceipts) || !reflect.DeepEqual(before.Facts.Evidence, result.Aggregate.Facts.Evidence) || !reflect.DeepEqual(before.ExecutionBudget, result.Aggregate.ExecutionBudget) || repository.saves != 1 {
				t.Fatal("stale/reopen changed historical receipt/effect/budget or repeated CAS")
			}
			if err := runtimeTestEvidenceAggregateRoundTrip(t, result.Aggregate).Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Complete read compilation must match the independent public gates, including
// first errors, while returning detached query data without any owner write.
func TestEvidenceApplicabilityReadMatchesIndependentCompleteGates(t *testing.T) {
	base, _ := applicabilityTransitionFixture(t)
	for _, arm := range []string{"unchanged", "header", "receipt", "budget", "evidence", "scope"} {
		t.Run(arm, func(t *testing.T) {
			a := base.CloneRaw()
			switch arm {
			case "header":
				a.ResultFeedbackVersion = 0
			case "receipt":
				a.OperationReceipts[0].AfterHash = "altered original receipt"
			case "budget":
				a.ExecutionBudget.Policy.PolicyRevision = "altered current policy"
			case "evidence":
				a.Facts.Evidence.Accepted[0].Summary += "altered original evidence"
			case "scope":
				a.Facts.WorkspaceID = "different-workspace"
			}
			before := a.CloneRaw()
			want, required, wantErr := NextRuntimeEvidenceApplicabilityRequest(a)
			got, source, gotRequired, gotErr := CompileRuntimeEvidenceApplicabilityRead(a)
			if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) || required != gotRequired || !reflect.DeepEqual(got, want) {
				t.Fatalf("complete read changed request/first error: %v / %v", gotErr, wantErr)
			}
			if gotErr == nil && gotRequired {
				independent, err := CompileRuntimeOperationReadSource(a, want.DispatchID, want.OperationID, want.SourceReceiptHash)
				if err != nil || !reflect.DeepEqual(independent, source) {
					t.Fatalf("read source differs from independent exact compiler: %v", err)
				}
				source.OriginalDependencies.Entries[0].Revision = "caller mutation"
				source.Operation.TargetRefs[0] = "caller mutation"
				source.ApplicabilitySource.OwnerRevisionSet.Entries[0].Revision = "caller mutation"
			}
			if !reflect.DeepEqual(a, before) {
				t.Fatal("read compile/result mutation changed original authority")
			}
		})
	}
}
