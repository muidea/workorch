package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRuntimeOrchestrator_RuntimeRepairExhaustionUsesDialogLanguage(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	workspaceID := common.WorkspaceID("ws-runtime-repair-localized")
	runID := common.RunID("run-runtime-repair-localized")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		DoDID:            common.DoDID("dod-runtime-repair-localized"),
		State:            common.RunStateRunning,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{"dialogLanguage": "zh-CN"},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		stepID := common.StepID(fmt.Sprintf("%s/seed-repair-%d", runID, attempt))
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    stepID,
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateCompleted,
			Input: map[string]any{stepInputKeyRuntimeRepairFollowUp: true},
		}); err != nil {
			t.Fatalf("seed repair step: %v", err)
		}
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{},
		events.NewMemoryHub(),
	)
	failedStep := &runmodel.Step{
		ID:    common.StepID(fmt.Sprintf("%s/block/run-tests", runID)),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{stepInputKeyGraphNodeID: "run-tests"},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|node=run-tests",
		},
	}

	if err := orch.dispatchRuntimeRepairFollowUp(ctx, run, failedStep, errors.New("exit status 1")); err != nil {
		t.Fatalf("repair exhaustion: %v", err)
	}
	updated, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	ticket, err := ticketRepo.Get(ctx, updated.BlockingTicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if !strings.Contains(ticket.Reason, "自动修复尝试已耗尽") || strings.Contains(ticket.Question, "Automatic runtime repair") {
		t.Fatalf("ticket = %+v, want Chinese repair exhaustion prompt", ticket)
	}
}

func TestRuntimeOrchestrator_DuplicateRuntimeFailureUsesDialogLanguage(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	workspaceID := common.WorkspaceID("ws-duplicate-failure-localized")
	runID := common.RunID("run-duplicate-failure-localized")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		State:            common.RunStateRunning,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{"dialogLanguage": "zh-CN"},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{},
		events.NewMemoryHub(),
	)
	failedStep := &runmodel.Step{
		ID:    common.StepID(fmt.Sprintf("%s/block/run-tests", runID)),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|node=run-tests",
		},
	}

	if err := orch.blockOnDuplicateRuntimeFailure(ctx, run, failedStep, errors.New("exit status 1")); err != nil {
		t.Fatalf("duplicate failure block: %v", err)
	}
	updated, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	ticket, err := ticketRepo.Get(ctx, updated.BlockingTicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if !strings.Contains(ticket.Reason, "检测到重复的运行时失败签名") || strings.Contains(ticket.Question, "The same runtime failure") {
		t.Fatalf("ticket = %+v, want Chinese duplicate failure prompt", ticket)
	}
	if ticket.Context[decisionContextKeyRecoveryPolicy] != decisionContextRecoveryPolicyRepairPlan {
		t.Fatalf("ticket context recoveryPolicy = %#v, want %q", ticket.Context[decisionContextKeyRecoveryPolicy], decisionContextRecoveryPolicyRepairPlan)
	}
	if ticket.Context["runtimeRepairLoopState"] != "duplicate_failure_escalated" {
		t.Fatalf("ticket context runtimeRepairLoopState = %#v, want duplicate_failure_escalated", ticket.Context["runtimeRepairLoopState"])
	}
	allowed, ok := ticket.Context["allowedRecoveryActions"].([]string)
	if !ok || len(allowed) != 1 || allowed[0] != "replan" {
		t.Fatalf("ticket context allowedRecoveryActions = %#v, want [replan]", ticket.Context["allowedRecoveryActions"])
	}
}

func TestRuntimeOrchestrator_PatchApprovalUsesDialogLanguage(t *testing.T) {
	ctx := context.Background()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		memory.NewStepRepository(),
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{},
		events.NewMemoryHub(),
	)

	spec, err := orch.patchApprovalBlockSpec(ctx, "run-patch-localized", "zh-CN", map[string]any{
		stepOutputKeyPatches: []map[string]any{{
			"patchType": "file_write",
			"targetRef": ".env",
			"content":   "TOKEN=secret\n",
		}},
	})
	if err != nil {
		t.Fatalf("patch approval: %v", err)
	}
	if !strings.Contains(spec.Reason, "需要审批") || !strings.Contains(spec.Action, "审批应用") {
		t.Fatalf("spec = %+v, want Chinese patch approval prompt", spec)
	}
}
