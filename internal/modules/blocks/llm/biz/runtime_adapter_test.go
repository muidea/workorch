package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type staticProvider struct {
	payload []byte
	err     error
	request llmmodels.ProviderRequest
}

func (p *staticProvider) Complete(_ context.Context, request llmmodels.ProviderRequest) ([]byte, error) {
	p.request = request
	return p.payload, p.err
}

type sequenceProvider struct {
	payloads [][]byte
	requests []llmmodels.ProviderRequest
}

func (p *sequenceProvider) Complete(_ context.Context, request llmmodels.ProviderRequest) ([]byte, error) {
	p.requests = append(p.requests, request)
	if len(p.payloads) == 0 {
		return nil, common.ErrInvalidArgument
	}
	payload := p.payloads[0]
	p.payloads = p.payloads[1:]
	return payload, nil
}

func TestRuntimeAdapterFailsClearlyWhenProviderUnavailable(t *testing.T) {
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Unavailable:       true,
		UnavailableReason: "llm provider is not configured",
	}, nil)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("execute err = %v, want invalid argument", err)
	}
	if result.Output["status"] != "failed" || result.Output["stage"] != "llm_provider_unavailable" {
		t.Fatalf("output = %+v, want provider unavailable failure", result.Output)
	}
}

func TestRuntimeAdapterRepairsInvalidDecisionOnce(t *testing.T) {
	provider := &sequenceProvider{payloads: [][]byte{
		[]byte(`{
			"workspaceId":"ws-1",
			"thoughtSummary":"need product direction",
			"completionState":"blocked_on_decision",
			"requiredDecision":"missing implementation path"
		}`),
		[]byte(`{
			"thoughtSummary":"need product direction",
			"completionState":"blocked_on_decision",
			"requiredDecision":{
				"reason":"missing implementation path",
				"decisionKind":"provide_missing_info",
				"question":"Should the run use the shell sandbox first?"
			}
		}`),
	}}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if len(provider.requests) != 2 || provider.requests[1].Repair == nil {
		t.Fatalf("requests = %+v, want one repair retry", provider.requests)
	}
	if provider.requests[1].Repair.Attempt != 1 ||
		provider.requests[1].Repair.ParseError == "" ||
		provider.requests[1].Repair.OriginalOutput == "" {
		t.Fatalf("repair request = %+v, want parse context", provider.requests[1].Repair)
	}
	if result.Output["stage"] != "llm_decision_required" ||
		result.Output["blockDecisionKind"] != "provide_missing_info" {
		t.Fatalf("output = %+v, want repaired decision", result.Output)
	}
	repair, ok := result.Output["llmRepair"].(map[string]any)
	if !ok || repair["attempted"] != true || repair["parseError"] == "" {
		t.Fatalf("llmRepair = %+v, want successful repair metadata", result.Output["llmRepair"])
	}
}

func TestRuntimeAdapterReportsRepairFailure(t *testing.T) {
	provider := &sequenceProvider{payloads: [][]byte{
		[]byte(`{"workspaceId":"ws-1","completionState":"continue"}`),
		[]byte(`{"completionState":"unknown"}`),
	}}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("execute err = %v, want invalid argument", err)
	}
	if result.Output["stage"] != "llm_schema_invalid" ||
		result.Output["repairAttempted"] != true ||
		result.Output["parseError"] == "" ||
		result.Output["repairError"] == "" ||
		result.Output["rawContentPreview"] == "" ||
		result.Output["repairRawContentPreview"] == "" {
		t.Fatalf("output = %+v, want repair failure diagnostics", result.Output)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("provider calls = %d, want original plus one repair", len(provider.requests))
	}
}

func TestRuntimeAdapterParsesDecisionAndReturnsNormalizedOutput(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"thoughtSummary":"need to validate with go test",
		"completionState":"continue",
		"shellCommands":[{"command":"go","args":["test","./..."]}]
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if provider.request.Schema == "" || provider.request.Config.Ready != true {
		t.Fatalf("provider request = %+v, want schema and ready redacted config", provider.request)
	}
	if result.Output["status"] != "completed" ||
		result.Output["stage"] != "llm_shell_plan" ||
		result.Output["nextKind"] != "shell/command" {
		t.Fatalf("output = %+v, want shell plan", result.Output)
	}
}

