package orchestrator

// Owner Biz persistence tests.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// failOnceRuntimeStateRepository injects a process loss immediately before the
// Run-owner CAS write. It deliberately delegates Load to the real repository
// so restart assertions never observe a test-local aggregate cache.
type failOnceRuntimeStateRepository struct {
	runtimev2.RuntimeStateRepository
	err    error
	failed bool
}

func (r *failOnceRuntimeStateRepository) Save(ctx context.Context, aggregate runtimev2.RuntimeAggregate, expectedRevision uint64) error {
	if !r.failed {
		r.failed = true
		return r.err
	}
	return r.RuntimeStateRepository.Save(ctx, aggregate, expectedRevision)
}

// failWhenRuntimeStateRepository targets a single persisted aggregate shape.
// It models a process loss at a named CAS boundary without treating earlier
// transitions (for example the execution claim) as failed writes.
type failWhenRuntimeStateRepository struct {
	runtimev2.RuntimeStateRepository
	err    error
	match  func(runtimev2.RuntimeAggregate) bool
	failed bool
}

// panicAfterRuntimeStateSaveRepository models a caller that loses the return
// path only after the Run-owner CAS write has become durable. It must never be
// used to simulate a pre-commit error: restart recovery has to observe the
// committed aggregate and must not replay its transition.
type panicAfterRuntimeStateSaveRepository struct {
	runtimev2.RuntimeStateRepository
	match  func(runtimev2.RuntimeAggregate) bool
	marker string
	fired  bool
}

func (r *panicAfterRuntimeStateSaveRepository) Save(ctx context.Context, aggregate runtimev2.RuntimeAggregate, expectedRevision uint64) error {
	matched := !r.fired && r.match != nil && r.match(aggregate)
	if err := r.RuntimeStateRepository.Save(ctx, aggregate, expectedRevision); err != nil {
		return err
	}
	if matched {
		r.fired = true
		panic(r.marker)
	}
	return nil
}

func (r *failWhenRuntimeStateRepository) Save(ctx context.Context, aggregate runtimev2.RuntimeAggregate, expectedRevision uint64) error {
	if !r.failed && r.match != nil && r.match(aggregate) {
		r.failed = true
		return r.err
	}
	return r.RuntimeStateRepository.Save(ctx, aggregate, expectedRevision)
}

type runtimeOperationExecutorFunc func(context.Context, runtimev2.RuntimeOperationExecutionRequest) (runtimev2.RuntimeOperationExecutionResult, error)

func (f runtimeOperationExecutorFunc) ExecuteRuntimeOperation(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest) (runtimev2.RuntimeOperationExecutionResult, error) {
	return f(ctx, request)
}

func TestRuntimeV2StateRepositoryPersistsAndLoadsThroughRunOwner(t *testing.T) {
	ctx := context.Background()
	if err := runtimeV2Bootstrap().Validate(); err != nil {
		t.Fatalf("test runtime bootstrap must validate: %v", err)
	}
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	// This boundary must survive a real Run-owner persistence reload. A memory
	// repository can prove reducer atomicity, but it cannot prove that the
	// committed assessment/coverage/repair delta is what a restarted process
	// reads from its durable aggregate.
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}
	module := runmodule.New()
	if err := module.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	repository := runtimeV2StateRepository{hub: hub}
	aggregate := runtimeV2Aggregate(t, 1)
	if current, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2"); err != nil || current.RuntimeBootstrap == nil || !runtimecontract.SameRuntimeBootstrap(*current.RuntimeBootstrap, runtimeV2Bootstrap()) {
		t.Fatalf("persisted bootstrap mismatch current=%+v err=%v", current, err)
	}
	if err := repository.Save(ctx, aggregate, 0); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := repository.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.StateRevision != 1 || loaded.Decision.AuthoritySnapshot.ID != aggregate.Decision.AuthoritySnapshot.ID ||
		!runtimecontract.SameRuntimeDefinitionContext(loaded.Definition, aggregate.Definition) {
		t.Fatalf("loaded aggregate = %+v", loaded)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.RuntimeContractVersion != runtimecontract.RuntimeContractVersionV2 || persisted.RuntimeState == nil {
		t.Fatalf("persisted runtime state = %+v", persisted)
	}
	if !runtimecontract.SameRuntimeDefinitionContext(persisted.RuntimeState.Definition, runtimeV2Definition()) {
		t.Fatalf("persisted runtime definition = %+v", persisted.RuntimeState.Definition)
	}
}

func TestRuntimeV2InitializesFromPersistedRunBootstrap(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStatePending,
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
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := biz.InitializeRuntimeV2FromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.Facts.Focus.ID != "focus-v2" || aggregate.Definition.GoalID != "goal-v2" || aggregate.StateRevision != 1 {
		t.Fatalf("initialized aggregate = %+v", aggregate)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeContractVersion != runtimecontract.RuntimeContractVersionV2 || persisted.RuntimeState == nil || persisted.RuntimeBootstrap == nil {
		t.Fatalf("persisted run = %+v err=%v", persisted, err)
	}
}

func TestRuntimeV2StateRepositoryRejectsStaleRevision(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	aggregate := runtimeV2Aggregate(t, 2)
	record := runtimeStateRecordFromAggregate(aggregate)
	run := runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())
	run.RuntimeContractVersion = runtimecontract.RuntimeContractVersionV2
	run.RuntimeState = &record
	if err := runs.Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	module := runmodule.New()
	if err := module.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	aggregate.StateRevision = 3
	err := (runtimeV2StateRepository{hub: hub}).Save(ctx, aggregate, 1)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Save stale revision err = %v, want conflict", err)
	}
}

// The owner-catalog dependency is the authority for an MCP failure target.
// Exercise the production Run-owner CAS repository here, rather than only the
// controller fixture repository: after a process restart the stale target must
// not be reconstructed from the persisted failure or a prior candidate
// attempt.
func TestRuntimeV2StateRepositoryRestartDoesNotReuseStaleMCPOwnerFailureTarget(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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

	facts := runtimeV2Aggregate(t, 1).Facts
	facts.RecoveryPolicy.MaxPrerequisiteRefreshes = 2
	facts.Focus.TargetRole = "implementation"
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:blocks/mcp", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, SelectorRefs: []string{"server-a"}, ObservedRevision: "mcp-catalog-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleMCP, RequiredCapabilities: []string{runtimecontract.StepKindMCPToolCall},
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP,
			SubjectRef: "mcp_server_catalog", Revision: "mcp-catalog-v1",
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	targetObservation := &runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation",
		ObservedRevision: "mcp-catalog-v1", TargetRefs: []string{"implementation.go"},
	}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassBuildFailure, Signature: "mcp-failure", Message: "owner observed failure",
		TargetRefs: []string{"implementation.go"}, TargetObservation: targetObservation,
	}
	facts.Resources.FailureTarget = &runtimecontract.FailureTarget{
		Signature: "mcp-failure", TargetRefs: []string{"implementation.go"}, TargetObservation: targetObservation,
	}
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}

	repository := runtimeV2StateRepository{hub: hub}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(ctx, "ws-v2", "run-v2", 0, "mcp-owner-catalog-prefix"); err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		SubjectRef: "mcp_server_catalog", ExpectedRevision: "mcp-catalog-v1", ObservedRevision: "mcp-catalog-v2",
		Reason: "MCP owner catalog changed",
	}
	// A crash before the CAS commit must leave the prior sealed observation and
	// pending candidate intact. A fresh controller can then safely retry the
	// same owner stale signal; no in-memory mutation is authority.
	beforeCommitRepository := &failOnceRuntimeStateRepository{
		RuntimeStateRepository: repository, err: errors.New("injected stale invalidation save failure"),
	}
	beforeCommitController, err := runtimev2.NewRuntimeController(beforeCommitRepository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beforeCommitController.InvalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-v2", "run-v2", 0, stale); err == nil {
		t.Fatal("stale invalidation unexpectedly committed despite injected Run-owner CAS failure")
	}
	beforeCommit, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if beforeCommit.Facts.Resources.FailureTarget == nil || beforeCommit.Facts.Failure == nil ||
		beforeCommit.Facts.Failure.TargetObservation == nil || len(beforeCommit.Facts.Prerequisites.Pending) != 0 ||
		len(beforeCommit.CandidateAttempts) != 1 || beforeCommit.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStatePending ||
		beforeCommit.Facts.Recovery.PrerequisiteRefreshes != 0 {
		t.Fatalf("pre-commit failure left a partially invalidated aggregate: %+v", beforeCommit)
	}
	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: repository, marker: "mcp-owner-catalog-stale-response-loss",
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			return aggregate.Facts.Resources.FailureTarget == nil && aggregate.Facts.Failure != nil &&
				len(aggregate.Facts.Failure.TargetRefs) == 0 && aggregate.Facts.Failure.TargetObservation == nil &&
				len(aggregate.Facts.Prerequisites.Pending) == 1 && len(aggregate.CandidateAttempts) == 1 &&
				aggregate.CandidateAttempts[0].State == runtimecontract.RuntimeCandidateAttemptStateInvalidated
		},
	}
	responseLost, err := runtimev2.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "mcp-owner-catalog-stale-response-loss" {
				t.Fatalf("unexpected stale response-loss marker: %v", recovered)
			}
		}()
		_, _ = responseLost.InvalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-v2", "run-v2", 0, stale)
		t.Fatal("expected stale invalidation response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("stale invalidation response-loss CAS point was not reached")
	}

	// New controller instance models a process restart; it must load only the
	// Run-owner persisted aggregate, with no process-local authority cache.
	restarted, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.StateRevision <= beforeCommit.StateRevision || loaded.Facts.Resources.FailureTarget != nil ||
		loaded.Facts.Failure == nil || len(loaded.Facts.Failure.TargetRefs) != 0 || loaded.Facts.Failure.TargetObservation != nil ||
		loaded.Facts.RepairDirective != nil || len(loaded.Facts.Prerequisites.Pending) != 1 ||
		loaded.Facts.Prerequisites.Pending[0].ID != prerequisite.ID || len(loaded.CandidateAttempts) != 1 ||
		loaded.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated {
		t.Fatalf("restart retained stale MCP repair authority: loaded=%+v", loaded)
	}
	duplicate, err := restarted.InvalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-v2", "run-v2", 0, stale)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.StateRevision != loaded.StateRevision || duplicate.Facts.Recovery.PrerequisiteRefreshes != loaded.Facts.Recovery.PrerequisiteRefreshes {
		t.Fatalf("duplicate stale invalidation changed persisted recovery state: before=%+v after=%+v", loaded.Facts.Recovery, duplicate.Facts.Recovery)
	}

	// Reacquisition is a new owner fact, not a resurrection of the stale
	// observation. Persist the catalog-v2 inventory and prove a subsequent
	// restart retains only that revision while the old failure target remains
	// unavailable until a new owner receipt establishes it.
	refreshedInventory := inventory
	refreshedInventory.ObservedRevision = "mcp-catalog-v2"
	refreshedInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(refreshedInventory)
	refreshedObservation := observation
	refreshedObservation.ContentHash = refreshedInventory.SnapshotID
	refreshedObservation.Dependencies[0].Revision = "mcp-catalog-v2"
	refreshedObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(refreshedObservation)
	reacquired, err := restarted.CommitPrerequisite(ctx, "ws-v2", "run-v2", runtimev2.PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, OperationAdmissibility: &refreshedInventory, Observation: &refreshedObservation,
	})
	if err != nil || len(reacquired.Facts.Prerequisites.Pending) != 0 || len(reacquired.Facts.Resources.OperationAdmissibilities) != 1 ||
		reacquired.Facts.Resources.OperationAdmissibilities[0].ObservedRevision != "mcp-catalog-v2" || reacquired.Facts.Resources.FailureTarget != nil ||
		reacquired.Facts.Failure == nil || len(reacquired.Facts.Failure.TargetRefs) != 0 || reacquired.Facts.Failure.TargetObservation != nil {
		t.Fatalf("stale reacquisition did not replace only the owner catalog fact: aggregate=%+v err=%v", reacquired, err)
	}
	restartedAfterReacquire, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := restartedAfterReacquire.Load(ctx, "ws-v2", "run-v2")
	if err != nil || reloaded.StateRevision != reacquired.StateRevision || len(reloaded.Facts.Resources.OperationAdmissibilities) != 1 ||
		reloaded.Facts.Resources.OperationAdmissibilities[0].ObservedRevision != "mcp-catalog-v2" || len(reloaded.Facts.Resources.PrerequisiteObservations) != 1 ||
		reloaded.Facts.Resources.PrerequisiteObservations[0].Dependencies[0].Revision != "mcp-catalog-v2" || reloaded.Facts.Resources.FailureTarget != nil {
		t.Fatalf("restart did not retain only reacquired owner revision: aggregate=%+v err=%v", reloaded, err)
	}
	if _, err := restartedAfterReacquire.CommitPrerequisite(ctx, "ws-v2", "run-v2", runtimev2.PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, OperationAdmissibility: &refreshedInventory, Observation: &refreshedObservation,
	}); err == nil {
		t.Fatal("duplicate stale reacquisition unexpectedly mutated aggregate")
	}
}

