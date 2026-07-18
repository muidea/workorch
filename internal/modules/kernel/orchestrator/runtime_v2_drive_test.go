package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type panicAfterOwnerReceiptExecutor struct {
	delegate eventRuntimeV2OperationExecutor
}

type failOnceResultRepository struct {
	repository.ResultRepository
	remainingFailures int
}

type failOnceCASRuntimeV2Repository struct {
	delegate          runtimev2.RuntimeStateRepository
	remainingFailures int
}

type runtimeV2SaveCrashRepository struct {
	delegate runtimev2.RuntimeStateRepository
	match    func(runtimev2.RuntimeAggregate) bool
	after    bool
	fired    bool
	marker   string
}

func (r *runtimeV2SaveCrashRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimev2.RuntimeAggregate, error) {
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *runtimeV2SaveCrashRepository) Save(ctx context.Context, aggregate runtimev2.RuntimeAggregate, expectedRevision uint64) error {
	matched := !r.fired && r.match != nil && r.match(aggregate)
	if matched && !r.after {
		r.fired = true
		panic(r.marker)
	}
	if err := r.delegate.Save(ctx, aggregate, expectedRevision); err != nil {
		return err
	}
	if matched && r.after {
		r.fired = true
		panic(r.marker)
	}
	return nil
}

func (r *failOnceCASRuntimeV2Repository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimev2.RuntimeAggregate, error) {
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *failOnceCASRuntimeV2Repository) Save(ctx context.Context, aggregate runtimev2.RuntimeAggregate, expectedRevision uint64) error {
	if expectedRevision > 0 && r.remainingFailures > 0 {
		r.remainingFailures--
		return common.ErrConflict
	}
	return r.delegate.Save(ctx, aggregate, expectedRevision)
}

func (r *failOnceResultRepository) Create(ctx context.Context, item *resultmodel.Result) error {
	if r.remainingFailures > 0 {
		r.remainingFailures--
		return errors.New("injected terminal result delivery failure")
	}
	return r.ResultRepository.Create(ctx, item)
}

func (e panicAfterOwnerReceiptExecutor) ExecuteRuntimeOperation(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest) (runtimev2.RuntimeOperationExecutionResult, error) {
	_, err := e.delegate.ExecuteRuntimeOperation(ctx, request)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	panic("runtime-v2-test-crash-after-owner-receipt")
}

func TestDriveRuntimeV2FileBackedRunExecutesDurableFileOwnerAndRestarts(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)

	deliverable := filepath.Join(root, "workspace")
	internalDir := filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir,
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	candidateCount := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCount++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Candidate: runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "write-1", Kind: runtimecontract.StepKindFileEdit,
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
				Parameters: runtimeV2FileEditParameters("target.txt"),
			}}}}},
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	module := newRuntimeV2DriveTestModule(t, hub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	command := orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"}
	completed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	content, readErr := os.ReadFile(filepath.Join(deliverable, "target.txt"))
	if readErr != nil || string(content) != "new\n" {
		t.Fatalf("durable file content = %q err=%v", string(content), readErr)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || persisted.RuntimeState.SchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent || len(persisted.RuntimeState.OperationReceipts) != 1 {
		t.Fatalf("file-backed runtime state = %+v err=%v", persisted, err)
	}
	receipt := persisted.RuntimeState.OperationReceipts[0]
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || receipt.ObservedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable || !receipt.Changed || receipt.BeforeHash == receipt.AfterHash {
		t.Fatalf("durable owner receipt = %+v", receipt)
	}

	driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2)
	restarted := newRuntimeV2DriveTestModule(t, hub)
	driveObserver = magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restarted.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	replayed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil || replayed.StateRevision != completed.StateRevision || candidateCount != 1 {
		t.Fatalf("file-backed restart replay=%+v complete=%+v candidates=%d err=%v", replayed, completed, candidateCount, err)
	}
}

