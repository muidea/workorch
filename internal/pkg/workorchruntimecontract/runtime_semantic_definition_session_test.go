package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func referenceSemanticLedgerDefinitionGate(ledger RuntimeSemanticPlanningLedger, seed RuntimeDefinitionSeed, focuses []RuntimeDefinitionFocus) error {
	if ledger.Manifest != nil {
		if err := referenceSemanticManifestDefinitionGate(*ledger.Manifest, seed, focuses); err != nil {
			return fmt.Errorf("runtime aggregate semantic active manifest: %w", err)
		}
	}
	for i, generation := range ledger.History {
		if generation.Snapshot.Manifest == nil {
			continue
		}
		if err := referenceSemanticManifestDefinitionGate(*generation.Snapshot.Manifest, seed, focuses); err != nil {
			return fmt.Errorf("runtime aggregate semantic history manifest %d: %w", i, err)
		}
	}
	return nil
}

func assertSemanticDefinitionSession(t testing.TB, ledger RuntimeSemanticPlanningLedger, seed RuntimeDefinitionSeed, focuses []RuntimeDefinitionFocus) {
	t.Helper()
	input := struct {
		Ledger  RuntimeSemanticPlanningLedger
		Seed    RuntimeDefinitionSeed
		Focuses []RuntimeDefinitionFocus
	}{ledger, seed, focuses}
	before, _ := json.Marshal(input)
	for range 2 {
		got := ValidateRuntimeSemanticLedgerDefinitionSources(ledger, seed, focuses)
		want := referenceSemanticLedgerDefinitionGate(ledger, seed, focuses)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("ledger Definition first error differs: %v != %v", got, want)
		}
	}
	after, _ := json.Marshal(input)
	if !bytes.Equal(before, after) {
		t.Fatal("Definition source gate changed original")
	}
	if err := json.Unmarshal(before, &input); err != nil {
		t.Fatal(err)
	}
	got := ValidateRuntimeSemanticLedgerDefinitionSources(input.Ledger, input.Seed, input.Focuses)
	want := referenceSemanticLedgerDefinitionGate(input.Ledger, input.Seed, input.Focuses)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatal("Definition source gate changed after JSON reload")
	}
}

func mutateSemanticDefinitionSession(r *RuntimeStateRecord, arm uint8, raw string) {
	switch arm % 9 {
	case 0:
		r.Facts.DefinitionSeed.Hash = raw
	case 1:
		r.Facts.DefinitionSeed.CoverageItems[0].TargetText = raw
	case 2:
		r.Facts.DefinitionFocuses[0].AssessmentText = raw
	case 3:
		r.Facts.DefinitionFocuses = append(r.Facts.DefinitionFocuses, r.Facts.DefinitionFocuses[0])
	case 4:
		if r.SemanticPlanning.Manifest != nil {
			r.SemanticPlanning.Manifest.Hash = raw
		}
	case 5:
		if len(r.SemanticPlanning.History) > 0 && r.SemanticPlanning.History[0].Snapshot.Manifest != nil {
			r.SemanticPlanning.History[0].Snapshot.Manifest.Hash = raw
		}
	case 6:
		if r.SemanticPlanning.Manifest != nil && len(r.SemanticPlanning.Manifest.OwnerCatalogs) > 0 {
			r.SemanticPlanning.Manifest.OwnerCatalogs[0].SnapshotID = raw
		}
	case 7:
		r.Facts.DefinitionSeed.CoverageItems[0].Order++
	case 8:
		r.Facts.DefinitionFocuses[0].RequiredEvidenceKinds = nil
	}
}

func semanticDefinitionSessionFixture(t testing.TB) RuntimeStateRecord {
	t.Helper()
	ledger := semanticBindingIndexedMultiBatchFixture(t, 3)
	seed := ledger.DefinitionSeed.Clone()
	focuses := semanticDefinitionFocuses(seed.CoverageItems)
	for range 3 {
		ledger.History = append(ledger.History, RuntimeSemanticPlanningGeneration{Snapshot: RuntimeSemanticPlanningLedger{Manifest: ledger.Manifest}})
	}
	return RuntimeStateRecord{Facts: RuntimeFacts{DefinitionSeed: seed, DefinitionFocuses: focuses}, SemanticPlanning: ledger}
}

