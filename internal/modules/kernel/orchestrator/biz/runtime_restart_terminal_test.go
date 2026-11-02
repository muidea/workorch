package orchestrator

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	resultbiz "github.com/muidea/workorch/internal/modules/blocks/result/biz"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func restartTerminalPredecessorFixture(t *testing.T) (runtime.RuntimeAggregate, *runmodel.Run) {
	t.Helper()
	a := runtimeTerminalAggregate(t)
	a.Facts.RecoveryPolicy.MaxTerminalDeliveryAttempts = 1
	a.Facts.ExecutionBudgetPolicy = runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(a.Facts.ConvergencePolicy, a.Facts.RecoveryPolicy)
	a.Facts.Recovery.TerminalDeliveryAttempts = 1
	a.Facts.Recovery.TerminalDeliveryIntervention = runtimeTerminalRecoveryRequestForTest(mustTerminalDeliveryRequest(t, a), 0)
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	r := runtimeRunForTest(common.RunStateActive, a.Definition)
	record := runtimeStateRecordFromAggregate(a)
	r.RuntimeState = &record
	b := r.RuntimeBootstrap.Clone()
	b.Definition.StartupInputHash = "restart-original-startup"
	intent := &runmodel.ReplacementIntent{
		RequestID: "restart-original-terminal", WorkspaceID: r.WorkspaceID, GoalID: b.Definition.GoalID, DoDID: b.Definition.DoDID,
		DefinitionRevision: b.Definition.Revision, StartupInputHash: b.Definition.StartupInputHash, Bootstrap: &b,
		ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
		Kind:                runtimecontract.RuntimeTerminationSuperseded, Reason: "workspace execution restart/restart-original-terminal", State: runmodel.ReplacementIntentPending,
	}
	intent.Recovery = runmodel.ReplacementRecoveryLease{
		RequestID: intent.RequestID, IntentHash: runmodel.StableReplacementIntentHash(*intent),
		Stage: runmodel.ReplacementRecoveryAdministrativeTermination, State: runmodel.ReplacementRecoveryScheduled,
		Generation: 1, MaxAttempts: 2, DueAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	r.ReplacementIntent = intent
	if !intent.ValidFor(r) {
		t.Fatal("invalid replacement fixture")
	}
	return a, r
}

func restartTerminalCommand(r *runmodel.Run) orchestratorevents.RequestRuntimeTerminationCommand {
	i := r.ReplacementIntent
	return orchestratorevents.RequestRuntimeTerminationCommand{
		WorkspaceID: r.WorkspaceID, RunID: r.ID, Kind: i.Kind, RequestedBy: "blocks/run", Reason: i.Reason,
		ReplacementRequestID: i.RequestID, ReplacementIntentHash: runmodel.StableReplacementIntentHash(*i), RecoveryGeneration: i.Recovery.Generation,
	}
}

func TestRestartTerminalDeliveryRejectsUnclaimedOrStaleAuthority(t *testing.T) {
	_, original := restartTerminalPredecessorFixture(t)
	original.ReplacementIntent.Recovery.State = runmodel.ReplacementRecoveryRunning
	original.ReplacementIntent.Recovery.DueAt = ""
	command := restartTerminalCommand(original)
	if err := validateRestartTerminalDeliveryCommand(command, original); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*orchestratorevents.RequestRuntimeTerminationCommand, *runmodel.Run)
	}{
		{"wrong workspace", func(c *orchestratorevents.RequestRuntimeTerminationCommand, _ *runmodel.Run) { c.WorkspaceID = "other" }},
		{"wrong run", func(c *orchestratorevents.RequestRuntimeTerminationCommand, _ *runmodel.Run) { c.RunID = "other" }},
		{"wrong owner", func(c *orchestratorevents.RequestRuntimeTerminationCommand, _ *runmodel.Run) {
			c.RequestedBy = "application/workorchd"
		}},
		{"missing request", func(c *orchestratorevents.RequestRuntimeTerminationCommand, _ *runmodel.Run) {
			c.ReplacementRequestID = ""
		}},
		{"wrong request", func(c *orchestratorevents.RequestRuntimeTerminationCommand, _ *runmodel.Run) {
			c.ReplacementRequestID = "other"
		}},
		{"wrong hash", func(c *orchestratorevents.RequestRuntimeTerminationCommand, _ *runmodel.Run) {
			c.ReplacementIntentHash = "other"
		}},
		{"stale generation", func(c *orchestratorevents.RequestRuntimeTerminationCommand, _ *runmodel.Run) { c.RecoveryGeneration++ }},
		{"wrong reason", func(c *orchestratorevents.RequestRuntimeTerminationCommand, _ *runmodel.Run) { c.Reason = "other" }},
		{"unclaimed", func(_ *orchestratorevents.RequestRuntimeTerminationCommand, r *runmodel.Run) {
			r.ReplacementIntent.Recovery.State = runmodel.ReplacementRecoveryScheduled
			r.ReplacementIntent.Recovery.DueAt = time.Now().UTC().Format(time.RFC3339Nano)
		}},
		{"exhausted", func(_ *orchestratorevents.RequestRuntimeTerminationCommand, r *runmodel.Run) {
			r.ReplacementIntent.Recovery.State = runmodel.ReplacementRecoveryExhausted
			r.ReplacementIntent.Recovery.Attempt = 2
			r.ReplacementIntent.Recovery.LastError = "owner unavailable"
		}},
		{"missing intent", func(_ *orchestratorevents.RequestRuntimeTerminationCommand, r *runmodel.Run) {
			r.ReplacementIntent = nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, r := command, runmodel.CloneRun(original)
			tc.mutate(&c, r)
			if err := validateRestartTerminalDeliveryCommand(c, r); err == nil {
				t.Fatal("unbound replacement authority accepted")
			}
		})
	}
}