func TestDriveRuntimeV2FileBackedRestartRecoversCrashAfterOwnerReceipt(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)

	deliverable := filepath.Join(root, "workspace")
	internalDir := filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir,
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	candidateCount := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCount++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Candidate: runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "write-1", Kind: runtimecontract.StepKindFileEdit,
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
				Parameters: runtimeV2FileEditParameters("target.txt"),
			}}}}},
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	module := newRuntimeV2DriveTestModule(t, hub)
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := module.bizPtr.InitializeRuntimeV2FromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = module.prepareRuntimeV2Prerequisites(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	aggregate, err = module.requestAndSealRuntimeV2CandidateWithRetry(ctx, aggregate, prefix)
	if err != nil || aggregate.ActiveDispatch == nil {
		t.Fatalf("seal before crash aggregate=%+v err=%v", aggregate, err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "runtime-v2-test-crash-after-owner-receipt" {
				t.Fatalf("unexpected crash marker: %v", recovered)
			}
		}()
		_, _ = module.bizPtr.ExecuteRuntimeV2Dispatch(ctx, "ws-v2", "run-v2", panicAfterOwnerReceiptExecutor{delegate: eventRuntimeV2OperationExecutor{hub: hub}})
		t.Fatal("expected injected crash after owner receipt")
	}()

	content, readErr := os.ReadFile(filepath.Join(deliverable, "target.txt"))
	if readErr != nil || string(content) != "new\n" {
		t.Fatalf("side effect before crash content=%q err=%v", string(content), readErr)
	}
	crashedRun, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || crashedRun.RuntimeState == nil || crashedRun.RuntimeState.ActiveDispatch == nil {
		t.Fatalf("crashed persisted run=%+v err=%v", crashedRun, err)
	}
	if len(crashedRun.RuntimeState.OperationReceipts) != 1 || crashedRun.RuntimeState.EvidenceSequence != 0 {
		t.Fatalf("crash boundary receipts=%d evidenceSequence=%d", len(crashedRun.RuntimeState.OperationReceipts), crashedRun.RuntimeState.EvidenceSequence)
	}
	execution := crashedRun.RuntimeState.ActiveDispatch.OperationExecutions[0]
	if execution.State != runtimecontract.OperationExecutionOwnerCommitted || execution.Fence == 0 {
		t.Fatalf("crash boundary execution=%+v", execution)
	}

	restarted := newRuntimeV2DriveTestModule(t, hub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restarted.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Action != runtimecontract.RuntimeActionComplete || candidateCount != 1 {
		t.Fatalf("restart completion=%+v candidateCount=%d", completed, candidateCount)
	}
	completedRun, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || completedRun.RuntimeState == nil {
		t.Fatalf("completed persisted run=%+v err=%v", completedRun, err)
	}
	if len(completedRun.RuntimeState.OperationReceipts) != 1 || len(completedRun.RuntimeState.Facts.Evidence.Accepted) != 1 || completedRun.RuntimeState.EvidenceSequence != 1 {
		t.Fatalf("restart exactly-once receipts=%d accepted=%d evidenceSequence=%d", len(completedRun.RuntimeState.OperationReceipts), len(completedRun.RuntimeState.Facts.Evidence.Accepted), completedRun.RuntimeState.EvidenceSequence)
	}
	if completedRun.RuntimeState.OperationReceipts[0].ExecutionKey != execution.ExecutionKey || completedRun.RuntimeState.OperationReceipts[0].Fence != execution.Fence {
		t.Fatal(fmt.Sprintf("restart replaced owner receipt: before=%+v after=%+v", execution, completedRun.RuntimeState.OperationReceipts[0]))
	}
}

func TestRuntimeV2FileBackedClaimEvidenceAndDispatchCloseCrashMatrix(t *testing.T) {
	testCases := []struct {
		name       string
		validation bool
		after      bool
		match      func(runtimev2.RuntimeAggregate) bool
	}{
		{
			name: "claim_cas_before", validation: true,
			match: func(aggregate runtimev2.RuntimeAggregate) bool {
				return aggregate.ActiveDispatch != nil && len(aggregate.ActiveDispatch.OperationExecutions) == 1 &&
					aggregate.ActiveDispatch.OperationExecutions[0].State == runtimecontract.OperationExecutionClaimed && len(aggregate.OperationReceipts) == 0
			},
		},
		{
			name: "claim_cas_after_execute_before", validation: true, after: true,
			match: func(aggregate runtimev2.RuntimeAggregate) bool {
				return aggregate.ActiveDispatch != nil && len(aggregate.ActiveDispatch.OperationExecutions) == 1 &&
					aggregate.ActiveDispatch.OperationExecutions[0].State == runtimecontract.OperationExecutionClaimed && len(aggregate.OperationReceipts) == 0
			},
		},
		{
			name: "evidence_and_dispatch_close_cas_before", validation: false,
			match: func(aggregate runtimev2.RuntimeAggregate) bool { return aggregate.EvidenceSequence == 1 },
		},
		{
			name: "evidence_and_dispatch_close_cas_after", validation: false, after: true,
			match: func(aggregate runtimev2.RuntimeAggregate) bool { return aggregate.EvidenceSequence == 1 },
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			t.Cleanup(func() { hub.Terminate(ctx) })
			root := t.TempDir()
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			bootstrap := runtimeV2Bootstrap()
			if testCase.validation {
				bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileRead}
				bootstrap.Focuses[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
				bootstrap.Requirements = []runtimecontract.RuntimeRequirement{{
					Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.StepKindFileRead},
				}}
			}
			if err := runs.Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
				DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}
			runOwner := runmodule.New()
			if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runOwner.Stop(ctx) })
			startRuntimeV2ReceiptTestOwner(t, hub)

			deliverable := filepath.Join(root, "workspace")
			internalDir := filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("old\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
				result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
			})
			t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
			fileOwner := filemodule.New()
			if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := fileOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = fileOwner.Stop(ctx) })

			module := newRuntimeV2DriveTestModule(t, hub)
			run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := module.bizPtr.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err = module.prepareRuntimeV2Prerequisites(ctx, aggregate)
			if err != nil {
				t.Fatal(err)
			}
			prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
			if _, err := module.bizPtr.BeginRuntimeV2CandidateAttempt(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
				t.Fatal(err)
			}
			target := "target.txt"
			candidate := runtimecontract.RuntimeCandidate{}
			if testCase.validation {
				candidate.Validation = &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
					ID: "read-1", Kind: runtimecontract.StepKindFileRead, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
					Parameters: []runtimecontract.OperationParameter{{Name: "path", String: &target}},
				}}}}
			} else {
				candidate.StateChange = &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
					ID: "edit-1", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
					Parameters: runtimeV2FileEditParameters(target),
				}}}}
			}
			sealed, err := module.bizPtr.AcceptRuntimeV2CandidateAttempt(ctx, "ws-v2", "run-v2", 0, candidate)
			if err != nil || sealed.ActiveDispatch == nil {
				t.Fatalf("sealed aggregate=%+v err=%v", sealed, err)
			}
			baseRepository := runtimeV2StateRepository{hub: hub}
			crashRepository := &runtimeV2SaveCrashRepository{delegate: baseRepository, match: testCase.match, after: testCase.after, marker: testCase.name}
			crashingController, err := runtimev2.NewRuntimeController(crashRepository)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != testCase.name {
						t.Fatalf("unexpected crash marker: %v", recovered)
					}
				}()
				_, _ = crashingController.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", eventRuntimeV2OperationExecutor{hub: hub})
				t.Fatal("expected injected save crash")
			}()
			if !crashRepository.fired {
				t.Fatal("crash point was not reached")
			}

			restarted, err := runtimev2.NewRuntimeController(baseRepository)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Decision.Action != runtimecontract.RuntimeActionComplete {
				loaded, err = restarted.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", eventRuntimeV2OperationExecutor{hub: hub})
				if err != nil {
					t.Fatal(err)
				}
			}
			if loaded.Decision.Action != runtimecontract.RuntimeActionComplete || len(loaded.OperationReceipts) != 1 || len(loaded.Facts.Evidence.Accepted) != 1 || loaded.EvidenceSequence != 1 {
				t.Fatalf("restart exactly-once aggregate=%+v", loaded)
			}
			if testCase.validation && testCase.after && loaded.Facts.Recovery.OperationRecoveryAttempts != 1 {
				t.Fatalf("claimed safe recovery ledger=%+v", loaded.Facts.Recovery)
			}
			content, readErr := os.ReadFile(filepath.Join(deliverable, "target.txt"))
			want := "old\n"
			if !testCase.validation {
				want = "new\n"
			}
			if readErr != nil || string(content) != want {
				t.Fatalf("restart content=%q want=%q err=%v", string(content), want, readErr)
			}
		})
	}
}