// Prerequisite acquisition is the first authority transition before candidate
// generation. Use the production Run-owner CAS repository to prove that a
// process loss before that CAS cannot make a scratch resource appear
// committed, and that a restarted controller resumes from the sole committed
// aggregate rather than a controller-local mutation.
func TestRuntimeV2StateRepositoryPrerequisiteCommitCrashRestartIsAtomic(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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

	repository := runtimeV2StateRepository{hub: hub}
	facts := runtimeV2Aggregate(t, 1).Facts
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), []runtimev2.RuntimeRequirement{{
		Kind: runtimecontract.PrerequisiteKindValidationScratch, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
		Reason: "test prerequisite commit atomicity",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(initialized.Facts.Prerequisites.Pending) != 1 {
		t.Fatalf("initialized aggregate = %+v", initialized)
	}
	prerequisiteID := initialized.Facts.Prerequisites.Pending[0].ID
	result := runtimev2.PrerequisiteResult{
		PrerequisiteID:    prerequisiteID,
		ValidationScratch: &runtimecontract.ValidationScratch{Reference: ".workorch-validation/run-v2", Revision: "scratch-v1"},
	}

	beforeCommitRepository := &failOnceRuntimeStateRepository{
		RuntimeStateRepository: repository,
		err:                    errors.New("injected prerequisite CAS failure"),
	}
	beforeCommitController, err := runtimev2.NewRuntimeController(beforeCommitRepository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beforeCommitController.CommitPrerequisite(ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("prerequisite commit unexpectedly persisted despite injected Run-owner CAS failure")
	}

	// A new controller instance is the restart boundary: it must see the old
	// pending prerequisite and no resource written by the failed CAS attempt.
	restartedBeforeCommit, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedBeforeCommit, err := restartedBeforeCommit.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loadedBeforeCommit.StateRevision != initialized.StateRevision || loadedBeforeCommit.Facts.Resources.ValidationScratch != nil ||
		len(loadedBeforeCommit.Facts.Prerequisites.Pending) != 1 || loadedBeforeCommit.Facts.Prerequisites.Pending[0].ID != prerequisiteID {
		t.Fatalf("failed prerequisite CAS partially persisted aggregate: %+v", loadedBeforeCommit)
	}

	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: repository, marker: "prerequisite-response-loss",
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			return aggregate.Facts.Resources.ValidationScratch != nil && aggregate.Facts.Resources.ValidationScratch.Revision == "scratch-v1" && len(aggregate.Facts.Prerequisites.Pending) == 0
		},
	}
	responseLost, err := runtimev2.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "prerequisite-response-loss" {
				t.Fatalf("unexpected prerequisite response-loss marker: %v", recovered)
			}
		}()
		_, _ = responseLost.CommitPrerequisite(ctx, "ws-v2", "run-v2", result)
		t.Fatal("expected prerequisite response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("prerequisite response-loss CAS point was not reached")
	}

	restartedAfterCommit, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedAfterCommit, err := restartedAfterCommit.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loadedAfterCommit.StateRevision <= initialized.StateRevision || loadedAfterCommit.Facts.Resources.ValidationScratch == nil ||
		loadedAfterCommit.Facts.Resources.ValidationScratch.Revision != "scratch-v1" || len(loadedAfterCommit.Facts.Prerequisites.Pending) != 0 {
		t.Fatalf("restart did not reuse committed prerequisite aggregate: %+v", loadedAfterCommit)
	}
	if _, err := restartedAfterCommit.CommitPrerequisite(ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("duplicate prerequisite result unexpectedly mutated committed aggregate")
	}
	final, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if final.StateRevision != loadedAfterCommit.StateRevision || final.Facts.Resources.ValidationScratch == nil || final.Facts.Resources.ValidationScratch.Revision != "scratch-v1" {
		t.Fatalf("duplicate prerequisite result changed persisted aggregate: %+v", final)
	}
}

// Target selection carries the owner-bound repair surface. It is therefore not
// sufficient to cover a generic prerequisite: a failed selection CAS must not
// leave a restarted controller with an apparently grounded target that File
// never committed into RuntimeFacts.
func TestRuntimeV2StateRepositoryTargetSelectionCommitCrashRestartIsAtomic(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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

	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Focus.TargetRefs = nil
	facts.Focus.TargetRole = "deliverable"
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "file/unique_workspace_entry"
	facts.Focus.RequireTargetGrounding = true
	repository := runtimeV2StateRepository{hub: hub}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var prerequisite runtimecontract.RuntimePrerequisite
	for _, value := range initialized.Facts.Prerequisites.Pending {
		if value.Kind == runtimecontract.PrerequisiteKindTargetSelection {
			prerequisite = value
			break
		}
	}
	if prerequisite.ID == "" {
		t.Fatalf("target selection prerequisite missing: %+v", initialized.Facts.Prerequisites)
	}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: prerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/unique_workspace_entry",
		Disposition: runtimecontract.RuntimeTargetSelectionSelected,
		TargetRefs:  []string{"implementation.go"}, CandidateTargetRefs: []string{"implementation.go"}, CandidateCount: 1,
		ObservedRevision: "workspace-tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: selection.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: "workspace-tree-v1",
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	result := runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSelection: &selection, Observation: &observation}

	beforeCommitRepository := &failOnceRuntimeStateRepository{
		RuntimeStateRepository: repository,
		err:                    errors.New("injected target selection CAS failure"),
	}
	beforeCommitController, err := runtimev2.NewRuntimeController(beforeCommitRepository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beforeCommitController.CommitPrerequisite(ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("target selection unexpectedly persisted despite injected Run-owner CAS failure")
	}

	restartedBeforeCommit, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedBeforeCommit, err := restartedBeforeCommit.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loadedBeforeCommit.StateRevision != initialized.StateRevision || loadedBeforeCommit.Facts.Resources.TargetSelection != nil {
		t.Fatalf("failed target-selection CAS partially persisted authority: %+v", loadedBeforeCommit)
	}
	foundPending := false
	for _, pending := range loadedBeforeCommit.Facts.Prerequisites.Pending {
		foundPending = foundPending || pending.ID == prerequisite.ID
	}
	if !foundPending {
		t.Fatalf("failed target-selection CAS removed prerequisite: %+v", loadedBeforeCommit.Facts.Prerequisites)
	}

	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: repository, marker: "target-selection-response-loss",
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			return aggregate.Facts.Resources.TargetSelection != nil && aggregate.Facts.Resources.TargetSelection.SnapshotID == selection.SnapshotID
		},
	}
	responseLost, err := runtimev2.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "target-selection-response-loss" {
				t.Fatalf("unexpected target-selection response-loss marker: %v", recovered)
			}
		}()
		_, _ = responseLost.CommitPrerequisite(ctx, "ws-v2", "run-v2", result)
		t.Fatal("expected target-selection response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("target-selection response-loss CAS point was not reached")
	}
	restartedAfterCommit, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedAfterCommit, err := restartedAfterCommit.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loadedAfterCommit.StateRevision <= initialized.StateRevision || loadedAfterCommit.Facts.Resources.TargetSelection == nil ||
		loadedAfterCommit.Facts.Resources.TargetSelection.SnapshotID != selection.SnapshotID {
		t.Fatalf("restart did not reuse committed target selection: %+v", loadedAfterCommit)
	}
	if _, err := restartedAfterCommit.CommitPrerequisite(ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("duplicate target selection unexpectedly mutated committed aggregate")
	}
	final, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if final.StateRevision != loadedAfterCommit.StateRevision || final.Facts.Resources.TargetSelection == nil ||
		final.Facts.Resources.TargetSelection.SnapshotID != selection.SnapshotID {
		t.Fatalf("duplicate target selection changed persisted aggregate: %+v", final)
	}
}

// Creation scope is a distinct owner observation from an existing target.
// Exercise its Run-owner CAS boundaries explicitly: a restart may observe
// either the pending prerequisite or the fully committed scope, never a
// partial scope that would authorize a candidate without File's fact.
func TestRuntimeV2StateRepositoryCreationSelectionCommitCrashRestartIsAtomic(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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

	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Focus.TargetRefs = nil
	facts.Focus.TargetRole = "deliverable"
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "file/workspace_deliverable"
	facts.Focus.RequireTargetGrounding = true
	repository := runtimeV2StateRepository{hub: hub}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var prerequisite runtimecontract.RuntimePrerequisite
	for _, value := range initialized.Facts.Prerequisites.Pending {
		if value.Kind == runtimecontract.PrerequisiteKindTargetSelection {
			prerequisite = value
			break
		}
	}
	if prerequisite.ID == "" {
		t.Fatalf("creation selection prerequisite missing: %+v", initialized.Facts.Prerequisites)
	}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: prerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionCreate, CandidateCount: 0, ObservedRevision: "workspace-tree-empty-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: selection.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: selection.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	result := runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSelection: &selection, Observation: &observation}

	beforeCommit := &failOnceRuntimeStateRepository{RuntimeStateRepository: repository, err: errors.New("injected creation selection CAS failure")}
	failedController, err := runtimev2.NewRuntimeController(beforeCommit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := failedController.CommitPrerequisite(ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("creation selection persisted despite injected Run-owner CAS failure")
	}
	loadedBefore, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil || loadedBefore.StateRevision != initialized.StateRevision || loadedBefore.Facts.Resources.TargetSelection != nil {
		t.Fatalf("failed creation selection CAS partially persisted authority=%+v err=%v", loadedBefore, err)
	}

	responseLoss := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: repository, marker: "creation-selection-response-loss",
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			return aggregate.Facts.Resources.TargetSelection != nil && aggregate.Facts.Resources.TargetSelection.SnapshotID == selection.SnapshotID
		},
	}
	responseLost, err := runtimev2.NewRuntimeController(responseLoss)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "creation-selection-response-loss" {
				t.Fatalf("unexpected creation selection response-loss marker: %v", recovered)
			}
		}()
		_, _ = responseLost.CommitPrerequisite(ctx, "ws-v2", "run-v2", result)
		t.Fatal("expected creation selection response loss after durable CAS")
	}()
	if !responseLoss.fired {
		t.Fatal("creation selection response-loss CAS point was not reached")
	}
	restarted, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedAfter, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || loadedAfter.StateRevision <= initialized.StateRevision || loadedAfter.Facts.Resources.TargetSelection == nil ||
		loadedAfter.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionCreate ||
		loadedAfter.Facts.Resources.TargetSelection.SnapshotID != selection.SnapshotID {
		t.Fatalf("restart did not reuse committed creation selection=%+v err=%v", loadedAfter, err)
	}
	if _, err := restarted.CommitPrerequisite(ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("duplicate creation selection unexpectedly mutated committed aggregate")
	}
	final, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil || final.StateRevision != loadedAfter.StateRevision || final.Facts.Resources.TargetSelection == nil ||
		final.Facts.Resources.TargetSelection.SnapshotID != selection.SnapshotID {
		t.Fatalf("duplicate creation selection changed persisted aggregate=%+v err=%v", final, err)
	}
}

