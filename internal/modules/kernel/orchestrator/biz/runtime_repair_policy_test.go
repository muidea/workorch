package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func shouldBlockOnDuplicateRuntimeFailureForTest(ctx context.Context, orch *RuntimeOrchestrator, run *runmodel.Run, failedStep *runmodel.Step) bool {
	return shouldBlockOnDuplicateRuntimeFailure(ctx, orch.steps, orch.repairManager(), orch.policy, orch.tickets != nil, run, failedStep)
}

func runtimeRecoveryActionForFailedStepForTest(ctx context.Context, orch *RuntimeOrchestrator, run *runmodel.Run, failedStep *runmodel.Step) runtimeRecoveryAction {
	return runtimeRecoveryActionForFailedStep(ctx, orch.repairManager(), orch.policy, orch.runtime != nil, orch.tickets != nil, run, failedStep)
}

func TestExternalDependencyValidationFailureBlocksInsteadOfRepeatingRepair(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-external-dependency-block")
	failed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/verify"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=resolve-dependency|exitCode=1",
			"stage":                   "shell_command_failed",
			"validationFailureDiagnostic": map[string]any{
				"schemaVersion":                 runtimemodel.ValidationFailureDiagnosticSchemaVersion,
				"class":                         runtimemodel.ValidationDiagnosticExternalDependency,
				"recommendedEvidenceTargetType": runtimemodel.ValidationEvidenceTargetDependencyContract,
				"stderrPreview":                 `fetch "https://example.invalid/module": dial tcp 203.0.113.10:443: i/o timeout`,
			},
		},
	}
	orch := &RuntimeOrchestrator{
		steps:   memory.NewStepRepository(),
		tickets: memory.NewDecisionTicketRepository(),
		runtime: &stubRuntimeAdapter{},
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 10}),
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-external-dependency-block", InitialStepKind: llmWorkspaceExecutionStepKind}

	action := runtimeRecoveryActionForFailedStepForTest(ctx, orch, run, failed)

	if action != runtimeRecoveryActionRuntimeRepairExhausted {
		t.Fatalf("recovery action = %s, want runtime repair exhausted blocker for external dependency", action)
	}
	diagnostics := runtimeFailureDecisionDiagnostics(failed, errors.New("exit status 1"))
	if diagnostics["externalDependencyUnavailable"] != true || diagnostics["requiresExternalDependency"] != true {
		t.Fatalf("diagnostics = %+v, want external dependency blocker metadata", diagnostics)
	}
}

func TestMaxRuntimeRepairAttemptsUsesConfiguredPolicy(t *testing.T) {
	orch := &RuntimeOrchestrator{
		policy: NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 10}),
	}
	if got := maxRuntimeRepairAttemptsForPolicy(orch.policy); got != 10 {
		t.Fatalf("max runtime repair attempts = %d, want 10", got)
	}
}

func TestRuntimeFailureOutputPreservesEvidenceQualityCoverageDiagnostics(t *testing.T) {
	err := evidenceKindRequirementError(profileEvidenceKindRequirement{
		Scope:             "deliverable",
		AcceptedStepKinds: []string{"patch/apply"},
	}, "stateful-output", "run/graph/001-read", "", "requires evidence from step kinds: patch/apply")

	output := runtimeFailureOutput(map[string]any{
		"runtimeDiagnostics": map[string]any{
			"stage": "llm_dod_assessment",
		},
	}, err)

	diagnostics := mapFromValue(output["runtimeDiagnostics"])
	if diagnostics["kind"] != "dod_evidence_quality_gap" || diagnostics["stage"] != "llm_dod_assessment" {
		t.Fatalf("runtimeDiagnostics = %+v, want merged evidence-quality diagnostics", diagnostics)
	}
	missing := mapSliceFromValue(diagnostics["missingDeliverableCoverage"])
	if len(missing) != 1 || stringFromMap(missing[0], "item") != "stateful-output" {
		t.Fatalf("runtimeDiagnostics.missingDeliverableCoverage = %+v, want failed deliverable item", missing)
	}
	topLevelMissing := mapSliceFromValue(output["missingDeliverableCoverage"])
	if len(topLevelMissing) != 1 || stringFromMap(topLevelMissing[0], "item") != "stateful-output" {
		t.Fatalf("missingDeliverableCoverage = %+v, want promoted diagnostics gap", topLevelMissing)
	}
}