func TestDriveRuntimeV2FileBackedTwoFocusStateValidationAssessmentAndRestart(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
	results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
	failingResults := &failOnceResultRepository{ResultRepository: results, remainingFailures: 1}
	artifacts := filerepository.NewArtifactRepository(filepath.Join(root, "artifacts.json"))
	decisionTickets := filerepository.NewDecisionTicketRepository(filepath.Join(root, "decision-tickets.json"))
	decisionResolutions := filerepository.NewDecisionResolutionRepository(filepath.Join(root, "decision-resolutions.json"))
	bootstrap := runtimeV2Bootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindFileRead, runtimecontract.RuntimeBlockKindDecisionRequest}
	bootstrap.Focuses = []runtimecontract.CoverageFocus{
		{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, TargetRefs: []string{"focus-1.txt"}, RequireAssessment: true},
		{ID: "focus-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, TargetRefs: []string{".env"}, RequireAssessment: true},
	}
	bootstrap.CoverageItems = []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1"}, {ID: "cwi-2"}}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindPatchApply}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.StepKindFileRead}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleDecision, CapabilityClass: runtimecontract.RuntimeCapabilityClassGovernance, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindDecisionRequest}},
	}
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)
	decisionOwner := decisionmodule.New()
	if err := decisionOwner.Configure(decisionmodule.Dependencies{Tickets: decisionTickets, Resolutions: decisionResolutions, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := decisionOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = decisionOwner.Stop(ctx) })

	deliverable := filepath.Join(root, "workspace")
	internalDir := filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deliverable, "focus-1.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir,
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	resultOwner := resultmodule.New()
	if err := resultOwner.Configure(resultmodule.Dependencies{Results: failingResults, Artifacts: artifacts, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resultOwner.Stop(ctx) })

	candidateCount := 0
	providerTransientInjected := false
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCount++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		focus := command.Decision.Focus
		coverageID := focus.CoverageWorkItemIDs[0]
		target := focus.TargetRefs[0]
		response := orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction}
		if focus.ID == "focus-2" && command.Decision.Action == runtimecontract.RuntimeActionProduceStateChange && !providerTransientInjected {
			providerTransientInjected = true
			response.Failure = &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, Reason: "injected provider transport interruption"}
			result.Set(response, nil)
			return
		}
		switch command.Decision.Action {
		case runtimecontract.RuntimeActionProduceStateChange:
			if focus.ID == "focus-2" {
				patchType, content := "file_write", "TOKEN=granted\n"
				response.Candidate = runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
					ID: "patch-" + focus.ID, Kind: runtimecontract.RuntimeBlockKindPatchApply,
					CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target},
					Parameters: []runtimecontract.OperationParameter{{Name: "targetRef", String: &target}, {Name: "patchType", String: &patchType}, {Name: "content", String: &content}},
				}}}}}
			} else {
				oldText, newText := "old\n", "new\n"
				response.Candidate = runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
					ID: "edit-" + focus.ID, Kind: runtimecontract.StepKindFileEdit,
					CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target},
					Parameters: []runtimecontract.OperationParameter{{Name: "path", String: &target}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}},
				}}}}}
			}
		case runtimecontract.RuntimeActionProduceValidation:
			response.Candidate = runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "read-" + focus.ID, Kind: runtimecontract.StepKindFileRead,
				CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target},
				Parameters: []runtimecontract.OperationParameter{{Name: "path", String: &target}},
			}}}}}
		case runtimecontract.RuntimeActionProduceAssessment:
			refs := make([]string, 0)
			for _, evidence := range command.CandidateContext.Evidence.Accepted {
				for _, evidenceCoverageID := range evidence.CoverageWorkItemIDs {
					if evidenceCoverageID == coverageID {
						refs = append(refs, string(evidence.ID))
						break
					}
				}
			}
			response.Candidate = runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
				FocusID: focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID},
				Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: focus.ID + " satisfied",
				Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: coverageID, Satisfied: true}}, EvidenceRefs: refs,
			}}
		default:
			t.Errorf("unexpected candidate action %s", command.Decision.Action)
		}
		result.Set(response, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	moduleBiz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	casRepository := &failOnceCASRuntimeV2Repository{delegate: runtimeV2StateRepository{hub: hub}, remainingFailures: 1}
	if err := moduleBiz.ConfigureRuntimeV2Repository(casRepository); err != nil {
		t.Fatal(err)
	}
	module := &Module{bizPtr: moduleBiz, rawEventHub: hub}
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	command := orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"}
	if err := module.scheduleRuntimeV2Drive(ctx, "ws-v2", "run-v2", "cas-conflict-injection"); err != nil {
		t.Fatal(err)
	}
	casRecovered, err := module.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil || casRecovered.Facts.Recovery.CASReloadAttempts != 1 || casRecovered.Facts.Failure != nil {
		t.Fatalf("CAS recovery aggregate=%+v err=%v", casRecovered, err)
	}
	waiting, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if waiting.Disposition != orchestratorevents.RuntimeV2DriveDispositionGovernedWait || waiting.BlockingTicketID == "" || candidateCount != 5 {
		t.Fatalf("two-focus governance wait=%+v candidates=%d", waiting, candidateCount)
	}
	governed, err := module.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil || governed.ActiveDispatch == nil || len(governed.OperationGovernanceBindings) != 1 {
		t.Fatalf("governed aggregate=%+v err=%v", governed, err)
	}
	binding := governed.OperationGovernanceBindings[0]
	request := governed.GovernanceRequests[len(governed.GovernanceRequests)-1]
	staleResolution := runtimecontract.RuntimeGovernanceResolution{
		ResolutionID: "resolution-focus-2-stale", TicketID: request.TicketID, WorkspaceID: "ws-v2", RunID: "run-v2",
		AuthoritySnapshotID: "stale-governance-authority", DispatchID: request.DispatchID,
		DependencyKind: request.DependencyKind, SubjectRef: request.SubjectRef,
		Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "stale approval",
	}
	staleCommit, err := module.bizPtr.CommitRuntimeV2GovernanceResolution(ctx, staleResolution)
	if err != nil || staleCommit.Disposition != runtimev2.CommitDispositionStale || staleCommit.Aggregate.ActiveDispatch == nil || len(staleCommit.Aggregate.Diagnostics) == 0 {
		t.Fatalf("stale governance commit=%+v err=%v", staleCommit, err)
	}
	resolution := runtimecontract.RuntimeGovernanceResolution{
		ResolutionID: "resolution-focus-2", TicketID: request.TicketID, WorkspaceID: "ws-v2", RunID: "run-v2",
		AuthoritySnapshotID: request.AuthoritySnapshotID, DispatchID: request.DispatchID,
		DependencyKind: request.DependencyKind, SubjectRef: request.SubjectRef,
		Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "file patch approved",
	}
	grantCrashRepository := &runtimeV2SaveCrashRepository{
		delegate: runtimeV2StateRepository{hub: hub}, after: true, marker: "governance-grant-cas-after",
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			return aggregate.ActiveDispatch != nil && len(aggregate.OperationGovernanceBindings) == 1 &&
				aggregate.OperationGovernanceBindings[0].State == runtimecontract.RuntimeOperationGovernanceBindingGranted
		},
	}
	grantCrashBiz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := grantCrashBiz.ConfigureRuntimeV2Repository(grantCrashRepository); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "governance-grant-cas-after" {
				t.Fatalf("unexpected governance grant crash marker: %v", recovered)
			}
		}()
		_, _ = grantCrashBiz.CommitRuntimeV2GovernanceResolution(ctx, resolution)
		t.Fatal("expected crash after governance grant CAS")
	}()
	if !grantCrashRepository.fired {
		t.Fatal("governance grant crash point was not reached")
	}
	continued, err := module.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil || continued.ActiveDispatch == nil {
		t.Fatalf("persisted governance continuation=%+v err=%v", continued, err)
	}
	resumeExecution := continued.ActiveDispatch.OperationExecutions[0]
	if resumeExecution.ExecutionKey != binding.ExecutionKey || resumeExecution.Fence != binding.Fence+1 || resumeExecution.State != runtimecontract.OperationExecutionClaimed {
		t.Fatalf("governance resume execution=%+v binding=%+v", resumeExecution, binding)
	}
	duplicateContinuation, err := module.bizPtr.CommitRuntimeV2GovernanceResolution(ctx, resolution)
	if err != nil || duplicateContinuation.Disposition != runtimev2.CommitDispositionDuplicate || duplicateContinuation.Aggregate.ActiveDispatch == nil || duplicateContinuation.Aggregate.ActiveDispatch.ID != continued.ActiveDispatch.ID {
		t.Fatalf("duplicate governance continuation=%+v err=%v", duplicateContinuation, err)
	}
	completed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Action != runtimecontract.RuntimeActionComplete || completed.Terminal == nil || candidateCount != 7 {
		t.Fatalf("two-focus completion=%+v candidates=%d", completed, candidateCount)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil {
		t.Fatalf("persisted two-focus run=%+v err=%v", persisted, err)
	}
	if len(persisted.RuntimeState.OperationReceipts) != 5 || len(persisted.RuntimeState.Facts.Assessments) != 2 {
		t.Fatalf("two-focus receipts=%d assessments=%d", len(persisted.RuntimeState.OperationReceipts), len(persisted.RuntimeState.Facts.Assessments))
	}
	if persisted.RuntimeState.Facts.Recovery.ProviderTransportAttempts != 1 {
		t.Fatalf("provider transport recovery ledger=%+v", persisted.RuntimeState.Facts.Recovery)
	}
	for _, item := range persisted.RuntimeState.Facts.Coverage {
		if item.State != runtimecontract.RuntimeCoverageItemStateSatisfied {
			t.Fatalf("coverage item not satisfied: %+v", item)
		}
	}
	content, readErr := os.ReadFile(filepath.Join(deliverable, "focus-1.txt"))
	if readErr != nil || string(content) != "new\n" {
		t.Fatalf("focus-1 content=%q err=%v", string(content), readErr)
	}
	content, readErr = os.ReadFile(filepath.Join(deliverable, ".env"))
	if readErr != nil || string(content) != "TOKEN=granted\n" {
		t.Fatalf("focus-2 governed content=%q err=%v", string(content), readErr)
	}
	if _, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(persisted.RuntimeState.OperationReceipts, binding.ExecutionKey, binding.Fence); !ok {
		t.Fatal("governance receipt was not preserved")
	}
	if receipt, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(persisted.RuntimeState.OperationReceipts, binding.ExecutionKey, binding.Fence+1); !ok || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
		t.Fatalf("governance resume receipt=%+v found=%v", receipt, ok)
	}
	driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2)
	restarted := newRuntimeV2DriveTestModule(t, hub)
	driveObserver = magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restarted.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	replayed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil || replayed.StateRevision != completed.StateRevision || candidateCount != 7 {
		t.Fatalf("two-focus restart=%+v completed=%+v candidates=%d err=%v", replayed, completed, candidateCount, err)
	}
	aggregate, err := restarted.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	deliveryBiz := orchestratorbiz.NewRuntimeOrchestrator(runs, workspaces, nil, nil)
	deliveryModule := &Module{bizPtr: deliveryBiz, rawEventHub: hub}
	if err := deliveryModule.deliverRuntimeV2Terminal(ctx, aggregate); err == nil {
		t.Fatal("first terminal delivery must fail before Result persistence")
	}
	if _, err := results.Get(ctx, "ws-v2/run-v2/result/dod-assessment"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("failed terminal delivery unexpectedly persisted Result: %v", err)
	}
	recoveryAggregate, consumed, err := restarted.bizPtr.ConsumeRuntimeV2RecoveryBudget(ctx, "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetTerminalDelivery)
	if err != nil || !consumed || recoveryAggregate.Facts.Recovery.TerminalDeliveryAttempts != 1 {
		t.Fatalf("terminal delivery recovery budget aggregate=%+v consumed=%v err=%v", recoveryAggregate, consumed, err)
	}
	if err := resultOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	restartedResultOwner := resultmodule.New()
	if err := restartedResultOwner.Configure(resultmodule.Dependencies{Results: results, Artifacts: artifacts, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedResultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedResultOwner.Stop(ctx) })
	if err := deliveryModule.deliverRuntimeV2Terminal(ctx, recoveryAggregate); err != nil {
		t.Fatal(err)
	}
	resultItem, err := results.Get(ctx, "ws-v2/run-v2/result/dod-assessment")
	if err != nil || resultItem.State != common.ResultStateAccepted || resultItem.RuntimeCloseout == nil || len(resultItem.RuntimeCloseout.SatisfiedCoverageWorkItemIDs) != 2 {
		t.Fatalf("two-focus result=%+v err=%v", resultItem, err)
	}
	terminalRun, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || terminalRun.State != common.RunStateCompleted {
		t.Fatalf("two-focus terminal run=%+v err=%v", terminalRun, err)
	}
	releasedWorkspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil || releasedWorkspace.ActiveRunID != "" {
		t.Fatalf("two-focus released workspace=%+v err=%v", releasedWorkspace, err)
	}
}