// A selected target is not usable until File has committed its current
// snapshot. Verify the second half of that grounding gate has the same CAS
// atomicity: a restart may see either the pending prerequisite or the complete
// snapshot, never a hybrid that lets a provider request bypass freshness.
func TestRuntimeV2StateRepositoryTargetSnapshotCommitCrashRestartIsAtomic(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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

	repository := runtimeV2StateRepository{hub: hub}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Focus.RequireTargetGrounding = true
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var prerequisite runtimecontract.RuntimePrerequisite
	for _, value := range initialized.Facts.Prerequisites.Pending {
		if value.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && value.SubjectRef == "target.txt" {
			prerequisite = value
			break
		}
	}
	if prerequisite.ID == "" {
		t.Fatalf("target snapshot prerequisite missing: %+v", initialized.Facts.Prerequisites)
	}
	result := runtimev2.PrerequisiteResult{
		PrerequisiteID: prerequisite.ID,
		TargetSnapshot: &runtimecontract.TargetSnapshot{
			TargetRef: "target.txt", Content: "before\n", ContentHash: "target-hash-v1",
			ContentState: runtimecontract.SnapshotContentStateComplete,
		},
	}
	beforeCommitRepository := &failOnceRuntimeStateRepository{
		RuntimeStateRepository: repository,
		err:                    errors.New("injected target snapshot CAS failure"),
	}
	beforeCommitController, err := runtimev2.NewRuntimeController(beforeCommitRepository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beforeCommitController.CommitPrerequisite(ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("target snapshot unexpectedly persisted despite injected Run-owner CAS failure")
	}

	restartedBeforeCommit, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedBeforeCommit, err := restartedBeforeCommit.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loadedBeforeCommit.StateRevision != initialized.StateRevision || len(loadedBeforeCommit.Facts.Resources.TargetSnapshots) != 0 {
		t.Fatalf("failed target snapshot CAS partially persisted authority: %+v", loadedBeforeCommit)
	}
	foundPending := false
	for _, pending := range loadedBeforeCommit.Facts.Prerequisites.Pending {
		foundPending = foundPending || pending.ID == prerequisite.ID
	}
	if !foundPending {
		t.Fatalf("failed target snapshot CAS removed prerequisite: %+v", loadedBeforeCommit.Facts.Prerequisites)
	}

	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: repository, marker: "target-snapshot-response-loss",
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			return len(aggregate.Facts.Resources.TargetSnapshots) == 1 && aggregate.Facts.Resources.TargetSnapshots[0].ContentHash == "target-hash-v1"
		},
	}
	responseLost, err := runtimev2.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "target-snapshot-response-loss" {
				t.Fatalf("unexpected target-snapshot response-loss marker: %v", recovered)
			}
		}()
		_, _ = responseLost.CommitPrerequisite(ctx, "ws-v2", "run-v2", result)
		t.Fatal("expected target-snapshot response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("target-snapshot response-loss CAS point was not reached")
	}
	restartedAfterCommit, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedAfterCommit, err := restartedAfterCommit.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loadedAfterCommit.StateRevision <= initialized.StateRevision || len(loadedAfterCommit.Facts.Resources.TargetSnapshots) != 1 ||
		loadedAfterCommit.Facts.Resources.TargetSnapshots[0].ContentHash != "target-hash-v1" {
		t.Fatalf("restart did not reuse committed target snapshot: %+v", loadedAfterCommit)
	}
	if _, err := restartedAfterCommit.CommitPrerequisite(ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("duplicate target snapshot unexpectedly mutated committed aggregate")
	}
	final, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if final.StateRevision != loadedAfterCommit.StateRevision || len(final.Facts.Resources.TargetSnapshots) != 1 ||
		final.Facts.Resources.TargetSnapshots[0].ContentHash != "target-hash-v1" {
		t.Fatalf("duplicate target snapshot changed persisted aggregate: %+v", final)
	}
}

// Manifest and operation-admissibility are owner observations that gate all
// later provider work. A response lost after their Run-owner CAS must retain
// exactly the committed owner fact; a restart must never reacquire it from a
// controller-local value or create a second observation.
func TestRuntimeV2StateRepositoryOwnerPrerequisiteResponseLossIsCommittedOnly(t *testing.T) {
	testCases := []struct {
		name         string
		requirement  runtimev2.RuntimeRequirement
		resultFor    func(runtimecontract.RuntimePrerequisite) runtimev2.PrerequisiteResult
		committed    func(runtimev2.RuntimeAggregate) bool
		assertLoaded func(*testing.T, runtimev2.RuntimeAggregate)
	}{
		{
			name:        "manifest",
			requirement: runtimev2.RuntimeRequirement{Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile, Reason: "file owner manifest"},
			resultFor: func(prerequisite runtimecontract.RuntimePrerequisite) runtimev2.PrerequisiteResult {
				return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, Manifest: &runtimecontract.WorkspaceManifest{Revision: "workspace-tree-v1", TargetRefs: []string{"implementation.go"}}}
			},
			committed: func(aggregate runtimev2.RuntimeAggregate) bool {
				return aggregate.Facts.Resources.Manifest != nil && aggregate.Facts.Resources.Manifest.Revision == "workspace-tree-v1"
			},
			assertLoaded: func(t *testing.T, aggregate runtimev2.RuntimeAggregate) {
				t.Helper()
				if aggregate.Facts.Resources.Manifest == nil || aggregate.Facts.Resources.Manifest.Revision != "workspace-tree-v1" || len(aggregate.Facts.Resources.Manifest.TargetRefs) != 1 {
					t.Fatalf("committed manifest missing after response loss: %+v", aggregate.Facts.Resources.Manifest)
				}
			},
		},
		{
			name: "operation_admissibility",
			requirement: runtimev2.RuntimeRequirement{
				Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				RequiredCapabilities: []string{runtimecontract.StepKindFileWrite}, Reason: "file owner inventory",
			},
			resultFor: func(prerequisite runtimecontract.RuntimePrerequisite) runtimev2.PrerequisiteResult {
				inventory := runtimecontract.RuntimeOperationAdmissibility{
					PrerequisiteID: prerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"implementation.go"}, ObservedRevision: "workspace-tree-v1",
				}
				inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
				return runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID, OperationAdmissibility: &inventory}
			},
			committed: func(aggregate runtimev2.RuntimeAggregate) bool {
				return len(aggregate.Facts.Resources.OperationAdmissibilities) == 1 && aggregate.Facts.Resources.OperationAdmissibilities[0].ObservedRevision == "workspace-tree-v1"
			},
			assertLoaded: func(t *testing.T, aggregate runtimev2.RuntimeAggregate) {
				t.Helper()
				if len(aggregate.Facts.Resources.OperationAdmissibilities) != 1 || aggregate.Facts.Resources.OperationAdmissibilities[0].SnapshotID == "" ||
					aggregate.Facts.Resources.OperationAdmissibilities[0].OperationKinds[0] != runtimecontract.StepKindFileWrite {
					t.Fatalf("committed admissibility missing after response loss: %+v", aggregate.Facts.Resources.OperationAdmissibilities)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
			if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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

			repository := runtimeV2StateRepository{hub: hub}
			controller, err := runtimev2.NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			initialized, err := controller.Initialize(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), []runtimev2.RuntimeRequirement{testCase.requirement})
			if err != nil || len(initialized.Facts.Prerequisites.Pending) != 1 {
				t.Fatalf("prerequisite initialization aggregate=%+v err=%v", initialized, err)
			}
			result := testCase.resultFor(initialized.Facts.Prerequisites.Pending[0])
			responseLossRepository := &panicAfterRuntimeStateSaveRepository{
				RuntimeStateRepository: repository, marker: "owner-prerequisite-response-loss/" + testCase.name,
				match: testCase.committed,
			}
			responseLost, err := runtimev2.NewRuntimeController(responseLossRepository)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != "owner-prerequisite-response-loss/"+testCase.name {
						t.Fatalf("unexpected %s response-loss marker: %v", testCase.name, recovered)
					}
				}()
				_, _ = responseLost.CommitPrerequisite(ctx, "ws-v2", "run-v2", result)
				t.Fatal("expected owner prerequisite response loss after durable CAS")
			}()
			if !responseLossRepository.fired {
				t.Fatal("owner prerequisite response-loss CAS point was not reached")
			}

			restarted, err := runtimev2.NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
			if err != nil || loaded.StateRevision <= initialized.StateRevision || len(loaded.Facts.Prerequisites.Pending) != 0 {
				t.Fatalf("restart did not consume committed owner prerequisite: aggregate=%+v err=%v", loaded, err)
			}
			testCase.assertLoaded(t, loaded)
			if _, err := restarted.CommitPrerequisite(ctx, "ws-v2", "run-v2", result); err == nil {
				t.Fatal("duplicate owner prerequisite unexpectedly mutated committed aggregate")
			}
			reloaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
			if err != nil || reloaded.StateRevision != loaded.StateRevision {
				t.Fatalf("duplicate owner prerequisite changed aggregate: reloaded=%+v err=%v", reloaded, err)
			}
		})
	}
}