func TestStateChangingRepairRequiredForEvidenceQualityStateChangeGap(t *testing.T) {
	err := evidenceKindRequirementError(profileEvidenceKindRequirement{
		Scope:             "deliverable",
		AcceptedStepKinds: []string{"patch/apply"},
	}, "stateful-output", "run/graph/001-read", "", "requires evidence from step kinds: patch/apply")
	step := &runmodel.Step{
		ID:   "run-evidence-quality-gap/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
		},
		Output: runtimeFailureOutput(map[string]any{
			"stage": "llm_dod_assessment",
		}, err),
	}

	if !stateChangingRepairRequired(nil, step, err) {
		t.Fatalf("stateChangingRepairRequired = false, want state-changing repair for profile evidence-quality gap")
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
	if shouldBlockOnDuplicateRuntimeFailureForTest(ctx, orch, run, currentFailed) {
		t.Fatalf("expected duplicate shell-command failure to stay repairable before scoped budget")
	}
}

func TestRepeatedTargetDiagnosticUsesAutomaticStrategyRepairBeforeDecision(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-repeated-target-diagnostic")
	rootStepID := common.StepID(string(runID) + "/graph-001/001-test")
	signature := "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|exitCode=1|error=main_test.go:20:13: assignment mismatch"
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
			"stderr":                  "./main_test.go:20:13: assignment mismatch: 2 variables but constructor returns 1 value",
			"exitCode":                1,
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
			"stderr":                  "./main_test.go:20:13: assignment mismatch: 2 variables but constructor returns 1 value",
			"exitCode":                1,
		},
	}
	steps := []*runmodel.Step{prevFailed}
	for i := 1; i <= 3; i++ {
		steps = append(steps, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-%03d", runID, i)),
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
		})
	}
	steps = append(steps, currentFailed)
	for _, step := range steps {
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}
	orch := &RuntimeOrchestrator{
		steps:   repo,
		runtime: &stubRuntimeAdapter{},
		tickets: memory.NewDecisionTicketRepository(),
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 10}),
	}
	run := &runmodel.Run{ID: runID, DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	if shouldBlockOnDuplicateRuntimeFailureForTest(ctx, orch, run, currentFailed) {
		t.Fatalf("expected repeated target diagnostics to get one automatic strategy repair before decision")
	}
	if action := runtimeRecoveryActionForFailedStepForTest(ctx, orch, run, currentFailed); action != runtimeRecoveryActionRuntimeRepair {
		t.Fatalf("recoveryAction = %s, want runtime_repair strategy reassessment", action)
	}
	input := orch.runtimeRepairFollowUpStepInput(ctx, run, currentFailed, 4, nil)
	reassessment := mapFromValue(input["strategyReassessment"])
	if !boolValue(reassessment["required"]) || int64FromValue(reassessment["targetRepairAttempts"]) < 3 {
		t.Fatalf("strategyReassessment = %#v, want required repeated-target repair strategy", reassessment)
	}
	if got := input["targetRepairAttempts"]; int64FromValue(got) < 3 {
		t.Fatalf("targetRepairAttempts = %#v, want repeated target attempts projected", got)
	}
}

func TestShouldContinueRepeatedTargetDiagnosticUntilRepairBudgetExhausted(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-repeated-target-diagnostic-exhausted")
	rootStepID := common.StepID(string(runID) + "/graph-001/001-test")
	signature := "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|exitCode=1|error=main_test.go:20:13: assignment mismatch"
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
			"stderr":                  "./main_test.go:20:13: assignment mismatch: 2 variables but constructor returns 1 value",
			"exitCode":                1,
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
			"stderr":                  "./main_test.go:20:13: assignment mismatch: 2 variables but constructor returns 1 value",
			"exitCode":                1,
		},
	}
	steps := []*runmodel.Step{prevFailed}
	for i := 1; i < 6; i++ {
		steps = append(steps, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-%03d", runID, i)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
				stepInputKeyRepairOfFailureSig:    signature,
				stepInputKeyRepairOfRootStepID:    string(rootStepID),
				"strategyReassessment":            map[string]any{"required": true, "targetRepairAttempts": i},
			},
			Output: map[string]any{
				failureOutputKeyClass:     failureClassShellCommand,
				failureOutputKeySignature: signature,
				"stage":                   "shell_command_failed",
			},
		})
	}
	steps = append(steps, currentFailed)
	for _, step := range steps {
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
	if shouldBlockOnDuplicateRuntimeFailureForTest(ctx, orch, run, currentFailed) {
		t.Fatalf("expected repeated target diagnostics to continue automatic strategy repair before repair budget exhaustion")
	}
}

