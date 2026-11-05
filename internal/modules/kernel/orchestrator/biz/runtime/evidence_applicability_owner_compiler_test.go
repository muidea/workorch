package runtime

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
)

func TestEvidenceApplicabilityOwnerProjectionAdmissionAndDetachedConsumers(t *testing.T) {
	base, _ := applicabilityTransitionFixture(t)
	a, err := AdmitRuntimeOwnerStateRecord(ownerStateRecord(base))
	if err != nil {
		t.Fatal(err)
	}
	want, wantSource, wantRequired, wantErr := CompileRuntimeEvidenceApplicabilityRead(base)
	for range 2 {
		got, source, required, e := a.EvidenceApplicabilityRead()
		if e != wantErr || got != want || required != wantRequired || !reflect.DeepEqual(source, wantSource) {
			t.Fatal("owner projection differs from independent compiler", e, wantErr)
		}
		if required {
			source.Operation.TargetRefs[0] = "returned mutation"
			source.ApplicabilitySource.OwnerRevisionSet.Entries[0].Revision = "returned mutation"
		}
	}
	// The current query is deterministic after serialization and has no shadow
	// owner flag. Its typed request must bind the new revision, never a prior view.
	wire, _ := json.Marshal(a)
	var restored RuntimeAggregate
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	got, _, required, e := restored.EvidenceApplicabilityRead()
	if e != nil || got != want || required != wantRequired {
		t.Fatal("restored projection differs", e)
	}
	a.StateRevision++
	newer, _, _, e := a.EvidenceApplicabilityRead()
	if e != nil || newer == got {
		t.Fatal("new revision reused old query", e)
	}

}

func TestEvidenceApplicabilityOwnerProjectionRejectsRealMissingAndTamperedSource(t *testing.T) {
	base, _ := applicabilityTransitionFixture(t)
	for _, arm := range []string{"header", "receipt", "budget", "evidence", "scope", "duplicate"} {
		t.Run(arm, func(t *testing.T) {
			a := base.CloneRaw()
			switch arm {
			case "header":
				a.ResultFeedbackVersion = 0
			case "receipt":
				a.OperationReceipts[0].AfterHash = "same identity altered receipt"
			case "budget":
				a.ExecutionBudget.Policy.PolicyRevision = "drift"
			case "evidence":
				a.Facts.Evidence.Accepted[0].Summary += "raw drift"
			case "scope":
				a.Facts.WorkspaceID = "other"
			case "duplicate":
				a.OperationReceipts = append(a.OperationReceipts, a.OperationReceipts[0])
			}
			if _, err := AdmitRuntimeOwnerStateRecord(ownerStateRecord(a)); err == nil {
				t.Fatal("raw record admitted invalid source")
			}
		})
	}
}

func TestEvidenceApplicabilityOwnerProjectionConcurrentWorkspaceIsolation(t *testing.T) {
	base, _ := applicabilityTransitionFixture(t)
	var group sync.WaitGroup
	for i := range 12 {
		group.Go(func() {
			a := base.CloneRaw()
			if i%2 != 0 {
				a.Facts.WorkspaceID = "unrelated"
				a.Facts.RunID = "unrelated"
			}
			_, want := AdmitRuntimeOwnerStateRecord(ownerStateRecord(a))
			_, got := AdmitRuntimeOwnerStateRecord(ownerStateRecord(a))
			if (want == nil) != (got == nil) {
				t.Error("concurrent owner admission crossed scope")
			}
		})
	}
	group.Wait()
}

func TestEvidenceApplicabilityControllerLoadAndEventReturnSameOwnerProjection(t *testing.T) {
	base, observation := applicabilityTransitionFixture(t)
	repo := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &base}}
	controller, _ := NewRuntimeController(repo)
	loaded, err := controller.Load(context.Background(), base.Facts.WorkspaceID, base.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	q, _, required, err := loaded.EvidenceApplicabilityRead()
	if err != nil || !required || q != observation.Request {
		t.Fatal("Load omitted owner projection", err)
	}
	// A read view never authorizes a receipt/budget/CAS. Original writer still
	// independently admits source and exact request on a duplicate or fresh call.
	if !reflect.DeepEqual(loaded.OperationReceipts, base.OperationReceipts) || !reflect.DeepEqual(loaded.ExecutionBudget, base.ExecutionBudget) {
		t.Fatal("query changed effects or budget")
	}
	event := RuntimeInternalEvent{Kind: RuntimeInternalEventEvidenceApplicabilityObserved,
		Scope:     RuntimeInternalEventScope{WorkspaceID: base.Facts.WorkspaceID, RunID: base.Facts.RunID},
		Authority: &RuntimeInternalAuthorityPayload{EvidenceApplicability: &observation}}
	result, err := controller.ApplyInternalEvent(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, required, err := result.Aggregate.EvidenceApplicabilityRead(); err != nil || required {
		t.Fatal("event omitted drained owner result", err)
	}
	duplicate, err := controller.ApplyInternalEvent(context.Background(), event)
	if err != nil || !reflect.DeepEqual(ownerStateRecord(result.Aggregate), ownerStateRecord(duplicate.Aggregate)) {
		t.Fatal("duplicate changed owner state", err)
	}
	if _, _, required, err := duplicate.Aggregate.EvidenceApplicabilityRead(); err != nil || required {
		t.Fatal("duplicate omitted read result", err)
	}
	if !reflect.DeepEqual(base.ExecutionBudget, result.Aggregate.ExecutionBudget) || !reflect.DeepEqual(base.OperationReceipts, result.Aggregate.OperationReceipts) {
		t.Fatal("observation invented effects or budget")
	}
}

func TestOwnerReadResultNeverAuthorizesCorruptWriter(t *testing.T) {
	base, observation := applicabilityTransitionFixture(t)
	admitted, err := AdmitRuntimeOwnerStateRecord(ownerStateRecord(base))
	if err != nil {
		t.Fatal(err)
	}
	// The transient read result is ordinary query data. Keeping it while
	// corrupting raw input cannot grant the writer admission, effect or budget.
	admitted.OperationReceipts[0].AfterHash = "tampered after admission"
	repo := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &admitted}}
	controller, _ := NewRuntimeController(repo)
	event := RuntimeInternalEvent{Kind: RuntimeInternalEventEvidenceApplicabilityObserved,
		Scope:     RuntimeInternalEventScope{WorkspaceID: base.Facts.WorkspaceID, RunID: base.Facts.RunID},
		Authority: &RuntimeInternalAuthorityPayload{EvidenceApplicability: &observation}}
	before := ownerStateRecord(*repo.aggregate).Clone()
	if _, err := controller.ApplyInternalEvent(context.Background(), event); err == nil {
		t.Fatal("read projection bypassed writer admission")
	}
	if !reflect.DeepEqual(before, ownerStateRecord(*repo.aggregate)) {
		t.Fatal("rejected input changed persisted state")
	}
}
