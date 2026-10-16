package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// Independent standalone gates are the original complete current/history
// sequence; they have no prior success input and cannot share a session.
func referenceValidationCatalogHistory(requirements []RuntimeValidationProofRequirement, current, history []RuntimeOperationAdmissibility) error {
	if err := ValidateRuntimeValidationProofRequirementCatalogApplicability(requirements, current); err != nil {
		return fmt.Errorf("runtime aggregate current validation proof applicability: %w", err)
	}
	for index, catalog := range history {
		if err := ValidateRuntimeValidationProofRequirementCatalogApplicability(requirements, []RuntimeOperationAdmissibility{catalog}); err != nil {
			return fmt.Errorf("runtime aggregate sealed validation proof applicability %d: %w", index, err)
		}
	}
	return nil
}

func validationCatalogHistoryFixture(t *testing.T) ([]RuntimeValidationProofRequirement, []RuntimeOperationAdmissibility, []RuntimeOperationAdmissibility) {
	t.Helper()
	manifest, _, _ := semanticInputReviewV2Fixture(t)
	var requirements []RuntimeValidationProofRequirement
	for _, obligation := range manifest.ProofObligations {
		if obligation.ValidationProofRequirement != nil {
			requirements = append(requirements, obligation.ValidationProofRequirement.Clone())
		}
	}
	current := CloneRuntimeOperationAdmissibilities(manifest.OwnerCatalogs)
	history := append(CloneRuntimeOperationAdmissibilities(current), CloneRuntimeOperationAdmissibilities(current)...)
	if err := referenceValidationCatalogHistory(requirements, current, history); err != nil {
		t.Fatal(err)
	}
	return requirements, current, history
}

func assertValidationCatalogHistory(t testing.TB, requirements []RuntimeValidationProofRequirement, current, history []RuntimeOperationAdmissibility) {
	t.Helper()
	rawRequirements := cloneRuntimeStateRawSlice(requirements, cloneRuntimeValidationProofRequirementRaw)
	rawCurrent := cloneRuntimeStateRawSlice(current, cloneRuntimeOperationAdmissibilityRaw)
	rawHistory := cloneRuntimeStateRawSlice(history, cloneRuntimeOperationAdmissibilityRaw)
	original, _ := json.Marshal(struct {
		Requirements []RuntimeValidationProofRequirement
		Current      []RuntimeOperationAdmissibility
		History      []RuntimeOperationAdmissibility
	}{requirements, current, history})
	for range 2 {
		got := ValidateRuntimeValidationProofRequirementCatalogHistory(requirements, current, history)
		want := referenceValidationCatalogHistory(requirements, current, history)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("complete current/history gate or first error changed: %v != %v", got, want)
		}
	}
	after, _ := json.Marshal(struct {
		Requirements []RuntimeValidationProofRequirement
		Current      []RuntimeOperationAdmissibility
		History      []RuntimeOperationAdmissibility
	}{requirements, current, history})
	if string(original) != string(after) || !reflect.DeepEqual(requirements, rawRequirements) || !reflect.DeepEqual(current, rawCurrent) || !reflect.DeepEqual(history, rawHistory) {
		t.Fatal("catalog history validation changed original source bytes")
	}
	var loaded struct {
		Requirements []RuntimeValidationProofRequirement
		Current      []RuntimeOperationAdmissibility
		History      []RuntimeOperationAdmissibility
	}
	if err := json.Unmarshal(original, &loaded); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(ValidateRuntimeValidationProofRequirementCatalogHistory(loaded.Requirements, loaded.Current, loaded.History)) != fmt.Sprint(referenceValidationCatalogHistory(loaded.Requirements, loaded.Current, loaded.History)) {
		t.Fatal("fresh reload borrowed prior current/history success")
	}
}

func TestValidationCatalogHistoryRechecksSelfSealedParticipation(t *testing.T) {
	requirements, current, history := validationCatalogHistoryFixture(t)
	for index := range history {
		catalog := &history[index]
		if len(catalog.ValidationSurfaces) == 0 {
			continue
		}
		other := requirements[0].Clone()
		other.CoverageWorkItemID = "cwi-other"
		other.Hash = StableRuntimeValidationProofRequirementHash(other)
		if err := other.Validate(); err != nil {
			t.Fatal(err)
		}
		catalog.ValidationRecipes = nil
		catalog.ValidationSurfaces = []RuntimeValidationSurface{
			bindRuntimeValidationSurfaceForTest(runtimeExactProofCloneReadySurfaceFixture(catalog.ValidationSurfaces[0].ID), other).Normalized(),
		}
		catalog.ValidationRecipeRefs = RuntimeSelectableValidationRecipeRefs(catalog.ValidationSurfaces)
		catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
		if err := catalog.Validate(); err != nil {
			t.Fatal("counterexample must retain valid owner catalog identity", err)
		}
		assertValidationCatalogHistory(t, requirements, current, history)
		if ValidateRuntimeValidationProofRequirementCatalogHistory(requirements, current, history) == nil {
			t.Fatal("history self-sealed another source and borrowed current requirement credit")
		}
		return
	}
	t.Fatal("fixture has no validation catalog")
}

