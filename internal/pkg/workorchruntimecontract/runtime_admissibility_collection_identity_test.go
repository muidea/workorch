package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func referenceAdmissibilityCollectionIdentity(values []RuntimeOperationAdmissibility) error {
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		if err := raw.Validate(); err != nil {
			return err
		}
		value := raw.Normalized()
		if _, duplicate := seen[value.PrerequisiteID]; duplicate {
			return fmt.Errorf("runtime operation admissibility duplicates prerequisite %q", value.PrerequisiteID)
		}
		seen[value.PrerequisiteID] = struct{}{}
	}
	return nil
}

func TestAdmissibilityCollectionScalarIdentityPreservesFullRawGate(t *testing.T) {
	manifest, _, _, _ := semanticInputReviewFixture(t)
	base := manifest.OwnerCatalogs[0].Clone()
	for _, scenario := range []string{"nil", "empty", "single", "duplicate", "trim-duplicate", "invalid-before-duplicate", "input-bytes", "raw-hash", "nested-duplicate"} {
		t.Run(scenario, func(t *testing.T) {
			values := []RuntimeOperationAdmissibility{base.Clone()}
			switch scenario {
			case "nil":
				values = nil
			case "empty":
				values = []RuntimeOperationAdmissibility{}
			case "duplicate":
				values = append(values, base.Clone())
			case "trim-duplicate":
				other := base.Clone()
				other.PrerequisiteID = " " + other.PrerequisiteID + " "
				values = append(values, other)
			case "invalid-before-duplicate":
				other := base.Clone()
				other.SnapshotID = "wrong"
				values = append(values, other)
			case "input-bytes":
				for i := range values[0].ValidationSurfaces {
					for j := range values[0].ValidationSurfaces[i].ProofRequirementBindings {
						if subject := values[0].ValidationSurfaces[i].ProofRequirementBindings[j].InputReview; subject != nil {
							subject.Files[0].Content += "changed"
						}
					}
				}
			case "raw-hash":
				values[0].SnapshotID = " " + values[0].SnapshotID
			case "nested-duplicate":
				values[0].OperationVariants = append(values[0].OperationVariants, values[0].OperationVariants[0])
			}
			before, _ := json.Marshal(values)
			want := cwiFocusBindingsError(referenceAdmissibilityCollectionIdentity(values))
			if got := cwiFocusBindingsError(ValidateRuntimeOperationAdmissibilities(values)); got != want {
				t.Fatalf("full gate/first-error mismatch: %q != %q", got, want)
			}
			after, _ := json.Marshal(values)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("collection identity changed raw catalogs")
			}
			var loaded []RuntimeOperationAdmissibility
			if err := json.Unmarshal(before, &loaded); err != nil || cwiFocusBindingsError(ValidateRuntimeOperationAdmissibilities(loaded)) != want {
				t.Fatalf("reload differs: %v", err)
			}
		})
	}
}
