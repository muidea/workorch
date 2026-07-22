package orchestrator

// Owner Biz drive tests.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	gitmodule "github.com/muidea/workorch/internal/modules/blocks/git"
	gitbiz "github.com/muidea/workorch/internal/modules/blocks/git/biz"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	shellmodule "github.com/muidea/workorch/internal/modules/blocks/shell"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	workspacemodule "github.com/muidea/workorch/internal/modules/blocks/workspace"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
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

// panicAfterOwnerReceiptCommitErrorExecutor models process loss after a
// capability owner applied its durable effect but before the owner receipt CAS
// was committed. The nested EventHub error is intentionally not converted into
// an outcome_unknown transition: the restart must use only the sealed proof.
type panicAfterOwnerReceiptCommitErrorExecutor struct {
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

// inMemoryRuntimeV2StateRepository keeps the candidate/stale mainline test
// independent of the Run owner. Production recovery is covered by the
// file-backed fixtures; this direct repository only proves the controller CAS
// transition owned by Orchestrator.
type inMemoryRuntimeV2StateRepository struct {
	aggregate *runtimev2.RuntimeAggregate
}

func (r *inMemoryRuntimeV2StateRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimev2.RuntimeAggregate, error) {
	if r == nil || r.aggregate == nil || r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return runtimev2.RuntimeAggregate{}, common.ErrNotFound
	}
	return *r.aggregate, nil
}

func (r *inMemoryRuntimeV2StateRepository) Save(_ context.Context, aggregate runtimev2.RuntimeAggregate, expectedRevision uint64) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if r.aggregate != nil && r.aggregate.StateRevision != expectedRevision {
		return common.ErrConflict
	}
	copy := aggregate
	r.aggregate = &copy
	return nil
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

func (e panicAfterOwnerReceiptCommitErrorExecutor) ExecuteRuntimeOperation(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest) (runtimev2.RuntimeOperationExecutionResult, error) {
	result, err := e.delegate.ExecuteRuntimeOperation(ctx, request)
	if err != nil {
		panic("runtime-v2-test-crash-after-owner-receipt-commit-error")
	}
	return result, nil
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

// R3-3 / H5-C: each subtest interrupts the same owner-grounded sequence both
// immediately before and immediately after one Run-owner CAS. A fresh
// Orchestrator Biz then has no choice but to load that file-backed aggregate
// and continue through the regular typed EventHub owners. Target selection is
// intentionally the File owner's bounded complete workspace scope; neither
// the fixture nor the Orchestrator synthesizes an implementation path from a
// path hint.
func TestRuntimeV2FileBackedGroundedTargetStateSequenceRestartsAtCommittedBoundaries(t *testing.T) {
	boundaries := []struct {
		name  string
		match func(runtimev2.RuntimeAggregate) bool
	}{
		{
			name: "target_selection",
			match: func(aggregate runtimev2.RuntimeAggregate) bool {
				return aggregate.Facts.Resources.TargetSelection != nil
			},
		},
		{
			name: "candidate_proposal",
			match: func(aggregate runtimev2.RuntimeAggregate) bool {
				for _, attempt := range aggregate.CandidateAttempts {
					if attempt.Proposal != nil && aggregate.ActiveDispatch == nil {
						return true
					}
				}
				return false
			},
		},
		{
			name: "receipt_and_evidence",
			match: func(aggregate runtimev2.RuntimeAggregate) bool {
				return len(aggregate.OperationReceipts) == 1 && len(aggregate.Facts.Evidence.Accepted) == 1
			},
		},
		{
			name: "assessment_terminal",
			match: func(aggregate runtimev2.RuntimeAggregate) bool {
				return aggregate.Decision.Action == runtimecontract.RuntimeActionComplete && aggregate.Decision.Terminal != nil
			},
		},
	}
	for _, tc := range boundaries {
		for _, after := range []bool{false, true} {
			phase := "before_commit"
			if after {
				phase = "after_commit_response_loss"
			}
			t.Run(tc.name+"_"+phase, func(t *testing.T) {
				ctx := context.Background()
				hub := magicevent.NewHub(32)
				t.Cleanup(func() { hub.Terminate(ctx) })
				root := t.TempDir()
				deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
				if err := os.MkdirAll(deliverable, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(internalDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(deliverable, "implementation.txt"), []byte("before\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(deliverable, "support.txt"), []byte("support\n"), 0o644); err != nil {
					t.Fatal(err)
				}

				bootstrap := runtimeV2GroundedTargetSequenceBootstrap()
				bootstrap.Focuses[0].TargetSelectorRef = "file/workspace_deliverable"
				runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
				workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
				results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
				if err := runs.Create(ctx, &runmodel.Run{
					ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
					DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
				}); err != nil {
					t.Fatal(err)
				}
				if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}); err != nil {
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
				workspaceOwner := workspacemodule.New()
				if err := workspaceOwner.Configure(workspacemodule.Dependencies{
					Workspaces: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(),
					Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub,
				}); err != nil {
					t.Fatal(err)
				}
				if err := workspaceOwner.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
				resultOwner := resultmodule.New()
				if err := resultOwner.Configure(resultmodule.Dependencies{Results: results, Artifacts: memory.NewArtifactRepository(), EventHub: hub}); err != nil {
					t.Fatal(err)
				}
				if err := resultOwner.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = resultOwner.Stop(ctx) })
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
					t.Fatal(err)
				}
				if err := fileOwner.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
				startRuntimeV2ReceiptTestOwner(t, hub)

				providerCalls := 0
				llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
				llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
					providerCalls++
					command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
					candidate, err := runtimeV2GroundedTargetSequenceCandidate(command)
					if err != nil {
						t.Errorf("grounded sequence candidate: %v", err)
						result.Set(orchestratorevents.RequestRuntimeCandidateResult{
							Transaction: command.Transaction,
							Failure:     &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassInternal, Reason: err.Error()},
						}, nil)
						return
					}
					result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: candidate}, nil)
				})
				t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

				crashMarker := "grounded-sequence-" + tc.name + "-" + phase
				crashRepository := &runtimeV2SaveCrashRepository{
					delegate: runtimeV2StateRepository{hub: hub}, match: tc.match, after: after, marker: crashMarker,
				}
				crashingModule := newRuntimeV2GroundedTargetSequenceModule(t, hub, crashRepository)
				func() {
					defer func() {
						if recovered := recover(); recovered != crashMarker {
							t.Fatalf("unexpected crash marker: %v", recovered)
						}
					}()
					if err := crashingModule.scheduleRuntimeV2Drive(ctx, "ws-v2", "run-v2", crashMarker); err != nil {
						t.Fatalf("drive before crash: %v", err)
					}
					t.Fatalf("expected crash %s %s CAS", phase, tc.name)
				}()
				if !crashRepository.fired {
					t.Fatalf("crash boundary %s was not reached", tc.name)
				}
				crashed, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				if err != nil || crashed.RuntimeState == nil {
					t.Fatalf("crashed aggregate missing from Run owner: run=%+v err=%v", crashed, err)
				}
				crashedAggregate := runtimeAggregateFromRecord(*crashed.RuntimeState)
				if after && !tc.match(crashedAggregate) {
					t.Fatalf("committed %s boundary is absent after response loss: %+v", tc.name, crashedAggregate)
				}
				if !after && tc.match(crashedAggregate) {
					t.Fatalf("uncommitted %s boundary leaked into restarted aggregate: %+v", tc.name, crashedAggregate)
				}

				restarted := newRuntimeV2GroundedTargetSequenceModule(t, hub, runtimeV2StateRepository{hub: hub})
				if err := restarted.scheduleRuntimeV2Drive(ctx, "ws-v2", "run-v2", "restart:"+tc.name+":"+phase); err != nil {
					t.Fatalf("restart drive: %v", err)
				}
				persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				if err != nil || persisted.RuntimeState == nil || persisted.State != common.RunStateCompleted {
					t.Fatalf("restart terminal run=%+v state=%+v err=%v", persisted, persisted.RuntimeState, err)
				}
				state := persisted.RuntimeState
				if state.Decision.Action != runtimecontract.RuntimeActionComplete || state.Decision.Terminal == nil ||
					state.Facts.Resources.TargetSelection == nil || state.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionScope ||
					len(state.Facts.Resources.TargetSnapshots) != 0 ||
					len(state.OperationReceipts) != 2 || len(state.Facts.Evidence.Accepted) != 2 || len(state.Facts.Assessments) != 1 {
					t.Fatalf("restart state sequence did not converge exactly once: %+v", state)
				}
				wantProviderCalls := 3
				// A proposal CAS-before crash is the one permitted retry point: no
				// proposal exists in the restarted aggregate, so SM-3 has to request
				// the state-change candidate again. Every other interrupted boundary
				// has a committed selection/proposal/dispatch or receipt to consume.
				if tc.name == "candidate_proposal" && !after {
					wantProviderCalls = 4
				}
				if providerCalls != wantProviderCalls {
					t.Fatalf("provider calls=%d, want %d", providerCalls, wantProviderCalls)
				}
				if content, err := os.ReadFile(filepath.Join(deliverable, "implementation.txt")); err != nil || string(content) != "after\n" {
					t.Fatalf("target durable effect=%q err=%v", content, err)
				}
				items, err := results.ListByRun(ctx, "run-v2")
				if err != nil || len(items) != 1 || items[0].State != common.ResultStateAccepted {
					t.Fatalf("terminal projection Result=%+v err=%v", items, err)
				}
				workspace, err := workspaces.Get(ctx, "ws-v2")
				if err != nil || workspace.ActiveRunID != "" {
					t.Fatalf("terminal projection Workspace=%+v err=%v", workspace, err)
				}
			})
		}
	}
}

// TestRuntimeV2FileBackedAmbiguousTargetSelectionFailsWithoutProvider keeps
// target grounding on the production typed-owner path. A profile may declare
// File's neutral unique-entry selector, but a multi-file workspace is an
// explicit ambiguous observation, not permission for the Orchestrator, LLM,
// or validation entry to choose an implementation target.
func TestRuntimeV2FileBackedAmbiguousTargetSelectionFailsWithoutProvider(t *testing.T) {
	t.Run("state_change", func(t *testing.T) {
		testRuntimeV2FileBackedUnresolvedTargetSelectionFailsWithoutProvider(t, runtimeV2GroundedTargetSequenceBootstrap(), runtimecontract.RuntimeTargetSelectionAmbiguous, map[string]string{"validation-entry.go": "package fixture\n", "implementation.go": "package fixture\n"})
	})
	t.Run("direct_validation", func(t *testing.T) {
		testRuntimeV2FileBackedUnresolvedTargetSelectionFailsWithoutProvider(t, runtimeV2GroundedTargetValidationBootstrap(), runtimecontract.RuntimeTargetSelectionAmbiguous, map[string]string{"validation-entry.go": "package fixture\n", "implementation.go": "package fixture\n"})
	})
}

// The legacy unique-entry selector reports an empty directory as absent. The
// production workspace-deliverable selector has a separate owner-grounded
// creation path below; neither path requests TUI/Web target input.
func TestRuntimeV2FileBackedAbsentTargetSelectionFailsWithoutProvider(t *testing.T) {
	testRuntimeV2FileBackedUnresolvedTargetSelectionFailsWithoutProvider(t, runtimeV2GroundedTargetSequenceBootstrap(), runtimecontract.RuntimeTargetSelectionAbsent, nil)
}

