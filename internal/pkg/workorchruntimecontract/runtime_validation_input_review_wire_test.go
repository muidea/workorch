package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func inputReviewWireForBinding(t *testing.T, binding RuntimeValidationProofRequirementBinding, report RuntimeValidationInputReview) *runtimeValidationInputReviewPayload {
	t.Helper()
	manifest, document, _, _ := semanticInputReviewFixture(t)
	proposal := document.Proposals[1]
	for i := range manifest.OwnerCatalogs {
		catalog := &manifest.OwnerCatalogs[i]
		if catalog.OwnerModuleID == proposal.OwnerModuleID {
			catalog.ValidationSurfaces[0].ProofRequirementBindings[0] = binding.Clone()
		}
	}
	wire, err := runtimeValidationInputReviewToWire(&report, proposal, manifest.OwnerCatalogs)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

func inputReviewSourceWireFixture(t *testing.T) (RuntimeSemanticPlanManifest, RuntimeSemanticPlanDocument) {
	t.Helper()
	manifest, document, _, _ := semanticInputReviewFixture(t)
	binding, report := diagnosticSourceReviewFixture(t)
	content := "complete supporting fixture\n"
	binding.InputReview.Files = append(binding.InputReview.Files, RuntimeValidationInputReviewFile{TargetRef: "checks/support.go", Content: content, ContentHash: RuntimeValidationInputContentHash(content)})
	binding.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
	report.SubjectHash = binding.InputReview.Hash
	for i := range report.Checks {
		report.Checks[i].Obligations[0].CriterionText = document.Proposals[1].ValidationInputReview.Checks[i].Obligations[0].CriterionText
	}
	report.DiagnosticSources[0].InputTargetRefs = []string{"checks/input_test.go", "checks/support.go"}
	for i := range manifest.OwnerCatalogs {
		if manifest.OwnerCatalogs[i].OwnerModuleID == document.Proposals[1].OwnerModuleID {
			manifest.OwnerCatalogs[i].ValidationSurfaces[0].ProofRequirementBindings[0] = binding
		}
	}
	manifest = sealSemanticTestManifest(t, manifest)
	document.ManifestHash = manifest.Hash
	document.Proposals[1].ValidationInputReview = &report
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	return manifest, document.Normalized()
}

func TestInputReviewWireDerivesOnlyExactOwnerReferences(t *testing.T) {
	manifest, document := inputReviewSourceWireFixture(t)
	proposalIndex := inputReviewProposalIndex(t, document)
	raw, err := EncodeRuntimeSemanticPlanPayloadWithManifest(manifest, document)
	if err != nil {
		t.Fatal(err)
	}
	var wire runtimeSemanticPlanPayload
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	review := wire.Proposals[proposalIndex].ValidationInputReview
	if !reflect.DeepEqual(review.DiagnosticSources[0].SupportingInputTargetRefs, []string{"checks/support.go"}) || len(review.DiagnosticSources[1].SupportingInputTargetRefs) != 0 {
		t.Fatal("lost supporting observation or exposed fixed anchor")
	}
	var object map[string]any
	_ = json.Unmarshal(raw, &object)
	r := object["proposals"].([]any)[proposalIndex].(map[string]any)["validationInputReview"].(map[string]any)
	for _, group := range []string{"checks", "diagnosticSources"} {
		for _, item := range r[group].([]any) {
			if _, present := item.(map[string]any)["inputTargetRefs"]; present {
				t.Fatal("redundant owner refs on provider wire")
			}
		}
	}
	before := append([]byte(nil), raw...)
	for range 2 {
		decoded, err := DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, raw)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decoded.Proposals[proposalIndex].ValidationInputReview, document.Proposals[proposalIndex].ValidationInputReview) || StableRuntimeSemanticPlanDocumentHash(decoded) != StableRuntimeSemanticPlanDocumentHash(document) {
			t.Fatal("wire lost complete report or semantic identity")
		}
		decoded.Proposals[proposalIndex].ValidationInputReview.DiagnosticSources[0].InputTargetRefs[0] = "mutated"
	}
	if !bytes.Equal(before, raw) {
		t.Fatal("decoder changed raw provider bytes")
	}
	bad := document.Proposals[proposalIndex].ValidationInputReview.Clone()
	bad.Checks[0].InputTargetRefs = append(bad.Checks[0].InputTargetRefs, "checks/support.go")
	if _, err := runtimeValidationInputReviewToWire(&bad, document.Proposals[proposalIndex], manifest.OwnerCatalogs); err == nil {
		t.Fatal("encoder silently lost check scope")
	}
}