// Candidate proposal and dispatch seal are two distinct SM-3 CAS boundaries.
// A process loss before either Run-owner write must leave a restart with the
// last committed pending attempt only: a provider response is not a proposal
// until it is persisted, and a proposal is not executable until the sealed
// dispatch is persisted with it.
func TestRuntimeV2StateRepositoryCandidateProposalAndDispatchSealCrashRestartAreAtomic(t *testing.T) {
	newFixture := func(t *testing.T) (context.Context, runtimev2.RuntimeStateRepository, *runtimev2.RuntimeController) {
		t.Helper()
		ctx := context.Background()
		hub := magicevent.NewHub(16)
		t.Cleanup(func() { hub.Terminate(ctx) })
		runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
		if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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

		repository := runtimeV2StateRepository{hub: hub}
		controller, err := runtimev2.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := controller.Initialize(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil); err != nil {
			t.Fatal(err)
		}
		if _, err := controller.BeginCandidateAttempt(ctx, "ws-v2", "run-v2", 0, "candidate-cas-prefix"); err != nil {
			t.Fatal(err)
		}
		return ctx, repository, controller
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "candidate-cas-edit", Kind: runtimecontract.StepKindFileEdit,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
		Parameters: runtimeV2FileEditParameters("target.txt"),
	}}}}}

	t.Run("proposal", func(t *testing.T) {
		ctx, repository, _ := newFixture(t)
		beforeCommit, err := runtimev2.NewRuntimeController(&failOnceRuntimeStateRepository{
			RuntimeStateRepository: repository,
			err:                    errors.New("injected candidate proposal CAS failure"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := beforeCommit.AdmitCandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate); err == nil {
			t.Fatal("candidate proposal unexpectedly persisted despite injected Run-owner CAS failure")
		}

		restarted, err := runtimev2.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil {
			t.Fatal(err)
		}
		if len(loaded.CandidateAttempts) != 1 || loaded.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStatePending ||
			loaded.CandidateAttempts[0].Proposal != nil || loaded.ActiveDispatch != nil {
			t.Fatalf("failed proposal CAS partially persisted candidate authority: %+v", loaded)
		}

		proposal, err := restarted.AdmitCandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate)
		if err != nil || !proposal.Admission.Accepted() || proposal.Aggregate.CandidateAttempts[0].Proposal == nil {
			t.Fatalf("proposal retry=%+v err=%v", proposal, err)
		}
		persisted, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || persisted.CandidateAttempts[0].Proposal == nil || persisted.ActiveDispatch != nil {
			t.Fatalf("restart did not reuse committed proposal: aggregate=%+v err=%v", persisted, err)
		}
	})

	t.Run("dispatch_seal", func(t *testing.T) {
		ctx, repository, controller := newFixture(t)
		proposal, err := controller.AdmitCandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate)
		if err != nil || !proposal.Admission.Accepted() || proposal.Aggregate.CandidateAttempts[0].Proposal == nil {
			t.Fatalf("proposal setup=%+v err=%v", proposal, err)
		}
		beforeCommit, err := runtimev2.NewRuntimeController(&failOnceRuntimeStateRepository{
			RuntimeStateRepository: repository,
			err:                    errors.New("injected dispatch seal CAS failure"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := beforeCommit.AdmitAndSealCandidateAttempt(ctx, "ws-v2", "run-v2", 0, candidate); err == nil {
			t.Fatal("dispatch seal unexpectedly persisted despite injected Run-owner CAS failure")
		}

		restarted, err := runtimev2.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil {
			t.Fatal(err)
		}
		if len(loaded.CandidateAttempts) != 1 || loaded.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStatePending ||
			loaded.CandidateAttempts[0].Proposal == nil || loaded.ActiveDispatch != nil {
			t.Fatalf("failed dispatch-seal CAS partially persisted executable authority: %+v", loaded)
		}

		sealed, err := restarted.AdmitAndSealCandidateAttempt(ctx, "ws-v2", "run-v2", 0, candidate)
		if err != nil || !sealed.Admission.Accepted() || sealed.Aggregate.ActiveDispatch == nil ||
			sealed.Aggregate.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted ||
			sealed.Aggregate.CandidateAttempts[0].Proposal != nil {
			t.Fatalf("dispatch-seal retry=%+v err=%v", sealed, err)
		}
		persisted, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || persisted.ActiveDispatch == nil ||
			persisted.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
			t.Fatalf("restart did not reuse committed sealed dispatch: aggregate=%+v err=%v", persisted, err)
		}
	})

	t.Run("proposal_response_loss", func(t *testing.T) {
		ctx, repository, _ := newFixture(t)
		responseLossRepository := &panicAfterRuntimeStateSaveRepository{
			RuntimeStateRepository: repository, marker: "candidate-proposal-response-loss",
			match: func(aggregate runtimev2.RuntimeAggregate) bool {
				return aggregate.ActiveDispatch == nil && len(aggregate.CandidateAttempts) == 1 &&
					aggregate.CandidateAttempts[0].State == runtimecontract.RuntimeCandidateAttemptStatePending && aggregate.CandidateAttempts[0].Proposal != nil
			},
		}
		responseLost, err := runtimev2.NewRuntimeController(responseLossRepository)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != "candidate-proposal-response-loss" {
					t.Fatalf("unexpected proposal response-loss marker: %v", recovered)
				}
			}()
			_, _ = responseLost.AdmitCandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate)
			t.Fatal("expected proposal response loss after durable CAS")
		}()
		if !responseLossRepository.fired {
			t.Fatal("proposal response-loss CAS point was not reached")
		}

		restarted, err := runtimev2.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || loaded.ActiveDispatch != nil || len(loaded.CandidateAttempts) != 1 || loaded.CandidateAttempts[0].Proposal == nil {
			t.Fatalf("restart did not retain committed proposal after response loss: aggregate=%+v err=%v", loaded, err)
		}
		sealed, err := restarted.AdmitAndSealCandidateAttempt(ctx, "ws-v2", "run-v2", 0, candidate)
		if err != nil || sealed.Aggregate.ActiveDispatch == nil || sealed.Aggregate.ActiveDispatch.CandidateHash != runtimecontract.StableRuntimeCandidateHash(candidate) {
			t.Fatalf("response-loss proposal did not seal the original candidate: sealed=%+v err=%v", sealed, err)
		}
	})

	t.Run("dispatch_seal_response_loss", func(t *testing.T) {
		ctx, repository, controller := newFixture(t)
		if _, err := controller.AdmitCandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate); err != nil {
			t.Fatal(err)
		}
		responseLossRepository := &panicAfterRuntimeStateSaveRepository{
			RuntimeStateRepository: repository, marker: "dispatch-seal-response-loss",
			match: func(aggregate runtimev2.RuntimeAggregate) bool {
				return aggregate.ActiveDispatch != nil && len(aggregate.CandidateAttempts) == 1 &&
					aggregate.CandidateAttempts[0].State == runtimecontract.RuntimeCandidateAttemptStateAccepted
			},
		}
		responseLost, err := runtimev2.NewRuntimeController(responseLossRepository)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != "dispatch-seal-response-loss" {
					t.Fatalf("unexpected dispatch-seal response-loss marker: %v", recovered)
				}
			}()
			_, _ = responseLost.AdmitAndSealCandidateAttempt(ctx, "ws-v2", "run-v2", 0, candidate)
			t.Fatal("expected dispatch-seal response loss after durable CAS")
		}()
		if !responseLossRepository.fired {
			t.Fatal("dispatch-seal response-loss CAS point was not reached")
		}

		restarted, err := runtimev2.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || loaded.ActiveDispatch == nil || len(loaded.CandidateAttempts) != 1 ||
			loaded.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || loaded.CandidateAttempts[0].Proposal != nil {
			t.Fatalf("restart did not retain sealed dispatch after response loss: aggregate=%+v err=%v", loaded, err)
		}
		replayed, err := restarted.AdmitAndSealCandidateAttempt(ctx, "ws-v2", "run-v2", 0, candidate)
		if err != nil || replayed.Aggregate.StateRevision != loaded.StateRevision || replayed.Aggregate.ActiveDispatch == nil ||
			replayed.Aggregate.ActiveDispatch.ID != loaded.ActiveDispatch.ID || len(replayed.Aggregate.CompletedDispatches) != len(loaded.CompletedDispatches) {
			t.Fatalf("committed dispatch response-loss replay was not idempotent: replayed=%+v loaded=%+v err=%v", replayed, loaded, err)
		}
	})
}

// An owner-observed validation failure becomes FailureFact, FailureTarget,
// rejected evidence and a fresh target prerequisite in one authority commit.
// Exercise that transition through the production Run-owner repository: a
// crash before the receipt CAS must not leak any of those facts, while a
// restart can safely replay the read-only validation operation and commit the
// complete observation exactly once.
func TestRuntimeV2StateRepositoryOwnerFailureObservationReceiptCrashRestartIsAtomic(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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

	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Focus.TargetRefs = []string{"validation-entry.txt"}
	facts.Focus.TargetRole = "implementation"
	facts.Focus.RequireTargetGrounding = true
	facts.Capabilities = runtimecontract.RuntimeCapabilities{
		StateChangeKinds: []string{runtimecontract.StepKindFileEdit},
		ValidationKinds:  []string{runtimecontract.StepKindFileRead},
	}
	facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
		TargetRef: "validation-entry.txt", ContentHash: "validation-entry-v1", ContentState: runtimecontract.SnapshotContentStateComplete,
	}}
	catalogObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: "capability:blocks/mcp:failure-observer",
		Kind:           runtimecontract.PrerequisiteKindCapability,
		OwnerModuleID:  runtimecontract.RuntimeModuleMCP,
		ContentHash:    "mcp-catalog-v1",
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP,
			SubjectRef: "mcp_server_catalog", Revision: "mcp-catalog-v1",
		}},
	}
	catalogObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(catalogObservation)
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{{
		ID: catalogObservation.PrerequisiteID, Kind: catalogObservation.Kind, OwnerModuleID: catalogObservation.OwnerModuleID,
		RequiredCapabilities: []string{runtimecontract.StepKindMCPToolCall}, Reason: "failure observer catalog",
	}}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{catalogObservation}

	repository := runtimeV2StateRepository{hub: hub}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil || initialized.Decision.Action != runtimecontract.RuntimeActionProduceValidation {
		t.Fatalf("validation aggregate initialization=%+v err=%v", initialized, err)
	}
	if _, err := controller.BeginCandidateAttempt(ctx, "ws-v2", "run-v2", 0, "failure-observation-prefix"); err != nil {
		t.Fatal(err)
	}
	path := "validation-entry.txt"
	candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "failure-observation-read", Kind: runtimecontract.StepKindFileRead,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{path},
		Parameters: []runtimecontract.OperationParameter{{Name: "path", String: &path}},
	}}}}}
	sealed, err := controller.AdmitAndSealCandidateAttempt(ctx, "ws-v2", "run-v2", 0, candidate)
	if err != nil || !sealed.Admission.Accepted() || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("validation candidate seal=%+v err=%v", sealed, err)
	}

	observedFailure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassBuildFailure, Code: "implementation_check_failed",
		Signature: "implementation-check-failed", Message: "capability observed implementation failure",
		TargetRefs: []string{"implementation.go"},
		TargetObservation: &runtimecontract.RuntimeFailureTargetObservation{
			OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation",
			ObservedRevision: "mcp-catalog-v1", TargetRefs: []string{"implementation.go"},
		},
	}
	newExecutor := func(owner *runtimev2.RuntimeController, crashOnReceiptError bool) runtimev2.RuntimeOperationExecutor {
		return runtimeOperationExecutorFunc(func(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest) (runtimev2.RuntimeOperationExecutionResult, error) {
			receipt := runtimecontract.RuntimeOperationReceipt{
				ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
				Outcome: runtimecontract.RuntimeOperationOutcomeRejected, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
				Failure: &observedFailure, Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "implementation check failed"},
			}
			_, committed, err := owner.CommitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt)
			if err != nil {
				if crashOnReceiptError {
					panic("failure-observation-receipt-cas")
				}
				return runtimev2.RuntimeOperationExecutionResult{}, err
			}
			return runtimev2.RuntimeOperationExecutionResult{
				EventID: "failure-observation-event", OperationID: request.Operation.ID, EvidenceKind: runtimecontract.EvidenceKindValidation,
				Status: runtimev2.ExecutionEvidenceStatusRejected, SourceRef: "workorch://validation/failure-observation",
				Failure: &observedFailure, Evidence: committed.Evidence, Receipt: committed,
			}, nil
		})
	}

	beforeCommitRepository := &failWhenRuntimeStateRepository{
		RuntimeStateRepository: repository,
		err:                    errors.New("injected owner failure receipt CAS failure"),
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			return len(aggregate.OperationReceipts) == 1
		},
	}
	beforeCommitController, err := runtimev2.NewRuntimeController(beforeCommitRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "failure-observation-receipt-cas" {
				t.Fatalf("unexpected receipt crash marker: %v", recovered)
			}
		}()
		_, _ = beforeCommitController.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", newExecutor(beforeCommitController, true))
		t.Fatal("expected injected failure-observation receipt crash")
	}()
	if !beforeCommitRepository.failed {
		t.Fatal("failure-observation receipt CAS crash point was not reached")
	}

	restarted, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	beforeRetry, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if len(beforeRetry.OperationReceipts) != 0 || beforeRetry.Facts.Failure != nil || beforeRetry.Facts.Resources.FailureTarget != nil ||
		len(beforeRetry.Facts.Evidence.Rejected) != 0 || beforeRetry.ActiveDispatch == nil {
		t.Fatalf("receipt CAS failure partially persisted failure authority: %+v", beforeRetry)
	}

	committed, err := restarted.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", newExecutor(restarted, false))
	if err != nil {
		t.Fatal(err)
	}
	if len(committed.OperationReceipts) != 1 || len(committed.Facts.Evidence.Rejected) != 1 || committed.Facts.Failure == nil ||
		committed.Facts.Resources.FailureTarget == nil || committed.Facts.Resources.FailureTarget.TargetObservation == nil ||
		committed.Facts.Resources.FailureTarget.TargetObservation.SubjectRef != "server-a" ||
		committed.Decision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("restarted failure-observation commit=%+v", committed)
	}
	persisted, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.StateRevision != committed.StateRevision || len(persisted.OperationReceipts) != 1 ||
		persisted.Facts.Resources.FailureTarget == nil || len(persisted.Facts.Evidence.Rejected) != 1 {
		t.Fatalf("restart did not reuse committed failure observation: aggregate=%+v err=%v", persisted, err)
	}
}