func TestRuntimeV2FileBackedEmptyWorkspaceCreationSelectionContinuesThroughCandidate(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bootstrap := runtimeV2GroundedTargetSequenceBootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}
	bootstrap.Focuses[0].TargetSelectorRef = "file/workspace_deliverable"
	bootstrap.Focuses[0].AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}
	bootstrap.Requirements[0].RequiredCapabilities = []string{runtimecontract.StepKindFileWrite}
	bootstrap.Requirements = append(bootstrap.Requirements, runtimecontract.RuntimeRequirement{
		Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		RequiredCapabilities: []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}, Reason: "empty workspace File creation inventory",
	})
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID, DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
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
	candidateCalls := 0
	initialSelection := runtimecontract.RuntimeTargetSelectionDisposition("")
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		if candidateCalls == 1 && command.CandidateContext.Resources.TargetSelection != nil {
			initialSelection = command.CandidateContext.Resources.TargetSelection.Disposition
		}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: runtimeV2EmptyWorkspaceCreationCandidate(command)}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	module := newRuntimeV2GroundedTargetSequenceModule(t, hub, runtimeV2StateRepository{hub: hub})
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })

	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 3 {
		t.Fatalf("empty workspace creation completion=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
	}
	content, err := os.ReadFile(filepath.Join(deliverable, "implementation.txt"))
	if err != nil || string(content) != "created\n" {
		t.Fatalf("empty workspace creation file=%q err=%v", string(content), err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || initialSelection != runtimecontract.RuntimeTargetSelectionCreate ||
		persisted.RuntimeState.Facts.Resources.TargetSelection == nil || persisted.RuntimeState.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionSelected ||
		len(persisted.RuntimeState.Facts.Resources.TargetSnapshots) != 1 || len(persisted.RuntimeState.OperationReceipts) != 2 {
		t.Fatalf("empty workspace creation authority=%+v err=%v", persisted, err)
	}
}

func TestRuntimeV2FileBackedEmptyWorkspaceCreationReceiptCrashRestartDoesNotReplayCreate(t *testing.T) {
	for _, after := range []bool{false, true} {
		phase := "before_receipt_evidence_commit"
		if after {
			phase = "after_receipt_evidence_commit_response_loss"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			t.Cleanup(func() { hub.Terminate(ctx) })
			root := t.TempDir()
			deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDir, 0o755); err != nil {
				t.Fatal(err)
			}
			bootstrap := runtimeV2GroundedTargetSequenceBootstrap()
			bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}
			bootstrap.Focuses[0].TargetSelectorRef = "file/workspace_deliverable"
			bootstrap.Focuses[0].AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}
			bootstrap.Requirements[0].RequiredCapabilities = []string{runtimecontract.StepKindFileWrite}
			bootstrap.Requirements = append(bootstrap.Requirements, runtimecontract.RuntimeRequirement{
				Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				RequiredCapabilities: []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}, Reason: "empty workspace File creation inventory",
			})
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
			results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
			if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID, DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
				t.Fatal(err)
			}
			if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}); err != nil {
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
			workspaceOwner := workspacemodule.New()
			if err := workspaceOwner.Configure(workspacemodule.Dependencies{
				Workspaces: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(),
				Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub,
			}); err != nil {
				t.Fatal(err)
			}
			if err := workspaceOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
			resultOwner := resultmodule.New()
			if err := resultOwner.Configure(resultmodule.Dependencies{Results: results, Artifacts: memory.NewArtifactRepository(), EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := resultOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = resultOwner.Stop(ctx) })
			startRuntimeV2ReceiptTestOwner(t, hub)
			fileOwner := filemodule.New()
			if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := fileOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
			candidateCalls := 0
			llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				candidateCalls++
				command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
				result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: runtimeV2EmptyWorkspaceCreationCandidate(command)}, nil)
			})
			t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

			marker := "empty-workspace-create-receipt-" + phase
			crashRepository := &runtimeV2SaveCrashRepository{
				delegate: runtimeV2StateRepository{hub: hub}, after: after, marker: marker,
				match: func(aggregate runtimev2.RuntimeAggregate) bool {
					return len(aggregate.OperationReceipts) == 1 && len(aggregate.Facts.Evidence.Accepted) == 1
				},
			}
			crashing := newRuntimeV2GroundedTargetSequenceModule(t, hub, crashRepository)
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected empty-workspace receipt crash marker: %v", recovered)
					}
				}()
				if err := crashing.scheduleRuntimeV2Drive(ctx, "ws-v2", "run-v2", marker); err != nil {
					t.Fatalf("drive before empty-workspace receipt crash: %v", err)
				}
				t.Fatal("expected empty-workspace receipt crash")
			}()
			if !crashRepository.fired {
				t.Fatal("empty-workspace receipt crash point was not reached")
			}
			created, err := os.ReadFile(filepath.Join(deliverable, "implementation.txt"))
			if err != nil || string(created) != "created\n" {
				t.Fatalf("empty-workspace first create content=%q err=%v", string(created), err)
			}

			restarted := newRuntimeV2GroundedTargetSequenceModule(t, hub, runtimeV2StateRepository{hub: hub})
			if err := restarted.scheduleRuntimeV2Drive(ctx, "ws-v2", "run-v2", "restart:"+phase); err != nil {
				t.Fatalf("restart after empty-workspace receipt crash: %v", err)
			}
			persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || persisted.RuntimeState == nil || persisted.State != common.RunStateCompleted ||
				persisted.RuntimeState.Decision.Action != runtimecontract.RuntimeActionComplete ||
				persisted.RuntimeState.Facts.Resources.TargetSelection == nil || persisted.RuntimeState.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionSelected ||
				len(persisted.RuntimeState.Facts.Resources.TargetSnapshots) != 1 || len(persisted.RuntimeState.OperationReceipts) != 2 ||
				len(persisted.RuntimeState.Facts.Evidence.Accepted) != 2 || candidateCalls != 3 {
				t.Fatalf("empty-workspace receipt restart authority=%+v candidateCalls=%d err=%v", persisted, candidateCalls, err)
			}
		})
	}
}

// TestRuntimeV2FileBackedBoundedWorkspaceScopeAllowsNewDeliverable verifies
// the owner-chain counterpart to empty-workspace creation. A complete bounded
// scope is authority for File to assess a concrete new workspace-relative
// target, not an Orchestrator permission to synthesize one. The durable
// receipt must then invalidate the old scope and require a fresh complete
// scope before validation and assessment can converge. Scope contents remain
// File feasibility inventory; they must never be read as concrete snapshots.
func TestRuntimeV2FileBackedBoundedWorkspaceScopeAllowsNewDeliverable(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for target, content := range map[string]string{"implementation.txt": "before\n", "support.txt": "support\n"} {
		if err := os.WriteFile(filepath.Join(deliverable, target), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bootstrap := runtimeV2GroundedTargetSequenceBootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}
	bootstrap.Focuses[0].TargetSelectorRef = "file/workspace_deliverable"
	bootstrap.Focuses[0].AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}
	bootstrap.Requirements[0].RequiredCapabilities = []string{runtimecontract.StepKindFileWrite}
	bootstrap.Requirements = append(bootstrap.Requirements, runtimecontract.RuntimeRequirement{
		Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		RequiredCapabilities: []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}, Reason: "bounded workspace File creation inventory",
	})
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID, DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
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
	candidateCalls := 0
	stateChangeCalls := 0
	initialSelection := runtimecontract.RuntimeTargetSelectionDisposition("")
	initialSnapshots := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		if candidateCalls == 1 && command.CandidateContext.Resources.TargetSelection != nil {
			initialSelection = command.CandidateContext.Resources.TargetSelection.Disposition
			initialSnapshots = len(command.CandidateContext.Resources.TargetSnapshots)
		}
		if command.Decision.Action == runtimecontract.RuntimeActionProduceStateChange {
			stateChangeCalls++
			if stateChangeCalls == 1 {
				// A valid File write to unchanged content produces an owner no-op
				// receipt.  Scope must remain feasibility inventory through this
				// repair, not become the concrete directive target that blocks the
				// next File proposal below.
				result.Set(orchestratorevents.RequestRuntimeCandidateResult{
					Transaction: command.Transaction,
					Candidate:   runtimeV2WorkspaceCreationCandidate(command, "implementation.txt", "before\n"),
				}, nil)
				return
			}
		}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Candidate:   runtimeV2WorkspaceCreationCandidate(command, "new-deliverable.txt", "created\n"),
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	module := newRuntimeV2GroundedTargetSequenceModule(t, hub, runtimeV2StateRepository{hub: hub})
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })

	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 4 || stateChangeCalls != 2 {
		t.Fatalf("bounded scope new-deliverable completion=%+v candidateCalls=%d stateChangeCalls=%d err=%v", completed, candidateCalls, stateChangeCalls, err)
	}
	content, err := os.ReadFile(filepath.Join(deliverable, "new-deliverable.txt"))
	if err != nil || string(content) != "created\n" {
		t.Fatalf("bounded scope new-deliverable content=%q err=%v", string(content), err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || initialSelection != runtimecontract.RuntimeTargetSelectionScope || initialSnapshots != 0 ||
		persisted.RuntimeState.Facts.Resources.TargetSelection == nil || persisted.RuntimeState.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionScope ||
		persisted.RuntimeState.Facts.Resources.TargetSelection.CandidateCount != 3 || len(persisted.RuntimeState.Facts.Resources.TargetSnapshots) != 0 ||
		len(persisted.RuntimeState.OperationReceipts) != 3 || len(persisted.RuntimeState.Facts.Evidence.Accepted) != 2 || len(persisted.RuntimeState.Facts.Evidence.Rejected) != 1 ||
		persisted.RuntimeState.Facts.Resources.FailureTarget != nil || (persisted.RuntimeState.Facts.RepairDirective != nil && len(persisted.RuntimeState.Facts.RepairDirective.TargetRefs) != 0) {
		t.Fatalf("bounded scope new-deliverable authority=%+v err=%v", persisted, err)
	}
}

func runtimeV2EmptyWorkspaceCreationCandidate(command orchestratorevents.RequestRuntimeCandidateCommand) runtimecontract.RuntimeCandidate {
	return runtimeV2WorkspaceCreationCandidate(command, "implementation.txt", "created\n")
}

func runtimeV2WorkspaceCreationCandidate(command orchestratorevents.RequestRuntimeCandidateCommand, target, content string) runtimecontract.RuntimeCandidate {
	switch command.Decision.Action {
	case runtimecontract.RuntimeActionProduceStateChange:
		return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "create-deliverable", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
			Parameters: []runtimecontract.OperationParameter{{Name: "targetRef", String: &target}, {Name: "content", String: &content}},
		}}}}}
	case runtimecontract.RuntimeActionProduceValidation:
		return runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "read-created-deliverable", Kind: runtimecontract.StepKindFileRead, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target}, Parameters: []runtimecontract.OperationParameter{{Name: "path", String: &target}},
		}}}}}
	case runtimecontract.RuntimeActionProduceAssessment:
		refs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
		for _, evidence := range command.CandidateContext.Evidence.Accepted {
			refs = append(refs, string(evidence.ID))
		}
		return runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "created workspace deliverable validated", Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: refs}}, EvidenceRefs: refs}}
	default:
		return runtimecontract.RuntimeCandidate{}
	}
}

func testRuntimeV2FileBackedUnresolvedTargetSelectionFailsWithoutProvider(t *testing.T, bootstrap runtimecontract.RuntimeBootstrap, disposition runtimecontract.RuntimeTargetSelectionDisposition, files map[string]string) {
	t.Helper()
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(deliverable, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
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

	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Failure: &runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassInternal, Reason: "provider must not be called for unresolved target selection",
		}}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	governanceRequests := 0
	governanceObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleDecision, hub)
	governanceObserver.Subscribe(orchestratorevents.TopicOrchestratorOpenRuntimeGovernanceRequest, func(_ magicevent.Event, result magicevent.Result) {
		governanceRequests++
		result.Set(nil, nil)
	})
	t.Cleanup(func() {
		governanceObserver.Unsubscribe(orchestratorevents.TopicOrchestratorOpenRuntimeGovernanceRequest)
	})
	module := newRuntimeV2GroundedTargetSequenceModule(t, hub, runtimeV2StateRepository{hub: hub})
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })

	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal || completed.Action != runtimecontract.RuntimeActionFail || completed.Terminal == nil || completed.Terminal.Code != "target_grounding_unresolved" {
		t.Fatalf("unresolved owner target result=%+v err=%v", completed, err)
	}
	if providerCalls != 0 {
		t.Fatalf("unresolved target selection called provider %d times", providerCalls)
	}
	if governanceRequests != 0 {
		t.Fatalf("unresolved target selection opened %d governance requests", governanceRequests)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || persisted.RuntimeState.ActiveDispatch != nil || persisted.RuntimeState.Decision.Action != runtimecontract.RuntimeActionFail || persisted.RuntimeState.Decision.Terminal == nil ||
		persisted.RuntimeState.Facts.Resources.TargetSelection == nil || persisted.RuntimeState.Facts.Resources.TargetSelection.Disposition != disposition ||
		persisted.RuntimeState.Facts.Resources.TargetSelection.CandidateCount != len(files) || len(persisted.RuntimeState.Facts.Resources.TargetSnapshots) != 0 ||
		len(persisted.RuntimeState.OperationReceipts) != 0 || len(persisted.RuntimeState.Facts.Evidence.Accepted) != 0 {
		t.Fatalf("unresolved target selection persisted wrong authority state=%+v err=%v", persisted, err)
	}
}

