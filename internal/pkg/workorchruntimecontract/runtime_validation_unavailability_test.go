package workorchruntimecontract

import (
	"encoding/json"
	"testing"
)

func TestValidationDiagnosticUnavailabilityIsExactAndCannotGrantRepair(t *testing.T) {
	d := ValidationFailureDiagnostic{SchemaVersion: ValidationFailureDiagnosticSchemaVersion, Class: ValidationDiagnosticOutputInconclusive, Unavailability: ValidationDiagnosticAssertionsUnavailable, OwnerCode: "owner_assertions_incomplete", Stage: "ordinary-test", Operation: RuntimeBlockKindManagedValidationSession, RecommendedEvidenceTargetType: ValidationEvidenceTargetValidationOutput}
	if err := validateRuntimeValidationInputDiagnostic(d); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeValidationDiagnosticFailureObservation(nil, RuntimeValidationProofRequirementBinding{}, nil, &RuntimeValidationFailureObservation{}, &d); err == nil {
		t.Fatal("unavailable diagnostic granted a separate failure target observation")
	}
	hash := StableRuntimeValidationFailureObservationDiagnosticHash(d, []string{"scope"})
	for _, change := range []func(*ValidationFailureDiagnostic){func(v *ValidationFailureDiagnostic) { v.Unavailability = ValidationDiagnosticInputChanged }, func(v *ValidationFailureDiagnostic) { v.Unavailability = "" }} {
		other := d.Clone()
		change(&other)
		if hash == StableRuntimeValidationFailureObservationDiagnosticHash(other, []string{"scope"}) || SameValidationFailureDiagnostic(&d, &other) {
			t.Fatal("diagnostic outcome lost identity")
		}
	}
	for name, change := range map[string]func(*ValidationFailureDiagnostic){"unknown": func(v *ValidationFailureDiagnostic) { v.Unavailability = "invented" }, "target": func(v *ValidationFailureDiagnostic) { v.TargetRefs = []string{"source"} }, "class": func(v *ValidationFailureDiagnostic) { v.Class = ValidationDiagnosticTestFailed }, "missing-owner": func(v *ValidationFailureDiagnostic) { v.OwnerCode = "" }} {
		t.Run(name, func(t *testing.T) {
			other := d.Clone()
			change(&other)
			if validateRuntimeValidationInputDiagnostic(other) == nil {
				t.Fatal("unavailable diagnostic accepted ambiguous/repair authority")
			}
		})
	}
	body, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var loaded ValidationFailureDiagnostic
	if err = json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	if !SameValidationFailureDiagnostic(&loaded, &d) {
		t.Fatal("persistence lost outcome")
	}
}
