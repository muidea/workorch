package llm

import (
	"context"
	"errors"
	"testing"

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
