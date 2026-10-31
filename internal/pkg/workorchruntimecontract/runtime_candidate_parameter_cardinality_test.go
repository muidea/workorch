package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func cardinalityTestManifest(t *testing.T) RuntimeCandidateContractManifest {
	t.Helper()
	return mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{State: ConvergenceStateNeedStateChange, RequiredOutput: RequiredOutputContract{
		AcceptedKinds: []string{StepKindFileEdit}, AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"first.txt", "second.txt"},
		RequiredOperationSequence: []RuntimeRequiredOperationVariant{{RequirementID: "first", Kind: StepKindFileEdit, TargetRef: "first.txt"}, {RequirementID: "second", Kind: StepKindFileEdit, TargetRef: "second.txt"}},
	}})
}

const cardinalityRejectedTestDocument = `{"operations":[{"requiredOperationRef":"first","parameters":[{"name":"oldText","string":"a"},{"name":"newText","string":"b"},{"name":"oldText","string":"c"},{"name":"newText","string":"d"}]},{"requiredOperationRef":"second","parameters":[{"name":"oldText","string":"x"},{"name":"newText","string":"y"},{"name":"oldText","string":"x"},{"name":"newText","string":"z"},{"name":"occurrence","integer":1}]}]}`
const cardinalityCorrectedTestDocument = `{"operations":[{"requiredOperationRef":"first","parameters":[{"name":"newText","string":"b d"},{"name":"oldText","string":"a c"}]},{"requiredOperationRef":"second","parameters":[{"name":"oldText","string":"x x"},{"name":"newText","string":"y z"},{"name":"occurrence","integer":1}]}]}`

