package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestValidationInputReviewV2BoundsWholeReportAndPreservesUnicodeCriteria(t *testing.T) {
	_, document, binding := semanticInputReviewV2Fixture(t)
	report := document.Proposals[1].ValidationInputReview.Clone()
	check := &report.Checks[0]
	check.Obligations = nil
	for i := 0; i < RuntimeValidationInputReviewMaxObligations; i++ {
		check.Obligations = append(check.Obligations, RuntimeValidationInputObligationReview{
			CriterionText: fmt.Sprintf("original criterion %d", i), Finding: RuntimeValidationInputCovered,
			Reason: strings.Repeat("x", RuntimeValidationInputReviewMaxReasonBytes), InputTargetRefs: check.InputTargetRefs,
		})
	}
	encoded, err := json.Marshal(report)
	if err != nil || len(encoded) <= RuntimeValidationInputReviewMaxReportBytes || report.ValidateAgainstBinding(binding) == nil {
		t.Fatal("individually bounded fields bypassed the whole-report byte budget")
	}
	check.Obligations = check.Obligations[:8]
	if err := report.ValidateAgainstBinding(binding); err != nil {
		t.Fatalf("bounded complete wire report rejected: %v", err)
	}
	for name, test := range map[string]struct {
		original string
		excerpts []string
		valid    bool
	}{
		"complete Chinese":   {"提供工具；\n并提供文档。", []string{"提供工具；", "并提供文档。"}, true},
		"missing conjunct":   {"提供工具；\n并提供文档。", []string{"提供工具；"}, false},
		"missing connector":  {"提供工具；\n并提供文档。", []string{"提供工具；", "提供文档。"}, false},
		"foreign paraphrase": {"提供工具；\n并提供文档。", []string{"提供工具；", "并说明每个工具。"}, false},
		"repeated source":    {"run run run", []string{"run"}, true},
	} {
		t.Run(name, func(t *testing.T) {
			value := RuntimeValidationInputReview{Checks: []RuntimeValidationInputCheckReview{{}}}
			for _, excerpt := range test.excerpts {
				value.Checks[0].Obligations = append(value.Checks[0].Obligations, RuntimeValidationInputObligationReview{CriterionText: excerpt})
			}
			if (value.ValidateCriterionText(test.original) == nil) != test.valid {
				t.Fatal("textual provenance did not preserve the immutable criterion")
			}
		})
	}
}

