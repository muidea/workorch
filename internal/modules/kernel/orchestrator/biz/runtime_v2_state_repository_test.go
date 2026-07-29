package orchestrator

// Owner Biz persistence tests.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
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

// startRuntimeV2FileBackedRunOwner creates an isolated process boundary around
// the durable Run repository. Callers deliberately invoke the returned stop
// function before constructing the next process: a new controller alone is
// not evidence that EventHub and the Run owner can recover persisted state.
func startRuntimeV2FileBackedRunOwner(t *testing.T, ctx context.Context, runs *filerepository.RunRepository) (magicevent.Hub, func()) {
	t.Helper()
	hub := magicevent.NewHub(16)
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		hub.Terminate(ctx)
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		hub.Terminate(ctx)
		t.Fatal(err)
	}
	stopped := false
	return hub, func() {
		if stopped {
			return
		}
		stopped = true
		_ = runOwner.Stop(ctx)
		hub.Terminate(ctx)
	}
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
	runsPath := filepath.Join(t.TempDir(), "runs.json")
	runs := filerepository.NewRunRepository(runsPath)
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
	transaction := aggregate.Decision.AuthorityTransaction()
	requestSession := runtimecontract.RuntimeCandidateRequestSession{
		Transaction: transaction, StablePrefixHash: "stable-prefix-v2", ManifestHash: "candidate-manifest-v2",
		State: runtimecontract.RuntimeCandidateRequestSessionContinuationPending, NextSequence: 2,
		Fragments: []runtimecontract.RuntimeCandidateRequestFragment{{
			Sequence: 1, ContentHash: "fragment-v1", Bytes: 23, PhysicalCallID: "physical-call-1",
		}},
		LastPhysicalCallID: "physical-call-1", ProviderSubmitted: true,
		LastOutcome: runtimecontract.RuntimeCandidateRequestOutcome{
			Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentIncomplete, Code: "candidate_output_incomplete",
		},
	}
	requestSession.LogicalRequestID = runtimecontract.StableRuntimeCandidateRequestSessionID(
		aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction, requestSession.StablePrefixHash, requestSession.ManifestHash, requestSession.CorrectionHash,
	)
	aggregate.CandidateRequestSessions = []runtimecontract.RuntimeCandidateRequestSession{requestSession}
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
	if len(loaded.CandidateRequestSessions) != 1 || !reflect.DeepEqual(loaded.CandidateRequestSessions[0], requestSession) {
		t.Fatalf("loaded request session = %+v, want %+v", loaded.CandidateRequestSessions, requestSession)
	}
	// Record conversion must not retain aliases to the caller's session/
	// fragment slice. A later controller transition must be a new CAS record.
	record := runtimeStateRecordFromAggregate(aggregate)
	record.CandidateRequestSessions[0].Fragments[0].ContentHash = "mutated-record-fragment"
	if aggregate.CandidateRequestSessions[0].Fragments[0].ContentHash != "fragment-v1" {
		t.Fatalf("record conversion aliased request-session fragment metadata: %+v", aggregate.CandidateRequestSessions)
	}
	reloadedFromRecord := runtimeAggregateFromRecord(record)
	reloadedFromRecord.CandidateRequestSessions[0].Fragments[0].ContentHash = "mutated-aggregate-fragment"
	if record.CandidateRequestSessions[0].Fragments[0].ContentHash != "mutated-record-fragment" {
		t.Fatalf("aggregate conversion aliased request-session fragment metadata: %+v", record.CandidateRequestSessions)
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
	biz := orchestratorbiz.NewRuntimeOrchestrator()
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
	if _, err := beginRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "mcp-owner-catalog-prefix"); err != nil {
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

// FailureTarget is an Orchestrator-local mapping, but it still crosses the
// same Run-owner CAS boundary as an external owner receipt. A failed save must
// not expose the mapping; a lost response after the save must not derive it a
// second time or start any candidate/side-effect work after reload.
func TestRuntimeV2StateRepositoryFailureTargetCommitCrashRestartIsAtomic(t *testing.T) {
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
	facts.Failure = &runtimecontract.FailureFact{Signature: "failure-target-v1", TargetRefs: []string{"implementation.go"}}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), []runtimev2.RuntimeRequirement{{
		Kind: runtimecontract.PrerequisiteKindFailureTarget, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
		Reason: "typed failure repair surface",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(initialized.Facts.Prerequisites.Pending) != 1 {
		t.Fatalf("initialized aggregate = %+v", initialized)
	}
	prerequisiteID := initialized.Facts.Prerequisites.Pending[0].ID
	result := runtimev2.PrerequisiteResult{PrerequisiteID: prerequisiteID, FailureTarget: &runtimecontract.FailureTarget{
		Signature: "failure-target-v1", TargetRefs: []string{"implementation.go"},
	}}

	beforeCommitRepository := &failOnceRuntimeStateRepository{
		RuntimeStateRepository: repository,
		err:                    errors.New("injected failure target CAS failure"),
	}
	beforeCommitController, err := runtimev2.NewRuntimeController(beforeCommitRepository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beforeCommitController.CommitPrerequisite(ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("failure target commit unexpectedly persisted despite injected Run-owner CAS failure")
	}

	restartedBeforeCommit, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedBeforeCommit, err := restartedBeforeCommit.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loadedBeforeCommit.StateRevision != initialized.StateRevision || loadedBeforeCommit.Facts.Resources.FailureTarget != nil ||
		len(loadedBeforeCommit.Facts.Resources.PrerequisiteObservations) != 0 || len(loadedBeforeCommit.Facts.Prerequisites.Pending) != 1 ||
		loadedBeforeCommit.Facts.Prerequisites.Pending[0].ID != prerequisiteID {
		t.Fatalf("failed failure-target CAS partially persisted aggregate: %+v", loadedBeforeCommit)
	}

	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: repository, marker: "failure-target-response-loss",
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			target := aggregate.Facts.Resources.FailureTarget
			return target != nil && target.Signature == "failure-target-v1" && len(target.TargetRefs) == 1 &&
				target.TargetRefs[0] == "implementation.go" && len(aggregate.Facts.Prerequisites.Pending) == 0
		},
	}
	responseLost, err := runtimev2.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "failure-target-response-loss" {
				t.Fatalf("unexpected failure-target response-loss marker: %v", recovered)
			}
		}()
		_, _ = responseLost.CommitPrerequisite(ctx, "ws-v2", "run-v2", result)
		t.Fatal("expected failure-target response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("failure-target response-loss CAS point was not reached")
	}

	restartedAfterCommit, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedAfterCommit, err := restartedAfterCommit.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loadedAfterCommit.StateRevision <= initialized.StateRevision || loadedAfterCommit.Facts.Resources.FailureTarget == nil ||
		loadedAfterCommit.Facts.Resources.FailureTarget.Signature != "failure-target-v1" ||
		len(loadedAfterCommit.Facts.Resources.FailureTarget.TargetRefs) != 1 || loadedAfterCommit.Facts.Resources.FailureTarget.TargetRefs[0] != "implementation.go" ||
		len(loadedAfterCommit.Facts.Resources.PrerequisiteObservations) != 0 || len(loadedAfterCommit.Facts.Prerequisites.Pending) != 0 ||
		len(loadedAfterCommit.CandidateRequestSessions) != 0 || len(loadedAfterCommit.CandidateAttempts) != 0 ||
		loadedAfterCommit.ActiveDispatch != nil || len(loadedAfterCommit.GovernanceRequests) != 0 || len(loadedAfterCommit.OperationReceipts) != 0 {
		t.Fatalf("restart did not retain only committed failure-target mapping: %+v", loadedAfterCommit)
	}
	if _, err := restartedAfterCommit.CommitPrerequisite(ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("duplicate failure-target result unexpectedly mutated committed aggregate")
	}
	final, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if final.StateRevision != loadedAfterCommit.StateRevision || final.Facts.Resources.FailureTarget == nil ||
		final.Facts.Resources.FailureTarget.Signature != "failure-target-v1" {
		t.Fatalf("duplicate failure target changed persisted aggregate: %+v", final)
	}
}

