package orchestrator

// Owner Biz projection tests.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	resultbiz "github.com/muidea/workorch/internal/modules/blocks/result/biz"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	workspacebiz "github.com/muidea/workorch/internal/modules/blocks/workspace/biz"

	magicevent "github.com/muidea/magicCommon/event"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodule "github.com/muidea/workorch/internal/modules/blocks/workspace"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// failAfterResultCreateRepository simulates a process/transport interruption
// after Result durability but before the owner can acknowledge the command.
// The EventHub deliberately recovers handler panics, so returning an error
// gives this file-backed test the same persisted boundary without test logs
// depending on panic recovery internals.
type failAfterResultCreateRepository struct {
	repository.ResultRepository
	fired bool
}

func (r *failAfterResultCreateRepository) Get(ctx context.Context, id common.ResultID) (*resultmodel.Result, error) {
	if r.fired {
		return nil, common.ErrConflict
	}
	return r.ResultRepository.Get(ctx, id)
}

func startTerminalDeliveryResourceOwners(t *testing.T, ctx context.Context, hub magicevent.Hub, runs repository.RunRepository, workspaces repository.WorkspaceRepository) {
	t.Helper()
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspacebiz.Dependencies{Workspaces: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(), Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := workspaceOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
}

func (r *failAfterResultCreateRepository) Create(ctx context.Context, item *resultmodel.Result) error {
	if err := r.ResultRepository.Create(ctx, item); err != nil {
		return err
	}
	if !r.fired {
		r.fired = true
		return common.ErrConflict
	}
	return nil
}

// failBeforeResultCreateRepository keeps the terminal aggregate durable while
// making the first Result delivery unavailable. A fresh Result owner after a
// process restart must create the Result once and continue Run/Workspace
// projection from that persisted aggregate.
type failBeforeResultCreateRepository struct {
	repository.ResultRepository
	fired bool
}

func (r *failBeforeResultCreateRepository) Create(_ context.Context, _ *resultmodel.Result) error {
	r.fired = true
	return common.ErrConflict
}

// These wrappers model the owner-side acknowledgement gap: the owner has
// durably applied its own state update, but the synchronous EventHub command
// cannot report success because the process/transport stops immediately
// afterward.  Restart must therefore replay the same sealed terminal without
// rebuilding Runtime authority or creating a second Result.
type failAfterWorkspaceUpdateRepository struct {
	repository.WorkspaceRepository
	fired bool
}

func (r *failAfterWorkspaceUpdateRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if r.fired {
		return nil, common.ErrConflict
	}
	return r.WorkspaceRepository.Get(ctx, id)
}

func (r *failAfterWorkspaceUpdateRepository) CompareAndSwap(ctx context.Context, item *workspacemodel.Workspace, expectedRevision uint64) error {
	if err := r.WorkspaceRepository.CompareAndSwap(ctx, item, expectedRevision); err != nil {
		return err
	}
	if !r.fired {
		r.fired = true
		return common.ErrConflict
	}
	return nil
}

type failAfterRunUpdateRepository struct {
	repository.RunRepository
	fired bool
}

// The following wrappers fail before a CAS write. They model a genuine
// concurrent owner update (rather than an acknowledgement loss after the
// write) and let the owner-local reducer prove that it replays the same fact.
type failBeforeWorkspaceCASRepository struct {
	repository.WorkspaceRepository
	fired bool
}

func (r *failBeforeWorkspaceCASRepository) CompareAndSwap(ctx context.Context, item *workspacemodel.Workspace, expectedRevision uint64) error {
	if !r.fired {
		r.fired = true
		return common.ErrConflict
	}
	return r.WorkspaceRepository.CompareAndSwap(ctx, item, expectedRevision)
}

type failBeforeRunTerminalCASRepository struct {
	repository.RunRepository
	fired bool
}

func (r *failBeforeRunTerminalCASRepository) CompareAndSwap(ctx context.Context, item *runmodel.Run, expectedRevision uint64) error {
	if !r.fired && item != nil && item.TerminalDelivery != nil {
		r.fired = true
		return common.ErrConflict
	}
	return r.RunRepository.CompareAndSwap(ctx, item, expectedRevision)
}

type resultCreateRaceRepository struct {
	repository.ResultRepository
	fired bool
}

func (r *resultCreateRaceRepository) Create(ctx context.Context, item *resultmodel.Result) error {
	if !r.fired {
		r.fired = true
		if err := r.ResultRepository.Create(ctx, item); err != nil {
			return err
		}
		return common.ErrConflict
	}
	return r.ResultRepository.Create(ctx, item)
}

func (r *failAfterRunUpdateRepository) GetByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID, id common.RunID) (*runmodel.Run, error) {
	if r.fired {
		return nil, common.ErrConflict
	}
	return r.RunRepository.GetByWorkspaceID(ctx, workspaceID, id)
}

func (r *failAfterRunUpdateRepository) CompareAndSwap(ctx context.Context, item *runmodel.Run, expectedRevision uint64) error {
	if err := r.RunRepository.CompareAndSwap(ctx, item, expectedRevision); err != nil {
		return err
	}
	// Runtime aggregate acknowledgement persistence is also stored through
	// the Run owner.  This crash point must model the later SM-1 terminal
	// projection only, otherwise it fires while recording Result/Workspace
	// acknowledgement and never exercises the Run owner boundary it claims to
	// cover.
	if !r.fired && item != nil && item.TerminalDelivery != nil {
		r.fired = true
		return common.ErrConflict
	}
	return nil
}

func TestDeliverRuntimeTerminalCreatesResultAndProjectsOwnersIdempotently(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	resultRepo := memory.NewResultRepository()
	resultOwner := resultmodule.New()
	if err := resultOwner.Configure(resultbiz.Dependencies{
		Results: resultRepo, Artifacts: memory.NewArtifactRepository(), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resultOwner.Stop(ctx) })
	aggregate := runtimeAggregate(t, 2)
	aggregate.Facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_contract_defect",
		Signature: "contract-defect", Message: "sealed contract mismatch",
	}
	decision, err := runtime.ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	record := runtimeStateRecordFromAggregate(aggregate)
	runFixture := runtimeRunForTest(common.RunStateActive, aggregate.Definition)
	runFixture.RuntimeState = &record
	runFixture.RuntimeLifecycleFence = runtimeLifecycleFenceFromModel(runFixture)
	if err := runs.Create(ctx, runFixture); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspacebiz.Dependencies{Workspaces: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(), Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := workspaceOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
	biz := newRuntimeTestOwner()
	module := newTestBiz(biz, hub, nil)
	for attempt := 0; attempt < 2; attempt++ {
		if err := module.deliverRuntimeTerminal(ctx, aggregate); err != nil {
			t.Fatalf("delivery attempt %d: %v", attempt, err)
		}
	}
	item, err := resultRepo.Get(ctx, "ws-v2/run-v2/result/dod-assessment")
	if err != nil || item.State != common.ResultStateRejected {
		t.Fatalf("result = %+v err=%v", item, err)
	}
	run, err := runs.Get(ctx, "run-v2")
	if err != nil || run.State != common.RunStateFailed {
		t.Fatalf("run = %+v err=%v", run, err)
	}
	workspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil || workspace.ActiveRunID != "" {
		t.Fatalf("workspace = %+v err=%v", workspace, err)
	}
}

func TestAdministrativeTerminationDeliveryRequestUsesSealedFailureIdentity(t *testing.T) {
	aggregate := runtimeAggregate(t, 7)
	aggregate.Termination = &runtimecontract.RuntimeTerminationRecord{
		Kind: runtimecontract.RuntimeTerminationSuperseded, RequestedBy: "application/workorchd", Reason: "replacement requested", Revision: 8, RequestedAt: "2026-07-27T00:00:00Z",
	}
	request := terminalDeliveryRequest(aggregate)
	if err := request.Validate(); err != nil {
		t.Fatalf("administrative request: %v", err)
	}
	if request.Outcome.Kind != runtimecontract.TerminalOutcomeKindSuperseded || request.Outcome.Code != "administrative_superseded" ||
		request.Outcome.Summary != "replacement requested" || request.CausationID == "" {
		t.Fatalf("administrative request = %+v", request)
	}
}

