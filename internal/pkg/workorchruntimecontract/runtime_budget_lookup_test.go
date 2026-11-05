package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"testing"
)

func budgetLookupFixture(t testing.TB, count int) RuntimeExecutionBudgetLedger {
	t.Helper()
	l, err := NewRuntimeExecutionBudgetLedger(DefaultRuntimeExecutionBudgetPolicy())
	if err != nil {
		t.Fatal(err)
	}
	d, _ := RuntimeBudgetDimensionPolicyFor(l.Policy, RuntimeBudgetDimensionProviderCall)
	states := []RuntimeBudgetAllocationState{RuntimeBudgetAllocationConfirmed, RuntimeBudgetAllocationUnknown, RuntimeBudgetAllocationReserved, RuntimeBudgetAllocationReleased, RuntimeBudgetAllocationExhausted}
	for i := range count {
		a := RuntimeBudgetAllocation{Dimension: d.Dimension, Unit: d.Unit, PolicyRevision: l.Policy.PolicyRevision,
			LimitAtAllocation: d.Limit, MaxIncrement: d.MaxIncrement, ReservedAmount: 1,
			LogicalConsumptionID: fmt.Sprintf("logical-%d", i), ScopeRef: fmt.Sprintf("scope-%d", i), State: states[i%len(states)]}
		if a.State != RuntimeBudgetAllocationReserved {
			a.OwnerFactRef = fmt.Sprintf("owner-original-%d", i)
		}
		a.AllocationID = StableRuntimeBudgetAllocationID(a)
		a.AllocationHash = StableRuntimeBudgetAllocationHash(a)
		l.Allocations = append(l.Allocations, a)
		if a.State == RuntimeBudgetAllocationReserved || a.State == RuntimeBudgetAllocationReleased {
			continue
		}
		f := RuntimeBudgetConsumptionFact{AllocationID: a.AllocationID, LogicalConsumptionID: a.LogicalConsumptionID,
			Owner: RuntimeModuleLLM, PhysicalConsumptionID: fmt.Sprintf("physical-%d", i), Unit: a.Unit,
			OwnerResultRef: a.OwnerFactRef, OwnerResultHash: "original-owner-hash"}
		if a.State == RuntimeBudgetAllocationConfirmed {
			f.ConfirmedAmount = 1
		} else {
			f.UnknownAmount = 1
		}
		f.ConsumptionID = StableRuntimeBudgetConsumptionID(f)
		f.ConsumptionHash = StableRuntimeBudgetConsumptionHash(f)
		l.Consumptions = append(l.Consumptions, f)
	}
	if err := referenceBudgetBeforeLookupValidate(l); err != nil {
		t.Fatal(err)
	}
	return l
}

func compareBudgetLookup(t testing.TB, l RuntimeExecutionBudgetLedger) {
	t.Helper()
	before, _ := json.Marshal(l)
	if got, want := l.Validate(), referenceBudgetBeforeLookupValidate(l); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("budget admission first error: %v != %v", got, want)
	}
	for _, usage := range [][]RuntimeExecutionBudgetSourceUsage{nil, {{}}, {{}, {}}} {
		got, gotErr := BuildRuntimeExecutionBudgetView("workspace", "run", 7, l, " wait-original ", " terminal-original ", usage...)
		want, wantErr := referenceBudgetBeforeLookupView("workspace", "run", 7, l, " wait-original ", " terminal-original ", usage...)
		if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) || !reflect.DeepEqual(got, want) {
			t.Fatalf("budget view/first error: %v != %v", gotErr, wantErr)
		}
	}
	for _, d := range l.Policy.Dimensions {
		got, gotErr := RuntimeBudgetRemaining(l, d.Dimension)
		want, wantErr := referenceBudgetBeforeLookupRemaining(l, d.Dimension)
		if got != want || fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
			t.Fatalf("budget remaining/first error: %v != %v", gotErr, wantErr)
		}
	}
	after, _ := json.Marshal(l)
	if string(before) != string(after) {
		t.Fatal("budget lookup changed original input")
	}
}

func TestBudgetLookupMatchesOriginalAdmissionAndProjection(t *testing.T) {
	l := budgetLookupFixture(t, 10)
	compareBudgetLookup(t, l)
	for _, field := range []string{"Allocations", "Consumptions", "Reconciliations"} {
		changed := l.Clone()
		ownerLineageFieldMutations(reflect.ValueOf(&changed).Elem().FieldByName(field), func() { compareBudgetLookup(t, changed) })
	}
	for _, mutate := range []func(*RuntimeExecutionBudgetLedger){
		func(l *RuntimeExecutionBudgetLedger) { l.Consumptions = append(l.Consumptions, l.Consumptions[0]) },
		func(l *RuntimeExecutionBudgetLedger) {
			f := l.Consumptions[0]
			f.PhysicalConsumptionID = "second-physical"
			f.ConsumptionID, f.ConsumptionHash = StableRuntimeBudgetConsumptionID(f), StableRuntimeBudgetConsumptionHash(f)
			l.Consumptions = append(l.Consumptions, f)
		},
		func(l *RuntimeExecutionBudgetLedger) { l.Consumptions = l.Consumptions[1:] },
		func(l *RuntimeExecutionBudgetLedger) {
			f := l.Consumptions[0]
			f.ConsumptionID, f.ConfirmedAmount, f.UnknownAmount = " a-duplicate-first ", 0, 1
			l.Consumptions = append(l.Consumptions, f)
			slices.Reverse(l.Consumptions)
		},
		func(l *RuntimeExecutionBudgetLedger) { l.Allocations = append(l.Allocations, l.Allocations[0]) },
		func(l *RuntimeExecutionBudgetLedger) { slices.Reverse(l.Allocations); slices.Reverse(l.Consumptions) },
		func(l *RuntimeExecutionBudgetLedger) {
			l.Consumptions[0].AllocationID = " " + l.Consumptions[0].AllocationID + " "
		},
	} {
		changed := l.Clone()
		mutate(&changed)
		compareBudgetLookup(t, changed)
	}
	body, _ := json.Marshal(l)
	var loaded RuntimeExecutionBudgetLedger
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	compareBudgetLookup(t, loaded)
	// Fresh calls use their actual input, never an earlier view or acceptance.
	loaded.Consumptions[0].OwnerResultHash = "same-id raw drift"
	compareBudgetLookup(t, loaded)
	compareBudgetLookup(t, l)
}

func TestBudgetLookupReadOnlyArchive(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		compareBudgetLookup(t, run.RuntimeState.ExecutionBudget)
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func BenchmarkBudgetLookupScaling(b *testing.B) {
	for _, count := range []int{32, 128, 512} {
		l := budgetLookupFixture(b, count)
		for _, old := range []bool{true, false} {
			b.Run(fmt.Sprintf("rows_%d/original_%t", count, old), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var err error
					if old {
						err = referenceBudgetBeforeLookupValidate(l)
					} else {
						err = l.Validate()
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