func TestShouldBlockOnRepeatedTargetDiagnosticWhenRepairBudgetExhausted(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-repeated-target-diagnostic-budget-exhausted")
	rootStepID := common.StepID(string(runID) + "/graph-001/001-test")
	signature := "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|exitCode=1|error=main_test.go:20:13: assignment mismatch"
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
			"stderr":                  "./main_test.go:20:13: assignment mismatch: 2 variables but constructor returns 1 value",
			"exitCode":                1,
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
			"stderr":                  "./main_test.go:20:13: assignment mismatch: 2 variables but constructor returns 1 value",
			"exitCode":                1,
		},
	}
	steps := []*runmodel.Step{prevFailed}
	for i := 1; i <= 6; i++ {
		steps = append(steps, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-%03d", runID, i)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
				stepInputKeyRepairOfFailureSig:    signature,
				stepInputKeyRepairOfRootStepID:    string(rootStepID),
				"strategyReassessment":            map[string]any{"required": true, "targetRepairAttempts": i},
			},
			Output: map[string]any{
				failureOutputKeyClass:     failureClassShellCommand,
				failureOutputKeySignature: signature,
				"stage":                   "shell_command_failed",
			},
		})
	}
	steps = append(steps, currentFailed)
	for _, step := range steps {
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}
	orch := &RuntimeOrchestrator{
		steps:   repo,
		tickets: memory.NewDecisionTicketRepository(),
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 6}),
	}
	run := &runmodel.Run{ID: runID, DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	if !shouldBlockOnDuplicateRuntimeFailureForTest(ctx, orch, run, currentFailed) {
		t.Fatalf("expected repeated target diagnostics to escalate after repair budget exhaustion")
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
	if shouldBlockOnDuplicateRuntimeFailureForTest(ctx, orch, run, currentFailed) {
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
	if shouldAttemptProviderRetry(ctx, orch.repairManager(), orch.policy, orch.runtime != nil, run, failed) {
		t.Fatalf("expected runtime contract violation to avoid provider retry")
	}
	if action := runtimeRecoveryActionForFailedStepForTest(ctx, orch, run, failed); action != runtimeRecoveryActionRuntimeRepair {
		t.Fatalf("recoveryActionForFailedStep = %s, want runtime_repair", action)
	}
}

func TestRunLevelIneffectiveConvergenceEscalatesRuntimeRepair(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-ineffective-convergence")
	repo := memory.NewStepRepository()
	var current *runmodel.Step
	for idx := 1; idx <= minIneffectiveConvergenceRecoverySteps; idx++ {
		step := &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/recovery-%03d", runID, idx)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateCompleted,
			Input: map[string]any{
				"graphFollowUp": true,
			},
			Output: map[string]any{"stage": "llm_execution_graph_plan"},
		}
		if idx <= minIneffectiveConvergenceContractFailures {
			step.State = common.StepStateFailed
			step.Input[stepInputKeyRuntimeRepairFollowUp] = true
			step.Output = map[string]any{
				failureOutputKeyClass:          failureClassRuntime,
				failureOutputKeySignature:      fmt.Sprintf("runtime-contract-%03d", idx),
				outputKeyContractViolationType: contractViolationValidationEvidenceRequired,
				"stage":                        "llm_execution_graph_plan",
				"reason":                       "runtime contract violation",
			}
			current = step
		}
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}
	orch := &RuntimeOrchestrator{
		runtime: &stubRuntimeAdapter{},
		steps:   repo,
		tickets: memory.NewDecisionTicketRepository(),
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 20}),
	}
	run := &runmodel.Run{ID: runID, DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	if stats := orch.repairManager().runLevelIneffectiveConvergenceStats(ctx, run, current); !stats.exceeded() {
		t.Fatalf("run-level ineffective convergence stats = %+v, want exceeded", stats)
	}
	if action := runtimeRecoveryActionForFailedStepForTest(ctx, orch, run, current); action != runtimeRecoveryActionRuntimeRepairExhausted {
		t.Fatalf("recoveryActionForFailedStep = %s, want runtime_repair_exhausted", action)
	}
}