func mutateValidationCatalogHistory(requirements *[]RuntimeValidationProofRequirement, current, history []RuntimeOperationAdmissibility, arm uint8, raw string) {
	switch arm % 9 {
	case 0:
		(*requirements)[0].Hash = raw
	case 1:
		*requirements = append(*requirements, (*requirements)[0])
	case 2:
		(*requirements)[0].RequiredCheckRefs = append((*requirements)[0].RequiredCheckRefs, (*requirements)[0].RequiredCheckRefs[0])
	case 3:
		current[0].SnapshotID = raw
	case 4:
		history[0].SnapshotID = raw
	case 5:
		history[len(history)-1].ObservedRevision = raw
	case 6:
		current[0].SnapshotID = raw
		history[0].SnapshotID = "second error"
	case 7:
		(*requirements)[0].DefinitionRevision = raw
		history[0].SnapshotID = "second error"
	case 8:
		for i := range history {
			if len(history[i].ValidationSurfaces) != 0 {
				history[i].ValidationSurfaces[0].ProofRequirementBindings[0].RequiredCheckRefs[0] = raw
				break
			}
		}
	}
}

func TestValidationCatalogHistoryCompleteOrderAndIndependentCalls(t *testing.T) {
	assertValidationCatalogHistory(t, nil, nil, nil)
	for arm := uint8(0); arm < 9; arm++ {
		requirements, current, history := validationCatalogHistoryFixture(t)
		assertValidationCatalogHistory(t, requirements, current, history)
		mutateValidationCatalogHistory(&requirements, current, history, arm, "same-key\x00原文漂移")
		assertValidationCatalogHistory(t, requirements, current, history)
		if ValidateRuntimeValidationProofRequirementCatalogHistory(requirements, current, history) == nil {
			t.Fatalf("changed source %d accepted prior success", arm)
		}
	}
}

func TestValidationCatalogHistoryPrivatePreimageMustBeCompleteRaw(t *testing.T) {
	requirements, current, _ := validationCatalogHistoryFixture(t)
	prior := runtimeValidationCatalogRequirementPreimage{
		raw:       cloneRuntimeStateRawSlice(requirements, cloneRuntimeValidationProofRequirementRaw),
		requested: NormalizeRuntimeValidationProofRequirements(requirements),
	}
	saved := cloneRuntimeStateRawSlice(prior.raw, cloneRuntimeValidationProofRequirementRaw)
	requirements[0].RequiredCheckRefs[0] = "raw drift with unchanged hash"
	got := validateRuntimeValidationProofRequirementCatalogApplicability(requirements, current, &prior)
	want := ValidateRuntimeValidationProofRequirementCatalogApplicability(requirements, current)
	if got == nil || fmt.Sprint(got) != fmt.Sprint(want) || !reflect.DeepEqual(saved, prior.raw) {
		t.Fatal("private requirement preimage borrowed hash-only success or aliased input")
	}
}

func validationCatalogHistoryFromRecord(t testing.TB, record RuntimeStateRecord) ([]RuntimeValidationProofRequirement, []RuntimeOperationAdmissibility, []RuntimeOperationAdmissibility) {
	t.Helper()
	requirements, err := CompileRuntimeValidationProofRequirements(record.Facts.DefinitionRevision, record.Facts.DefinitionSeed.CoverageItems, record.Facts.DefinitionFocuses)
	if err != nil {
		t.Fatal(err)
	}
	var history []RuntimeOperationAdmissibility
	if record.SemanticPlanning.Manifest != nil {
		history = append(history, record.SemanticPlanning.Manifest.OwnerCatalogs...)
	}
	for _, generation := range record.SemanticPlanning.History {
		if generation.Snapshot.Manifest != nil {
			history = append(history, generation.Snapshot.Manifest.OwnerCatalogs...)
		}
	}
	return requirements, record.Facts.Resources.OperationAdmissibilities, history
}

func TestValidationCatalogHistoryReadOnlyArchiveOracle(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		requirements, current, history := validationCatalogHistoryFromRecord(t, run.RuntimeState)
		if err := referenceValidationCatalogHistory(requirements, current, history); err != nil {
			t.Fatal(err)
		}
		assertValidationCatalogHistory(t, requirements, current, history)
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func FuzzValidationCatalogHistoryCompleteOracle(f *testing.F) {
	for arm := uint8(0); arm < 9; arm++ {
		f.Add(arm, "raw\x00")
	}
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		requirements, current, history := validationCatalogHistoryFixture(t)
		mutateValidationCatalogHistory(&requirements, current, history, arm, raw)
		assertValidationCatalogHistory(t, requirements, current, history)
	})
}

func BenchmarkValidationCatalogHistoryArchive(b *testing.B) {
	_, _, runs := runtimeStateConsumerArchive(b)
	for id, run := range runs {
		requirements, current, history := validationCatalogHistoryFromRecord(b, run.RuntimeState)
		for _, mode := range []string{"independent", "single_call"} {
			b.Run(id+"/"+mode, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var err error
					if mode == "independent" {
						err = referenceValidationCatalogHistory(requirements, current, history)
					} else {
						err = ValidateRuntimeValidationProofRequirementCatalogHistory(requirements, current, history)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