func runtimeV2GroundedTargetSequenceBootstrap() runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeV2Bootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileEdit, runtimecontract.StepKindFileRead}
	bootstrap.Focuses = []runtimecontract.CoverageFocus{{
		ID: "focus-v2", SliceID: "slice-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
		TargetRole:            "deliverable", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
		TargetSelectorRef: "file/unique_workspace_entry", RequireTargetGrounding: true, RequireAssessment: true,
		AcceptedStepKinds: []string{runtimecontract.StepKindFileEdit, runtimecontract.StepKindFileRead},
	}}
	bootstrap.Slices[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.StepKindFileRead}},
	}
	return bootstrap
}

func runtimeV2GroundedTargetValidationBootstrap() runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeV2GroundedTargetSequenceBootstrap()
	bootstrap.Focuses[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	bootstrap.Focuses[0].RequireDirectEvidence = true
	bootstrap.Slices[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	return bootstrap
}

func runtimeV2GroundedTargetSequenceCandidate(command orchestratorevents.RequestRuntimeCandidateCommand) (runtimecontract.RuntimeCandidate, error) {
	target := "implementation.txt"
	switch command.Decision.Action {
	case runtimecontract.RuntimeActionProduceStateChange:
		oldText, newText := "before\n", "after\n"
		return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "grounded-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
			Parameters: []runtimecontract.OperationParameter{{Name: "path", String: &target}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}},
		}}}}}, nil
	case runtimecontract.RuntimeActionProduceValidation:
		return runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "grounded-read", Kind: runtimecontract.StepKindFileRead, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
			Parameters: []runtimecontract.OperationParameter{{Name: "path", String: &target}},
		}}}}}, nil
	case runtimecontract.RuntimeActionProduceAssessment:
		refs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
		for _, evidence := range command.CandidateContext.Evidence.Accepted {
			refs = append(refs, string(evidence.ID))
		}
		return runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
			FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionSatisfied,
			Summary: "grounded target evidence satisfied",
			Items:   []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: refs}}, EvidenceRefs: refs,
		}}, nil
	default:
		return runtimecontract.RuntimeCandidate{}, fmt.Errorf("unexpected grounded sequence action %q", command.Decision.Action)
	}
}

func newRuntimeV2GroundedTargetSequenceModule(t *testing.T, hub magicevent.Hub, repository runtimev2.RuntimeStateRepository) *Biz {
	t.Helper()
	module := New()
	if err := module.Configure(Dependencies{
		ToolCalls: memory.NewToolCallRepository(), Patches: memory.NewPatchRepository(),
		ValidationResults: memory.NewValidationResultRepository(), EvidenceRecords: memory.NewEvidenceRecordRepository(),
		CoverageWorkItems: memory.NewCoverageWorkItemRepository(), ArtifactContinuations: memory.NewArtifactContinuationRepository(),
		CommandHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := module.bizPtr.ConfigureRuntimeV2Repository(repository); err != nil {
		t.Fatal(err)
	}
	return module
}

func TestRuntimeV2FileBackedGitCapabilityStateSequenceMatrix(t *testing.T) {
	testCases := []struct {
		name               string
		prepare            func(*testing.T, string)
		expectedCandidates [][]string
		expectedFinalRefs  []string
		expectedCount      int
	}{
		{
			name: "repository_absent",
			expectedCandidates: [][]string{
				{"create-commit", "repo-init", "stage-changes"},
				{"create-commit", "stage-changes"},
				{"create-commit"},
			},
			expectedCount: 3,
			prepare: func(t *testing.T, deliverable string) {
				t.Helper()
				writeRuntimeV2GitFixtureFile(t, deliverable)
			},
		},
		{
			name: "initialized_unstaged",
			expectedCandidates: [][]string{
				{"create-commit", "stage-changes"},
				{"create-commit"},
			},
			expectedCount: 2,
			prepare: func(t *testing.T, deliverable string) {
				t.Helper()
				runRuntimeV2Git(t, deliverable, "init", "-q")
				runRuntimeV2Git(t, deliverable, "config", "user.name", "WorkOrch Test")
				runRuntimeV2Git(t, deliverable, "config", "user.email", "workorch@example.test")
				writeRuntimeV2GitFixtureFile(t, deliverable)
			},
		},
		{
			name: "initialized_staged", expectedCandidates: [][]string{{"create-commit"}}, expectedCount: 1,
			prepare: func(t *testing.T, deliverable string) {
				t.Helper()
				runRuntimeV2Git(t, deliverable, "init", "-q")
				runRuntimeV2Git(t, deliverable, "config", "user.name", "WorkOrch Test")
				runRuntimeV2Git(t, deliverable, "config", "user.email", "workorch@example.test")
				writeRuntimeV2GitFixtureFile(t, deliverable)
				runRuntimeV2Git(t, deliverable, "add", "target.txt")
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			t.Cleanup(func() { hub.Terminate(ctx) })
			root := t.TempDir()
			deliverable := filepath.Join(root, "workspace")
			internalDir := filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDir, 0o755); err != nil {
				t.Fatal(err)
			}
			testCase.prepare(t, deliverable)

			bootstrap := runtimeV2GitCapabilityStateBootstrap()
			bootstrap.Requirements = append(bootstrap.Requirements, runtimecontract.RuntimeRequirement{
				Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleGit,
				RequiredCapabilities: []string{runtimecontract.StepKindGitWrite}, Reason: "git owner inventory",
			})
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
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
				result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
			})
			t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
			startRuntimeV2FileManifestOwner(t, ctx, hub)

			receipts, err := gitbiz.NewCapabilityStateReceiptStore(filepath.Join(root, "git-owner-data"))
			if err != nil {
				t.Fatal(err)
			}
			gitOwner := gitmodule.New()
			if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: receipts}); err != nil {
				t.Fatal(err)
			}
			if err := gitOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = gitOwner.Stop(ctx) })

			candidateCalls := 0
			candidateRefsMatch := true
			llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				candidateCalls++
				command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
				refs := requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence)
				if candidateCalls > len(testCase.expectedCandidates) || !equalStrings(refs, testCase.expectedCandidates[candidateCalls-1]) {
					candidateRefsMatch = false
				}
				result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: runtimeV2GitStateCandidate(refs)}, nil)
			})
			t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

			module := newRuntimeV2DriveTestModule(t, hub)
			driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
			driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
			t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
			completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
			if err != nil {
				t.Fatal(err)
			}
			if completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != len(testCase.expectedCandidates) || !candidateRefsMatch {
				t.Fatalf("completed=%+v candidateCalls=%d candidateRefsMatch=%t", completed, candidateCalls, candidateRefsMatch)
			}
			persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || persisted.RuntimeState == nil || len(persisted.RuntimeState.OperationReceipts) != testCase.expectedCount {
				t.Fatalf("persisted=%+v err=%v", persisted, err)
			}
			if len(persisted.RuntimeState.Facts.Resources.CapabilityStates) != 1 || !equalStrings(requiredOperationRefs(persisted.RuntimeState.Facts.Resources.CapabilityStates[0].RequiredOperationSequence), testCase.expectedFinalRefs) {
				t.Fatalf("capability state = %+v", persisted.RuntimeState.Facts.Resources.CapabilityStates)
			}
			if len(persisted.RuntimeState.Facts.Resources.OperationAdmissibilities) != 1 ||
				persisted.RuntimeState.Facts.Resources.OperationAdmissibilities[0].AcquisitionBinding == "" ||
				persisted.RuntimeState.Facts.Resources.OperationAdmissibilities[0].AcquisitionBinding != persisted.RuntimeState.Facts.Resources.CapabilityStates[0].AcquisitionBinding {
				t.Fatalf("Git capability/admissibility binding = states=%+v inventories=%+v", persisted.RuntimeState.Facts.Resources.CapabilityStates, persisted.RuntimeState.Facts.Resources.OperationAdmissibilities)
			}
			if output := runtimeV2GitOutput(t, deliverable, "rev-list", "--count", "HEAD"); strings.TrimSpace(output) != "1" {
				t.Fatalf("commit count = %q, want 1", output)
			}
		})
	}
}