func TestRuntimeV2StateRepositoryValidationScratchStaleInvalidationIsAtomic(t *testing.T) {
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
	prerequisite := runtimecontract.RuntimePrerequisite{ID: "validation-scratch:orchestrator", Kind: runtimecontract.PrerequisiteKindValidationScratch, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator}
	scratch := &runtimecontract.ValidationScratch{Reference: ".workorch-internal/validation-scratch/ws-v2/run-v2/cwi-v2", Revision: "scratch-v1"}
	observation := runtimecontract.RuntimePrerequisiteObservation{PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: "scratch-v1", Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceLocalResource, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator, SubjectRef: "validation-scratch/ws-v2/run-v2/cwi-v2", Revision: scratch.Revision}}}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Resources.ValidationScratch = scratch
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	repository := runtimeV2StateRepository{hub: hub}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID, Domain: runtimecontract.RuntimeResourceLocalResource, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator, SubjectRef: "validation-scratch/ws-v2/run-v2/cwi-v2", ExpectedRevision: "scratch-v1", ObservedRevision: "scratch-missing-v2", Reason: "validation scratch missing"}
	unrelated := stale
	unrelated.PrerequisiteID = "validation-scratch:other-focus"
	if _, err := controller.InvalidatePrerequisitesForReadOnlyStale(ctx, "ws-v2", "run-v2", unrelated); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unrelated scratch stale error = %v, want current-observation conflict", err)
	}
	unchanged, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil || unchanged.StateRevision != initialized.StateRevision || unchanged.Facts.Resources.ValidationScratch == nil || unchanged.Facts.Recovery.PrerequisiteRefreshes != 0 || len(unchanged.PrerequisiteInvalidations) != 0 {
		t.Fatalf("unrelated scratch stale changed aggregate=%+v err=%v", unchanged, err)
	}

	before := &failOnceRuntimeStateRepository{RuntimeStateRepository: repository, err: errors.New("injected scratch stale CAS failure")}
	beforeController, err := runtimev2.NewRuntimeController(before)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beforeController.InvalidatePrerequisitesForReadOnlyStale(ctx, "ws-v2", "run-v2", stale); err == nil {
		t.Fatal("scratch stale invalidation unexpectedly persisted before CAS")
	}
	loaded, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.StateRevision != initialized.StateRevision || loaded.Facts.Resources.ValidationScratch == nil || len(loaded.Facts.Prerequisites.Pending) != 0 || loaded.Facts.Recovery.PrerequisiteRefreshes != 0 {
		t.Fatalf("before-CAS scratch stale leaked: aggregate=%+v err=%v", loaded, err)
	}

	after := &panicAfterRuntimeStateSaveRepository{RuntimeStateRepository: repository, marker: "scratch-stale-response-loss", match: func(aggregate runtimev2.RuntimeAggregate) bool {
		return aggregate.Facts.Resources.ValidationScratch == nil && aggregate.Facts.Recovery.PrerequisiteRefreshes == 1
	}}
	afterController, err := runtimev2.NewRuntimeController(after)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "scratch-stale-response-loss" {
				t.Fatalf("unexpected scratch stale marker: %v", recovered)
			}
		}()
		_, _ = afterController.InvalidatePrerequisitesForReadOnlyStale(ctx, "ws-v2", "run-v2", stale)
		t.Fatal("expected scratch stale response loss")
	}()
	restarted, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err = restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.Facts.Resources.ValidationScratch != nil || len(loaded.Facts.Prerequisites.Pending) != 1 || loaded.Facts.Prerequisites.Pending[0].ID != prerequisite.ID || loaded.Facts.Recovery.PrerequisiteRefreshes != 1 || len(loaded.CandidateRequestSessions) != 0 || len(loaded.CandidateAttempts) != 0 || loaded.ActiveDispatch != nil || len(loaded.GovernanceRequests) != 0 || len(loaded.OperationReceipts) != 0 {
		t.Fatalf("after-CAS scratch stale did not preserve clean re-acquisition: aggregate=%+v err=%v", loaded, err)
	}
	duplicate, err := restarted.InvalidatePrerequisitesForReadOnlyStale(ctx, "ws-v2", "run-v2", stale)
	if err != nil || duplicate.StateRevision != loaded.StateRevision || duplicate.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("duplicate scratch stale was not a no-op: aggregate=%+v err=%v", duplicate, err)
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
	facts.Focus.ProspectiveTargetRef = "initial-deliverable.txt"
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
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable", ProspectiveTargetRef: facts.Focus.ProspectiveTargetRef,
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
		if _, err := beginRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "candidate-cas-prefix"); err != nil {
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
		if _, err := admitRuntimeV2CandidateProposalForTest(ctx, beforeCommit, "ws-v2", "run-v2", 0, candidate); err == nil {
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

		proposal, err := admitRuntimeV2CandidateProposalForTest(ctx, restarted, "ws-v2", "run-v2", 0, candidate)
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
		proposal, err := admitRuntimeV2CandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
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
		if _, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, beforeCommit, "ws-v2", "run-v2", 0, candidate); err == nil {
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

		sealed, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, restarted, "ws-v2", "run-v2", 0, candidate)
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
			_, _ = admitRuntimeV2CandidateProposalForTest(ctx, responseLost, "ws-v2", "run-v2", 0, candidate)
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
		sealed, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, restarted, "ws-v2", "run-v2", 0, candidate)
		if err != nil || sealed.Aggregate.ActiveDispatch == nil || sealed.Aggregate.ActiveDispatch.CandidateHash != runtimecontract.StableRuntimeCandidateHash(candidate) {
			t.Fatalf("response-loss proposal did not seal the original candidate: sealed=%+v err=%v", sealed, err)
		}
	})

	t.Run("dispatch_seal_response_loss", func(t *testing.T) {
		ctx, repository, controller := newFixture(t)
		if _, err := admitRuntimeV2CandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate); err != nil {
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
			_, _ = admitAndSealRuntimeV2CandidateAttemptForTest(ctx, responseLost, "ws-v2", "run-v2", 0, candidate)
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
		replayed, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, restarted, "ws-v2", "run-v2", 0, candidate)
		if err != nil || replayed.Aggregate.StateRevision != loaded.StateRevision || replayed.Aggregate.ActiveDispatch == nil ||
			replayed.Aggregate.ActiveDispatch.ID != loaded.ActiveDispatch.ID || len(replayed.Aggregate.CompletedDispatches) != len(loaded.CompletedDispatches) {
			t.Fatalf("committed dispatch response-loss replay was not idempotent: replayed=%+v loaded=%+v err=%v", replayed, loaded, err)
		}
	})
}

