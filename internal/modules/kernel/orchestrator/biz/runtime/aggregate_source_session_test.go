package runtime

import (
	"encoding/json"
	"reflect"
	"testing"
)

func assertAggregateSourceSession(t testing.TB, a RuntimeAggregate) {
	t.Helper()
	original := a.CloneRaw()
	for range 2 {
		got, want := a.Validate(), referenceAggregateSourceSessionValidate(a)
		if (got == nil) != (want == nil) {
			t.Fatalf("complete aggregate first error differs: %v != %v", got, want)
		}
	}
	if !reflect.DeepEqual(a, original) {
		t.Fatal("aggregate source validation changed original")
	}
	body, _ := json.Marshal(a)
	var loaded RuntimeAggregate
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	if (loaded.Validate() == nil) != (referenceAggregateSourceSessionValidate(loaded) == nil) {
		t.Fatal("reloaded aggregate changed source validation")
	}
}

func TestAggregateSourceSessionCompleteOrderAndLaterCalls(t *testing.T) {
	for _, base := range aggregateDispatchReuseFixtures(t) {
		assertAggregateSourceSession(t, base)
		for _, mutate := range []func(*RuntimeAggregate){
			func(a *RuntimeAggregate) { a.Decision.RequiredOutput.ImplementationWorkBindingHash = "missing" },
			func(a *RuntimeAggregate) {
				a.ImplementationWork.PurposeChoices = append(a.ImplementationWork.PurposeChoices, a.ImplementationWork.PurposeChoices...)
			},
			func(a *RuntimeAggregate) { a.EvidenceSequence++ },
			func(a *RuntimeAggregate) { a.Facts.Assessments = nil },
			func(a *RuntimeAggregate) { a.OperationReceipts = nil },
			func(a *RuntimeAggregate) { a.Facts.WorkspaceID = "" },
			func(a *RuntimeAggregate) { a.Facts.DefinitionRevision = "raw\x00drift" },
			func(a *RuntimeAggregate) { a.StateRevision = 0 },
			func(a *RuntimeAggregate) {
				a.OperationReceipts = nil
				a.EvidenceSequence++
				a.Decision.RequiredOutput.ImplementationWorkBindingHash = "missing"
			},
		} {
			changed := base.CloneRaw()
			mutate(&changed)
			assertAggregateSourceSession(t, changed)
			assertAggregateSourceSession(t, base)
		}
	}
}
