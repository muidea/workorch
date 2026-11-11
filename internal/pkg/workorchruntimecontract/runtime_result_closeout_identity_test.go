package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestResultCloseoutPersistenceIdentityPreservesExactRepresentedFacts(t *testing.T) {
	original := &RuntimeResultCloseout{OriginalDiagnostic: &RuntimeOriginalDiagnostic{
		OriginalMessage: " original\n", TargetRefs: []string{},
		FieldViolations:    []RuntimeFieldViolation{{Path: "subject", Allowed: []string{}, Required: []string{" b ", "a", " b "}}},
		SemanticViolations: []RuntimeSemanticContractViolation{},
	}}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var loaded RuntimeResultCloseout
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(original, &loaded) || !original.SamePersistedValue(&loaded) {
		t.Fatal("file representation lost its delivery identity across optional empty collections")
	}
	for name, mutate := range map[string]func(*RuntimeResultCloseout){
		"message whitespace": func(c *RuntimeResultCloseout) { c.OriginalDiagnostic.OriginalMessage = "original" },
		"source order":       func(c *RuntimeResultCloseout) { c.OriginalDiagnostic.FieldViolations[0].Required[0] = "a" },
		"source identity":    func(c *RuntimeResultCloseout) { c.OriginalDiagnostic.TargetRefs = []string{"other"} },
		"non optional empty": func(c *RuntimeResultCloseout) { c.TargetRefs = []string{} },
		"revision":           func(c *RuntimeResultCloseout) { c.RuntimeStateRevision++ },
	} {
		t.Run(name, func(t *testing.T) {
			changed := loaded.CloneRaw()
			mutate(&changed)
			if original.SamePersistedValue(&changed) {
				t.Fatal("represented material drift was accepted")
			}
		})
	}
	if original.OriginalDiagnostic.TargetRefs == nil || original.OriginalDiagnostic.FieldViolations[0].Allowed == nil {
		t.Fatal("comparison rewrote the original allocation shape")
	}
	var absent *RuntimeResultCloseout
	if !absent.SamePersistedValue(nil) || absent.SamePersistedValue(&RuntimeResultCloseout{}) || original.SamePersistedValue(nil) {
		t.Fatal("missing material became equal to present material")
	}
}
