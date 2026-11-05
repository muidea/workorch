package runtime

import (
	"fmt"
	"reflect"
	"testing"
)

func assertReceiptJoinInputProjection(t *testing.T, base RuntimeAggregate) {
	t.Helper()
	if err := validateRuntimeAggregateReceiptJoins(base); err != nil {
		t.Fatal("fixture must have a valid exact receipt relation", err)
	}
	check := func(a RuntimeAggregate) {
		t.Helper()
		original := a.CloneRaw()
		want := validateRuntimeAggregateReceiptJoins(a)
		if got := validateRuntimeReceiptJoinTransition(nil, a); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("cold typed receipt inputs changed original rejection: %v != %v", got, want)
		}
		if got := validateRuntimeReceiptJoinTransition(&base, a); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("formal preimage hid changed receipt source: %v != %v", got, want)
		}
		if !reflect.DeepEqual(original, a) {
			t.Fatal("receipt source consumption changed original bytes")
		}
	}
	check(base)
	for _, body := range []bool{false, true} {
		value := base.CloneRaw()
		if body {
			value = runtimeTestEvidenceAggregateRoundTrip(t, value)
		}
		// Independent cold comparison covers every aggregate and Facts field,
		// including scope, evidence membership, receipt, dispatch and fences.
		for i := 0; i < reflect.TypeOf(value).NumField(); i++ {
			a := value.CloneRaw()
			field := reflect.ValueOf(&a).Elem().Field(i)
			field.Set(reflect.Zero(field.Type()))
			check(a)
		}
		for i := 0; i < reflect.TypeOf(value.Facts).NumField(); i++ {
			a := value.CloneRaw()
			field := reflect.ValueOf(&a.Facts).Elem().Field(i)
			field.Set(reflect.Zero(field.Type()))
			check(a)
		}
		for _, arm := range []string{"evidence-body", "receipt-body", "execution-fence", "duplicate-event", "orphan-evidence"} {
			t.Run(arm, func(t *testing.T) {
				a := value.CloneRaw()
				switch arm {
				case "evidence-body":
					a.Facts.Evidence.Accepted[0].Summary += "原文变化"
				case "receipt-body":
					a.OperationReceipts[0].Evidence.Summary += "原文变化"
				case "execution-fence":
					a.CompletedDispatches[0].OperationExecutions[0].Fence++
				case "duplicate-event":
					a.ProcessedEventIDs = append(a.ProcessedEventIDs, a.ProcessedEventIDs[0])
				case "orphan-evidence":
					a.Facts.Evidence.Accepted[0].ID += "-orphan"
				}
				check(a)
				if validateRuntimeReceiptJoinTransition(&base, a) == nil {
					t.Fatal("changed source was not rejected")
				}
			})
		}
	}
}

func TestReceiptJoinTransitionConsumesOnlyExactCommittedSources(t *testing.T) {
	assertReceiptJoinInputProjection(t, validationAggregate(t))
}

func TestReceiptJoinSourceProjectionCompleteArchive(t *testing.T) {
	a, _ := reconciliationScanArchive(t)
	assertReceiptJoinInputProjection(t, a)
}
