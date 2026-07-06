package orchestrator

import (
	"context"
	"testing"

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRuntimeOrchestratorBlocksOperatorMCPToolCallBeforeRuntimeExecution(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "completed", "stage": "mcp_tool_call_completed"}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-mcp-approval")
	runID := common.RunID("run-mcp-approval")
	stepID := common.StepID("run-mcp-approval/graph-001/001-call")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
		ExecutionContext: map[string]any{
			"mcpServers": []map[string]any{{
				"id":                    "mcp-operator",
				"allowedTools":          []string{"repo.summary"},
				"defaultApprovalPolicy": "operator",
			}},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	err := orch.dispatchStepWithWorkspace(ctx, run, stepID, "mcp/tool_call", map[string]any{
		"serverId": "mcp-operator",
		"tool":     "repo.summary",
	}, nil)
	if err != nil {
		t.Fatalf("dispatch mcp approval preflight: %v", err)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime calls before approval = %#v, want none", runtime.calls)
	}
	blocked, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run: %v", err)
	}
	if blocked.State != common.RunStateBlockedOnApproval || blocked.BlockingApprovalID == "" {
		t.Fatalf("blocked run = %+v, want blocked_on_approval", blocked)
	}
	approval, err := approvalRepo.Get(ctx, blocked.BlockingApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if approval.StepID != stepID || approval.Context["approvalPolicy"] != "operator" || approval.Context["operation"] != "mcp/tool_call" {
		t.Fatalf("approval = %+v, want mcp operator preflight context", approval)
	}

	approval.State = common.ApprovalStateApproved
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after approval: %v", err)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != stepID {
		t.Fatalf("runtime calls after approval = %#v, want [%s]", runtime.calls, stepID)
	}
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateSucceeded {
		t.Fatalf("step state = %s, want completed", step.State)
	}
}

func TestRuntimeOrchestratorCreateApprovalPublishesNotificationThroughAsyncHub(t *testing.T) {
	ctx := context.Background()
	approvalRepo := memory.NewApprovalRepository()
	hub := &recordingApprovalNotificationAsyncHub{}
	orch := &RuntimeOrchestrator{
		approvals: approvalRepo,
		eventHub:  hub,
	}
	run := &runmodel.Run{
		ID:          "run-create-approval-async",
		WorkspaceID: "ws-create-approval-async",
	}
	step := &runmodel.Step{
		ID:    "run-create-approval-async/step-001",
		RunID: run.ID,
	}

	if err := orch.createApproval(ctx, run, step, "approval-create-async", "requires approval", "apply patch", map[string]any{"operation": "test"}); err != nil {
		t.Fatalf("createApproval: %v", err)
	}
	if _, err := approvalRepo.Get(ctx, "approval-create-async"); err != nil {
		t.Fatalf("get created approval: %v", err)
	}
	if len(hub.sync) != 0 {
		t.Fatalf("sync publishes = %+v, want none for approval notification", hub.sync)
	}
	if len(hub.async) != 1 || hub.async[0].Topic != approvalevents.TopicApprovalCreated {
		t.Fatalf("async publishes = %+v, want one approval created notification", hub.async)
	}
	if hub.async[0].WorkspaceID != string(run.WorkspaceID) || hub.async[0].RunID != string(run.ID) || hub.async[0].StepID != string(step.ID) {
		t.Fatalf("async scope = workspace:%q run:%q step:%q", hub.async[0].WorkspaceID, hub.async[0].RunID, hub.async[0].StepID)
	}
}

type recordingApprovalNotificationAsyncHub struct {
	sync  []events.Envelope
	async []events.Envelope
}

func (h *recordingApprovalNotificationAsyncHub) Publish(_ context.Context, envelope events.Envelope) error {
	h.sync = append(h.sync, envelope)
	return nil
}

func (h *recordingApprovalNotificationAsyncHub) PublishAsync(_ context.Context, envelope events.Envelope) error {
	h.async = append(h.async, envelope)
	return nil
}

func (h *recordingApprovalNotificationAsyncHub) Subscribe(string, events.Handler) func() {
	return func() {}
}