// Assessment is the SM-2 boundary that turns accepted evidence into coverage
// progress (and possibly a terminal authority). Its CAS must therefore keep
// the assessment dispatch, coverage, repair ledger and terminal decision
// indivisible across a Run-owner restart.
func TestRuntimeV2StateRepositoryAssessmentCommitCrashRestartIsAtomic(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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

	repository := runtimeV2StateRepository{hub: hub}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Focus.RequireAssessment = true
	if _, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(ctx, "ws-v2", "run-v2", 0, "assessment-state-change-prefix"); err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "assessment-write", Kind: runtimecontract.StepKindFileEdit,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
		Parameters: runtimeV2FileEditParameters("target.txt"),
	}}}}}
	sealed, err := controller.AdmitAndSealCandidateAttempt(ctx, "ws-v2", "run-v2", 0, candidate)
	if err != nil || !sealed.Admission.Accepted() || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("state-change seal=%+v err=%v", sealed, err)
	}
	stateChangeExecutor := runtimeOperationExecutorFunc(func(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest) (runtimev2.RuntimeOperationExecutionResult, error) {
		receipt := runtimecontract.RuntimeOperationReceipt{
			ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
			Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
			Changed: true, Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "state change applied"},
		}
		_, committed, err := controller.CommitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt)
		if err != nil {
			return runtimev2.RuntimeOperationExecutionResult{}, err
		}
		return runtimev2.RuntimeOperationExecutionResult{
			EventID: "assessment-state-change-evidence", OperationID: request.Operation.ID, EvidenceKind: runtimecontract.EvidenceKindStateChange,
			Status: runtimev2.ExecutionEvidenceStatusAccepted, SourceRef: "workorch://state-change/assessment-write",
			Evidence: committed.Evidence, Receipt: committed,
		}, nil
	})
	afterEvidence, err := controller.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", stateChangeExecutor)
	if err != nil || afterEvidence.Decision.Action != runtimecontract.RuntimeActionProduceAssessment {
		t.Fatalf("state-change evidence did not reach assessment: aggregate=%+v err=%v", afterEvidence, err)
	}
	if _, err := controller.BeginCandidateAttempt(ctx, "ws-v2", "run-v2", 0, "assessment-commit-prefix"); err != nil {
		t.Fatal(err)
	}
	assessmentCandidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "current focus satisfied",
		EvidenceRefs: []string{"assessment-state-change-evidence"},
	}}
	assessmentDispatch, err := controller.AdmitAndSealCandidateAttempt(ctx, "ws-v2", "run-v2", 0, assessmentCandidate)
	if err != nil || !assessmentDispatch.Admission.Accepted() || assessmentDispatch.Aggregate.ActiveDispatch == nil {
		t.Fatalf("assessment seal=%+v err=%v", assessmentDispatch, err)
	}

	beforeCommit, err := runtimev2.NewRuntimeController(&failOnceRuntimeStateRepository{
		RuntimeStateRepository: repository,
		err:                    errors.New("injected assessment CAS failure"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beforeCommit.CommitAssessment(ctx, "ws-v2", "run-v2"); err == nil {
		t.Fatal("assessment unexpectedly persisted despite injected Run-owner CAS failure")
	}

	restarted, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.StateRevision != assessmentDispatch.Aggregate.StateRevision || loaded.ActiveDispatch == nil || loaded.ActiveDispatch.Candidate.Assessment == nil ||
		len(loaded.Facts.Assessments) != 0 || loaded.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateOpen ||
		loaded.Decision.Action != runtimecontract.RuntimeActionProduceAssessment {
		t.Fatalf("assessment CAS failure partially persisted progress or terminal authority: %+v", loaded)
	}

	// The same assessment transition now commits, but its caller loses the
	// response after the Run-owner durable write. The next controller is only
	// allowed to consume that exact aggregate; it must not re-run assessment or
	// advance focus a second time.
	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: repository, marker: "assessment-cas-response-loss",
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			return aggregate.Decision.Action == runtimecontract.RuntimeActionComplete && aggregate.ActiveDispatch == nil &&
				len(aggregate.Facts.Assessments) == 1 && aggregate.Facts.Coverage[0].State == runtimecontract.RuntimeCoverageItemStateSatisfied
		},
	}
	responseLost, err := runtimev2.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "assessment-cas-response-loss" {
				t.Fatalf("unexpected assessment response-loss marker: %v", recovered)
			}
		}()
		_, _ = responseLost.CommitAssessment(ctx, "ws-v2", "run-v2")
		t.Fatal("expected assessment response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("assessment response-loss CAS point was not reached")
	}

	restartedAfterResponseLoss, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := restartedAfterResponseLoss.Load(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.StateRevision <= assessmentDispatch.Aggregate.StateRevision || persisted.Decision.Action != runtimecontract.RuntimeActionComplete ||
		persisted.ActiveDispatch != nil || len(persisted.Facts.Assessments) != 1 || persisted.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateSatisfied || persisted.Facts.Repair.LastProgressRevision == "" {
		t.Fatalf("restart did not reuse response-loss assessment transition: aggregate=%+v err=%v", persisted, err)
	}
	if _, err := restartedAfterResponseLoss.CommitAssessment(ctx, "ws-v2", "run-v2"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("committed assessment was replayed instead of rejected: %v", err)
	}
	reloaded, err := restartedAfterResponseLoss.Load(ctx, "ws-v2", "run-v2")
	if err != nil || reloaded.StateRevision != persisted.StateRevision || len(reloaded.Facts.Assessments) != 1 ||
		reloaded.Facts.Repair.LastProgressRevision != persisted.Facts.Repair.LastProgressRevision {
		t.Fatalf("assessment response-loss replay changed committed aggregate: reloaded=%+v err=%v", reloaded, err)
	}
}

// A repairable assessment that reports the same persisted gap is not progress.
// Its CAS recovery must retain the current focus, coverage and no-progress
// lineage whether the write fails before commit or the caller loses the
// response after the Run owner has persisted it.
func TestRuntimeV2StateRepositorySameGapAssessmentCrashRestartRetainsRepairWindow(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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

	repository := runtimeV2StateRepository{hub: hub}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Focus.RequireAssessment = true
	facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: facts.Focus.ID, SliceID: facts.Focus.SliceID, RepairAttempts: 2, NoProgressCount: 1,
		ActiveGap: &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...), Reason: "same gap"},
	}
	if _, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(ctx, "ws-v2", "run-v2", 0, "same-gap-state-prefix"); err != nil {
		t.Fatal(err)
	}
	stateCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "same-gap-write", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
	}}}}}
	sealed, err := controller.AdmitAndSealCandidateAttempt(ctx, "ws-v2", "run-v2", 0, stateCandidate)
	if err != nil || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("same-gap state candidate seal=%+v err=%v", sealed, err)
	}
	executor := runtimeOperationExecutorFunc(func(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest) (runtimev2.RuntimeOperationExecutionResult, error) {
		receipt := runtimecontract.RuntimeOperationReceipt{
			ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID, Outcome: runtimecontract.RuntimeOperationOutcomeAccepted,
			ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true, Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "same-gap write"},
		}
		_, committed, err := controller.CommitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt)
		if err != nil {
			return runtimev2.RuntimeOperationExecutionResult{}, err
		}
		return runtimev2.RuntimeOperationExecutionResult{EventID: "same-gap-evidence", OperationID: request.Operation.ID, EvidenceKind: runtimecontract.EvidenceKindStateChange, Status: runtimev2.ExecutionEvidenceStatusAccepted, Evidence: committed.Evidence, Receipt: committed}, nil
	})
	afterEvidence, err := controller.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", executor)
	if err != nil || afterEvidence.Decision.Action != runtimecontract.RuntimeActionProduceAssessment {
		t.Fatalf("same-gap state evidence did not reach assessment: aggregate=%+v err=%v", afterEvidence, err)
	}
	if _, err := controller.BeginCandidateAttempt(ctx, "ws-v2", "run-v2", 0, "same-gap-assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	assessmentCandidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionRepairableGap,
		Summary: "same gap persists", Gap: &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Reason: "same gap"},
	}}
	assessmentDispatch, err := controller.AdmitAndSealCandidateAttempt(ctx, "ws-v2", "run-v2", 0, assessmentCandidate)
	if err != nil || assessmentDispatch.Aggregate.ActiveDispatch == nil {
		t.Fatalf("same-gap assessment seal=%+v err=%v", assessmentDispatch, err)
	}

	beforeCommit, err := runtimev2.NewRuntimeController(&failOnceRuntimeStateRepository{RuntimeStateRepository: repository, err: errors.New("injected same-gap assessment CAS failure")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beforeCommit.CommitAssessment(ctx, "ws-v2", "run-v2"); err == nil {
		t.Fatal("same-gap assessment unexpectedly persisted before CAS")
	}
	restartedBefore, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := restartedBefore.Load(ctx, "ws-v2", "run-v2")
	if err != nil || pending.ActiveDispatch == nil || pending.ActiveDispatch.Candidate.Assessment == nil || len(pending.Facts.Assessments) != 0 || pending.Facts.Repair.LastProgressRevision != "" || pending.Facts.Repair.NoProgressCount != 1 {
		t.Fatalf("same-gap pre-commit restart leaked progress: aggregate=%+v err=%v", pending, err)
	}

	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: repository, marker: "same-gap-assessment-response-loss",
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			return aggregate.ActiveDispatch == nil && len(aggregate.Facts.Assessments) == 1 && aggregate.Facts.Assessments[0].Disposition == runtimecontract.AssessmentDispositionRepairableGap
		},
	}
	responseLost, err := runtimev2.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "same-gap-assessment-response-loss" {
				t.Fatalf("unexpected same-gap response-loss marker: %v", recovered)
			}
		}()
		_, _ = responseLost.CommitAssessment(ctx, "ws-v2", "run-v2")
		t.Fatal("expected same-gap assessment response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("same-gap assessment response-loss CAS point was not reached")
	}

	restarted, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.StateRevision <= assessmentDispatch.Aggregate.StateRevision || persisted.ActiveDispatch != nil || len(persisted.Facts.Assessments) != 1 ||
		persisted.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateOpen || persisted.Facts.Repair.LastProgressRevision != "" || persisted.Facts.Repair.ActiveGap == nil ||
		persisted.Facts.Repair.NoProgressCount < 1 || len(persisted.Facts.Repair.NoProgressOperationShapes) == 0 {
		t.Fatalf("same-gap response-loss restart changed repair window: aggregate=%+v err=%v", persisted, err)
	}
	if _, err := restarted.CommitAssessment(ctx, "ws-v2", "run-v2"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("same-gap assessment replay was not rejected: %v", err)
	}
}