func TestInputReviewWireRejectsMissingForeignAndRetiredFields(t *testing.T) {
	manifest, document := inputReviewSourceWireFixture(t)
	proposalIndex := inputReviewProposalIndex(t, document)
	raw, err := EncodeRuntimeSemanticPlanPayloadWithManifest(manifest, document)
	if err != nil {
		t.Fatal(err)
	}
	schemaText, err := RuntimeSemanticPlanJSONSchemaForRequest(manifest, document.PlanStep, document.BatchID, document.BatchSequence, document.BatchCoverageWorkItemIDs)
	if err != nil {
		t.Fatal(err)
	}
	schemaDoc, err := jsonschema.UnmarshalJSON(strings.NewReader(schemaText))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("https://workorch.invalid/review-wire.json", schemaDoc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("https://workorch.invalid/review-wire.json")
	if err != nil {
		t.Fatal(err)
	}
	var valid any
	_ = json.Unmarshal(raw, &valid)
	if err := schema.Validate(valid); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"retired check", "retired source", "foreign obligation", "empty obligation refs", "missing obligations", "duplicate obligation", "missing source", "duplicate source", "foreign source", "stale subject", "foreign owner", "foreign recipe", "foreign cwi", "foreign support", "repeat anchor", "duplicate support", "empty support", "null support", "wrong support type"} {
		t.Run(name, func(t *testing.T) {
			var wire map[string]any
			_ = json.Unmarshal(raw, &wire)
			p := wire["proposals"].([]any)[proposalIndex].(map[string]any)
			r := p["validationInputReview"].(map[string]any)
			c := r["checks"].([]any)[0].(map[string]any)
			s := r["diagnosticSources"].([]any)[0].(map[string]any)
			switch name {
			case "retired check":
				c["inputTargetRefs"] = []string{"checks/input_test.go"}
			case "retired source":
				s["inputTargetRefs"] = []string{"checks/input_test.go"}
			case "foreign obligation":
				c["obligations"].([]any)[0].(map[string]any)["inputTargetRefs"] = []string{"foreign/input"}
			case "empty obligation refs":
				c["obligations"].([]any)[0].(map[string]any)["inputTargetRefs"] = []string{}
			case "missing obligations":
				delete(c, "obligations")
			case "duplicate obligation":
				c["obligations"] = append(c["obligations"].([]any), c["obligations"].([]any)[0])
			case "missing source":
				r["diagnosticSources"] = r["diagnosticSources"].([]any)[:1]
			case "duplicate source":
				r["diagnosticSources"].([]any)[1] = s
			case "foreign source":
				s["sourceRef"] = "foreign"
			case "stale subject":
				r["subjectHash"] = "stale"
			case "foreign owner":
				p["ownerModuleId"] = "foreign/owner"
			case "foreign recipe":
				p["validationRecipeId"] = "foreign/recipe"
			case "foreign cwi":
				p["coverageWorkItemId"] = "foreign-cwi"
			case "foreign support":
				s["supportingInputTargetRefs"] = []string{"foreign/input"}
			case "repeat anchor":
				s["supportingInputTargetRefs"] = []string{"checks/input_test.go"}
			case "duplicate support":
				s["supportingInputTargetRefs"] = []string{"checks/support.go", "checks/support.go"}
			case "empty support":
				s["supportingInputTargetRefs"] = []string{}
			case "null support":
				s["supportingInputTargetRefs"] = nil
			case "wrong support type":
				s["supportingInputTargetRefs"] = "checks/support.go"
			}
			changed, _ := json.Marshal(wire)
			before := append([]byte(nil), changed...)
			_, decodeErr := DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, changed)
			if decodeErr == nil {
				t.Fatal("invalid wire admitted")
			}
			if name == "foreign support" || name == "empty obligation refs" {
				var diagnostic *RuntimeCandidatePayloadDiagnostic
				if !errors.As(decodeErr, &diagnostic) || len(diagnostic.SemanticViolations) != 1 || diagnostic.SemanticViolations[0].ValidateAgainstManifest(manifest) != nil {
					t.Fatalf("missing exact correction witness: %v", decodeErr)
				}
				suffix := ".supportingInputTargetRefs"
				if name == "empty obligation refs" {
					suffix = ".obligations[0].inputTargetRefs"
				}
				if !strings.HasSuffix(diagnostic.Path, suffix) {
					t.Fatalf("correction names a derived field: %s", diagnostic.Path)
				}
			}
			if !bytes.Equal(before, changed) {
				t.Fatal("rejected original was rewritten")
			}
			// Semantic duplicate excerpts are enforced locally; JSON Schema only
			// constrains transport shape, membership, and complete selected sets.
			if name != "duplicate obligation" && schema.Validate(wire) == nil {
				t.Fatal("schema permits invalid current wire")
			}
		})
	}
}

func inputReviewProposalIndex(t *testing.T, document RuntimeSemanticPlanDocument) int {
	t.Helper()
	for i, p := range document.Proposals {
		if p.ValidationInputReview != nil {
			return i
		}
	}
	t.Fatal("fixture lacks review")
	return -1
}
