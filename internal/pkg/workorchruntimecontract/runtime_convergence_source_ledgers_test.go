package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"testing"
)

func assertConvergenceSourceLedgerOracle(t testing.TB, r RuntimeStateRecord) {
	t.Helper()
	before, _ := json.Marshal(r)
	s := historicalSessionArchiveSources(r)
	for range 2 {
		got := ValidateRuntimeConvergenceSourceLedgers(s, r.Decision.RequiredOutput, r.StateRevision)
		want := referenceConvergenceSourceLedgers(s, r.Decision.RequiredOutput, r.StateRevision)
		if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
			t.Fatalf("ordered source gate first error differs: %v != %v", got, want)
		}
	}
	after, _ := json.Marshal(r)
	if !bytes.Equal(before, after) {
		t.Fatal("source gate changed original raw input")
	}
	var loaded RuntimeStateRecord
	if err := json.Unmarshal(before, &loaded); err != nil {
		t.Fatal(err)
	}
	s = historicalSessionArchiveSources(loaded)
	got := ValidateRuntimeConvergenceSourceLedgers(s, loaded.Decision.RequiredOutput, loaded.StateRevision)
	want := referenceConvergenceSourceLedgers(s, loaded.Decision.RequiredOutput, loaded.StateRevision)
	if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
		t.Fatalf("reloaded source gate first error differs: %v != %v", got, want)
	}
}

func TestConvergenceSourceLedgersCompleteOrderOracle(t *testing.T) {
	assertConvergenceSourceLedgerOracle(t, RuntimeStateRecord{})
	for arm := uint8(0); arm < 18; arm++ {
		r := terminalHistoricalAcceptanceFixture(t)
		mutateRuntimeStateConsumerSession(&r, arm, "raw\x00漂移")
		assertConvergenceSourceLedgerOracle(t, r)
	}
}

func TestConvergenceSourceLedgersReadOnlyArchiveOracle(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		r := run.RuntimeState
		if err := referenceConvergenceSourceLedgers(historicalSessionArchiveSources(r), r.Decision.RequiredOutput, r.StateRevision); err != nil {
			t.Fatal("archive original source gate rejected", err)
		}
		assertConvergenceSourceLedgerOracle(t, r)
		for arm := uint8(0); arm < 18; arm++ {
			changed := r.Clone()
			mutateRuntimeStateConsumerSession(&changed, arm, "same-hash raw drift")
			assertConvergenceSourceLedgerOracle(t, changed)
		}
		changed := r.Clone()
		changed.Decision.RequiredOutput.ImplementationWorkBindingHash = "unknown-live-binding"
		assertConvergenceSourceLedgerOracle(t, changed)
		// Multiple failures retain the earlier historical/work gate's rejection.
		changed.OperationReceipts = nil
		changed.CompletedDispatches = nil
		assertConvergenceSourceLedgerOracle(t, changed)
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func FuzzConvergenceSourceLedgerCompleteOracle(f *testing.F) {
	for arm := uint8(0); arm < 18; arm++ {
		f.Add(arm, "原\x00文")
	}
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		r := terminalHistoricalAcceptanceFixture(t)
		mutateRuntimeStateConsumerSession(&r, arm, raw)
		assertConvergenceSourceLedgerOracle(t, r)
	})
}

func BenchmarkConvergenceSourceLedgersCompleteArchive(b *testing.B) {
	_, _, runs := runtimeStateConsumerArchive(b)
	for _, run := range runs {
		r := run.RuntimeState
		s := historicalSessionArchiveSources(r)
		for _, independent := range []bool{true, false} {
			name := "shared-complete-gate"
			if independent {
				name = "independent-consumers"
			}
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					var err error
					if independent {
						err = referenceConvergenceSourceLedgers(s, r.Decision.RequiredOutput, r.StateRevision)
					} else {
						err = ValidateRuntimeConvergenceSourceLedgers(s, r.Decision.RequiredOutput, r.StateRevision)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
