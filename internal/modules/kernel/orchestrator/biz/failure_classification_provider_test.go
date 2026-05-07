package orchestrator

import (
	"errors"
	"testing"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestClassifyRuntimeFailureProviderContextTooLarge(t *testing.T) {
	step := &runModel.Step{
		ID:    common.StepID("run-x/step-1"),
		Kind:  "llm/workspace_execution",
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage": "llm_provider_error",
			"error": "maximum context length exceeded for this model",
		},
	}
	classification := classifyRuntimeFailure(step, step.Output, "runtime step failed", errors.New("maximum context length exceeded for this model"))
	if classification.Class != failureClassContextTooLarge {
		t.Fatalf("classification = %+v, want %s", classification, failureClassContextTooLarge)
	}
}

func TestClassifyRuntimeFailureProviderRateLimited(t *testing.T) {
	step := &runModel.Step{
		ID:    common.StepID("run-x/step-1"),
		Kind:  "llm/workspace_execution",
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage": "llm_provider_error",
			"error": "http status 429 too many requests",
		},
	}
	classification := classifyRuntimeFailure(step, step.Output, "runtime step failed", errors.New("http status 429 too many requests"))
	if classification.Class != failureClassRateLimited {
		t.Fatalf("classification = %+v, want %s", classification, failureClassRateLimited)
	}
}

func TestClassifyRuntimeFailureProviderAuthOrConfig(t *testing.T) {
	step := &runModel.Step{
		ID:    common.StepID("run-x/step-1"),
		Kind:  "llm/workspace_execution",
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage": "llm_provider_auth_or_config_error",
			"error": "unauthorized api key",
		},
	}
	classification := classifyRuntimeFailure(step, step.Output, "runtime step failed", errors.New("unauthorized api key"))
	if classification.Class != failureClassAuthOrConfigError {
		t.Fatalf("classification = %+v, want %s", classification, failureClassAuthOrConfigError)
	}
}