// The direct repository fixtures below prove storage crash boundaries. This
// test proves the production owner boundary itself: terminal delivery must use
// Result, Workspace and Run typed EventHub commands, with the persisted
// aggregate remaining the only authority across a duplicate delivery.
func TestDeliverRuntimeTerminalUsesTypedOwnerEventContracts(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	results := memory.NewResultRepository()

	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspacebiz.Dependencies{
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
	if err := resultOwner.Configure(resultbiz.Dependencies{Results: results, Artifacts: memory.NewArtifactRepository(), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resultOwner.Stop(ctx) })

	aggregate := runtimeAggregate(t, 2)
	aggregate.Facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_contract_defect",
		Signature: "contract-defect", Message: "sealed contract mismatch",
	}
	decision, err := runtime.ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	record := runtimeStateRecordFromAggregate(aggregate)
	runFixture := runtimeRunForTest(common.RunStateActive, aggregate.Definition)
	runFixture.RuntimeState = &record
	if err := runs.Create(ctx, runFixture); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}

	owner := New()
	if err := owner.Configure(Dependencies{
		ToolCalls: memory.NewToolCallRepository(), Patches: memory.NewPatchRepository(),
		ValidationResults: memory.NewValidationResultRepository(), EvidenceRecords: memory.NewEvidenceRecordRepository(),
		CoverageWorkItems: memory.NewCoverageWorkItemRepository(), ArtifactContinuations: memory.NewArtifactContinuationRepository(),
		CommandHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := owner.deliverRuntimeTerminal(ctx, aggregate); err != nil {
		items, _ := results.ListByRun(ctx, "run-v2")
		run, _ := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
		workspace, _ := workspaces.Get(ctx, "ws-v2")
		t.Fatalf("typed terminal delivery err=%v results=%+v run=%+v workspace=%+v", err, items, run, workspace)
	}
	if err := owner.deliverRuntimeTerminal(ctx, aggregate); err != nil {
		t.Fatalf("duplicate typed terminal delivery: %v", err)
	}
	items, err := results.ListByRun(ctx, "run-v2")
	if err != nil || len(items) != 1 || items[0].State != common.ResultStateRejected {
		t.Fatalf("Result owner delivery=%+v err=%v", items, err)
	}
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || run.State != common.RunStateFailed {
		t.Fatalf("Run owner terminal projection=%+v err=%v", run, err)
	}
	workspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil || workspace.ActiveRunID != "" {
		t.Fatalf("Workspace owner release=%+v err=%v", workspace, err)
	}
}

// R3/R4: a Result durable write is not the only terminal delivery boundary.
// Exercise the real Run/Workspace owner EventHub handlers with file-backed
// repositories when either owner writes successfully but loses its response.
// The restarted delivery process must converge from the persisted terminal
// aggregate and the partial owner projection, without a new decision,
// operation, evidence or Result.
func TestDeliverRuntimeTerminalRestartsAfterOwnerProjectionAcknowledgementLoss(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name           string
		administrative bool
		wrap           func(repository.RunRepository, repository.WorkspaceRepository) (repository.RunRepository, repository.WorkspaceRepository, func() bool)
	}{
		{
			name: "workspace_release_persisted_before_acknowledgement",
			wrap: func(runs repository.RunRepository, workspaces repository.WorkspaceRepository) (repository.RunRepository, repository.WorkspaceRepository, func() bool) {
				interrupted := &failAfterWorkspaceUpdateRepository{WorkspaceRepository: workspaces}
				return runs, interrupted, func() bool { return interrupted.fired }
			},
		},
		{
			name:           "administrative_supersede_workspace_release_persisted_before_acknowledgement",
			administrative: true,
			wrap: func(runs repository.RunRepository, workspaces repository.WorkspaceRepository) (repository.RunRepository, repository.WorkspaceRepository, func() bool) {
				interrupted := &failAfterWorkspaceUpdateRepository{WorkspaceRepository: workspaces}
				return runs, interrupted, func() bool { return interrupted.fired }
			},
		},
		{
			name: "run_terminal_persisted_before_acknowledgement",
			wrap: func(runs repository.RunRepository, workspaces repository.WorkspaceRepository) (repository.RunRepository, repository.WorkspaceRepository, func() bool) {
				interrupted := &failAfterRunUpdateRepository{RunRepository: runs}
				return interrupted, workspaces, func() bool { return interrupted.fired }
			},
		},
		{
			name:           "administrative_supersede_run_terminal_persisted_before_acknowledgement",
			administrative: true,
			wrap: func(runs repository.RunRepository, workspaces repository.WorkspaceRepository) (repository.RunRepository, repository.WorkspaceRepository, func() bool) {
				interrupted := &failAfterRunUpdateRepository{RunRepository: runs}
				return interrupted, workspaces, func() bool { return interrupted.fired }
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			baseRuns := filerepository.NewRunRepository(root + "/runs.json")
			baseWorkspaces := filerepository.NewWorkspaceRepository(root + "/workspaces.json")
			results := filerepository.NewResultRepository(root + "/results.json")
			firstRuns, firstWorkspaces, fired := tc.wrap(baseRuns, baseWorkspaces)
			aggregate := runtimeOwnerGroundedTerminalAggregate(t)
			if tc.administrative {
				// RequestTermination must be the only source of an administrative
				// terminal record.  Start from a valid, non-terminal aggregate and
				// let the controller CAS-persist the record below.
				aggregate = runtimeAggregate(t, 2)
			}
			record := runtimeStateRecordFromAggregate(aggregate)
			runFixture := runtimeRunForTest(common.RunStateActive, aggregate.Definition)
			runFixture.RuntimeState = &record
			if err := baseRuns.Create(ctx, runFixture); err != nil {
				t.Fatal(err)
			}
			if err := baseWorkspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
				t.Fatal(err)
			}

			firstHub := magicevent.NewHub(32)
			firstRunOwner := runmodule.New()
			if err := firstRunOwner.Configure(runbiz.Dependencies{Runs: firstRuns, EventHub: firstHub}); err != nil {
				t.Fatal(err)
			}
			if err := firstRunOwner.Start(); err != nil {
				t.Fatal(err)
			}
			firstWorkspaceOwner := workspacemodule.New()
			if err := firstWorkspaceOwner.Configure(workspacebiz.Dependencies{
				Workspaces: firstWorkspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(),
				Templates: memory.NewWorkspaceTemplateRepository(), EventHub: firstHub,
			}); err != nil {
				t.Fatal(err)
			}
			if err := firstWorkspaceOwner.Start(); err != nil {
				t.Fatal(err)
			}
			firstResultOwner := resultmodule.New()
			if err := firstResultOwner.Configure(resultbiz.Dependencies{
				Results: results, Artifacts: memory.NewArtifactRepository(), EventHub: firstHub,
			}); err != nil {
				t.Fatal(err)
			}
			if err := firstResultOwner.Start(); err != nil {
				t.Fatal(err)
			}
			firstOrchestrator := New()
			if err := firstOrchestrator.Configure(Dependencies{
				ToolCalls: memory.NewToolCallRepository(), Patches: memory.NewPatchRepository(),
				ValidationResults: memory.NewValidationResultRepository(), EvidenceRecords: memory.NewEvidenceRecordRepository(),
				CoverageWorkItems: memory.NewCoverageWorkItemRepository(), ArtifactContinuations: memory.NewArtifactContinuationRepository(),
				CommandHub: firstHub,
			}); err != nil {
				t.Fatal(err)
			}
			if tc.administrative {
				terminated, err := runtimeTestControllerRequestTermination(firstOrchestrator.runtimeController, ctx, "ws-v2", "run-v2", runtimecontract.RuntimeTerminationSuperseded, "application/workorchd", "replacement requested")
				if err != nil {
					t.Fatalf("persist administrative termination: %v", err)
				}
				aggregate = terminated
			}
			if err := firstOrchestrator.deliverRuntimeTerminal(ctx, aggregate); err == nil {
				t.Fatal("terminal owner acknowledgement loss unexpectedly succeeded")
			}
			if !fired() {
				t.Fatal("injected owner acknowledgement loss was not reached")
			}
			if items, err := results.ListByRun(ctx, "run-v2"); err != nil || len(items) != 1 {
				t.Fatalf("partial delivery Result=%+v err=%v", items, err)
			}
			workspace, workspaceErr := baseWorkspaces.Get(ctx, "ws-v2")
			if workspaceErr != nil {
				t.Fatalf("partial delivery workspace=%+v err=%v", workspace, workspaceErr)
			}
			if strings.Contains(tc.name, "workspace_release") && workspace.ActiveRunID != "" {
				t.Fatalf("workspace transition was not durable before acknowledgement loss: %+v", workspace)
			}
			if strings.Contains(tc.name, "run_terminal") && workspace.ActiveRunID != "" {
				t.Fatalf("workspace release was not durable before Run acknowledgement loss: %+v", workspace)
			}

			if err := firstResultOwner.Stop(ctx); err != nil {
				t.Fatal(err)
			}
			if err := firstWorkspaceOwner.Stop(ctx); err != nil {
				t.Fatal(err)
			}
			if err := firstRunOwner.Stop(ctx); err != nil {
				t.Fatal(err)
			}
			firstHub.Terminate(ctx)

			restartedHub := magicevent.NewHub(32)
			t.Cleanup(func() { restartedHub.Terminate(ctx) })
			restartedRunOwner := runmodule.New()
			if err := restartedRunOwner.Configure(runbiz.Dependencies{Runs: baseRuns, EventHub: restartedHub}); err != nil {
				t.Fatal(err)
			}
			if err := restartedRunOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
			restartedWorkspaceOwner := workspacemodule.New()
			if err := restartedWorkspaceOwner.Configure(workspacebiz.Dependencies{
				Workspaces: baseWorkspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(),
				Templates: memory.NewWorkspaceTemplateRepository(), EventHub: restartedHub,
			}); err != nil {
				t.Fatal(err)
			}
			if err := restartedWorkspaceOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = restartedWorkspaceOwner.Stop(ctx) })
			restartedResultOwner := resultmodule.New()
			if err := restartedResultOwner.Configure(resultbiz.Dependencies{
				Results: results, Artifacts: memory.NewArtifactRepository(), EventHub: restartedHub,
			}); err != nil {
				t.Fatal(err)
			}
			if err := restartedResultOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = restartedResultOwner.Stop(ctx) })
			persisted, err := baseRuns.Get(ctx, "run-v2")
			if err != nil || persisted.RuntimeState == nil {
				t.Fatalf("persisted Run=%+v err=%v", persisted, err)
			}
			restartedAggregate := runtimeAggregateFromRecord(*persisted.RuntimeState)
			if err := restartedAggregate.Validate(); err != nil {
				t.Fatalf("reloaded aggregate: %v", err)
			}
			if tc.administrative {
				assertAdministrativeSupersedeAuthority(t, restartedAggregate)
			} else {
				assertOwnerGroundedTerminalAuthority(t, restartedAggregate)
			}
			terminalRevision := restartedAggregate.StateRevision
			restartedOrchestrator := New()
			if err := restartedOrchestrator.Configure(Dependencies{
				ToolCalls: memory.NewToolCallRepository(), Patches: memory.NewPatchRepository(),
				ValidationResults: memory.NewValidationResultRepository(), EvidenceRecords: memory.NewEvidenceRecordRepository(),
				CoverageWorkItems: memory.NewCoverageWorkItemRepository(), ArtifactContinuations: memory.NewArtifactContinuationRepository(),
				CommandHub: restartedHub,
			}); err != nil {
				t.Fatal(err)
			}
			if err := restartedOrchestrator.deliverRuntimeTerminal(ctx, restartedAggregate); err != nil {
				t.Fatalf("restart terminal delivery: %v", err)
			}
			// The same restarted process may receive a duplicate delivery trigger
			// after the first replay has completed.  It must reuse the persisted
			// terminal authority and the already-created Result rather than
			// allocating another projection or attempting a reducer transition.
			if err := restartedOrchestrator.deliverRuntimeTerminal(ctx, restartedAggregate); err != nil {
				t.Fatalf("duplicate restart terminal delivery: %v", err)
			}
			expectedRunState := common.RunStateFailed
			if tc.administrative {
				expectedRunState = common.RunStateSuperseded
			}
			if run, err := baseRuns.Get(ctx, "run-v2"); err != nil || run.State != expectedRunState {
				t.Fatalf("restart Run=%+v err=%v", run, err)
			}
			if workspace, err := baseWorkspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "" {
				t.Fatalf("restart Workspace=%+v err=%v", workspace, err)
			}
			if items, err := results.ListByRun(ctx, "run-v2"); err != nil || len(items) != 1 || items[0].State != common.ResultStateRejected {
				t.Fatalf("restart Result=%+v err=%v", items, err)
			}
			projectedRun, err := baseRuns.Get(ctx, "run-v2")
			if err != nil || projectedRun.RuntimeState == nil {
				t.Fatalf("projected Run=%+v err=%v", projectedRun, err)
			}
			projectedAggregate := runtimeAggregateFromRecord(*projectedRun.RuntimeState)
			remainingAcknowledgements := uint64(2)
			if strings.Contains(tc.name, "run_terminal") {
				remainingAcknowledgements = 1
			}
			if err := projectedAggregate.Validate(); err != nil || projectedAggregate.StateRevision != terminalRevision+remainingAcknowledgements || len(projectedAggregate.DeliveryAcknowledgements) != 3 {
				t.Fatalf("terminal delivery acknowledgement ledger did not converge exactly once: aggregate=%+v err=%v", projectedAggregate, err)
			}
			if tc.administrative {
				assertAdministrativeSupersedeAuthority(t, projectedAggregate)
			} else {
				assertOwnerGroundedTerminalAuthority(t, projectedAggregate)
			}

			// A delayed owner-catalog stale result belongs to the old candidate
			// transaction. Once terminal authority is persisted there is no
			// pending candidate to invalidate: SM-4 must reject it rather than
			// reopen prerequisite acquisition or erase the sealed observation.
			lateController, err := runtime.NewRuntimeController(runtimeStateRepository{hub: restartedHub})
			if err != nil {
				t.Fatal(err)
			}
			_, lateErr := runtimeTestControllerInvalidateCandidateStale(lateController, ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimePrerequisiteStale{
				PrerequisiteID: "operation_admissibility:blocks/mcp:terminal", ObservationID: "terminal-mcp-catalog-v1",
				Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP,
				SubjectRef: "mcp-terminal-owner", ExpectedRevision: "mcp-catalog-terminal-v1", ObservedRevision: "mcp-catalog-terminal-v2",
				Reason: "late MCP catalog response after terminal projection",
			})
			if !errors.Is(lateErr, common.ErrConflict) {
				t.Fatalf("late stale result after terminal = %v, want conflict", lateErr)
			}
			unchangedRun, err := baseRuns.Get(ctx, "run-v2")
			if err != nil || unchangedRun.RuntimeState == nil {
				t.Fatalf("late stale Run=%+v err=%v", unchangedRun, err)
			}
			unchangedAggregate := runtimeAggregateFromRecord(*unchangedRun.RuntimeState)
			if err := unchangedAggregate.Validate(); err != nil || unchangedAggregate.StateRevision != terminalRevision+remainingAcknowledgements {
				t.Fatalf("late stale result mutated terminal aggregate: aggregate=%+v err=%v", unchangedAggregate, err)
			}
			if tc.administrative {
				assertAdministrativeSupersedeAuthority(t, unchangedAggregate)
			} else {
				assertOwnerGroundedTerminalAuthority(t, unchangedAggregate)
			}
		})
	}
}

