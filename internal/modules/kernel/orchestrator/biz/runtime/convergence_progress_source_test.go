package runtime

import (
	"encoding/json"
	"reflect"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestConvergenceProgressConsumesOnlyItsExactInputProjection(t *testing.T) {
	_, _, base, _, _, _ := sealedSemanticPlanningAggregate(t)
	for _, reload := range []bool{false, true} {
		if reload {
			body, err := json.Marshal(base)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(body, &base); err != nil {
				t.Fatal(err)
			}
		}
		check := func(a RuntimeAggregate) {
			t.Helper()
			original := a.CloneRaw()
			if got, want := RuntimeConvergenceProgressSignature(a), referenceFullInputConvergenceProgressSignature(a); got != want {
				t.Fatalf("progress query changed original meaning: reload=%v got=%s want=%s", reload, got, want)
			}
			if !reflect.DeepEqual(a, original) {
				t.Fatal("progress query mutated original authority")
			}
		}
		check(base)
		// Every current field can independently become absent. Unrelated
		// inputs must stay excluded, and selected inputs retain their original
		// canonical/empty/duplicate semantics. Invalid sources grant no credit.
		for i := 0; i < reflect.TypeOf(base.Facts).NumField(); i++ {
			a := base.CloneRaw()
			field := reflect.ValueOf(&a.Facts).Elem().Field(i)
			field.Set(reflect.Zero(field.Type()))
			check(a)
		}
		for _, arm := range []string{"observation", "catalog", "scope", "duplicate", "history"} {
			a := base.CloneRaw()
			switch arm {
			case "observation":
				a.Facts.Resources.PrerequisiteObservations = append(a.Facts.Resources.PrerequisiteObservations, rc.RuntimePrerequisiteObservation{PrerequisiteID: "new observation"})
			case "catalog":
				a.Facts.Resources.OperationAdmissibilities = append(a.Facts.Resources.OperationAdmissibilities, rc.RuntimeOperationAdmissibility{SnapshotID: "unrelated catalog正文"})
			case "scope":
				a.Facts.Focus.ID += " other scope"
			case "duplicate":
				a.Facts.RemainingFocuses = append(a.Facts.RemainingFocuses, a.Facts.Focus, a.Facts.Focus)
			case "history":
				a.SemanticPlanning.History = []rc.RuntimeSemanticPlanningGeneration{{Snapshot: a.SemanticPlanning.WithoutHistory()}}
			}
			check(a)
		}
	}
}
