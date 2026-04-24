package legacy

import (
	"context"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/legacy/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestMinimalRuntimeAdapter_ExecuteStepReturnsCompletedPayload(t *testing.T) {
	adapter := NewMinimalRuntimeAdapter()

	output, err := adapter.ExecuteStep(
		context.Background(),
		common.RunID("run-1"),
		common.StepID("run-1/step-1"),
	)
	if err != nil {
		t.Fatalf("execute step: %v", err)
	}
	if output["runId"] != "run-1" {
		t.Fatalf("runId = %v, want run-1", output["runId"])
	}
	if output["stepId"] != "run-1/step-1" {
		t.Fatalf("stepId = %v, want run-1/step-1", output["stepId"])
	}
	if output["status"] != "completed" {
		t.Fatalf("status = %v, want completed", output["status"])
	}
}

func TestMinimalRuntimeAdapter_ExecuteStepReturnsErrorForFailureRun(t *testing.T) {
	adapter := NewMinimalRuntimeAdapter()

	_, err := adapter.ExecuteStep(
		context.Background(),
		common.RunID("run-workorch-"+modulecommon.RuntimeFailureRunToken),
		common.StepID("run-workorch-runtime-failed/step-1"),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "legacy runtime execution failed") {
		t.Fatalf("err = %v, want legacy runtime failure", err)
	}
}

func TestMinimalRuntimeAdapter_ExecuteStepReturnsNextStepForMultiStepRun(t *testing.T) {
	adapter := NewMinimalRuntimeAdapter()

	output, err := adapter.ExecuteStep(
		context.Background(),
		common.RunID("run-workorch-"+modulecommon.MultiStepRunToken),
		common.StepID("run-workorch-multi-step/step-1"),
	)
	if err != nil {
		t.Fatalf("execute step: %v", err)
	}
	if output["nextStep"] != "run-workorch-multi-step/step-2" {
		t.Fatalf("nextStep = %v, want run-workorch-multi-step/step-2", output["nextStep"])
	}
	if output["stage"] != "prepare" {
		t.Fatalf("stage = %v, want prepare", output["stage"])
	}
}

func TestMinimalRuntimeAdapter_ExecuteStepReturnsDecisionBlockForRuntimeDecisionRun(t *testing.T) {
	adapter := NewMinimalRuntimeAdapter()

	output, err := adapter.ExecuteStep(
		context.Background(),
		common.RunID("run-workorch-"+modulecommon.RuntimeDecisionBlockRunToken),
		common.StepID("run-workorch-runtime-decision-block/step-1"),
	)
	if err != nil {
		t.Fatalf("execute step: %v", err)
	}
	if output["blockDecisionReason"] != "workorch runtime decision gate" {
		t.Fatalf("blockDecisionReason = %v, want workorch runtime decision gate", output["blockDecisionReason"])
	}
	if output["blockDecisionTicketId"] != "ticket-run-workorch-runtime-decision-block-runtime-decision" {
		t.Fatalf("blockDecisionTicketId = %v, want ticket-run-workorch-runtime-decision-block-runtime-decision", output["blockDecisionTicketId"])
	}
	if output["nextStep"] != "run-workorch-runtime-decision-block/step-2" {
		t.Fatalf("nextStep = %v, want run-workorch-runtime-decision-block/step-2", output["nextStep"])
	}
}

func TestMinimalRuntimeAdapter_ExecuteStepReturnsApprovalBlockForRuntimeApprovalRun(t *testing.T) {
	adapter := NewMinimalRuntimeAdapter()

	output, err := adapter.ExecuteStep(
		context.Background(),
		common.RunID("run-workorch-"+modulecommon.RuntimeApprovalBlockRunToken),
		common.StepID("run-workorch-runtime-approval-block/step-1"),
	)
	if err != nil {
		t.Fatalf("execute step: %v", err)
	}
	if output["blockApprovalReason"] != "workorch runtime approval gate" {
		t.Fatalf("blockApprovalReason = %v, want workorch runtime approval gate", output["blockApprovalReason"])
	}
	if output["blockApprovalId"] != "approval-run-workorch-runtime-approval-block-runtime-approval" {
		t.Fatalf("blockApprovalId = %v, want approval-run-workorch-runtime-approval-block-runtime-approval", output["blockApprovalId"])
	}
	if output["nextStep"] != "run-workorch-runtime-approval-block/step-2" {
		t.Fatalf("nextStep = %v, want run-workorch-runtime-approval-block/step-2", output["nextStep"])
	}
}

func TestMinimalRuntimeAdapter_ExecuteStepReturnsBranchRouteForConditionalBranchRun(t *testing.T) {
	adapter := NewMinimalRuntimeAdapter()

	output, err := adapter.ExecuteStep(
		context.Background(),
		common.RunID("run-workorch-"+modulecommon.ConditionalBranchRunToken),
		common.StepID("run-workorch-conditional-branch/step-1"),
	)
	if err != nil {
		t.Fatalf("execute step: %v", err)
	}
	if output["branch"] != "fast-track" {
		t.Fatalf("branch = %v, want fast-track", output["branch"])
	}
	if output["nextStep"] != "run-workorch-conditional-branch/step-fast" {
		t.Fatalf("nextStep = %v, want run-workorch-conditional-branch/step-fast", output["nextStep"])
	}
}

func TestMinimalRuntimeAdapter_ExecuteStepReturnsReviewRouteForConditionalBranchSlowRun(t *testing.T) {
	adapter := NewMinimalRuntimeAdapter()

	output, err := adapter.ExecuteStep(
		context.Background(),
		common.RunID("run-workorch-"+modulecommon.ConditionalBranchRunToken+"-branch-slow"),
		common.StepID("run-workorch-conditional-branch-slow/step-1"),
	)
	if err != nil {
		t.Fatalf("execute step: %v", err)
	}
	if output["branch"] != "review-path" {
		t.Fatalf("branch = %v, want review-path", output["branch"])
	}
	if output["nextStep"] != "run-workorch-conditional-branch-branch-slow/step-review" {
		t.Fatalf("nextStep = %v, want run-workorch-conditional-branch-branch-slow/step-review", output["nextStep"])
	}
}

func TestMinimalRuntimeAdapter_ExecuteStepReturnsFirstGateForMultiDecisionRun(t *testing.T) {
	adapter := NewMinimalRuntimeAdapter()

	output, err := adapter.ExecuteStep(
		context.Background(),
		common.RunID("run-workorch-"+modulecommon.MultiDecisionGateRunToken),
		common.StepID("run-workorch-multi-decision-gate/step-1"),
	)
	if err != nil {
		t.Fatalf("execute step: %v", err)
	}
	if output["blockDecisionReason"] != "workorch runtime gate 1" {
		t.Fatalf("blockDecisionReason = %v, want workorch runtime gate 1", output["blockDecisionReason"])
	}
	if output["blockDecisionTicketId"] != "ticket-run-workorch-multi-decision-gate-gate-1" {
		t.Fatalf("blockDecisionTicketId = %v, want ticket-run-workorch-multi-decision-gate-gate-1", output["blockDecisionTicketId"])
	}
	if output["nextStep"] != "run-workorch-multi-decision-gate/step-2" {
		t.Fatalf("nextStep = %v, want run-workorch-multi-decision-gate/step-2", output["nextStep"])
	}
}

func TestMinimalRuntimeAdapter_ExecuteStepReturnsSecondGateForMultiDecisionRun(t *testing.T) {
	adapter := NewMinimalRuntimeAdapter()

	output, err := adapter.ExecuteStep(
		context.Background(),
		common.RunID("run-workorch-"+modulecommon.MultiDecisionGateRunToken),
		common.StepID("run-workorch-multi-decision-gate/step-2"),
	)
	if err != nil {
		t.Fatalf("execute step: %v", err)
	}
	if output["blockDecisionReason"] != "workorch runtime gate 2" {
		t.Fatalf("blockDecisionReason = %v, want workorch runtime gate 2", output["blockDecisionReason"])
	}
	if output["blockDecisionTicketId"] != "ticket-run-workorch-multi-decision-gate-gate-2" {
		t.Fatalf("blockDecisionTicketId = %v, want ticket-run-workorch-multi-decision-gate-gate-2", output["blockDecisionTicketId"])
	}
	if output["nextStep"] != "run-workorch-multi-decision-gate/step-3" {
		t.Fatalf("nextStep = %v, want run-workorch-multi-decision-gate/step-3", output["nextStep"])
	}
}

func TestMinimalRuntimeAdapter_ExecuteStepReturnsDecisionGateForMixedDecisionApprovalRun(t *testing.T) {
	adapter := NewMinimalRuntimeAdapter()

	output, err := adapter.ExecuteStep(
		context.Background(),
		common.RunID("run-workorch-"+modulecommon.MixedDecisionApprovalRunToken),
		common.StepID("run-workorch-mixed-decision-approval/step-1"),
	)
	if err != nil {
		t.Fatalf("execute step: %v", err)
	}
	if output["blockDecisionReason"] != "workorch mixed decision gate" {
		t.Fatalf("blockDecisionReason = %v, want workorch mixed decision gate", output["blockDecisionReason"])
	}
	if output["blockDecisionTicketId"] != "ticket-run-workorch-mixed-decision-approval-mixed-decision" {
		t.Fatalf("blockDecisionTicketId = %v, want ticket-run-workorch-mixed-decision-approval-mixed-decision", output["blockDecisionTicketId"])
	}
	if output["nextStep"] != "run-workorch-mixed-decision-approval/step-2" {
		t.Fatalf("nextStep = %v, want run-workorch-mixed-decision-approval/step-2", output["nextStep"])
	}
}

func TestMinimalRuntimeAdapter_ExecuteStepReturnsApprovalGateForMixedDecisionApprovalRun(t *testing.T) {
	adapter := NewMinimalRuntimeAdapter()

	output, err := adapter.ExecuteStep(
		context.Background(),
		common.RunID("run-workorch-"+modulecommon.MixedDecisionApprovalRunToken),
		common.StepID("run-workorch-mixed-decision-approval/step-2"),
	)
	if err != nil {
		t.Fatalf("execute step: %v", err)
	}
	if output["blockApprovalReason"] != "workorch mixed approval gate" {
		t.Fatalf("blockApprovalReason = %v, want workorch mixed approval gate", output["blockApprovalReason"])
	}
	if output["blockApprovalId"] != "approval-run-workorch-mixed-decision-approval-mixed-approval" {
		t.Fatalf("blockApprovalId = %v, want approval-run-workorch-mixed-decision-approval-mixed-approval", output["blockApprovalId"])
	}
	if output["nextStep"] != "run-workorch-mixed-decision-approval/step-3" {
		t.Fatalf("nextStep = %v, want run-workorch-mixed-decision-approval/step-3", output["nextStep"])
	}
}