func TestDriveRuntimeV2CompletesTypedPathAndIsRestartIdempotent(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir(),
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	fileObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	fileObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
		response, err := orchestratorevents.ResolveRuntimeCapabilities(command, runtimecontract.RuntimeModuleFile, runtimecontract.StepKindFileEdit)
		if err != nil {
			t.Errorf("resolve capabilities: %v", err)
			return
		}
		result.Set(response, nil)
	})
	operationCount := 0
	fileObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation, func(event magicevent.Event, result magicevent.Result) {
		operationCount++
		command := event.Data().(orchestratorevents.ExecuteFileEditOperationCommand)
		receipt := orchestratorevents.NewRuntimeOperationReceipt(command.Execution, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusAccepted, command.TargetRefs, runtimecontract.RuntimeOperationEvidence{Summary: "状态变更已完成"}, nil, nil)
		committed, err := orchestratorevents.CommitPersistedRuntimeOperationReceipt(event.Context(), hub, runtimecontract.RuntimeModuleFile, command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
		if err != nil {
			t.Errorf("commit owner receipt: %v", err)
			return
		}
		result.Set(orchestratorevents.ExecuteFileEditOperationResult{
			Receipt: committed,
			EventID: "event-" + command.OperationID, OperationID: command.OperationID,
			EvidenceKind: command.EvidenceKind, Status: runtimecontract.RuntimeOperationExecutionStatusAccepted,
			SourceRef: "workorch://runtime-operations/" + command.DispatchID + "/" + command.OperationID,
			Evidence:  runtimecontract.RuntimeOperationEvidence{Summary: "状态变更已完成"},
		}, nil)
	})
	t.Cleanup(func() {
		fileObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation)
		fileObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
	})

	candidateCount := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCount++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Candidate: runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "write-1", Kind: runtimecontract.StepKindFileEdit,
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
				Parameters: runtimeV2FileEditParameters("target.txt"),
			}}}}},
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	module := newRuntimeV2DriveTestModule(t, hub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)

	command := orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"}
	completed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal || completed.Action != runtimecontract.RuntimeActionComplete || completed.Terminal == nil {
		t.Fatalf("completed drive result = %+v", completed)
	}
	if candidateCount != 1 || operationCount != 1 {
		t.Fatalf("candidateCount=%d operationCount=%d, want 1/1", candidateCount, operationCount)
	}

	replayed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.StateRevision != completed.StateRevision || replayed.AuthoritySnapshotID != completed.AuthoritySnapshotID || candidateCount != 1 || operationCount != 1 {
		t.Fatalf("replayed=%+v completed=%+v counts=%d/%d", replayed, completed, candidateCount, operationCount)
	}

	driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2)
	restarted := newRuntimeV2DriveTestModule(t, hub)
	driveObserver = magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restarted.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	recovered, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.StateRevision != completed.StateRevision || recovered.AuthoritySnapshotID != completed.AuthoritySnapshotID || candidateCount != 1 || operationCount != 1 {
		t.Fatalf("recovered=%+v completed=%+v counts=%d/%d", recovered, completed, candidateCount, operationCount)
	}
}