// A partial assessment is the only repairable assessment that may reset the
// no-progress window: its persisted neutral gap must strictly narrow. The
// complete coverage/repair delta must nevertheless remain one Run-owner CAS
// transition across both pre-commit failure and post-commit response loss.
func TestRuntimeV2StateRepositoryPartialGapAssessmentCrashRestartPersistsSingleDelta(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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

	repository := runtimeV2StateRepository{hub: hub}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Focus.RequireAssessment = true
	facts.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-a", "cwi-b"}
	facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{
		{ID: "cwi-a", SliceID: facts.Focus.SliceID, State: runtimecontract.RuntimeCoverageItemStateOpen},
		{ID: "cwi-b", SliceID: facts.Focus.SliceID, State: runtimecontract.RuntimeCoverageItemStateOpen},
	}
	facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: facts.Focus.ID, SliceID: facts.Focus.SliceID, RepairAttempts: 2, ValidationOnlyRetries: 1, NoProgressCount: 2,
		SignatureCounts:           []runtimecontract.RuntimeFailureSignatureCount{{Signature: "prior-gap", Count: 2}},
		NoProgressOperationShapes: []runtimecontract.RuntimeRejectedOperationShape{{Operation: runtimecontract.RuntimeOperation{Kind: runtimecontract.StepKindFileEdit}}},
		ActiveGap:                 &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a", "cwi-b"}, Reason: "both remain open"},
	}
	if _, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(ctx, "ws-v2", "run-v2", 0, "partial-gap-state-prefix"); err != nil {
		t.Fatal(err)
	}
	stateCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "partial-gap-write", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a", "cwi-b"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
	}}}}}
	sealed, err := controller.AdmitAndSealCandidateAttempt(ctx, "ws-v2", "run-v2", 0, stateCandidate)
	if err != nil || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("partial-gap state candidate seal=%+v err=%v", sealed, err)
	}
	executor := runtimeOperationExecutorFunc(func(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest) (runtimev2.RuntimeOperationExecutionResult, error) {
		receipt := runtimecontract.RuntimeOperationReceipt{
			ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID, Outcome: runtimecontract.RuntimeOperationOutcomeAccepted,
			ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true, Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "partial-gap write"},
		}
		_, committed, err := controller.CommitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt)
		if err != nil {
			return runtimev2.RuntimeOperationExecutionResult{}, err
		}
		return runtimev2.RuntimeOperationExecutionResult{EventID: "partial-gap-evidence", OperationID: request.Operation.ID, EvidenceKind: runtimecontract.EvidenceKindStateChange, Status: runtimev2.ExecutionEvidenceStatusAccepted, Evidence: committed.Evidence, Receipt: committed}, nil
	})
	afterEvidence, err := controller.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", executor)
	if err != nil || afterEvidence.Decision.Action != runtimecontract.RuntimeActionProduceAssessment {
		t.Fatalf("partial-gap state evidence did not reach assessment: aggregate=%+v err=%v", afterEvidence, err)
	}
	if _, err := controller.BeginCandidateAttempt(ctx, "ws-v2", "run-v2", 0, "partial-gap-assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	assessmentCandidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a", "cwi-b"}, Disposition: runtimecontract.AssessmentDispositionRepairableGap,
		Summary: "one remaining item", Items: []runtimecontract.RuntimeAssessmentItemFact{
			{CoverageWorkItemID: "cwi-a", Satisfied: true, EvidenceRefs: []string{"partial-gap-evidence"}},
			{CoverageWorkItemID: "cwi-b", Satisfied: false, Reason: "still open"},
		}, Gap: &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-b"}, Reason: "one remaining item"},
	}}
	assessmentDispatch, err := controller.AdmitAndSealCandidateAttempt(ctx, "ws-v2", "run-v2", 0, assessmentCandidate)
	if err != nil || assessmentDispatch.Aggregate.ActiveDispatch == nil {
		t.Fatalf("partial-gap assessment seal=%+v err=%v", assessmentDispatch, err)
	}

	beforeCommit, err := runtimev2.NewRuntimeController(&failOnceRuntimeStateRepository{RuntimeStateRepository: repository, err: errors.New("injected partial-gap assessment CAS failure")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beforeCommit.CommitAssessment(ctx, "ws-v2", "run-v2"); err == nil {
		t.Fatal("partial-gap assessment unexpectedly persisted before CAS")
	}
	restartedBefore, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := restartedBefore.Load(ctx, "ws-v2", "run-v2")
	if err != nil || pending.ActiveDispatch == nil || len(pending.Facts.Assessments) != 0 || pending.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateOpen || pending.Facts.Coverage[1].State != runtimecontract.RuntimeCoverageItemStateOpen || pending.Facts.Repair.LastProgressRevision != "" || pending.Facts.Repair.NoProgressCount != 2 {
		t.Fatalf("partial-gap pre-commit restart leaked delta: aggregate=%+v err=%v", pending, err)
	}

	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: repository, marker: "partial-gap-assessment-response-loss",
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			return aggregate.ActiveDispatch == nil && len(aggregate.Facts.Assessments) == 1 && aggregate.Facts.Assessments[0].Disposition == runtimecontract.AssessmentDispositionRepairableGap
		},
	}
	responseLost, err := runtimev2.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "partial-gap-assessment-response-loss" {
				t.Fatalf("unexpected partial-gap response-loss marker: %v", recovered)
			}
		}()
		_, _ = responseLost.CommitAssessment(ctx, "ws-v2", "run-v2")
		t.Fatal("expected partial-gap assessment response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("partial-gap assessment response-loss CAS point was not reached")
	}

	restarted, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.StateRevision <= assessmentDispatch.Aggregate.StateRevision || persisted.ActiveDispatch != nil || len(persisted.Facts.Assessments) != 1 ||
		persisted.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateSatisfied || persisted.Facts.Coverage[1].State != runtimecontract.RuntimeCoverageItemStateUnsatisfied ||
		persisted.Facts.Repair.LastProgressRevision == "" || persisted.Facts.Repair.ActiveGap == nil || len(persisted.Facts.Repair.ActiveGap.CoverageWorkItemIDs) != 1 ||
		persisted.Facts.Repair.ActiveGap.CoverageWorkItemIDs[0] != "cwi-b" || persisted.Facts.Repair.NoProgressCount != 0 || len(persisted.Facts.Repair.NoProgressOperationShapes) != 0 {
		t.Fatalf("partial-gap response-loss restart did not persist one gap delta: aggregate=%+v err=%v", persisted, err)
	}
	if _, err := restarted.CommitAssessment(ctx, "ws-v2", "run-v2"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("partial-gap assessment replay was not rejected: %v", err)
	}
}

func TestRuntimeV2MainlinePreparesLocalPrerequisiteThroughPersistedController(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: t.TempDir(),
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	facts := runtimeV2Aggregate(t, 1).Facts
	initialized, err := biz.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), []runtimev2.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindValidationScratch}})
	if err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(biz, hub, nil)
	prepared, err := module.prepareRuntimeV2Prerequisites(ctx, initialized)
	if err != nil {
		t.Fatalf("prepareRuntimeV2Prerequisites: %v", err)
	}
	if prepared.Decision.Action != runtimecontract.RuntimeActionProduceStateChange || prepared.Facts.Resources.ValidationScratch == nil {
		t.Fatalf("prepared aggregate = %+v", prepared)
	}
	loaded, err := biz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.StateRevision != prepared.StateRevision {
		t.Fatalf("persisted prepared aggregate = %+v err=%v", loaded, err)
	}
}

func TestRuntimeV2OperationAdmissibilityRefreshUsesGroundedFailureTarget(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	defer observer.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility)
	var captured orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand
	observer.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, func(event magicevent.Event, result magicevent.Result) {
		captured = event.Data().(orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand)
		inventory := runtimecontract.RuntimeOperationAdmissibility{
			PrerequisiteID: captured.PrerequisiteID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			OperationKinds: captured.RequiredKinds, TargetRefs: captured.RequiredTargetRefs, ObservedRevision: "file-inventory-v1",
		}
		inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
		observation := runtimeV2FileAdmissibilityObservation(inventory)
		result.Set(orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{PrerequisiteID: captured.PrerequisiteID, Admissibility: inventory, Observation: &observation}, nil)
	})

	aggregate := runtimeV2Aggregate(t, 1)
	aggregate.Facts.Focus.TargetRefs = nil
	aggregate.Facts.Focus.TargetRole = "implementation"
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{TargetRefs: []string{"implementation.go"}}
	decision, err := runtimecontract.SealRuntimeDecision(aggregate.Facts, runtimecontract.RuntimeDecision{
		State: aggregate.Decision.State, Action: aggregate.Decision.Action, Focus: aggregate.Facts.Focus, RequiredOutput: aggregate.Decision.RequiredOutput,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: "operation_admissibility:file-refresh", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit},
	}
	module := newTestBiz(orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil), hub, nil)
	resolved, err := module.resolveRuntimeV2Prerequisite(ctx, aggregate, t.TempDir(), prerequisite)
	if err != nil {
		t.Fatal(err)
	}
	if len(captured.RequiredTargetRefs) != 1 || captured.RequiredTargetRefs[0] != "implementation.go" ||
		resolved.OperationAdmissibility == nil || len(resolved.OperationAdmissibility.TargetRefs) != 1 || resolved.OperationAdmissibility.TargetRefs[0] != "implementation.go" {
		t.Fatalf("operation admissibility refresh lost persisted grounded failure target: command=%+v result=%+v", captured, resolved)
	}
}

func TestRuntimeV2MainlineRequestsAndSealsTypedCandidate(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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

	var requestedDefinition runtimecontract.RuntimeDefinitionContext
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		requestedDefinition = command.DefinitionContext
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Candidate: runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "write-1", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
				Parameters: runtimeV2FileEditParameters("target.txt"),
			}}}}},
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: "/tmp/workspace"}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	fileObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.ExecuteFileEditOperationCommand)
		receipt := orchestratorevents.NewRuntimeOperationReceipt(command.Execution, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusAccepted, command.TargetRefs, runtimecontract.RuntimeOperationEvidence{}, nil, nil, "2026-07-20T00:00:00Z")
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
		}, nil)
	})
	t.Cleanup(func() { fileObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation) })

	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(biz, hub, nil)
	sealed, err := module.requestAndSealRuntimeV2Candidate(ctx, initialized, "stable-prefix-v2", 0)
	if err != nil {
		t.Fatalf("requestAndSealRuntimeV2Candidate: %v", err)
	}
	if sealed.ActiveDispatch == nil || sealed.ActiveDispatch.Transaction != initialized.Decision.AuthorityTransaction() {
		t.Fatalf("sealed aggregate = %+v", sealed)
	}
	if !runtimecontract.SameRuntimeDefinitionContext(requestedDefinition, runtimeV2Definition()) {
		t.Fatalf("candidate request definition = %+v", requestedDefinition)
	}
	completed, err := module.executeRuntimeV2ActiveDispatch(ctx, sealed)
	if err != nil {
		t.Fatalf("executeRuntimeV2ActiveDispatch: %v", err)
	}
	if completed.ActiveDispatch != nil || completed.Decision.Action != runtimecontract.RuntimeActionComplete {
		t.Fatalf("completed aggregate = %+v", completed)
	}
	loaded, err := biz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.ActiveDispatch != nil || loaded.Decision.Action != runtimecontract.RuntimeActionComplete {
		t.Fatalf("persisted completed aggregate = %+v err=%v", loaded, err)
	}
}

func TestRuntimeV2OperationExecutorRoutesTypedCommandToCapabilityOwner(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: "/tmp/workspace", InternalDirectory: "/tmp/workspace-internal",
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	var captured orchestratorevents.ExecuteFileEditOperationCommand
	fileObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	fileObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation, func(event magicevent.Event, result magicevent.Result) {
		captured = event.Data().(orchestratorevents.ExecuteFileEditOperationCommand)
		result.Set(orchestratorevents.ExecuteFileEditOperationResult{
			EventID: "event-write-1", OperationID: "write-1", EvidenceKind: runtimecontract.EvidenceKindStateChange,
			Status: runtimecontract.RuntimeOperationExecutionStatusAccepted, SourceRef: "workorch://runtime-operations/dispatch-1/write-1",
		}, nil)
	})
	t.Cleanup(func() { fileObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation) })

	response, err := (eventRuntimeV2OperationExecutor{hub: hub}).ExecuteRuntimeOperation(ctx, runtimev2.RuntimeOperationExecutionRequest{
		WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: "dispatch-1",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-1"},
		Operation: runtimecontract.RuntimeOperation{
			ID: "write-1", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
		},
		EvidenceKind:     runtimecontract.EvidenceKindStateChange,
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{ID: "policy-1", AppliesTo: runtimecontract.StepKindFileEdit}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != runtimecontract.RuntimeOperationExecutionStatusAccepted || captured.WorkspaceDeliverableDirectory != "/tmp/workspace" || captured.OperationID != "write-1" || captured.Data.Path != "target.txt" ||
		len(captured.EvidencePolicies) != 1 || captured.EvidencePolicies[0].ID != "policy-1" {
		t.Fatalf("response=%+v command=%+v", response, captured)
	}
}

func TestRuntimeV2OperationExecutorPassesFocusTargetRoleToMCPOwner(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: "/tmp/workspace", InternalDirectory: "/tmp/workspace-internal",
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	var captured orchestratorevents.ExecuteMCPOperationCommand
	mcpObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleMCP, hub)
	mcpObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteMCPOperation, func(event magicevent.Event, result magicevent.Result) {
		captured = event.Data().(orchestratorevents.ExecuteMCPOperationCommand)
		result.Set(orchestratorevents.ExecuteMCPOperationResult{
			EventID: "event-mcp-1", OperationID: "mcp-1", EvidenceKind: runtimecontract.EvidenceKindValidation,
			Status: runtimecontract.RuntimeOperationExecutionStatusAccepted, SourceRef: "workorch://runtime-operations/dispatch-1/mcp-1",
		}, nil)
	})
	t.Cleanup(func() { mcpObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteMCPOperation) })

	serverID, tool := "server-a", "lookup.references"
	response, err := (eventRuntimeV2OperationExecutor{hub: hub}).ExecuteRuntimeOperation(ctx, runtimev2.RuntimeOperationExecutionRequest{
		WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: "dispatch-1",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-1"}, TargetRole: "implementation",
		Operation: runtimecontract.RuntimeOperation{
			ID: "mcp-1", Kind: runtimecontract.StepKindMCPToolCall, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			Parameters: []runtimecontract.OperationParameter{{Name: "serverId", String: &serverID}, {Name: "tool", String: &tool}},
		}, EvidenceKind: runtimecontract.EvidenceKindValidation,
	})
	if err != nil || response.Status != runtimecontract.RuntimeOperationExecutionStatusAccepted || captured.TargetRole != "implementation" ||
		captured.Data.ServerID != "server-a" || captured.Data.Tool != "lookup.references" {
		t.Fatalf("response=%+v command=%+v err=%v", response, captured, err)
	}
}