func TestParameterCardinalityCompleteDocumentAndFrozenCorrection(t *testing.T) {
	t.Parallel()
	m := cardinalityTestManifest(t)
	raw := []byte(cardinalityRejectedTestDocument)
	_, err := DecodeRuntimeCandidatePayloadWithManifest(m, raw)
	var diagnostic *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Path != "$.operations[0].parameters[2].name" || diagnostic.Actual != "duplicate_oldText" || diagnostic.ParameterCardinality == nil {
		t.Fatalf("missing complete diagnostic: %v", err)
	}
	v := diagnostic.ParameterCardinality
	if err := ValidateRuntimeCandidateParameterCardinality(m, v); err != nil {
		t.Fatal(err)
	}
	if v.DocumentSHA256 != cardinalitySHA(raw) || v.DocumentBytes != len(raw) || len(v.Operations) != 2 || len(v.Operations[1].Parameters) != 3 {
		t.Fatal("document identity/complete set lost")
	}
	if !v.MatchesDiagnostic(RuntimeCandidateFailureDiagnosticFromPayloadDiagnostic(diagnostic)) {
		t.Fatal("first-error lineage lost")
	}
	for i := 0; i < 3; i++ {
		_, err := DecodeRuntimeCandidatePayloadWithManifest(m, raw)
		var replay *RuntimeCandidatePayloadDiagnostic
		if !errors.As(err, &replay) || !reflect.DeepEqual(v, replay.ParameterCardinality) || string(raw) != cardinalityRejectedTestDocument {
			t.Fatal("nondeterministic replay or raw mutation")
		}
	}
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(m, []byte(cardinalityCorrectedTestDocument))
	if err != nil {
		t.Fatal(err)
	}
	ops := []RuntimeOperation{candidate.StateChange.Operations[0].Operation, candidate.StateChange.Operations[1].Operation}
	if err := ValidateRuntimeCandidateParameterCardinalityCorrection(m, v, ops); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func([]RuntimeOperation){
		"target":  func(o []RuntimeOperation) { o[0].TargetRefs = []string{"else.txt"} },
		"binding": func(o []RuntimeOperation) { o[0].BindingRefs = []string{"foreign"} },
		"slot":    func(o []RuntimeOperation) { o[0].RequiredOperationRef = "second" },
		"frozen occurrence": func(o []RuntimeOperation) {
			for i := range o[1].Parameters {
				if o[1].Parameters[i].Name == "occurrence" {
					v := int64(2)
					o[1].Parameters[i].Integer = &v
				}
			}
		},
		"extra": func(o []RuntimeOperation) {
			v := "extra"
			o[0].Parameters = append(o[0].Parameters, OperationParameter{Name: "extra", String: &v})
		},
		"still repeated": func(o []RuntimeOperation) { o[0].Parameters = append(o[0].Parameters, o[0].Parameters[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			body, _ := json.Marshal(ops)
			var changed []RuntimeOperation
			json.Unmarshal(body, &changed)
			mutate(changed)
			if ValidateRuntimeCandidateParameterCardinalityCorrection(m, v, changed) == nil {
				t.Fatal("unrelated correction accepted")
			}
		})
	}
	if ValidateRuntimeCandidateParameterCardinalityCorrection(m, v, ops[:1]) == nil {
		t.Fatal("dropped operation accepted")
	}
}

func TestParameterCardinalityRefusesPartialMixedAndOverflowFacts(t *testing.T) {
	t.Parallel()
	m := cardinalityTestManifest(t)
	for name, doc := range map[string]string{
		"invalid slot":       strings.Replace(cardinalityRejectedTestDocument, `,{"requiredOperationRef":"second"`, `,{"requiredOperationRef":"absent"`, 1),
		"type":               strings.Replace(cardinalityRejectedTestDocument, `"integer":1`, `"string":"1"`, 1),
		"unknown name":       strings.Replace(cardinalityRejectedTestDocument, `"name":"occurrence"`, `"name":"unknown"`, 1),
		"JSON duplicate key": strings.Replace(cardinalityRejectedTestDocument, `"name":"oldText"`, `"name":"ignored","name":"oldText"`, 1),
		"missing required":   strings.ReplaceAll(cardinalityRejectedTestDocument, `"name":"newText"`, `"name":"oldText"`),
		"extra object field": strings.Replace(cardinalityRejectedTestDocument, `"integer":1`, `"integer":1,"comment":"no"`, 1),
		"null sibling":       strings.Replace(cardinalityRejectedTestDocument, `"integer":1`, `"integer":1,"string":null`, 1),
		"duplicate slot":     strings.Replace(cardinalityRejectedTestDocument, `"requiredOperationRef":"second"`, `"requiredOperationRef":"first"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeRuntimeCandidatePayloadWithManifest(m, []byte(doc))
			var d *RuntimeCandidatePayloadDiagnostic
			if !errors.As(err, &d) || d.ParameterCardinality != nil {
				t.Fatalf("mixed document gained correction facts: %v", err)
			}
		})
	}
	var document map[string]any
	json.Unmarshal([]byte(cardinalityRejectedTestDocument), &document)
	ops := document["operations"].([]any)
	op := ops[1].(map[string]any)
	params := op["parameters"].([]any)
	for len(params) <= maxRuntimeCandidateCardinalityParameters {
		params = append(params, params[0])
	}
	op["parameters"] = params
	body, _ := json.Marshal(document)
	_, err := DecodeRuntimeCandidatePayloadWithManifest(m, body)
	var d *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &d) || d.ParameterCardinality != nil {
		t.Fatal("overflow produced a partial fact")
	}
}

func TestParameterCardinalityDoesNotRepairVariantOrHeaderAuthority(t *testing.T) {
	m := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{State: ConvergenceStateNeedStateChange, RequiredOutput: RequiredOutputContract{AcceptedKinds: []string{RuntimeBlockKindPatchApply}, AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go", "second.go"}}})
	doc := `{"operations":[{"kind":"patch/apply","targetRefs":["main.go"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"patchType","string":"text_replace"},{"name":"content","string":"text"},{"name":"baseHash","string":"base"}]}]}`
	_, err := DecodeRuntimeCandidatePayloadWithManifest(m, []byte(doc))
	var diagnostic *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Actual != "duplicate_patchType" || diagnostic.ParameterCardinality != nil {
		t.Fatalf("ambiguous variant gained correction authority: %v", err)
	}
	doc = `{"operations":[{"kind":"patch/apply","targetRefs":null,"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"a"},{"name":"content","string":"b"},{"name":"baseHash","string":"base"}]}]}`
	_, err = DecodeRuntimeCandidatePayloadWithManifest(m, []byte(doc))
	if !errors.As(err, &diagnostic) || diagnostic.Actual != "duplicate_content" || diagnostic.ParameterCardinality != nil {
		t.Fatalf("missing header selector gained correction authority: %v", err)
	}
}

func TestParameterCardinalityHashesCloneAndObservationBinding(t *testing.T) {
	t.Parallel()
	m := cardinalityTestManifest(t)
	_, err := DecodeRuntimeCandidatePayloadWithManifest(m, []byte(cardinalityRejectedTestDocument))
	var raw *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &raw) {
		t.Fatal(err)
	}
	d := RuntimeCandidateFailureDiagnosticFromPayloadDiagnostic(raw)
	d.ManifestHash = m.Hash
	d.Objectives = []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionConsolidateParameters}
	w := RuntimeCandidateCorrectionWitness{Dimension: RuntimeCandidateCorrectionWitnessParameterCardinality, Path: "$.operations", RejectedValue: RuntimeCandidateParameterNamesRepeated, AlternativeValue: RuntimeCandidateParameterNamesUnique, ParameterCardinality: d.ParameterCardinality.Clone()}
	o := RuntimeCandidateCorrectionObservation{Class: RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "cardinality", Diagnostic: d, SealedCorrectionHash: StableRuntimeCandidateSealedCorrectionHash(m.Hash, d.Objectives, &w)}
	if err := ValidateRuntimeCandidateCorrectionAgainstObservation(o, m.Hash, d.Objectives, w); err != nil {
		t.Fatal(err)
	}
	hash := StableRuntimeCandidateRequestCorrectionHash(&o, m.Hash, d.Objectives, &w)
	copy := d.Clone()
	copy.ParameterCardinality.Operations[0].Parameters[0].Indexes[1] = 99
	if reflect.DeepEqual(copy.ParameterCardinality, d.ParameterCardinality) || d.ParameterCardinality.Validate() != nil {
		t.Fatal("diagnostic clone aliases original")
	}
	for name, mutate := range map[string]func(*RuntimeCandidateParameterCardinalityViolation){
		"index": func(v *RuntimeCandidateParameterCardinalityViolation) { v.Operations[0].Parameters[0].Indexes[1] = 99 },
		"wire": func(v *RuntimeCandidateParameterCardinalityViolation) {
			v.Operations[0].WireRule.WireContractHash = "foreign"
		},
		"name": func(v *RuntimeCandidateParameterCardinalityViolation) {
			v.Operations[0].Parameters[0].Name = "undeclared"
		},
		"document":          func(v *RuntimeCandidateParameterCardinalityViolation) { v.DocumentSHA256 = strings.Repeat("0", 64) },
		"omitted operation": func(v *RuntimeCandidateParameterCardinalityViolation) { v.Operations = v.Operations[:1] },
		"omitted group": func(v *RuntimeCandidateParameterCardinalityViolation) {
			v.Operations[1].Parameters = v.Operations[1].Parameters[1:]
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := *w.Clone()
			mutate(changed.ParameterCardinality)
			if ValidateRuntimeCandidateCorrectionAgainstObservation(o, m.Hash, d.Objectives, changed) == nil {
				t.Fatal("tampered witness accepted")
			}
			if StableRuntimeCandidateRequestCorrectionHash(&o, m.Hash, d.Objectives, &changed) == hash {
				t.Fatal("hash omitted cardinality fact")
			}
		})
	}
}