func TestRuntimeV2FileBackedGitRestartAfterInitEvidenceCommitContinuesSequence(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeV2GitFixtureFile(t, deliverable)

	bootstrap := runtimeV2GitCapabilityStateBootstrap()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
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
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeV2FileManifestOwner(t, ctx, hub)
	receipts, err := gitbiz.NewCapabilityStateReceiptStore(filepath.Join(root, "git-owner-data"))
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: receipts}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gitOwner.Stop(ctx) })

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
	if _, err := module.bizPtr.AcceptRuntimeV2CandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimeV2GitStateCandidate([]string{"create-commit", "repo-init", "stage-changes"})); err != nil {
		t.Fatal(err)
	}
	baseRepository := runtimeV2StateRepository{hub: hub}
	crashRepository := &runtimeV2SaveCrashRepository{
		delegate: baseRepository, after: true, marker: "git-init-evidence",
		match: func(current runtimev2.RuntimeAggregate) bool {
			if current.ActiveDispatch != nil || current.EvidenceSequence != 1 || len(current.OperationReceipts) != 1 ||
				current.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(current.CompletedDispatches) != 1 ||
				current.CompletedDispatches[0].State != runtimev2.DispatchStateInvalidated || len(current.CompletedDispatches[0].CompletedOperationIDs) != 1 {
				return false
			}
			return current.CompletedDispatches[0].CompletedOperationIDs[0] == "git-repo-init"
		},
	}
	crashingController, err := runtimev2.NewRuntimeController(crashRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "git-init-evidence" {
				t.Fatalf("unexpected crash marker: %v", recovered)
			}
		}()
		_, _ = crashingController.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", eventRuntimeV2OperationExecutor{hub: hub})
		t.Fatal("expected injected crash")
	}()
	crashed, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || crashed.RuntimeState == nil || len(crashed.RuntimeState.OperationReceipts) != 1 {
		t.Fatalf("crashed run=%+v err=%v", crashed, err)
	}
	initReceipt := crashed.RuntimeState.OperationReceipts[0]
	if initReceipt.OperationID != "git-repo-init" {
		t.Fatalf("first receipt=%+v", initReceipt)
	}

	candidateCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Candidate:   runtimeV2GitStateCandidate(requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence)),
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	restarted := newRuntimeV2DriveTestModule(t, hub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restarted.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 2 {
		t.Fatalf("restart completed=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || len(persisted.RuntimeState.OperationReceipts) != 3 {
		t.Fatalf("persisted=%+v err=%v", persisted, err)
	}
	countInit := 0
	for _, receipt := range persisted.RuntimeState.OperationReceipts {
		if receipt.OperationID == "git-repo-init" {
			countInit++
			if receipt.ExecutionKey != initReceipt.ExecutionKey || receipt.Fence != initReceipt.Fence {
				t.Fatalf("init receipt was replaced: before=%+v after=%+v", initReceipt, receipt)
			}
		}
	}
	if countInit != 1 || strings.TrimSpace(runtimeV2GitOutput(t, deliverable, "rev-list", "--count", "HEAD")) != "1" {
		t.Fatalf("init count=%d", countInit)
	}
}

func TestRuntimeV2FileBackedGitQueryReceiptCrashBeforePrerequisiteCommitReusesSnapshot(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeV2GitFixtureFile(t, deliverable)
	bootstrap := runtimeV2GitCapabilityStateBootstrap()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
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
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeV2FileManifestOwner(t, ctx, hub)
	storeRoot := filepath.Join(root, "git-owner-data")
	store, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: store}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}

	baseRepository := runtimeV2StateRepository{hub: hub}
	crashRepository := &runtimeV2SaveCrashRepository{
		delegate: baseRepository, marker: "git-query-before-prerequisite-commit",
		match: func(current runtimev2.RuntimeAggregate) bool {
			return len(current.Facts.Resources.CapabilityStates) == 1 && current.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite
		},
	}
	crashingBiz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := crashingBiz.ConfigureRuntimeV2Repository(crashRepository); err != nil {
		t.Fatal(err)
	}
	crashingModule := newTestBiz(crashingBiz, hub, nil)
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := crashingBiz.InitializeRuntimeV2FromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "git-query-before-prerequisite-commit" {
				t.Fatalf("unexpected crash marker: %v", recovered)
			}
		}()
		_, _ = crashingModule.prepareRuntimeV2Prerequisites(ctx, aggregate)
		t.Fatal("expected injected crash")
	}()
	if !crashRepository.fired {
		t.Fatal("query/prerequisite crash point was not reached")
	}
	crashed, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || crashed.RuntimeState == nil || len(crashed.RuntimeState.Facts.Resources.CapabilityStates) != 0 {
		t.Fatalf("crashed run=%+v err=%v", crashed, err)
	}
	var prerequisiteID string
	for _, prerequisite := range crashed.RuntimeState.Facts.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindCapabilityState {
			prerequisiteID = prerequisite.ID
			break
		}
	}
	if prerequisiteID == "" {
		t.Fatalf("missing persisted capability-state prerequisite: %+v", crashed.RuntimeState.Facts.Prerequisites)
	}
	acquisitionKey := stableRuntimeV2CapabilityStateAcquisitionKey("ws-v2", "run-v2", prerequisiteID, 0)
	queried, found, err := store.Load("ws-v2", acquisitionKey)
	if err != nil || !found {
		t.Fatalf("durable query receipt found=%t err=%v", found, err)
	}
	if err := gitOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	reopenedStore, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	restartedGitOwner := gitmodule.New()
	if err := restartedGitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: reopenedStore}); err != nil {
		t.Fatal(err)
	}
	if err := restartedGitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedGitOwner.Stop(ctx) })

	candidateCalls := 0
	firstCandidateReusedQueryReceipt := false
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Candidate:   runtimeV2GitStateCandidate(requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence)),
		}, nil)
		if candidateCalls == 1 && (len(command.CandidateContext.Resources.CapabilityStates) != 1 ||
			command.CandidateContext.Resources.CapabilityStates[0].SnapshotID != queried.SnapshotID ||
			command.CandidateContext.Resources.CapabilityStates[0].AcquisitionKey != queried.AcquisitionKey ||
			command.CandidateContext.Resources.CapabilityStates[0].QueryReceiptRef != queried.QueryReceiptRef) {
			t.Errorf("restart did not reuse durable query receipt in first candidate: %+v", command.CandidateContext.Resources.CapabilityStates)
			return
		}
		if candidateCalls == 1 {
			firstCandidateReusedQueryReceipt = true
		}
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	restarted := newRuntimeV2DriveTestModule(t, hub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restarted.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 3 || !firstCandidateReusedQueryReceipt {
		t.Fatalf("restart completed=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || len(persisted.RuntimeState.Facts.Resources.CapabilityStates) != 1 {
		t.Fatalf("persisted=%+v err=%v", persisted, err)
	}
	if len(persisted.RuntimeState.OperationReceipts) != 3 {
		t.Fatalf("restart did not complete refreshed Git sequence: receipts=%+v", persisted.RuntimeState.OperationReceipts)
	}
}

func TestRuntimeV2FileBackedGitSnapshotCommitCrashBeforeCandidateReusesCommittedSnapshot(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeV2GitFixtureFile(t, deliverable)
	bootstrap := runtimeV2GitCapabilityStateBootstrap()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
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
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeV2FileManifestOwner(t, ctx, hub)
	storeRoot := filepath.Join(root, "git-owner-data")
	store, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: store}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}

	// This is the durable boundary under test: all prerequisite snapshots have
	// committed, while no candidate attempt/dispatch has been persisted yet.
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
	if aggregate.Decision.Action != runtimecontract.RuntimeActionProduceStateChange || len(aggregate.Facts.Resources.CapabilityStates) != 1 || len(aggregate.CandidateAttempts) != 0 || aggregate.ActiveDispatch != nil {
		t.Fatalf("snapshot/candidate boundary aggregate=%+v", aggregate)
	}
	committedSnapshot := aggregate.Facts.Resources.CapabilityStates[0]
	if queried, found, err := store.Load("ws-v2", committedSnapshot.AcquisitionKey); err != nil || !found || queried.SnapshotID != committedSnapshot.SnapshotID {
		t.Fatalf("committed snapshot has no durable owner query receipt: found=%t queried=%+v err=%v", found, queried, err)
	}

	// Simulate process loss precisely after the Run aggregate CAS. Reopen the
	// owner store and drive from the persisted aggregate; no prerequisite query
	// may be scheduled because the matching snapshot is already authoritative.
	if err := gitOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	reopenedStore, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	restartedGitOwner := gitmodule.New()
	if err := restartedGitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: reopenedStore}); err != nil {
		t.Fatal(err)
	}
	if err := restartedGitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedGitOwner.Stop(ctx) })
	candidateCalls := 0
	firstCandidateReusedSnapshot := false
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Candidate:   runtimeV2GitStateCandidate(requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence)),
		}, nil)
		if candidateCalls == 1 && (len(command.CandidateContext.Resources.CapabilityStates) != 1 || command.CandidateContext.Resources.CapabilityStates[0].SnapshotID != committedSnapshot.SnapshotID ||
			command.CandidateContext.Resources.CapabilityStates[0].AcquisitionKey != committedSnapshot.AcquisitionKey) {
			t.Errorf("candidate did not reuse committed capability snapshot: %+v", command.CandidateContext.Resources.CapabilityStates)
			return
		}
		if candidateCalls == 1 {
			firstCandidateReusedSnapshot = true
		}
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, newRuntimeV2DriveTestModule(t, hub).handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 3 || !firstCandidateReusedSnapshot {
		t.Fatalf("restart after snapshot commit completed=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || len(persisted.RuntimeState.Facts.Resources.CapabilityStates) != 1 || len(persisted.RuntimeState.OperationReceipts) != 3 {
		t.Fatalf("persisted state=%+v err=%v", persisted, err)
	}
}

func TestDriveRuntimeV2FileBackedFreshFileShellGitAcceptance(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	hub := magicevent.NewHub(64)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
	results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
	artifacts := filerepository.NewArtifactRepository(filepath.Join(root, "artifacts.json"))
	bootstrap := runtimeV2FileShellGitBootstrap()
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
	shellOwner := shellmodule.New()
	if err := shellOwner.Configure(shellmodule.Dependencies{EventHub: hub, RuntimeAdapter: shellbiz.NewRuntimeAdapter()}); err != nil {
		t.Fatal(err)
	}
	if err := shellOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shellOwner.Stop(ctx) })
	gitReceipts, err := gitbiz.NewCapabilityStateReceiptStore(filepath.Join(root, "git-owner-data"))
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: gitReceipts}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gitOwner.Stop(ctx) })
	resultOwner := resultmodule.New()
	if err := resultOwner.Configure(resultmodule.Dependencies{Results: results, Artifacts: artifacts, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resultOwner.Stop(ctx) })

	candidateAttempts := map[string][]int{}
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		focusID, action := command.Decision.Focus.ID, command.Decision.Action
		candidateAttempts[focusID+"/"+string(action)] = append(candidateAttempts[focusID+"/"+string(action)], command.CandidateAttempt)
		candidate, candidateErr := runtimeV2FileShellGitCandidate(command)
		if candidateErr != nil {
			t.Errorf("candidate for %s/%s: %v", focusID, action, candidateErr)
			failure := runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassInternal, Reason: candidateErr.Error()}
			result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Failure: &failure}, nil)
			return
		}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: candidate}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	module := newRuntimeV2DriveTestModule(t, hub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Action != runtimecontract.RuntimeActionComplete || completed.Terminal == nil {
		current, loadErr := module.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
		t.Fatalf("file-shell-git drive completion=%+v terminal=%+v aggregate=%+v loadErr=%v err=%v", completed, completed.Terminal, current, loadErr, err)
	}
	if content, readErr := os.ReadFile(filepath.Join(deliverable, "main.go")); readErr != nil || string(content) != "package main\n// edited\n" {
		t.Fatalf("fresh File sequence content=%q err=%v", string(content), readErr)
	}
	t.Logf("file-shell-git candidate attempts: %+v", candidateAttempts)
	if got := strings.TrimSpace(runtimeV2GitOutput(t, deliverable, "rev-list", "--count", "HEAD")); got != "1" {
		t.Fatalf("fresh Git sequence commit count=%q", got)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil {
		t.Fatalf("persisted file-shell-git run=%+v err=%v", persisted, err)
	}
	state := persisted.RuntimeState
	if state.Facts.Recovery.PrerequisiteRefreshes != 0 || state.Facts.Recovery.PrerequisiteResourceChangeRefreshes < 4 || len(state.PrerequisiteInvalidations) < 4 {
		t.Fatalf("durable effects did not trigger persisted freshness re-acquisition: recovery=%+v invalidations=%+v", state.Facts.Recovery, state.PrerequisiteInvalidations)
	}
	if state.Facts.Recovery.ProviderTransportAttempts != 0 || state.Facts.Failure != nil {
		t.Fatalf("freshness path consumed unrelated budget or left failure: recovery=%+v failure=%+v", state.Facts.Recovery, state.Facts.Failure)
	}
	for key, attempts := range candidateAttempts {
		for _, attempt := range attempts {
			if attempt != 0 {
				t.Fatalf("fresh authority transaction %s started candidate attempt %d, want 0", key, attempt)
			}
		}
	}
	if attempts := candidateAttempts["focus-git/"+string(runtimecontract.RuntimeActionProduceStateChange)]; len(attempts) < 3 {
		t.Fatalf("Git init/add/commit did not use refreshed transactions: %+v", candidateAttempts)
	}
	for _, receipt := range state.OperationReceipts {
		if receipt.PrerequisiteStale != nil || receipt.Outcome == runtimecontract.RuntimeOperationOutcomeRejected {
			t.Fatalf("fresh positive path produced stale/rejected receipt: %+v", receipt)
		}
	}
	aggregate, err := module.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	delivery := newTestBiz(orchestratorbiz.NewRuntimeOrchestrator(runs, workspaces, nil, nil), hub, nil)
	if err := delivery.deliverRuntimeV2Terminal(ctx, aggregate); err != nil {
		t.Fatal(err)
	}
	resultItem, err := results.Get(ctx, "ws-v2/run-v2/result/dod-assessment")
	if err != nil || resultItem.State != common.ResultStateAccepted {
		t.Fatalf("accepted Result=%+v err=%v", resultItem, err)
	}
	terminalRun, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || terminalRun.State != common.RunStateCompleted {
		t.Fatalf("terminal Run=%+v err=%v", terminalRun, err)
	}
	workspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil || workspace.ActiveRunID != "" {
		t.Fatalf("terminal Workspace=%+v err=%v", workspace, err)
	}
}

func runtimeV2FileShellGitBootstrap() runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeV2Bootstrap()
	// This fixture deliberately performs five distinct durable effects
	// (create, edit, init, add, commit). Each can invalidate a mutable
	// prerequisite. The activation-sealed resource-change allowance covers this
	// normal progressing path; stale/no-progress recovery remains independent.
	bootstrap.RecoveryPolicy.MaxPrerequisiteResourceChangeRefreshes = 8
	bootstrap.Definition.CapabilityContract = []string{
		runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit,
		runtimecontract.RuntimeBlockKindManagedValidationSession, runtimecontract.StepKindGitWrite,
	}
	bootstrap.Focuses = []runtimecontract.CoverageFocus{
		{ID: "focus-file", SliceID: "slice-file", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-file"}, TargetRefs: []string{"main.go"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}},
		{ID: "focus-edit", SliceID: "slice-edit", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-edit"}, TargetRefs: []string{"main.go"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileEdit}},
		{ID: "focus-git", SliceID: "slice-git", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-git"}, TargetRefs: []string{"main.go"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.StepKindGitWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}, RequireAssessment: true},
	}
	bootstrap.Slices = []runtimecontract.RuntimeAcceptanceSlice{
		{ID: "slice-file", Title: "file", Scope: "fixture", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-file"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
		{ID: "slice-edit", Title: "edit", Scope: "fixture", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-edit"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
		{ID: "slice-git", Title: "git", Scope: "fixture", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-git"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
	}
	bootstrap.CoverageItems = []runtimecontract.RuntimeCoverageItem{{ID: "cwi-file", SliceID: "slice-file"}, {ID: "cwi-edit", SliceID: "slice-edit"}, {ID: "cwi-git", SliceID: "slice-git"}}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile, Reason: "workspace manifest"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindGitWrite}},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit}, Reason: "file owner inventory"},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "shell validation inventory"},
		{Kind: runtimecontract.PrerequisiteKindCapabilityState, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", Reason: "git sequence"},
		// Every executable capability needs its owner-authored admissibility
		// prerequisite. Git's inventory is bound to the capability-state receipt
		// above and must be invalidated/reacquired after the File tree changes;
		// omitting it would let the fixture generate git/write without its sealed
		// owner inventory, a bootstrap-contract violation rather than freshness.
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleGit, RequiredCapabilities: []string{runtimecontract.StepKindGitWrite}, Reason: "git owner inventory"},
	}
	return bootstrap
}

