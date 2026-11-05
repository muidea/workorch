package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"testing"
)

func comparePreparationProduct(t testing.TB, p RuntimeValidationProofPreparation) {
	t.Helper()
	before := cloneRuntimeValidationProofPreparationRaw(p)
	wantErr := referencePreparationBeforeProduct(p)
	got, err := validateAndNormalizeRuntimeValidationProofPreparation(p)
	if fmt.Sprint(err) != fmt.Sprint(wantErr) || fmt.Sprint(p.Validate()) != fmt.Sprint(wantErr) {
		t.Fatalf("preparation first error changed: %v != %v", err, wantErr)
	}
	if err == nil && !reflect.DeepEqual(got, p.Normalized()) {
		t.Fatal("preparation canonical projection changed")
	}
	if err != nil && !reflect.DeepEqual(got, RuntimeValidationProofPreparation{}) {
		t.Fatal("rejected preparation exposed a product")
	}
	if !reflect.DeepEqual(before, p) {
		t.Fatal("preparation compiler modified raw source")
	}
}

func comparePreparationSurfaceProduct(t testing.TB, s RuntimeValidationSurface) {
	t.Helper()
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	want, wantErr := referenceSurfaceBeforePreparationProduct(s)
	got, err := validateAndNormalizeRuntimeValidationSurface(s)
	if fmt.Sprint(err) != fmt.Sprint(wantErr) || !reflect.DeepEqual(got, want) {
		t.Fatalf("surface preparation product or first error changed: %v != %v", err, wantErr)
	}
	for _, p := range s.ProofRequirementPreparations {
		comparePreparationProduct(t, p)
	}
	values, collectionErr := validateAndNormalizeRuntimeValidationProofPreparations(s.ProofRequirementPreparations)
	if fmt.Sprint(collectionErr) != fmt.Sprint(referencePreparationCollectionBeforeProduct(s.ProofRequirementPreparations)) {
		t.Fatal("preparation collection first error changed")
	}
	if collectionErr == nil && !reflect.DeepEqual(values, normalizeRuntimeValidationProofPreparations(s.ProofRequirementPreparations)) || collectionErr != nil && values != nil {
		t.Fatal("preparation collection canonical/zero product changed")
	}
	// The public enclosing surface must detach the entire graph, including
	// original diagnostic bytes that the private validator borrowed read-only.
	if err == nil {
		for i := range got.ProofRequirementPreparations {
			p := &got.ProofRequirementPreparations[i]
			if len(p.CheckAttestations) != 0 {
				p.CheckAttestations[0].RequiredCheckRef = "changed"
			}
			if p.OriginalDiagnostic != nil {
				p.OriginalDiagnostic.OriginalMessage = "changed"
				if len(p.OriginalDiagnostic.FieldViolations) != 0 {
					p.OriginalDiagnostic.FieldViolations[0].Actual = "changed"
				}
			}
			if len(p.ProspectiveTargetSatisfactionConstraint.AllOf) != 0 {
				p.ProspectiveTargetSatisfactionConstraint.AllOf[0].ID = "changed"
			}
		}
	}
	after, _ := json.Marshal(s)
	if !bytes.Equal(before, after) {
		t.Fatal("surface product aliases or modified input")
	}
}