func TestRuntimeAdapterSelectsRequestedProvider(t *testing.T) {
	localProvider := &staticProvider{payload: []byte(`{"completionState":"continue"}`)}
	reasoningProvider := &staticProvider{payload: []byte(`{
		"completionState":"continue",
		"shellCommands":[{"command":"go","args":["test","./..."]}]
	}`)}
	adapter := NewRuntimeAdapterWithProviders("local", map[string]ProviderRuntime{
		"local": {
			Config: llmmodels.ProviderConfig{
				ID:       "local",
				Provider: llmcommon.ProviderCompatible,
				APIKey:   "local-secret",
				Model:    "local-model",
			},
			Provider: localProvider,
		},
		"reasoning": {
			Config: llmmodels.ProviderConfig{
				ID:       "reasoning",
				Provider: llmcommon.ProviderOpenAI,
				APIKey:   "reasoning-secret",
				Model:    "reasoning-model",
			},
			Provider: reasoningProvider,
		},
	})
	request := llmStepRequest()
	request.Input["executionContext"] = map[string]any{
		llmcommon.ExecutionContextLLMProviderID: "reasoning",
	}

	result, err := adapter.ExecuteRuntimeStep(context.Background(), request)
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if localProvider.request.Model != "" {
		t.Fatalf("local provider was called: %+v", localProvider.request)
	}
	if reasoningProvider.request.Model != "reasoning-model" || result.Output["nextKind"] != "shell/command" {
		t.Fatalf("reasoning request=%+v output=%+v, want selected reasoning provider", reasoningProvider.request, result.Output)
	}
}

func TestRuntimeAdapterFailsUnknownRequestedProvider(t *testing.T) {
	adapter := NewRuntimeAdapterWithProviders("local", map[string]ProviderRuntime{
		"local": {
			Config: llmmodels.ProviderConfig{
				ID:       "local",
				Provider: llmcommon.ProviderCompatible,
				APIKey:   "local-secret",
				Model:    "local-model",
			},
			Provider: &staticProvider{payload: []byte(`{"completionState":"continue"}`)},
		},
	})
	request := llmStepRequest()
	request.Input[llmcommon.ExecutionContextLLMProviderID] = "missing"

	result, err := adapter.ExecuteRuntimeStep(context.Background(), request)
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("execute err = %v, want invalid argument", err)
	}
	provider, ok := result.Output["provider"].(llmmodels.RedactedProviderConfig)
	if result.Output["stage"] != "llm_provider_unavailable" || !ok || provider.ID != "missing" {
		t.Fatalf("output = %+v, want missing provider failure", result.Output)
	}
}

func TestRuntimeAdapterLoadsProviderConfigAtExecutionTime(t *testing.T) {
	firstProviderCalls := 0
	firstProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstProviderCalls++
		writeChatCompletion(t, w, `{"completionState":"continue","shellCommands":[{"command":"echo","args":["first"]}]}`)
	}))
	defer firstProvider.Close()
	secondProviderCalls := 0
	secondProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondProviderCalls++
		writeChatCompletion(t, w, `{"completionState":"continue","shellCommands":[{"command":"echo","args":["second"]}]}`)
	}))
	defer secondProvider.Close()

	baseURL := firstProvider.URL + "/v1"
	adapter := NewRuntimeAdapterFromConfigLoader(func() (string, map[string]llmmodels.ProviderConfig, error) {
		return "local", map[string]llmmodels.ProviderConfig{
			"local": {
				ID:       "local",
				Provider: llmcommon.ProviderCompatible,
				BaseURL:  baseURL,
				APIKey:   "secret-value",
				Model:    "test-model",
				Timeout:  time.Second,
			},
		}, nil
	})

	firstResult, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("first execute runtime step: %v", err)
	}
	baseURL = secondProvider.URL + "/v1"
	secondResult, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("second execute runtime step: %v", err)
	}
	if firstProviderCalls != 1 || secondProviderCalls != 1 {
		t.Fatalf("provider calls first=%d second=%d, want one call each", firstProviderCalls, secondProviderCalls)
	}
	if firstResult.Output["nextInput"].(map[string]any)["args"].([]string)[0] != "first" ||
		secondResult.Output["nextInput"].(map[string]any)["args"].([]string)[0] != "second" {
		t.Fatalf("outputs first=%+v second=%+v, want runtime-loaded provider results", firstResult.Output, secondResult.Output)
	}
}

func writeChatCompletion(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	payload := map[string]any{
		"choices": []map[string]any{{
			"message": map[string]string{"content": content},
		}},
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("write chat completion: %v", err)
	}
}

func TestRuntimeAdapterNormalizesRequiredDecisionFields(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"thoughtSummary":"need product direction",
		"completionState":"blocked_on_decision",
		"requiredDecision":{
			"reason":"missing implementation path",
			"decisionKind":"provide_missing_info",
			"question":"Should the run use the shell sandbox first?",
			"context":{"topic":"execution"}
		}
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	if result.Output["stage"] != "llm_decision_required" ||
		result.Output["blockDecisionReason"] != "missing implementation path" ||
		result.Output["blockDecisionKind"] != "provide_missing_info" ||
		result.Output["blockDecisionQuestion"] != "Should the run use the shell sandbox first?" {
		t.Fatalf("output = %+v, want structured decision block", result.Output)
	}
	contextMap, ok := result.Output["blockDecisionContext"].(map[string]any)
	if !ok || contextMap["topic"] != "execution" {
		t.Fatalf("blockDecisionContext = %+v, want topic execution", result.Output["blockDecisionContext"])
	}
}

