package orchestrator

import (
	"context"
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestMaxRuntimeRepairAttemptsUsesConfiguredPolicy(t *testing.T) {
	orch := &RuntimeOrchestrator{
		policy: NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 10}),
	}
	if got := maxRuntimeRepairAttemptsForPolicy(orch.policy); got != 10 {
		t.Fatalf("max runtime repair attempts = %d, want 10", got)
	}
}

func TestShouldNotBlockOnDuplicateRuntimeFailureBeforeScopedBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-duplicate-shell")
	rootStepID := common.StepID(string(runID) + "/step-1")
	signature := "class=shell_command_failed|kind=shell/command|failureTargets=main.go|exitCode=1"
	repo := memory.NewStepRepository()
	prevFailed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/failed-prev"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: signature,
			"stage":                   "shell_command_failed",
		},
	}
	repairAttempt := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
			stepInputKeyRepairOfFailureSig:    signature,
			stepInputKeyRepairOfRootStepID:    string(rootStepID),
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: signature,
			"stage":                   "shell_command_failed",
		},
	}
	currentFailed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/failed-current"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRepairOfRootStepID: string(rootStepID),
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: signature,
			"stage":                   "shell_command_failed",
		},
	}
	for _, step := range []*runmodel.Step{prevFailed, repairAttempt, currentFailed} {
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}
	orch := &RuntimeOrchestrator{
		steps:   repo,
		tickets: memory.NewDecisionTicketRepository(),
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 10}),
	}
	run := &runmodel.Run{ID: runID, DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	if orch.shouldBlockOnDuplicateRuntimeFailure(ctx, run, currentFailed) {
		t.Fatalf("expected duplicate shell-command failure to stay repairable before scoped budget")
	}
}

func TestShouldBlockOnDuplicateRuntimeFailureForRepeatedLLMDecisionInvalid(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-duplicate-llm-schema")
	signature := "class=llm_decision_invalid|kind=llm/workspace_execution|stage=llm_schema_invalid|error=invalid llm decision json"
	repo := memory.NewStepRepository()
	prevFailed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/failed-prev"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: signature,
			"stage":                   "llm_schema_invalid",
		},
	}
	repairAttempt := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfFailureClass:  failureClassLLMDecisionInvalid,
			stepInputKeyRepairOfFailureSig:    signature,
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: signature,
			"stage":                   "llm_schema_invalid",
		},
	}
	currentFailed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/failed-current"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: signature,
			"stage":                   "llm_schema_invalid",
		},
	}
	for _, step := range []*runmodel.Step{prevFailed, repairAttempt, currentFailed} {
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}
	orch := &RuntimeOrchestrator{
		steps:   repo,
		tickets: memory.NewDecisionTicketRepository(),
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 10}),
	}
	run := &runmodel.Run{ID: runID, DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	if orch.shouldBlockOnDuplicateRuntimeFailure(ctx, run, currentFailed) {
		t.Fatalf("expected repeated llm decision invalid failure to continue schema recovery before repair budget is exhausted")
	}
}

func TestRuntimeRepairFollowUpStepInputIncludesFailureContext(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-repair-context")
	signature := "class=shell_command_failed|kind=shell/command|failureTargets=main.go|exitCode=1"
	repo := memory.NewStepRepository()
	_ = repo
	orch := &RuntimeOrchestrator{
		steps:  repo,
		policy: NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 10}),
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-metrics", DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/failed-current"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: signature,
		},
	}
	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failed, 2, nil)
	if input[stepInputKeyRuntimeRepairFollowUp] != true {
		t.Fatalf("runtimeRepairFollowUp = %#v, want true", input[stepInputKeyRuntimeRepairFollowUp])
	}
	if input[stepInputKeyRepairOfStepID] != string(failed.ID) {
		t.Fatalf("repairOfStepId = %#v, want %q", input[stepInputKeyRepairOfStepID], failed.ID)
	}
	if input[stepInputKeyRecoveryAction] != string(runtimeRecoveryActionRuntimeRepair) {
		t.Fatalf("recoveryAction = %#v, want runtime_repair", input[stepInputKeyRecoveryAction])
	}
	if input["maxRuntimeRepairAttempts"] != 10 {
		t.Fatalf("maxRuntimeRepairAttempts = %#v, want 10", input["maxRuntimeRepairAttempts"])
	}
	lastFailure := mapFromValue(input["lastFailureDiagnostics"])
	if lastFailure["failureSignature"] != signature || lastFailure["failureClass"] != failureClassShellCommand {
		t.Fatalf("lastFailureDiagnostics = %#v, want failure signature/class captured", lastFailure)
	}
}