func TestDriveRuntimeV2ReturnsPersistedGovernedWait(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	bootstrap := runtimeV2Bootstrap()
	bootstrap.Definition.CapabilityContract = append(bootstrap.Definition.CapabilityContract, runtimecontract.RuntimeBlockKindDecisionRequest)
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	decisionOwner := decisionmodule.New()
	if err := decisionOwner.Configure(decisionmodule.Dependencies{
		Tickets: memory.NewDecisionTicketRepository(), Resolutions: memory.NewDecisionResolutionRepository(), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := decisionOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = decisionOwner.Stop(ctx) })

	module := newRuntimeV2DriveTestModule(t, hub)
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceDependency, Signature: "external-approval", Message: "需要外部裁决",
	}
	if _, err := module.bizPtr.InitializeRuntimeV2(ctx, facts, bootstrap.Definition, nil); err != nil {
		t.Fatal(err)
	}

	requestCount := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requestCount++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Candidate: runtimecontract.RuntimeCandidate{Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
				Kind: "external-approval", SubjectRef: "subject-1", Reason: "需要外部裁决",
			}}},
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	command := orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"}
	waiting, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if waiting.Disposition != orchestratorevents.RuntimeV2DriveDispositionGovernedWait || waiting.Action != runtimecontract.RuntimeActionRequestGovernance || waiting.BlockingTicketID == "" || requestCount != 0 {
		t.Fatalf("waiting=%+v requestCount=%d", waiting, requestCount)
	}
	replayed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.StateRevision != waiting.StateRevision || replayed.BlockingTicketID != waiting.BlockingTicketID || requestCount != 0 {
		t.Fatalf("replayed=%+v waiting=%+v requestCount=%d", replayed, waiting, requestCount)
	}
}