func TestPreparationProductOriginalFieldsAndErrors(t *testing.T) {
	p := proofPreparationDiagnosticReadFixture(t)
	comparePreparationProduct(t, p)
	for i := 0; i < reflect.TypeFor[RuntimeValidationProofPreparation]().NumField(); i++ {
		t.Run(reflect.TypeFor[RuntimeValidationProofPreparation]().Field(i).Name, func(t *testing.T) {
			value := cloneRuntimeValidationProofPreparationRaw(p)
			reflect.ValueOf(&value).Elem().Field(i).SetZero()
			comparePreparationProduct(t, value)
		})
	}
	s := runtimeExactProofClonePreparingSurfaceFixture("recipe").Normalized()
	s.ProofRequirementPreparations[0] = p
	for _, mutate := range []func(*RuntimeValidationSurface){
		func(*RuntimeValidationSurface) {},
		func(v *RuntimeValidationSurface) { v.ProofRequirementPreparations = nil },
		func(v *RuntimeValidationSurface) {
			v.ProofRequirementPreparations = []RuntimeValidationProofPreparation{}
		},
		func(v *RuntimeValidationSurface) {
			v.ProofRequirementPreparations = append(v.ProofRequirementPreparations, v.ProofRequirementPreparations[0])
		},
		func(v *RuntimeValidationSurface) { v.ProofRequirementPreparations[0].RequirementHash += " " },
		func(v *RuntimeValidationSurface) {
			v.ProofRequirementPreparations[0].CheckAttestations = append(v.ProofRequirementPreparations[0].CheckAttestations, v.ProofRequirementPreparations[0].CheckAttestations[0])
		},
		func(v *RuntimeValidationSurface) {
			v.ProofRequirementPreparations[0].OriginalDiagnostic.OwnerModuleID += "-wrong"
		},
		func(v *RuntimeValidationSurface) {
			v.ProofRequirementPreparations[0].ProspectiveTargetSatisfactionConstraint.InputSchemaHash += "-wrong"
		},
		func(v *RuntimeValidationSurface) {
			v.ProofRequirementPreparations[0].ContentRequirements = "missing contract"
		},
	} {
		value := s.Clone()
		mutate(&value)
		comparePreparationSurfaceProduct(t, value)
		encoded, _ := json.Marshal(value)
		var reloaded RuntimeValidationSurface
		if err := json.Unmarshal(encoded, &reloaded); err != nil {
			t.Fatal(err)
		}
		comparePreparationSurfaceProduct(t, reloaded)
	}
}

func preparationProductArchiveSurfaces(r RuntimeStateRecord) []RuntimeValidationSurface {
	var surfaces []RuntimeValidationSurface
	for _, dispatch := range r.CompletedDispatches {
		for _, catalog := range dispatch.OperationAdmissibilities {
			surfaces = append(surfaces, catalog.ValidationSurfaces...)
		}
		if dispatch.AssessmentAuthorityInput != nil {
			for _, catalog := range dispatch.AssessmentAuthorityInput.OperationAdmissibilities {
				surfaces = append(surfaces, catalog.ValidationSurfaces...)
			}
		}
	}
	return surfaces
}

func TestPreparationProductCompleteArchive(t *testing.T) {
	body, path, runs := applicabilityScanArchive(t)
	count := 0
	for _, run := range runs {
		for _, s := range preparationProductArchiveSurfaces(run.RuntimeState) {
			count += len(s.ProofRequirementPreparations)
			comparePreparationSurfaceProduct(t, s)
			value := s.Clone()
			slices.Reverse(value.ProofRequirementPreparations)
			comparePreparationSurfaceProduct(t, value)
			for _, p := range s.ProofRequirementPreparations {
				if len(p.SourceObligationChecks) == 0 {
					continue
				}
				p = p.Clone()
				p.SourceObligationChecks[0].CheckRef = "missing-source-native-check"
				comparePreparationProduct(t, p)
				if err := p.Validate(); err == nil {
					t.Fatal("real source obligation drift was accepted")
				}
				break
			}
		}
	}
	if count == 0 {
		t.Fatal("archive has no original preparations")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("preparation replay changed archive")
	}
}

func BenchmarkPreparationProductCompleteArchive(b *testing.B) {
	_, _, runs := applicabilityScanArchive(b)
	for _, run := range runs {
		surfaces := preparationProductArchiveSurfaces(run.RuntimeState)
		for _, original := range []bool{true, false} {
			name := "current"
			if original {
				name = "reference"
			}
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					for _, s := range surfaces {
						var err error
						if original {
							_, err = referenceSurfaceBeforePreparationProduct(s)
						} else {
							_, err = validateAndNormalizeRuntimeValidationSurface(s)
						}
						if err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		}
	}
}