func TestRestartTerminalPredecessorBusyGateDoesNotDeliverOrMutate(t *testing.T) {
	a, r := restartTerminalPredecessorFixture(t)
	repo := &inMemoryRuntimeStateRepository{aggregate: &a}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repo); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	m := newTestBiz(owner, hub, nil)
	key := runtimeDriveKey(r.WorkspaceID, r.ID)
	token, acquired := m.beginRuntimeDriveWithToken(key)
	if !acquired {
		t.Fatal("missing original gate")
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, "blocks/run", ModuleID, magicevent.NewHeader(), context.Background(), restartTerminalCommand(r))
	result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
	m.handleRuntimeTerminationRequest(event, result)
	if _, err := result.Get(); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("busy gate err=%v", err)
	}
	if gate, ok := m.currentRuntimeDriveGate(key); !ok || gate.Token != token || !reflect.DeepEqual(*repo.aggregate, a) {
		t.Fatal("replacement stole original gate or changed terminal")
	}
}

type restartResultFailure struct {
	repository.ResultRepository
	failAlways bool
	failed     bool
}

func (r *restartResultFailure) Create(ctx context.Context, value *resultmodel.Result) error {
	if r.failAlways {
		return errors.New("fixed Result owner unavailable")
	}
	if err := r.ResultRepository.Create(ctx, value); err != nil {
		return err
	}
	if !r.failed {
		r.failed = true
		return errors.New("fixed Result response lost after commit")
	}
	return nil
}