func TestDeliverRuntimeTerminalReplaysSameIdentityAfterOwnerCASConflict(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		wrap func(repository.ResultRepository, repository.RunRepository, repository.WorkspaceRepository) (repository.ResultRepository, repository.RunRepository, repository.WorkspaceRepository, func() bool)
	}{
		{
			name: "result_create_race",
			wrap: func(results repository.ResultRepository, runs repository.RunRepository, workspaces repository.WorkspaceRepository) (repository.ResultRepository, repository.RunRepository, repository.WorkspaceRepository, func() bool) {
				racing := &resultCreateRaceRepository{ResultRepository: results}
				return racing, runs, workspaces, func() bool { return racing.fired }
			},
		},
		{
			name: "workspace_release_cas_conflict",
			wrap: func(results repository.ResultRepository, runs repository.RunRepository, workspaces repository.WorkspaceRepository) (repository.ResultRepository, repository.RunRepository, repository.WorkspaceRepository, func() bool) {
				conflicting := &failBeforeWorkspaceCASRepository{WorkspaceRepository: workspaces}
				return results, runs, conflicting, func() bool { return conflicting.fired }
			},
		},
		{
			name: "run_terminal_cas_conflict",
			wrap: func(results repository.ResultRepository, runs repository.RunRepository, workspaces repository.WorkspaceRepository) (repository.ResultRepository, repository.RunRepository, repository.WorkspaceRepository, func() bool) {
				conflicting := &failBeforeRunTerminalCASRepository{RunRepository: runs}
				return results, conflicting, workspaces, func() bool { return conflicting.fired }
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hub := magicevent.NewHub(32)
			t.Cleanup(func() { hub.Terminate(ctx) })
			baseResults := memory.NewResultRepository()
			baseRuns := memory.NewRunRepository()
			baseWorkspaces := memory.NewWorkspaceRepository()
			results, runs, workspaces, fired := tc.wrap(baseResults, baseRuns, baseWorkspaces)
			aggregate := runtimeOwnerGroundedTerminalAggregate(t)
			record := runtimeStateRecordFromAggregate(aggregate)
			runFixture := runtimeRunForTest(common.RunStateActive, aggregate.Definition)
			runFixture.RuntimeState = &record
			if err := baseRuns.Create(ctx, runFixture); err != nil {
				t.Fatal(err)
			}
			if err := baseWorkspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
				t.Fatal(err)
			}
			runOwner := runmodule.New()
			if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runOwner.Stop(ctx) })
			workspaceOwner := workspacemodule.New()
			if err := workspaceOwner.Configure(workspacebiz.Dependencies{Workspaces: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(), Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := workspaceOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
			resultOwner := resultmodule.New()
			if err := resultOwner.Configure(resultbiz.Dependencies{Results: results, Artifacts: memory.NewArtifactRepository(), EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := resultOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = resultOwner.Stop(ctx) })
			orchestratorOwner := New()
			if err := orchestratorOwner.Configure(Dependencies{
				ToolCalls: memory.NewToolCallRepository(), Patches: memory.NewPatchRepository(),
				ValidationResults: memory.NewValidationResultRepository(), EvidenceRecords: memory.NewEvidenceRecordRepository(),
				CoverageWorkItems: memory.NewCoverageWorkItemRepository(), ArtifactContinuations: memory.NewArtifactContinuationRepository(),
				CommandHub: hub,
			}); err != nil {
				t.Fatal(err)
			}
			if err := orchestratorOwner.deliverRuntimeTerminal(ctx, aggregate); err != nil {
				t.Fatalf("terminal delivery after CAS conflict: %v", err)
			}
			if !fired() {
				t.Fatal("injected owner CAS conflict was not reached")
			}
			if items, err := baseResults.ListByRun(ctx, "run-v2"); err != nil || len(items) != 1 || items[0].State != common.ResultStateRejected {
				t.Fatalf("Result after CAS replay=%+v err=%v", items, err)
			}
			if run, err := baseRuns.Get(ctx, "run-v2"); err != nil || run.State != common.RunStateFailed {
				t.Fatalf("Run after CAS replay=%+v err=%v", run, err)
			}
			if workspace, err := baseWorkspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "" {
				t.Fatalf("Workspace after CAS replay=%+v err=%v", workspace, err)
			}
			persisted, err := baseRuns.Get(ctx, "run-v2")
			if err != nil || persisted.RuntimeState == nil {
				t.Fatalf("persisted aggregate after CAS replay=%+v err=%v", persisted, err)
			}
			projected := runtimeAggregateFromRecord(*persisted.RuntimeState)
			if err := projected.Validate(); err != nil || len(projected.DeliveryAcknowledgements) != 3 {
				t.Fatalf("CAS replay changed terminal authority or acknowledgement ledger: aggregate=%+v err=%v", projected, err)
			}
			assertOwnerGroundedTerminalAuthority(t, projected)
		})
	}
}