func TestRuntimeV2MainlineRetriesSameCandidateContractOnce(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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

	var attempts []orchestratorevents.RequestRuntimeCandidateCommand
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		attempts = append(attempts, command)
		if command.CandidateAttempt == 0 {
			failure := runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassSchema, Reason: "invalid schema"}
			result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Failure: &failure}, nil)
			return
		}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Candidate: runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "write-1", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
				TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
			}}}}},
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(biz, hub, nil)
	sealed, err := module.requestAndSealRuntimeV2CandidateWithRetry(ctx, initialized, "stable-prefix-v2")
	if err != nil {
		t.Fatal(err)
	}
	if sealed.ActiveDispatch == nil || len(attempts) != 2 || attempts[0].CandidateAttempt != 0 || attempts[1].CandidateAttempt != 1 ||
		attempts[0].Transaction != attempts[1].Transaction || attempts[0].StablePrefixHash != attempts[1].StablePrefixHash {
		t.Fatalf("attempts=%+v sealed=%+v", attempts, sealed)
	}
	if len(sealed.CandidateAttempts) != 2 || sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateFailed || sealed.CandidateAttempts[1].State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
		t.Fatalf("persisted attempts = %+v", sealed.CandidateAttempts)
	}
}

func TestRuntimeV2MainlineRecordsInfeasibleCandidateBeforeDispatchSeal(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir()}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	probeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	probeObserver.Subscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.ProbeRuntimeOperationFeasibilityCommand)
		failure := &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_target_not_regular", Signature: "operation-admissibility/file/edit/operation_target_not_regular", Message: "target is a directory", TargetRefs: []string{"."}}
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: command.Operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Failure: failure}, nil)
	})
	t.Cleanup(func() {
		probeObserver.Unsubscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	})

	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		candidate := runtimecontract.RuntimeCandidate{
			StateChange: &runtimecontract.StateChangeCandidate{
				Operations: []runtimecontract.StateChangingOperation{{
					Operation: runtimecontract.RuntimeOperation{
						ID: "directory-edit", Kind: runtimecontract.StepKindFileEdit,
						CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"."},
						Parameters: runtimeV2FileEditParameters("."),
					},
				}},
			},
		}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: candidate}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	facts := runtimeV2Aggregate(t, 1).Facts
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"."}, ObservedRevision: "test-revision",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{
		runtimeV2OperationAdmissibilityPrerequisite(inventory),
	}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{
		runtimeV2FileAdmissibilityObservation(inventory),
	}
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(biz, hub, nil)
	failed, err := module.requestAndSealRuntimeV2Candidate(ctx, initialized, "stable-admissibility-prefix", 0)
	if err != nil {
		t.Fatal(err)
	}
	if failed.ActiveDispatch != nil || len(failed.CandidateAttempts) != 1 || failed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateFailed || failed.CandidateAttempts[0].Failure == nil || failed.CandidateAttempts[0].Failure.Class != runtimecontract.RuntimeCandidateFailureClassInfeasible || failed.CandidateAttempts[0].Failure.Code != "operation_target_not_regular" {
		t.Fatalf("infeasible candidate was sealed or not persisted as an admission failure: %+v", failed)
	}
	if len(failed.OperationReceipts) != 0 || len(failed.Facts.Evidence.Accepted) != 0 || len(failed.Facts.Evidence.Rejected) != 0 {
		t.Fatalf("infeasible candidate created execution state: %+v", failed)
	}
}

func TestRuntimeV2CandidateAdmissionSkipsOwnerProbeForAssessment(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(1)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := newTestBiz(NewRuntimeOrchestrator(nil, nil, nil, nil), hub, nil)
	candidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionRepairableGap, Summary: "assessment-only"}}
	failure, stale, err := module.probeRuntimeV2CandidateFeasibility(ctx, runtimev2.RuntimeAggregate{}, t.TempDir(), candidate, 0)
	if err != nil || failure != nil || stale != nil {
		t.Fatalf("assessment candidate should not enter owner operation probe: failure=%+v stale=%+v err=%v", failure, stale, err)
	}
}

func TestRuntimeV2MainlineRetriesCorrectedInfeasibleCandidateBeforeSeal(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir()}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	probes := 0
	probeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	probeObserver.Subscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, func(event magicevent.Event, result magicevent.Result) {
		probes++
		command := event.Data().(orchestratorevents.ProbeRuntimeOperationFeasibilityCommand)
		response := orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: command.Operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: probes != 1}
		if !response.Feasible {
			response.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_target_not_regular", Signature: "operation-admissibility/file/edit/operation_target_not_regular", Message: "target is a directory", TargetRefs: []string{"."}}
		}
		result.Set(response, nil)
	})
	t.Cleanup(func() {
		probeObserver.Unsubscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	})

	var requests []orchestratorevents.RequestRuntimeCandidateCommand
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		requests = append(requests, command)
		target := "target.txt"
		if command.CandidateAttempt == 0 {
			target = "."
		} else if command.PreviousFailure == nil || command.PreviousFailure.Class != runtimecontract.RuntimeCandidateFailureClassInfeasible || command.PreviousFailure.Code != "operation_target_not_regular" {
			t.Fatalf("retry did not receive persisted infeasibility: %+v", command.PreviousFailure)
		}
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "edit-" + target, Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target}, Parameters: runtimeV2FileEditParameters(target),
		}}}}}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: candidate}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	facts := runtimeV2Aggregate(t, 1).Facts
	inventory := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{".", "target.txt"}, ObservedRevision: "test-revision"}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{
		runtimeV2OperationAdmissibilityPrerequisite(inventory),
	}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{
		runtimeV2FileAdmissibilityObservation(inventory),
	}
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeV2CandidateWithRetry(ctx, initialized, "stable-admissibility-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if sealed.ActiveDispatch == nil || probes != 2 || len(requests) != 2 || len(sealed.CandidateAttempts) != 2 ||
		sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateFailed || sealed.CandidateAttempts[1].State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
		t.Fatalf("corrected candidate was not sealed after one infeasible retry: probes=%d requests=%+v aggregate=%+v", probes, requests, sealed)
	}
	if len(sealed.OperationReceipts) != 0 || len(sealed.Facts.Evidence.Accepted) != 0 || len(sealed.Facts.Evidence.Rejected) != 0 {
		t.Fatalf("candidate admission created execution state: %+v", sealed)
	}
}

func TestRuntimeV2MainlineExhaustsTwoInfeasibleCandidatesWithoutDispatch(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir()}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	probeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	probeObserver.Subscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.ProbeRuntimeOperationFeasibilityCommand)
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: command.Operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_target_not_regular", Signature: "operation-admissibility/file/edit/operation_target_not_regular", Message: "target is a directory", TargetRefs: []string{"."}}}, nil)
	})
	t.Cleanup(func() {
		probeObserver.Unsubscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	})

	requests := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requests++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		target := "."
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "directory-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target}, Parameters: runtimeV2FileEditParameters(target),
		}}}}}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: candidate}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	facts := runtimeV2Aggregate(t, 1).Facts
	inventory := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"."}, ObservedRevision: "test-revision"}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeV2CandidateWithRetry(ctx, initialized, "stable-admissibility-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || terminal.ActiveDispatch != nil || terminal.Decision.Action != runtimecontract.RuntimeActionFail || terminal.Facts.Failure == nil || terminal.Facts.Failure.Class != runtimecontract.FailureClassCandidateExhausted || terminal.Facts.Failure.Code != "candidate_retry_exhausted" {
		t.Fatalf("two infeasible candidates did not terminalize as bounded exhaustion: requests=%d aggregate=%+v", requests, terminal)
	}
	if len(terminal.OperationReceipts) != 0 || len(terminal.Facts.Evidence.Accepted) != 0 || len(terminal.Facts.Evidence.Rejected) != 0 {
		t.Fatalf("infeasible candidates created execution state: %+v", terminal)
	}
}

func TestRuntimeV2MainlineRestartsFromPersistedCandidateProposalWithoutProviderReplay(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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

	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) {
		providerCalls++
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	_, err := biz.InitializeRuntimeV2(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "persisted-proposal-prefix"
	pending, err := biz.BeginRuntimeV2CandidateAttempt(ctx, "ws-v2", "run-v2", 0, prefix)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "persisted-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
	}}}}}
	persisted, err := biz.RecordRuntimeV2CandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.StateRevision <= pending.StateRevision || len(persisted.CandidateAttempts) != 1 || persisted.CandidateAttempts[0].Proposal == nil {
		t.Fatalf("candidate proposal was not persisted: pending=%+v persisted=%+v", pending, persisted)
	}
	restarted, err := biz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(biz, hub, nil)
	sealed, err := module.requestAndSealRuntimeV2Candidate(ctx, restarted, prefix, 0)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || sealed.ActiveDispatch == nil || sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || sealed.CandidateAttempts[0].Proposal != nil {
		t.Fatalf("restart replayed provider or did not seal persisted proposal: calls=%d aggregate=%+v", providerCalls, sealed)
	}
}

func TestRuntimeV2MainlinePartialReplayClosesAttemptThenSealsCorrectedRetry(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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

	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	repository := runtimeV2StateRepository{hub: hub}
	if err := biz.ConfigureRuntimeV2Repository(repository); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	replayedOperation := runtimecontract.RuntimeOperation{
		ID: "replayed-edit", Kind: runtimecontract.StepKindFileEdit,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
		Parameters: runtimeV2FileEditParameters("target.txt"),
	}
	rejectedCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: replayedOperation}}}}
	previousDispatch, err := runtimev2.SealDispatchWithActivation(initialized.Decision, rejectedCandidate, initialized.Facts.Activation)
	if err != nil {
		t.Fatal(err)
	}
	previousDispatch.State = runtimev2.DispatchStateCommitted
	seeded := initialized
	seeded.CompletedDispatches = []runtimev2.SealedDispatch{previousDispatch}
	seeded.Facts.Evidence.Rejected = []runtimecontract.EvidenceFact{{
		ID: "rejected-edit", Kind: runtimecontract.EvidenceKindStateChange, DispatchID: previousDispatch.ID, OperationID: replayedOperation.ID,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Revision: "evidence-v2-00000000000000000001",
		FailureClass: runtimecontract.FailureClassCandidateContractFailure, FailureCode: "base_mismatch", FailureSignature: "rejected-edit",
	}}
	seeded.Facts.EvidenceRevision = "evidence-v2-00000000000000000001"
	// The current authority must be sealed against the updated evidence lineage;
	// the rejected dispatch itself remains history, never an active candidate.
	seeded.Facts, seeded.Decision, err = runtimev2.ReduceRuntimeStateWithDirective(seeded.Facts, &initialized.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	seeded.StateRevision++
	if err := repository.Save(ctx, seeded, initialized.StateRevision); err != nil {
		t.Fatal(err)
	}

	var requests []orchestratorevents.RequestRuntimeCandidateCommand
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		requests = append(requests, command)
		candidate := runtimecontract.RuntimeCandidate{}
		if command.CandidateAttempt == 0 {
			newOperation := replayedOperation
			newOperation.ID = "new-edit"
			newText := "corrected"
			newOperation.Parameters = runtimeV2FileEditParameters("target.txt")
			newOperation.Parameters[2].String = &newText
			candidate.StateChange = &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: replayedOperation}, {Operation: newOperation}}}
		} else {
			if command.PreviousFailure == nil || command.PreviousFailure.Code != "candidate_partially_replays_rejected_shape" {
				t.Errorf("retry missing typed partial replay failure: %+v", command.PreviousFailure)
			}
			corrected := replayedOperation
			corrected.ID = "corrected-edit"
			newText := "corrected"
			corrected.Parameters = runtimeV2FileEditParameters("target.txt")
			corrected.Parameters[2].String = &newText
			candidate.StateChange = &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: corrected}}}
		}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: candidate}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	module := newTestBiz(biz, hub, nil)
	first, err := module.requestAndSealRuntimeV2Candidate(ctx, seeded, "partial-replay-prefix", 0)
	if err != nil || len(requests) != 1 || len(first.CandidateAttempts) != 1 || first.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateFailed || first.CandidateAttempts[0].Failure == nil || first.CandidateAttempts[0].Failure.Code != "candidate_partially_replays_rejected_shape" || first.Decision.Action == runtimecontract.RuntimeActionFail {
		t.Fatalf("partial replay did not atomically close attempt 0 before restart: aggregate=%+v err=%v requests=%+v", first, err, requests)
	}
	restarted, err := biz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := module.requestAndSealRuntimeV2CandidateWithRetry(ctx, restarted, "partial-replay-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || sealed.ActiveDispatch == nil || len(sealed.CandidateAttempts) != 2 ||
		sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateFailed || sealed.CandidateAttempts[0].Failure == nil ||
		sealed.CandidateAttempts[0].Failure.Code != "candidate_partially_replays_rejected_shape" ||
		sealed.CandidateAttempts[1].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || sealed.Decision.Action == runtimecontract.RuntimeActionFail {
		t.Fatalf("partial replay did not converge through typed retry: requests=%+v aggregate=%+v", requests, sealed)
	}
	if requests[0].StablePrefixHash != requests[1].StablePrefixHash || requests[0].Transaction != requests[1].Transaction {
		t.Fatalf("partial replay retry drifted the sealed request contract: %+v", requests)
	}
}