func TestRunLevelIneffectiveConvergenceEscalatesRepeatedCoverageFailures(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-ineffective-coverage")
	repo := memory.NewStepRepository()
	var current *runmodel.Step
	for idx := 1; idx <= minIneffectiveConvergenceRecoverySteps; idx++ {
		step := &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-coverage-%03d", runID, idx)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRepairOfFailureClass:  failureClassDoDCoverageMissing,
				stepInputKeyRepairOfFailureSig:    "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
			},
			Output: map[string]any{
				failureOutputKeyClass:     failureClassDoDCoverageMissing,
				failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
				"stage":                   "llm_dod_assessment",
				"reason":                  "invalid argument: dodAssessment is not satisfied",
			},
		}
		current = step
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}
	orch := &RuntimeOrchestrator{
		runtime: &stubRuntimeAdapter{},
		steps:   repo,
		tickets: memory.NewDecisionTicketRepository(),
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 20}),
	}
	run := &runmodel.Run{ID: runID, DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	stats := orch.repairManager().runLevelIneffectiveConvergenceStats(ctx, run, current)
	if !stats.exceeded() {
		t.Fatalf("run-level coverage convergence stats = %+v, want exceeded", stats)
	}
	if stats.CoverageFailures == 0 || !stats.CurrentIsCoverage {
		t.Fatalf("coverage convergence stats = %+v, want coverage failure counters", stats)
	}
	if action := runtimeRecoveryActionForFailedStepForTest(ctx, orch, run, current); action != runtimeRecoveryActionRuntimeRepairExhausted {
		t.Fatalf("recoveryActionForFailedStep = %s, want runtime_repair_exhausted", action)
	}
}

func TestRunLevelIneffectiveConvergenceIgnoresProviderOnlyFailures(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-only-convergence")
	repo := memory.NewStepRepository()
	var current *runmodel.Step
	for idx := 1; idx <= minIneffectiveConvergenceRecoverySteps+2; idx++ {
		step := &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/provider-retry-%03d", runID, idx)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyProviderRetryFollowUp: true,
			},
			Output: map[string]any{
				failureOutputKeyClass:     failureClassProviderTimeout,
				failureOutputKeySignature: fmt.Sprintf("provider-timeout-%03d", idx),
				"stage":                   "llm_provider_timeout",
			},
		}
		current = step
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}
	orch := &RuntimeOrchestrator{
		runtime: &stubRuntimeAdapter{},
		steps:   repo,
		tickets: memory.NewDecisionTicketRepository(),
	}
	run := &runmodel.Run{ID: runID, DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	if stats := orch.repairManager().runLevelIneffectiveConvergenceStats(ctx, run, current); stats.exceeded() {
		t.Fatalf("run-level ineffective convergence stats = %+v, want not exceeded for provider-only failures", stats)
	}
	if orch.repairManager().runLevelIneffectiveConvergenceExceeded(ctx, run, current) {
		t.Fatalf("run-level ineffective convergence should ignore provider-only failures")
	}
}

func TestProviderRetryBudgetShrinksBeforeStateChange(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-retry-before-state-change")
	rootStepID := common.StepID(string(runID) + "/planning-001")
	repo := memory.NewStepRepository()
	run := &runmodel.Run{
		ID:              runID,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	for attempt := 1; attempt <= maxProviderRetryAttemptsBeforeStateChange; attempt++ {
		if err := repo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/provider-retry-%03d", runID, attempt)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyProviderRetryFollowUp: true,
				stepInputKeyProviderRetryRootStep: string(rootStepID),
			},
			Output: map[string]any{
				failureOutputKeyClass: failureClassProviderTimeout,
				"stage":               "llm_provider_timeout",
			},
		}); err != nil {
			t.Fatalf("seed provider retry %d: %v", attempt, err)
		}
	}
	repair := newRepairSubsystem(repo, nil)
	failed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/provider-retry-002"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryRootStep: string(rootStepID),
		},
		Output: map[string]any{
			failureOutputKeyClass: failureClassProviderTimeout,
			"stage":               "llm_provider_timeout",
		},
	}
	policy := NormalizeRuntimePolicy(RuntimePolicy{MaxProviderRetryAttempts: 10})
	if got := repair.effectiveMaxProviderRetryAttempts(ctx, policy, run, failed); got != maxProviderRetryAttemptsBeforeStateChange {
		t.Fatalf("effective max provider retry attempts = %d, want pre-state-change budget", got)
	}
	if shouldAttemptProviderRetry(ctx, repair, policy, true, run, failed) {
		t.Fatal("shouldAttemptProviderRetry = true, want exhausted before state-changing output")
	}
	if !providerRetryBudgetExhausted(ctx, repair, policy, run, failed) {
		t.Fatal("providerRetryBudgetExhausted = false, want exhausted before state-changing output")
	}
}