func runtimeTerminalAggregate(t *testing.T) runtime.RuntimeAggregate {
	t.Helper()
	aggregate := runtimeAggregate(t, 2)
	aggregate.Facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_contract_defect",
		Signature: "contract-defect", Message: "sealed contract mismatch",
	}
	decision, err := runtime.ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("terminal aggregate: %v", err)
	}
	return aggregate
}

// runtimeOwnerGroundedTerminalAggregate models a terminal that already
// contains a compatible capability-owner observation. Terminal delivery must
// only project this committed authority; Result, Workspace and Run owners may
// not clear, reinterpret or rebuild it after an acknowledgement loss.
func runtimeOwnerGroundedTerminalAggregate(t *testing.T) runtime.RuntimeAggregate {
	t.Helper()
	aggregate := runtimeTerminalAggregate(t)
	observation := &runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID:    runtimecontract.RuntimeModuleMCP,
		SubjectRef:       "mcp-terminal-owner",
		TargetRole:       "implementation",
		ObservedRevision: "mcp-catalog-terminal-v1",
		TargetRefs:       []string{"implementation.go"},
	}
	aggregate.Facts.Failure.TargetRefs = []string{"implementation.go"}
	aggregate.Facts.Failure.TargetObservation = observation
	aggregate.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{
		Signature:         aggregate.Facts.Failure.Signature,
		TargetRefs:        []string{"implementation.go"},
		TargetObservation: observation,
	}
	decision, err := runtime.ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("owner-grounded terminal aggregate: %v", err)
	}
	return aggregate
}

func assertOwnerGroundedTerminalAuthority(t *testing.T, aggregate runtime.RuntimeAggregate) {
	t.Helper()
	target := aggregate.Facts.Resources.FailureTarget
	if aggregate.Decision.Terminal == nil || aggregate.Facts.Failure == nil || target == nil || target.TargetObservation == nil ||
		target.Signature != aggregate.Facts.Failure.Signature || len(target.TargetRefs) != 1 || target.TargetRefs[0] != "implementation.go" ||
		target.TargetObservation.OwnerModuleID != runtimecontract.RuntimeModuleMCP || target.TargetObservation.SubjectRef != "mcp-terminal-owner" ||
		target.TargetObservation.TargetRole != "implementation" || target.TargetObservation.ObservedRevision != "mcp-catalog-terminal-v1" {
		t.Fatalf("terminal projection lost owner-grounded authority: aggregate=%+v", aggregate)
	}
}

func assertAdministrativeSupersedeAuthority(t *testing.T, aggregate runtime.RuntimeAggregate) {
	t.Helper()
	if aggregate.Termination == nil || aggregate.Termination.Kind != runtimecontract.RuntimeTerminationSuperseded ||
		aggregate.Termination.RequestedBy != "application/workorchd" || aggregate.Termination.Reason != "replacement requested" ||
		aggregate.Decision.Terminal != nil {
		t.Fatalf("administrative terminal projection changed sealed termination authority: aggregate=%+v", aggregate)
	}
}

