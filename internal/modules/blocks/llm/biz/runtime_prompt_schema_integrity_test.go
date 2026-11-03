package llm

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPromptSchemaSharingPreservesLiteralDataAndExactNumbers(t *testing.T) {
	// These keys are literal business values, not schema annotations or refs.
	literal := `{"title":"exact title","description":"exact description","$ref":"literal","id":"literal id","number":9007199254740993}`
	child := `{"type":"object","description":"schema prose","properties":{"id":{"const":` + literal + `},"title":{"enum":[` + literal + `]}},"required":["id","title"],"additionalProperties":false}`
	raw := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://workorch.invalid/prompt","type":"object","properties":{"first":` + child + `,"second":` + child + `}}`)
	compact := compactSemanticPlanPromptSchema(raw)
	if !bytes.Contains(compact, []byte(`"$defs"`)) || len(compact) >= len(raw) || bytes.Contains(compact, []byte("schema prose")) {
		t.Fatal("schema sharing/annotation removal missing")
	}
	decode := func(raw []byte) any {
		var value any
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if err := d.Decode(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	var expanded any
	value := decode(compact).(map[string]any)
	defs := value["$defs"].(map[string]any)
	expanded = visitPromptSchema(value, func(node map[string]any) any {
		if ref, ok := node["$ref"].(string); ok {
			body, found := defs[strings.TrimPrefix(ref, "#/$defs/")]
			if !found || len(node) != 1 {
				t.Fatal("invalid local ref")
			}
			return body
		}
		delete(node, "$defs")
		return node
	})
	expected := decode(raw)
	visitPromptSchema(expected, func(node map[string]any) any { delete(node, "description"); return node })
	if !reflect.DeepEqual(expanded, expected) {
		t.Fatal("literal fields or exact number changed")
	}
	for range 3 {
		if !bytes.Equal(compact, compactSemanticPlanPromptSchema(raw)) {
			t.Fatal("non-deterministic prompt schema")
		}
	}
}

func TestSemanticSchemaIntegrityRejectsRenderedLossOrMutation(t *testing.T) {
	for _, field := range []string{"missing", "weakened", "foreign"} {
		t.Run(field, func(t *testing.T) {
			r := runtimeSemanticProviderTestRequest(t)
			p, err := runtimeProviderRequest(r)
			if err != nil {
				t.Fatal(err)
			}
			var authority runtimeSemanticAuthorityProjection
			if err := json.Unmarshal([]byte(p.PromptContext.Sections[3].Content), &authority); err != nil {
				t.Fatal(err)
			}
			switch field {
			case "missing":
				authority.Schema = nil
			case "weakened":
				authority.Schema = json.RawMessage(`{"type":"object"}`)
			case "foreign":
				authority.Schema = json.RawMessage(`{"const":"foreign workspace"}`)
			}
			body, _ := json.Marshal(authority)
			p.PromptContext.Sections[3].Content = string(body)
			err = validateRuntimeSemanticContextIntegrity(r.SemanticContext, r.SemanticManifest, r.CandidateAttempt, r.SemanticCorrectionObservation, r.SemanticCorrectionDirective, p.ContextBudget, p.PromptContext)
			if err == nil || !strings.Contains(err.Error(), "P0.semantic_output_schema") {
				t.Fatalf("schema corruption accepted: %v", err)
			}
		})
	}
	if samePromptSchemaJSON([]byte(`{"x":9007199254740993}`), []byte(`{"x":9007199254740992}`)) || samePromptSchemaJSON([]byte(`{} {}`), []byte(`{}`)) {
		t.Fatal("numbers or trailing schema data ignored")
	}
}

func TestSemanticAcquisitionInstructionsFollowExactSelectedObligations(t *testing.T) {
	manifest := rc.RuntimeSemanticPlanManifest{ProofObligations: []rc.RuntimeSemanticProofObligation{
		{CoverageWorkItemID: "ordinary"},
		{CoverageWorkItemID: "acquire", OwnerFactAcquisition: &rc.RuntimeSemanticOwnerFactAcquisition{}},
	}}
	for _, tc := range []struct {
		step rc.RuntimeSemanticPlanStep
		ids  []common.CoverageWorkItemID
		want bool
	}{
		{rc.RuntimeSemanticPlanStepCoverageOrder, nil, false},
		{rc.RuntimeSemanticPlanStepProofBatch, []common.CoverageWorkItemID{"ordinary"}, false},
		{rc.RuntimeSemanticPlanStepProofBatch, []common.CoverageWorkItemID{"acquire"}, true},
		{rc.RuntimeSemanticPlanStepProofBatch, []common.CoverageWorkItemID{"ordinary", "acquire"}, true},
		{rc.RuntimeSemanticPlanStepProofBatch, []common.CoverageWorkItemID{"foreign"}, false},
	} {
		c := rc.RuntimeSemanticPlanRequestContext{PlanStep: tc.step, BatchCoverageWorkItemIDs: tc.ids}
		got := runtimeSemanticAcquisitionInstruction(c, manifest)
		if (got == semanticAcquisitionInstruction) != tc.want || (!tc.want && got != "") {
			t.Fatalf("wrong selected instruction for %v: %q", tc, got)
		}
	}
}

// Test callers may mutate typed sources independently of the renderer. Compile
// their expected schema here; production consumes its already-prepared schema
// and does not repeat full manifest/catalog admission after rendering.
func validateRuntimeSemanticContextIntegrity(context rc.RuntimeSemanticPlanRequestContext, manifest rc.RuntimeSemanticPlanManifest, attempt int, previous *rc.RuntimeSemanticCorrectionObservation, directive *rc.RuntimeSemanticCorrectionDirective, budget *models.ProviderContextBudget, prompt *models.PromptContext) error {
	schema, err := rc.RuntimeSemanticPlanJSONSchemaForRequest(manifest, context.PlanStep, context.BatchID, context.BatchSequence, context.BatchCoverageWorkItemIDs)
	if err != nil {
		return err
	}
	return validateRuntimeSemanticRenderedIntegrity(schema, context, manifest, attempt, previous, directive, budget, prompt)
}

func TestPromptSchemaCompactionNeverRepairsMalformedInput(t *testing.T) {
	for _, raw := range []string{`{"type":"object"} {}`, `{"type":`, `[] trailing`} {
		if !bytes.Equal(compactSemanticPlanPromptSchema([]byte(raw)), []byte(raw)) {
			t.Fatal("invalid schema repaired")
		}
		if _, err := compactRuntimePromptSchema([]byte(raw)); err == nil {
			t.Fatal("invalid schema silently accepted")
		}
	}
}