func TestRuntimeContractViolationUsesRuntimeRepairInsteadOfProviderRetry(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-llm-contract-violation")
	repo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		runtime: &stubRuntimeAdapter{},
		steps:   repo,
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxProviderRetryAttempts: 3}),
	}
	run := &runmodel.Run{ID: runID, DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-graph-002-verify"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
		},
		Output: map[string]any{
			failureOutputKeyClass: failureClassRuntime,
			"stage":               "llm_execution_graph_plan",
			"error":               "invalid argument: executionGraph patch/apply input invalid: invalid argument: patch diff contains redacted runtime markers",
			"reason":              "invalid argument: executionGraph patch/apply input invalid: invalid argument: patch diff contains redacted runtime markers",
		},
	}
	if orch.shouldAttemptProviderRetry(ctx, run, failed) {
		t.Fatalf("expected runtime contract violation to avoid provider retry")
	}
	if action := orch.recoveryActionForFailedStep(ctx, run, failed); action != runtimeRecoveryActionRuntimeRepair {
		t.Fatalf("recoveryActionForFailedStep = %s, want runtime_repair", action)
	}
}

func TestShouldBlockOnDuplicateRuntimeFailureForRepeatedRuntimeContractLoopWithoutNewEvidence(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-duplicate-runtime-contract-loop")
	signature := "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|error=invalid argument: executiongraph patch/apply input invalid: invalid argument: patch diff contains redacted runtime markers"
	repo := memory.NewStepRepository()
	prevFailed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: signature,
			"stage":                   "llm_execution_graph_plan",
			"error":                   "invalid argument: executionGraph patch/apply input invalid: invalid argument: patch diff contains redacted runtime markers",
		},
	}
	repairAttempt := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-attempt-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfFailureClass:  failureClassRuntime,
			stepInputKeyRepairOfFailureSig:    signature,
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: signature,
			"stage":                   "llm_execution_graph_plan",
			"error":                   "invalid argument: executionGraph patch/apply input invalid: invalid argument: patch diff contains redacted runtime markers",
		},
	}
	currentFailed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-002"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: signature,
			"stage":                   "llm_execution_graph_plan",
			"error":                   "invalid argument: executionGraph patch/apply input invalid: invalid argument: patch diff contains redacted runtime markers",
		},
	}
	for _, step := range []*runmodel.Step{prevFailed, repairAttempt, currentFailed} {
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}
	orch := &RuntimeOrchestrator{
		steps:   repo,
		tickets: memory.NewDecisionTicketRepository(),
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 10}),
	}
	run := &runmodel.Run{ID: runID, DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	if !orch.shouldBlockOnDuplicateRuntimeFailure(ctx, run, currentFailed) {
		t.Fatalf("expected repeated runtime contract loop to be blocked when no new evidence exists")
	}
}

func TestShouldNotBlockOnDuplicateRuntimeFailureForRepeatedRuntimeContractLoopWithNewEvidence(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-duplicate-runtime-contract-loop-with-evidence")
	signature := "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|error=invalid argument: executiongraph patch/apply input invalid: invalid argument: patch diff contains redacted runtime markers"
	repo := memory.NewStepRepository()
	prevFailed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: signature,
			"stage":                   "llm_execution_graph_plan",
			"error":                   "invalid argument: executionGraph patch/apply input invalid: invalid argument: patch diff contains redacted runtime markers",
		},
	}
	repairAttempt := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-attempt-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfFailureClass:  failureClassRuntime,
			stepInputKeyRepairOfFailureSig:    signature,
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: signature,
			"stage":                   "llm_execution_graph_plan",
			"error":                   "invalid argument: executionGraph patch/apply input invalid: invalid argument: patch diff contains redacted runtime markers",
		},
	}
	newEvidence := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001a-evidence"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"status": "completed",
			"stage":  "patch_apply",
		},
	}
	currentFailed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-002"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: signature,
			"stage":                   "llm_execution_graph_plan",
			"error":                   "invalid argument: executionGraph patch/apply input invalid: invalid argument: patch diff contains redacted runtime markers",
		},
	}
	for _, step := range []*runmodel.Step{prevFailed, repairAttempt, newEvidence, currentFailed} {
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}
	orch := &RuntimeOrchestrator{
		steps:   repo,
		tickets: memory.NewDecisionTicketRepository(),
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 10}),
	}
	run := &runmodel.Run{ID: runID, DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	if orch.shouldBlockOnDuplicateRuntimeFailure(ctx, run, currentFailed) {
		t.Fatalf("expected duplicate runtime contract loop to continue when new action evidence exists")
	}
}