// Actual Run replacement recovery -> typed EventHub command -> original
// Result/Workspace/Run delivery -> Run-private successor activation. Only
// capability/LLM execution is absent: this verifies restart, not project credit.
func TestRestartTerminalPredecessorOwnerMainline(t *testing.T) {
	for _, mode := range []string{"intervention", "result-response-loss", "fresh-process-running-claim", "bounded-owner-failure", "ordinary-active-predecessor"} {
		t.Run(mode, func(t *testing.T) {
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-v2")
			root := t.TempDir()
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
			results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
			a, predecessor := restartTerminalPredecessorFixture(t)
			if mode == "ordinary-active-predecessor" {
				a = runtimeAggregate(t, 2)
				record := runtimeStateRecordFromAggregate(a)
				predecessor.RuntimeState = &record
			}
			if mode == "fresh-process-running-claim" {
				predecessor.ReplacementIntent.Recovery.State = runmodel.ReplacementRecoveryRunning
				predecessor.ReplacementIntent.Recovery.DueAt = ""
			}
			if err := runs.Create(ctx, predecessor); err != nil {
				t.Fatal(err)
			}
			if err := workspaces.Create(ctx, runtimeWorkspaceForPersistedRunTest(t, ctx, runs, "ws-v2", "run-v2", root, "")); err != nil {
				t.Fatal(err)
			}
			hub := magicevent.NewHub(32)
			t.Cleanup(func() { hub.Terminate(context.Background()) })
			workspaceOwner := workspacemodule.New()
			if err := workspaceOwner.Configure(workspaceOwnerTestDependencies(hub, workspaces, workspaces, memory.NewGoalRepository(), memory.NewDoDRepository(), memory.NewWorkspaceTemplateRepository())); err != nil {
				t.Fatal(err)
			}
			if err := workspaceOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
			var resultRepository repository.ResultRepository = results
			if mode == "result-response-loss" || mode == "bounded-owner-failure" {
				resultRepository = &restartResultFailure{ResultRepository: results, failAlways: mode == "bounded-owner-failure"}
			}
			resultOwner := resultmodule.New()
			if err := resultOwner.Configure(resultbiz.Dependencies{OwnerRepositories: testResultOwnerRepositories(resultRepository, memory.NewArtifactRepository(), memory.NewResultWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := resultOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = resultOwner.Stop(ctx) })
			module := newRuntimeProjectionTestModule(t, hub)
			observer := magicevent.NewSimpleObserver(ModuleID, hub)
			if err := observer.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, module.handleRuntimeTerminationRequest); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination) })
			background := magictask.NewBackgroundRoutine(8)
			t.Cleanup(func() { background.Shutdown(context.Background()) })
			runOwner := runmodule.New()
			deps := runOwnerTestDependencies(runs, nil, nil, hub)
			deps.BackgroundRoutine = background
			if err := runOwner.Configure(deps); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runOwner.Stop(ctx) })
			deadline := time.Now().Add(8 * time.Second)
			var stored *runmodel.Run
			for {
				var err error
				stored, err = runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				if err != nil {
					t.Fatal(err)
				}
				recovery := stored.ReplacementIntent.Recovery
				if stored.ReplacementIntent.State == runmodel.ReplacementIntentCompleted || recovery.State == runmodel.ReplacementRecoveryExhausted {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("replacement stalled: %+v", stored.ReplacementIntent)
				}
				time.Sleep(10 * time.Millisecond)
			}
			final := runtimeAggregateFromRecord(*stored.RuntimeState)
			if err := final.Validate(); err != nil {
				t.Fatal(err)
			}
			if (final.Termination != nil) != (mode == "ordinary-active-predecessor") || !reflect.DeepEqual(final.Decision, a.Decision) || !reflect.DeepEqual(final.Facts.Failure, a.Facts.Failure) ||
				!reflect.DeepEqual(final.Facts.Recovery, a.Facts.Recovery) || !reflect.DeepEqual(final.ExecutionBudget, a.ExecutionBudget) || !reflect.DeepEqual(final.Definition, a.Definition) || !reflect.DeepEqual(final.Facts.Coverage, a.Facts.Coverage) {
				t.Fatal("restart rewrote original terminal, recovery budget, definition or acceptance")
			}
			inventory, err := runs.ListByWorkspaceID(ctx, "ws-v2")
			if err != nil {
				t.Fatal(err)
			}
			if mode == "bounded-owner-failure" {
				if stored.State != common.RunStateActive || len(inventory) != 1 || len(final.DeliveryAcknowledgements) != 0 || stored.ReplacementIntent.Recovery.Attempt != 2 || stored.ReplacementIntent.Recovery.State != runmodel.ReplacementRecoveryExhausted {
					t.Fatalf("owner failure escaped bounded recovery: %+v", stored.ReplacementIntent)
				}
				return
			}
			wantState := common.RunStateFailed
			if mode == "ordinary-active-predecessor" {
				wantState = common.RunStateSuperseded
			}
			if stored.State != wantState || stored.TerminalResultID == "" || !runtimeTerminalDeliveryFullyAcknowledged(final) || len(inventory) != 2 || stored.ReplacementIntent.State != runmodel.ReplacementIntentCompleted {
				t.Fatalf("restart did not close original terminal and activate one successor: %+v", stored.ReplacementIntent)
			}
			child, err := runs.GetByWorkspaceID(ctx, "ws-v2", stored.ReplacementIntent.SuccessorRunID)
			if err != nil || child.State != common.RunStateActive || child.Lineage.PredecessorRunID != stored.ID || child.Lineage.PredecessorResultID != stored.TerminalResultID || !reflect.DeepEqual(child.RuntimeBootstrap, predecessor.ReplacementIntent.Bootstrap) {
				t.Fatal("successor activation changed input or lost failed predecessor lineage")
			}
			items, err := results.ListByRun(ctx, "run-v2")
			if err != nil || len(items) != 1 || items[0].State != common.ResultStateRejected || items[0].ID != stored.TerminalResultID {
				t.Fatal("restart duplicated or changed original Result")
			}
			if mode == "result-response-loss" && stored.ReplacementIntent.Recovery.Generation < 3 {
				t.Fatal("Run recovery did not advance the failed delivery generation before successor materialization")
			}
			if mode == "fresh-process-running-claim" && stored.ReplacementIntent.Recovery.Generation < 3 {
				t.Fatal("fresh process reused stale running replacement generation")
			}
			// A delayed duplicate from the old claimed generation cannot repeat
			// owner I/O, reset either budget, or create another successor.
			stale := restartTerminalCommand(predecessor)
			event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, "blocks/run", ModuleID, magicevent.NewHeader(), ctx, stale)
			response := hub.Send(event)
			if response == nil || !errors.Is(response.Error(), common.ErrConflict) {
				t.Fatal("completed replacement accepted a stale delivery command")
			}
			after, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || !reflect.DeepEqual(after, stored) {
				t.Fatal("stale replacement delivery rewrote predecessor")
			}
		})
	}
}