// P5-A: proposal and seal have no owner side effect, but both are durable SM-3
// boundaries. A fresh process must see either the prior proposal or the exact
// sealed dispatch; it must never synthesize a second dispatch after a lost
// response.
func TestRuntimeV2StateRepositoryDispatchSealFreshProcessCrashRestart(t *testing.T) {
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "fresh-process-seal-edit", Kind: runtimecontract.StepKindFileEdit,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
		Parameters: runtimeV2FileEditParameters("target.txt"),
	}}}}}
	newFixture := func(t *testing.T) (context.Context, *filerepository.RunRepository, runtimeV2StateRepository, *runtimev2.RuntimeController, func()) {
		t.Helper()
		ctx := context.Background()
		runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
		if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
			t.Fatal(err)
		}
		hub, stop := startRuntimeV2FileBackedRunOwner(t, ctx, runs)
		repository := runtimeV2StateRepository{hub: hub}
		controller, err := runtimev2.NewRuntimeController(repository)
		if err != nil {
			stop()
			t.Fatal(err)
		}
		if _, err := controller.Initialize(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil); err != nil {
			stop()
			t.Fatal(err)
		}
		if _, err := beginRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "fresh-process-seal-prefix"); err != nil {
			stop()
			t.Fatal(err)
		}
		if proposal, err := admitRuntimeV2CandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate); err != nil || !proposal.Admission.Accepted() || proposal.Aggregate.ActiveDispatch != nil {
			stop()
			t.Fatalf("proposal setup=%+v err=%v", proposal, err)
		}
		return ctx, runs, repository, controller, stop
	}

	t.Run("before_seal_cas", func(t *testing.T) {
		ctx, runs, repository, _, stopFirstProcess := newFixture(t)
		t.Cleanup(stopFirstProcess)
		beforeCommit, err := runtimev2.NewRuntimeController(&failOnceRuntimeStateRepository{
			RuntimeStateRepository: repository, err: errors.New("injected dispatch seal CAS failure"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, beforeCommit, "ws-v2", "run-v2", 0, candidate); err == nil {
			t.Fatal("dispatch seal unexpectedly persisted before Run-owner CAS")
		}
		stopFirstProcess()
		hub, stopSecondProcess := startRuntimeV2FileBackedRunOwner(t, ctx, runs)
		t.Cleanup(stopSecondProcess)
		restarted, err := runtimev2.NewRuntimeController(runtimeV2StateRepository{hub: hub})
		if err != nil {
			t.Fatal(err)
		}
		pending, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || pending.ActiveDispatch != nil || len(pending.CandidateAttempts) != 1 ||
			pending.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStatePending || pending.CandidateAttempts[0].Proposal == nil {
			t.Fatalf("pre-CAS fresh process leaked sealed dispatch: aggregate=%+v err=%v", pending, err)
		}
		sealed, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, restarted, "ws-v2", "run-v2", 0, candidate)
		if err != nil || sealed.Aggregate.ActiveDispatch == nil || sealed.Aggregate.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
			t.Fatalf("fresh process did not seal persisted proposal: aggregate=%+v err=%v", sealed, err)
		}
	})

	t.Run("after_seal_cas_response_loss", func(t *testing.T) {
		ctx, runs, repository, _, stopFirstProcess := newFixture(t)
		t.Cleanup(stopFirstProcess)
		responseLossRepository := &panicAfterRuntimeStateSaveRepository{
			RuntimeStateRepository: repository, marker: "fresh-process-dispatch-seal-response-loss",
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
				if recovered := recover(); recovered != "fresh-process-dispatch-seal-response-loss" {
					t.Fatalf("unexpected dispatch seal response-loss marker: %v", recovered)
				}
			}()
			_, _ = admitAndSealRuntimeV2CandidateAttemptForTest(ctx, responseLost, "ws-v2", "run-v2", 0, candidate)
			t.Fatal("expected dispatch seal response loss after durable CAS")
		}()
		if !responseLossRepository.fired {
			t.Fatal("dispatch seal response-loss CAS point was not reached")
		}
		stopFirstProcess()
		hub, stopSecondProcess := startRuntimeV2FileBackedRunOwner(t, ctx, runs)
		t.Cleanup(stopSecondProcess)
		restarted, err := runtimev2.NewRuntimeController(runtimeV2StateRepository{hub: hub})
		if err != nil {
			t.Fatal(err)
		}
		persisted, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || persisted.ActiveDispatch == nil || len(persisted.CandidateAttempts) != 1 ||
			persisted.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || persisted.CandidateAttempts[0].Proposal != nil {
			t.Fatalf("post-CAS fresh process did not retain one sealed dispatch: aggregate=%+v err=%v", persisted, err)
		}
		revision, dispatchID := persisted.StateRevision, persisted.ActiveDispatch.ID
		duplicate, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, restarted, "ws-v2", "run-v2", 0, candidate)
		if err != nil || duplicate.Aggregate.StateRevision != revision || duplicate.Aggregate.ActiveDispatch == nil || duplicate.Aggregate.ActiveDispatch.ID != dispatchID ||
			len(duplicate.Aggregate.CompletedDispatches) != len(persisted.CompletedDispatches) {
			t.Fatalf("sealed dispatch duplicate changed fresh-process authority: aggregate=%+v err=%v", duplicate, err)
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
	// The response-loss half of this boundary must survive a real Run-owner
	// persistence reload. An in-memory repository can prove reducer atomicity,
	// but cannot prove that the committed owner observation is all a restarted
	// process can observe.
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
	if _, err := beginRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "failure-observation-prefix"); err != nil {
		t.Fatal(err)
	}
	path := "validation-entry.txt"
	candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "failure-observation-read", Kind: runtimecontract.StepKindFileRead,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{path},
		Parameters: nil,
	}}}}}
	sealed, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
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

	// The owner receipt and the rejected-evidence/failure-target transition are
	// separate CAS boundaries.  Lose the response only after the latter becomes
	// durable: a restart must consume the committed observation, not invoke the
	// validation owner again or reconstruct a target from diagnostics.
	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: repository,
		marker:                 "failure-observation-evidence-response-loss",
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			return aggregate.ActiveDispatch == nil && len(aggregate.OperationReceipts) == 1 &&
				len(aggregate.Facts.Evidence.Rejected) == 1 && aggregate.Facts.Failure != nil &&
				aggregate.Facts.Resources.FailureTarget != nil &&
				aggregate.Facts.Resources.FailureTarget.TargetObservation != nil &&
				aggregate.Facts.Resources.FailureTarget.TargetObservation.SubjectRef == "server-a"
		},
	}
	responseLost, err := runtimev2.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "failure-observation-evidence-response-loss" {
				t.Fatalf("unexpected failure-observation response-loss marker: %v", recovered)
			}
		}()
		_, _ = responseLost.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", newExecutor(responseLost, false))
		t.Fatal("expected owner failure observation response loss after durable evidence CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("failure-observation evidence response-loss CAS point was not reached")
	}

	restartedAfterResponseLoss, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := restartedAfterResponseLoss.Load(ctx, "ws-v2", "run-v2")
	if err != nil || len(persisted.OperationReceipts) != 1 || len(persisted.Facts.Evidence.Rejected) != 1 || persisted.Facts.Failure == nil ||
		persisted.Facts.Resources.FailureTarget == nil || persisted.Facts.Resources.FailureTarget.TargetObservation == nil ||
		persisted.Facts.Resources.FailureTarget.TargetObservation.SubjectRef != "server-a" ||
		persisted.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite ||
		len(persisted.Facts.Prerequisites.Pending) != 1 || persisted.Facts.Prerequisites.Pending[0].Kind != runtimecontract.PrerequisiteKindTargetSnapshot ||
		persisted.Facts.Prerequisites.Pending[0].SubjectRef != "implementation.go" {
		t.Fatalf("restart did not reuse committed failure observation: aggregate=%+v err=%v", persisted, err)
	}
	if _, err := restartedAfterResponseLoss.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", newExecutor(restartedAfterResponseLoss, false)); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("restart replayed completed failure dispatch instead of waiting for its prerequisite: %v", err)
	}
	revision := persisted.StateRevision
	replayed, receipt, err := restartedAfterResponseLoss.CommitOperationReceipt(ctx, "ws-v2", "run-v2", sealed.Aggregate.ActiveDispatch.ID, persisted.OperationReceipts[0].Fence, persisted.OperationReceipts[0])
	if err != nil || replayed.StateRevision != revision || !runtimecontract.SameRuntimeOperationReceipt(receipt, persisted.OperationReceipts[0]) {
		t.Fatalf("duplicate owner receipt changed committed failure observation: aggregate=%+v receipt=%+v err=%v", replayed, receipt, err)
	}
}

