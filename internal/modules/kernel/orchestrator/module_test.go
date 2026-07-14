package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	orchestratorservice "github.com/muidea/workorch/internal/modules/kernel/orchestrator/service"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if module.servicePtr != nil {
		t.Fatal("new module service pointer is non-nil before Configure")
	}

	deps := moduleContractDependencies()
	if err := module.Configure(deps); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.servicePtr == nil {
		t.Fatal("service pointer is nil after Configure")
	}
	if module.bizPtr == nil {
		t.Fatal("biz pointer is nil after Configure")
	}
	if module.bizPtr.RuntimeV2RepositoryConfigured() == false {
		t.Fatal("Runtime V2 repository is not configured")
	}
	if module.rawEventHub == nil {
		t.Fatal("command hub is nil after Configure")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstSubscriptionCount := len(module.rawTopics)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe orchestrator commands")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(module.rawTopics) != firstSubscriptionCount {
		t.Fatalf("second Start subscription count = %d, want %d", len(module.rawTopics), firstSubscriptionCount)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.servicePtr != nil ||
		module.bizPtr != nil ||
		module.rawEventHub != nil ||
		module.steps != nil ||
		module.backgroundRoutine != nil ||
		len(module.rawTopics) != 0 {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresCoreDependencies(t *testing.T) {
	base := moduleContractDependencies()
	tests := []struct {
		name string
		deps Dependencies
	}{
		{name: "missing steps", deps: func() Dependencies { d := base; d.Steps = nil; return d }()},
		{name: "missing tool calls", deps: func() Dependencies { d := base; d.ToolCalls = nil; return d }()},
		{name: "missing patches", deps: func() Dependencies { d := base; d.Patches = nil; return d }()},
		{name: "missing validation results", deps: func() Dependencies { d := base; d.ValidationResults = nil; return d }()},
		{name: "missing checkpoints", deps: func() Dependencies { d := base; d.RuntimeCheckpoints = nil; return d }()},
		{name: "missing evidence", deps: func() Dependencies { d := base; d.EvidenceRecords = nil; return d }()},
		{name: "missing coverage", deps: func() Dependencies { d := base; d.CoverageWorkItems = nil; return d }()},
		{name: "missing artifact continuations", deps: func() Dependencies { d := base; d.ArtifactContinuations = nil; return d }()},
		{name: "missing command hub", deps: func() Dependencies { d := base; d.CommandHub = nil; return d }()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}

func TestModuleStartRunCommandAcceptsStateAndSchedulesBackgroundDispatch(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-background-start")
	workspaceID := common.WorkspaceID("ws-module-background-start")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository())

	bg := &recordingBackgroundRoutine{}
	module := &Module{
		bizPtr:            orch,
		servicePtr:        orchestratorservice.New(orchestratorServiceDependencies(orch)),
		backgroundRoutine: bg,
	}

	_, err := module.executeMagicCommand(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommandStartRun,
		"test",
		orchestratorevents.CommandDestination,
		magicevent.NewHeader(),
		common.ContextWithRuntimeWorkspaceID(common.ContextWithRuntimeLLMProviderID(ctx, "provider-test"), workspaceID),
		orchestratorevents.StartRunCommand{RunID: runID},
	))
	if err != nil {
		t.Fatalf("start command err = %v", err)
	}
	if bg.asyncFunctionCalls != 1 || bg.lastAsyncFunction == nil {
		t.Fatalf("background calls = %d fn=%v, want one scheduled dispatch", bg.asyncFunctionCalls, bg.lastAsyncFunction != nil)
	}
	run, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	if run.State != common.RunStateActive {
		t.Fatalf("run state = %s, want running before background dispatch executes", run.State)
	}
	workspace, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("read workspace: %v", err)
	}
	if workspace.State != common.WorkspaceStateActive || workspace.ActiveRunID != runID {
		t.Fatalf("workspace = %+v, want active workspace with active run", workspace)
	}
}

func TestModuleResumeRunCommandSchedulesRuntimeV2Drive(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-background-resume")
	workspaceID := common.WorkspaceID("ws-module-background-resume")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository())

	bg := &recordingBackgroundRoutine{}
	module := &Module{
		bizPtr:            orch,
		servicePtr:        orchestratorservice.New(orchestratorServiceDependencies(orch)),
		backgroundRoutine: bg,
	}

	_, err := module.executeMagicCommand(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommandResumeRun,
		"test",
		orchestratorevents.CommandDestination,
		magicevent.NewHeader(),
		common.ContextWithRuntimeLLMProviderID(ctx, "provider-test"),
		orchestratorevents.ResumeRunCommand{
			WorkspaceID: workspaceID,
			RunID:       runID,
		},
	))
	if err != nil {
		t.Fatalf("resume command err = %v", err)
	}
	if bg.asyncFunctionCalls != 1 || bg.lastAsyncFunction == nil {
		t.Fatalf("background calls = %d fn=%v, want one scheduled resume", bg.asyncFunctionCalls, bg.lastAsyncFunction != nil)
	}
}

func TestModuleResumeRunCommandRejectsTerminalRunBeforeBackgroundDispatch(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-module-background-resume-terminal")
	workspaceID := common.WorkspaceID("ws-module-background-resume-terminal")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateFailed}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		memoryrepo.NewApprovalRepository())

	bg := &recordingBackgroundRoutine{}
	module := &Module{
		bizPtr:            orch,
		servicePtr:        orchestratorservice.New(orchestratorServiceDependencies(orch)),
		backgroundRoutine: bg,
	}

	_, err := module.executeMagicCommand(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommandResumeRun,
		"test",
		orchestratorevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		orchestratorevents.ResumeRunCommand{WorkspaceID: workspaceID, RunID: runID},
	))
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("resume terminal err = %v, want conflict", err)
	}
	if bg.asyncFunctionCalls != 0 {
		t.Fatalf("background calls = %d, want terminal run rejected before scheduling", bg.asyncFunctionCalls)
	}
}

func moduleContractDependencies() Dependencies {
	rawHub := magicevent.NewHub(32)
	return Dependencies{
		Steps:                 memoryrepo.NewStepRepository(),
		ToolCalls:             memoryrepo.NewToolCallRepository(),
		Patches:               memoryrepo.NewPatchRepository(),
		ValidationResults:     memoryrepo.NewValidationResultRepository(),
		RuntimeCheckpoints:    memoryrepo.NewRuntimeCheckpointRepository(),
		EvidenceRecords:       memoryrepo.NewEvidenceRecordRepository(),
		CoverageWorkItems:     memoryrepo.NewCoverageWorkItemRepository(),
		ArtifactContinuations: memoryrepo.NewArtifactContinuationRepository(),
		CommandHub:            rawHub,
	}
}

func orchestratorServiceDependencies(orch *orchestratorbiz.RuntimeOrchestrator) orchestratorservice.Dependencies {
	return orchestratorservice.Dependencies{
		BlockOnDecision: orch.BlockOnDecision,
		BlockOnApproval: orch.BlockOnApproval,
		CancelRun:       orch.CancelRun,
		SupersedeRun:    orch.SupersedeRun,
		FailRun:         orch.FailRun,
	}
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