func TestProviderRetryBudgetUsesPolicyAfterStateChangeAttempt(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-retry-after-state-change")
	rootStepID := common.StepID(string(runID) + "/planning-001")
	repo := memory.NewStepRepository()
	run := &runmodel.Run{
		ID:              runID,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	for attempt := 1; attempt <= maxProviderRetryAttemptsBeforeStateChange; attempt++ {
		if err := repo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/provider-retry-stateful-%03d", runID, attempt)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyProviderRetryFollowUp: true,
				stepInputKeyProviderRetryRootStep: string(rootStepID),
			},
			Output: map[string]any{
				failureOutputKeyClass: failureClassProviderTimeout,
				"stage":               "llm_provider_timeout",
			},
		}); err != nil {
			t.Fatalf("seed provider retry %d: %v", attempt, err)
		}
	}
	if err := repo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/001-apply"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("seed state-changing step: %v", err)
	}
	repair := newRepairSubsystem(repo, nil)
	failed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/provider-retry-002"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryRootStep: string(rootStepID),
		},
		Output: map[string]any{
			failureOutputKeyClass: failureClassProviderTimeout,
			"stage":               "llm_provider_timeout",
		},
	}
	policy := NormalizeRuntimePolicy(RuntimePolicy{MaxProviderRetryAttempts: 10})
	if got := repair.effectiveMaxProviderRetryAttempts(ctx, policy, run, failed); got != 10 {
		t.Fatalf("effective max provider retry attempts = %d, want configured policy after state change", got)
	}
	if !shouldAttemptProviderRetry(ctx, repair, policy, true, run, failed) {
		t.Fatal("shouldAttemptProviderRetry = false, want retry under configured budget after state-changing output")
	}
}

func TestEffectiveMaxRuntimeRepairAttemptsShrinksForRepeatedReadOnlyStateChangingLoopWithTargetEvidence(t *testing.T) {
	ctx := context.Background()
	repair := newRepairSubsystem(memory.NewStepRepository(), nil)
	policy := NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 10})
	run := &runmodel.Run{
		ID:              "run-read-only-state-changing-loop",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		DoDID:           common.DoDID("dod"),
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-read-only-state-changing-loop/repair-002",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"stateChangingOutputRequired":     true,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"repairActionPackage": map[string]any{
				"targetSnapshots": []map[string]any{{
					"targetRef":        "main.go",
					"exists":           true,
					"contentComplete":  true,
					"safeForFileWrite": true,
					"contentHash":      "hash-main",
					"content":          "package main\n",
				}},
				"requiredOutput": stateChangingRepairRequiredOutput(),
			},
		},
		Output: map[string]any{
			outputKeyContractViolationType: contractViolationRepairGraphWithoutPatchApply,
			"stage":                        "llm_execution_graph_plan",
			"error":                        "invalid argument: state-changing llm repair step returned executionGraph without patch/apply",
		},
	}

	if got := repair.effectiveMaxRuntimeRepairAttempts(ctx, policy, run, failed); got != maxRepeatedStateChangingContractLoopAttempts {
		t.Fatalf("effective max runtime repair attempts = %d, want %d", got, maxRepeatedStateChangingContractLoopAttempts)
	}
}

func TestCoverageStateChangeGapEscalatesAfterValidationOnlyRepair(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-coverage-state-change-gap")
	signature := "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment"
	repo := memory.NewStepRepository()
	previousRepair := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfFailureClass:  failureClassDoDCoverageMissing,
			stepInputKeyRepairOfFailureSig:    signature,
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: signature,
			"stage":                   "llm_dod_assessment",
		},
	}
	if err := repo.Create(ctx, previousRepair); err != nil {
		t.Fatalf("seed previous repair: %v", err)
	}
	repair := newRepairSubsystem(repo, nil)
	run := &runmodel.Run{ID: runID, DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-002"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfFailureClass:  failureClassDoDCoverageMissing,
			stepInputKeyRepairOfFailureSig:    signature,
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: signature,
			"stage":                   "llm_dod_assessment",
		},
	}
	input := map[string]any{
		"coverageTargetSet": []map[string]any{{
			"id":                    "cwi-state-change",
			"targetText":            "produce deliverable implementation",
			"missingEvidenceKinds":  []string{"state_change", "validation"},
			"requiredEvidenceKinds": []string{"state_change", "validation"},
		}},
	}

	if !coverageValidationRepairShouldEscalateToStateChanging(ctx, repair, run, failed, input) {
		t.Fatalf("coverageValidationRepairShouldEscalateToStateChanging = false, want state-change gap escalation")
	}

	if err := repo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/001-apply"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"status": "completed",
			"stage":  "patch_apply",
		},
	}); err != nil {
		t.Fatalf("seed completed state change: %v", err)
	}
	if coverageValidationRepairShouldEscalateToStateChanging(ctx, repair, run, failed, input) {
		t.Fatalf("coverageValidationRepairShouldEscalateToStateChanging = true, want no escalation after completed state-change evidence")
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
	if !shouldBlockOnDuplicateRuntimeFailureForTest(ctx, orch, run, currentFailed) {
		t.Fatalf("expected repeated runtime contract loop to be blocked when no new evidence exists")
	}
}