// Terminal delivery recovery is scheduling metadata only. Its bounded ledger
// must never mutate the already-sealed terminal authority; startup first gets
// only the Run-owned opaque record and classifies terminal delivery after load.
func TestRuntimeTerminalDeliveryBudgetExhaustionRetainsStartupRecoveryInventory(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runs := filerepository.NewRunRepository(root + "/runs.json")
	aggregate := runtimeTerminalAggregate(t)
	aggregate.Facts.RecoveryPolicy.MaxTerminalDeliveryAttempts = 1
	decision, err := runtime.ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("terminal aggregate with bounded delivery policy: %v", err)
	}
	record := runtimeStateRecordFromAggregate(aggregate)
	run := runtimeRunForTest(common.RunStateActive, aggregate.Definition)
	run.RuntimeState = &record
	if err := runs.Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	sameTerminalDecision := func(value runtimecontract.RuntimeDecision) bool {
		return value.State == aggregate.Decision.State &&
			value.AuthoritySnapshot.ID == aggregate.Decision.AuthoritySnapshot.ID &&
			value.Terminal != nil && aggregate.Decision.Terminal != nil &&
			value.Terminal.Code == aggregate.Decision.Terminal.Code &&
			value.Terminal.Summary == aggregate.Decision.Terminal.Summary
	}

	firstHub := magicevent.NewHub(16)
	firstRunOwner := runmodule.New()
	if err := firstRunOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: firstHub}); err != nil {
		t.Fatal(err)
	}
	if err := firstRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	firstOrchestrator := newRuntimeTestOwner()
	if err := firstOrchestrator.ConfigureRuntimeRepository(runtimeStateRepository{hub: firstHub}); err != nil {
		t.Fatal(err)
	}
	charged, consumed, err := firstOrchestrator.ConsumeRuntimeRecoveryBudget(ctx, "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetTerminalDelivery)
	if err != nil || !consumed || charged.Facts.Recovery.TerminalDeliveryAttempts != 1 || !sameTerminalDecision(charged.Decision) {
		t.Fatalf("first delivery charge aggregate=%+v consumed=%v err=%v", charged, consumed, err)
	}
	exhausted, consumed, err := firstOrchestrator.ConsumeRuntimeRecoveryBudget(ctx, "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetTerminalDelivery)
	if err != nil || consumed || exhausted.StateRevision != charged.StateRevision || !sameTerminalDecision(exhausted.Decision) || exhausted.Facts.Recovery.TerminalDeliveryAttempts != 1 {
		t.Fatalf("exhausted delivery charge aggregate=%+v consumed=%v err=%v", exhausted, consumed, err)
	}
	if err := firstRunOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	firstHub.Terminate(ctx)

	restartedHub := magicevent.NewHub(16)
	t.Cleanup(func() { restartedHub.Terminate(ctx) })
	restartedRunOwner := runmodule.New()
	if err := restartedRunOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	inventory, err := queryRuntimeRecoveryInventory(ctx, restartedHub)
	if err != nil || len(inventory) != 1 || inventory[0].State != common.RunStateActive || !inventory[0].HasRuntimeState ||
		inventory[0].RuntimeStateRevision != charged.StateRevision || inventory[0].RuntimeStateRef != "runtime-state/ws-v2/run-v2/"+fmt.Sprint(charged.StateRevision) {
		t.Fatalf("terminal startup inventory=%+v err=%v", inventory, err)
	}
	persisted, err := runs.Get(ctx, "run-v2")
	if err != nil || persisted.RuntimeState == nil || !sameTerminalDecision(persisted.RuntimeState.Decision) || persisted.RuntimeState.Facts.Recovery.TerminalDeliveryAttempts != 1 {
		t.Fatalf("persisted terminal authority=%+v err=%v", persisted, err)
	}
}

// A terminal-delivery-pending inventory item must not merely be visible to a
// restarted process. Startup recovery has to schedule the ordinary SM-4 drive
// and replay the already persisted terminal through the production Result ->
// Workspace -> Run owner chain.  This is deliberately file-backed: no
// process-local aggregate or direct repository write may supply the recovery
// authority.
func TestRuntimeStartupRecoveryDeliversPendingTerminalThroughOwnerChain(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runs := filerepository.NewRunRepository(root + "/runs.json")
	workspaces := filerepository.NewWorkspaceRepository(root + "/workspaces.json")
	results := filerepository.NewResultRepository(root + "/results.json")
	aggregate := runtimeTerminalAggregate(t)
	aggregate.Facts.RecoveryPolicy.MaxTerminalDeliveryAttempts = 1
	aggregate.Facts.Recovery.TerminalDeliveryAttempts = 1
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("pending terminal aggregate: %v", err)
	}
	record := runtimeStateRecordFromAggregate(aggregate)
	run := runtimeRunForTest(common.RunStateActive, aggregate.Definition)
	run.RuntimeState = &record
	if err := runs.Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}

	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspacebiz.Dependencies{
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
	if err := resultOwner.Configure(resultbiz.Dependencies{
		Results: results, Artifacts: memory.NewArtifactRepository(), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resultOwner.Stop(ctx) })

	owner := New()
	if err := owner.Configure(Dependencies{
		ToolCalls: memory.NewToolCallRepository(), Patches: memory.NewPatchRepository(),
		ValidationResults: memory.NewValidationResultRepository(), EvidenceRecords: memory.NewEvidenceRecordRepository(),
		CoverageWorkItems: memory.NewCoverageWorkItemRepository(), ArtifactContinuations: memory.NewArtifactContinuationRepository(),
		CommandHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatalf("startup recovery delivery: %v", err)
	}
	t.Cleanup(func() { _ = owner.Stop(ctx) })

	items, err := results.ListByRun(ctx, "run-v2")
	if err != nil || len(items) != 1 || items[0].State != common.ResultStateRejected {
		t.Fatalf("startup recovery Result=%+v err=%v", items, err)
	}
	workspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil || workspace.ActiveRunID != "" {
		t.Fatalf("startup recovery Workspace=%+v err=%v", workspace, err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.State != common.RunStateFailed || persisted.RuntimeState == nil {
		t.Fatalf("startup recovery Run=%+v err=%v", persisted, err)
	}
	if persisted.RuntimeState.Facts.Recovery.TerminalDeliveryAttempts != 1 || persisted.RuntimeState.Decision.AuthoritySnapshot.ID != aggregate.Decision.AuthoritySnapshot.ID ||
		persisted.RuntimeState.Decision.Terminal == nil || aggregate.Decision.Terminal == nil || persisted.RuntimeState.Decision.Terminal.Code != aggregate.Decision.Terminal.Code {
		t.Fatalf("startup recovery rewrote terminal authority: %+v", persisted.RuntimeState)
	}
	if inventory, err := queryRuntimeRecoveryInventory(ctx, hub); err != nil || len(inventory) != 0 {
		t.Fatalf("terminal delivery remained in recovery inventory=%+v err=%v", inventory, err)
	}
}

// A prerequisite refresh is a recoverable SM-2/SM-3 transition until the
// reducer persists its own refresh-exhausted terminal.  SM-4 delivery must
// reject that aggregate before it can create a rejected Result or project a
// terminal Run/Workspace lifecycle.
func TestDeliverRuntimeTerminalRejectsRecoverablePrerequisiteRefresh(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	resultRepo := memory.NewResultRepository()
	resultOwner := resultmodule.New()
	if err := resultOwner.Configure(resultbiz.Dependencies{
		Results: resultRepo, Artifacts: memory.NewArtifactRepository(), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resultOwner.Stop(ctx) })

	aggregate := runtimeAggregate(t, 2)
	// This is the persisted post-invalidation shape: the affected mutable
	// prerequisite is pending, the refresh ledger is below its sealed limit,
	// and only the reducer may later decide whether it becomes terminal.
	aggregate.Facts.Prerequisites.Pending = []runtimecontract.RuntimePrerequisite{{
		ID: "operation_admissibility:file", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit},
	}}
	aggregate.Facts.Recovery.PrerequisiteRefreshes = 1
	aggregate.Facts.RecoveryPolicy.MaxPrerequisiteRefreshes = 2
	decision, err := runtime.ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || decision.Terminal != nil {
		t.Fatalf("recoverable refresh decision = %+v", decision)
	}
	aggregate.Decision = decision
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("recoverable aggregate must remain valid: %v", err)
	}

	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	record := runtimeStateRecordFromAggregate(aggregate)
	bootstrap := runtimeBootstrap()
	runFixture := &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive,
		RuntimeBootstrap: &bootstrap, RuntimeState: &record,
	}
	runFixture.RuntimeLifecycleFence = runtimeLifecycleFenceFromModel(runFixture)
	if err := runs.Create(ctx, runFixture); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}

	module := newTestBiz(newRuntimeTestOwner(), hub, nil)
	if err := module.deliverRuntimeTerminal(ctx, aggregate); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("recoverable refresh terminal delivery error = %v, want invalid argument", err)
	}
	if _, err := resultRepo.Get(ctx, "ws-v2/run-v2/result/dod-assessment"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("recoverable refresh created Result: %v", err)
	}
	if run, err := runs.Get(ctx, "run-v2"); err != nil || run.State != common.RunStateActive {
		t.Fatalf("recoverable refresh projected Run terminal: %+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "run-v2" {
		t.Fatalf("recoverable refresh released Workspace: %+v err=%v", workspace, err)
	}
}