func runtimeV2FileShellGitCandidate(command orchestratorevents.RequestRuntimeCandidateCommand) (runtimecontract.RuntimeCandidate, error) {
	focus := command.Decision.Focus
	coverageID := focus.CoverageWorkItemIDs[0]
	switch focus.ID {
	case "focus-file":
		switch command.Decision.Action {
		case runtimecontract.RuntimeActionProduceStateChange:
			target, content := "main.go", "package main\n"
			return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: "file-create", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target}, Parameters: []runtimecontract.OperationParameter{{Name: "targetRef", String: &target}, {Name: "content", String: &content}}}}}}}, nil
		case runtimecontract.RuntimeActionProduceValidation:
			return runtimeV2ManagedShellValidationCandidate("validate-created", coverageID, "main.go", "package main"), nil
		}
	case "focus-edit":
		if command.Decision.Action == runtimecontract.RuntimeActionProduceStateChange {
			path, oldText, newText := "main.go", "package main\n", "package main\n// edited\n"
			return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: "file-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{path}, Parameters: []runtimecontract.OperationParameter{{Name: "path", String: &path}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}}}}}}}, nil
		}
	case "focus-git":
		switch command.Decision.Action {
		case runtimecontract.RuntimeActionProduceStateChange:
			return runtimeV2GitStateCandidateForCoverage(requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence), coverageID, "main.go"), nil
		case runtimecontract.RuntimeActionProduceValidation:
			return runtimeV2ManagedShellValidationCandidate("validate-committed", coverageID, "main.go", "edited"), nil
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
			return runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{FocusID: focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "git sequence validated", Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: coverageID, Satisfied: true}}, EvidenceRefs: refs}}, nil
		}
	}
	return runtimecontract.RuntimeCandidate{}, fmt.Errorf("unexpected focus/action %s/%s", focus.ID, command.Decision.Action)
}

func runtimeV2ManagedShellValidationCandidate(id string, coverageID common.CoverageWorkItemID, target, want string) runtimecontract.RuntimeCandidate {
	command, script := "sh", "test -f "+target+" && grep -q '"+want+"' "+target
	return runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: id, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{{Name: "command", String: &command}, {Name: "args", StringList: []string{"-c", script}}, {Name: "targetRef", String: &target}},
	}}}}}
}

func runtimeV2GitStateCandidateForCoverage(requirementRefs []string, coverageID common.CoverageWorkItemID, target string) runtimecontract.RuntimeCandidate {
	operations := make([]runtimecontract.StateChangingOperation, 0, len(requirementRefs))
	for _, ref := range requirementRefs {
		operation := runtimecontract.RuntimeOperation{ID: "fresh-git-" + ref, Kind: runtimecontract.StepKindGitWrite, RequiredOperationRef: ref, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target}}
		switch ref {
		case "repo-init":
			value := "init"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}}
		case "stage-changes":
			value := "add"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}, {Name: "pathspecs", StringList: []string{target}}}
		case "create-commit":
			value, message := "commit", "fresh runtime v2 fixture"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}, {Name: "message", String: &message}}
		}
		switch ref {
		case "stage-changes":
			if containsRuntimeV2String(requirementRefs, "repo-init") {
				operation.DependsOn = []string{"fresh-git-repo-init"}
			}
		case "create-commit":
			if containsRuntimeV2String(requirementRefs, "stage-changes") {
				operation.DependsOn = []string{"fresh-git-stage-changes"}
			}
		}
		operations = append(operations, runtimecontract.StateChangingOperation{Operation: operation})
	}
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: operations}}
}

func TestRuntimeV2FileBackedGitStaleRefreshCrashRestartDoesNotReplayOldDispatch(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeV2GitFixtureFile(t, deliverable)
	bootstrap := runtimeV2GitCapabilityStateBootstrap()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
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
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeV2FileManifestOwner(t, ctx, hub)
	storeRoot := filepath.Join(root, "git-owner-data")
	store, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: store}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}

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
	if _, err := module.bizPtr.AcceptRuntimeV2CandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimeV2GitStateCandidate([]string{"create-commit", "repo-init", "stage-changes"})); err != nil {
		t.Fatal(err)
	}

	// External owner state changes after the sealed absent-repository snapshot
	// and before the first operation. The old init must become a typed stale
	// receipt, never a replayable execution failure or an ordinary repair.
	runRuntimeV2Git(t, deliverable, "init", "-q")
	runRuntimeV2Git(t, deliverable, "config", "user.name", "WorkOrch Test")
	runRuntimeV2Git(t, deliverable, "config", "user.email", "workorch@example.test")
	baseRepository := runtimeV2StateRepository{hub: hub}
	crashRepository := &runtimeV2SaveCrashRepository{
		delegate: baseRepository, after: true, marker: "git-stale-refresh",
		match: func(current runtimev2.RuntimeAggregate) bool {
			return current.ActiveDispatch == nil && current.Decision.Action == runtimecontract.RuntimeActionAcquirePrerequisite &&
				current.Facts.Recovery.PrerequisiteRefreshes == 1 && len(current.Facts.Resources.CapabilityStates) == 0
		},
	}
	crashingController, err := runtimev2.NewRuntimeController(crashRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "git-stale-refresh" {
				t.Fatalf("unexpected stale-refresh crash marker: %v", recovered)
			}
		}()
		_, _ = crashingController.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", eventRuntimeV2OperationExecutor{hub: hub})
		t.Fatal("expected injected stale-refresh crash")
	}()
	crashed, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || crashed.RuntimeState == nil || crashed.RuntimeState.ActiveDispatch != nil ||
		crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 1 || len(crashed.RuntimeState.Facts.Resources.CapabilityStates) != 0 ||
		len(crashed.RuntimeState.OperationReceipts) != 1 || crashed.RuntimeState.OperationReceipts[0].PrerequisiteStale == nil || crashed.RuntimeState.EvidenceSequence != 0 {
		t.Fatalf("stale refresh crash boundary=%+v err=%v", crashed, err)
	}
	if err := gitOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	reopenedStore, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	restartedGitOwner := gitmodule.New()
	if err := restartedGitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: reopenedStore}); err != nil {
		t.Fatal(err)
	}
	if err := restartedGitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedGitOwner.Stop(ctx) })
	candidateCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		refs := requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: runtimeV2GitStateCandidate(refs)}, nil)
		expected := []string{"create-commit", "stage-changes"}
		if candidateCalls == 2 {
			expected = []string{"create-commit"}
		}
		if !equalStrings(refs, expected) {
			t.Errorf("stale refresh used wrong required sequence on candidate %d: %v", candidateCalls, refs)
		}
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, newRuntimeV2DriveTestModule(t, hub).handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 2 {
		t.Fatalf("stale refresh restart completed=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || len(persisted.RuntimeState.OperationReceipts) != 3 {
		t.Fatalf("stale refresh restart persisted receipts=%+v err=%v", persisted.RuntimeState.OperationReceipts, err)
	}
	initReceipts := 0
	for _, receipt := range persisted.RuntimeState.OperationReceipts {
		if receipt.OperationID == "git-repo-init" {
			initReceipts++
			if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || receipt.PrerequisiteStale == nil {
				t.Fatalf("old init was replayed or reclassified: %+v", receipt)
			}
		}
	}
	if initReceipts != 1 || strings.TrimSpace(runtimeV2GitOutput(t, deliverable, "rev-list", "--count", "HEAD")) != "1" {
		t.Fatalf("old init dispatch was replayed, receipts=%d", initReceipts)
	}
}

func runtimeV2GitCapabilityStateBootstrap() runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeV2Bootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindGitWrite}
	bootstrap.Focuses[0].AcceptedStepKinds = []string{runtimecontract.StepKindGitWrite}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapabilityState, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", Reason: "repository state required"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindGitWrite}, Reason: "git write capability required"},
	}
	return bootstrap
}

func runtimeV2GitStateCandidate(requirementRefs []string) runtimecontract.RuntimeCandidate {
	operations := make([]runtimecontract.StateChangingOperation, 0, len(requirementRefs))
	for _, ref := range requirementRefs {
		operation := runtimecontract.RuntimeOperation{ID: "git-" + ref, Kind: runtimecontract.StepKindGitWrite, RequiredOperationRef: ref, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}}
		switch ref {
		case "repo-init":
			value := "init"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}}
		case "stage-changes":
			value := "add"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}, {Name: "pathspecs", StringList: []string{"target.txt"}}}
		case "create-commit":
			value, message := "commit", "runtime v2 fixture"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}, {Name: "message", String: &message}}
		}
		switch ref {
		case "stage-changes":
			if containsRuntimeV2String(requirementRefs, "repo-init") {
				operation.DependsOn = []string{"git-repo-init"}
			}
		case "create-commit":
			if containsRuntimeV2String(requirementRefs, "stage-changes") {
				operation.DependsOn = []string{"git-stage-changes"}
			}
		}
		operations = append(operations, runtimecontract.StateChangingOperation{Operation: operation})
	}
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: operations}}
}

