package orchestrator

import (
	"context"
	"testing"
	"time"

	magictask "github.com/muidea/magicCommon/task"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	orchestratorservice "github.com/muidea/workorch/internal/modules/kernel/orchestrator/service"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestModuleStartRunCommandAcceptsStateAndSchedulesBackgroundDispatch(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-background-start")
	workspaceID := common.WorkspaceID("ws-module-background-start")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	hub := events.NewMemoryHub()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		moduleNoopRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository(),
		hub,
	)
	bg := &recordingBackgroundRoutine{}
	module := &Module{
		bizPtr:            orch,
		servicePtr:        orchestratorservice.New(orchestratorServiceDependencies(orch)),
		backgroundRoutine: bg,
	}

	err := module.handleCommand(
		common.ContextWithRuntimeLLMProviderID(ctx, "provider-test"),
		events.Envelope{
			Topic:       orchestratorevents.TopicOrchestratorCommandStartRun,
			SubjectType: "run",
			SubjectID:   string(runID),
			WorkspaceID: string(workspaceID),
			RunID:       string(runID),
			Payload:     map[string]any{events.PayloadKeyRunID: string(runID)},
		},
	)
	if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("start command err = %v, want response", err)
	}
	if bg.asyncFunctionCalls != 1 || bg.lastAsyncFunction == nil {
		t.Fatalf("background calls = %d fn=%v, want one scheduled dispatch", bg.asyncFunctionCalls, bg.lastAsyncFunction != nil)
	}
	run, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	if run.State != common.RunStateRunning {
		t.Fatalf("run state = %s, want running before background dispatch executes", run.State)
	}
	workspace, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("read workspace: %v", err)
	}
	if workspace.State != common.WorkspaceStateRunning || workspace.ActiveRunID != runID {
		t.Fatalf("workspace = %+v, want running active run", workspace)
	}
}

func orchestratorServiceDependencies(orch *orchestratorbiz.RuntimeOrchestrator) orchestratorservice.Dependencies {
	return orchestratorservice.Dependencies{
		StartRun:         orch.StartRun,
		DispatchNextStep: orch.DispatchNextStep,
		BlockOnDecision:  orch.BlockOnDecision,
		BlockOnApproval:  orch.BlockOnApproval,
		ResumeRun:        orch.ResumeRun,
		ResumeRunPatch:   orch.ResumeRunWithExecutionContextPatch,
		ValidateRun:      orch.ValidateRun,
		CompleteRun:      orch.CompleteRun,
		CancelRun:        orch.CancelRun,
		SupersedeRun:     orch.SupersedeRun,
		FailRun:          orch.FailRun,
	}
}

type moduleNoopRuntimeAdapter struct{}

func (moduleNoopRuntimeAdapter) ExecuteRuntimeStep(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	return runtimemodel.StepExecutionResult{Output: map[string]any{"completionState": "completed"}}, nil
}

type recordingBackgroundRoutine struct {
	asyncFunctionCalls int
	lastAsyncFunction  func()
	err                error
}

func (r *recordingBackgroundRoutine) AsyncTask(task magictask.Task) error {
	return r.AsyncFunction(task.Run)
}

func (r *recordingBackgroundRoutine) SyncTask(task magictask.Task) error {
	task.Run()
	return nil
}

func (r *recordingBackgroundRoutine) SyncTaskWithTimeOut(task magictask.Task, _ time.Duration) error {
	task.Run()
	return nil
}

func (r *recordingBackgroundRoutine) AsyncFunction(function func()) error {
	r.asyncFunctionCalls++
	r.lastAsyncFunction = function
	return r.err
}

func (r *recordingBackgroundRoutine) SyncFunction(function func()) error {
	function()
	return nil
}

func (r *recordingBackgroundRoutine) SyncFunctionWithTimeOut(function func(), _ time.Duration) error {
	function()
	return nil
}

func (r *recordingBackgroundRoutine) Timer(context.Context, magictask.Task, time.Duration, time.Duration) error {
	return nil
}

func (r *recordingBackgroundRoutine) Shutdown(context.Context) bool {
	return true
}