func TestDeliverRuntimeTerminalRestartsAfterResultPersistenceBeforeOwnerProjection(t *testing.T) {
	ctx := context.Background()
	firstHub := magicevent.NewHub(32)
	root := t.TempDir()
	resultRepo := filerepository.NewResultRepository(root + "/results.json")
	interruptedRepo := &failAfterResultCreateRepository{ResultRepository: resultRepo}
	resultOwner := resultmodule.New()
	if err := resultOwner.Configure(resultbiz.Dependencies{
		Results: interruptedRepo, Artifacts: filerepository.NewArtifactRepository(root + "/artifacts.json"), EventHub: firstHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}

	aggregate := runtimeAggregate(t, 2)
	aggregate.Facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_contract_defect",
		Signature: "contract-defect", Message: "sealed contract mismatch",
	}
	decision, err := runtime.ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	runs := filerepository.NewRunRepository(root + "/runs.json")
	workspaces := filerepository.NewWorkspaceRepository(root + "/workspaces.json")
	record := runtimeStateRecordFromAggregate(aggregate)
	bootstrap := runtimeBootstrap()
	runFixture := &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive, RuntimeBootstrap: &bootstrap, RuntimeState: &record}
	runFixture.RuntimeLifecycleFence = runtimeLifecycleFenceFromModel(runFixture)
	if err := runs.Create(ctx, runFixture); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	firstModule := newTestBiz(newRuntimeTestOwner(), firstHub, nil)
	if err := firstModule.deliverRuntimeTerminal(ctx, aggregate); err == nil {
		t.Fatal("expected Result-persisted delivery interruption")
	}
	if !interruptedRepo.fired {
		t.Fatal("Result persistence crash point was not reached")
	}
	resultID := common.ResultID("ws-v2/run-v2/result/dod-assessment")
	if item, err := resultRepo.Get(ctx, resultID); err != nil || item.State != common.ResultStateRejected {
		t.Fatalf("persisted Result=%+v err=%v", item, err)
	}
	if run, err := runs.Get(ctx, "run-v2"); err != nil || run.State != common.RunStateActive {
		t.Fatalf("Run changed before restart projection: %+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "run-v2" {
		t.Fatalf("Workspace changed before restart projection: %+v err=%v", workspace, err)
	}
	if err := resultOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	firstHub.Terminate(ctx)

	// The Result has already reached durable storage. Recreate the complete
	// delivery process and reload the terminal aggregate before asking Result
	// for the idempotent create and Run/Workspace projection.
	restartedHub := magicevent.NewHub(32)
	t.Cleanup(func() { restartedHub.Terminate(ctx) })
	restartedOwner := resultmodule.New()
	if err := restartedOwner.Configure(resultbiz.Dependencies{
		Results: resultRepo, Artifacts: filerepository.NewArtifactRepository(root + "/artifacts.json"), EventHub: restartedHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := restartedOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedOwner.Stop(ctx) })
	startTerminalDeliveryResourceOwners(t, ctx, restartedHub, runs, workspaces)
	persistedRun, err := runs.Get(ctx, "run-v2")
	if err != nil || persistedRun.RuntimeState == nil {
		t.Fatalf("persisted terminal Run=%+v err=%v", persistedRun, err)
	}
	restartedAggregate := runtimeAggregateFromRecord(*persistedRun.RuntimeState)
	if err := restartedAggregate.Validate(); err != nil {
		t.Fatalf("reloaded terminal aggregate: %v", err)
	}
	restartedModule := newTestBiz(newRuntimeTestOwner(), restartedHub, nil)
	if err := restartedModule.deliverRuntimeTerminal(ctx, restartedAggregate); err != nil {
		t.Fatal(err)
	}
	if run, err := runs.Get(ctx, "run-v2"); err != nil || run.State != common.RunStateFailed {
		t.Fatalf("Run restart projection=%+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "" {
		t.Fatalf("Workspace restart projection=%+v err=%v", workspace, err)
	}
	items, err := resultRepo.ListByRun(ctx, "run-v2")
	if err != nil || len(items) != 1 {
		t.Fatalf("Result restart delivery duplicated item=%+v err=%v", items, err)
	}
}

// Administrative termination is a persisted SM-4 authority, not a direct Run
// state change. If Result reaches durability but its acknowledgement is lost,
// a new process must reuse that same termination record and complete the
// ordinary Result -> Workspace -> Run sequence without producing another
// Result or changing the administrative kind.
func TestAdministrativeSupersedeRestartsAfterResultPersistenceBeforeOwnerProjection(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	firstHub := magicevent.NewHub(32)
	resultRepo := filerepository.NewResultRepository(root + "/results.json")
	interruptedResults := &failAfterResultCreateRepository{ResultRepository: resultRepo}
	runs := filerepository.NewRunRepository(root + "/runs.json")
	workspaces := filerepository.NewWorkspaceRepository(root + "/workspaces.json")

	aggregate := runtimeAggregate(t, 2)
	record := runtimeStateRecordFromAggregate(aggregate)
	runFixture := runtimeRunForTest(common.RunStateActive, aggregate.Definition)
	runFixture.RuntimeState = &record
	if err := runs.Create(ctx, runFixture); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	firstRunOwner := runmodule.New()
	if err := firstRunOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: firstHub}); err != nil {
		t.Fatal(err)
	}
	if err := firstRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	firstResultOwner := resultmodule.New()
	if err := firstResultOwner.Configure(resultbiz.Dependencies{Results: interruptedResults, Artifacts: filerepository.NewArtifactRepository(root + "/artifacts.json"), EventHub: firstHub}); err != nil {
		t.Fatal(err)
	}
	if err := firstResultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	firstOrchestrator := newRuntimeTestOwner()
	if err := firstOrchestrator.ConfigureRuntimeRepository(runtimeStateRepository{hub: firstHub}); err != nil {
		t.Fatal(err)
	}
	terminated, err := runtimeTestControllerRequestTermination(firstOrchestrator.runtimeController, ctx, "ws-v2", "run-v2", runtimecontract.RuntimeTerminationSuperseded, "application/workorchd", "replacement requested")
	if err != nil {
		t.Fatalf("persist administrative termination: %v", err)
	}
	firstModule := newTestBiz(firstOrchestrator, firstHub, nil)
	if err := firstModule.deliverRuntimeTerminal(ctx, terminated); err == nil {
		t.Fatal("expected Result-persisted administrative delivery interruption")
	}
	if !interruptedResults.fired {
		t.Fatal("administrative Result persistence crash point was not reached")
	}
	if item, err := resultRepo.Get(ctx, "ws-v2/run-v2/result/dod-assessment"); err != nil || item.State != common.ResultStateRejected {
		t.Fatalf("persisted administrative Result=%+v err=%v", item, err)
	}
	if run, err := runs.Get(ctx, "run-v2"); err != nil || run.State != common.RunStateActive {
		t.Fatalf("Run changed before administrative restart projection: %+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "run-v2" {
		t.Fatalf("Workspace changed before administrative restart projection: %+v err=%v", workspace, err)
	}
	if err := firstResultOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := firstRunOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	firstHub.Terminate(ctx)

	restartedHub := magicevent.NewHub(32)
	t.Cleanup(func() { restartedHub.Terminate(ctx) })
	restartedResultOwner := resultmodule.New()
	if err := restartedResultOwner.Configure(resultbiz.Dependencies{Results: resultRepo, Artifacts: filerepository.NewArtifactRepository(root + "/artifacts.json"), EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedResultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedResultOwner.Stop(ctx) })
	startTerminalDeliveryResourceOwners(t, ctx, restartedHub, runs, workspaces)
	persistedRun, err := runs.Get(ctx, "run-v2")
	if err != nil || persistedRun.RuntimeState == nil {
		t.Fatalf("persisted administrative Run=%+v err=%v", persistedRun, err)
	}
	restartedAggregate := runtimeAggregateFromRecord(*persistedRun.RuntimeState)
	if err := restartedAggregate.Validate(); err != nil {
		t.Fatalf("reloaded administrative aggregate: %v", err)
	}
	assertAdministrativeSupersedeAuthority(t, restartedAggregate)
	restartedModule := newTestBiz(newRuntimeTestOwner(), restartedHub, nil)
	if err := restartedModule.deliverRuntimeTerminal(ctx, restartedAggregate); err != nil {
		t.Fatalf("restart administrative delivery: %v", err)
	}
	if err := restartedModule.deliverRuntimeTerminal(ctx, restartedAggregate); err != nil {
		t.Fatalf("duplicate restart administrative delivery: %v", err)
	}
	if run, err := runs.Get(ctx, "run-v2"); err != nil || run.State != common.RunStateSuperseded {
		t.Fatalf("administrative Run restart projection=%+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "" || workspace.State != common.WorkspaceStateSuperseded {
		t.Fatalf("administrative Workspace restart projection=%+v err=%v", workspace, err)
	}
	items, err := resultRepo.ListByRun(ctx, "run-v2")
	if err != nil || len(items) != 1 || items[0].State != common.ResultStateRejected {
		t.Fatalf("administrative Result restart delivery=%+v err=%v", items, err)
	}
}

func TestDeliverRuntimePrerequisiteRefreshExhaustedRestartsAfterTerminalPersistenceBeforeResult(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runs := filerepository.NewRunRepository(root + "/runs.json")
	workspaces := filerepository.NewWorkspaceRepository(root + "/workspaces.json")
	resultRepo := filerepository.NewResultRepository(root + "/results.json")
	aggregate := runtimeAggregate(t, 2)
	aggregate.Facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted, Code: "prerequisite_refresh_exhausted",
		Signature: "refresh-budget:receipt-1", Message: "prerequisite refresh budget exhausted after owner resource change",
	}
	decision, err := runtime.ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	record := runtimeStateRecordFromAggregate(aggregate)
	bootstrap := runtimeBootstrap()
	runFixture := &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive, RuntimeBootstrap: &bootstrap, RuntimeState: &record}
	runFixture.RuntimeLifecycleFence = runtimeLifecycleFenceFromModel(runFixture)
	if err := runs.Create(ctx, runFixture); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}

	firstHub := magicevent.NewHub(32)
	firstOwner := resultmodule.New()
	interruptedRepo := &failBeforeResultCreateRepository{ResultRepository: resultRepo}
	if err := firstOwner.Configure(resultbiz.Dependencies{
		Results: interruptedRepo, Artifacts: filerepository.NewArtifactRepository(root + "/artifacts.json"), EventHub: firstHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := firstOwner.Start(); err != nil {
		t.Fatal(err)
	}
	firstModule := newTestBiz(newRuntimeTestOwner(), firstHub, nil)
	if err := firstModule.deliverRuntimeTerminal(ctx, aggregate); err == nil {
		t.Fatal("expected terminal delivery interruption before Result persistence")
	}
	if !interruptedRepo.fired {
		t.Fatal("Result pre-persistence crash point was not reached")
	}
	if _, err := resultRepo.Get(ctx, "ws-v2/run-v2/result/dod-assessment"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("Result persisted before restart: %v", err)
	}
	if run, err := runs.Get(ctx, "run-v2"); err != nil || run.State != common.RunStateActive {
		t.Fatalf("Run changed before Result persistence: %+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "run-v2" {
		t.Fatalf("Workspace changed before Result persistence: %+v err=%v", workspace, err)
	}
	if err := firstOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	firstHub.Terminate(ctx)

	// Recreate the EventHub, owner and delivery Biz, then reload the terminal
	// aggregate from its file-backed Run record rather than retaining memory.
	restartedHub := magicevent.NewHub(32)
	t.Cleanup(func() { restartedHub.Terminate(ctx) })
	restartedOwner := resultmodule.New()
	if err := restartedOwner.Configure(resultbiz.Dependencies{
		Results: resultRepo, Artifacts: filerepository.NewArtifactRepository(root + "/artifacts.json"), EventHub: restartedHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := restartedOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedOwner.Stop(ctx) })
	startTerminalDeliveryResourceOwners(t, ctx, restartedHub, runs, workspaces)
	persistedRun, err := runs.Get(ctx, "run-v2")
	if err != nil || persistedRun.RuntimeState == nil {
		t.Fatalf("persisted terminal Run=%+v err=%v", persistedRun, err)
	}
	restartedAggregate := runtimeAggregateFromRecord(*persistedRun.RuntimeState)
	if err := restartedAggregate.Validate(); err != nil {
		t.Fatalf("reloaded terminal aggregate: %v", err)
	}
	restartedModule := newTestBiz(newRuntimeTestOwner(), restartedHub, nil)
	if err := restartedModule.deliverRuntimeTerminal(ctx, restartedAggregate); err != nil {
		t.Fatal(err)
	}
	items, err := resultRepo.ListByRun(ctx, "run-v2")
	if err != nil || len(items) != 1 || items[0].State != common.ResultStateRejected || items[0].RuntimeCloseout == nil ||
		items[0].RuntimeCloseout.FailureClass != runtimecontract.FailureClassPrerequisiteRefreshExhausted ||
		items[0].RuntimeCloseout.FailureCode != "prerequisite_refresh_exhausted" {
		t.Fatalf("Result after restart=%+v err=%v", items, err)
	}
	if run, err := runs.Get(ctx, "run-v2"); err != nil || run.State != common.RunStateFailed {
		t.Fatalf("Run after restart projection=%+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "" {
		t.Fatalf("Workspace after restart projection=%+v err=%v", workspace, err)
	}
}

func TestRuntimeResultProjectionBuildsAcceptedDoDAssessment(t *testing.T) {
	aggregate := runtime.RuntimeAggregate{
		Definition: runtimecontract.RuntimeDefinitionContext{GoalID: "goal-1", DoDID: "dod-1"},
		Facts: runtimecontract.RuntimeFacts{
			Activation:         runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
			WorkspaceID:        "ws-1",
			RunID:              "run-1",
			DefinitionRevision: "definition-1",
			Focus:              runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
			Coverage:           []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"event-1"}}},
			Assessments: []runtimecontract.RuntimeAssessmentFact{{
				ID: "assessment-1", FocusID: "focus-1", Scope: "dodItem", RequirementID: "dod-001", RequirementText: "行为已验证",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
				Disposition:         runtimecontract.AssessmentDispositionSatisfied, EvidenceRefs: []string{"event-1"}, ArtifactRefs: []string{"artifact-assessment"},
				Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-1", Satisfied: true, EvidenceRefs: []string{"event-1"}, ArtifactRefs: []common.ArtifactID{"artifact-item"}}},
			}}},
		Decision: runtimecontract.RuntimeDecision{
			State:             runtimecontract.ConvergenceStateComplete,
			AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority-1"},
			Terminal:          &runtimecontract.RuntimeTerminalDecision{Code: "coverage_satisfied", Summary: "coverage satisfied"},
		},
	}
	item := runtimeResultMaterialFromAggregate(aggregate)
	if item.ID != "ws-1/run-1/result/dod-assessment" || !item.DoDAssessment.Satisfied ||
		len(item.DoDAssessment.Items) != 1 || item.DoDAssessment.Items[0].Item != "行为已验证" || len(item.EvidenceRefs) != 1 {
		t.Fatalf("runtime result = %+v", item)
	}
	if item.RuntimeCloseout == nil || item.RuntimeCloseout.Disposition != runtimecontract.AssessmentDispositionSatisfied ||
		len(item.RuntimeCloseout.SatisfiedCoverageWorkItemIDs) != 1 || item.RuntimeCloseout.SatisfiedCoverageWorkItemIDs[0] != "cwi-1" {
		t.Fatalf("runtime closeout = %+v", item.RuntimeCloseout)
	}
	if item.RuntimeCloseout.SchemaVersion != runtimecontract.RuntimeResultCloseoutSchemaVersionCurrent {
		t.Fatalf("runtime closeout schema = %d", item.RuntimeCloseout.SchemaVersion)
	}
	if len(item.ArtifactRefs) != 2 || item.ArtifactRefs[0] != "artifact-assessment" || item.ArtifactRefs[1] != "artifact-item" ||
		len(item.SummaryDetail.ArtifactRefs) != 2 || len(item.RuntimeCloseout.ArtifactRefs) != 2 {
		t.Fatalf("runtime artifact refs = result:%v summary:%v closeout:%v", item.ArtifactRefs, item.SummaryDetail.ArtifactRefs, item.RuntimeCloseout.ArtifactRefs)
	}
}

