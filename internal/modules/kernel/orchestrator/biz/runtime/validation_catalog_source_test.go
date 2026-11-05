package runtime

import (
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"reflect"
	"strings"
	"testing"
)

func TestValidationCatalogSourceSelectionMatchesOriginal(t *testing.T) {
	first, observation := runtimeWebCatalogFixture(t, "source", "revision")
	op := runtimecontract.RuntimeOperation{Kind: runtimecontract.StepKindWebSearch}
	for _, name := range []string{"original", "whitespace", "missing", "observation-kind", "observation-missing", "invalid-catalog", "duplicates", "unrelated"} {
		t.Run(name, func(t *testing.T) {
			catalog, obs := first.Normalized(), observation.Normalized()
			switch name {
			case "whitespace":
				catalog.PrerequisiteID = " " + catalog.PrerequisiteID + " "
				catalog.OwnerModuleID = " " + catalog.OwnerModuleID + " "
				catalog.SnapshotID = " " + catalog.SnapshotID + " "
				catalog.AcquisitionBinding = " " + catalog.AcquisitionBinding + " "
				obs.PrerequisiteID = " " + obs.PrerequisiteID + " "
				obs.Kind = runtimecontract.PrerequisiteKind(" " + string(obs.Kind) + " ")
				obs.OwnerModuleID = " " + obs.OwnerModuleID + " "
				obs.ContentHash = " " + obs.ContentHash + " "
			case "missing":
				catalog.SnapshotID = "different"
			case "observation-kind":
				obs.Kind = runtimecontract.PrerequisiteKind("other")
			case "observation-missing":
				obs.ObservationID = ""
			case "invalid-catalog":
				catalog.ObservedRevision = ""
			}
			values := []runtimecontract.RuntimeOperationAdmissibility{catalog}
			observations := []runtimecontract.RuntimePrerequisiteObservation{obs}
			if name == "duplicates" {
				values = append(values, catalog)
				observations = append(observations, obs)
			}
			if name == "unrelated" {
				other := first.Normalized()
				other.SnapshotID = "unrelated"
				other.ValidationSurfaces = []runtimecontract.RuntimeValidationSurface{{ID: "invalid unrelated body"}}
				values = append([]runtimecontract.RuntimeOperationAdmissibility{other}, values...)
			}
			before := runtimecontract.CloneRuntimeOperationAdmissibilities(values)
			want := historySourceReferenceRuntimeOwnerCatalogForOperation(values, observations, op, first.SnapshotID, first.AcquisitionBinding)
			got := runtimeOwnerCatalogForOperation(values, observations, op, first.SnapshotID, first.AcquisitionBinding)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("selection differs from full original")
			}
			if !reflect.DeepEqual(runtimecontract.CloneRuntimeOperationAdmissibilities(values), before) {
				t.Fatal("selection mutated source")
			}
			if name == "whitespace" && got == nil {
				t.Fatal("canonical identity rejected")
			}
			wantObs := historySourceReferenceRuntimeOwnerCatalogObservationForOperation(observations, &catalog)
			gotObs := runtimeOwnerCatalogObservationForOperation(observations, &catalog)
			if !reflect.DeepEqual(gotObs, wantObs) {
				t.Fatal("observation differs from complete original")
			}
			if got != nil {
				got.OperationKinds[0] = "changed"
				if strings.TrimSpace(values[len(values)-1].OperationKinds[0]) == "changed" {
					t.Fatal("catalog aliases source")
				}
			}
		})
	}
}

func TestValidationCatalogSourceArchiveScopeAndProofOracle(t *testing.T) {
	a, _ := reconciliationScanArchive(t)
	checked := 0
	for _, d := range a.CompletedDispatches {
		ops, err := orderedCandidateOperations(d.Candidate)
		if err != nil {
			continue
		}
		for _, op := range ops {
			if runtimecontract.CanonicalRuntimeBlockKind(op.Kind) != runtimecontract.RuntimeBlockKindManagedValidationSession {
				continue
			}
			e, _ := operationExecutionRecord(d.OperationExecutions, op.ID)
			catalog := runtimeOwnerCatalogForOperation(d.OperationAdmissibilities, d.OperationAdmissibilityObservations, op, e.AdmissibilitySnapshotID, e.AcquisitionBinding)
			oldCatalog := historySourceReferenceRuntimeOwnerCatalogForOperation(d.OperationAdmissibilities, d.OperationAdmissibilityObservations, op, e.AdmissibilitySnapshotID, e.AcquisitionBinding)
			if !reflect.DeepEqual(catalog, oldCatalog) {
				t.Fatal("archive original catalog differs")
			}
			want, wantErr := historySourceReferenceRuntimeValidationScopeTargetRefs(d, op)
			got, gotErr := runtimeValidationScopeTargetRefs(d, op)
			if !reflect.DeepEqual(got, want) || !sameHistorySourceError(gotErr, wantErr) {
				t.Fatal("archive scope differs")
			}
			scope, err := runtimeValidationScopeFromOwnerCatalog(d, op, catalog)
			if !reflect.DeepEqual(scope, want) || !sameHistorySourceError(err, wantErr) {
				t.Fatal("same-call source scope differs")
			}
			proof, err := runtimeValidationProofBindingForOperation(d, op, catalog)
			oldProof, oldErr := historySourceReferenceRuntimeValidationProofBindingForOperation(d, op, oldCatalog)
			if !reflect.DeepEqual(proof, oldProof) || !sameHistorySourceError(err, oldErr) {
				t.Fatal("proof differs")
			}
			if catalog != nil {
				changed := catalog.Normalized()
				changed.SnapshotID = "foreign"
				if _, err := runtimeValidationScopeFromOwnerCatalog(d, op, &changed); err == nil {
					t.Fatal("foreign catalog granted scope")
				}
				if len(scope) > 0 {
					scope[0] = "changed"
					if catalog.TargetRefs[0] == "changed" {
						t.Fatal("scope aliases selected source")
					}
				}
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("archive lacks managed-validation source")
	}
}
func sameHistorySourceError(a, b error) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Error() == b.Error()
}
