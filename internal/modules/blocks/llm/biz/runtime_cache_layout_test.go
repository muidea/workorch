package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func cacheJSONValue(t testing.TB, raw []byte) any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func assertRuntimeCacheLayoutOperations(t *testing.T, input llmmodels.RuntimeCandidateRequest) {
	t.Helper()
	old, err := runtimeProviderRequest(input)
	if err != nil {
		t.Fatal(err)
	}
	expected := input.ExpectedDocument.Clone()
	expected.OperationsContextEncoding = rc.RuntimeOperationsContextEncoding
	input.ExpectedDocument = &expected
	modern, err := runtimeProviderRequest(input)
	if err != nil {
		t.Fatal(err)
	}
	if modern.Schema != old.Schema {
		t.Fatal("v4 changed strict transport grammar")
	}
	before := cacheJSONValue(t, []byte(old.PromptContext.Sections[3].Content)).(map[string]any)
	after := cacheJSONValue(t, []byte(modern.PromptContext.Sections[3].Content)).(map[string]any)
	decision := after["decision"].(map[string]any)
	if decision["FocusRef"] != "context.focus" {
		t.Fatal("duplicate focus was not explicitly referenced")
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("v4 lost or changed authority, constraint, source body or binding")
	}
	if !strings.HasPrefix(modern.PromptContext.Sections[3].Content, `{"context":`) {
		t.Fatal("volatile decision precedes reusable context")
	}
	for _, i := range []int{0, 1, 2, 4, 5} {
		if modern.PromptContext.Sections[i].Content != old.PromptContext.Sections[i].Content {
			t.Fatalf("v4 changed C%d", i)
		}
	}
	for _, strict := range []bool{false, true} {
		modern.Config.Capabilities.JSONSchemaResponse = strict
		old.Config.Capabilities.JSONSchemaResponse = strict
		modern.RawTextOutput = !strict
		old.RawTextOutput = !strict
		if responsesPromptCacheKey(modern) != responsesPromptCacheKey(old) || responsesCacheSchemaScope(modern) != "runtime/operations-schema" {
			t.Fatal("C3 layout fragmented stable routing family")
		}
		if !reflect.DeepEqual(responsesTextForRequest(modern.Config.Capabilities, modern), responsesTextForRequest(old.Config.Capabilities, old)) {
			t.Fatal("new layout changed native transport schema")
		}
		first := responsesWireInputForRequest(&modern)
		second := responsesWireInputForRequest(&modern)
		if !bytes.Equal(first, second) {
			t.Fatal("snapshot recovery changed provider wire")
		}
	}
	broken := modern
	prompt := *modern.PromptContext
	prompt.Sections = append([]llmmodels.PromptSection(nil), prompt.Sections...)
	broken.PromptContext = &prompt
	prompt.Sections[3].Content = strings.Replace(prompt.Sections[3].Content, `"FocusRef":"context.focus"`, `"FocusRef":"other.focus"`, 1)
	budget := *modern.ContextBudget
	if validateRuntimeCandidateContextIntegrity(input.DefinitionContext, input.CandidateContext, 0, nil, &budget, &prompt) == nil {
		t.Fatal("foreign focus reference passed current-context guard")
	}
	limited := input
	ceiling := *input.CandidateContext.Resources.ProviderCeiling
	ceiling.MaxBytes = 1
	limited.CandidateContext.Resources.ProviderCeiling = &ceiling
	generation, err := (&Biz{}).GenerateCandidate(context.Background(), limited)
	if err == nil || !strings.Contains(err.Error(), "exceeds provider ceiling") || generation.Audit == nil || generation.Audit.ProviderSubmitted {
		t.Fatal("v4 bypassed operations ceiling before provider submission", err)
	}
}