func containsRuntimeV2String(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func requiredOperationRefs(values []runtimecontract.RuntimeRequiredOperationVariant) []string {
	ret := make([]string, 0, len(values))
	for _, value := range values {
		ret = append(ret, value.RequirementID)
	}
	return ret
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func writeRuntimeV2GitFixtureFile(t *testing.T, deliverable string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("runtime v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runRuntimeV2Git(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func runtimeV2GitOutput(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(output)
}

func TestRuntimeV2FileBackedPatchVariantMatrix(t *testing.T) {
	testCases := []struct {
		name      string
		target    string
		initial   string
		variant   string
		candidate func() runtimecontract.RuntimeCandidate
		want      string
	}{
		{
			name: "file_write", target: "written.txt", initial: "old\n", variant: "file_write", want: "written\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, content, target := "file_write", "written\n", "written.txt"
				return runtimeV2PatchCandidate("patch-file-write", target, []runtimecontract.OperationParameter{{Name: "targetRef", String: &target}, {Name: "patchType", String: &patchType}, {Name: "content", String: &content}})
			},
		},
		{
			name: "text_replace", target: "replace.txt", initial: "old\n", variant: "text_replace", want: "new\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, oldText, newText, target := "text_replace", "old\n", "new\n", "replace.txt"
				return runtimeV2PatchCandidate("patch-text-replace", target, []runtimecontract.OperationParameter{{Name: "targetRef", String: &target}, {Name: "patchType", String: &patchType}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}})
			},
		},
		{
			name: "unified_diff", target: "diff.txt", initial: "one\ntwo\nthree\n", variant: "unified_diff", want: "one\nTWO\nthree\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, target := "unified_diff", "diff.txt"
				diff := "--- a/diff.txt\n+++ b/diff.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n"
				return runtimeV2PatchCandidate("patch-unified-diff", target, []runtimecontract.OperationParameter{{Name: "targetRef", String: &target}, {Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}})
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			t.Cleanup(func() { hub.Terminate(ctx) })
			root := t.TempDir()
			deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if testCase.initial != "" {
				if err := os.WriteFile(filepath.Join(deliverable, testCase.target), []byte(testCase.initial), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			bootstrap := runtimeV2PatchVariantBootstrap(testCase.variant, testCase.target)
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
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
			llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
				result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: testCase.candidate()}, nil)
			})
			t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
			module := newRuntimeV2DriveTestModule(t, hub)
			driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
			driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
			t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
			completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
			if err != nil || completed.Action != runtimecontract.RuntimeActionComplete {
				t.Fatalf("completed=%+v err=%v", completed, err)
			}
			content, err := os.ReadFile(filepath.Join(deliverable, testCase.target))
			if err != nil || string(content) != testCase.want {
				t.Fatalf("content=%q err=%v want=%q", content, err, testCase.want)
			}
		})
	}
}

func runtimeV2PatchVariantBootstrap(variant, target string) runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeV2Bootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.RuntimeBlockKindPatchApply}
	bootstrap.Focuses[0].AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindPatchApply}
	bootstrap.Focuses[0].AcceptedOperationVariants = []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: variant}}
	bootstrap.Focuses[0].TargetRefs = []string{target}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindPatchApply}, Reason: "patch capability required"},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindPatchApply}, Reason: "patch owner inventory required"},
	}
	return bootstrap
}

func runtimeV2PatchCandidate(id, target string, parameters []runtimecontract.OperationParameter) runtimecontract.RuntimeCandidate {
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: id, Kind: runtimecontract.RuntimeBlockKindPatchApply, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target}, Parameters: parameters}}}}}
}

func TestRuntimeV2FileBackedPatchReplayProofRestartsAfterOwnerReceiptCASFailure(t *testing.T) {
	testCases := []struct {
		name        string
		target      string
		initial     string
		variant     string
		operationID string
		want        string
		candidate   func(string) runtimecontract.RuntimeCandidate
	}{
		{
			name: "file_write", target: "written.txt", initial: "old\n", variant: "file_write", operationID: "patch-file-write", want: "written\n",
			candidate: func(target string) runtimecontract.RuntimeCandidate {
				patchType, content := "file_write", "written\n"
				return runtimeV2PatchCandidate("patch-file-write", target, []runtimecontract.OperationParameter{{Name: "targetRef", String: &target}, {Name: "patchType", String: &patchType}, {Name: "content", String: &content}})
			},
		},
		{
			name: "text_replace", target: "written.txt", initial: "old\n", variant: "text_replace", operationID: "patch-text-replace", want: "written\n",
			candidate: func(target string) runtimecontract.RuntimeCandidate {
				patchType, oldText, newText := "text_replace", "old\n", "written\n"
				return runtimeV2PatchCandidate("patch-text-replace", target, []runtimecontract.OperationParameter{{Name: "targetRef", String: &target}, {Name: "patchType", String: &patchType}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}})
			},
		},
		{
			name: "unified_diff", target: "written.txt", initial: "old\n", variant: "unified_diff", operationID: "patch-unified-diff", want: "written\n",
			candidate: func(target string) runtimecontract.RuntimeCandidate {
				patchType := "unified_diff"
				diff := "--- a/written.txt\n+++ b/written.txt\n@@ -1,1 +1,1 @@\n-old\n+written\n"
				return runtimeV2PatchCandidate("patch-unified-diff", target, []runtimecontract.OperationParameter{{Name: "targetRef", String: &target}, {Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}})
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testRuntimeV2FileBackedPatchReplayProofRestartsAfterOwnerReceiptCASFailure(t, testCase.target, testCase.initial, testCase.variant, testCase.operationID, testCase.want, testCase.candidate)
		})
	}
}

