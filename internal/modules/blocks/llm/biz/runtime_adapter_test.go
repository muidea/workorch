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

func TestParseLLMExecutionDecisionRejectsSatisfiedDoDWithoutEvidence(t *testing.T) {
	_, err := ParseLLMExecutionDecision([]byte(`{
		"completionState":"completed",
		"dodAssessment":{"satisfied":true,"items":[{"item":"done","state":"satisfied"}]}
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