func TestSemanticDefinitionSessionFullOracleAndRawIsolation(t *testing.T) {
	r := semanticDefinitionSessionFixture(t)
	seed, focuses, m := r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses, *r.SemanticPlanning.Manifest
	if err := referenceSemanticManifestDefinitionGate(m, seed, focuses); err != nil {
		t.Fatal(err)
	}
	session := newRuntimeSemanticValidationSession()
	for range 2 {
		if err := validateRuntimeSemanticManifestDefinitionInSession(m, seed, focuses, session); err != nil {
			t.Fatal(err)
		}
	}
	if len(session.definitionSources) != 1 {
		t.Fatal("same complete raw Definition source was not reused")
	}
	saved := cloneRuntimeStateRawSlice(focuses, cloneRuntimeDefinitionFocusRaw)
	focuses[0].AssessmentText = "same seed/hash; different original criterion"
	got := validateRuntimeSemanticManifestDefinitionInSession(m, seed, focuses, session)
	want := referenceSemanticManifestDefinitionGate(m, seed, focuses)
	if got == nil || fmt.Sprint(got) != fmt.Sprint(want) || !reflect.DeepEqual(saved, session.definitionSources[0].focuses) {
		t.Fatal("source alias mutation borrowed or changed saved raw preimage")
	}
	for arm := uint8(0); arm < 9; arm++ {
		r := semanticDefinitionSessionFixture(t)
		assertSemanticDefinitionSession(t, r.SemanticPlanning, r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses)
		mutateSemanticDefinitionSession(&r, arm, "raw\x00漂移")
		assertSemanticDefinitionSession(t, r.SemanticPlanning, r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses)
	}
}

func TestSemanticDefinitionSessionManifestAliasCannotBorrowSuccess(t *testing.T) {
	r := semanticDefinitionSessionFixture(t)
	m := *r.SemanticPlanning.Manifest
	seed, focuses := r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses
	session := newRuntimeSemanticValidationSession()
	if err := validateRuntimeSemanticManifestDefinitionInSession(m, seed, focuses, session); err != nil {
		t.Fatal(err)
	}
	m.OwnerCatalogs[0].SnapshotID = "caller-mutated-same-manifest-hash"
	got := validateRuntimeSemanticManifestDefinitionInSession(m, seed, focuses, session)
	want := referenceSemanticManifestDefinitionGate(m, seed, focuses)
	if want == nil || got == nil || fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("manifest alias borrowed old success: got=%v want=%v", got, want)
	}
}

func TestSemanticDefinitionSessionReadOnlyArchiveOracle(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		r := run.RuntimeState
		if err := referenceSemanticLedgerDefinitionGate(r.SemanticPlanning, r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses); err != nil {
			t.Fatal("archive original Definition gate rejected", err)
		}
		assertSemanticDefinitionSession(t, r.SemanticPlanning, r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses)
		for arm := uint8(0); arm < 9; arm++ {
			changed := r.Clone()
			mutateSemanticDefinitionSession(&changed, arm, "same-hash raw drift")
			assertSemanticDefinitionSession(t, changed.SemanticPlanning, changed.Facts.DefinitionSeed, changed.Facts.DefinitionFocuses)
		}
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func FuzzSemanticDefinitionSessionCompleteOracle(f *testing.F) {
	for arm := uint8(0); arm < 9; arm++ {
		f.Add(arm, "原\x00文")
	}
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		r := semanticDefinitionSessionFixture(t)
		mutateSemanticDefinitionSession(&r, arm, raw)
		assertSemanticDefinitionSession(t, r.SemanticPlanning, r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses)
	})
}

func BenchmarkSemanticDefinitionSessionCompleteArchive(b *testing.B) {
	_, _, runs := runtimeStateConsumerArchive(b)
	for _, run := range runs {
		r := run.RuntimeState
		for _, independent := range []bool{true, false} {
			name := "shared-call"
			if independent {
				name = "independent-manifests"
			}
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					var err error
					if independent {
						err = referenceSemanticLedgerDefinitionGate(r.SemanticPlanning, r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses)
					} else {
						err = ValidateRuntimeSemanticLedgerDefinitionSources(r.SemanticPlanning, r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