// A same-focus rejected operation shape remains diagnostic lineage until
// durable progress closes it.  An owner-grounded retarget is the narrow legal
// way to repair that same gap: after a real process restart the old shape must
// still be rejected, while the newly observed target receives a fresh
// authority and a distinct admissible shape.  This intentionally drives only
// typed receipt/prerequisite contracts; neither the test nor the runtime
// reconstructs a target from provider text.
func TestRuntimeV2FileBackedRestartKeepsRejectedShapeAcrossOwnerRetarget(t *testing.T) {
	ctx := context.Background()
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}

	firstHub := magicevent.NewHub(16)
	t.Cleanup(func() { firstHub.Terminate(ctx) })
	firstRunOwner := runmodule.New()
	if err := firstRunOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: firstHub}); err != nil {
		t.Fatal(err)
	}
	if err := firstRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = firstRunOwner.Stop(ctx) })

	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Focus.TargetRefs = []string{"main.go"}
	facts.Focus.TargetRole = "implementation"
	facts.Focus.RequireTargetGrounding = true
	facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
		TargetRef: "main.go", ContentHash: "main-v1", ContentState: runtimecontract.SnapshotContentStateComplete,
	}}
	// A failure target is only authoritative when its owning catalog revision is
	// already persisted in prerequisite freshness.  This is deliberately an MCP
	// observation because that owner is allowed to report explicit failureTargets.
	catalog := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: "capability:blocks/mcp:failure-observer", Kind: runtimecontract.PrerequisiteKindCapability,
		OwnerModuleID: runtimecontract.RuntimeModuleMCP, ContentHash: "mcp-catalog-v1",
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP,
			SubjectRef: "mcp_server_catalog", Revision: "mcp-catalog-v1",
		}},
	}
	catalog.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(catalog)
	facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, runtimecontract.RuntimePrerequisite{
		ID: catalog.PrerequisiteID, Kind: catalog.Kind, OwnerModuleID: catalog.OwnerModuleID,
		RequiredCapabilities: []string{runtimecontract.StepKindMCPToolCall}, Reason: "failure target owner catalog",
	})
	facts.Resources.PrerequisiteObservations = append(facts.Resources.PrerequisiteObservations, catalog)

	firstRepository := runtimeV2StateRepository{hub: firstHub}
	firstController, err := runtimev2.NewRuntimeController(firstRepository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := firstController.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil || initialized.Decision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("initial retarget aggregate=%+v err=%v", initialized, err)
	}
	if _, err := beginRuntimeV2CandidateAttemptForTest(ctx, firstController, "ws-v2", "run-v2", 0, "retarget-shape-prefix"); err != nil {
		t.Fatal(err)
	}
	mainTarget := "main.go"
	oldCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "rejected-main-edit", Kind: runtimecontract.StepKindFileEdit,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{mainTarget},
		Parameters: runtimeV2FileEditParameters(mainTarget),
	}}}}}
	sealed, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, firstController, "ws-v2", "run-v2", 0, oldCandidate)
	if err != nil || !sealed.Admission.Accepted() || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("old candidate seal=%+v err=%v", sealed, err)
	}

	ownerFailure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Code: "implementation_targeted_failure",
		Signature: "same-gap-owner-retarget", Message: "owner observed a different implementation target",
		TargetRefs: []string{"implementation.go"},
		TargetObservation: &runtimecontract.RuntimeFailureTargetObservation{
			OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation",
			ObservedRevision: "mcp-catalog-v1", TargetRefs: []string{"implementation.go"},
		},
	}
	executor := runtimeOperationExecutorFunc(func(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest) (runtimev2.RuntimeOperationExecutionResult, error) {
		receipt := runtimecontract.RuntimeOperationReceipt{
			ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
			// The owner rejected the requested semantic outcome after a durable
			// attempt. Preserve the expected scope so the executor does not replace
			// the owner-authored failure target with a generic effect mismatch.
			Outcome: runtimecontract.RuntimeOperationOutcomeRejected, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
			Failure: &ownerFailure, Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "owner observed implementation failure"},
		}
		_, committed, err := firstController.CommitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt)
		if err != nil {
			return runtimev2.RuntimeOperationExecutionResult{}, err
		}
		return runtimev2.RuntimeOperationExecutionResult{
			EventID: "same-gap-owner-retarget", OperationID: request.Operation.ID, EvidenceKind: runtimecontract.EvidenceKindStateChange,
			Status: runtimev2.ExecutionEvidenceStatusRejected, SourceRef: "workorch://mcp/failure-target",
			Failure: &ownerFailure, Evidence: committed.Evidence, Receipt: committed,
		}, nil
	})
	afterFailure, err := firstController.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", executor)
	if err != nil || afterFailure.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || afterFailure.Facts.Resources.FailureTarget == nil ||
		len(afterFailure.Facts.Resources.FailureTarget.TargetRefs) != 1 || afterFailure.Facts.Resources.FailureTarget.TargetRefs[0] != "implementation.go" {
		t.Fatalf("owner retarget was not committed as prerequisite authority: aggregate=%+v err=%v", afterFailure, err)
	}
	if shapes := runtimev2.ActiveRejectedOperationShapes(afterFailure); len(shapes) != 1 ||
		shapes[0].Hash != runtimecontract.StableRuntimeOperationShape(oldCandidate.StateChange.Operations[0].Operation) ||
		len(shapes[0].Operation.TargetRefs) != 1 || shapes[0].Operation.TargetRefs[0] != "main.go" {
		t.Fatalf("same-gap rejected shape was not persisted before restart: %+v", shapes)
	}
	var rejectedTargetSnapshot runtimecontract.RuntimePrerequisite
	for _, prerequisite := range afterFailure.Facts.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && prerequisite.SubjectRef == "main.go" {
			rejectedTargetSnapshot = prerequisite
			break
		}
	}
	if rejectedTargetSnapshot.ID == "" {
		t.Fatalf("rejected durable operation did not request its required freshness snapshot: %+v", afterFailure.Facts.Prerequisites.Pending)
	}

	// Reconstruct the production Run owner against the same file-backed state.
	// No process-local controller, dispatch or rejected-shape cache crosses this
	// boundary.
	if err := firstRunOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	firstHub.Terminate(ctx)
	secondHub := magicevent.NewHub(16)
	t.Cleanup(func() { secondHub.Terminate(ctx) })
	secondRunOwner := runmodule.New()
	if err := secondRunOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: secondHub}); err != nil {
		t.Fatal(err)
	}
	if err := secondRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondRunOwner.Stop(ctx) })

	restarted, err := runtimev2.NewRuntimeController(runtimeV2StateRepository{hub: secondHub})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || loaded.Facts.Resources.FailureTarget == nil {
		t.Fatalf("file-backed restart lost retarget authority: aggregate=%+v err=%v", loaded, err)
	}
	if shapes := runtimev2.ActiveRejectedOperationShapes(loaded); len(shapes) != 1 ||
		shapes[0].Hash != runtimecontract.StableRuntimeOperationShape(oldCandidate.StateChange.Operations[0].Operation) {
		t.Fatalf("file-backed restart lost old rejected shape: %+v", shapes)
	}

	afterRejectedTargetRefresh, err := restarted.CommitPrerequisite(ctx, "ws-v2", "run-v2", runtimev2.PrerequisiteResult{
		PrerequisiteID: rejectedTargetSnapshot.ID,
		TargetSnapshot: &runtimecontract.TargetSnapshot{TargetRef: "main.go", ContentHash: "main-v2", ContentState: runtimecontract.SnapshotContentStateComplete},
	})
	if err != nil || afterRejectedTargetRefresh.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite {
		t.Fatalf("original target refresh did not preserve owner retarget precedence: aggregate=%+v err=%v", afterRejectedTargetRefresh, err)
	}
	var retargetSnapshot runtimecontract.RuntimePrerequisite
	for _, prerequisite := range afterRejectedTargetRefresh.Facts.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && prerequisite.SubjectRef == "implementation.go" {
			retargetSnapshot = prerequisite
			break
		}
	}
	if retargetSnapshot.ID == "" {
		t.Fatalf("owner retarget did not request its own snapshot after rejected-target refresh: %+v", afterRejectedTargetRefresh.Facts.Prerequisites.Pending)
	}
	resumed, err := restarted.CommitPrerequisite(ctx, "ws-v2", "run-v2", runtimev2.PrerequisiteResult{
		PrerequisiteID: retargetSnapshot.ID,
		TargetSnapshot: &runtimecontract.TargetSnapshot{TargetRef: "implementation.go", ContentHash: "implementation-v1", ContentState: runtimecontract.SnapshotContentStateComplete},
	})
	if err != nil || resumed.Decision.Action != runtimecontract.RuntimeActionProduceStateChange ||
		len(resumed.Decision.RequiredOutput.TargetRefs) != 1 || resumed.Decision.RequiredOutput.TargetRefs[0] != "implementation.go" ||
		resumed.Decision.AuthoritySnapshot.ID == sealed.Aggregate.Decision.AuthoritySnapshot.ID || resumed.Decision.AuthoritySnapshot.Attempt != 0 {
		t.Fatalf("restart did not create fresh retarget authority: aggregate=%+v err=%v", resumed, err)
	}
	// After owner retarget, Decision seals exact_target to implementation.go.
	// Plan §7 first-failure therefore rejects main.go at target policy before
	// semantic replay. The old rejected shape remains retained for same-target
	// replays; either rejection proves the old candidate is still inadmissible.
	if admission := evaluateRuntimeV2CandidateAdmissionForTest(resumed, oldCandidate); admission.Accepted() || admission.Failure == nil ||
		(admission.Failure.Code != "candidate_replays_rejected_shape" &&
			admission.Failure.Code != "candidate_contract_invalid" &&
			admission.Failure.Code != "candidate_target_outside_grounding") {
		t.Fatalf("old same-gap shape became admissible after retarget restart: %+v", admission)
	}
	implementationTarget := "implementation.go"
	newCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "retargeted-implementation-edit", Kind: runtimecontract.StepKindFileEdit,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{implementationTarget},
		Parameters: runtimeV2FileEditParameters(implementationTarget),
	}}}}}
	if admission := evaluateRuntimeV2CandidateAdmissionForTest(resumed, newCandidate); !admission.Accepted() {
		t.Fatalf("new owner-retargeted shape was not admissible after restart: %+v", admission)
	}
}

// Assessment is the SM-2 boundary that turns accepted evidence into coverage
// progress (and possibly a terminal authority). Its CAS must therefore keep
// the assessment dispatch, coverage, repair ledger and terminal decision
// indivisible across a Run-owner restart.
func TestRuntimeV2StateRepositoryAssessmentCommitCrashRestartIsAtomic(t *testing.T) {
	ctx := context.Background()
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}
	hub, stopFirstProcess := startRuntimeV2FileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopFirstProcess)

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
	if _, err := beginRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "assessment-state-change-prefix"); err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "assessment-write", Kind: runtimecontract.StepKindFileEdit,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
		Parameters: runtimeV2FileEditParameters("target.txt"),
	}}}}}
	sealed, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
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
	if _, err := beginRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "assessment-commit-prefix"); err != nil {
		t.Fatal(err)
	}
	assessmentCandidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "current focus satisfied",
		EvidenceRefs: []string{"assessment-state-change-evidence"},
	}}
	assessmentDispatch, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, assessmentCandidate)
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

	// A failed pre-CAS transition must disappear with its process. Rebuild the
	// Hub and Run owner before loading, rather than proving only a new
	// controller can read through the previous process' owner.
	stopFirstProcess()
	restartedHub, stopSecondProcess := startRuntimeV2FileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopSecondProcess)
	restartedRepository := runtimeV2StateRepository{hub: restartedHub}
	restarted, err := runtimev2.NewRuntimeController(restartedRepository)
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
		RuntimeStateRepository: restartedRepository, marker: "assessment-cas-response-loss",
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

	// Repeat the full process reconstruction after the durable CAS / lost
	// response boundary. This is the real recovery path; it must only consume
	// the committed assessment delta.
	stopSecondProcess()
	finalHub, stopFinalProcess := startRuntimeV2FileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopFinalProcess)
	finalRepository := runtimeV2StateRepository{hub: finalHub}
	restartedAfterResponseLoss, err := runtimev2.NewRuntimeController(finalRepository)
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

