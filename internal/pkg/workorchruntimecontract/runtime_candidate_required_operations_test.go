package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func requiredOperationsManifest(t *testing.T) RuntimeCandidateContractManifest {
	t.Helper()
	return mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{State: ConvergenceStateNeedStateChange, RequiredOutput: RequiredOutputContract{AcceptedKinds: []string{StepKindGitWrite}, AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredOperationSequence: []RuntimeRequiredOperationVariant{{RequirementID: "stage", Kind: StepKindGitWrite, VariantID: "add"}, {RequirementID: "commit", Kind: StepKindGitWrite, VariantID: "commit", DependsOnRequirementIDs: []string{"stage"}}}}})
}

func TestRequiredOperationsHashesCloneAndObservationBinding(t *testing.T) {
	t.Parallel()
	m := requiredOperationsManifest(t)
	_, err := DecodeRuntimeCandidatePayloadWithManifest(m, []byte(`{"operations":[{"requiredOperationRef":"stage","parameters":[{"name":"pathspecs","stringList":["main.go"]}]}]}`))
	var raw *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &raw) || raw.RequiredOperations == nil {
		t.Fatal(err)
	}
	d := RuntimeCandidateFailureDiagnosticFromPayloadDiagnostic(raw)
	d.ManifestHash = m.Hash
	d.Objectives = []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionSupplyRequiredOperations}
	w := RuntimeCandidateCorrectionWitness{Dimension: RuntimeCandidateCorrectionWitnessRequiredOperations, Path: "$.operations", RejectedValue: "required_slots_missing", AlternativeValue: "all_required_slots_present", RequiredOperations: d.RequiredOperations.Clone()}
	o := RuntimeCandidateCorrectionObservation{Class: RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "required-operations", Diagnostic: d, SealedCorrectionHash: StableRuntimeCandidateSealedCorrectionHash(m.Hash, d.Objectives, &w)}
	if err := ValidateRuntimeCandidateCorrectionAgainstObservation(o, m.Hash, d.Objectives, w); err != nil {
		t.Fatal(err)
	}
	requestHash := StableRuntimeCandidateRequestCorrectionHash(&o, m.Hash, d.Objectives, &w)
	for name, mutate := range map[string]func(*RuntimeCandidateRequiredOperationsViolation){
		"manifest":      func(v *RuntimeCandidateRequiredOperationsViolation) { v.ManifestHash = "foreign" },
		"document":      func(v *RuntimeCandidateRequiredOperationsViolation) { v.DocumentSHA256 = strings.Repeat("0", 64) },
		"bytes":         func(v *RuntimeCandidateRequiredOperationsViolation) { v.DocumentBytes++ },
		"missing":       func(v *RuntimeCandidateRequiredOperationsViolation) { v.MissingRefs[0] = "foreign" },
		"retained slot": func(v *RuntimeCandidateRequiredOperationsViolation) { v.Retained[0].RequiredOperationRef = "foreign" },
		"retained value": func(v *RuntimeCandidateRequiredOperationsViolation) {
			v.Retained[0].ValueHash = strings.Repeat("0", 64)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := *w.Clone()
			mutate(changed.RequiredOperations)
			if ValidateRuntimeCandidateCorrectionAgainstObservation(o, m.Hash, d.Objectives, changed) == nil {
				t.Fatal("tampered witness accepted")
			}
			if StableRuntimeCandidateRequestCorrectionHash(&o, m.Hash, d.Objectives, &changed) == requestHash || StableRuntimeCandidateSealedCorrectionHash(m.Hash, d.Objectives, &changed) == o.SealedCorrectionHash {
				t.Fatal("hash omitted required-operation source")
			}
		})
	}
	for name, mutate := range map[string]func(*RuntimeCandidateFailureDiagnostic){
		"path":         func(d *RuntimeCandidateFailureDiagnostic) { d.Path = "$.other" },
		"actual":       func(d *RuntimeCandidateFailureDiagnostic) { d.Actual = "missing_required_operation_foreign" },
		"required set": func(d *RuntimeCandidateFailureDiagnostic) { d.Required = d.Required[:1] },
	} {
		t.Run(name, func(t *testing.T) {
			changed := o.Clone()
			mutate(changed.Diagnostic)
			if changed.Validate() == nil {
				t.Fatal("diagnostic changed exact omission source")
			}
		})
	}
	if err := ValidateRuntimeCandidateCorrectionAgainstObservation(o, m.Hash, d.Objectives, w); err != nil {
		t.Fatal("detached mutations changed original correction", err)
	}
}
func TestCandidateRequiredOperationsCompleteOmissionAndPartialCorrection(t *testing.T) {
	m := requiredOperationsManifest(t)
	stage := `{"requiredOperationRef":"stage","parameters":[{"name":"pathspecs","stringList":["main.go"]}]}`
	commit := `{"requiredOperationRef":"commit","parameters":[{"name":"message","string":"deliver"}]}`
	for _, raw := range []string{`{"operations":[]}`, `{"operations":[` + stage + `]}`} {
		t.Run(raw, func(t *testing.T) {
			_, err := DecodeRuntimeCandidatePayloadWithManifest(m, []byte(raw))
			var d *RuntimeCandidatePayloadDiagnostic
			if !errors.As(err, &d) || d.RequiredOperations == nil {
				t.Fatalf("missing typed omission: %v", err)
			}
			fact := d.RequiredOperations
			if fact.DocumentBytes != len(raw) || fact.DocumentSHA256 != cardinalitySHA([]byte(raw)) || ValidateRuntimeCandidateRequiredOperations(m, fact) != nil {
				t.Fatal("lost original document/manifest identity")
			}
			full, err := DecodeRuntimeCandidatePayloadWithManifest(m, []byte(`{"operations":[`+commit+`,`+stage+`]}`))
			if err != nil {
				t.Fatal(err)
			}
			ops, _ := runtimeCandidatePayloadOperations(full)
			if err := ValidateRuntimeCandidateRequiredOperationsCorrection(m, fact, ops); err != nil {
				t.Fatal(err)
			}
			drift := m.Clone()
			drift.Hash = "foreign"
			if ValidateRuntimeCandidateRequiredOperations(drift, fact) == nil {
				t.Fatal("foreign manifest accepted")
			}
			if ValidateRuntimeCandidateRequiredOperationsCorrection(m, fact, ops[:1]) == nil {
				t.Fatal("partial retry accepted")
			}
			if len(fact.Retained) > 0 {
				changed, err := DecodeRuntimeCandidatePayloadWithManifest(m, []byte(`{"operations":[`+strings.ReplaceAll(stage, "main.go", "other.go")+`,`+commit+`]}`))
				if err != nil {
					t.Fatal(err)
				}
				ops, _ = runtimeCandidatePayloadOperations(changed)
				if ValidateRuntimeCandidateRequiredOperationsCorrection(m, fact, ops) == nil {
					t.Fatal("retained value changed")
				}
			}
			body, _ := json.Marshal(fact)
			var restored RuntimeCandidateRequiredOperationsViolation
			if json.Unmarshal(body, &restored) != nil || ValidateRuntimeCandidateRequiredOperations(m, &restored) != nil {
				t.Fatal("reload lost typed source")
			}
		})
	}
	for _, raw := range []string{`{"operations":[` + stage + `,` + stage + `]}`, `{"operations":[{"requiredOperationRef":"unknown","parameters":[]}]}`, `{"operations":[],"operations":[]}`} {
		_, err := DecodeRuntimeCandidatePayloadWithManifest(m, []byte(raw))
		var d *RuntimeCandidatePayloadDiagnostic
		if err == nil {
			t.Fatal("unknown/duplicate accepted")
		}
		if errors.As(err, &d) && d.RequiredOperations != nil {
			t.Fatal("ambiguous source received omission correction")
		}
	}
}
