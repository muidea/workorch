package run

import (
	"context"
	"errors"
	"testing"

	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/contracts"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestService_CreateRunStartRunCompleteRun_Mainline(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		&serviceRuntimeAdapter{output: map[string]any{"status": "ok"}},
		runRepo,
		stepRepo,
		workspaceRepo,
	)
	service := NewService(runRepo, workspaceRepo, orch)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	if err := service.CreateRun(ctx, contracts.CreateRunCommand{RunID: runID, WorkspaceID: workspaceID}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	created, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get created run: %v", err)
	}
	if created.State != common.RunStateCreated {
		t.Fatalf("created run state = %q, want created", created.State)
	}

	if err := service.StartRun(ctx, contracts.StartRunCommand{RunID: runID}); err != nil {
		t.Fatalf("start run: %v", err)
	}

	completed, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get completed run: %v", err)
	}
	if completed.State != common.RunStateCompleted {
		t.Fatalf("completed run state = %q, want completed", completed.State)
	}

	step, err := stepRepo.Get(ctx, common.StepID(string(runID)+"/step-1"))
	if err != nil {
		t.Fatalf("get completed step: %v", err)
	}
	if step.State != common.StepStateCompleted {
		t.Fatalf("step state = %q, want completed", step.State)
	}
}

func TestService_CreateRun_ValidatesWorkspaceAndArguments(t *testing.T) {
	ctx := context.Background()
	service := NewService(memory.NewRunRepository(), memory.NewWorkspaceRepository(), nil)

	if err := service.CreateRun(ctx, contracts.CreateRunCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty create run err = %v, want invalid argument", err)
	}

	if err := service.CreateRun(ctx, contracts.CreateRunCommand{RunID: common.RunID("run-1"), WorkspaceID: common.WorkspaceID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing workspace err = %v, want not found", err)
	}
}

func TestService_CompleteRun_WithoutOrchestrator(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	service := NewService(runRepo, workspaceRepo, nil)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := service.CreateRun(ctx, contracts.CreateRunCommand{RunID: runID, WorkspaceID: workspaceID}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	if err := service.CompleteRun(ctx, contracts.CompleteRunCommand{RunID: runID}); err != nil {
		t.Fatalf("complete run: %v", err)
	}

	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateCompleted {
		t.Fatalf("run state = %q, want completed", r.State)
	}
}

type serviceRuntimeAdapter struct {
	output map[string]any
	err    error
}

func (s *serviceRuntimeAdapter) ExecuteStep(_ context.Context, _ common.RunID, _ common.StepID) (map[string]any, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.output, nil
}
