package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestValidationInputMaintenanceRejectsConflictingBindingMappings(t *testing.T) {
	addition := RuntimeValidationInputAddition{RequiredCheckRef: "check", OriginalNativeCheckRef: "original", NativeCheckRef: "supplement", TargetRef: "checks/input.test"}
	repair := testValidationInputRepair()
	for _, repairing := range []bool{false, true} {
		base := RuntimeValidationCheckAttestationBinding{RequiredCheckRef: "check", ObservationProtocolRef: "owner/v4", NativeCheckRef: "original"}
		matches := addition.MatchesBinding
		if repairing {
			base.SupplementalNativeCheckRef = "supplement"
			matches = repair.MatchesBinding
		}
		for _, tc := range []struct {
			name  string
			extra RuntimeValidationCheckAttestationBinding
			want  bool
		}{
			{"independent refinement", RuntimeValidationCheckAttestationBinding{RequiredCheckRef: "other", ObservationProtocolRef: "owner/v4", NativeCheckRef: "other-original", SupplementalNativeCheckRef: "other-supplement"}, true},
			{"duplicate", base, false},
			{"conflicting original", RuntimeValidationCheckAttestationBinding{RequiredCheckRef: "check", ObservationProtocolRef: "owner/v4", NativeCheckRef: "replacement"}, false},
			{"reused original", RuntimeValidationCheckAttestationBinding{RequiredCheckRef: "other", ObservationProtocolRef: "owner/v4", NativeCheckRef: "original"}, false},
			{"reused supplement", RuntimeValidationCheckAttestationBinding{RequiredCheckRef: "other", ObservationProtocolRef: "owner/v4", NativeCheckRef: "supplement"}, false},
			{"reused supplemental mapping", RuntimeValidationCheckAttestationBinding{RequiredCheckRef: "other", ObservationProtocolRef: "owner/v4", NativeCheckRef: "other", SupplementalNativeCheckRef: "supplement"}, false},
		} {
			t.Run(tc.name+map[bool]string{true: "/repair", false: "/addition"}[repairing], func(t *testing.T) {
				for _, reverse := range []bool{false, true} {
					proofs := []CoverageEvidenceProof{
						{EvidenceKind: EvidenceKindValidation, ValidationCheckAttestations: []RuntimeValidationCheckAttestationBinding{base}},
						{EvidenceKind: EvidenceKindValidation, ValidationCheckAttestations: []RuntimeValidationCheckAttestationBinding{tc.extra}},
					}
					if reverse {
						proofs[0], proofs[1] = proofs[1], proofs[0]
					}
					binding := CoverageEvidenceBinding{Proofs: proofs}
					body, err := json.Marshal(binding)
					if err != nil {
						t.Fatal(err)
					}
					var reloaded CoverageEvidenceBinding
					if err := json.Unmarshal(body, &reloaded); err != nil {
						t.Fatal(err)
					}
					for i := 0; i < 2; i++ {
						if matches(binding) != tc.want || matches(reloaded) != tc.want {
							t.Fatalf("mapping admission mismatch: %+v", binding)
						}
					}
					if !reflect.DeepEqual(binding, reloaded) {
						t.Fatal("pure admission mutated binding")
					}
				}
			})
		}
	}
}