func TestShouldBlockOnDuplicateRuntimeContractLoopUsingInheritedRepairTargetScope(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-loop-inherited-target")
	rootStepID := common.StepID(string(runID) + "/graph-001/001-verify")
	signature := "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|error=invalid argument: strategyReassessment.required=true requires fields"
	repo := memory.NewStepRepository()
	targetPackage := map[string]any{
		"targetSnapshots": []map[string]any{{
			"targetRef":   "target.txt",
			"contentHash": "sha256:target",
			"complete":    true,
		}},
		"currentFailure": map[string]any{
			"failureClass":       failureClassShellCommand,
			"failureSignature":   "class=shell_command_failed|kind=shell/command|node=verify|failureTargets=target.txt|exitCode=1",
			"failureTargetScope": "failureTargets=target.txt",
		},
	}
	steps := []*runmodel.Step{
		{
			ID:    common.StepID(string(runID) + "/repair-attempt-001"),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  1,
				stepInputKeyRepairOfFailureClass:  failureClassRuntime,
				stepInputKeyRepairOfFailureSig:    signature,
				stepInputKeyRepairOfRootStepID:    string(rootStepID),
				"repairActionPackage":             targetPackage,
			},
			Output: map[string]any{
				failureOutputKeyClass:     failureClassRuntime,
				failureOutputKeySignature: signature,
				"stage":                   "llm_execution_graph_plan",
				"error":                   "invalid argument: strategyReassessment.required=true requires fields",
			},
		},
		{
			ID:    common.StepID(string(runID) + "/repair-attempt-002"),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  2,
				stepInputKeyRepairOfFailureClass:  failureClassRuntime,
				stepInputKeyRepairOfFailureSig:    signature,
				stepInputKeyRepairOfRootStepID:    string(rootStepID),
				"repairActionPackage":             targetPackage,
			},
			Output: map[string]any{
				failureOutputKeyClass:     failureClassRuntime,
				failureOutputKeySignature: signature,
				"stage":                   "llm_execution_graph_plan",
				"error":                   "invalid argument: strategyReassessment.required=true requires fields",
			},
		},
	}
	for _, step := range steps {
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}
	currentFailed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-attempt-003"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  3,
			stepInputKeyRepairOfFailureClass:  failureClassRuntime,
			stepInputKeyRepairOfFailureSig:    signature,
			stepInputKeyRepairOfRootStepID:    string(rootStepID),
			"repairActionPackage":             targetPackage,
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: signature,
			"stage":                   "llm_execution_graph_plan",
			"error":                   "invalid argument: strategyReassessment.required=true requires fields",
		},
	}
	if err := repo.Create(ctx, currentFailed); err != nil {
		t.Fatalf("seed current step: %v", err)
	}
	orch := &RuntimeOrchestrator{
		steps:   repo,
		tickets: memory.NewDecisionTicketRepository(),
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 3}),
	}
	run := &runmodel.Run{ID: runID, DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	if got := runtimeFailureTargetScopeFromStep(currentFailed); got != "failureTargets=target.txt" {
		t.Fatalf("runtimeFailureTargetScopeFromStep = %q, want target.txt scope", got)
	}
	if !shouldBlockOnDuplicateRuntimeFailureForTest(ctx, orch, run, currentFailed) {
		t.Fatalf("expected repeated runtime contract loop to use inherited target scope and block at budget")
	}
}

