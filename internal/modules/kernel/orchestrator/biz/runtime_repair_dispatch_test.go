package orchestrator

import (
	"context"
	"errors"
	"testing"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

type repairDispatchSpecBuilderStub struct{}

func (repairDispatchSpecBuilderStub) runtimeRepairFollowUpStepInput(_ context.Context, _ *runmodel.Run, _ *runmodel.Step, attempt int, execErr error) map[string]any {
	return map[string]any{
		stepInputKeyRuntimeRepairFollowUp: true,
		stepInputKeyRuntimeRepairAttempt:  attempt,
		"failureReason":                   errorString(execErr),
	}
}

type repairBlockedRecoveryExecutorStub struct {
	called runtimeRecoveryAction
	err    error
}

func (s *repairBlockedRecoveryExecutorStub) blockOnProviderRetryExhaustion(context.Context, *runmodel.Run, *runmodel.Step, error) error {
	s.called = runtimeRecoveryActionProviderRetryExhausted
	return s.err
}

func (s *repairBlockedRecoveryExecutorStub) blockOnRuntimeRepairExhaustion(context.Context, *runmodel.Run, *runmodel.Step, error) error {
	s.called = runtimeRecoveryActionRuntimeRepairExhausted
	return s.err
}

func (s *repairBlockedRecoveryExecutorStub) blockOnDuplicateRuntimeFailure(context.Context, *runmodel.Run, *runmodel.Step, error) error {
	s.called = runtimeRecoveryActionDuplicateFailure
	return s.err
}

func (s *repairBlockedRecoveryExecutorStub) blockOnContextIntegrityGate(context.Context, *runmodel.Run, *runmodel.Step, error) error {
	s.called = runtimeRecoveryActionContextIntegrityBlocked
	return s.err
}

func TestRepairSubsystemRecoveryDispatchSpecPlansProviderRetry(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-repair-dispatch-provider")
	rootStepID := common.StepID(runID + "/planning-001")
	stepRepo := memory.NewStepRepository()
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    providerRetryStepID(runID, rootStepID, 1),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryRootStep: string(rootStepID),
		},
		Output: map[string]any{"stage": "llm_provider_timeout"},
	}); err != nil {
		t.Fatalf("seed provider retry: %v", err)
	}
	failedStep := &runmodel.Step{
		ID:    rootStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}

	spec, ok, err := newRepairSubsystem(stepRepo, nil).recoveryDispatchSpec(ctx, &runmodel.Run{ID: runID}, failedStep, errors.New("timeout"), runtimeRecoveryActionProviderRetry, maxProviderRetryAttempts, repairDispatchSpecBuilderStub{}, false)

	if err != nil || !ok {
		t.Fatalf("recoveryDispatchSpec ok/err = %v/%v, want provider retry spec", ok, err)
	}
	if spec.ID != providerRetryStepID(runID, rootStepID, 2) || spec.Kind != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("spec = %+v, want provider retry attempt 2", spec)
	}
	if spec.Input[stepInputKeyProviderRetryAttempt] != 2 {
		t.Fatalf("provider retry attempt = %#v, want 2", spec.Input[stepInputKeyProviderRetryAttempt])
	}
}

func TestRepairSubsystemRecoveryDispatchSpecPlansRuntimeRepairWithExactScope(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-repair-dispatch-runtime")
	stepRepo := memory.NewStepRepository()
	failedStep := &runmodel.Step{
		ID:    common.StepID(runID + "/verify"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime|target=exact",
		},
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    runtimeRepairFollowUpStepID(runID, failedStep, 1),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfFailureClass:  failureClassRuntime,
			stepInputKeyRepairOfFailureSig:    "class=runtime|target=exact",
		},
		Output: map[string]any{"status": "failed"},
	}); err != nil {
		t.Fatalf("seed runtime repair: %v", err)
	}

	spec, ok, err := newRepairSubsystem(stepRepo, nil).recoveryDispatchSpec(ctx, &runmodel.Run{ID: runID, InitialStepKind: llmcommon.StepKindWorkspaceExecution}, failedStep, errors.New("failed"), runtimeRecoveryActionRuntimeRepair, maxProviderRetryAttempts, repairDispatchSpecBuilderStub{}, false)

	if err != nil || !ok {
		t.Fatalf("recoveryDispatchSpec ok/err = %v/%v, want runtime repair spec", ok, err)
	}
	if spec.ID != runtimeRepairFollowUpStepID(runID, failedStep, 2) || spec.Kind != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("spec = %+v, want runtime repair attempt 2", spec)
	}
	if spec.Input[stepInputKeyRuntimeRepairAttempt] != 2 {
		t.Fatalf("runtime repair attempt = %#v, want 2", spec.Input[stepInputKeyRuntimeRepairAttempt])
	}
}

func TestRepairSubsystemBlockRecoveryActionPersistsBlockerBeforeErrBlocked(t *testing.T) {
	executor := &repairBlockedRecoveryExecutorStub{}
	err := newRepairSubsystem(nil, nil).blockRecoveryActionFromFailedStep(
		context.Background(),
		&runmodel.Run{ID: "run-repair-block"},
		&runmodel.Step{ID: "run-repair-block/failed", State: common.StepStateFailed},
		errors.New("failed"),
		runtimeRecoveryActionDuplicateFailure,
		executor,
	)

	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("blockRecoveryActionFromFailedStep error = %v, want ErrBlocked", err)
	}
	if executor.called != runtimeRecoveryActionDuplicateFailure {
		t.Fatalf("executor side effect = %s, want duplicate failure persisted before ErrBlocked", executor.called)
	}
}

func TestRepairSubsystemFailRunReasonFromFailedStep(t *testing.T) {
	manager := newRepairSubsystem(nil, nil)
	tests := []struct {
		name       string
		failedStep *runmodel.Step
		execErr    error
		want       string
	}{
		{
			name:       "exec error takes precedence",
			failedStep: &runmodel.Step{Output: map[string]any{"error": "output error"}},
			execErr:    errors.New("exec failed"),
			want:       "exec failed",
		},
		{
			name:       "output error fallback",
			failedStep: &runmodel.Step{Output: map[string]any{"error": " output error "}},
			want:       "output error",
		},
		{
			name:       "output reason fallback",
			failedStep: &runmodel.Step{Output: map[string]any{"reason": " output reason "}},
			want:       "output reason",
		},
		{
			name: "default reason",
			want: "runtime step failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := manager.failRunReasonFromFailedStep(tt.failedStep, tt.execErr); got != tt.want {
				t.Fatalf("failRunReasonFromFailedStep() = %q, want %q", got, tt.want)
			}
		})
	}
}