func TestRuntimeResultProjectionBuildsStructuredRejectedCloseout(t *testing.T) {
	aggregate := runtime.RuntimeAggregate{
		Definition: runtimecontract.RuntimeDefinitionContext{GoalID: "goal-1", DoDID: "dod-1", Revision: "definition-1"},
		Facts: runtimecontract.RuntimeFacts{
			Activation:         runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
			WorkspaceID:        "ws-1",
			RunID:              "run-1",
			DefinitionRevision: "definition-1",
			Focus: runtimecontract.CoverageFocus{
				ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"},
			},
			Coverage: []runtimecontract.RuntimeCoverageItemFact{
				{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"ev-1"}},
				{ID: "cwi-2", State: runtimecontract.RuntimeCoverageItemStateUnsatisfied},
			},
			Assessments: []runtimecontract.RuntimeAssessmentFact{{
				ID: "assessment-1", FocusID: "focus-1", Scope: "dodItem", RequirementID: "dod-001", RequirementText: "行为一",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"},
				Disposition:         runtimecontract.AssessmentDispositionRepairableGap,
				Summary:             "implementation incomplete",
				Items: []runtimecontract.RuntimeAssessmentItemFact{
					{CoverageWorkItemID: "cwi-1", Satisfied: true, EvidenceRefs: []string{"ev-1"}},
					{CoverageWorkItemID: "cwi-2", Satisfied: false, Reason: "missing implementation"},
				},
				Gap: &runtimecontract.RuntimeAssessmentGap{
					CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-2"},
					MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
					TargetRefs:           []string{"main.go"},
					Reason:               "tools not implemented",
					FailureCode:          "implementation_gap",
					FailureSignature:     "sig-impl",
				},
				EvidenceRefs: []string{"ev-1"},
			}},
			Repair: runtimecontract.RuntimeRepairLedger{LastCycleID: "cycle-1"},
		},
		Decision: runtimecontract.RuntimeDecision{
			State:             runtimecontract.ConvergenceStateFailed,
			AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority-1"},
			Terminal:          &runtimecontract.RuntimeTerminalDecision{Code: "no_progress_exhausted", Summary: "repair budget exhausted"},
		},
	}
	item := runtimeResultMaterialFromAggregate(aggregate)
	if item.RuntimeCloseout == nil {
		t.Fatalf("rejected result = %+v", item)
	}
	closeout := item.RuntimeCloseout
	if closeout.DefinitionRevision != "definition-1" || closeout.FailureCode != "implementation_gap" {
		t.Fatalf("closeout identity = %+v", closeout)
	}
	if len(closeout.SatisfiedCoverageWorkItemIDs) != 1 || closeout.SatisfiedCoverageWorkItemIDs[0] != "cwi-1" {
		t.Fatalf("satisfied = %v", closeout.SatisfiedCoverageWorkItemIDs)
	}
	if len(closeout.UnsatisfiedCoverageWorkItemIDs) != 1 || closeout.UnsatisfiedCoverageWorkItemIDs[0] != "cwi-2" {
		t.Fatalf("unsatisfied = %v", closeout.UnsatisfiedCoverageWorkItemIDs)
	}
	if len(closeout.Gaps) != 1 || len(closeout.MissingEvidenceKinds) != 1 || closeout.MissingEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange {
		t.Fatalf("gaps/missing = %+v %v", closeout.Gaps, closeout.MissingEvidenceKinds)
	}
	if len(closeout.TargetRefs) != 1 || closeout.TargetRefs[0] != "main.go" {
		t.Fatalf("target refs = %v", closeout.TargetRefs)
	}
	if closeout.Disposition != runtimecontract.AssessmentDispositionRepairableGap {
		t.Fatalf("disposition = %q", closeout.Disposition)
	}
}