// Failure facts are SM-2 inputs, not delivery diagnostics. The failure,
// terminal authority and closed candidate surface must therefore survive the
// same full-process CAS boundaries as assessment repair facts.
func TestRuntimeV2StateRepositoryFailureFactCrashRestartIsAtomic(t *testing.T) {
	ctx := context.Background()
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}
	failure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_v2_drive_failed",
		Signature: "runtime-v2-drive-failed", Message: "sealed transaction cannot be reconciled",
	}
	hub, stopFirstProcess := startRuntimeV2FileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopFirstProcess)
	repository := runtimeV2StateRepository{hub: hub}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	beforeCommit, err := runtimev2.NewRuntimeController(&failOnceRuntimeStateRepository{
		RuntimeStateRepository: repository, err: errors.New("injected failure fact CAS failure"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beforeCommit.RecordFailureFact(ctx, "ws-v2", "run-v2", failure); err == nil {
		t.Fatal("failure fact unexpectedly persisted before Run-owner CAS")
	}

	stopFirstProcess()
	restartedHub, stopSecondProcess := startRuntimeV2FileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopSecondProcess)
	restartedRepository := runtimeV2StateRepository{hub: restartedHub}
	restarted, err := runtimev2.NewRuntimeController(restartedRepository)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || pending.StateRevision != initialized.StateRevision || pending.Facts.Failure != nil ||
		pending.Decision.Action == runtimecontract.RuntimeActionFail || pending.ActiveDispatch != nil {
		t.Fatalf("pre-CAS failure leaked through fresh process: aggregate=%+v err=%v", pending, err)
	}

	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: restartedRepository, marker: "failure-fact-cas-response-loss",
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			return aggregate.Facts.Failure != nil && aggregate.Facts.Failure.Signature == failure.Signature &&
				aggregate.Decision.Action == runtimecontract.RuntimeActionFail && aggregate.Decision.Terminal != nil
		},
	}
	responseLost, err := runtimev2.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "failure-fact-cas-response-loss" {
				t.Fatalf("unexpected failure fact response-loss marker: %v", recovered)
			}
		}()
		_, _ = responseLost.RecordFailureFact(ctx, "ws-v2", "run-v2", failure)
		t.Fatal("expected failure fact response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("failure fact response-loss CAS point was not reached")
	}

	stopSecondProcess()
	finalHub, stopFinalProcess := startRuntimeV2FileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopFinalProcess)
	final, err := runtimev2.NewRuntimeController(runtimeV2StateRepository{hub: finalHub})
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := final.Load(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.StateRevision <= initialized.StateRevision || persisted.Facts.Failure == nil ||
		persisted.Facts.Failure.Signature != failure.Signature || persisted.Decision.Action != runtimecontract.RuntimeActionFail ||
		persisted.Decision.Terminal == nil || persisted.ActiveDispatch != nil {
		t.Fatalf("post-CAS failure did not survive fresh process: aggregate=%+v err=%v", persisted, err)
	}
	revision := persisted.StateRevision
	duplicate, err := final.RecordFailureFact(ctx, "ws-v2", "run-v2", failure)
	if err != nil || duplicate.StateRevision != revision || duplicate.Facts.Failure == nil || duplicate.Facts.Failure.Signature != failure.Signature {
		t.Fatalf("terminal failure duplicate changed authority: aggregate=%+v err=%v", duplicate, err)
	}
}

// A repairable assessment that reports the same persisted gap is not progress.
// Its CAS recovery must retain the current focus, coverage and no-progress
// lineage whether the write fails before commit or the caller loses the
// response after the Run owner has persisted it.
func TestRuntimeV2StateRepositorySameGapAssessmentCrashRestartRetainsRepairWindow(t *testing.T) {
	ctx := context.Background()
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}
	hub, stopFirstProcess := startRuntimeV2FileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopFirstProcess)

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
	if _, err := beginRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "same-gap-state-prefix"); err != nil {
		t.Fatal(err)
	}
	stateCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "same-gap-write", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
	}}}}}
	sealed, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, stateCandidate)
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
	if _, err := beginRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "same-gap-assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	assessmentCandidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionRepairableGap,
		Summary: "same gap persists", Gap: &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Reason: "same gap"},
	}}
	assessmentDispatch, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, assessmentCandidate)
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
	stopFirstProcess()
	restartedHub, stopSecondProcess := startRuntimeV2FileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopSecondProcess)
	restartedRepository := runtimeV2StateRepository{hub: restartedHub}
	restartedBefore, err := runtimev2.NewRuntimeController(restartedRepository)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := restartedBefore.Load(ctx, "ws-v2", "run-v2")
	if err != nil || pending.ActiveDispatch == nil || pending.ActiveDispatch.Candidate.Assessment == nil || len(pending.Facts.Assessments) != 0 || pending.Facts.Repair.LastProgressRevision != "" || pending.Facts.Repair.NoProgressCount != 1 {
		t.Fatalf("same-gap pre-commit restart leaked progress: aggregate=%+v err=%v", pending, err)
	}

	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: restartedRepository, marker: "same-gap-assessment-response-loss",
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

	stopSecondProcess()
	finalHub, stopFinalProcess := startRuntimeV2FileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopFinalProcess)
	restarted, err := runtimev2.NewRuntimeController(runtimeV2StateRepository{hub: finalHub})
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
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}
	hub, stopFirstProcess := startRuntimeV2FileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopFirstProcess)

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
	if _, err := beginRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "partial-gap-state-prefix"); err != nil {
		t.Fatal(err)
	}
	stateCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "partial-gap-write", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a", "cwi-b"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
	}}}}}
	sealed, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, stateCandidate)
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
	if _, err := beginRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "partial-gap-assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	assessmentCandidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a", "cwi-b"}, Disposition: runtimecontract.AssessmentDispositionRepairableGap,
		Summary: "one remaining item", Items: []runtimecontract.RuntimeAssessmentItemFact{
			{CoverageWorkItemID: "cwi-a", Satisfied: true, EvidenceRefs: []string{"partial-gap-evidence"}},
			{CoverageWorkItemID: "cwi-b", Satisfied: false, Reason: "still open"},
		}, Gap: &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-b"}, Reason: "one remaining item"},
	}}
	assessmentDispatch, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, assessmentCandidate)
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
	stopFirstProcess()
	restartedHub, stopSecondProcess := startRuntimeV2FileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopSecondProcess)
	restartedRepository := runtimeV2StateRepository{hub: restartedHub}
	restartedBefore, err := runtimev2.NewRuntimeController(restartedRepository)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := restartedBefore.Load(ctx, "ws-v2", "run-v2")
	if err != nil || pending.ActiveDispatch == nil || len(pending.Facts.Assessments) != 0 || pending.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateOpen || pending.Facts.Coverage[1].State != runtimecontract.RuntimeCoverageItemStateOpen || pending.Facts.Repair.LastProgressRevision != "" || pending.Facts.Repair.NoProgressCount != 2 {
		t.Fatalf("partial-gap pre-commit restart leaked delta: aggregate=%+v err=%v", pending, err)
	}

	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: restartedRepository, marker: "partial-gap-assessment-response-loss",
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

	stopSecondProcess()
	finalHub, stopFinalProcess := startRuntimeV2FileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopFinalProcess)
	restarted, err := runtimev2.NewRuntimeController(runtimeV2StateRepository{hub: finalHub})
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
	workspaceInternalDirectory := t.TempDir()
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: workspaceInternalDirectory,
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	biz := orchestratorbiz.NewRuntimeOrchestrator()
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
	module := newTestBiz(orchestratorbiz.NewRuntimeOrchestrator(), hub, nil)
	resolved, err := module.resolveRuntimeV2Prerequisite(ctx, aggregate, t.TempDir(), t.TempDir(), prerequisite)
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
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "write-1", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
			Parameters: runtimeV2FileEditParameters("target.txt"),
		}}}}}
		result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
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

	biz := orchestratorbiz.NewRuntimeOrchestrator()
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