func TestRuntimeV2ProviderAuthenticationCreatesConfigurationDependency(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Focus:   runtimecontract.CoverageFocus{ID: "focus-1"},
		Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassProviderAuthentication, Message: "invalid provider credential"},
		Resources: runtimecontract.RuntimeResourceFacts{ProviderCeiling: &runtimecontract.ProviderCeiling{
			Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"},
		}},
	}
	dependency := runtimeV2GovernanceDependency(runtimev2.RuntimeAggregate{Facts: facts})
	if dependency.Kind != string(runtimecontract.GovernanceConfiguration) || dependency.SubjectRef != "ai-proxy/model-1" || dependency.Reason != "invalid provider credential" {
		t.Fatalf("configuration dependency = %+v", dependency)
	}
}

func TestRuntimeV2GovernanceDependencyPreservesSuspendedOperationBinding(t *testing.T) {
	aggregate := runtimev2.RuntimeAggregate{
		Facts: runtimecontract.RuntimeFacts{
			Focus: runtimecontract.CoverageFocus{ID: "focus-1"},
			Failure: &runtimecontract.FailureFact{
				Class: runtimecontract.FailureClassGovernanceRequired, Message: "approval required",
			},
		},
		OperationGovernanceBindings: []runtimecontract.RuntimeOperationGovernanceBinding{{
			ExecutionKey: "exec-1", Fence: 1, DependencyKind: string(runtimecontract.GovernanceApproval), SubjectRef: ".env",
			State: runtimecontract.RuntimeOperationGovernanceBindingSuspended,
		}},
	}
	dependency := runtimeV2GovernanceDependency(aggregate)
	if dependency.Kind != string(runtimecontract.GovernanceApproval) || dependency.SubjectRef != ".env" || dependency.Reason != "approval required" {
		t.Fatalf("operation governance dependency = %+v", dependency)
	}
}