func TestRuntimeResultProjectionExcludesSettledClaimedGapFromTerminalFailure(t *testing.T) {
	aggregate := runtime.RuntimeAggregate{
		Definition: runtimecontract.RuntimeDefinitionContext{GoalID: "goal-1", DoDID: "dod-1", Revision: "definition-1"},
		Facts: runtimecontract.RuntimeFacts{
			Activation:  runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
			WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1",
			Focus: runtimecontract.CoverageFocus{ID: "focus-cwi-029", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-029"}},
			Coverage: []runtimecontract.RuntimeCoverageItemFact{
				{ID: "cwi-026", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"ev-cwi-026"}},
				{ID: "cwi-029", State: runtimecontract.RuntimeCoverageItemStateUnsatisfied},
			},
			Assessments: []runtimecontract.RuntimeAssessmentFact{{
				ID: "assessment-cwi-026", FocusID: "focus-cwi-026", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-026"},
				ClaimedDisposition: runtimecontract.AssessmentDispositionRepairableGap,
				Disposition:        runtimecontract.AssessmentDispositionSatisfied,
				Gap: &runtimecontract.RuntimeAssessmentGap{
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-026"}, MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
					TargetRefs: []string{"mcp-lsp-bridge/server.go"}, Reason: "historical claimed gap", FailureCode: "historical_implementation_gap", FailureSignature: "cwi-026-gap",
				},
			}},
			Failure: &runtimecontract.FailureFact{
				Class: runtimecontract.FailureClassCandidateInfeasible, Code: "validation_recipe_unavailable", Signature: "cwi-029-validation-recipe-unavailable", Message: "no current validation recipe",
			},
		},
		Decision: runtimecontract.RuntimeDecision{
			State: runtimecontract.ConvergenceStateFailed, AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority-93"},
			Terminal: &runtimecontract.RuntimeTerminalDecision{Code: "semantic_contract_failed", Summary: "semantic feasibility exhausted"},
		},
	}
	closeout := runtimeResultMaterialFromAggregate(aggregate).RuntimeCloseout
	if closeout == nil || closeout.FailureClass != runtimecontract.FailureClassCandidateInfeasible || closeout.FailureCode != "validation_recipe_unavailable" || closeout.FailureSignature != "cwi-029-validation-recipe-unavailable" {
		t.Fatalf("terminal failure did not use current authority: %+v", closeout)
	}
	if len(closeout.Gaps) != 0 || len(closeout.MissingEvidenceKinds) != 0 || containsRuntimeResultString(closeout.TargetRefs, "mcp-lsp-bridge/server.go") {
		t.Fatalf("settled claimed gap leaked into terminal closeout: %+v", closeout)
	}
	if len(closeout.SatisfiedCoverageWorkItemIDs) != 1 || closeout.SatisfiedCoverageWorkItemIDs[0] != "cwi-026" ||
		len(closeout.UnsatisfiedCoverageWorkItemIDs) != 1 || closeout.UnsatisfiedCoverageWorkItemIDs[0] != "cwi-029" {
		t.Fatalf("terminal coverage = satisfied %v unsatisfied %v", closeout.SatisfiedCoverageWorkItemIDs, closeout.UnsatisfiedCoverageWorkItemIDs)
	}
}

func containsRuntimeResultString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestRuntimeResultProjectionPackagesLedgerEvidenceBeforeAssessment(t *testing.T) {
	aggregate := runtime.RuntimeAggregate{
		Definition: runtimecontract.RuntimeDefinitionContext{GoalID: "goal-1", DoDID: "dod-1", Revision: "definition-1"},
		Facts: runtimecontract.RuntimeFacts{
			Activation:  runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
			WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "runtime-evidence-00000000000000000002",
			Focus:    runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
			Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateUnsatisfied}},
			Evidence: runtimecontract.EvidenceLedger{
				Accepted: []runtimecontract.EvidenceFact{{ID: "accepted-1", SourceRef: "workorch://accepted", EvidenceRefs: []string{"accepted-detail"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}},
				Rejected: []runtimecontract.EvidenceFact{{ID: "rejected-1", SourceRef: "workorch://rejected", EvidenceRefs: []string{"rejected-detail"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}},
			},
			Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateExhausted, Code: "candidate_retry_exhausted", Signature: "candidate-retry-exhausted", Message: "candidate exhausted"},
		},
		Decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateFailed, AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority-1"}, Terminal: &runtimecontract.RuntimeTerminalDecision{Code: "candidate_retry_exhausted", Summary: "candidate exhausted"}},
	}
	item := runtimeResultMaterialFromAggregate(aggregate)
	for _, ref := range []string{"accepted-1", "workorch://accepted", "accepted-detail", "rejected-1", "workorch://rejected", "rejected-detail"} {
		if !containsResultEvidenceRef(item.EvidenceRefs, ref) {
			t.Fatalf("result evidence refs = %v, missing %q", item.EvidenceRefs, ref)
		}
	}
	if item.RuntimeCloseout == nil || item.RuntimeCloseout.FailureClass != runtimecontract.FailureClassCandidateExhausted || len(item.RuntimeCloseout.EvidenceRefs) != 6 {
		t.Fatalf("runtime closeout = %+v", item.RuntimeCloseout)
	}
}

func containsResultEvidenceRef(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