func semanticInputReviewV2Fixture(t *testing.T) (RuntimeSemanticPlanManifest, RuntimeSemanticPlanDocument, RuntimeValidationProofRequirementBinding) {
	t.Helper()
	manifest, document, _, binding := semanticInputReviewFixture(t)
	binding.InputReview.ProtocolRef = RuntimeValidationInputReviewProtocol
	binding.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
	for i := range manifest.OwnerCatalogs {
		catalog := &manifest.OwnerCatalogs[i]
		if catalog.OwnerModuleID != RuntimeModuleShell {
			continue
		}
		catalog.ValidationSurfaces[0].ProofRequirementBindings[0] = binding.Clone()
		catalog.ValidationRecipes = []RuntimeValidationRecipe{{ID: binding.ValidationRecipeID, AdapterRef: "owner/review/v1", Command: "check", InputReviewProtocolRef: RuntimeValidationInputReviewProtocol}}
		catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	document.ManifestHash = manifest.Hash
	review := document.Proposals[1].ValidationInputReview
	review.SubjectHash = binding.InputReview.Hash
	for i := range review.Checks {
		review.Checks[i].Obligations = []RuntimeValidationInputObligationReview{{CriterionText: manifest.CoverageItems[0].TargetText, Finding: RuntimeValidationInputCovered, Reason: "The complete criterion has an explicit assertion.", InputTargetRefs: append([]string(nil), review.Checks[i].InputTargetRefs...)}}
	}
	return manifest, document, binding
}

func TestValidationInputReviewV2AdmissionAndNoCreditForOmissions(t *testing.T) {
	manifest, document, binding := semanticInputReviewV2Fixture(t)
	before := cloneRuntimeSemanticPlanDocumentRaw(document)
	payload, err := EncodeRuntimeSemanticPlanPayloadWithManifest(manifest, document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, payload)
	if err != nil || !reflect.DeepEqual(decoded.Normalized(), document.Normalized()) {
		t.Fatalf("complete v2 report round trip: %v", err)
	}
	for name, mutate := range map[string]func(*RuntimeValidationInputReview){
		"missing mapping":       func(r *RuntimeValidationInputReview) { r.Checks[0].Obligations = nil },
		"partial original text": func(r *RuntimeValidationInputReview) { r.Checks[0].Obligations[0].CriterionText = "work" },
		"foreign criterion": func(r *RuntimeValidationInputReview) {
			r.Checks[0].Obligations[0].CriterionText = "replacement criterion"
		},
		"duplicate excerpt": func(r *RuntimeValidationInputReview) {
			r.Checks[0].Obligations = append(r.Checks[0].Obligations, r.Checks[0].Obligations[0])
		},
		"false consistent": func(r *RuntimeValidationInputReview) {
			r.Checks[0].Obligations[0].Finding = RuntimeValidationInputMissing
		},
		"unknown observation": func(r *RuntimeValidationInputReview) { r.Checks[0].Obligations[0].Finding = "approve" },
		"unobserved file": func(r *RuntimeValidationInputReview) {
			r.Checks[0].Obligations[0].InputTargetRefs = []string{"unobserved"}
		},
		"unbounded mapping": func(r *RuntimeValidationInputReview) {
			r.Checks[0].Obligations = make([]RuntimeValidationInputObligationReview, RuntimeValidationInputReviewMaxObligations+1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := cloneRuntimeSemanticPlanDocumentRaw(document)
			mutate(bad.Proposals[1].ValidationInputReview)
			if bad.ValidateAgainstManifest(manifest) == nil {
				t.Fatal("invalid v2 report admitted without provider schema")
			}
		})
	}
	if !reflect.DeepEqual(document, before) {
		t.Fatal("nested obligation clone aliases the sealed report")
	}
	for _, finding := range []string{RuntimeValidationInputMissing, RuntimeValidationInputConflicting, RuntimeValidationInputUnknown} {
		bad := cloneRuntimeSemanticPlanDocumentRaw(document)
		check := &bad.Proposals[1].ValidationInputReview.Checks[0]
		check.Obligations[0].Finding = finding
		check.Conclusion = RuntimeValidationInputConflict
		if finding == RuntimeValidationInputUnknown {
			check.Conclusion = RuntimeValidationInputInconclusive
		}
		if bad.ValidateAgainstManifest(manifest) != nil {
			t.Fatal("complete negative observation treated as malformed")
		}
		facts := []RuntimeSemanticProofFeasibility{
			semanticFeasibilityForTest(t, manifest, bad.Proposals[0], []string{"main.go"}, "", nil),
			semanticFeasibilityForTest(t, manifest, bad.Proposals[1], []string{"main.go"}, binding.ValidationRecipeID, binding.RequiredCheckRefs),
		}
		if focuses, _, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, bad, facts); err == nil || len(focuses) != 0 {
			t.Fatal("negative input review became a validation binding")
		}
		if StableRuntimeSemanticPlanDocumentHash(bad) == StableRuntimeSemanticPlanDocumentHash(document) {
			t.Fatal("semantic document hash ignored obligation observations")
		}
	}
	for _, replacement := range []string{`"obligations":null`, `"obligations":[]`, `"obligations":[{"criterionText":"works","finding":"covered","reason":"ok","inputTargetRefs":["checks/input_test.go"],"approval":true}]`} {
		// Exercise the closed wire shape directly as well as manifest admission.
		raw := `{"subjectHash":"subject","checks":[{"requiredCheckRef":"check","conclusion":"consistent","reason":"ok","inputTargetRefs":["input"],` + replacement + `}]}`
		if validateRuntimeValidationInputReviewPayload("$.review", []byte(raw)) == nil {
			t.Fatal("open, null or empty obligation payload accepted")
		}
	}
}