func TestDriveRuntimeV2CandidateFailureStopsAfterSameContractRetry(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	module := newRuntimeV2DriveTestModule(t, hub)
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
	if _, err := module.bizPtr.InitializeRuntimeV2(ctx, facts, bootstrap.Definition, nil); err != nil {
		t.Fatal(err)
	}

	requestCount := 0
	var requests []orchestratorevents.RequestRuntimeCandidateCommand
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requestCount++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		requests = append(requests, command)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Failure: &runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassProvider, Reason: "provider unavailable",
			},
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })

	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 || completed.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal ||
		completed.Action != runtimecontract.RuntimeActionFail || completed.Terminal == nil || completed.Terminal.Code != "candidate_retry_exhausted" {
		t.Fatalf("completed=%+v requestCount=%d, want bounded terminal after two requests", completed, requestCount)
	}
	if len(requests) != 2 || requests[0].PreviousFailure != nil || requests[1].PreviousFailure == nil ||
		requests[1].PreviousFailure.Class != runtimecontract.RuntimeCandidateFailureClassProvider || requests[1].PreviousFailure.Reason != "provider unavailable" {
		t.Fatalf("candidate retry did not carry the persisted first failure: %+v", requests)
	}
}

func newRuntimeV2DriveTestModule(t *testing.T, hub magicevent.Hub) *Module {
	t.Helper()
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	return &Module{bizPtr: biz, rawEventHub: hub}
}

