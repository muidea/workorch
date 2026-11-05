package runtime

import (
	"encoding/json"
	"reflect"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Called with the exact two-CWI proof fixture by the cold-admission test. The
// transition has no retained success state: the original is either detached in
// process or loaded from JSON, as after an owner recovery.
func assertRuntimeValidationCatalogSourceTransitions(t *testing.T, facts rc.RuntimeFacts) {
	t.Helper()
	base := RuntimeAggregate{Facts: facts}
	if err := validateRuntimeAggregateValidationProofApplicability(base); err != nil {
		t.Fatal(err)
	}
	for _, reload := range []bool{false, true} {
		before := base.CloneRaw()
		if reload {
			body, err := json.Marshal(before)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(body, &before); err != nil {
				t.Fatal(err)
			}
		}
		original := before.CloneRaw()
		for _, arm := range []string{"same", "unrelated-failure", "retire", "fresh-catalog", "raw-drift", "self-sealed-missing-proof", "new-history-missing-proof", "definition-obligation", "definition-focus"} {
			t.Run(arm, func(t *testing.T) {
				after := before.CloneRaw()
				valid := false
				catalog := &after.Facts.Resources.OperationAdmissibilities[0]
				switch arm {
				case "same":
					valid = true
				case "unrelated-failure":
					after.Facts.Failure = &rc.FailureFact{Code: "runtime_drive_failed", Message: "new fact, unchanged proof source"}
					valid = true
				case "retire":
					after.SemanticPlanning.History = []rc.RuntimeSemanticPlanningGeneration{{Snapshot: rc.RuntimeSemanticPlanningLedger{Manifest: &rc.RuntimeSemanticPlanManifest{OwnerCatalogs: after.Facts.Resources.OperationAdmissibilities}}}}
					valid = true
				case "fresh-catalog":
					catalog.ObservedRevision += "-fresh"
					catalog.SnapshotID = rc.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
					valid = true
				case "raw-drift":
					catalog.ValidationSurfaces[0].ProofRequirementBindings = nil
				case "self-sealed-missing-proof", "new-history-missing-proof":
					catalog.ValidationSurfaces[0].ProofRequirementBindings = nil
					catalog.SnapshotID = rc.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
					if arm == "new-history-missing-proof" {
						after.SemanticPlanning.History = []rc.RuntimeSemanticPlanningGeneration{{Snapshot: rc.RuntimeSemanticPlanningLedger{Manifest: &rc.RuntimeSemanticPlanManifest{OwnerCatalogs: after.Facts.Resources.OperationAdmissibilities}}}}
						after.Facts.Resources.OperationAdmissibilities = before.CloneRaw().Facts.Resources.OperationAdmissibilities
					}
				case "definition-obligation":
					after.Facts.DefinitionSeed.CoverageItems[0].TargetText += " plus new obligation"
				case "definition-focus":
					after.Facts.DefinitionFocuses[0].RequiredCheckRefs = []string{"new-exact-check"}
				}
				saved := after.CloneRaw()
				err := validateRuntimeAggregateValidationProofApplicabilityTransition(&before, after)
				if (err == nil) != valid {
					t.Fatalf("reload=%v valid=%v: %v", reload, valid, err)
				}
				// The independent cold gate must agree for both new facts and
				// actual source changes; a self hash cannot provide acceptance.
				if cold := validateRuntimeAggregateValidationProofApplicability(after); (cold == nil) != valid {
					t.Fatalf("cold proof source disagrees reload=%v: %v", reload, cold)
				}
				if !reflect.DeepEqual(after, saved) || !reflect.DeepEqual(before, original) {
					t.Fatal("proof source consumption mutated original or postimage")
				}
			})
		}
	}
}