func TestCacheLayoutKeepsExactScalarsArrayOrderAndSourceBodies(t *testing.T) {
	raw := []byte(`{"schema":{"properties":{"manifestHash":{"const":"old"},"value":{"type":"string"}}},"manifest":{"hash":"change"},"outputInstruction":"original","planStep":"proof_batch","definitionSeed":{"hash":"stable"},"runId":"r","workspaceId":"w","soul":{"text":"a"},"extra":{"number":9007199254740993,"empty":[],"absent":null,"ordered":["z","a"],"source":"{\"manifestHash\":\"untrusted\"}\n<&>"}}`)
	out, err := cacheLayoutJSON(raw, "semantic")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cacheJSONValue(t, raw), cacheJSONValue(t, out)) {
		t.Fatal("layout changed JSON values")
	}
	if len(out) != len(raw) || !bytes.Contains(out, []byte("9007199254740993")) {
		t.Fatal("layout re-encoded scalar or changed size")
	}
	again, err := cacheLayoutJSON(out, "semantic")
	if err != nil || !bytes.Equal(again, out) {
		t.Fatal("layout is not deterministic/idempotent")
	}
	changed := bytes.Replace(raw, []byte(`"const":"old"`), []byte(`"const":"new"`), 1)
	next, err := cacheLayoutJSON(changed, "semantic")
	if err != nil {
		t.Fatal(err)
	}
	if sharedCacheBytes(out, next) <= sharedCacheBytes(raw, changed) {
		t.Fatal("current manifest const still splits reusable schema first")
	}
	for _, bad := range []string{`{"x":1,"x":2}`, `{"x":[{"a":1,"a":2}]}`, `{} {}`, `{"x":`} {
		if _, err := cacheLayoutJSON([]byte(bad), "semantic"); err == nil {
			t.Fatal("ambiguous compiler JSON accepted")
		}
	}
}

func sharedCacheBytes(a, b []byte) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func TestCacheLayoutSemanticPreservesContractsAndRejectsUnknownVersion(t *testing.T) {
	input := runtimeSemanticRequestWithExpectedDocument(t)
	original, err := runtimeSemanticPlanProviderRequest(input)
	if err != nil {
		t.Fatal(err)
	}
	expected := input.ExpectedDocument.Clone()
	expected.SemanticContextEncoding = rc.RuntimeSemanticContextEncoding
	expected.Hash = rc.StableRuntimeExpectedDocumentHash(expected)
	input.ExpectedDocument = &expected
	modern, err := runtimeSemanticPlanProviderRequest(input)
	if err != nil {
		t.Fatal(err)
	}
	if modern.Schema != original.Schema {
		t.Fatal("semantic strict transport schema changed")
	}
	for i := range original.PromptContext.Sections {
		a, b := original.PromptContext.Sections[i], modern.PromptContext.Sections[i]
		if a.Role != b.Role || a.ID != b.ID {
			t.Fatal("message role/section changed")
		}
		if i == 3 {
			if !reflect.DeepEqual(cacheJSONValue(t, []byte(a.Content)), cacheJSONValue(t, []byte(b.Content))) {
				t.Fatal("semantic current context changed")
			}
		} else if a.Content != b.Content {
			t.Fatalf("semantic C%d changed", i)
		}
	}
	if responsesPromptCacheKey(modern) != responsesPromptCacheKey(original) {
		t.Fatal("C3 layout changed prefix key")
	}
	input.ExpectedDocument.SemanticContextEncoding = "unknown"
	if _, err := runtimeSemanticPlanProviderRequest(input); err == nil {
		t.Fatal("unsupported semantic context encoding accepted")
	}
}