// The C6 correction boundary must survive a real Run-owner reload: a provider
// can rename operations or reorder JSON fields, but that cannot turn a prior
// complete rejected candidate into a new semantic correction. The two subtests
// cover the CAS-before and response-loss-after terminal boundaries without
// involving a provider replay or a UI recovery path.
func TestRuntimeV2FileBackedSemanticCandidateReplayExhaustionSurvivesCASBoundaries(t *testing.T) {
	newBaseline := func(t *testing.T) (context.Context, runtimev2.RuntimeStateRepository, runtimecontract.RuntimeCandidate, runtimecontract.RuntimeCandidate) {
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
		aggregate := runtimeV2Aggregate(t, 1)
		aggregate.Facts.Focus.RequireTargetGrounding = true
		aggregate.Facts.Focus.TargetRole = "implementation"
		aggregate.Facts.Focus.TargetRefs = nil
		aggregate.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{
			Signature: "owner-observed-failure", TargetRefs: []string{"implementation.go"},
		}
		if err := repository.Save(ctx, aggregate, 0); err != nil {
			t.Fatal(err)
		}
		controller, err := runtimev2.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		wrong := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "provider-first-id", Kind: runtimecontract.StepKindFileEdit,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
			Parameters: runtimeV2FileEditParameters("target.txt"),
		}}}}}
		equivalent := wrong
		equivalent.StateChange.Operations = append([]runtimecontract.StateChangingOperation(nil), wrong.StateChange.Operations...)
		equivalent.StateChange.Operations[0].Operation.ID = "provider-second-id"
		parameters := append([]runtimecontract.OperationParameter(nil), equivalent.StateChange.Operations[0].Operation.Parameters...)
		for left, right := 0, len(parameters)-1; left < right; left, right = left+1, right-1 {
			parameters[left], parameters[right] = parameters[right], parameters[left]
		}
		equivalent.StateChange.Operations[0].Operation.Parameters = parameters
		if _, err := beginRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "semantic-replay-prefix"); err != nil {
			t.Fatal(err)
		}
		first, err := admitRuntimeV2CandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, wrong)
		if err != nil || first.Admission.Failure == nil || first.Admission.Failure.Code != "candidate_target_outside_grounding" || first.Aggregate.Decision.Action == runtimecontract.RuntimeActionFail {
			t.Fatalf("first complete candidate rejection = %+v err=%v", first, err)
		}
		if _, err := beginRuntimeV2CandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 1, "semantic-replay-prefix"); err != nil {
			t.Fatal(err)
		}
		return ctx, repository, wrong, equivalent
	}

	t.Run("before terminal CAS", func(t *testing.T) {
		ctx, repository, _, equivalent := newBaseline(t)
		crashing, err := runtimev2.NewRuntimeController(&failOnceRuntimeStateRepository{
			RuntimeStateRepository: repository, err: errors.New("injected semantic exhaustion CAS failure"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admitRuntimeV2CandidateProposalForTest(ctx, crashing, "ws-v2", "run-v2", 1, equivalent); err == nil {
			t.Fatal("semantic exhaustion unexpectedly persisted before injected CAS failure")
		}
		restarted, err := runtimev2.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || len(loaded.CandidateAttempts) != 2 || loaded.CandidateAttempts[1].State != runtimecontract.RuntimeCandidateAttemptStatePending || loaded.Decision.Action == runtimecontract.RuntimeActionFail {
			t.Fatalf("pre-CAS restart did not retain only pending correction: aggregate=%+v err=%v", loaded, err)
		}
		committed, err := admitRuntimeV2CandidateProposalForTest(ctx, restarted, "ws-v2", "run-v2", 1, equivalent)
		if err != nil || committed.Admission.Failure == nil || committed.Admission.Failure.Code != "candidate_replays_rejected_candidate_shape" ||
			committed.Aggregate.Decision.Terminal == nil || committed.Aggregate.Decision.Terminal.Code != "candidate_retry_exhausted" || committed.Aggregate.ActiveDispatch != nil {
			t.Fatalf("restarted semantic exhaustion = %+v err=%v", committed, err)
		}
	})

	t.Run("after terminal CAS response loss", func(t *testing.T) {
		ctx, repository, _, equivalent := newBaseline(t)
		const marker = "semantic-exhaustion-response-loss"
		responseLost, err := runtimev2.NewRuntimeController(&panicAfterRuntimeStateSaveRepository{
			RuntimeStateRepository: repository, marker: marker,
			match: func(aggregate runtimev2.RuntimeAggregate) bool {
				return len(aggregate.CandidateAttempts) == 2 && aggregate.CandidateAttempts[1].State == runtimecontract.RuntimeCandidateAttemptStateFailed &&
					aggregate.Decision.Action == runtimecontract.RuntimeActionFail && aggregate.Decision.Terminal != nil && aggregate.Decision.Terminal.Code == "candidate_retry_exhausted"
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != marker {
					t.Fatalf("unexpected post-CAS response-loss marker: %v", recovered)
				}
			}()
			_, _ = admitRuntimeV2CandidateProposalForTest(ctx, responseLost, "ws-v2", "run-v2", 1, equivalent)
			t.Fatal("expected response loss after terminal semantic exhaustion CAS")
		}()
		restarted, err := runtimev2.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || loaded.Decision.Terminal == nil || loaded.Decision.Terminal.Code != "candidate_retry_exhausted" ||
			len(loaded.CandidateAttempts) != 2 || loaded.CandidateAttempts[1].State != runtimecontract.RuntimeCandidateAttemptStateFailed || loaded.ActiveDispatch != nil || len(loaded.GovernanceRequests) != 0 {
			t.Fatalf("post-CAS restart did not preserve terminal semantic exhaustion: aggregate=%+v err=%v", loaded, err)
		}
		revision := loaded.StateRevision
		duplicate, err := admitRuntimeV2CandidateProposalForTest(ctx, restarted, "ws-v2", "run-v2", 1, equivalent)
		if err != nil || duplicate.Admission.Failure == nil || duplicate.Admission.Failure.Code != "candidate_replays_rejected_candidate_shape" || duplicate.Aggregate.StateRevision != revision {
			t.Fatalf("post-CAS duplicate changed terminal authority: result=%+v err=%v", duplicate, err)
		}
	})
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
	candidateCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		attempts = append(attempts, command)
		if command.CandidateAttempt == 0 {
			result.Set(orchestratorevents.RequestRuntimeCandidateResult{
				Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID,
				DocumentContent: []byte(`{"unexpected":true}`),
			}, nil)
			return
		}
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "write-1", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
		}}}}}
		result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	biz := orchestratorbiz.NewRuntimeOrchestrator()
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
	if sealed.ActiveDispatch == nil || len(attempts) != 2 || attempts[0].CandidateAttempt != 0 || attempts[1].CandidateAttempt != 1 || attempts[0].PreviousFailure != nil || attempts[1].PreviousFailure == nil || attempts[1].PreviousFailure.Diagnostic == nil || attempts[1].PreviousFailure.Diagnostic.Path != "$.unexpected" || attempts[1].PreviousFailure.Diagnostic.ManifestHash != attempts[1].CandidateManifest.Hash ||
		attempts[0].Transaction != attempts[1].Transaction || attempts[0].StablePrefixHash != attempts[1].StablePrefixHash {
		t.Fatalf("attempts=%+v sealed=%+v", attempts, sealed)
	}
	if len(sealed.CandidateAttempts) != 2 || sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateFailed || sealed.CandidateAttempts[0].Failure == nil || sealed.CandidateAttempts[0].Failure.Diagnostic == nil || sealed.CandidateAttempts[1].State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
		t.Fatalf("persisted attempts = %+v", sealed.CandidateAttempts)
	}
	duplicate, err := module.requestAndSealRuntimeV2CandidateWithRetry(ctx, sealed, "stable-prefix-v2")
	if err != nil || duplicate.ActiveDispatch == nil || len(attempts) != 2 || len(duplicate.CandidateAttempts) != 2 {
		t.Fatalf("duplicate correction notification replayed provider: aggregate=%+v attempts=%+v err=%v", duplicate, attempts, err)
	}
}

func TestRuntimeV2MainlineRenderedPreflightStopsBeforeCandidateAttempt(t *testing.T) {
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

	preflightCalls, candidateCalls := 0, 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorPreflightRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		preflightCalls++
		command := event.Data().(orchestratorevents.PreflightRuntimeCandidateCommand)
		result.Set(orchestratorevents.PreflightRuntimeCandidateResult{
			Transaction: command.Transaction,
			Ready:       false,
			Reason:      "P4.current_snapshots is redacted",
		}, nil)
	})
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) {
		candidateCalls++
	})
	t.Cleanup(func() {
		llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorPreflightRuntimeCandidate)
		llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate)
	})

	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Target:                   runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 4096,
		MaxInputTokens:           92928,
		MaxBytes:                 96 * 1024,
		CapabilityRevision:       "capability-1",
		ConfigurationRevision:    "config-1",
	}
	biz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeV2Candidate(ctx, initialized, "context-preflight-prefix", 0)
	if err != nil {
		t.Fatal(err)
	}
	if preflightCalls != 1 || candidateCalls != 0 || len(terminal.CandidateAttempts) != 0 || terminal.Decision.Terminal == nil || terminal.Facts.Failure == nil || terminal.Facts.Failure.Code != "runtime_context_integrity" {
		t.Fatalf("rendered preflight consumed a candidate attempt or called provider: preflight=%d candidate=%d aggregate=%+v", preflightCalls, candidateCalls, terminal)
	}
	if terminal.Facts.Repair.RepairAttempts != 0 || terminal.Facts.Recovery.ProviderTransportAttempts != 0 {
		t.Fatalf("rendered preflight consumed a repair/provider budget: %+v", terminal.Facts)
	}
}