func TestRuntimeV2EnsureCandidateFeasibleBlocksProviderWithoutKinds(t *testing.T) {
	aggregate := runtimev2.RuntimeAggregate{
		Decision: runtimecontract.RuntimeDecision{
			Action: runtimecontract.RuntimeActionProduceValidation,
			RequiredOutput: runtimecontract.RequiredOutputContract{
				Kind:                runtimecontract.RequiredOutputKindValidation,
				AcceptedKinds:       []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			},
		},
		Facts: runtimecontract.RuntimeFacts{
			// file/edit is pure durable — kind-level validation feasibility must exclude it.
			Capabilities: runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.StepKindFileEdit}},
		},
	}
	if err := runtimeV2EnsureCandidateFeasible(aggregate); err == nil || !strings.Contains(err.Error(), "no feasible operation kinds") {
		t.Fatalf("expected infeasible validation, got %v", err)
	}
	aggregate.Facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	if err := runtimeV2EnsureCandidateFeasible(aggregate); err != nil {
		t.Fatalf("managed validation should be feasible: %v", err)
	}
}

func TestRuntimeV2DecisionPersistsFeasibilityWithoutCallingProvider(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-v2", RunID: "run-v2", DefinitionRevision: "definition-v2", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus:    runtimecontract.CoverageFocus{ID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-v2", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		// Pure durable kind in ValidationKinds proves the pre-provider feasibility gate.
		// Dual-use shell/command is intentionally feasible at kind level under RVC-R6.
		Capabilities: runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.StepKindFileEdit}},
	}
	decision, err := runtimev2.ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	repository := &runtimeV2DriveRepository{aggregate: runtimev2.RuntimeAggregate{StateRevision: 1, Definition: runtimeV2Definition(), Facts: facts.Normalized(), Decision: decision}}
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(repository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	providerCalls := 0
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providerCalls++ })
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	module := &Module{bizPtr: biz, rawEventHub: hub}
	next, err := module.driveRuntimeV2Decision(context.Background(), repository.aggregate, "unused-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || next.Facts.Feasibility == nil || next.Decision.Action != runtimecontract.RuntimeActionFail || next.Decision.Terminal == nil || next.Decision.Terminal.Code != "capability_unavailable" {
		t.Fatalf("feasibility transition = %+v providerCalls=%d", next, providerCalls)
	}
}

type runtimeV2DriveRepository struct {
	aggregate runtimev2.RuntimeAggregate
}

func (r *runtimeV2DriveRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimev2.RuntimeAggregate, error) {
	if r == nil || r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return runtimev2.RuntimeAggregate{}, common.ErrNotFound
	}
	return r.aggregate, nil
}

func (r *runtimeV2DriveRepository) Save(_ context.Context, aggregate runtimev2.RuntimeAggregate, expectedRevision uint64) error {
	if r == nil || r.aggregate.StateRevision != expectedRevision {
		return common.ErrConflict
	}
	r.aggregate = aggregate
	return nil
}
