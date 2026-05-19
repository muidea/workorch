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

func TestClassifyRuntimeFailureStrategyReassessmentContractIsNotAuthOrConfig(t *testing.T) {
	err := errors.New("invalid argument: strategyReassessment.required=true requires state-changing patch output to declare failedSurface/rootCauseTarget/repairTargetDecision/forbiddenRepair/verificationPlan aligned with current failed surface")
	step := &runModel.Step{
		ID:    common.StepID("run-x/step-1"),
		Kind:  "llm/workspace_execution",
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage": "llm_execution_graph_plan",
			"error": err.Error(),
		},
	}
	classification := classifyRuntimeFailure(step, step.Output, "runtime step failed", err)
	if classification.Class != failureClassRuntime {
		t.Fatalf("classification = %+v, want %s", classification, failureClassRuntime)
	}
	if isProviderFailureClass(classification.Class) {
		t.Fatalf("runtime contract failure classified as provider failure: classification=%+v", classification)
	}
}

func TestClassifyRuntimeFailureProviderStreamIncomplete(t *testing.T) {
	step := &runModel.Step{
		ID:    common.StepID("run-x/step-1"),
		Kind:  "llm/workspace_execution",
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage": "llm_provider_stream_incomplete",
			"error": "invalid argument: llm provider stream incomplete: ended before [DONE]",
		},
	}
	classification := classifyRuntimeFailure(step, step.Output, "runtime step failed", errors.New("invalid argument: llm provider stream incomplete: ended before [DONE]"))
	if classification.Class != failureClassProviderStreamIncomplete {
		t.Fatalf("classification = %+v, want %s", classification, failureClassProviderStreamIncomplete)
	}
	if !isLLMProviderFailureStage("llm_provider_stream_incomplete") || !isProviderFailureClass(classification.Class) {
		t.Fatalf("stream incomplete not treated as provider failure: classification=%+v", classification)
	}
}

func TestClassifyRuntimeFailureLLMDecisionContractIsNotProviderSchemaDrift(t *testing.T) {
	step := &runModel.Step{
		ID:    common.StepID("run-x/step-1"),
		Kind:  "llm/workspace_execution",
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage": "llm_schema_invalid",
			"error": "invalid argument: executionGraph patch/apply targetRef is required",
		},
	}
	classification := classifyRuntimeFailure(step, step.Output, "runtime step failed", errors.New("invalid argument: executionGraph patch/apply targetRef is required"))
	if classification.Class != failureClassLLMDecisionInvalid {
		t.Fatalf("classification = %+v, want %s", classification, failureClassLLMDecisionInvalid)
	}
	if isLLMProviderFailureStage("llm_schema_invalid") || isProviderFailureClass(classification.Class) {
		t.Fatalf("llm_schema_invalid classified as provider failure: classification=%+v", classification)
	}
}

func TestClassifyRuntimeFailureProviderSchemaDriftRemainsProviderFailure(t *testing.T) {
	step := &runModel.Step{
		ID:    common.StepID("run-x/step-1"),
		Kind:  "llm/workspace_execution",
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage": "llm_provider_schema_drift",
			"error": "provider response schema changed",
		},
	}
	classification := classifyRuntimeFailure(step, step.Output, "runtime step failed", errors.New("provider response schema changed"))
	if classification.Class != failureClassProviderSchemaDrift {
		t.Fatalf("classification = %+v, want %s", classification, failureClassProviderSchemaDrift)
	}
	if !isLLMProviderFailureStage("llm_provider_schema_drift") || !isProviderFailureClass(classification.Class) {
		t.Fatalf("provider schema drift not treated as provider failure: classification=%+v", classification)
	}
}

func TestClassifyRuntimeFailureDoDCoverageMissingHasPriorityOverAuthKeywords(t *testing.T) {
	step := &runModel.Step{
		ID:    common.StepID("run-x/step-1"),
		Kind:  "llm/workspace_execution",
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage": "llm_dod_assessment",
			"error": "invalid argument: dodAssessment does not cover all current DoD items; missing: API Key auth behavior",
		},
	}
	classification := classifyRuntimeFailure(step, step.Output, "runtime completion gate failed", errors.New("invalid argument: dodAssessment does not cover all current DoD items; missing: API Key auth behavior"))
	if classification.Class != failureClassDoDCoverageMissing {
		t.Fatalf("classification = %+v, want %s", classification, failureClassDoDCoverageMissing)
	}
}