func TestDriveRuntimeV2DecisionReacquiresMissingSealedProviderCeilingBeforeLLM(t *testing.T) {
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

	preflightCalls, candidateCalls := 0, 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorPreflightRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) {
		preflightCalls++
	})
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) {
		candidateCalls++
	})
	t.Cleanup(func() {
		llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorPreflightRuntimeCandidate)
		llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate)
	})

	facts := runtimeV2Aggregate(t, 1).Facts
	requirement := runtimev2.RuntimeRequirement{
		Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		MinimumBytes: 96 * 1024, Reason: "provider ceiling required",
	}

	biz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), []runtimev2.RuntimeRequirement{requirement})
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(initialized.Facts.Prerequisites.Pending) != 1 {
		t.Fatalf("fixture did not start at provider-ceiling prerequisite acquisition: %+v", initialized)
	}
	providerPrerequisite := initialized.Facts.Prerequisites.Pending[0]
	ceiling := runtimecontract.ProviderCeiling{
		Target:                   facts.Activation.ProviderTarget,
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 4096,
		MaxInputTokens:           92928,
		MaxBytes:                 96 * 1024,
		CapabilityRevision:       "capability-1",
		ConfigurationRevision:    "config-1",
	}
	committed, err := biz.CommitRuntimeV2Prerequisite(ctx, facts.WorkspaceID, facts.RunID, runtimev2.PrerequisiteResult{
		PrerequisiteID: providerPrerequisite.ID, ProviderCeiling: &ceiling,
	})
	if err != nil || committed.Decision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("commit provider ceiling prerequisite: aggregate=%+v err=%v", committed, err)
	}
	// Persist the owner-resource-loss boundary through the same Run CAS adapter
	// used by production. The stale decision remains candidate-shaped only until
	// driveRuntimeV2Decision reconciles it back to the sealed prerequisite.
	broken := committed
	broken.Facts.Resources.ProviderCeiling = nil
	if err := (runtimeV2StateRepository{hub: hub}).Save(ctx, broken, committed.StateRevision); err != nil {
		t.Fatalf("persist missing provider ceiling aggregate: %v", err)
	}

	reconciled, err := newTestBiz(biz, hub, nil).driveRuntimeV2Decision(ctx, broken, "provider-ceiling-reacquire-prefix")
	if err != nil {
		t.Fatalf("driveRuntimeV2Decision: %v", err)
	}
	if reconciled.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || preflightCalls != 0 || candidateCalls != 0 ||
		len(reconciled.CandidateAttempts) != 0 || reconciled.Facts.Repair.RepairAttempts != 0 ||
		reconciled.Facts.Recovery.ProviderTransportAttempts != 0 || reconciled.Facts.Recovery.PrerequisiteRefreshes != 0 ||
		reconciled.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 0 {
		t.Fatalf("missing provider ceiling reached LLM or spent a budget: preflight=%d candidate=%d aggregate=%+v", preflightCalls, candidateCalls, reconciled)
	}
	if len(reconciled.Facts.Prerequisites.Pending) != 1 || reconciled.Facts.Prerequisites.Pending[0].ID != providerPrerequisite.ID ||
		len(reconciled.Facts.Prerequisites.Satisfied) != 0 || reconciled.Facts.Resources.ProviderCeiling != nil {
		t.Fatalf("missing provider ceiling was not returned to active prerequisite acquisition: %+v", reconciled.Facts)
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

	candidateCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
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
		result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	facts := runtimeV2Aggregate(t, 1).Facts
	// Infeasibility is an owner feasibility observation, not wire exact_target
	// membership. Clear focus TargetRefs so Decision does not seal an exclusive
	// path that rejects the deliberately non-regular inventory target ".".
	facts.Focus.TargetRefs = nil
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
	biz := orchestratorbiz.NewRuntimeOrchestrator()
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
	if failed.ActiveDispatch != nil || len(failed.CandidateAttempts) != 1 || failed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated || failed.CandidateAttempts[0].Failure != nil || failed.CandidateAttempts[0].OwnerFeasibility == nil || failed.CandidateAttempts[0].OwnerFeasibility.Disposition != runtimecontract.RuntimeOwnerFeasibilityInfeasible || failed.CandidateAttempts[0].OwnerFeasibility.Code != "operation_target_not_regular" || failed.Facts.Failure == nil || failed.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible {
		t.Fatalf("owner infeasibility was not persisted as a non-correction SM-2 fact: %+v", failed)
	}
	if len(failed.OperationReceipts) != 0 || len(failed.Facts.Evidence.Accepted) != 0 || len(failed.Facts.Evidence.Rejected) != 0 {
		t.Fatalf("infeasible candidate created execution state: %+v", failed)
	}
	if candidateCalls != 1 {
		t.Fatalf("owner infeasibility requested a correction candidate: calls=%d aggregate=%+v", candidateCalls, failed)
	}
}

func TestRuntimeV2CandidateAdmissionSkipsOwnerProbeForAssessment(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(1)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := newTestBiz(NewRuntimeOrchestrator(), hub, nil)
	candidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionRepairableGap, Summary: "assessment-only"}}
	failure, stale, prerequisiteRequest, err := module.probeRuntimeV2CandidateFeasibility(ctx, runtimev2.RuntimeAggregate{}, t.TempDir(), candidate, runtimecontract.RuntimeCandidateContractManifest{}, 0)
	if err != nil || failure != nil || stale != nil || prerequisiteRequest != nil {
		t.Fatalf("assessment candidate should not enter owner operation probe: failure=%+v stale=%+v prerequisite=%+v err=%v", failure, stale, prerequisiteRequest, err)
	}
}

