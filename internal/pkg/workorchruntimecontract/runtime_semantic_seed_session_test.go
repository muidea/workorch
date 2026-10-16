package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func mutateSemanticSessionSeed(seed *RuntimeDefinitionSeed, arm uint8, value string) {
	switch arm % 9 {
	case 0:
	case 1:
		seed.Hash = value
	case 2:
		seed.CoverageItems[0].TargetText = value
	case 3:
		seed.CoverageItems = append(seed.CoverageItems, seed.CoverageItems[0])
	case 4:
		seed.Slices[0].CoverageWorkItemIDs[0] = "changed"
	case 5:
		seed.PolicyHash = value
	case 6:
		seed.DefinitionRevision = value
	case 7:
		seed.CoverageItems[0].SourceID = value
	case 8:
		seed.Slices = nil
	}
}

func compareSemanticSeedSession(t *testing.T, seed RuntimeDefinitionSeed, session *runtimeSemanticValidationSession) {
	t.Helper()
	before, err := json.Marshal(seed)
	if err != nil {
		t.Fatal(err)
	}
	got, want := validateRuntimeSemanticSeedInSession(seed, session), seed.Validate()
	if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
		t.Fatalf("seed first error differs: %v != %v", got, want)
	}
	after, _ := json.Marshal(seed)
	if !bytes.Equal(before, after) {
		t.Fatal("seed validator mutated raw source")
	}
}

func compareSemanticLedgerSeedSession(t *testing.T, ledger RuntimeSemanticPlanningLedger) {
	t.Helper()
	// Each path receives independent raw storage: legacy ledger normalization
	// can use nested backing slices for comparison, never share the oracle graph.
	left, right := cloneRuntimeSemanticPlanningLedgerRaw(ledger), cloneRuntimeSemanticPlanningLedgerRaw(ledger)
	before, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	got := left.Validate()
	want := referenceSemanticLedgerBeforeSeedSession(right, right.Normalized(), newRuntimeSemanticValidationSession())
	if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
		t.Fatalf("ledger first error differs: %v != %v", got, want)
	}
	after, _ := json.Marshal(ledger)
	if !bytes.Equal(before, after) {
		t.Fatal("ledger validation changed input source")
	}
}

func TestSemanticSeedSessionMatchesFullGateAndRawIsolation(t *testing.T) {
	for arm := uint8(0); arm < 9; arm++ {
		seed, _, _ := definitionImplementationWorkFixture(t)
		session := newRuntimeSemanticValidationSession()
		if err := validateRuntimeSemanticSeedInSession(seed, session); err != nil {
			t.Fatal(err)
		}
		mutateSemanticSessionSeed(&seed, arm, "same-hash changed raw")
		compareSemanticSeedSession(t, seed, session)
		compareSemanticSeedSession(t, seed, newRuntimeSemanticValidationSession())
		body, _ := json.Marshal(seed)
		var loaded RuntimeDefinitionSeed
		if err := json.Unmarshal(body, &loaded); err != nil {
			t.Fatal(err)
		}
		compareSemanticSeedSession(t, loaded, newRuntimeSemanticValidationSession())
	}
	seed, _, _ := definitionImplementationWorkFixture(t)
	ledger, err := NewRuntimeSemanticPlanningLedger(seed)
	if err != nil || ledger.Validate() != nil {
		t.Fatal("invalid ledger fixture", err)
	}
	compareSemanticLedgerSeedSession(t, ledger)
	for arm := uint8(0); arm < 9; arm++ {
		changed := cloneRuntimeSemanticPlanningLedgerRaw(ledger)
		mutateSemanticSessionSeed(&changed.DefinitionSeed, arm, "changed")
		compareSemanticLedgerSeedSession(t, changed)
	}
}

func FuzzSemanticSeedSessionMatchesFullGate(f *testing.F) {
	for arm := uint8(0); arm < 9; arm++ {
		f.Add(arm, "raw")
	}
	f.Fuzz(func(t *testing.T, arm uint8, value string) {
		if len(value) > 128 {
			t.Skip()
		}
		seed, _, _ := definitionImplementationWorkFixture(t)
		session := newRuntimeSemanticValidationSession()
		if err := validateRuntimeSemanticSeedInSession(seed, session); err != nil {
			t.Fatal(err)
		}
		mutateSemanticSessionSeed(&seed, arm, value)
		compareSemanticSeedSession(t, seed, session)
	})
}

func TestSemanticSeedSessionExplicitPublicArchiveEquivalence(t *testing.T) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		t.Skip("explicit local archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState RuntimeStateRecord }
	if err := json.Unmarshal(body, &runs); err != nil {
		t.Fatal(err)
	}
	for _, run := range runs {
		base := run.RuntimeState.SemanticPlanning
		if len(base.History) < 2 {
			t.Fatal("archive lacks multiple real semantic generations")
		}
		if err := base.Validate(); err != nil {
			t.Fatal(err)
		}
		compareSemanticLedgerSeedSession(t, base)
		for _, mutate := range []func(*RuntimeSemanticPlanningLedger){
			func(l *RuntimeSemanticPlanningLedger) { l.DefinitionSeed.Hash = "drift" },
			func(l *RuntimeSemanticPlanningLedger) {
				l.History[0].Snapshot.DefinitionSeed.CoverageItems[0].TargetText = "same hash changed raw"
			},
			func(l *RuntimeSemanticPlanningLedger) { l.History[0].Snapshot.DefinitionSeed.Hash = "drift" },
			func(l *RuntimeSemanticPlanningLedger) {
				l.History[1].RetiredAtStateRevision = l.History[0].RetiredAtStateRevision
			},
			func(l *RuntimeSemanticPlanningLedger) {
				l.History[0].Snapshot.Manifest.OwnerCatalogs[0].SnapshotID = "drift"
			},
		} {
			changed := cloneRuntimeSemanticPlanningLedgerRaw(base)
			mutate(&changed)
			compareSemanticLedgerSeedSession(t, changed)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("read-only test changed archive")
	}
}

func BenchmarkSemanticSeedSessionPublicArchiveCompleteGate(b *testing.B) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		b.Skip("explicit local archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState RuntimeStateRecord }
	if err := json.Unmarshal(body, &runs); err != nil {
		b.Fatal(err)
	}
	for _, run := range runs {
		base := run.RuntimeState.SemanticPlanning
		for _, reference := range []bool{true, false} {
			name := "session"
			if reference {
				name = "original"
			}
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					var err error
					if reference {
						err = referenceSemanticLedgerBeforeSeedSession(base, base.Normalized(), newRuntimeSemanticValidationSession())
					} else {
						err = base.Validate()
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