func TestRecoveryActionAllowsValidationWhenFailedPatchAlreadySatisfiedAtRepairBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-already-satisfied-patch-budget")
	workspaceID := common.WorkspaceID("ws-already-satisfied-patch-budget")
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "target.txt"), []byte("alpha\nfresh\nomega\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	workspaceRepo := memory.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "ws",
		DeliverableDirectory: deliverableDirectory,
		State:                common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	stepRepo := memory.NewStepRepository()
	signature := "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|failureTargets=target.txt|error=conflict: patch text_replace oldtext not found"
	repairAttempt := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-graph-001-001-patch"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfFailureClass:  failureClassPatchApply,
			stepInputKeyRepairOfFailureSig:    signature,
			stepInputKeyRepairOfRootStepID:    string(common.StepID(string(runID) + "/step-1")),
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: signature,
			"stage":                   "patch_apply",
			"error":                   "conflict: patch text_replace oldText not found",
		},
	}
	if err := stepRepo.Create(ctx, repairAttempt); err != nil {
		t.Fatalf("seed repair attempt: %v", err)
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/001-patch"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{
			"targetRef": "target.txt",
			"patchType": "text_replace",
			"oldText":   "stale",
			"newText":   "fresh",
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "patch_apply",
			"error":                   "conflict: patch text_replace oldText not found",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: signature,
			"patches": []map[string]any{{
				"targetRef": "target.txt",
				"patchType": "text_replace",
				"oldText":   "stale",
				"newText":   "fresh",
			}},
		},
	}
	orch := &RuntimeOrchestrator{
		runtime:    &stubRuntimeAdapter{},
		steps:      stepRepo,
		workspaces: workspaceRepo,
		tickets:    memory.NewDecisionTicketRepository(),
		policy:     NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 1}),
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	if len(orch.repairManager().runtimeRepairPatchAlreadySatisfiedSignals(ctx, run, failedStep)) != 1 {
		t.Fatalf("runtimeRepairPatchAlreadySatisfiedSignals missing for satisfied stale patch")
	}
	if shouldEscalateRuntimeRepairExhaustion(ctx, orch.repairManager(), orch.policy, true, run, failedStep) {
		t.Fatalf("shouldEscalateRuntimeRepairExhaustion = true, want false for already-satisfied patch")
	}
	if got := runtimeRecoveryActionForFailedStepForTest(ctx, orch, run, failedStep); got != runtimeRecoveryActionRuntimeRepair {
		t.Fatalf("recovery action = %s, want %s", got, runtimeRecoveryActionRuntimeRepair)
	}
}

func TestRecoveryActionAllowsPatchRefreshWhenBaseHashStaleAtRepairBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-stale-basehash-patch-budget")
	workspaceID := common.WorkspaceID("ws-stale-basehash-patch-budget")
	deliverableDirectory := t.TempDir()
	currentContent := "alpha\ncurrent\nomega\n"
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "target.txt"), []byte(currentContent), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	workspaceRepo := memory.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "ws",
		DeliverableDirectory: deliverableDirectory,
		State:                common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	stepRepo := memory.NewStepRepository()
	signature := "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|failureTargets=target.txt|error=conflict: patch basehash mismatch"
	repairAttempt := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-graph-001-001-patch"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfFailureClass:  failureClassPatchApply,
			stepInputKeyRepairOfFailureSig:    signature,
			stepInputKeyRepairOfRootStepID:    string(common.StepID(string(runID) + "/step-1")),
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: signature,
			"stage":                   "patch_apply",
			"error":                   "conflict: patch baseHash mismatch",
		},
	}
	if err := stepRepo.Create(ctx, repairAttempt); err != nil {
		t.Fatalf("seed repair attempt: %v", err)
	}
	staleBaseHash := sha256HexBytes([]byte("alpha\nprevious\nomega\n"))
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/001-patch"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{
			"targetRef": "target.txt",
			"patchType": "text_replace",
			"baseHash":  staleBaseHash,
			"oldText":   "current",
			"newText":   "fresh",
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "patch_apply",
			"error":                   "conflict: patch baseHash mismatch",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: signature,
			"patches": []map[string]any{{
				"targetRef": "target.txt",
				"patchType": "text_replace",
				"baseHash":  staleBaseHash,
				"oldText":   "current",
				"newText":   "fresh",
			}},
		},
	}
	orch := &RuntimeOrchestrator{
		runtime:    &stubRuntimeAdapter{},
		steps:      stepRepo,
		workspaces: workspaceRepo,
		tickets:    memory.NewDecisionTicketRepository(),
		policy:     NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 1}),
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	signals := orch.repairManager().runtimeRepairPatchRefreshableSignals(ctx, run, failedStep)
	if len(signals) != 1 || stringFromMap(signals[0], "currentContentHash") != sha256HexBytes([]byte(currentContent)) {
		t.Fatalf("runtimeRepairPatchRefreshableSignals = %#v, want current target refresh signal", signals)
	}
	if shouldEscalateRuntimeRepairExhaustion(ctx, orch.repairManager(), orch.policy, true, run, failedStep) {
		t.Fatalf("shouldEscalateRuntimeRepairExhaustion = true, want false for refreshable stale baseHash patch")
	}
	if shouldBlockOnDuplicateRuntimeFailureForTest(ctx, orch, run, failedStep) {
		t.Fatalf("shouldBlockOnDuplicateRuntimeFailure = true, want false for refreshable stale baseHash patch")
	}
	if got := runtimeRecoveryActionForFailedStepForTest(ctx, orch, run, failedStep); got != runtimeRecoveryActionRuntimeRepair {
		t.Fatalf("recovery action = %s, want %s", got, runtimeRecoveryActionRuntimeRepair)
	}
	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 2, nil)
	refreshSignals := mapSliceFromValue(input["currentTargetRefreshRequiredForFailedPatch"])
	if len(refreshSignals) != 1 || stringFromMap(refreshSignals[0], "failedBaseHash") != staleBaseHash {
		t.Fatalf("currentTargetRefreshRequiredForFailedPatch = %#v, want stale baseHash signal", refreshSignals)
	}
}