func testRuntimeV2FileBackedPatchReplayProofRestartsAfterOwnerReceiptCASFailure(t *testing.T, target, initial, variant, operationID, want string, candidateForTarget func(string) runtimecontract.RuntimeCandidate) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bootstrap := runtimeV2PatchVariantBootstrap(variant, target)
	// Make the continuation cross the P4 assessment boundary as well as the
	// P3 owner-receipt boundary. Other patch-variant tests intentionally keep
	// their bootstrap validation-free, so scope this requirement to this
	// receipt-loss recovery sequence.
	bootstrap.Focuses[0].RequireAssessment = true
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deliverable, target), []byte(initial), 0o644); err != nil {
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
	baseRepository := runtimeV2StateRepository{hub: hub}
	failingRepository := &failWhenRuntimeStateRepository{
		RuntimeStateRepository: baseRepository, err: errors.New("injected owner receipt CAS failure"),
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			if aggregate.ActiveDispatch == nil || len(aggregate.OperationReceipts) != 1 || aggregate.EvidenceSequence != 0 {
				return false
			}
			execution, found := runtimeV2DriveOperationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, operationID)
			return found && execution.State == runtimecontract.OperationExecutionOwnerCommitted
		},
	}
	crashingBiz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := crashingBiz.ConfigureRuntimeV2Repository(failingRepository); err != nil {
		t.Fatal(err)
	}
	crashingModule := newTestBiz(crashingBiz, hub, nil)
	receiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, crashingModule.handleRuntimeV2OperationReceipt)
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, crashingModule.handleRuntimeV2OperationReceipt)

	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := crashingBiz.InitializeRuntimeV2FromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = crashingModule.prepareRuntimeV2Prerequisites(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	candidate := candidateForTarget(target)
	providerCalls := 0
	stateChangeCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		if command.Decision.Action == runtimecontract.RuntimeActionProduceAssessment {
			evidenceRefs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
			for _, evidence := range command.CandidateContext.Evidence.Accepted {
				evidenceRefs = append(evidenceRefs, string(evidence.ID))
			}
			result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
				FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionSatisfied,
				Summary: "replay-proof durable patch assessed", Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: evidenceRefs}}, EvidenceRefs: evidenceRefs,
			}}}, nil)
			return
		}
		if command.Decision.Action == runtimecontract.RuntimeActionProduceStateChange {
			stateChangeCalls++
		}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: candidate}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	aggregate, err = crashingModule.requestAndSealRuntimeV2CandidateWithRetry(ctx, aggregate, prefix)
	if err != nil || aggregate.ActiveDispatch == nil {
		t.Fatalf("seal replay-proof dispatch aggregate=%+v err=%v", aggregate, err)
	}
	sealedDispatchID := aggregate.ActiveDispatch.ID
	sealedExecution, found := runtimeV2DriveOperationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, operationID)
	if !found || sealedExecution.ReplayProof == nil {
		t.Fatalf("patch dispatch has no sealed replay proof: %+v", sealedExecution)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "runtime-v2-test-crash-after-owner-receipt-commit-error" {
				t.Fatalf("unexpected crash marker: %v", recovered)
			}
		}()
		_, _ = crashingBiz.ExecuteRuntimeV2Dispatch(ctx, "ws-v2", "run-v2", panicAfterOwnerReceiptCommitErrorExecutor{delegate: eventRuntimeV2OperationExecutor{hub: hub}})
		t.Fatal("expected injected receipt-CAS crash")
	}()
	if !failingRepository.failed {
		t.Fatal("owner receipt CAS failure point was not reached")
	}
	if content, err := os.ReadFile(filepath.Join(deliverable, target)); err != nil || string(content) != want {
		t.Fatalf("durable side effect before restart content=%q err=%v", content, err)
	}
	crashed, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || crashed.RuntimeState == nil || crashed.RuntimeState.ActiveDispatch == nil || len(crashed.RuntimeState.OperationReceipts) != 0 || crashed.RuntimeState.EvidenceSequence != 0 {
		t.Fatalf("receipt-CAS crash persisted state=%+v err=%v", crashed, err)
	}
	claimed, found := runtimeV2DriveOperationExecutionRecord(crashed.RuntimeState.ActiveDispatch.OperationExecutions, operationID)
	if !found || claimed.State != runtimecontract.OperationExecutionClaimed || claimed.Fence == 0 || claimed.ExecutionKey != sealedExecution.ExecutionKey || claimed.ReplayProof == nil {
		t.Fatalf("receipt-CAS crash lost claimed proof execution=%+v", claimed)
	}

	receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	restartedBiz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := restartedBiz.ConfigureRuntimeV2Repository(baseRepository); err != nil {
		t.Fatal(err)
	}
	restartedModule := newTestBiz(restartedBiz, hub, nil)
	restartedReceiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	restartedReceiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, restartedModule.handleRuntimeV2OperationReceipt)
	restartedReceiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, restartedModule.handleRuntimeV2OperationReceipt)
	t.Cleanup(func() {
		restartedReceiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		restartedReceiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})
	completed, err := restartedBiz.ExecuteRuntimeV2Dispatch(ctx, "ws-v2", "run-v2", eventRuntimeV2OperationExecutor{hub: hub})
	if err != nil || completed.ActiveDispatch != nil || completed.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || completed.Facts.Recovery.OperationRecoveryAttempts != 1 || len(completed.OperationReceipts) != 1 || len(completed.Facts.Evidence.Accepted) != 1 {
		t.Fatalf("restart did not recover proof-bound receipt exactly once aggregate=%+v err=%v", completed, err)
	}
	receipt := completed.OperationReceipts[0]
	if receipt.ExecutionKey != claimed.ExecutionKey || receipt.Fence != claimed.Fence || receipt.BeforeHash != claimed.ReplayProof.BeforeHash || receipt.AfterHash != claimed.ReplayProof.AfterHash {
		t.Fatalf("restart did not retain sealed execution/receipt delta receipt=%+v claimed=%+v", receipt, claimed)
	}
	// A duplicate owner delivery after process reconstruction must resolve the
	// same persisted key/fence receipt. It must neither reapply the durable
	// patch nor mutate the Run aggregate after the first recovered evidence
	// commit.
	var sealedDispatch runtimev2.SealedDispatch
	for _, value := range completed.CompletedDispatches {
		if value.ID == sealedDispatchID {
			sealedDispatch = value
			break
		}
	}
	if sealedDispatch.ID == "" || sealedDispatch.Candidate.StateChange == nil || len(sealedDispatch.Candidate.StateChange.Operations) != 1 {
		t.Fatalf("recovered dispatch is unavailable for duplicate delivery: %+v", completed.CompletedDispatches)
	}
	// Recreate the File capability owner before asking it about the already
	// committed receipt.  File owns execution only: the replay answer must come
	// from Orchestrator's persisted receipt query, never from a process-local
	// File adapter cache.
	if err := fileOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	restartedFileOwner := filemodule.New()
	if err := restartedFileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedFileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedFileOwner.Stop(ctx) })
	duplicate, err := (eventRuntimeV2OperationExecutor{hub: hub}).ExecuteRuntimeOperation(ctx, runtimev2.RuntimeOperationExecutionRequest{
		WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: sealedDispatch.ID, Transaction: sealedDispatch.Transaction,
		Operation: sealedDispatch.Candidate.StateChange.Operations[0].Operation, Execution: claimed,
		EvidenceKind: runtimecontract.EvidenceKindStateChange, TargetRole: completed.Facts.Focus.TargetRole,
	})
	if err != nil || duplicate.Receipt.ExecutionKey != receipt.ExecutionKey || duplicate.Receipt.Fence != receipt.Fence ||
		duplicate.Receipt.BeforeHash != receipt.BeforeHash || duplicate.Receipt.AfterHash != receipt.AfterHash {
		t.Fatalf("duplicate owner delivery did not reuse persisted receipt result=%+v err=%v", duplicate, err)
	}
	if content, readErr := os.ReadFile(filepath.Join(deliverable, target)); readErr != nil || string(content) != want {
		t.Fatalf("duplicate owner delivery rewrote durable target content=%q err=%v", content, readErr)
	}
	loaded, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.RuntimeState == nil || loaded.RuntimeState.StateRevision != completed.StateRevision ||
		len(loaded.RuntimeState.OperationReceipts) != 1 || len(loaded.RuntimeState.Facts.Evidence.Accepted) != 1 {
		t.Fatalf("duplicate owner delivery changed persisted aggregate=%+v err=%v", loaded, err)
	}

	// Continue through the normal drive after the reconstructed receipt. This
	// ties P3's owner-receipt loss to P4's evidence/assessment transition: the
	// restart may only consume the original key/fence receipt and must not ask
	// the provider for a second state-changing candidate before terminal
	// authority is committed.
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restartedModule.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	terminal, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || terminal.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal || terminal.Action != runtimecontract.RuntimeActionComplete || terminal.Terminal == nil {
		t.Fatalf("receipt-loss continuation did not commit terminal authority result=%+v err=%v", terminal, err)
	}
	if providerCalls != 2 || stateChangeCalls != 1 {
		t.Fatalf("receipt-loss continuation provider calls=%d state-change calls=%d, want one initial patch and one assessment", providerCalls, stateChangeCalls)
	}
	terminalAggregate, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || terminalAggregate.RuntimeState == nil || terminalAggregate.RuntimeState.ActiveDispatch != nil ||
		terminalAggregate.RuntimeState.Decision.Action != runtimecontract.RuntimeActionComplete || terminalAggregate.RuntimeState.Decision.Terminal == nil ||
		len(terminalAggregate.RuntimeState.OperationReceipts) != 1 || len(terminalAggregate.RuntimeState.Facts.Evidence.Accepted) != 1 ||
		len(terminalAggregate.RuntimeState.Facts.Assessments) != 1 {
		t.Fatalf("receipt-loss continuation changed P3/P4 identities or missed assessment terminal aggregate=%+v err=%v", terminalAggregate, err)
	}
	finalReceipt := terminalAggregate.RuntimeState.OperationReceipts[0]
	if finalReceipt.ExecutionKey != receipt.ExecutionKey || finalReceipt.Fence != receipt.Fence || finalReceipt.BeforeHash != receipt.BeforeHash || finalReceipt.AfterHash != receipt.AfterHash {
		t.Fatalf("terminal continuation replaced reconstructed receipt before=%+v after=%+v", receipt, finalReceipt)
	}
	// P3 may race with the eventual terminal delivery: a delayed duplicate
	// receipt must remain an idempotent lookup, while a different receipt for
	// the same sealed execution key/fence must be rejected.  In both cases the
	// already committed SM-2 terminal authority is immutable; no new dispatch,
	// evidence, candidate attempt, or provider request may appear.
	duplicateTerminal, duplicateReceipt, err := restartedBiz.CommitRuntimeV2OperationReceipt(
		ctx, "ws-v2", "run-v2", sealedDispatchID, finalReceipt.Fence, finalReceipt,
	)
	if err != nil || duplicateTerminal.StateRevision != terminalAggregate.RuntimeState.StateRevision ||
		!runtimecontract.SameRuntimeOperationReceipt(duplicateReceipt, finalReceipt) {
		t.Fatalf("duplicate receipt after terminal aggregate=%+v receipt=%+v err=%v", duplicateTerminal, duplicateReceipt, err)
	}
	tamperedReceipt := finalReceipt.Clone()
	tamperedReceipt.Evidence.Summary = "tampered delayed receipt"
	if _, _, err := restartedBiz.CommitRuntimeV2OperationReceipt(ctx, "ws-v2", "run-v2", sealedDispatchID, tamperedReceipt.Fence, tamperedReceipt); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("different delayed receipt after terminal err=%v, want conflict", err)
	}
	immutableTerminal, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || immutableTerminal.RuntimeState == nil ||
		immutableTerminal.RuntimeState.StateRevision != terminalAggregate.RuntimeState.StateRevision ||
		immutableTerminal.RuntimeState.Decision.Action != runtimecontract.RuntimeActionComplete ||
		len(immutableTerminal.RuntimeState.OperationReceipts) != 1 || len(immutableTerminal.RuntimeState.Facts.Evidence.Accepted) != 1 ||
		providerCalls != 2 || stateChangeCalls != 1 {
		t.Fatalf("late receipt changed terminal authority=%+v provider=%d stateChanges=%d err=%v", immutableTerminal, providerCalls, stateChangeCalls, err)
	}

	// Carry the recovered P3 receipt through a P5 owner acknowledgement loss.
	// The workspace observer was only the File prerequisite adapter; terminal
	// projection below deliberately uses the production Workspace owner.
	workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
	workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{
		ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2",
		DeliverableDirectory: deliverable, InternalDirectory: internalDir,
	}); err != nil {
		t.Fatal(err)
	}
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspacemodule.Dependencies{
		Workspaces: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(),
		Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaceOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
	deliveryModule := newTestBiz(orchestratorbiz.NewRuntimeOrchestrator(runs, workspaces, nil, nil), hub, nil)
	results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
	interruptedResults := &failAfterResultCreateRepository{ResultRepository: results}
	firstResultOwner := resultmodule.New()
	if err := firstResultOwner.Configure(resultmodule.Dependencies{
		Results: interruptedResults, Artifacts: filerepository.NewArtifactRepository(filepath.Join(root, "artifacts.json")), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := firstResultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	if err := deliveryModule.deliverRuntimeV2Terminal(ctx, runtimeAggregateFromRecord(*terminalAggregate.RuntimeState)); err == nil {
		t.Fatal("expected terminal Result acknowledgement loss after recovered receipt")
	}
	if !interruptedResults.fired {
		t.Fatal("terminal Result acknowledgement loss was not reached")
	}
	if run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2"); err != nil || run.State != common.RunStateActive {
		t.Fatalf("P5 projected Run before Result acknowledgement restart: %+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "run-v2" {
		t.Fatalf("P5 released Workspace before Result acknowledgement restart: %+v err=%v", workspace, err)
	}
	if err := firstResultOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	restartedResultOwner := resultmodule.New()
	if err := restartedResultOwner.Configure(resultmodule.Dependencies{
		Results: results, Artifacts: filerepository.NewArtifactRepository(filepath.Join(root, "artifacts.json")), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := restartedResultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedResultOwner.Stop(ctx) })
	persistedTerminal, err := restartedBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil || persistedTerminal.StateRevision != terminalAggregate.RuntimeState.StateRevision || persistedTerminal.Decision.Action != runtimecontract.RuntimeActionComplete {
		t.Fatalf("P3/P5 restart did not retain terminal authority=%+v err=%v", persistedTerminal, err)
	}
	if err := deliveryModule.deliverRuntimeV2Terminal(ctx, persistedTerminal); err != nil {
		t.Fatalf("P3/P5 terminal replay: %v", err)
	}
	if items, err := results.ListByRun(ctx, "run-v2"); err != nil || len(items) != 1 || items[0].State != common.ResultStateAccepted {
		t.Fatalf("P3/P5 Result replay=%+v err=%v", items, err)
	}
	if run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2"); err != nil || run.State != common.RunStateCompleted {
		t.Fatalf("P3/P5 Run replay=%+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "" {
		t.Fatalf("P3/P5 Workspace replay=%+v err=%v", workspace, err)
	}
	finalAggregate, err := restartedBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil || finalAggregate.StateRevision != terminalAggregate.RuntimeState.StateRevision ||
		len(finalAggregate.OperationReceipts) != 1 || len(finalAggregate.Facts.Evidence.Accepted) != 1 ||
		providerCalls != 2 || stateChangeCalls != 1 {
		t.Fatalf("P3/P5 replay rewrote authority=%+v provider=%d stateChanges=%d err=%v", finalAggregate, providerCalls, stateChangeCalls, err)
	}
}

func runtimeV2DriveOperationExecutionRecord(values []runtimecontract.RuntimeOperationExecutionRecord, operationID string) (runtimecontract.RuntimeOperationExecutionRecord, bool) {
	for _, value := range values {
		if value.OperationID == operationID {
			return value, true
		}
	}
	return runtimecontract.RuntimeOperationExecutionRecord{}, false
}

func TestRuntimeV2FileBackedPatchVariantRestartAfterOwnerReceipt(t *testing.T) {
	testCases := []struct {
		name      string
		target    string
		initial   string
		variant   string
		candidate func() runtimecontract.RuntimeCandidate
		want      string
	}{
		{
			name: "file_write", target: "written.txt", initial: "old\n", variant: "file_write", want: "written\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, content, target := "file_write", "written\n", "written.txt"
				return runtimeV2PatchCandidate("patch-file-write", target, []runtimecontract.OperationParameter{{Name: "targetRef", String: &target}, {Name: "patchType", String: &patchType}, {Name: "content", String: &content}})
			},
		},
		{
			name: "text_replace", target: "replace.txt", initial: "old\n", variant: "text_replace", want: "new\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, oldText, newText, target := "text_replace", "old\n", "new\n", "replace.txt"
				return runtimeV2PatchCandidate("patch-text-replace", target, []runtimecontract.OperationParameter{{Name: "targetRef", String: &target}, {Name: "patchType", String: &patchType}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}})
			},
		},
		{
			name: "unified_diff", target: "diff.txt", initial: "one\ntwo\nthree\n", variant: "unified_diff", want: "one\nTWO\nthree\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, target := "unified_diff", "diff.txt"
				diff := "--- a/diff.txt\n+++ b/diff.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n"
				return runtimeV2PatchCandidate("patch-unified-diff", target, []runtimecontract.OperationParameter{{Name: "targetRef", String: &target}, {Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}})
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			t.Cleanup(func() { hub.Terminate(ctx) })
			root := t.TempDir()
			deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if testCase.initial != "" {
				if err := os.WriteFile(filepath.Join(deliverable, testCase.target), []byte(testCase.initial), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			bootstrap := runtimeV2PatchVariantBootstrap(testCase.variant, testCase.target)
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
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
			candidateCalls := 0
			llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				candidateCalls++
				command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
				result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: testCase.candidate()}, nil)
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
				t.Fatalf("seal patch variant before crash aggregate=%+v err=%v", aggregate, err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != "runtime-v2-test-crash-after-owner-receipt" {
						t.Fatalf("unexpected patch variant crash marker: %v", recovered)
					}
				}()
				_, _ = module.bizPtr.ExecuteRuntimeV2Dispatch(ctx, "ws-v2", "run-v2", panicAfterOwnerReceiptExecutor{delegate: eventRuntimeV2OperationExecutor{hub: hub}})
				t.Fatal("expected injected patch variant crash after owner receipt")
			}()
			content, err := os.ReadFile(filepath.Join(deliverable, testCase.target))
			if err != nil || string(content) != testCase.want {
				state, _ := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				var failure *runtimecontract.FailureFact
				if state.RuntimeState != nil && len(state.RuntimeState.OperationReceipts) > 0 {
					failure = state.RuntimeState.OperationReceipts[0].Failure
				}
				t.Fatalf("side effect before patch restart content=%q err=%v want=%q failure=%+v", content, err, testCase.want, failure)
			}
			crashed, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil || crashed.RuntimeState.ActiveDispatch == nil || len(crashed.RuntimeState.OperationReceipts) != 1 || crashed.RuntimeState.EvidenceSequence != 0 {
				t.Fatalf("patch receipt crash boundary=%+v err=%v", crashed, err)
			}
			execution := crashed.RuntimeState.ActiveDispatch.OperationExecutions[0]
			if execution.State != runtimecontract.OperationExecutionOwnerCommitted || execution.Fence == 0 {
				t.Fatalf("patch receipt crash execution=%+v", execution)
			}
			restarted := newRuntimeV2DriveTestModule(t, hub)
			driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
			driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restarted.handleRuntimeV2Drive)
			t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
			completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
			if err != nil || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 1 {
				t.Fatalf("patch restart completion=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
			}
			persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || persisted.RuntimeState == nil || len(persisted.RuntimeState.OperationReceipts) != 1 || len(persisted.RuntimeState.Facts.Evidence.Accepted) != 1 {
				t.Fatalf("patch restart persisted=%+v err=%v", persisted, err)
			}
			receipt := persisted.RuntimeState.OperationReceipts[0]
			if receipt.ExecutionKey != execution.ExecutionKey || receipt.Fence != execution.Fence || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
				t.Fatalf("patch variant restart replaced receipt: before=%+v after=%+v", execution, receipt)
			}
		})
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
		{ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, TargetRefs: []string{"focus-1.txt"}, RequireAssessment: true},
		{ID: "focus-2", SliceID: "slice-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, TargetRefs: []string{".env"}, RequireAssessment: true},
	}
	bootstrap.Slices = []runtimecontract.RuntimeAcceptanceSlice{
		{ID: "slice-1", Title: "Focus 1", Scope: "test", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
		{ID: "slice-2", Title: "Focus 2", Scope: "test", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
	}
	bootstrap.CoverageItems = []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1"}, {ID: "cwi-2", SliceID: "slice-2"}}
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
	module := newTestBiz(moduleBiz, hub, nil)
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
	deliveryModule := newTestBiz(deliveryBiz, hub, nil)
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
		receipt := orchestratorevents.NewRuntimeOperationReceipt(command.Execution, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusAccepted, command.TargetRefs, runtimecontract.RuntimeOperationEvidence{Summary: "状态变更已完成"}, nil, nil, "2026-07-20T00:00:00Z")
		committed, err := commitRuntimeOperationReceiptForTest(event.Context(), hub, runtimecontract.RuntimeModuleFile, command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
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
	// Live-equivalent shape: an accepted durable operation invalidated mutable
	// prerequisite observations, so freshness reacquisition debt remains while a
	// later candidate contract consumes its bounded attempt 0/1 budget.
	facts.FreshnessReacquireCoverage = append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...)
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
				Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_response_truncated",
				Signature: "candidate-response-truncated", Reason: "provider response ended before candidate JSON completed: unexpected EOF",
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
		requests[1].PreviousFailure.Class != runtimecontract.RuntimeCandidateFailureClassSchema || requests[1].PreviousFailure.Code != "candidate_response_truncated" ||
		requests[1].PreviousFailure.Signature != "candidate-response-truncated" || requests[1].PreviousFailure.Reason != "provider response ended before candidate JSON completed: unexpected EOF" {
		t.Fatalf("candidate retry did not carry the persisted first failure: %+v", requests)
	}
}

func TestRequestAndSealRuntimeV2CandidateContinuesProviderLimitedOutput(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Activation.ExecutionContract.CandidateOutputContinuation = runtimecontract.RuntimeCandidateOutputContinuationPolicy{MaxChunks: 2, TailBytes: 12}
	repository := &inMemoryRuntimeV2StateRepository{}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	continuations := memory.NewArtifactContinuationRepository()
	owner := NewRuntimeOrchestratorWithResolutionsAndEvidence(nil, nil, nil, nil, nil, RuntimeEvidenceRepositories{ArtifactContinuations: continuations})
	if err := owner.ConfigureRuntimeV2Repository(repository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	first := `{"operations":[{"id":"op-1","kind":"file/edit","coverageWorkItemIds":["cwi-v2"],`
	second := `"targetRefs":["target.txt"],"parameters":[{"name":"path","string":"target.txt"},{"name":"oldText","string":"old"},{"name":"newText","string":"new"}]}]}`
	calls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		calls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		response := orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction}
		if command.OutputContinuation == nil {
			response.OutputFragment = &runtimecontract.RuntimeCandidateOutputFragment{Content: []byte(first), ReachedProviderLimit: true}
		} else {
			if command.OutputContinuation.Sequence != 2 || command.OutputContinuation.PreviousChunkTail != first[len(first)-12:] || command.StablePrefixHash != prefix {
				t.Fatalf("continuation command = %+v", command)
			}
			response.OutputFragment = &runtimecontract.RuntimeCandidateOutputFragment{Content: []byte(second)}
		}
		result.Set(response, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	completed, err := module.requestAndSealRuntimeV2CandidateWithRetry(ctx, initialized, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || completed.ActiveDispatch == nil || len(completed.CandidateAttempts) != 1 || completed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
		t.Fatalf("candidate output continuation completion = %+v calls=%d", completed, calls)
	}
	id := runtimecontract.StableRuntimeCandidateOutputContinuationID(completed.Facts.WorkspaceID, completed.Facts.RunID, initialized.Decision.AuthorityTransaction(), 0)
	continuation, err := continuations.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if continuation.State != common.ArtifactContinuationStateApplied || !continuation.FinalReceived || len(continuation.ReceivedSequences) != 2 || continuation.AuthorityTransaction != initialized.Decision.AuthorityTransaction() || continuation.StablePrefixHash != prefix {
		t.Fatalf("persisted candidate output continuation = %+v", continuation)
	}
}

func TestRequestAndSealRuntimeV2CandidateRestartsFromPersistedOutputChunk(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Activation.ExecutionContract.CandidateOutputContinuation = runtimecontract.RuntimeCandidateOutputContinuationPolicy{MaxChunks: 2, TailBytes: 12}
	state := &inMemoryRuntimeV2StateRepository{}
	controller, err := runtimev2.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	continuations := memory.NewArtifactContinuationRepository()
	owner := NewRuntimeOrchestratorWithResolutionsAndEvidence(nil, nil, nil, nil, nil, RuntimeEvidenceRepositories{ArtifactContinuations: continuations})
	if err := owner.ConfigureRuntimeV2Repository(state); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	pending, err := owner.BeginRuntimeV2CandidateAttempt(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, 0, prefix)
	if err != nil {
		t.Fatal(err)
	}
	policy := pending.Facts.Activation.ExecutionContract.CandidateOutputContinuation
	id := runtimecontract.StableRuntimeCandidateOutputContinuationID(pending.Facts.WorkspaceID, pending.Facts.RunID, pending.Decision.AuthorityTransaction(), 0)
	continuation, err := loadOrCreateRuntimeCandidateOutputContinuation(ctx, continuations, id, pending, 0, prefix, policy)
	if err != nil {
		t.Fatal(err)
	}
	first := `{"operations":[{"id":"op-1","kind":"file/edit","coverageWorkItemIds":["cwi-v2"],`
	if err := recordRuntimeCandidateOutputChunk(ctx, continuations, continuation, 1, []byte(first), ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reconcileRuntimeCandidateOutputContinuation(ctx, continuations, continuation); err != nil {
		t.Fatal(err)
	}
	calls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		calls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		if command.OutputContinuation == nil || command.OutputContinuation.Sequence != 2 || command.OutputContinuation.PreviousChunkTail != first[len(first)-12:] {
			t.Fatalf("restart continuation command = %+v", command)
		}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, OutputFragment: &runtimecontract.RuntimeCandidateOutputFragment{Content: []byte(`"targetRefs":["target.txt"],"parameters":[{"name":"path","string":"target.txt"},{"name":"oldText","string":"old"},{"name":"newText","string":"new"}]}]}`)}}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	completed, err := module.requestAndSealRuntimeV2CandidateWithRetry(ctx, pending, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || completed.ActiveDispatch == nil || completed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
		t.Fatalf("restart continuation completion = %+v calls=%d", completed, calls)
	}
}

func TestRequestAndSealRuntimeV2CandidateFailsWhenOutputContinuationBudgetExhausted(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Activation.ExecutionContract.CandidateOutputContinuation = runtimecontract.RuntimeCandidateOutputContinuationPolicy{MaxChunks: 1, TailBytes: 8}
	state := &inMemoryRuntimeV2StateRepository{}
	controller, err := runtimev2.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	continuations := memory.NewArtifactContinuationRepository()
	owner := NewRuntimeOrchestratorWithResolutionsAndEvidence(nil, nil, nil, nil, nil, RuntimeEvidenceRepositories{ArtifactContinuations: continuations})
	if err := owner.ConfigureRuntimeV2Repository(state); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		fragment := runtimecontract.RuntimeCandidateOutputFragment{Content: []byte(`{"operations":[`), ReachedProviderLimit: true}
		if command.OutputContinuation != nil {
			fragment.Content = []byte(`{"id":"op-1"`)
		}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, OutputFragment: &fragment}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	failed, err := module.requestAndSealRuntimeV2CandidateWithRetry(ctx, initialized, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Decision.Action != runtimecontract.RuntimeActionFail || failed.Decision.Terminal == nil || failed.Decision.Terminal.Code != "candidate_output_continuation_exhausted" {
		t.Fatalf("candidate output continuation exhaustion = %+v", failed.Decision)
	}
	id := runtimecontract.StableRuntimeCandidateOutputContinuationID(failed.Facts.WorkspaceID, failed.Facts.RunID, initialized.Decision.AuthorityTransaction(), 0)
	continuation, err := continuations.Get(ctx, id)
	if err != nil || continuation.State != common.ArtifactContinuationStateFailed {
		t.Fatalf("failed continuation = %+v err=%v", continuation, err)
	}
}

func TestRequestAndSealRuntimeV2CandidateReacquiresProviderCeilingAfterTypedStale(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeV2Aggregate(t, 1).Facts
	ceiling := runtimecontract.ProviderCeiling{
		Target:                   runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 4096,
		MaxInputTokens:           92928,
		MaxBytes:                 96 * 1024,
		CapabilityRevision:       "capability-v1",
		ConfigurationRevision:    "config-v1",
	}
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: "provider_ceiling:llm", Kind: runtimecontract.PrerequisiteKindProviderCeiling,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM, MinimumBytes: 32 * 1024,
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: "provider-ceiling-v1",
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
			SubjectRef: "ai-proxy/gpt-4o", Revision: ceiling.ConfigurationRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Resources.ProviderCeiling = &ceiling
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	repository := &inMemoryRuntimeV2StateRepository{}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	owner := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := owner.ConfigureRuntimeV2Repository(repository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		SubjectRef: "ai-proxy/gpt-4o", ExpectedRevision: "config-v1", ObservedRevision: "config-v2",
		Reason: "provider configuration changed",
	}
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Stale: &stale}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	invalidated, err := module.requestAndSealRuntimeV2Candidate(ctx, initialized, "stable-prefix-provider-config", 0)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite ||
		len(invalidated.CandidateAttempts) != 1 || invalidated.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated ||
		invalidated.CandidateAttempts[0].Failure != nil || invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 ||
		len(invalidated.Facts.Prerequisites.Pending) != 1 || invalidated.Facts.Resources.ProviderCeiling != nil ||
		len(invalidated.Facts.Evidence.Accepted) != 0 || len(invalidated.Facts.Evidence.Rejected) != 0 ||
		invalidated.Facts.Repair.RepairAttempts != 0 || invalidated.Facts.Recovery.ProviderTransportAttempts != 0 {
		t.Fatalf("provider configuration stale did not return to clean prerequisite acquisition: %+v", invalidated)
	}
}

func startRuntimeV2FileManifestOwner(t *testing.T, ctx context.Context, hub magicevent.Hub) {
	t.Helper()
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
}

func newRuntimeV2DriveTestModule(t *testing.T, hub magicevent.Hub) *Biz {
	t.Helper()
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	return newTestBiz(biz, hub, nil)
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
	module := newTestBiz(biz, hub, nil)
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