func TestRuntimeAdapterExposesExecutionGraph(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"thoughtSummary":"plan shell inspection and validation",
		"completionState":"continue",
		"executionGraph":{
			"nodes":[
				{"id":"inspect","kind":"shell/command","input":{"command":"ls","args":["-la"]}},
				{"id":"test","kind":"shell/command","dependsOn":["inspect"],"input":{"command":"go","args":["test","./..."]}}
			]
		}
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	graph, ok := result.Output["executionGraph"].(map[string]any)
	if result.Output["stage"] != "llm_execution_graph_plan" || !ok {
		t.Fatalf("output = %+v, want execution graph plan", result.Output)
	}
	nodes, ok := graph["nodes"].([]map[string]any)
	if !ok || len(nodes) != 2 || nodes[1]["kind"] != "shell/command" {
		t.Fatalf("nodes = %+v, want shell command graph nodes", graph["nodes"])
	}
}

func TestRuntimeAdapterReturnsErrorForFailedDecision(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"thoughtSummary":"DoD cannot be satisfied with current context",
		"completionState":"failed"
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("execute err = %v, want invalid argument", err)
	}
	if result.Output["status"] != "failed" ||
		result.Output["stage"] != "llm_failed" ||
		result.Output["error"] != "DoD cannot be satisfied with current context" {
		t.Fatalf("output = %+v, want failed llm decision", result.Output)
	}
}

func TestRuntimeAdapterExposesCompletedDoDAssessment(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{
		"thoughtSummary":"DoD satisfied by validation",
		"completionState":"completed",
		"dodAssessment":{
			"satisfied":true,
			"summary":"all checks passed",
			"items":[{"item":"tests pass","state":"satisfied","evidenceRefs":["run-1/validation/runtime-step"]}]
		}
	}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if err != nil {
		t.Fatalf("execute runtime step: %v", err)
	}
	assessment, ok := result.Output["dodAssessment"].(map[string]any)
	if !ok || assessment["satisfied"] != true || assessment["summary"] != "all checks passed" {
		t.Fatalf("dodAssessment = %+v, want satisfied assessment", result.Output["dodAssessment"])
	}
	items, ok := assessment["items"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("dodAssessment items = %+v, want one item", assessment["items"])
	}
}

func TestParseLLMExecutionDecisionRejectsSatisfiedDoDWithoutEvidence(t *testing.T) {
	_, err := ParseLLMExecutionDecision([]byte(`{
		"completionState":"completed",
		"dodAssessment":{"satisfied":true,"items":[{"item":"done","state":"satisfied"}]}
	}`))
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("parse err = %v, want invalid argument", err)
	}
}

func TestParseLLMExecutionDecisionAcceptsMarkdownFencedJSONObject(t *testing.T) {
	decision, err := ParseLLMExecutionDecision([]byte("```json\n{\"completionState\":\"continue\"}\n```"))
	if err != nil {
		t.Fatalf("parse decision: %v", err)
	}
	if decision.CompletionState != "continue" {
		t.Fatalf("completionState = %q, want continue", decision.CompletionState)
	}
}

func TestParseLLMExecutionDecisionRejectsUnknownFieldAfterNormalization(t *testing.T) {
	_, err := ParseLLMExecutionDecision([]byte("Here is the json:\n{\"workspaceId\":\"ws-1\",\"completionState\":\"continue\"}"))
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("parse err = %v, want invalid argument", err)
	}
}

func TestParseLLMExecutionDecisionRejectsInvalidExecutionGraph(t *testing.T) {
	_, err := ParseLLMExecutionDecision([]byte(`{
		"completionState":"continue",
		"executionGraph":{"nodes":[
			{"id":"a","kind":"shell/command","dependsOn":["b"]},
			{"id":"b","kind":"shell/command","dependsOn":["a"]}
		]}
	}`))
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("parse err = %v, want invalid argument", err)
	}
}

func TestParseLLMExecutionDecisionRejectsMemoryWriteWithoutSummary(t *testing.T) {
	_, err := ParseLLMExecutionDecision([]byte(`{
		"completionState":"continue",
		"memoryWrites":[{"memoryType":"workspace"}]
	}`))
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("parse err = %v, want invalid argument", err)
	}
}

func TestRuntimeAdapterFailsOnSchemaInvalidOutput(t *testing.T) {
	provider := &staticProvider{payload: []byte(`{"completionState":"unknown"}`)}
	adapter := NewRuntimeAdapter(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		APIKey:   "secret-value",
		Model:    "test-model",
	}, provider)

	result, err := adapter.ExecuteRuntimeStep(context.Background(), llmStepRequest())
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("execute err = %v, want invalid argument", err)
	}
	if result.Output["status"] != "failed" || result.Output["stage"] != "llm_schema_invalid" {
		t.Fatalf("output = %+v, want schema invalid failure", result.Output)
	}
}

func llmStepRequest() runtimemodel.StepExecutionRequest {
	return runtimemodel.StepExecutionRequest{
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/llm-1"),
		Kind:        llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			"goalId": "goal-1",
			"doDId":  "dod-1",
		},
	}
}