func TestCacheLayoutParallelWorkspaceProfilesKeepDistinctContexts(t *testing.T) {
	for _, scenario := range []struct{ workspace, goal string }{{"customer-a", "compute a schedule"}, {"customer-b", "analyze an archive"}, {"customer-c", "plan a landscape"}, {"customer-d", "draft a syllabus"}} {
		t.Run(scenario.workspace, func(t *testing.T) {
			t.Parallel()
			input := runtimeSemanticProviderTestRequest(t)
			c := &input.SemanticContext
			c.WorkspaceID, c.RunID = common.WorkspaceID(scenario.workspace), common.RunID("run-"+scenario.workspace)
			c.Definition.Revision = "definition-" + scenario.workspace
			c.Definition.GoalDefinition = scenario.goal
			c.Activation.ExecutionContract.ProfileContracts = []rc.RuntimeProfileContract{{ID: "profile-" + scenario.workspace, AcceptanceDimensions: []rc.RuntimeAcceptanceDimension{{ID: "criterion", Description: scenario.goal}}}}
			seed, err := rc.CompileRuntimeDefinitionSeed(c.Definition, c.Activation, c.DefinitionSeed.CoverageItems, c.DefinitionSeed.Slices)
			if err != nil {
				t.Fatal(err)
			}
			c.DefinitionSeed = seed
			input.SemanticManifest.WorkspaceID, input.SemanticManifest.RunID = c.WorkspaceID, c.RunID
			input.SemanticManifest.DefinitionRevision, input.SemanticManifest.DefinitionSeedHash = c.Definition.Revision, seed.Hash
			input.SemanticManifest.Hash = rc.StableRuntimeSemanticPlanManifestHash(input.SemanticManifest)
			input.StablePrefixHash = rc.StableRuntimeCandidatePrefixHashWithExecutionContract(c.Definition, c.Activation.ExecutionContract)
			refreshSemanticProgressForTest(t, &input)
			input.ExpectedDocument = &rc.RuntimeExpectedDocumentContract{SemanticContextEncoding: rc.RuntimeSemanticContextEncoding, MaxOutputTokens: c.ProviderCeiling.EffectiveMaxOutputTokens}
			original, err := runtimeSemanticPlanProviderRequest(input)
			if err != nil {
				t.Fatal(err)
			}
			input.ExpectedDocument.SemanticContextEncoding = rc.RuntimeSemanticContextEncoding
			r, err := runtimeSemanticPlanProviderRequest(input)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cacheJSONValue(t, []byte(original.PromptContext.Sections[3].Content)), cacheJSONValue(t, []byte(r.PromptContext.Sections[3].Content))) {
				t.Fatal("concurrent renderer changed context")
			}
			if r.WorkspaceID != scenario.workspace || !strings.Contains(r.PromptContext.Sections[1].Content, scenario.goal) || !strings.Contains(r.PromptContext.Sections[2].Content, "profile-"+scenario.workspace) {
				t.Fatal("concurrent renderer lost exact workspace/profile context")
			}
			before := responsesPromptCacheKey(r)
			if responsesPromptCacheKey(r) != responsesPromptCacheKey(original) {
				t.Fatal("current authority polluted stable scope")
			}
			foreign := r
			foreign.WorkspaceID = "foreign"
			if responsesPromptCacheKey(foreign) == before {
				t.Fatal("workspace scopes collapsed")
			}
			altered := r
			copyPrompt := *r.PromptContext
			copyPrompt.Sections = append([]llmmodels.PromptSection(nil), r.PromptContext.Sections...)
			altered.PromptContext = &copyPrompt
			copyPrompt.Sections[1].Content = "different profile and goal"
			if responsesPromptCacheKey(altered) == before {
				t.Fatal("profile/definition content collapsed")
			}
			replay, err := runtimeSemanticPlanProviderRequest(input)
			if err != nil || !bytes.Equal(responsesWireInputForRequest(&r), responsesWireInputForRequest(&replay)) {
				t.Fatal("concurrent recovery changed exact provider input", err)
			}
		})
	}
}

func TestCacheLayoutSemanticRejectsStaleUnknownAndOverBudgetBeforeSubmission(t *testing.T) {
	for _, scenario := range []struct {
		name, message string
		mutate        func(*llmmodels.RuntimeCandidateRequest)
	}{
		{"stale_prefix", "stable prefix mismatch", func(r *llmmodels.RuntimeCandidateRequest) { r.StablePrefixHash = "foreign-prefix" }},
		{"unknown_renderer", "unsupported semantic context encoding", func(r *llmmodels.RuntimeCandidateRequest) { r.ExpectedDocument.SemanticContextEncoding = "unknown" }},
		{"over_budget", "exceeds provider ceiling", func(r *llmmodels.RuntimeCandidateRequest) { r.SemanticContext.ProviderCeiling.MaxBytes = 1 }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			request := runtimeSemanticRequestWithExpectedDocument(t)
			request.ExpectedDocument.SemanticContextEncoding = rc.RuntimeSemanticContextEncoding
			scenario.mutate(&request)
			generation, err := (&Biz{}).GenerateCandidate(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), scenario.message) || generation.Audit == nil || generation.Audit.ProviderSubmitted {
				t.Fatal("v4 failed to reject before physical submission", err)
			}
		})
	}
}
