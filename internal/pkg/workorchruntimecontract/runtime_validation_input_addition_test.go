package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestValidationInputAdditionPreservesOriginalIdentityAndBoundsTarget(t *testing.T) {
	addition := RuntimeValidationInputAddition{RequiredCheckRef: "requirement", OriginalNativeCheckRef: "original", NativeCheckRef: "supplement", TargetRef: "checks/supplement.test"}
	binding := CoverageEvidenceBinding{Proofs: []CoverageEvidenceProof{{ValidationCheckAttestations: []RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "requirement", ObservationProtocolRef: "owner/v2", NativeCheckRef: "original"}}}}}
	if !addition.MatchesBinding(binding) {
		t.Fatal("exact original check did not match")
	}
	for _, target := range []string{"", "../outside", "/absolute", "checks/../original", "checks//input", `checks\input`, "C:/input", ".", "..", "checks/input\nextra"} {
		bad := addition
		bad.TargetRef = target
		if bad.Validate() == nil || bad.MatchesBinding(binding) {
			t.Fatalf("unsafe addition target admitted: %q", target)
		}
	}
	for _, mutate := range []func(*RuntimeValidationInputAddition){
		func(v *RuntimeValidationInputAddition) { v.RequiredCheckRef = "other-requirement" },
		func(v *RuntimeValidationInputAddition) { v.OriginalNativeCheckRef = "other-original" },
		func(v *RuntimeValidationInputAddition) { v.NativeCheckRef = v.OriginalNativeCheckRef },
	} {
		bad := addition
		mutate(&bad)
		if bad.MatchesBinding(binding) {
			t.Fatalf("changed acceptance identity admitted: %+v", bad)
		}
	}
	binding.Proofs[0].ValidationCheckAttestations[0].SupplementalNativeCheckRef = "supplement"
	if addition.MatchesBinding(binding) {
		t.Fatal("already supplemented check admitted another addition cycle")
	}
}

func TestSupplementalValidationAttestationIsConjunctive(t *testing.T) {
	binding := RuntimeValidationCheckAttestationBinding{RequiredCheckRef: "requirement", ObservationProtocolRef: "owner/v3", NativeCheckRef: "original", SupplementalNativeCheckRef: "supplement"}
	base := RuntimeValidationObservation{ObservationProtocolRef: "owner/v3", ExecutedChecks: binding.NativeCheckRefs(), PassedChecks: binding.NativeCheckRefs()}
	for _, tc := range []struct {
		name   string
		mutate func(*RuntimeValidationObservation)
		passed bool
	}{
		{"all_of", func(*RuntimeValidationObservation) {}, true},
		{"original_only", func(o *RuntimeValidationObservation) {
			o.ExecutedChecks, o.PassedChecks = []string{"original"}, []string{"original"}
		}, false},
		{"supplement_only", func(o *RuntimeValidationObservation) {
			o.ExecutedChecks, o.PassedChecks = []string{"supplement"}, []string{"supplement"}
		}, false},
		{"old_protocol", func(o *RuntimeValidationObservation) { o.ObservationProtocolRef = "owner/v2" }, false},
		{"original_failed", func(o *RuntimeValidationObservation) { o.FailedChecks = []string{"original"} }, false},
		{"supplement_skipped", func(o *RuntimeValidationObservation) { o.SkippedChecks = []string{"supplement"} }, false},
		{"supplement_incomplete", func(o *RuntimeValidationObservation) { o.IncompleteChecks = []string{"supplement"} }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observation := base.Clone()
			tc.mutate(&observation)
			if got := RuntimeValidationAttestationPassed(observation, binding); got != tc.passed {
				t.Fatalf("passed=%v want=%v observation=%+v", got, tc.passed, observation)
			}
		})
	}
	collision := RuntimeValidationCheckAttestationBinding{RequiredCheckRef: "other", ObservationProtocolRef: binding.ObservationProtocolRef, NativeCheckRef: "supplement"}
	if err := ValidateRuntimeValidationCheckAttestationBindings([]RuntimeValidationCheckAttestationBinding{binding, collision}, []string{"requirement", "other"}); err == nil {
		t.Fatal("supplement reused as another requirement's native check")
	}
}

func TestValidationInputExtensionsAreSealedAndLegacyEmptyHashesUnchanged(t *testing.T) {
	for _, legacy := range []any{RuntimeValidationCheckAttestationBinding{}, RuntimeValidationFailureRepairRoute{}, RuntimeSemanticValidationFailureRepair{}} {
		encoded, err := json.Marshal(legacy)
		if err != nil || strings.Contains(string(encoded), "SupplementalNativeCheckRef") || strings.Contains(string(encoded), "InputAddition") {
			t.Fatalf("empty extension changed legacy archive shape: %s %v", encoded, err)
		}
	}
	baseline := sha256.New()
	writeHashString(baseline, "legacy-prefix")
	extended := sha256.New()
	writeHashString(extended, "legacy-prefix")
	writeRuntimeSupplementalNativeCheckHash(extended, RuntimeValidationCheckAttestationBinding{})
	writeRuntimeValidationInputAdditionHash(extended, RuntimeValidationInputAddition{})
	if !reflect.DeepEqual(baseline.Sum(nil), extended.Sum(nil)) {
		t.Fatal("empty extension rewrote archived identity")
	}
	binding := testCoverageEvidenceBinding()
	legacyHash := StableCoverageEvidenceBindingHash(binding)
	// Identity hashing must retain additional checks even before validation of
	// the complete enclosing semantic/owner record.
	binding.Proofs[0].ValidationCheckAttestations = []RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "required", ObservationProtocolRef: "owner/v3", NativeCheckRef: "original"}}
	baseHash := StableCoverageEvidenceBindingHash(binding)
	binding.Proofs[0].ValidationCheckAttestations[0].SupplementalNativeCheckRef = "supplement"
	supplementHash := StableCoverageEvidenceBindingHash(binding)
	if supplementHash == baseHash || supplementHash == legacyHash {
		t.Fatal("supplement escaped binding identity")
	}
	payload, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded CoverageEvidenceBinding
	if err := json.Unmarshal(payload, &reloaded); err != nil {
		t.Fatal(err)
	}
	if StableCoverageEvidenceBindingHash(reloaded) != supplementHash {
		t.Fatal("reload lost conjunctive binding identity")
	}
	first := ValidationFailureDiagnostic{Class: ValidationDiagnosticOutputInconclusive, InputAddition: RuntimeValidationInputAddition{RequiredCheckRef: "required", OriginalNativeCheckRef: "original", NativeCheckRef: "supplement", TargetRef: "checks/new.test"}}
	second := first
	second.InputAddition.TargetRef = "checks/other.test"
	if SameValidationFailureDiagnostic(&first, &second) {
		t.Fatal("different typed input contracts compared equal")
	}
	for _, value := range []RuntimeValidationInputAddition{first.InputAddition, second.InputAddition} {
		h := sha256.New()
		writeRuntimeValidationInputAdditionHash(h, value)
		if reflect.DeepEqual(h.Sum(nil), sha256.New().Sum(nil)) {
			t.Fatal("input addition not hashed")
		}
	}
}