func TestRuntimeV2MainlineRestartSkipsPersistedFeasibilityProbe(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir()}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	providerCalls, probeCalls := 0, 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providerCalls++ })
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	probeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	probeObserver.Subscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, func(_ magicevent.Event, _ magicevent.Result) { probeCalls++ })
	t.Cleanup(func() {
		probeObserver.Unsubscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	})

	facts := runtimeV2Aggregate(t, 1).Facts
	inventory := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt"}, ObservedRevision: "test-revision"}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	if _, err := biz.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), nil); err != nil {
		t.Fatal(err)
	}
	const prefix = "persisted-feasibility-prefix"
	if _, err := biz.BeginRuntimeV2CandidateAttempt(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "persisted-feasible-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
	}}}}}
	if _, err := biz.RecordRuntimeV2CandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	if _, err := biz.RecordRuntimeV2CandidateFeasibility(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFeasibilityRecord{OperationID: "persisted-feasible-edit", AdmissibilitySnapshotID: inventory.SnapshotID, Feasible: true}); err != nil {
		t.Fatal(err)
	}
	restarted, err := biz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeV2Candidate(ctx, restarted, prefix, 0)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || probeCalls != 0 || sealed.ActiveDispatch == nil || sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || len(sealed.CandidateAttempts[0].Feasibilities) != 0 {
		t.Fatalf("restart replayed provider/probe or retained unsealed feasibility state: provider=%d probes=%d aggregate=%+v", providerCalls, probeCalls, sealed)
	}
}

func runtimeV2FileEditParameters(path string) []runtimecontract.OperationParameter {
	oldText := "old"
	newText := "new"
	return []runtimecontract.OperationParameter{
		{Name: "path", String: &path},
		{Name: "oldText", String: &oldText},
		{Name: "newText", String: &newText},
	}
}

func TestRuntimeV2MainlineStopsAfterSecondCandidateFailure(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
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
	requests := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requests++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		failure := runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassProvider, Reason: "provider unavailable"}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Failure: &failure}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeV2CandidateWithRetry(ctx, initialized, "stable-prefix-v2")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || terminal.Decision.Action != runtimecontract.RuntimeActionFail || terminal.ActiveDispatch != nil {
		t.Fatalf("requests=%d terminal=%+v", requests, terminal)
	}
}

func TestNextRuntimeV2CandidateAttemptResumesPersistedPendingOrFailedState(t *testing.T) {
	transaction := runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-1", FocusID: "focus-v2", Action: runtimecontract.RuntimeActionProduceStateChange, RequiredOutputHash: "output-v2", DefinitionRevision: "definition-v2", EvidenceRevision: "evidence-v2"}
	base := runtimev2.RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{WorkspaceID: "ws-v2", RunID: "run-v2"}, Decision: runtimecontract.RuntimeDecision{AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "snapshot-1", FocusID: "focus-v2", Action: runtimecontract.RuntimeActionProduceStateChange, RequiredOutputHash: "output-v2", DefinitionRevision: "definition-v2", EvidenceRevision: "evidence-v2"}}}
	contractID := runtimecontract.StableAuthorityContractID(base.Facts.WorkspaceID, base.Facts.RunID, transaction)
	tests := []struct {
		name     string
		attempts []runtimecontract.RuntimeCandidateAttemptRecord
		want     int
		ok       bool
	}{
		{name: "new", want: 0, ok: true},
		{name: "pending zero", attempts: []runtimecontract.RuntimeCandidateAttemptRecord{{AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStatePending}}, want: 0, ok: true},
		{name: "failed zero", attempts: []runtimecontract.RuntimeCandidateAttemptRecord{{AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStateFailed}}, want: 1, ok: true},
		{name: "pending one", attempts: []runtimecontract.RuntimeCandidateAttemptRecord{{AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStateFailed}, {AuthorityContractID: contractID, Transaction: transaction, Attempt: 1, State: runtimecontract.RuntimeCandidateAttemptStatePending}}, want: 1, ok: true},
		{name: "accepted", attempts: []runtimecontract.RuntimeCandidateAttemptRecord{{AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStateAccepted}}, ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			aggregate := base
			aggregate.CandidateAttempts = tc.attempts
			got, ok := nextRuntimeV2CandidateAttempt(aggregate)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("next attempt = %d/%t, want %d/%t", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestNextRuntimeV2CandidateAttemptRejectsResealRunaway(t *testing.T) {
	aggregate := runtimeV2Aggregate(t, 100)
	transaction := aggregate.Decision.AuthorityTransaction()
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction)
	for idx := 0; idx < 3; idx++ {
		resealed := transaction
		resealed.AuthoritySnapshotID = fmt.Sprintf("resealed-snapshot-%d", idx)
		resealed.Attempt = idx
		aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{
			AuthorityContractID: contractID, Transaction: resealed, Attempt: 0, StablePrefixHash: "stable-prefix-v2",
			State:   runtimecontract.RuntimeCandidateAttemptStateFailed,
			Failure: &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassContract, Reason: "invalid candidate"},
		})
	}
	if attempt, ok := nextRuntimeV2CandidateAttempt(aggregate); ok {
		t.Fatalf("reseal runaway offered candidate attempt %d", attempt)
	}
	if !runtimeV2CandidateContractExhausted(aggregate) {
		t.Fatal("reseal runaway was not recognized as exhausted")
	}
}

func runtimeV2Aggregate(t *testing.T, revision uint64) runtimev2.RuntimeAggregate {
	t.Helper()
	policy := runtimeV2TestAcceptanceSlicePolicy()
	facts := runtimecontract.RuntimeFacts{
		Activation: runtimecontract.RuntimeActivationContext{
			ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{
				OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
				AcceptanceSlicePolicy:     &policy,
			},
		},
		WorkspaceID: "ws-v2", RunID: "run-v2", DefinitionRevision: "definition-v2", EvidenceRevision: "evidence-v2-00000000000000000000",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-v2", SliceID: "slice-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{"target.txt"},
		},
		Coverage:          []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-v2", SliceID: "slice-v2", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities:      runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{runtimecontract.StepKindFileEdit}},
		ConvergencePolicy: runtimeV2TestConvergencePolicy(),
		RecoveryPolicy:    runtimecontract.DefaultRuntimeRecoveryPolicy(),
	}
	decision, err := runtimev2.ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	return runtimev2.RuntimeAggregate{StateRevision: revision, Definition: runtimeV2Definition(), Facts: facts, Decision: decision}
}

func runtimeV2FileAdmissibilityObservation(inventory runtimecontract.RuntimeOperationAdmissibility) runtimecontract.RuntimePrerequisiteObservation {
	inventory = inventory.Normalized()
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: inventory.PrerequisiteID,
		Kind:           runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID:  inventory.OwnerModuleID,
		ContentHash:    inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain:        runtimecontract.RuntimeResourceWorkspaceTree,
			OwnerModuleID: inventory.OwnerModuleID,
			SubjectRef:    "workspace_deliverable",
			Revision:      inventory.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func runtimeV2OperationAdmissibilityPrerequisite(inventory runtimecontract.RuntimeOperationAdmissibility) runtimecontract.RuntimePrerequisite {
	inventory = inventory.Normalized()
	return runtimecontract.RuntimePrerequisite{
		ID:                   inventory.PrerequisiteID,
		Kind:                 runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID:        inventory.OwnerModuleID,
		RequiredCapabilities: append([]string(nil), inventory.OperationKinds...),
	}
}

func runtimeV2Definition() runtimecontract.RuntimeDefinitionContext {
	return runtimecontract.RuntimeDefinitionContext{
		Revision: "definition-v2", GoalID: "goal-v2", GoalDefinition: "完成目标",
		DoDID: "dod-v2", DoDDefinition: "产生可验证交付物", CapabilityContract: []string{runtimecontract.StepKindFileEdit},
	}
}

func runtimeV2Bootstrap() runtimecontract.RuntimeBootstrap {
	policy := runtimeV2TestAcceptanceSlicePolicy()
	return runtimecontract.RuntimeBootstrap{
		SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
		Definition:    runtimeV2Definition(),
		Activation: runtimecontract.RuntimeActivationContext{
			ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{
				OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
				AcceptanceSlicePolicy:     &policy,
			},
		},
		Focuses: []runtimecontract.CoverageFocus{{
			ID: "focus-v2", SliceID: "slice-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{"target.txt"},
		}},
		Slices:        []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-v2", Title: "Test", Scope: "test", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed}},
		CoverageItems: []runtimecontract.RuntimeCoverageItem{{ID: "cwi-v2", SliceID: "slice-v2"}},
		Requirements: []runtimecontract.RuntimeRequirement{{
			Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit},
		}},
		ConvergencePolicy: runtimeV2TestConvergencePolicy(),
		RecoveryPolicy:    runtimecontract.DefaultRuntimeRecoveryPolicy(),
	}
}

func runtimeV2TestAcceptanceSlicePolicy() runtimecontract.RuntimeAcceptanceSlicePolicy {
	return runtimecontract.RuntimeAcceptanceSlicePolicy{
		SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2,
		MaxWorkItems:  4, MaxFocuses: 16, MaxCandidateContextItems: 4,
		Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{ID: "test", Title: "Test", Scope: "test", Priority: 1, MaxWorkItems: 4, Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true}}},
	}
}

func runtimeV2TestConvergencePolicy() runtimecontract.RuntimeConvergencePolicy {
	policy := runtimecontract.DefaultRuntimeConvergencePolicy()
	policy.MaxAcceptanceSliceWorkItems = 4
	policy.MaxFocuses = 16
	policy.MaxCandidateContextItems = 4
	return policy
}

func runtimeV2RunForTest(state common.RunState, definition runtimecontract.RuntimeDefinitionContext) *runmodel.Run {
	bootstrap := runtimeV2Bootstrap()
	bootstrap.Definition = definition
	return &runmodel.Run{
		ID:                 "run-v2",
		WorkspaceID:        "ws-v2",
		GoalID:             definition.GoalID,
		DoDID:              definition.DoDID,
		DefinitionRevision: definition.Revision,
		RuntimeBootstrap:   &bootstrap,
		State:              state,
	}
}