func TestValidationInputReviewV2DistinguishesMissingDocumentationFromContradiction(t *testing.T) {
	_, document, binding := semanticInputReviewV2Fixture(t)
	report := document.Proposals[1].ValidationInputReview.Clone()
	for i := range report.Checks {
		check := &report.Checks[i]
		check.Conclusion = RuntimeValidationInputConflict
		check.Obligations = []RuntimeValidationInputObligationReview{
			{CriterionText: "provide the required tools", Finding: RuntimeValidationInputCovered, Reason: "The input asserts the complete tool inventory.", InputTargetRefs: check.InputTargetRefs},
			{CriterionText: "and document every tool", Finding: RuntimeValidationInputMissing, Reason: "The input contains no assertion of the documentation obligation.", InputTargetRefs: check.InputTargetRefs},
		}
	}
	if report.ValidateAgainstBinding(binding) != nil || report.ValidateCriterionText("provide the required tools and document every tool") != nil || !RuntimeValidationInputReviewOnlyCoverageOmissions(report) {
		t.Fatal("complete typed coverage omission was not distinguished")
	}
	partial := report.Clone()
	partial.Checks[0].Obligations = partial.Checks[0].Obligations[:1]
	partial.Checks[0].Conclusion = RuntimeValidationInputConsistent
	if partial.ValidateCriterionText("provide the required tools and document every tool") == nil {
		t.Fatal("omitted documentation was admitted as a complete criterion mapping")
	}
	for _, finding := range []string{RuntimeValidationInputConflicting, RuntimeValidationInputUnknown} {
		changed := report.Clone()
		changed.Checks[0].Obligations[0].Finding = finding
		if RuntimeValidationInputReviewOnlyCoverageOmissions(changed) {
			t.Fatal("contradiction or unknown facts were reclassified as additive-only")
		}
	}
}

func TestValidationInputReviewRequiresCompleteCurrentShapeAndRejectsUnknownProtocol(t *testing.T) {
	manifest, document, _, old := semanticInputReviewFixture(t)
	legacy := document.Proposals[1].ValidationInputReview
	bytes, _ := json.Marshal(legacy)
	if !strings.Contains(string(bytes), "obligations") || legacy.ValidateAgainstBinding(old) != nil {
		t.Fatal("legacy report was changed")
	}
	empty := legacy.Clone()
	empty.Checks[0].Obligations = []RuntimeValidationInputObligationReview{}
	if empty.Clone().ValidateAgainstBinding(old) == nil {
		t.Fatal("clone laundered an undeclared v2 extension")
	}
	v2, _, _ := semanticInputReviewV2Fixture(t)
	for i := range v2.OwnerCatalogs {
		catalog := &v2.OwnerCatalogs[i]
		if catalog.OwnerModuleID != RuntimeModuleShell {
			continue
		}
		for _, mode := range []string{"drop review", "downgrade protocol", "undeclared protocol"} {
			bad := catalog.Clone()
			binding := &bad.ValidationSurfaces[0].ProofRequirementBindings[0]
			switch mode {
			case "drop review":
				binding.InputReview = nil
			case "downgrade protocol":
				binding.InputReview.ProtocolRef = "validation/input-consistency/v1"
				binding.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, *binding)
			default:
				bad.ValidationRecipes[0].InputReviewProtocolRef = ""
			}
			bad.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(bad)
			if bad.Validate() == nil {
				t.Fatalf("owner bypassed protocol seal: %s", mode)
			}
		}
	}
	if manifest.Hash == v2.Hash {
		t.Fatal("new review protocol reused the legacy manifest")
	}
}