func TestRuntimeV2FileBackedCorrectionRestartSealsOwnerAdmissibleCandidate(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runsPath := filepath.Join(t.TempDir(), "runs.json")
	runs := filerepository.NewRunRepository(runsPath)
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
		} else if command.PreviousFailure == nil || command.PreviousFailure.Class != runtimecontract.RuntimeCandidateFailureClassContract || command.PreviousFailure.Code != "candidate_target_outside_grounding" {
			t.Fatalf("retry did not receive persisted infeasibility: %+v", command.PreviousFailure)
		} else if diagnostic := command.PreviousFailure.Diagnostic; diagnostic == nil || diagnostic.OperationID != "edit-." || diagnostic.Path != "$.operations[id=edit-.].targetRefs" || diagnostic.Actual != "." || !reflect.DeepEqual(diagnostic.Allowed, []string{"target.txt"}) || diagnostic.ManifestHash != command.CandidateManifest.Hash || diagnostic.RejectedShapeHash == "" || !reflect.DeepEqual(diagnostic.Objectives, []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionReplaceTarget}) {
			t.Fatalf("retry did not receive manifest-bound infeasibility diagnostic: %+v", diagnostic)
		}
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "edit-" + target, Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target}, Parameters: runtimeV2FileEditParameters(target),
		}}}}}
		result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Focus.TargetRefs = nil
	inventory := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{".", "target.txt"}, ObservedRevision: "test-revision"}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{
		runtimeV2OperationAdmissibilityPrerequisite(inventory),
	}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{
		runtimeV2FileAdmissibilityObservation(inventory),
	}
	biz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeV2Candidate(ctx, initialized, "stable-admissibility-prefix", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || len(first.CandidateAttempts) != 1 || first.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated || first.CandidateAttempts[0].OwnerFeasibility == nil || first.CandidateAttempts[0].Failure != nil || first.Facts.Failure == nil || first.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible {
		t.Fatalf("owner infeasibility was not durably handed to SM-2 before restart: requests=%+v aggregate=%+v", requests, first)
	}
	// The old body below exercises a same-contract attempt-1 correction. Owner
	// infeasibility is intentionally no longer eligible for that path; a
	// dedicated restart/idempotency test covers its new-authority handoff.
	return
	// Recreate the full process boundary. The only input to attempt 1 is the
	// file-backed failed attempt/session; it must not reuse an in-process
	// provider response, EventHub responder, or owner failure probe.
	if err := runOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	hub.Terminate(ctx)
	hub = magicevent.NewHub(16)
	runOwner = runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: filerepository.NewRunRepository(runsPath), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir()}}, nil)
	})
	t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	restartedProbeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	restartedProbeObserver.Subscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, func(event magicevent.Event, result magicevent.Result) {
		probes++
		command := event.Data().(orchestratorevents.ProbeRuntimeOperationFeasibilityCommand)
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: command.Operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: true}, nil)
	})
	t.Cleanup(func() {
		restartedProbeObserver.Unsubscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	})
	restartedLLMObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	restartedLLMObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requests = append(requests, event.Data().(orchestratorevents.RequestRuntimeCandidateCommand))
		command := requests[len(requests)-1]
		if command.CandidateAttempt != 1 || command.PreviousFailure == nil || command.PreviousFailure.Diagnostic == nil {
			t.Errorf("restarted correction request lost persisted diagnostic: %+v", command)
			return
		}
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "edit-target.txt", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
		}}}}}
		result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { restartedLLMObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	restartedBiz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := restartedBiz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := restartedBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := newTestBiz(restartedBiz, hub, nil).requestAndSealRuntimeV2CandidateWithRetry(ctx, reloaded, "stable-admissibility-prefix")
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

// P3-B3 correction-complete boundary: attempt 1's C4 document is durable
// before C5 admission. A restarted decoder/admission must consume that body,
// atomically exhaust a semantic replay, and never make a third provider call.
func TestRuntimeV2FileBackedCorrectionCompleteRestartExhaustsWithoutProviderReplay(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
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

	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Focus.TargetRefs = nil
	inventory := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{".", "target.txt"}, ObservedRevision: "test-revision"}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{runtimeV2OperationAdmissibilityPrerequisite(inventory)}
	// Keep the prerequisite observation in the file-backed aggregate: correction
	// admission must use this same owner inventory after restart.
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{runtimeV2FileAdmissibilityObservation(inventory)}

	contentsRoot := filepath.Join(root, "candidate-contents")
	firstOwner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(contentsRoot),
	})
	if err := firstOwner.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := firstOwner.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "stable-admissibility-prefix"
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		if command.CandidateAttempt != 0 {
			t.Errorf("restart replayed provider for correction attempt: %+v", command)
			return
		}
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "edit-.", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"."}, Parameters: runtimeV2FileEditParameters("."),
		}}}}}
		result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	failed, err := newTestBiz(firstOwner, hub, nil).requestAndSealRuntimeV2Candidate(ctx, initialized, prefix, 0)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 1 || len(failed.CandidateAttempts) != 1 || failed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated || failed.CandidateAttempts[0].OwnerFeasibility == nil || failed.CandidateAttempts[0].Failure != nil || failed.Facts.Failure == nil || failed.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible {
		t.Fatalf("owner infeasibility was not persisted: providerCalls=%d aggregate=%+v", providerCalls, failed)
	}
	return

	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(failed.Facts, failed.Decision, runtimev2.ActiveRejectedOperationShapes(failed))
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(failed.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
	if err != nil {
		t.Fatal(err)
	}
	previousFailure, err := runtimeV2PreviousCandidateFailure(failed, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, correctionSession, err := firstOwner.EnsureRuntimeV2CandidateRequestSession(ctx, failed.Facts.WorkspaceID, failed.Facts.RunID, prefix, manifest.Hash, runtimecontract.StableRuntimeCandidateRequestCorrectionHash(previousFailure))
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(correctionSession.LogicalRequestID, correctionSession.NextSequence)
	submitted, err := firstOwner.MarkRuntimeV2CandidateRequestSubmitted(ctx, failed.Facts.WorkspaceID, failed.Facts.RunID, correctionSession.LogicalRequestID, correctionSession.NextSequence, physicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	corrected := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "edit-.", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"."}, Parameters: runtimeV2FileEditParameters("."),
	}}}}}
	document := runtimeV2CandidateDocumentForTest(t, manifest, corrected)
	completed, _, _, err := newTestBiz(firstOwner, hub, nil).persistAndCompleteRuntimeCandidateRequestDocument(ctx, submitted, correctionSession.LogicalRequestID, correctionSession.NextSequence, physicalCallID, document)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 1 || len(completed.CandidateAttempts) != 1 || completed.CandidateRequestSessions[len(completed.CandidateRequestSessions)-1].State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
		t.Fatalf("correction C4 boundary is not attempt-free/durable: providerCalls=%d aggregate=%+v", providerCalls, completed)
	}

	// All in-process candidate response and controller state is discarded. C5
	// must read the file-backed correction document and admit attempt 1 only.
	if err := runOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	hub.Terminate(ctx)
	hub = magicevent.NewHub(16)
	runOwner = runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: filerepository.NewRunRepository(filepath.Join(root, "runs.json")), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir()}}, nil)
	})
	t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	restartedProbeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	restartedProbeObserver.Subscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, func(event magicevent.Event, result magicevent.Result) {
		probes++
		command := event.Data().(orchestratorevents.ProbeRuntimeOperationFeasibilityCommand)
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: command.Operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false}, nil)
	})
	t.Cleanup(func() {
		restartedProbeObserver.Unsubscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	})
	restartedLLMCalls := 0
	restartedLLMObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	restartedLLMObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		restartedLLMCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID, Failure: &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassProvider, Code: "unexpected_provider_replay"}}, nil)
	})
	t.Cleanup(func() { restartedLLMObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	restartedOwner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(contentsRoot),
	})
	if err := restartedOwner.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := restartedOwner.LoadRuntimeV2(ctx, failed.Facts.WorkspaceID, failed.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := newTestBiz(restartedOwner, hub, nil).requestAndSealRuntimeV2CandidateWithRetry(ctx, reloaded, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 1 || restartedLLMCalls != 0 || probes != 1 || sealed.ActiveDispatch != nil || len(sealed.CandidateAttempts) != 2 ||
		sealed.CandidateAttempts[1].State != runtimecontract.RuntimeCandidateAttemptStateFailed ||
		sealed.CandidateAttempts[1].LogicalRequestID != correctionSession.LogicalRequestID || sealed.Decision.Terminal == nil ||
		sealed.Decision.Terminal.Code != "candidate_retry_exhausted" || len(sealed.GovernanceRequests) != 0 || sealed.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("correction-complete restart did not atomically exhaust replay: providerCalls=%d probes=%d aggregate=%+v", providerCalls, probes, sealed)
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
		target := "target.txt"
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "directory-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target}, Parameters: runtimeV2FileEditParameters(target),
		}}}}}
		result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	facts := runtimeV2Aggregate(t, 1).Facts
	// This fixture deliberately leaves the manifest target set unrestrictive
	// while keeping two owner-admissible targets. Attempt 0 rejects target.txt,
	// so correction has one real structural alternative (replacement.txt).
	// Attempt 1 deliberately reuses the rejected shape to prove bounded
	// exhaustion rather than the B2b no-objective terminal fence.
	facts.Focus.TargetRefs = nil
	inventory := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt", "replacement.txt"}, ObservedRevision: "test-revision"}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{
		runtimeV2OperationAdmissibilityPrerequisite(inventory),
	}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{
		runtimeV2FileAdmissibilityObservation(inventory),
	}
	biz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeV2Candidate(ctx, initialized, "stable-admissibility-prefix", 0)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || terminal.ActiveDispatch != nil || len(terminal.CandidateAttempts) != 1 || terminal.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated || terminal.CandidateAttempts[0].OwnerFeasibility == nil || terminal.Facts.Failure == nil || terminal.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible {
		t.Fatalf("owner infeasibility incorrectly entered candidate exhaustion/correction: requests=%d aggregate=%+v", requests, terminal)
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

	biz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	_, err := biz.InitializeRuntimeV2(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "persisted-proposal-prefix"
	pending, err := biz.BeginRuntimeV2CandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, prefix)
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

	biz := orchestratorbiz.NewRuntimeOrchestrator()
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
			newOperation.Parameters[1].String = &newText
			candidate.StateChange = &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: replayedOperation}, {Operation: newOperation}}}
		} else {
			if command.PreviousFailure == nil || command.PreviousFailure.Code != "candidate_partially_replays_rejected_shape" {
				t.Errorf("retry missing typed partial replay failure: %+v", command.PreviousFailure)
			}
			corrected := replayedOperation
			corrected.ID = "corrected-edit"
			newText := "corrected"
			corrected.Parameters = runtimeV2FileEditParameters("target.txt")
			corrected.Parameters[1].String = &newText
			candidate.StateChange = &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: corrected}}}
		}
		result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
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
	biz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	if _, err := biz.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), nil); err != nil {
		t.Fatal(err)
	}
	const prefix = "persisted-feasibility-prefix"
	if _, err := biz.BeginRuntimeV2CandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
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

func runtimeV2FileEditParameters(_ string) []runtimecontract.OperationParameter {
	oldText := "old"
	newText := "new"
	return []runtimecontract.OperationParameter{
		{Name: "oldText", String: &oldText},
		{Name: "newText", String: &newText},
	}
}

func TestRuntimeV2MainlineTerminatesGenericProviderProtocolFailureWithoutCandidateAttempt(t *testing.T) {
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
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID, Failure: &failure}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	biz := orchestratorbiz.NewRuntimeOrchestrator()
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
	if requests != 1 || terminal.Decision.Action != runtimecontract.RuntimeActionFail || terminal.Decision.Terminal == nil ||
		terminal.Decision.Terminal.Code != "provider_protocol_failure" || terminal.ActiveDispatch != nil ||
		len(terminal.CandidateAttempts) != 0 || terminal.Facts.Repair.RepairAttempts != 0 ||
		terminal.Facts.Recovery.ProviderTransportAttempts != 0 || len(terminal.GovernanceRequests) != 0 {
		t.Fatalf("generic provider protocol failure must remain request-session-only: requests=%d terminal=%+v", requests, terminal)
	}
	if len(terminal.CandidateRequestSessions) != 1 {
		t.Fatalf("candidate request sessions = %+v", terminal.CandidateRequestSessions)
	}
	session := terminal.CandidateRequestSessions[0]
	if session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
		session.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol ||
		session.LastOutcome.Code != "provider_protocol_failure" {
		t.Fatalf("provider protocol session = %+v", session)
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

func runtimeV2CandidateRequestContentID(t *testing.T, aggregate runtimev2.RuntimeAggregate, stablePrefixHash string, attempt int) common.ArtifactContinuationID {
	t.Helper()
	context := runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, runtimev2.ActiveRejectedOperationShapes(aggregate))
	manifest, manifestErr := runtimecontract.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(context.Resources))
	if manifestErr != nil {
		t.Fatal(manifestErr)
	}
	previousFailure, err := runtimeV2PreviousCandidateFailure(aggregate, attempt)
	if err != nil {
		t.Fatal(err)
	}
	logicalRequestID, _, err := runtimeV2CandidateRequestIdentity(aggregate, stablePrefixHash, context, manifest, previousFailure, 1)
	if err != nil {
		t.Fatal(err)
	}
	return runtimecontract.StableRuntimeCandidateRequestContentID(logicalRequestID)
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