func TestRecoveryActionDoesNotBypassPatchBudgetWithoutCurrentTargetRefreshEvidence(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-stale-basehash-missing-target")
	workspaceID := common.WorkspaceID("ws-stale-basehash-missing-target")
	deliverableDirectory := t.TempDir()
	workspaceRepo := memory.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "ws",
		DeliverableDirectory: deliverableDirectory,
		State:                common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	stepRepo := memory.NewStepRepository()
	signature := "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|failureTargets=missing.txt|error=conflict: patch basehash mismatch"
	repairAttempt := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-graph-001-001-patch"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfFailureClass:  failureClassPatchApply,
			stepInputKeyRepairOfFailureSig:    signature,
			stepInputKeyRepairOfRootStepID:    string(common.StepID(string(runID) + "/step-1")),
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: signature,
			"stage":                   "patch_apply",
			"error":                   "conflict: patch baseHash mismatch",
		},
	}
	if err := stepRepo.Create(ctx, repairAttempt); err != nil {
		t.Fatalf("seed repair attempt: %v", err)
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/001-patch"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{
			"targetRef": "missing.txt",
			"patchType": "text_replace",
			"baseHash":  sha256HexBytes([]byte("previous")),
			"oldText":   "current",
			"newText":   "fresh",
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "patch_apply",
			"error":                   "conflict: patch baseHash mismatch",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: signature,
		},
	}
	orch := &RuntimeOrchestrator{
		runtime:    &stubRuntimeAdapter{},
		steps:      stepRepo,
		workspaces: workspaceRepo,
		tickets:    memory.NewDecisionTicketRepository(),
		policy:     NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 1}),
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, DoDID: common.DoDID("dod"), InitialStepKind: llmWorkspaceExecutionStepKind}
	if signals := orch.repairManager().runtimeRepairPatchRefreshableSignals(ctx, run, failedStep); len(signals) != 0 {
		t.Fatalf("runtimeRepairPatchRefreshableSignals = %#v, want none without current target", signals)
	}
	if !shouldEscalateRuntimeRepairExhaustion(ctx, orch.repairManager(), orch.policy, true, run, failedStep) {
		t.Fatalf("shouldEscalateRuntimeRepairExhaustion = false, want true without current target refresh evidence")
	}
	if got := runtimeRecoveryActionForFailedStepForTest(ctx, orch, run, failedStep); got != runtimeRecoveryActionRuntimeRepairExhausted {
		t.Fatalf("recovery action = %s, want %s", got, runtimeRecoveryActionRuntimeRepairExhausted)
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
	if shouldBlockOnDuplicateRuntimeFailureForTest(ctx, orch, run, currentFailed) {
		t.Fatalf("expected duplicate runtime contract loop to continue when new action evidence exists")
	}
}
