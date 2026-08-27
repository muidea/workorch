package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type failRuntimeLeaseRenewalRepository struct {
	delegate                 runtime.RuntimeStateRepository
	remainingRenewalFailures int
	renewalFailures          int
}

type blockingRuntimeLeaseClaimLoadRepository struct {
	delegate runtime.RuntimeStateRepository
	entered  chan struct{}
	release  chan struct{}
	block    bool
}

type blockingRuntimeTimerSubmissionBackground struct {
	recordingBackgroundRoutine
	mu      sync.Mutex
	entered chan struct{}
	release chan struct{}
	queued  func()
}

// scheduledDrivePanicRepository places a process panic on either side of the
// first owner repository boundary. The one-shot fault lets the Orchestrator's
// panic fence reload the surviving aggregate and persist the ordinary SM-4
// DriveLease through the same repository immediately afterward.
type scheduledDrivePanicRepository struct {
	delegate           runtime.RuntimeStateRepository
	panicOnFirstLoad   bool
	panicAfterFirstCAS bool
	loadPanicFired     bool
	savePanicFired     bool
	persistedRevision  uint64
	marker             string
}

type failOnceRuntimeLoadRepository struct {
	delegate runtime.RuntimeStateRepository
	fired    bool
}

type panicOnceRuntimeTimerBackground struct {
	recordingBackgroundRoutine
	panicked          bool
	successfulSubmits int
}

func (b *panicOnceRuntimeTimerBackground) AsyncFunction(function func()) error {
	b.asyncFunctionCalls++
	if !b.panicked {
		b.panicked = true
		panic("injected Runtime timer submission panic")
	}
	b.successfulSubmits++
	b.lastAsyncFunction = function
	return nil
}

func (r *failOnceRuntimeLoadRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	if !r.fired {
		r.fired = true
		return runtime.RuntimeAggregate{}, errors.New("injected persisted lease preflight load failure")
	}
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *failOnceRuntimeLoadRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	return r.delegate.Save(ctx, aggregate, expectedRevision)
}

func (r *scheduledDrivePanicRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	if r.panicOnFirstLoad && !r.loadPanicFired {
		r.loadPanicFired = true
		panic(r.marker)
	}
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *scheduledDrivePanicRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	if err := r.delegate.Save(ctx, aggregate, expectedRevision); err != nil {
		return err
	}
	if r.panicAfterFirstCAS && !r.savePanicFired {
		r.savePanicFired = true
		r.persistedRevision = aggregate.StateRevision
		panic(r.marker)
	}
	return nil
}

func (r *scheduledDrivePanicRepository) reachedConfiguredBoundaries() bool {
	return (!r.panicOnFirstLoad || r.loadPanicFired) && (!r.panicAfterFirstCAS || r.savePanicFired)
}

func (b *blockingRuntimeTimerSubmissionBackground) AsyncFunction(function func()) error {
	b.mu.Lock()
	b.asyncFunctionCalls++
	b.mu.Unlock()
	b.entered <- struct{}{}
	<-b.release
	b.mu.Lock()
	b.lastAsyncFunction = function
	b.queued = function
	b.mu.Unlock()
	return nil
}

func (b *blockingRuntimeTimerSubmissionBackground) snapshot() (int, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.asyncFunctionCalls, b.queued
}

func (r *blockingRuntimeLeaseClaimLoadRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	if r.block {
		r.block = false
		close(r.entered)
		<-r.release
	}
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *blockingRuntimeLeaseClaimLoadRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	return r.delegate.Save(ctx, aggregate, expectedRevision)
}

func (r *failRuntimeLeaseRenewalRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *failRuntimeLeaseRenewalRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	current, err := r.delegate.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		return err
	}
	if r.remainingRenewalFailures > 0 && current.DriveLease != nil && current.DriveLease.State == runtimecontract.RuntimeDriveLeaseRunning &&
		aggregate.DriveLease != nil && aggregate.DriveLease.State == runtimecontract.RuntimeDriveLeaseScheduled &&
		aggregate.DriveLease.RecoveryGeneration == current.DriveLease.RecoveryGeneration+1 {
		r.remainingRenewalFailures--
		r.renewalFailures++
		return errors.New("injected drive lease renewal CAS failure")
	}
	return r.delegate.Save(ctx, aggregate, expectedRevision)
}

func TestScheduledRuntimeDrivePanicPersistsSingleOwnerSuccessorAndRestartMountsOnce(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		panicOnLoad   bool
		panicAfterCAS bool
		panicInFinish bool
	}{
		{name: "before aggregate CAS", panicOnLoad: true},
		{name: "after aggregate CAS", panicAfterCAS: true},
		{name: "during successor lease CAS", panicOnLoad: true, panicInFinish: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			aggregate := runtimeAggregate(t, 1)
			baseRepository := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
			faultRepository := &scheduledDrivePanicRepository{
				delegate: baseRepository, panicOnFirstLoad: testCase.panicOnLoad,
				panicAfterFirstCAS: testCase.panicAfterCAS || testCase.panicInFinish, marker: "scheduled-drive-" + testCase.name,
			}

			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(context.Background()) })
			runs := memory.NewRunRepository()
			if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, aggregate.Definition)); err != nil {
				t.Fatal(err)
			}
			runOwner := runmodule.New()
			if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runOwner.Stop(context.Background()) })

			owner := newRuntimeTestOwner()
			if err := owner.ConfigureRuntimeRepository(faultRepository); err != nil {
				t.Fatal(err)
			}
			background := &recordingBackgroundRoutine{}
			module := newTestBiz(owner, hub, background)
			key := runtimeDriveKey(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
			if !module.beginRuntimeDrive(key) {
				t.Fatal("scheduled drive did not acquire its sole process gate")
			}
			request := scheduledRuntimeDriveRequest{
				workspaceID: aggregate.Facts.WorkspaceID, runID: aggregate.Facts.RunID,
				source: "panic_fence_test", driveKey: key,
			}
			// The panic must not escape to magicCommon's process-level routine
			// recovery. The Orchestrator owner converts it into persisted SM-4
			// successor authority before this call returns.
			module.runScheduledRuntimeDrive(ctx, request)
			if !faultRepository.reachedConfiguredBoundaries() {
				t.Fatal("scheduled drive did not reach the injected panic boundary")
			}

			persisted, err := baseRepository.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
			if err != nil || persisted.DriveLease == nil {
				t.Fatalf("panic fence successor=%+v err=%v", persisted.DriveLease, err)
			}
			panicRevision := aggregate.StateRevision
			if testCase.panicAfterCAS {
				panicRevision = faultRepository.persistedRevision
				if panicRevision <= aggregate.StateRevision {
					t.Fatalf("after-CAS panic did not persist progress: revision=%d initial=%d", panicRevision, aggregate.StateRevision)
				}
			}
			lease := persisted.DriveLease.Clone()
			wantTimerCalls := 1
			if lease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
				lease.Reason != runtimecontract.RuntimeDriveLeaseReasonPersistedPending ||
				lease.FailureSignature != runtimeScheduledDrivePanicLeaseRequest("", panicRevision, true).FailureSignature ||
				lease.AggregateRevision != persisted.StateRevision || background.asyncFunctionCalls != wantTimerCalls ||
				(wantTimerCalls != 0 && background.lastAsyncFunction == nil) {
				t.Fatalf("panic successor lease=%+v stateRevision=%d timers=%d", lease, persisted.StateRevision, background.asyncFunctionCalls)
			}
			if testCase.panicInFinish && persisted.StateRevision != faultRepository.persistedRevision {
				// The lease CAS committed before its response was lost. The outer
				// fence must mount that exact durable generation in-process without
				// recursively retrying persistence.
				t.Fatalf("finish panic recursively rewrote persisted successor: panicRevision=%d stateRevision=%d", faultRepository.persistedRevision, persisted.StateRevision)
			}
			if module.beginRuntimeDrive(key) {
				t.Fatal("panic successor released the generation-zero gate")
			}
			if err := module.scheduleRuntimeDrive(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "duplicate_after_panic"); err != nil {
				t.Fatal(err)
			}
			if background.asyncFunctionCalls != wantTimerCalls {
				t.Fatalf("duplicate ingress mounted a second timer/drive: calls=%d", background.asyncFunctionCalls)
			}

			// Stop discards only local gate/timer machinery. A fresh owner must
			// adopt the exact persisted generation once; duplicate restart ingress
			// cannot create another worker or lease transition.
			if err := module.Stop(ctx); err != nil {
				t.Fatal(err)
			}
			restartedOwner := newRuntimeTestOwner()
			if err := restartedOwner.ConfigureRuntimeRepository(baseRepository); err != nil {
				t.Fatal(err)
			}
			restartedBackground := &recordingBackgroundRoutine{}
			restarted := newTestBiz(restartedOwner, hub, restartedBackground)
			t.Cleanup(func() { _ = restarted.Stop(context.Background()) })
			if err := restarted.scheduleRuntimeDrive(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "restart_after_panic"); err != nil {
				t.Fatal(err)
			}
			if restartedBackground.asyncFunctionCalls != 1 || restartedBackground.lastAsyncFunction == nil {
				t.Fatalf("restart did not schedule one owner worker: calls=%d", restartedBackground.asyncFunctionCalls)
			}
			restartWorker := restartedBackground.lastAsyncFunction
			restartWorker()
			if restartedBackground.asyncFunctionCalls != 2 || restartedBackground.lastAsyncFunction == nil {
				t.Fatalf("restart worker did not mount one persisted timer: calls=%d", restartedBackground.asyncFunctionCalls)
			}
			if err := restarted.scheduleRuntimeDrive(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "duplicate_restart_after_panic"); err != nil {
				t.Fatal(err)
			}
			if restartedBackground.asyncFunctionCalls != 2 || restarted.beginRuntimeDrive(key) {
				t.Fatalf("duplicate restart escaped singleflight: calls=%d", restartedBackground.asyncFunctionCalls)
			}
			afterRestart, err := baseRepository.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
			if err != nil || afterRestart.StateRevision != persisted.StateRevision || afterRestart.DriveLease == nil ||
				!runtimecontract.SameRuntimeDriveLeaseIdentity(*afterRestart.DriveLease, lease) {
				t.Fatalf("restart rewrote panic successor: before=%+v after=%+v err=%v", lease, afterRestart.DriveLease, err)
			}
		})
	}
}

func TestScheduledRuntimeDriveTimerSubmissionPanicClearsRegistrationAndRemountsExactLease(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeAggregate(t, 1)
	baseRepository := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
	faultRepository := &scheduledDrivePanicRepository{
		delegate: baseRepository, panicOnFirstLoad: true, marker: "drive-before-timer-submission",
	}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(faultRepository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	background := &panicOnceRuntimeTimerBackground{}
	module := newTestBiz(owner, hub, background)
	key := runtimeDriveKey(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if !module.beginRuntimeDrive(key) {
		t.Fatal("timer submission panic fixture did not acquire gate")
	}
	module.runScheduledRuntimeDrive(ctx, scheduledRuntimeDriveRequest{
		workspaceID: aggregate.Facts.WorkspaceID, runID: aggregate.Facts.RunID,
		source: "timer_submission_panic", driveKey: key,
	})
	persisted, err := baseRepository.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || !faultRepository.loadPanicFired || persisted.DriveLease == nil ||
		persisted.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		background.asyncFunctionCalls != 2 || !background.panicked || background.successfulSubmits != 1 || background.lastAsyncFunction == nil {
		t.Fatalf("timer panic successor=%+v calls=%d successful=%d err=%v", persisted.DriveLease, background.asyncFunctionCalls, background.successfulSubmits, err)
	}
	lease := persisted.DriveLease.Clone()
	module.driveMu.Lock()
	registration, registered := module.retryLeaseTimers[key]
	_, gateRetained := module.drivingRuns[key]
	module.driveMu.Unlock()
	if !registered || registration.Generation != lease.RecoveryGeneration || !gateRetained {
		t.Fatalf("timer remount registration=%+v registered=%v gate=%v lease=%+v", registration, registered, gateRetained, lease)
	}
	if err := module.scheduleRuntimeDrive(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "duplicate_after_timer_submission_panic"); err != nil {
		t.Fatal(err)
	}
	if background.asyncFunctionCalls != 2 || background.successfulSubmits != 1 {
		t.Fatalf("duplicate ingress remounted timer: calls=%d successful=%d", background.asyncFunctionCalls, background.successfulSubmits)
	}
}

func TestNormalDrivePersistedLeasePreflightFailureNeverReleasesActiveGate(t *testing.T) {
	t.Run("load failure persists one retry successor", func(t *testing.T) {
		ctx := context.Background()
		aggregate := runtimeAggregate(t, 1)
		baseRepository := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
		faultRepository := &failOnceRuntimeLoadRepository{delegate: baseRepository}
		owner := newRuntimeTestOwner()
		if err := owner.ConfigureRuntimeRepository(faultRepository); err != nil {
			t.Fatal(err)
		}
		background := &recordingBackgroundRoutine{}
		module := newTestBiz(owner, magicevent.NewHub(8), background)
		t.Cleanup(func() { module.EventHub().Terminate(context.Background()) })
		key := runtimeDriveKey(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
		if !module.beginRuntimeDrive(key) {
			t.Fatal("preflight load fixture did not acquire gate")
		}
		module.runScheduledRuntimeDrive(ctx, scheduledRuntimeDriveRequest{
			workspaceID: aggregate.Facts.WorkspaceID, runID: aggregate.Facts.RunID,
			source: "preflight_load_failure", driveKey: key,
		})
		persisted, err := baseRepository.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
		if err != nil || !faultRepository.fired || persisted.DriveLease == nil ||
			persisted.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
			persisted.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonScheduleRetry ||
			persisted.DriveLease.FailureSignature != "persisted-lease-preflight/load" ||
			background.asyncFunctionCalls != 1 || background.lastAsyncFunction == nil {
			t.Fatalf("preflight load successor=%+v calls=%d fired=%v err=%v", persisted.DriveLease, background.asyncFunctionCalls, faultRepository.fired, err)
		}
		if module.beginRuntimeDrive(key) {
			t.Fatal("preflight load failure released active gate")
		}
	})

	t.Run("running recovery failure remounts exact authority", func(t *testing.T) {
		ctx := context.Background()
		aggregate := runtimeAggregate(t, 1)
		baseRepository := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
		seedOwner := newRuntimeTestOwner()
		if err := seedOwner.ConfigureRuntimeRepository(baseRepository); err != nil {
			t.Fatal(err)
		}
		seedModule := newTestBiz(seedOwner, magicevent.NewHub(8), nil)
		t.Cleanup(func() { seedModule.EventHub().Terminate(context.Background()) })
		dueAt := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
		scheduled, _, err := seedModule.runtimeScheduleDriveLease(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeDriveLease{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			Reason: runtimecontract.RuntimeDriveLeaseReasonPersistedPending, FailureSignature: "preflight-running-authority", DueAt: dueAt,
		}, 0)
		if err != nil {
			t.Fatal(err)
		}
		running, claimed, err := seedModule.runtimeClaimDriveLease(ctx, scheduled, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil || !claimed || running.State != runtimecontract.RuntimeDriveLeaseRunning {
			t.Fatalf("seed running lease=%+v claimed=%v err=%v", running, claimed, err)
		}

		faultRepository := &failRuntimeLeaseRenewalRepository{delegate: baseRepository, remainingRenewalFailures: 1}
		owner := newRuntimeTestOwner()
		if err := owner.ConfigureRuntimeRepository(faultRepository); err != nil {
			t.Fatal(err)
		}
		background := &recordingBackgroundRoutine{}
		module := newTestBiz(owner, magicevent.NewHub(8), background)
		t.Cleanup(func() { module.EventHub().Terminate(context.Background()) })
		key := runtimeDriveKey(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
		if !module.beginRuntimeDrive(key) {
			t.Fatal("running preflight fixture did not acquire gate")
		}
		module.runScheduledRuntimeDrive(ctx, scheduledRuntimeDriveRequest{
			workspaceID: aggregate.Facts.WorkspaceID, runID: aggregate.Facts.RunID,
			source: "preflight_running_recover_failure", driveKey: key,
		})
		persisted, err := baseRepository.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
		if err != nil || faultRepository.renewalFailures != 1 || persisted.DriveLease == nil ||
			persisted.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
			persisted.DriveLease.RecoveryGeneration != running.RecoveryGeneration+1 ||
			persisted.DriveLease.FailureSignature != running.FailureSignature ||
			background.asyncFunctionCalls != 1 || background.lastAsyncFunction == nil {
			t.Fatalf("running preflight recovery successor=%+v calls=%d failures=%d err=%v", persisted.DriveLease, background.asyncFunctionCalls, faultRepository.renewalFailures, err)
		}
		if module.beginRuntimeDrive(key) {
			t.Fatal("running preflight recovery released active gate")
		}
	})

	t.Run("scheduled mount rejection retains exact durable lease", func(t *testing.T) {
		ctx := context.Background()
		aggregate := runtimeAggregate(t, 1)
		baseRepository := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
		seedOwner := newRuntimeTestOwner()
		if err := seedOwner.ConfigureRuntimeRepository(baseRepository); err != nil {
			t.Fatal(err)
		}
		seedHub := magicevent.NewHub(8)
		t.Cleanup(func() { seedHub.Terminate(context.Background()) })
		seedModule := newTestBiz(seedOwner, seedHub, nil)
		lease, _, err := seedModule.runtimeScheduleDriveLease(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeDriveLease{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			Reason: runtimecontract.RuntimeDriveLeaseReasonPersistedPending, FailureSignature: "preflight-scheduled-authority",
			DueAt: time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano),
		}, 0)
		if err != nil {
			t.Fatal(err)
		}
		owner := newRuntimeTestOwner()
		if err := owner.ConfigureRuntimeRepository(baseRepository); err != nil {
			t.Fatal(err)
		}
		background := &recordingBackgroundRoutine{err: errors.New("injected timer mount rejection")}
		hub := magicevent.NewHub(8)
		t.Cleanup(func() { hub.Terminate(context.Background()) })
		module := newTestBiz(owner, hub, background)
		key := runtimeDriveKey(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
		if !module.beginRuntimeDrive(key) {
			t.Fatal("scheduled preflight fixture did not acquire gate")
		}
		module.runScheduledRuntimeDrive(ctx, scheduledRuntimeDriveRequest{
			workspaceID: aggregate.Facts.WorkspaceID, runID: aggregate.Facts.RunID,
			source: "preflight_scheduled_mount_failure", driveKey: key,
		})
		persisted, err := baseRepository.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
		if err != nil || persisted.StateRevision != lease.AggregateRevision || persisted.DriveLease == nil ||
			!runtimecontract.SameRuntimeDriveLeaseIdentity(*persisted.DriveLease, lease) || background.asyncFunctionCalls != 2 {
			t.Fatalf("scheduled mount failure rewrote authority: before=%+v after=%+v calls=%d err=%v", lease, persisted.DriveLease, background.asyncFunctionCalls, err)
		}
		if module.beginRuntimeDrive(key) {
			t.Fatal("scheduled mount failure released active gate")
		}
	})
}

func runtimeBoundDefinitionClarificationAggregate(t *testing.T) runtime.RuntimeAggregate {
	t.Helper()
	ctx := context.Background()
	aggregate := runtimeAggregate(t, 1)
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion: runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:  runtimecontract.RuntimeProviderDocumentSemanticPlan,
		PlanStep:      runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		ManifestHash:  "semantic-manifest-lifecycle-gap", DefinitionSeedHash: aggregate.Facts.DefinitionSeed.Hash,
		LogicalRequestID: "semantic-request-lifecycle-gap", StablePrefixHash: "semantic-prefix-lifecycle-gap",
		MaxOutputTokens: 4096, MaxRecoveryAttempts: 1,
	}
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	aggregate.CandidateRequestSessions = append(aggregate.CandidateRequestSessions, runtimecontract.RuntimeCandidateRequestSession{
		LogicalRequestID: expected.LogicalRequestID, DocumentKind: expected.DocumentKind, ExpectedDocument: &expected,
		Transaction: aggregate.Decision.AuthorityTransaction(), StablePrefixHash: expected.StablePrefixHash, ManifestHash: expected.ManifestHash,
		State: runtimecontract.RuntimeCandidateRequestSessionDocumentComplete, NextSequence: 1,
		LastPhysicalCallID: "semantic-call-lifecycle-gap", ProviderSubmitted: true,
		LastOutcome: runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete},
	})
	repository := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM, OriginalClass: runtimecontract.RuntimeDefinitionGapResultKind,
		OriginalCode: string(runtimecontract.RuntimeDefinitionGapUndeclaredPrerequisite), OriginalMessage: "sealed requirement needs an undeclared owner capability",
		Signature: "runtime-definition-gap-lifecycle-fixture", TargetRefs: []string{"cwi-v2"},
	}
	observation := runtimecontract.RuntimeDefinitionGapObservation{
		SchemaVersion: runtimecontract.RuntimeDefinitionClarificationSchemaCurrent,
		WorkspaceID:   aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DefinitionRevision: aggregate.Definition.Revision, StartupInputHash: aggregate.Definition.StartupInputHash,
		AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, FocusRef: aggregate.Facts.Focus.ID,
		LogicalRequestID: expected.LogicalRequestID, SemanticDocumentHash: "semantic-document-lifecycle-gap",
		GapKind: runtimecontract.RuntimeDefinitionGapUndeclaredPrerequisite, OffendingItemRefs: []string{"cwi-v2"}, OriginalDiagnostic: diagnostic,
	}
	observation.ObservationID = runtimecontract.StableRuntimeDefinitionGapObservationID(observation)
	_, err = controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:      runtime.RuntimeInternalEventDefinitionGapObserved,
		Scope:     runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Authority: &runtime.RuntimeInternalAuthorityPayload{DefinitionGap: &observation},
	})
	if err != nil {
		t.Fatalf("observe Definition gap: %v", err)
	}
	sealed, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventDefinitionClarificationDispatchSealed,
		Scope:       runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Transaction: &runtime.RuntimeInternalTransactionPayload{},
	})
	if err != nil || sealed.Aggregate.DefinitionClarification == nil || sealed.Aggregate.DefinitionClarification.Dispatch == nil {
		t.Fatalf("seal Definition clarification dispatch: state=%+v err=%v", sealed.Aggregate.DefinitionClarification, err)
	}
	dispatch := sealed.Aggregate.DefinitionClarification.Dispatch.Clone()
	request := runtimecontract.RuntimeDefinitionClarificationRequestRecord{
		DispatchID: dispatch.DispatchID, TicketID: "runtime-definition-clarification:lifecycle-gap", RequestBindingHash: "definition-clarification-request-binding",
	}
	bound, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventDefinitionClarificationRequestObserved,
		Scope:       runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Transaction: &runtime.RuntimeInternalTransactionPayload{DefinitionClarificationRequest: &request},
	})
	if err != nil {
		t.Fatalf("bind Definition clarification request: %v", err)
	}
	// This helper returns one pre-existing owner snapshot, not the sequence of
	// private reducer commits used to build it. Seed that snapshot through the
	// Run owner's initial opaque CAS at revision 1; all behavior under test then
	// advances it through ordinary typed internal events.
	bound.Aggregate.StateRevision = 1
	if err := bound.Aggregate.Validate(); err != nil {
		t.Fatalf("normalize Definition clarification fixture revision: %v", err)
	}
	return bound.Aggregate
}

func TestRuntimeDefinitionClarificationPersistsExactLifecycleFenceAndBlocksRun(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeBoundDefinitionClarificationAggregate(t)
	handoff, err := runtimeGovernedWaitDriveResult(aggregate)
	if err != nil {
		t.Fatal(err)
	}

	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, aggregate.Definition)); err != nil {
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

	stateRepository := runtimeStateRepository{hub: hub}
	if err := stateRepository.Save(ctx, aggregate, 0); err != nil {
		t.Fatalf("persist bound aggregate: %v", err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || persisted.RuntimeLifecycleFence == nil || persisted.RuntimeState == nil {
		t.Fatalf("persisted Run lifecycle fence=%+v err=%v", persisted, err)
	}
	wantFence := runtimeGovernanceLifecycleFenceID(handoff.AuthoritySnapshotID, handoff.DispatchID)
	if persisted.RuntimeLifecycleFence.GovernanceFenceID != wantFence || persisted.RuntimeLifecycleFence.GovernanceResolutionID != "" || persisted.RuntimeLifecycleFence.RuntimeStateRevision != aggregate.StateRevision ||
		persisted.RuntimeLifecycleFence.RuntimeStateHash != runtimecontract.StableRuntimeStateRecordHash(*persisted.RuntimeState) {
		t.Fatalf("Definition clarification fence=%+v wantGovernance=%s", persisted.RuntimeLifecycleFence, wantFence)
	}

	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(stateRepository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	if err := module.driveAndProjectRuntimeLockedWithCommand(ctx, orchestratorevents.DriveRuntimeCommand{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}); err != nil {
		t.Fatalf("deliver Definition clarification lifecycle handoff: %v", err)
	}
	blocked, err := runs.GetByWorkspaceID(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || blocked.State != common.RunStateBlockedOnDecision || blocked.BlockingTicketID != handoff.BlockingTicketID {
		t.Fatalf("Run owner governance wait=%+v err=%v", blocked, err)
	}
	blockedRevision := blocked.Revision
	if blocked.RuntimeState.Facts.Recovery.TerminalDeliveryAttempts != 0 {
		t.Fatalf("governance handoff consumed terminal delivery budget: %+v", blocked.RuntimeState.Facts.Recovery)
	}
	if err := module.driveAndProjectRuntimeLockedWithCommand(ctx, orchestratorevents.DriveRuntimeCommand{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}); err != nil {
		t.Fatalf("duplicate lifecycle handoff: %v", err)
	}
	duplicate, err := runs.GetByWorkspaceID(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || duplicate.Revision != blockedRevision || duplicate.BlockingTicketID != blocked.BlockingTicketID {
		t.Fatalf("duplicate lifecycle handoff rewrote Run: before=%+v after=%+v err=%v", blocked, duplicate, err)
	}
}

func TestRuntimeGovernanceLifecycleHandoffMatrixUsesCurrentPersistedIdentity(t *testing.T) {
	baseAggregate := runtimeAggregate(t, 1)
	baseRecord := runtimeStateRecordFromAggregate(baseAggregate)
	baseRecord.Facts.DefinitionRevision = "definition-current"
	baseRecord.Facts.EvidenceRevision = "evidence-current"

	for _, tc := range []struct {
		name       string
		approval   bool
		resolution bool
	}{
		{name: "decision wait"},
		{name: "approval wait", approval: true},
		{name: "decision continuation", resolution: true},
		{name: "approval continuation", approval: true, resolution: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := baseRecord.Clone()
			record.Decision.AuthoritySnapshot.ID = "authority-current"
			request := runtimecontract.RuntimeGovernanceRequestRecord{
				AuthoritySnapshotID: "authority-current", DispatchID: "dispatch-current", TicketID: "ticket-current",
			}
			if tc.approval {
				request.TicketID = ""
				request.ApprovalID = "approval-current"
			}
			if tc.resolution {
				record.Decision.State = runtimecontract.ConvergenceStateNeedStateChange
				record.GovernanceResolutions = []runtimecontract.RuntimeGovernanceResolutionRecord{
					{AuthoritySnapshotID: "authority-stale", DispatchID: "dispatch-stale", TicketID: "ticket-stale", ResolutionID: "resolution-stale", Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue},
					{AuthoritySnapshotID: request.AuthoritySnapshotID, DispatchID: request.DispatchID, TicketID: request.TicketID, ApprovalID: request.ApprovalID, ResolutionID: "resolution-current", Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue},
				}
			} else {
				record.Decision.State = runtimecontract.ConvergenceStateNeedGovernance
				record.ActiveDispatch = &runtimecontract.RuntimeDispatchRecord{ID: request.DispatchID}
				record.GovernanceRequests = []runtimecontract.RuntimeGovernanceRequestRecord{
					{AuthoritySnapshotID: "authority-stale", DispatchID: "dispatch-stale", TicketID: "ticket-stale"},
					request,
				}
			}
			handoff, governed, err := runtimeGovernanceLifecycleHandoffFromState(&record)
			if err != nil || !governed || handoff.AuthoritySnapshotID != request.AuthoritySnapshotID || handoff.DispatchID != request.DispatchID ||
				handoff.TicketID != request.TicketID || handoff.ApprovalID != request.ApprovalID || handoff.FenceID() != "runtime-governance/authority-current/dispatch-current" {
				t.Fatalf("handoff=%+v governed=%v err=%v", handoff, governed, err)
			}
			if tc.resolution && handoff.ResolutionID != "resolution-current" {
				t.Fatalf("continuation resolution=%s", handoff.ResolutionID)
			}
			run := runtimeRunForTest(common.RunStateActive, baseAggregate.Definition)
			run.RuntimeState = &record
			fence, err := runtimeLifecycleFenceFromModel(run)
			if err != nil || fence == nil || fence.GovernanceFenceID != handoff.FenceID() || fence.GovernanceResolutionID != handoff.ResolutionID {
				t.Fatalf("persisted lifecycle fence=%+v handoff=%+v err=%v", fence, handoff, err)
			}
			if !tc.resolution && fence.GovernanceResolutionID != "" {
				t.Fatalf("wait fence carried resolution=%s", fence.GovernanceResolutionID)
			}
		})
	}

	stale := baseRecord.Clone()
	stale.Decision.State = runtimecontract.ConvergenceStateNeedGovernance
	stale.Decision.AuthoritySnapshot.ID = "authority-current"
	stale.ActiveDispatch = &runtimecontract.RuntimeDispatchRecord{ID: "dispatch-current"}
	stale.GovernanceRequests = []runtimecontract.RuntimeGovernanceRequestRecord{{
		AuthoritySnapshotID: "authority-stale", DispatchID: "dispatch-stale", TicketID: "ticket-stale",
	}}
	if handoff, governed, err := runtimeGovernanceLifecycleHandoffFromState(&stale); err != nil || governed || handoff.Kind != "" {
		t.Fatalf("stale owner request authorized handoff=%+v governed=%v err=%v", handoff, governed, err)
	}

	terminalAggregate := runtimeTerminalAggregate(t)
	terminalRecord := runtimeStateRecordFromAggregate(terminalAggregate)
	terminalRecord.GovernanceResolutions = []runtimecontract.RuntimeGovernanceResolutionRecord{{
		AuthoritySnapshotID: terminalRecord.Decision.AuthoritySnapshot.ID,
		DispatchID:          "dispatch-before-terminal",
		TicketID:            "ticket-before-terminal",
		ResolutionID:        "resolution-before-terminal",
		Outcome:             runtimecontract.RuntimeGovernanceResolutionOutcomeContinue,
	}}
	terminalRun := runtimeRunForTest(common.RunStateActive, terminalAggregate.Definition)
	terminalRun.RuntimeState = &terminalRecord
	terminalFence, err := runtimeLifecycleFenceFromModel(terminalRun)
	if err != nil || terminalFence == nil || terminalFence.TerminalFenceID == "" || terminalFence.GovernanceFenceID != "" || terminalFence.GovernanceResolutionID != "" {
		t.Fatalf("terminal lifecycle fence=%+v err=%v", terminalFence, err)
	}
}

func TestRunLifecycleAcknowledgementRequiresExactResolutionFence(t *testing.T) {
	fact := runevents.RuntimeContinuationEligibleFact{
		WorkspaceID: "ws-ack", RunID: "run-ack", SourceOwner: orchestratorLifecycleSource, CausationID: "cause-ack",
		AuthoritySnapshotID: "authority-ack", DefinitionRevision: "definition-ack", EvidenceRevision: "evidence-ack",
		DispatchID: "dispatch-ack", ResolutionID: "resolution-current", TicketID: "ticket-ack",
	}
	run := &runmodel.Run{
		ID: fact.RunID, WorkspaceID: fact.WorkspaceID, State: common.RunStateActive,
		RuntimeState: &runtimecontract.RuntimeStateRecord{StateRevision: 9},
		RuntimeLifecycleFence: &runtimecontract.RuntimeOpaqueLifecycleFence{
			WorkspaceID: fact.WorkspaceID, RunID: fact.RunID, RuntimeStateRevision: 9,
			GovernanceFenceID:      runtimeGovernanceLifecycleFenceID(fact.AuthoritySnapshotID, fact.DispatchID),
			GovernanceResolutionID: "resolution-stale",
		},
	}
	response := runevents.RunLifecycleCommandResult{Run: run}
	if err := validateRunContinuationAcknowledgement(response, fact, 9); err == nil {
		t.Fatal("continuation acknowledgement accepted stale persisted resolution")
	}
	run.RuntimeLifecycleFence.GovernanceResolutionID = fact.ResolutionID
	run.RuntimeLifecycleFence.TerminalFenceID = "terminal-conflict"
	if err := validateRunContinuationAcknowledgement(response, fact, 9); err == nil {
		t.Fatal("continuation acknowledgement accepted a dual-mode lifecycle fence")
	}
	run.RuntimeLifecycleFence.TerminalFenceID = ""
	if err := validateRunContinuationAcknowledgement(response, fact, 9); err != nil {
		t.Fatalf("exact continuation acknowledgement: %v", err)
	}
	wait := runevents.RuntimeGovernanceWaitFact{
		WorkspaceID: fact.WorkspaceID, RunID: fact.RunID, SourceOwner: orchestratorLifecycleSource, CausationID: "cause-wait",
		AuthoritySnapshotID: fact.AuthoritySnapshotID, DefinitionRevision: fact.DefinitionRevision, EvidenceRevision: fact.EvidenceRevision,
		DispatchID: fact.DispatchID, BlockingTicketID: fact.TicketID,
	}
	run.State = common.RunStateBlockedOnDecision
	run.BlockingTicketID = fact.TicketID
	if err := validateRunGovernanceWaitAcknowledgement(response, wait, 9); err == nil {
		t.Fatal("governance wait acknowledgement accepted a resolved fence")
	}
	run.RuntimeLifecycleFence.GovernanceResolutionID = ""
	if err := validateRunGovernanceWaitAcknowledgement(response, wait, 9); err != nil {
		t.Fatalf("unresolved wait acknowledgement: %v", err)
	}
}

type failBeforeGovernanceWaitCASRepository struct {
	repository.RunRepository
	fired bool
}

func (r *failBeforeGovernanceWaitCASRepository) CompareAndSwap(ctx context.Context, item *runmodel.Run, expectedRevision uint64) error {
	if !r.fired && item != nil && (item.State == common.RunStateBlockedOnDecision || item.State == common.RunStateBlockedOnApproval) {
		r.fired = true
		return common.ErrConflict
	}
	return r.RunRepository.CompareAndSwap(ctx, item, expectedRevision)
}

type failAfterGovernanceWaitCASRepository struct {
	repository.RunRepository
	fired bool
}

type rejectGovernanceContinuationCASRepository struct {
	repository.RunRepository
	armed bool
}

type failFirstRuntimeDriveReadAfterContinuationRepository struct {
	repository.RunRepository
	resolutionID common.ResolutionID
	armed        bool
	fired        bool
}

func (r *failFirstRuntimeDriveReadAfterContinuationRepository) GetByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (*runmodel.Run, error) {
	run, err := r.RunRepository.GetByWorkspaceID(ctx, workspaceID, runID)
	if err != nil {
		return nil, err
	}
	if r.armed && run.State == common.RunStateActive && run.RuntimeLifecycleFence != nil &&
		run.RuntimeLifecycleFence.GovernanceResolutionID == r.resolutionID {
		r.armed = false
		r.fired = true
		observed := run.RuntimeLifecycleFence.RuntimeStateRevision
		return nil, &runtime.RuntimeCASConflictError{
			WorkspaceID: workspaceID, RunID: runID, ExpectedRevision: observed - 1, ObservedRevision: observed,
		}
	}
	return run, nil
}

func (r *rejectGovernanceContinuationCASRepository) CompareAndSwap(ctx context.Context, item *runmodel.Run, expectedRevision uint64) error {
	if r.armed && item != nil && item.State == common.RunStateActive {
		return common.ErrConflict
	}
	return r.RunRepository.CompareAndSwap(ctx, item, expectedRevision)
}

func (r *failAfterGovernanceWaitCASRepository) GetByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (*runmodel.Run, error) {
	if r.fired {
		return nil, common.ErrConflict
	}
	return r.RunRepository.GetByWorkspaceID(ctx, workspaceID, runID)
}

func (r *failAfterGovernanceWaitCASRepository) CompareAndSwap(ctx context.Context, item *runmodel.Run, expectedRevision uint64) error {
	if err := r.RunRepository.CompareAndSwap(ctx, item, expectedRevision); err != nil {
		return err
	}
	if !r.fired && item != nil && (item.State == common.RunStateBlockedOnDecision || item.State == common.RunStateBlockedOnApproval) {
		r.fired = true
		return common.ErrConflict
	}
	return nil
}

func TestRuntimeDefinitionClarificationLifecycleHandoffCASAndFreshReload(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name           string
		wrap           func(repository.RunRepository) repository.RunRepository
		wantFirstError bool
	}{
		{name: "cas_before_owner_commit", wrap: func(base repository.RunRepository) repository.RunRepository {
			return &failBeforeGovernanceWaitCASRepository{RunRepository: base}
		}},
		{name: "response_loss_after_owner_commit", wantFirstError: true, wrap: func(base repository.RunRepository) repository.RunRepository {
			return &failAfterGovernanceWaitCASRepository{RunRepository: base}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			aggregate := runtimeBoundDefinitionClarificationAggregate(t)
			baseRuns := memory.NewRunRepository()
			if err := baseRuns.Create(ctx, runtimeRunForTest(common.RunStateActive, aggregate.Definition)); err != nil {
				t.Fatal(err)
			}
			firstHub := magicevent.NewHub(16)
			wrappedRuns := tc.wrap(baseRuns)
			firstRunOwner := runmodule.New()
			if err := firstRunOwner.Configure(runbiz.Dependencies{Runs: wrappedRuns, EventHub: firstHub}); err != nil {
				t.Fatal(err)
			}
			if err := firstRunOwner.Start(); err != nil {
				t.Fatal(err)
			}
			stateRepository := runtimeStateRepository{hub: firstHub}
			if err := stateRepository.Save(ctx, aggregate, 0); err != nil {
				t.Fatal(err)
			}
			owner := newRuntimeTestOwner()
			if err := owner.ConfigureRuntimeRepository(stateRepository); err != nil {
				t.Fatal(err)
			}
			module := newTestBiz(owner, firstHub, nil)
			firstErr := module.driveAndProjectRuntimeLockedWithCommand(ctx, orchestratorevents.DriveRuntimeCommand{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID})
			var deliveryErr *runtimeLifecycleOwnerDeliveryError
			if tc.wantFirstError != errors.As(firstErr, &deliveryErr) {
				t.Fatalf("first lifecycle delivery err=%v wantError=%v", firstErr, tc.wantFirstError)
			}
			blocked, err := baseRuns.GetByWorkspaceID(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
			if err != nil || blocked.State != common.RunStateBlockedOnDecision || blocked.RuntimeState.Facts.Recovery.TerminalDeliveryAttempts != 0 {
				t.Fatalf("owner state after interrupted lifecycle handoff=%+v err=%v", blocked, err)
			}
			if err := firstRunOwner.Stop(ctx); err != nil {
				t.Fatal(err)
			}
			firstHub.Terminate(ctx)

			// Fresh process reloads only persisted owner facts. The duplicate drive
			// observes the already-blocked Run and performs no second transition.
			restartedHub := magicevent.NewHub(16)
			t.Cleanup(func() { restartedHub.Terminate(ctx) })
			restartedRunOwner := runmodule.New()
			if err := restartedRunOwner.Configure(runbiz.Dependencies{Runs: baseRuns, EventHub: restartedHub}); err != nil {
				t.Fatal(err)
			}
			if err := restartedRunOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
			restartedOwner := newRuntimeTestOwner()
			if err := restartedOwner.ConfigureRuntimeRepository(runtimeStateRepository{hub: restartedHub}); err != nil {
				t.Fatal(err)
			}
			restartedModule := newTestBiz(restartedOwner, restartedHub, nil)
			beforeRevision := blocked.Revision
			if err := restartedModule.driveAndProjectRuntimeLockedWithCommand(ctx, orchestratorevents.DriveRuntimeCommand{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}); err != nil {
				t.Fatalf("fresh reload duplicate handoff: %v", err)
			}
			reloaded, err := baseRuns.GetByWorkspaceID(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
			if err != nil || reloaded.Revision != beforeRevision || reloaded.RuntimeState.Facts.Recovery.TerminalDeliveryAttempts != 0 {
				t.Fatalf("fresh reload rewrote owner state=%+v err=%v", reloaded, err)
			}
		})
	}
}

func TestRuntimeStartupRecoveryRedrivesPersistedDefinitionClarificationHandoff(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeBoundDefinitionClarificationAggregate(t)
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, aggregate.Definition)); err != nil {
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
	stateRepository := runtimeStateRepository{hub: hub}
	if err := stateRepository.Save(ctx, aggregate, 0); err != nil {
		t.Fatal(err)
	}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(stateRepository); err != nil {
		t.Fatal(err)
	}
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(owner, hub, background)
	if err := module.recoverRuntimeStartup(ctx); err != nil {
		t.Fatalf("recover persisted governance lifecycle handoff: %v", err)
	}
	if background.asyncFunctionCalls != 1 || background.lastAsyncFunction == nil {
		t.Fatalf("startup governance lifecycle handoff schedules=%d", background.asyncFunctionCalls)
	}
	background.lastAsyncFunction()
	blocked, err := runs.GetByWorkspaceID(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || blocked.State != common.RunStateBlockedOnDecision || blocked.RuntimeState.Facts.Recovery.TerminalDeliveryAttempts != 0 {
		t.Fatalf("startup governance lifecycle handoff state=%+v err=%v", blocked, err)
	}
}

func TestRuntimeLifecycleOwnerDeliveryUsesPersistedLeaseWithoutTerminalBudget(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeBoundDefinitionClarificationAggregate(t)
	// A nonterminal lifecycle handoff must retain and renew its SM-4 successor
	// without consuming the terminal-only delivery ledger.
	aggregate.Facts.RecoveryPolicy.MaxTerminalDeliveryAttempts = 1
	terminalDeliveryAttempts := aggregate.Facts.Recovery.TerminalDeliveryAttempts
	repository := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(owner, hub, background)
	result, err := runtimeGovernedWaitDriveResult(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	deliveryFailure := runtimeLifecycleOwnerDeliveryFailure(result, common.ErrConflict)
	retry := module.handleScheduledRuntimeDriveFailure(ctx, scheduledRuntimeDriveRequest{
		workspaceID: aggregate.Facts.WorkspaceID, runID: aggregate.Facts.RunID, source: "test", driveKey: runtimeDriveKey(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID),
	}, deliveryFailure)
	if retry == nil || retry.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry {
		t.Fatalf("lifecycle retry=%+v", retry)
	}
	if !module.scheduleRuntimeRecoveryLease(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, *retry) {
		t.Fatal("lifecycle retry lease was not persisted")
	}
	persisted, err := owner.runtimeController.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || persisted.DriveLease == nil || persisted.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry ||
		persisted.DriveLease.FailureSignature != retry.FailureSignature || persisted.Facts.Recovery.TerminalDeliveryAttempts != terminalDeliveryAttempts {
		t.Fatalf("persisted lifecycle retry=%+v recovery=%+v err=%v", persisted.DriveLease, persisted.Facts.Recovery, err)
	}
	beforeRevision := persisted.StateRevision
	if !module.scheduleRuntimeRecoveryLease(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, *retry) {
		t.Fatal("duplicate lifecycle retry lease was not retained")
	}
	duplicate, err := owner.runtimeController.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || duplicate.StateRevision != beforeRevision || duplicate.Facts.Recovery.TerminalDeliveryAttempts != terminalDeliveryAttempts {
		t.Fatalf("duplicate lifecycle retry changed budget/revision: before=%d after=%+v err=%v", beforeRevision, duplicate, err)
	}

	// Claim the first attempt and fail the same lifecycle delivery again. The
	// exact running generation must be atomically replaced by a later scheduled
	// generation, not mistaken for a duplicate and then released.
	claimed, claimedOK, err := module.runtimeClaimDriveLease(ctx, duplicate.DriveLease.Clone(), time.Now().UTC().Add(2*time.Second).Format(time.RFC3339Nano))
	if err != nil || !claimedOK || claimed.State != runtimecontract.RuntimeDriveLeaseRunning {
		t.Fatalf("claim lifecycle retry=%+v claimed=%v err=%v", claimed, claimedOK, err)
	}
	secondRetry := module.handleScheduledRuntimeDriveFailure(ctx, scheduledRuntimeDriveRequest{
		workspaceID: aggregate.Facts.WorkspaceID, runID: aggregate.Facts.RunID, source: "test", driveKey: runtimeDriveKey(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID),
		leaseGeneration: claimed.RecoveryGeneration,
	}, deliveryFailure)
	if secondRetry == nil {
		t.Fatalf("second lifecycle retry=%+v", secondRetry)
	}
	key := runtimeDriveKey(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if !module.beginRuntimeDrive(key) {
		t.Fatal("second lifecycle retry did not acquire its drive gate")
	}
	module.finishScheduledRuntimeDrive(ctx, scheduledRuntimeDriveRequest{
		workspaceID: aggregate.Facts.WorkspaceID, runID: aggregate.Facts.RunID, source: "test", driveKey: key,
		leaseGeneration: claimed.RecoveryGeneration,
	}, &secondRetry)
	renewed, err := owner.runtimeController.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || renewed.DriveLease == nil || renewed.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		renewed.DriveLease.RecoveryGeneration != claimed.RecoveryGeneration+1 || renewed.DriveLease.DueAt <= claimed.DueAt ||
		renewed.Facts.Recovery.TerminalDeliveryAttempts != terminalDeliveryAttempts {
		t.Fatalf("renewed lifecycle retry=%+v recovery=%+v err=%v", renewed.DriveLease, renewed.Facts.Recovery, err)
	}
	reloadedController, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedController.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || reloaded.DriveLease == nil || reloaded.DriveLease.RecoveryGeneration != renewed.DriveLease.RecoveryGeneration || reloaded.Facts.Recovery.TerminalDeliveryAttempts != terminalDeliveryAttempts {
		t.Fatalf("fresh controller lost lifecycle successor=%+v recovery=%+v err=%v", reloaded.DriveLease, reloaded.Facts.Recovery, err)
	}
}

func TestRuntimeLifecycleLeasePersistedBehindDriveGateIsMountedByOwnerExit(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeAggregate(t, 1)
	repository := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(owner, hub, background)
	key := runtimeDriveKey(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if !module.beginRuntimeDrive(key) {
		t.Fatal("old drive did not acquire gate")
	}
	handoff := runtimeGovernanceLifecycleHandoff{
		Kind: runtimeGovernanceLifecycleContinuationDecision, AuthoritySnapshotID: "authority-gate",
		DefinitionRevision: aggregate.Facts.DefinitionRevision, EvidenceRevision: aggregate.Facts.EvidenceRevision,
		DispatchID: "dispatch-gate", TicketID: "ticket-gate", ResolutionID: "resolution-gate",
	}
	deliveryErr := &runtimeLifecycleOwnerDeliveryError{handoff: handoff, cause: common.ErrConflict}
	if err := module.scheduleRuntimeLifecycleOwnerDeliveryRetry(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "governance_resolution", deliveryErr); err != nil {
		t.Fatalf("persist lifecycle lease behind old gate: %v", err)
	}
	if background.asyncFunctionCalls != 0 {
		t.Fatalf("timer bypassed live drive gate: calls=%d", background.asyncFunctionCalls)
	}
	persisted, err := owner.runtimeController.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || persisted.DriveLease == nil || persisted.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		persisted.Facts.Recovery.TerminalDeliveryAttempts != 0 {
		t.Fatalf("persisted gate handoff lease=%+v recovery=%+v err=%v", persisted.DriveLease, persisted.Facts.Recovery, err)
	}
	leaseRevision := persisted.StateRevision
	leaseGeneration := persisted.DriveLease.RecoveryGeneration

	// Model the old drive returning success. Its one exit path must retain the
	// gate and mount the exact persisted generation before it can become free.
	var retry *runtimeRecoveryLeaseRequest
	module.finishScheduledRuntimeDrive(ctx, scheduledRuntimeDriveRequest{
		workspaceID: aggregate.Facts.WorkspaceID, runID: aggregate.Facts.RunID, source: "old_drive_success", driveKey: key,
	}, &retry)
	if background.asyncFunctionCalls != 1 || background.lastAsyncFunction == nil {
		t.Fatalf("gate owner did not mount persisted lease: calls=%d", background.asyncFunctionCalls)
	}
	if module.beginRuntimeDrive(key) {
		t.Fatal("persisted lease timer lost the run-scoped drive gate")
	}

	// Duplicate and stale mount attempts are pure reads while the exact timer
	// owns the gate; neither creates a second timer nor consumes terminal budget.
	if err := module.scheduleRuntimeLifecycleOwnerDeliveryRetry(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "governance_resolution_duplicate", deliveryErr); err != nil {
		t.Fatalf("duplicate lifecycle lease: %v", err)
	}
	staleLease := persisted.DriveLease.Clone()
	if staleLease.RecoveryGeneration > 1 {
		staleLease.RecoveryGeneration--
	}
	if module.retainRuntimeDriveGateForPersistedLease(key, staleLease) {
		t.Fatal("stale lease generation took an occupied drive gate")
	}
	afterDuplicate, err := owner.runtimeController.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || afterDuplicate.StateRevision != leaseRevision || afterDuplicate.DriveLease == nil ||
		afterDuplicate.DriveLease.RecoveryGeneration != leaseGeneration || background.asyncFunctionCalls != 1 ||
		afterDuplicate.Facts.Recovery.TerminalDeliveryAttempts != 0 {
		t.Fatalf("duplicate/stale handoff changed lease=%+v calls=%d recovery=%+v err=%v", afterDuplicate.DriveLease, background.asyncFunctionCalls, afterDuplicate.Facts.Recovery, err)
	}

	// Stop discards only process-local gate/timer hints. The scheduled lease is
	// still durable for the already-covered fresh-process inventory recovery.
	if err := module.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if module.drivingRuns != nil || module.persistedLeaseMounts != nil || module.retryLeaseTimers != nil {
		t.Fatalf("Stop retained process-local drive state: driving=%v mounts=%v timers=%v", module.drivingRuns, module.persistedLeaseMounts, module.retryLeaseTimers)
	}
	restarted, err := repository.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || restarted.DriveLease == nil || restarted.DriveLease.RecoveryGeneration != leaseGeneration || restarted.Facts.Recovery.TerminalDeliveryAttempts != 0 {
		t.Fatalf("Stop lost persisted lifecycle successor=%+v recovery=%+v err=%v", restarted.DriveLease, restarted.Facts.Recovery, err)
	}
}

type blockFirstScheduledLeaseSaveRepository struct {
	delegate *inMemoryRuntimeStateRepository
	entered  chan struct{}
	release  chan struct{}
	blocked  bool
}

func (r *blockFirstScheduledLeaseSaveRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *blockFirstScheduledLeaseSaveRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	if !r.blocked && aggregate.DriveLease != nil && aggregate.DriveLease.State == runtimecontract.RuntimeDriveLeaseScheduled {
		r.blocked = true
		close(r.entered)
		<-r.release
	}
	return r.delegate.Save(ctx, aggregate, expectedRevision)
}

func TestRuntimeLifecycleLeaseSubmitterTakesGateWhenOldOwnerExitsBeforeLeaseCAS(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeAggregate(t, 1)
	baseRepository := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
	repository := &blockFirstScheduledLeaseSaveRepository{
		delegate: baseRepository, entered: make(chan struct{}), release: make(chan struct{}),
	}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(owner, hub, background)
	key := runtimeDriveKey(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if !module.beginRuntimeDrive(key) {
		t.Fatal("old drive did not acquire gate")
	}
	handoff := runtimeGovernanceLifecycleHandoff{
		Kind: runtimeGovernanceLifecycleContinuationDecision, AuthoritySnapshotID: "authority-cas-race",
		DefinitionRevision: aggregate.Facts.DefinitionRevision, EvidenceRevision: aggregate.Facts.EvidenceRevision,
		DispatchID: "dispatch-cas-race", TicketID: "ticket-cas-race", ResolutionID: "resolution-cas-race",
	}
	deliveryErr := &runtimeLifecycleOwnerDeliveryError{handoff: handoff, cause: common.ErrConflict}
	done := make(chan error, 1)
	go func() {
		done <- module.scheduleRuntimeLifecycleOwnerDeliveryRetry(
			ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "governance_resolution_cas_race", deliveryErr,
		)
	}()
	<-repository.entered
	var retry *runtimeRecoveryLeaseRequest
	module.finishScheduledRuntimeDrive(ctx, scheduledRuntimeDriveRequest{
		workspaceID: aggregate.Facts.WorkspaceID, runID: aggregate.Facts.RunID, source: "old_drive_success_before_lease_cas", driveKey: key,
	}, &retry)
	close(repository.release)
	if err := <-done; err != nil {
		t.Fatalf("lifecycle submitter did not take released gate: %v", err)
	}
	if background.asyncFunctionCalls != 1 || background.lastAsyncFunction == nil {
		t.Fatalf("lifecycle submitter did not mount its persisted lease: calls=%d", background.asyncFunctionCalls)
	}
	if module.beginRuntimeDrive(key) {
		t.Fatal("lifecycle lease timer did not retain the gate")
	}
	persisted, err := owner.runtimeController.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || persisted.DriveLease == nil || persisted.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		persisted.Facts.Recovery.TerminalDeliveryAttempts != 0 {
		t.Fatalf("CAS-race successor=%+v recovery=%+v err=%v", persisted.DriveLease, persisted.Facts.Recovery, err)
	}
}

func TestRuntimeLifecycleLeaseWinsWhenOldDriveAlsoRequestsRetry(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeAggregate(t, 1)
	repository := &inMemoryRuntimeStateRepository{aggregate: &aggregate}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(owner, hub, background)
	key := runtimeDriveKey(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if !module.beginRuntimeDrive(key) {
		t.Fatal("old drive did not acquire gate")
	}
	handoff := runtimeGovernanceLifecycleHandoff{
		Kind: runtimeGovernanceLifecycleContinuationDecision, AuthoritySnapshotID: "authority-retry-race",
		DefinitionRevision: aggregate.Facts.DefinitionRevision, EvidenceRevision: aggregate.Facts.EvidenceRevision,
		DispatchID: "dispatch-retry-race", TicketID: "ticket-retry-race", ResolutionID: "resolution-retry-race",
	}
	deliveryErr := &runtimeLifecycleOwnerDeliveryError{handoff: handoff, cause: common.ErrConflict}
	if err := module.scheduleRuntimeLifecycleOwnerDeliveryRetry(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "governance_resolution_retry_race", deliveryErr); err != nil {
		t.Fatal(err)
	}
	beforeFinish, err := owner.runtimeController.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || beforeFinish.DriveLease == nil {
		t.Fatalf("lifecycle lease before old retry=%+v err=%v", beforeFinish.DriveLease, err)
	}
	lifecycleSignature := beforeFinish.DriveLease.FailureSignature
	lifecycleGeneration := beforeFinish.DriveLease.RecoveryGeneration
	oldDriveRetry := &runtimeRecoveryLeaseRequest{
		Reason: runtimecontract.RuntimeDriveLeaseReasonCASReload, FailureSignature: "old-drive/cas-retry",
		Delay: time.Second, Source: "old_drive_retry",
	}
	module.finishScheduledRuntimeDrive(ctx, scheduledRuntimeDriveRequest{
		workspaceID: aggregate.Facts.WorkspaceID, runID: aggregate.Facts.RunID, source: "old_drive_retry", driveKey: key,
	}, &oldDriveRetry)
	persisted, err := owner.runtimeController.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || persisted.DriveLease == nil || persisted.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		persisted.DriveLease.FailureSignature != lifecycleSignature || persisted.DriveLease.RecoveryGeneration != lifecycleGeneration ||
		background.asyncFunctionCalls != 1 || persisted.Facts.Recovery.TerminalDeliveryAttempts != 0 {
		t.Fatalf("old retry replaced or lost lifecycle successor lease=%+v calls=%d recovery=%+v err=%v", persisted.DriveLease, background.asyncFunctionCalls, persisted.Facts.Recovery, err)
	}
	if module.beginRuntimeDrive(key) {
		t.Fatal("mounted lifecycle successor released the gate")
	}
}

func TestRuntimeDriveLeaseTimerGateExitMountsOverwritingGovernanceSuccessor(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		nextSignature string
	}{
		{name: "same stable signature newer generation", nextSignature: "lifecycle-owner-delivery/same-resolution"},
		{name: "different resolution signature", nextSignature: "lifecycle-owner-delivery/different-resolution"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			initial := runtimeAggregate(t, 1)
			repository := &inMemoryRuntimeStateRepository{aggregate: &initial}
			owner := newRuntimeTestOwner()
			if err := owner.ConfigureRuntimeRepository(repository); err != nil {
				t.Fatal(err)
			}
			hub := magicevent.NewHub(8)
			t.Cleanup(func() { hub.Terminate(ctx) })
			background := &recordingBackgroundRoutine{}
			module := newTestBiz(owner, hub, background)
			oldSignature := "lifecycle-owner-delivery/same-resolution"
			oldLease, _, err := module.runtimeScheduleDriveLease(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, runtimecontract.RuntimeDriveLease{
				WorkspaceID: initial.Facts.WorkspaceID, RunID: initial.Facts.RunID,
				Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: oldSignature,
				DueAt: time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
			}, 0)
			if err != nil {
				t.Fatal(err)
			}
			key := runtimeDriveKey(initial.Facts.WorkspaceID, initial.Facts.RunID)
			if !module.beginRuntimeDrive(key) || !module.enqueueRuntimeDriveLeaseTimer(ctx, oldLease, "old_resolution_timer") {
				t.Fatal("old persisted timer was not mounted behind the run gate")
			}
			if background.asyncFunctionCalls != 1 || background.lastAsyncFunction == nil {
				t.Fatalf("old timer schedules=%d", background.asyncFunctionCalls)
			}
			oldTimer := background.lastAsyncFunction

			// Model the exact post-CAS cutpoint: governance resolution has already
			// replaced the old scheduled generation, but its delivery path cannot
			// mount a timer because the old timer still owns the run gate.
			persisted, err := repository.Load(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID)
			if err != nil {
				t.Fatal(err)
			}
			expectedRevision := persisted.StateRevision
			persisted.StateRevision++
			successor := oldLease.Clone()
			successor.AggregateRevision = persisted.StateRevision
			successor.RecoveryGeneration++
			successor.FailureSignature = testCase.nextSignature
			successor.DueAt = time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
			persisted.DriveLease = &successor
			if err := repository.Save(ctx, persisted, expectedRevision); err != nil {
				t.Fatal(err)
			}
			ownsGate, err := module.retainRuntimeLifecycleOwnerDeliveryLeaseGate(successor, testCase.nextSignature)
			if err != nil || ownsGate {
				t.Fatalf("new successor bypassed old timer gate: owns=%v err=%v", ownsGate, err)
			}

			oldTimer()
			module.driveMu.Lock()
			mountedRegistration := module.retryLeaseTimers[key]
			_, latchRemains := module.persistedLeaseMounts[key]
			_, gateRetained := module.drivingRuns[key]
			module.driveMu.Unlock()
			current, err := repository.Load(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID)
			if err != nil || background.asyncFunctionCalls != 2 || background.lastAsyncFunction == nil ||
				mountedRegistration.Generation != successor.RecoveryGeneration || mountedRegistration.Epoch == 0 || latchRemains || !gateRetained || current.DriveLease == nil ||
				!runtimecontract.SameRuntimeDriveLeaseIdentity(*current.DriveLease, successor) {
				t.Fatalf("old timer did not atomically mount exact successor: calls=%d mounted=%+v latch=%v gate=%v persisted=%+v err=%v", background.asyncFunctionCalls, mountedRegistration, latchRemains, gateRetained, current.DriveLease, err)
			}
		})
	}
}

func TestRuntimeDriveLeaseRenewalCASFailureCannotReleaseOrBypassRunningAuthority(t *testing.T) {
	ctx := context.Background()
	initial := runtimeAggregate(t, 1)
	baseRepository := &inMemoryRuntimeStateRepository{aggregate: &initial}
	faultRepository := &failRuntimeLeaseRenewalRepository{delegate: baseRepository, remainingRenewalFailures: 2}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(faultRepository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(owner, hub, background)
	scheduled, _, err := module.runtimeScheduleDriveLease(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: initial.Facts.WorkspaceID, RunID: initial.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "running-authority",
		DueAt: time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	running, claimed, err := module.runtimeClaimDriveLease(ctx, scheduled, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil || !claimed || running.State != runtimecontract.RuntimeDriveLeaseRunning {
		t.Fatalf("claim running authority=%+v claimed=%v err=%v", running, claimed, err)
	}
	key := runtimeDriveKey(initial.Facts.WorkspaceID, initial.Facts.RunID)
	if !module.beginRuntimeDrive(key) {
		t.Fatal("running generation did not own local gate")
	}
	retry := &runtimeRecoveryLeaseRequest{
		Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "replacement-after-delivery-failure",
		Delay: time.Second, Source: "renewal_fault",
	}
	module.finishScheduledRuntimeDrive(ctx, scheduledRuntimeDriveRequest{
		workspaceID: initial.Facts.WorkspaceID, runID: initial.Facts.RunID, source: "renewal_fault",
		driveKey: key, leaseGeneration: running.RecoveryGeneration,
	}, &retry)
	afterFailures, err := baseRepository.Load(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID)
	module.driveMu.Lock()
	_, failedGateRetained := module.drivingRuns[key]
	module.driveMu.Unlock()
	if err != nil || faultRepository.renewalFailures != 2 || afterFailures.DriveLease == nil ||
		afterFailures.DriveLease.State != runtimecontract.RuntimeDriveLeaseRunning ||
		afterFailures.DriveLease.RecoveryGeneration != running.RecoveryGeneration || !failedGateRetained || background.asyncFunctionCalls != 0 {
		t.Fatalf("renewal CAS failure lost running authority/gate: failures=%d lease=%+v gate=%v timers=%d err=%v", faultRepository.renewalFailures, afterFailures.DriveLease, failedGateRetained, background.asyncFunctionCalls, err)
	}

	// A normal Start/Resume cannot acquire a second process gate or enter
	// DriveRuntime while the exact persisted running authority is parked.
	if err := module.scheduleRuntimeDrive(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, "normal_ingress_after_renewal_failure"); err != nil {
		t.Fatal(err)
	}
	if background.asyncFunctionCalls != 0 {
		t.Fatalf("normal ingress established a second scheduler: calls=%d", background.asyncFunctionCalls)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	preserved, err := baseRepository.Load(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID)
	if err != nil || preserved.DriveLease == nil || preserved.DriveLease.State != runtimecontract.RuntimeDriveLeaseRunning ||
		!runtimecontract.SameRuntimeDriveLeaseIdentity(*preserved.DriveLease, running) {
		t.Fatalf("Stop lost recovered durable authority: lease=%+v err=%v", preserved.DriveLease, err)
	}

	freshOwner := newRuntimeTestOwner()
	if err := freshOwner.ConfigureRuntimeRepository(faultRepository); err != nil {
		t.Fatal(err)
	}
	freshBackground := &recordingBackgroundRoutine{}
	freshModule := newTestBiz(freshOwner, hub, freshBackground)
	freshLease, err := freshModule.recoverRuntimeDriveLease(ctx, preserved, "fresh_restart")
	if err != nil || !freshModule.beginRuntimeDrive(key) || !freshModule.enqueueRuntimeDriveLeaseTimer(ctx, freshLease, "fresh_restart") {
		t.Fatalf("fresh restart did not mount preserved authority: lease=%+v err=%v", freshLease, err)
	}
	if freshLease.State != runtimecontract.RuntimeDriveLeaseScheduled || freshLease.RecoveryGeneration != running.RecoveryGeneration+1 {
		t.Fatalf("fresh restart did not recover exact running generation: lease=%+v", freshLease)
	}
	if err := freshModule.scheduleRuntimeDrive(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, "duplicate_fresh_resume"); err != nil {
		t.Fatal(err)
	}
	if freshBackground.asyncFunctionCalls != 1 {
		t.Fatalf("fresh restart admitted a second scheduler: calls=%d", freshBackground.asyncFunctionCalls)
	}
}

func TestRuntimeDriveLeaseTimerStopCancelsLocalWaitAndPreservesLease(t *testing.T) {
	ctx := context.Background()
	initial := runtimeAggregate(t, 1)
	repository := &inMemoryRuntimeStateRepository{aggregate: &initial}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	background := magictask.NewBackgroundRoutine(2)
	module := newTestBiz(owner, hub, background)
	lease, _, err := module.runtimeScheduleDriveLease(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: initial.Facts.WorkspaceID, RunID: initial.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "stop-cancel",
		DueAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano),
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	key := runtimeDriveKey(initial.Facts.WorkspaceID, initial.Facts.RunID)
	if !module.beginRuntimeDrive(key) || !module.enqueueRuntimeDriveLeaseTimer(ctx, lease, "stop_cancel") {
		t.Fatal("long-lived persisted timer was not mounted")
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if !background.Shutdown(shutdownCtx) {
		t.Fatal("Stop did not cancel the long-lived lease timer before background drain")
	}
	hub.Terminate(ctx)
	persisted, err := repository.Load(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID)
	if err != nil || persisted.DriveLease == nil || !runtimecontract.SameRuntimeDriveLeaseIdentity(*persisted.DriveLease, lease) {
		t.Fatalf("Stop changed persisted retry authority: lease=%+v err=%v", persisted.DriveLease, err)
	}
}

func TestRuntimeDriveLeaseTimerBlockingSubmissionCannotClaimAfterStopEpoch(t *testing.T) {
	ctx := context.Background()
	initial := runtimeAggregate(t, 1)
	repository := &inMemoryRuntimeStateRepository{aggregate: &initial}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	background := &blockingRuntimeTimerSubmissionBackground{
		entered: make(chan struct{}, 1), release: make(chan struct{}),
	}
	module := newTestBiz(owner, hub, background)
	lease, _, err := module.runtimeScheduleDriveLease(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: initial.Facts.WorkspaceID, RunID: initial.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "stop-blocked-submission",
		DueAt: time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	key := runtimeDriveKey(initial.Facts.WorkspaceID, initial.Facts.RunID)
	if !module.beginRuntimeDrive(key) {
		t.Fatal("blocked-submission timer did not acquire gate")
	}
	enqueueDone := make(chan bool, 1)
	go func() { enqueueDone <- module.enqueueRuntimeDriveLeaseTimer(ctx, lease, "stop_blocked_submission") }()
	select {
	case <-background.entered:
	case <-time.After(time.Second):
		t.Fatal("timer did not block in BackgroundRoutine submission")
	}

	// Keep Stop between its closing/cancel publication and dependency teardown.
	// A queued, already-due callback must observe the captured epoch cancellation
	// and return without waiting for this lifecycle reader or touching the lease.
	module.lifecycleMu.RLock()
	stopDone := make(chan error, 1)
	go func() { stopDone <- module.Stop(ctx) }()
	waitRuntimeLeaseTimerClosing(t, module)
	close(background.release)
	select {
	case accepted := <-enqueueDone:
		if !accepted {
			t.Fatal("pre-Stop timer submission was not accepted after queue release")
		}
	case <-time.After(time.Second):
		t.Fatal("blocked timer submission did not return")
	}
	calls, callback := background.snapshot()
	if calls != 1 || callback == nil {
		t.Fatalf("blocked timer submission calls=%d callback=%v", calls, callback != nil)
	}
	callbackDone := make(chan struct{})
	go func() {
		callback()
		close(callbackDone)
	}()
	select {
	case <-callbackDone:
	case <-time.After(time.Second):
		t.Fatal("canceled already-due callback attempted to enter the owner claim lane")
	}
	module.lifecycleMu.RUnlock()
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not finish after blocking lifecycle reader released")
	}

	persisted, err := repository.Load(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID)
	if err != nil || persisted.DriveLease == nil || !runtimecontract.SameRuntimeDriveLeaseIdentity(*persisted.DriveLease, lease) {
		t.Fatalf("canceled queued callback changed persisted lease: lease=%+v err=%v", persisted.DriveLease, err)
	}
	module.driveMu.Lock()
	closing := module.runtimeLeaseTimerClosing
	localStateEmpty := module.drivingRuns == nil && module.persistedLeaseMounts == nil && module.retryLeaseTimers == nil &&
		module.runtimeLeaseTimerCtx == nil && module.runtimeLeaseTimerCancel == nil
	module.driveMu.Unlock()
	if !closing || !localStateEmpty {
		t.Fatalf("Stop retained blocked-submission timer state: closing=%v empty=%v", closing, localStateEmpty)
	}
}

func TestRuntimeDriveLeaseTimerStopEpochRejectsDeferredSuccessorAfterClaimReturn(t *testing.T) {
	ctx := context.Background()
	initial := runtimeAggregate(t, 1)
	baseRepository := &inMemoryRuntimeStateRepository{aggregate: &initial}
	blockingRepository := &blockingRuntimeLeaseClaimLoadRepository{
		delegate: baseRepository, entered: make(chan struct{}), release: make(chan struct{}),
	}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(blockingRepository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(owner, hub, background)
	oldLease, _, err := module.runtimeScheduleDriveLease(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: initial.Facts.WorkspaceID, RunID: initial.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "stop-epoch-successor",
		DueAt: time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	key := runtimeDriveKey(initial.Facts.WorkspaceID, initial.Facts.RunID)
	if !module.beginRuntimeDrive(key) || !module.enqueueRuntimeDriveLeaseTimer(ctx, oldLease, "stop_epoch_old_timer") {
		t.Fatal("old timer was not mounted")
	}
	if background.asyncFunctionCalls != 1 || background.lastAsyncFunction == nil {
		t.Fatalf("old timer schedules=%d", background.asyncFunctionCalls)
	}
	oldTimer := background.lastAsyncFunction

	persisted, err := baseRepository.Load(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	expectedRevision := persisted.StateRevision
	persisted.StateRevision++
	successor := oldLease.Clone()
	successor.AggregateRevision = persisted.StateRevision
	successor.RecoveryGeneration++
	successor.DueAt = time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	persisted.DriveLease = &successor
	if err := baseRepository.Save(ctx, persisted, expectedRevision); err != nil {
		t.Fatal(err)
	}
	ownsGate, err := module.retainRuntimeLifecycleOwnerDeliveryLeaseGate(successor, successor.FailureSignature)
	if err != nil || ownsGate {
		t.Fatalf("successor bypassed old timer gate: owns=%v err=%v", ownsGate, err)
	}

	blockingRepository.block = true
	timerDone := make(chan struct{})
	go func() {
		oldTimer()
		close(timerDone)
	}()
	select {
	case <-blockingRepository.entered:
	case <-time.After(time.Second):
		t.Fatal("timer did not reach blocked claim load")
	}
	stopDone := make(chan error, 1)
	go func() { stopDone <- module.Stop(ctx) }()
	waitRuntimeLeaseTimerClosing(t, module)
	close(blockingRepository.release)
	select {
	case <-timerDone:
	case <-time.After(time.Second):
		t.Fatal("captured timer did not exit after claim release")
	}
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not finish after blocked claim returned")
	}
	current, err := baseRepository.Load(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID)
	if err != nil || current.DriveLease == nil || !runtimecontract.SameRuntimeDriveLeaseIdentity(*current.DriveLease, successor) ||
		background.asyncFunctionCalls != 1 {
		t.Fatalf("Stop epoch mounted a zombie successor: lease=%+v calls=%d err=%v", current.DriveLease, background.asyncFunctionCalls, err)
	}
	module.driveMu.Lock()
	closing := module.runtimeLeaseTimerClosing
	localStateEmpty := module.drivingRuns == nil && module.persistedLeaseMounts == nil && module.retryLeaseTimers == nil &&
		module.runtimeLeaseTimerCtx == nil && module.runtimeLeaseTimerCancel == nil
	module.driveMu.Unlock()
	if !closing || !localStateEmpty {
		t.Fatalf("Stop retained timer lifecycle state: closing=%v empty=%v", closing, localStateEmpty)
	}
}

func TestRuntimeLifecycleHandlerCannotCreateLeaseTimerAfterStopCancelPoint(t *testing.T) {
	ctx := context.Background()
	initial := runtimeAggregate(t, 1)
	repository := &inMemoryRuntimeStateRepository{aggregate: &initial}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(owner, hub, background)
	lease, _, err := module.runtimeScheduleDriveLease(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: initial.Facts.WorkspaceID, RunID: initial.Facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "handler-stop-epoch",
		DueAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano),
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	handlerEntered := make(chan struct{})
	handlerRelease := make(chan struct{})
	handlerDone := make(chan error, 1)
	go func() {
		module.lifecycleMu.RLock()
		close(handlerEntered)
		<-handlerRelease
		err := module.mountPersistedRuntimeLifecycleOwnerDeliveryLease(ctx, lease, "blocked_handler", lease.FailureSignature)
		module.lifecycleMu.RUnlock()
		handlerDone <- err
	}()
	<-handlerEntered
	stopDone := make(chan error, 1)
	go func() { stopDone <- module.Stop(ctx) }()
	waitRuntimeLeaseTimerClosing(t, module)
	close(handlerRelease)
	select {
	case err := <-handlerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked lifecycle handler did not finish")
	}
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not finish after lifecycle handler released")
	}
	persisted, err := repository.Load(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID)
	if err != nil || persisted.DriveLease == nil || !runtimecontract.SameRuntimeDriveLeaseIdentity(*persisted.DriveLease, lease) ||
		background.asyncFunctionCalls != 0 {
		t.Fatalf("handler created timer after Stop cancel point: lease=%+v calls=%d err=%v", persisted.DriveLease, background.asyncFunctionCalls, err)
	}
	module.driveMu.Lock()
	localStateEmpty := module.drivingRuns == nil && module.persistedLeaseMounts == nil && module.retryLeaseTimers == nil &&
		module.runtimeLeaseTimerCtx == nil && module.runtimeLeaseTimerCancel == nil
	module.driveMu.Unlock()
	if !localStateEmpty {
		t.Fatal("Stop retained process-local lease timer state")
	}
}

func waitRuntimeLeaseTimerClosing(t *testing.T, module *Biz) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		module.driveMu.Lock()
		closing := module.runtimeLeaseTimerClosing
		module.driveMu.Unlock()
		if closing {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Stop did not publish the lease timer closing boundary")
}

func TestRuntimeGovernanceContinuationSuccessHandoffRetainsDurableSingleSuccessor(t *testing.T) {
	ctx := context.Background()
	baseRuns := memory.NewRunRepository()
	hub := magicevent.NewHub(32)
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: baseRuns, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = runOwner.Stop(ctx)
		hub.Terminate(ctx)
	})

	definition := runtimeDefinition()
	definition.CapabilityContract = append(definition.CapabilityContract, "decision/resolution")
	runInput := runtimeRunForTest(common.RunStateActive, definition)
	if err := baseRuns.Create(ctx, runInput); err != nil {
		t.Fatal(err)
	}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(owner, hub, background)
	facts, err := runInput.RuntimeBootstrap.InitialFacts(runInput.WorkspaceID, runInput.ID)
	if err != nil {
		t.Fatal(err)
	}
	facts.Capabilities.GovernanceKinds = []string{"decision/resolution"}
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileEdit}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceDependency, Signature: "success-handoff", Message: "governance required",
		GovernanceDependency: &runtimecontract.ExternalDependency{Kind: "external-approval", SubjectRef: "subject-success-handoff", Reason: "governance required"},
	}
	initialized, err := owner.InitializeRuntime(ctx, facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(initialized.Facts.Resources))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := runtimeTestControllerSealGovernanceDispatch(owner.runtimeController, ctx, facts.WorkspaceID, facts.RunID, manifest, runtimecontract.RuntimeCandidate{
		Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: *facts.Failure.GovernanceDependency},
	})
	if err != nil || sealed.ActiveDispatch == nil {
		t.Fatalf("seal governance dispatch=%+v err=%v", sealed, err)
	}
	request := runtimecontract.RuntimeGovernanceRequestRecord{
		TicketID: "ticket-success-handoff", AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID: sealed.ActiveDispatch.ID, DependencyKind: facts.Failure.GovernanceDependency.Kind,
		SubjectRef:        facts.Failure.GovernanceDependency.SubjectRef,
		AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(runtimecontract.ExternalDependency{}.AuthorizationContext),
	}
	boundEvent, err := owner.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:      runtime.RuntimeInternalEventGovernanceRequestObserved,
		Scope:     runtime.RuntimeInternalEventScope{WorkspaceID: facts.WorkspaceID, RunID: facts.RunID},
		Authority: &runtime.RuntimeInternalAuthorityPayload{GovernanceRequest: &request},
	})
	if err != nil {
		t.Fatal(err)
	}
	boundRecord := runtimeStateRecordFromAggregate(boundEvent.Aggregate)
	waitHandoff, governed, err := runtimeGovernanceLifecycleHandoffFromState(&boundRecord)
	if err != nil || !governed || waitHandoff.Kind != runtimeGovernanceLifecycleWaitDecision {
		t.Fatalf("governance wait handoff=%+v governed=%v err=%v", waitHandoff, governed, err)
	}
	if err := module.applyRunGovernanceWait(
		ctx, facts.WorkspaceID, facts.RunID, waitHandoff.AuthoritySnapshotID, waitHandoff.DefinitionRevision,
		waitHandoff.EvidenceRevision, waitHandoff.DispatchID, waitHandoff.TicketID, waitHandoff.ApprovalID,
		boundEvent.Aggregate.StateRevision,
	); err != nil {
		t.Fatalf("apply governance wait: %v", err)
	}

	// This gate models the old SM-4 worker after Run acknowledged the wait but
	// before finishScheduledRuntimeDrive released its gate.
	driveKey := runtimeDriveKey(facts.WorkspaceID, facts.RunID)
	if !module.beginRuntimeDrive(driveKey) {
		t.Fatal("old drive did not acquire gate")
	}
	resolution := runtimecontract.RuntimeGovernanceResolution{
		ResolutionID: "resolution-success-handoff", TicketID: request.TicketID,
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
		AuthoritySnapshotID: request.AuthoritySnapshotID, DispatchID: request.DispatchID,
		DependencyKind: request.DependencyKind, SubjectRef: request.SubjectRef,
		Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "continue with durable successor",
	}
	event := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution,
		decisioncommon.ModuleID, ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.CommitRuntimeGovernanceResolutionCommand{Resolution: resolution},
	)
	result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
	module.handleRuntimeGovernanceResolution(event, result)
	data, magicErr := result.Get()
	if magicErr != nil {
		t.Fatalf("commit exact governance continuation: %v", magicErr)
	}
	response, ok := data.(orchestratorevents.CommitRuntimeGovernanceResolutionResult)
	if !ok || response.Disposition != orchestratorevents.RuntimeGovernanceResolutionDispositionCommitted {
		t.Fatalf("governance continuation response=%+v", data)
	}
	continued, err := baseRuns.GetByWorkspaceID(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || continued.State != common.RunStateActive || continued.RuntimeLifecycleFence == nil ||
		continued.RuntimeLifecycleFence.GovernanceResolutionID != resolution.ResolutionID {
		t.Fatalf("Run continuation=%+v err=%v", continued, err)
	}
	continuedRevision := continued.Revision
	persisted, err := owner.runtimeController.Load(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || persisted.DriveLease == nil || persisted.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		persisted.DriveLease.AggregateRevision != persisted.StateRevision || persisted.Facts.Recovery.TerminalDeliveryAttempts != 0 {
		t.Fatalf("atomic successor lease=%+v recovery=%+v err=%v", persisted.DriveLease, persisted.Facts.Recovery, err)
	}
	lease := persisted.DriveLease.Clone()
	if background.asyncFunctionCalls != 0 {
		t.Fatalf("continuation timer bypassed old gate: calls=%d", background.asyncFunctionCalls)
	}
	// The active notification cannot establish a second normal drive while the
	// old worker owns the gate.
	if err := module.scheduleRuntimeDrive(ctx, facts.WorkspaceID, facts.RunID, "run_state_active_test"); err != nil {
		t.Fatal(err)
	}
	if background.asyncFunctionCalls != 0 {
		t.Fatalf("active notification scheduled a second drive: calls=%d", background.asyncFunctionCalls)
	}

	// Response-loss duplicate proves the exact Run-owned resolution and joins
	// the same persisted generation without rewriting either owner.
	duplicateResult := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
	module.handleRuntimeGovernanceResolution(event, duplicateResult)
	duplicateData, duplicateMagicErr := duplicateResult.Get()
	duplicateResponse, duplicateOK := duplicateData.(orchestratorevents.CommitRuntimeGovernanceResolutionResult)
	if duplicateMagicErr != nil || !duplicateOK || duplicateResponse.Disposition != orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate {
		t.Fatalf("duplicate continuation response=%+v err=%v", duplicateData, duplicateMagicErr)
	}
	duplicateRun, err := baseRuns.GetByWorkspaceID(ctx, facts.WorkspaceID, facts.RunID)
	duplicateAggregate, loadErr := owner.runtimeController.Load(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || loadErr != nil || duplicateRun.Revision != continuedRevision || len(duplicateAggregate.GovernanceResolutions) != 1 ||
		duplicateAggregate.DriveLease == nil || !runtimecontract.SameRuntimeDriveLeaseIdentity(*duplicateAggregate.DriveLease, lease) ||
		background.asyncFunctionCalls != 0 {
		t.Fatalf("duplicate rewrote successor: run=%+v lease=%+v calls=%d err=%v loadErr=%v", duplicateRun, duplicateAggregate.DriveLease, background.asyncFunctionCalls, err, loadErr)
	}

	// A wrong resolution cannot pass the exact continuation fence or replace the
	// successor after the governed dispatch has already committed.
	wrong := resolution
	wrong.ResolutionID = "resolution-wrong-handoff"
	wrongEvent := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution,
		decisioncommon.ModuleID, ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.CommitRuntimeGovernanceResolutionCommand{Resolution: wrong},
	)
	wrongResult := magicevent.NewResult(wrongEvent.ID(), wrongEvent.Source(), wrongEvent.Destination())
	module.handleRuntimeGovernanceResolution(wrongEvent, wrongResult)
	wrongData, wrongErr := wrongResult.Get()
	wrongResponse, wrongOK := wrongData.(orchestratorevents.CommitRuntimeGovernanceResolutionResult)
	if wrongErr != nil || !wrongOK || wrongResponse.Disposition != orchestratorevents.RuntimeGovernanceResolutionDispositionStale {
		t.Fatalf("wrong resolution did not fail closed as stale: response=%+v err=%v", wrongData, wrongErr)
	}
	afterWrong, err := owner.runtimeController.Load(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || afterWrong.DriveLease == nil || afterWrong.DriveLease.FailureSignature != lease.FailureSignature ||
		afterWrong.DriveLease.RecoveryGeneration != lease.RecoveryGeneration+1 || afterWrong.DriveLease.AggregateRevision != afterWrong.StateRevision {
		t.Fatalf("wrong resolution changed successor=%+v err=%v", afterWrong.DriveLease, err)
	}
	lease = afterWrong.DriveLease.Clone()

	var retry *runtimeRecoveryLeaseRequest
	module.finishScheduledRuntimeDrive(ctx, scheduledRuntimeDriveRequest{
		workspaceID: facts.WorkspaceID, runID: facts.RunID, source: "old_wait_ack_success", driveKey: driveKey,
	}, &retry)
	if background.asyncFunctionCalls != 1 || background.lastAsyncFunction == nil {
		t.Fatalf("old gate exit did not mount the durable successor: calls=%d", background.asyncFunctionCalls)
	}
	if module.beginRuntimeDrive(driveKey) {
		t.Fatal("mounted successor did not retain the run gate")
	}

	if err := module.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	restartSnapshot, err := baseRuns.GetByWorkspaceID(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || restartSnapshot.RuntimeState == nil || restartSnapshot.RuntimeState.DriveLease == nil ||
		!runtimecontract.SameRuntimeDriveLeaseIdentity(*restartSnapshot.RuntimeState.DriveLease, lease) {
		t.Fatalf("Stop lost durable successor=%+v err=%v", restartSnapshot, err)
	}

	restartedOwner := newRuntimeTestOwner()
	if err := restartedOwner.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	restartedBackground := &recordingBackgroundRoutine{}
	restartedModule := newTestBiz(restartedOwner, hub, restartedBackground)
	if err := restartedModule.recoverRuntimeStartup(ctx); err != nil {
		t.Fatalf("fresh restart recovery: %v", err)
	}
	if restartedBackground.asyncFunctionCalls != 1 || restartedBackground.lastAsyncFunction == nil {
		t.Fatalf("fresh restart did not mount one successor: calls=%d", restartedBackground.asyncFunctionCalls)
	}
	if err := restartedModule.recoverRuntimeStartup(ctx); err != nil {
		t.Fatalf("duplicate startup recovery: %v", err)
	}
	if restartedBackground.asyncFunctionCalls != 1 {
		t.Fatalf("duplicate startup mounted a second successor: calls=%d", restartedBackground.asyncFunctionCalls)
	}
	beforeTransitions, err := restartedOwner.runtimeController.Load(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || beforeTransitions.DriveLease == nil {
		t.Fatalf("load continuation successor before transition proof: lease=%+v err=%v", beforeTransitions.DriveLease, err)
	}
	firstTransition, claimed, err := restartedModule.runtimeClaimDriveLease(
		ctx, beforeTransitions.DriveLease.Clone(), time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano),
	)
	if err != nil || !claimed || firstTransition.State != runtimecontract.RuntimeDriveLeaseRunning {
		t.Fatalf("first persisted continuation transition=%+v claimed=%v err=%v", firstTransition, claimed, err)
	}
	firstRun, err := baseRuns.GetByWorkspaceID(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	firstAggregate, err := restartedOwner.runtimeController.Load(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || runtimeDriveContinuationFence(firstAggregate, firstRun) != nil {
		t.Fatalf("first refreshed exact continuation fence failed: revision=%d err=%v", firstAggregate.StateRevision, err)
	}
	secondTransition, changed, err := restartedModule.runtimeRecoverDriveLease(
		ctx, facts.WorkspaceID, facts.RunID, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil || !changed || secondTransition.State != runtimecontract.RuntimeDriveLeaseScheduled {
		t.Fatalf("second persisted continuation transition=%+v changed=%v err=%v", secondTransition, changed, err)
	}
	secondAggregate, err := restartedOwner.runtimeController.Load(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if runtimeDriveContinuationFence(secondAggregate, firstRun) == nil {
		t.Fatal("stale Run owner snapshot incorrectly proved a later aggregate revision")
	}
	secondRun, err := baseRuns.GetByWorkspaceID(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || runtimeDriveContinuationFence(secondAggregate, secondRun) != nil ||
		secondAggregate.StateRevision < beforeTransitions.StateRevision+2 || secondAggregate.Facts.Recovery.TerminalDeliveryAttempts != 0 {
		t.Fatalf("second refreshed exact continuation fence failed: before=%d after=%d recovery=%+v err=%v", beforeTransitions.StateRevision, secondAggregate.StateRevision, secondAggregate.Facts.Recovery, err)
	}
}

func TestRuntimeGovernanceContinuationHistoricalDuplicateDoesNotReplayAcrossLaterBlocker(t *testing.T) {
	ctx := context.Background()
	definition := runtimeDefinition()
	definition.CapabilityContract = append(definition.CapabilityContract, "decision/resolution")

	// Build a real committed continuation ledger first. The second fixture below
	// grafts the immutable historical request/dispatch/resolution records into a
	// later, independently persisted blocker. This is the response-loss cut after
	// the old successor was consumed and released: only history remains, while
	// the current Runtime/Run fence belongs to the new blocker.
	historyRepository := &inMemoryRuntimeStateRepository{}
	historyOwner := newRuntimeTestOwner()
	if err := historyOwner.ConfigureRuntimeRepository(historyRepository); err != nil {
		t.Fatal(err)
	}
	historyRun := runtimeRunForTest(common.RunStateActive, definition)
	historyFacts, err := historyRun.RuntimeBootstrap.InitialFacts(historyRun.WorkspaceID, historyRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	historyFacts.Capabilities.GovernanceKinds = []string{"decision/resolution"}
	historyFacts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileEdit}
	historyFacts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceDependency, Signature: "historical-continuation", Message: "first governance required",
		GovernanceDependency: &runtimecontract.ExternalDependency{Kind: "external-approval", SubjectRef: "subject-historical-continuation", Reason: "first governance required"},
	}
	historyInitialized, err := historyOwner.InitializeRuntime(ctx, historyFacts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	historyManifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(historyInitialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(historyInitialized.Facts.Resources))
	if err != nil {
		t.Fatal(err)
	}
	historySealed, err := runtimeTestControllerSealGovernanceDispatch(historyOwner.runtimeController, ctx, historyFacts.WorkspaceID, historyFacts.RunID, historyManifest, runtimecontract.RuntimeCandidate{
		Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: *historyFacts.Failure.GovernanceDependency},
	})
	if err != nil || historySealed.ActiveDispatch == nil {
		t.Fatalf("seal historical governance dispatch=%+v err=%v", historySealed, err)
	}
	historyRequest := runtimecontract.RuntimeGovernanceRequestRecord{
		TicketID: "ticket-historical-continuation", AuthoritySnapshotID: historySealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID: historySealed.ActiveDispatch.ID, DependencyKind: historyFacts.Failure.GovernanceDependency.Kind,
		SubjectRef:        historyFacts.Failure.GovernanceDependency.SubjectRef,
		AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(runtimecontract.ExternalDependency{}.AuthorizationContext),
	}
	if _, err := historyOwner.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:      runtime.RuntimeInternalEventGovernanceRequestObserved,
		Scope:     runtime.RuntimeInternalEventScope{WorkspaceID: historyFacts.WorkspaceID, RunID: historyFacts.RunID},
		Authority: &runtime.RuntimeInternalAuthorityPayload{GovernanceRequest: &historyRequest},
	}); err != nil {
		t.Fatal(err)
	}
	historicalResolution := runtimecontract.RuntimeGovernanceResolution{
		ResolutionID: "resolution-historical-continuation", TicketID: historyRequest.TicketID,
		WorkspaceID: historyFacts.WorkspaceID, RunID: historyFacts.RunID,
		AuthoritySnapshotID: historyRequest.AuthoritySnapshotID, DispatchID: historyRequest.DispatchID,
		DependencyKind: historyRequest.DependencyKind, SubjectRef: historyRequest.SubjectRef,
		Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "first governance continued",
	}
	historyCommit, err := runtimeTestControllerCommitGovernanceResolution(historyOwner.runtimeController, ctx, historicalResolution)
	if err != nil || historyCommit.Disposition != runtime.CommitDispositionCommitted || len(historyCommit.Aggregate.GovernanceResolutions) != 1 {
		t.Fatalf("commit historical continuation=%+v err=%v", historyCommit, err)
	}

	later := runtimeBoundDefinitionClarificationAggregate(t)
	later.GovernanceRequests = append(later.GovernanceRequests, historyCommit.Aggregate.GovernanceRequests...)
	later.GovernanceResolutions = append(later.GovernanceResolutions, historyCommit.Aggregate.GovernanceResolutions...)
	later.CompletedDispatches = append(later.CompletedDispatches, historyCommit.Aggregate.CompletedDispatches...)
	later.Facts.Evidence.Accepted = append(later.Facts.Evidence.Accepted, historyCommit.Aggregate.Facts.Evidence.Accepted...)
	later.Facts = later.Facts.Normalized()
	later.ProcessedEventIDs = append(later.ProcessedEventIDs, historicalResolution.EventID())
	later.DriveLease = nil
	if err := later.Validate(); err != nil {
		t.Fatalf("later blocker with historical continuation is invalid: %v", err)
	}

	baseRuns := memory.NewRunRepository()
	hub := magicevent.NewHub(32)
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: baseRuns, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = runOwner.Stop(ctx)
		hub.Terminate(ctx)
	})
	if err := baseRuns.Create(ctx, runtimeRunForTest(common.RunStateActive, later.Definition)); err != nil {
		t.Fatal(err)
	}
	stateRepository := runtimeStateRepository{hub: hub}
	if err := stateRepository.Save(ctx, later, 0); err != nil {
		t.Fatalf("persist later blocker: %v", err)
	}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(stateRepository); err != nil {
		t.Fatal(err)
	}
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(owner, hub, background)
	laterRecord := runtimeStateRecordFromAggregate(later)
	laterHandoff, governed, err := runtimeGovernanceLifecycleHandoffFromState(&laterRecord)
	if err != nil || !governed || laterHandoff.FenceID() == runtimeGovernanceLifecycleFenceID(historicalResolution.AuthoritySnapshotID, historicalResolution.DispatchID) {
		t.Fatalf("later governance handoff=%+v governed=%v err=%v", laterHandoff, governed, err)
	}
	if err := module.applyRunGovernanceWait(
		ctx, later.Facts.WorkspaceID, later.Facts.RunID, laterHandoff.AuthoritySnapshotID, laterHandoff.DefinitionRevision,
		laterHandoff.EvidenceRevision, laterHandoff.DispatchID, laterHandoff.TicketID, laterHandoff.ApprovalID, later.StateRevision,
	); err != nil {
		t.Fatalf("apply later governance wait: %v", err)
	}
	beforeRun, err := baseRuns.GetByWorkspaceID(ctx, later.Facts.WorkspaceID, later.Facts.RunID)
	if err != nil || beforeRun.State != common.RunStateBlockedOnDecision || beforeRun.RuntimeLifecycleFence == nil ||
		beforeRun.RuntimeLifecycleFence.GovernanceFenceID != laterHandoff.FenceID() || beforeRun.RuntimeLifecycleFence.GovernanceResolutionID != "" {
		t.Fatalf("later Run blocker=%+v err=%v", beforeRun, err)
	}
	beforeAggregate, err := owner.runtimeController.Load(ctx, later.Facts.WorkspaceID, later.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}

	event := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution,
		decisioncommon.ModuleID, ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.CommitRuntimeGovernanceResolutionCommand{Resolution: historicalResolution},
	)
	result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
	module.handleRuntimeGovernanceResolution(event, result)
	data, magicErr := result.Get()
	response, ok := data.(orchestratorevents.CommitRuntimeGovernanceResolutionResult)
	if magicErr != nil || !ok || response.Disposition != orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate {
		t.Fatalf("historical continuation duplicate response=%+v err=%v", data, magicErr)
	}
	afterRun, runErr := baseRuns.GetByWorkspaceID(ctx, later.Facts.WorkspaceID, later.Facts.RunID)
	afterAggregate, loadErr := owner.runtimeController.Load(ctx, later.Facts.WorkspaceID, later.Facts.RunID)
	if runErr != nil || loadErr != nil || afterRun.Revision != beforeRun.Revision || afterRun.State != common.RunStateBlockedOnDecision ||
		afterRun.BlockingTicketID != beforeRun.BlockingTicketID || afterRun.RuntimeLifecycleFence == nil ||
		afterRun.RuntimeLifecycleFence.GovernanceFenceID != laterHandoff.FenceID() || afterAggregate.StateRevision != beforeAggregate.StateRevision ||
		afterAggregate.DriveLease != nil || len(afterAggregate.ProcessedEventIDs) != len(beforeAggregate.ProcessedEventIDs) ||
		background.asyncFunctionCalls != 0 {
		t.Fatalf("historical duplicate changed current owner state: run=%+v aggregateRevision=%d/%d lease=%+v timers=%d runErr=%v loadErr=%v", afterRun, beforeAggregate.StateRevision, afterAggregate.StateRevision, afterAggregate.DriveLease, background.asyncFunctionCalls, runErr, loadErr)
	}
}

func TestRuntimeGovernanceFailureUsesAtomicTerminalSuccessorAcrossPartialAckAndRestart(t *testing.T) {
	ctx := context.Background()
	baseRuns := memory.NewRunRepository()
	hub := magicevent.NewHub(32)
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: baseRuns, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = runOwner.Stop(ctx)
		hub.Terminate(ctx)
	})

	definition := runtimeDefinition()
	definition.CapabilityContract = append(definition.CapabilityContract, "decision/resolution")
	runInput := runtimeRunForTest(common.RunStateActive, definition)
	if err := baseRuns.Create(ctx, runInput); err != nil {
		t.Fatal(err)
	}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(owner, hub, background)
	facts, err := runInput.RuntimeBootstrap.InitialFacts(runInput.WorkspaceID, runInput.ID)
	if err != nil {
		t.Fatal(err)
	}
	facts.Capabilities.GovernanceKinds = []string{"decision/resolution"}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceDependency, Signature: "terminal-successor", Message: "governance required",
		GovernanceDependency: &runtimecontract.ExternalDependency{Kind: "external-approval", SubjectRef: "subject-terminal-successor", Reason: "governance required"},
	}
	initialized, err := owner.InitializeRuntime(ctx, facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(initialized.Facts.Resources))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := runtimeTestControllerSealGovernanceDispatch(owner.runtimeController, ctx, facts.WorkspaceID, facts.RunID, manifest, runtimecontract.RuntimeCandidate{
		Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: *facts.Failure.GovernanceDependency},
	})
	if err != nil || sealed.ActiveDispatch == nil {
		t.Fatalf("seal governance dispatch=%+v err=%v", sealed, err)
	}
	request := runtimecontract.RuntimeGovernanceRequestRecord{
		TicketID: "ticket-terminal-successor", AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID: sealed.ActiveDispatch.ID, DependencyKind: facts.Failure.GovernanceDependency.Kind,
		SubjectRef:        facts.Failure.GovernanceDependency.SubjectRef,
		AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(runtimecontract.ExternalDependency{}.AuthorizationContext),
	}
	boundEvent, err := owner.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:      runtime.RuntimeInternalEventGovernanceRequestObserved,
		Scope:     runtime.RuntimeInternalEventScope{WorkspaceID: facts.WorkspaceID, RunID: facts.RunID},
		Authority: &runtime.RuntimeInternalAuthorityPayload{GovernanceRequest: &request},
	})
	if err != nil {
		t.Fatal(err)
	}
	boundRecord := runtimeStateRecordFromAggregate(boundEvent.Aggregate)
	waitHandoff, governed, err := runtimeGovernanceLifecycleHandoffFromState(&boundRecord)
	if err != nil || !governed || waitHandoff.Kind != runtimeGovernanceLifecycleWaitDecision {
		t.Fatalf("governance wait handoff=%+v governed=%v err=%v", waitHandoff, governed, err)
	}
	if err := module.applyRunGovernanceWait(
		ctx, facts.WorkspaceID, facts.RunID, waitHandoff.AuthoritySnapshotID, waitHandoff.DefinitionRevision,
		waitHandoff.EvidenceRevision, waitHandoff.DispatchID, waitHandoff.TicketID, waitHandoff.ApprovalID,
		boundEvent.Aggregate.StateRevision,
	); err != nil {
		t.Fatalf("apply governance wait: %v", err)
	}
	oldLease, _, err := module.runtimeScheduleDriveLease(ctx, facts.WorkspaceID, facts.RunID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonCASReload, FailureSignature: "old-pre-resolution-lease",
		DueAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano),
	}, 0)
	if err != nil {
		t.Fatal(err)
	}

	resultCalls := 0
	resultObserver := magicevent.NewSimpleObserver(resultevents.CommandDestination, hub)
	resultObserver.Subscribe(resultevents.TopicResultFactTerminalDelivery, func(event magicevent.Event, result magicevent.Result) {
		resultCalls++
		command, ok := event.Data().(resultevents.RuntimeTerminalDeliveryFact)
		if !ok {
			result.Set(nil, nil)
			return
		}
		identity := command.Identity()
		ack := runtimecontract.RuntimeDeliveryAcknowledgement{
			WorkspaceID: identity.WorkspaceID, RunID: identity.RunID, TerminalDeliveryID: identity.TerminalDeliveryID,
			Owner: runtimecontract.RuntimeDeliveryOwnerResult, AcknowledgedState: runtimecontract.RuntimeDeliveryAcknowledgedResultRecorded,
			AuthoritySnapshotID: identity.AuthoritySnapshotID, CausationID: command.CausationID, Outcome: identity.Outcome,
		}
		result.Set(resultevents.ResultLifecycleCommandResult{DeliveryAck: &ack}, nil)
	})
	t.Cleanup(func() { resultObserver.Unsubscribe(resultevents.TopicResultFactTerminalDelivery) })

	driveKey := runtimeDriveKey(facts.WorkspaceID, facts.RunID)
	if !module.beginRuntimeDrive(driveKey) {
		t.Fatal("old governance-wait drive did not acquire gate")
	}
	resolution := runtimecontract.RuntimeGovernanceResolution{
		ResolutionID: "resolution-terminal-successor", TicketID: request.TicketID,
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
		AuthoritySnapshotID: request.AuthoritySnapshotID, DispatchID: request.DispatchID,
		DependencyKind: request.DependencyKind, SubjectRef: request.SubjectRef,
		Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeFail, Summary: "governance rejected",
	}
	event := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution,
		decisioncommon.ModuleID, ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.CommitRuntimeGovernanceResolutionCommand{Resolution: resolution},
	)
	result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
	module.handleRuntimeGovernanceResolution(event, result)
	data, magicErr := result.Get()
	response, ok := data.(orchestratorevents.CommitRuntimeGovernanceResolutionResult)
	if magicErr != nil || !ok || response.Disposition != orchestratorevents.RuntimeGovernanceResolutionDispositionCommitted || response.Terminal == nil {
		t.Fatalf("terminal governance response=%+v err=%v", data, magicErr)
	}
	persisted, err := owner.runtimeController.Load(ctx, facts.WorkspaceID, facts.RunID)
	wantSignature, signatureErr := runtime.RuntimeGovernanceResolutionDriveLeaseSignature(resolution)
	if err != nil || signatureErr != nil || persisted.DriveLease == nil || persisted.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		persisted.DriveLease.FailureSignature != wantSignature || persisted.DriveLease.RecoveryGeneration != oldLease.RecoveryGeneration+1 ||
		persisted.DriveLease.AggregateRevision != persisted.StateRevision || persisted.Facts.Recovery.TerminalDeliveryAttempts != 0 ||
		len(persisted.DeliveryAcknowledgements) != 0 || resultCalls != 0 || background.asyncFunctionCalls != 0 {
		t.Fatalf("atomic terminal successor=%+v recovery=%+v acks=%+v resultCalls=%d timers=%d err=%v signatureErr=%v", persisted.DriveLease, persisted.Facts.Recovery, persisted.DeliveryAcknowledgements, resultCalls, background.asyncFunctionCalls, err, signatureErr)
	}
	terminalLease := persisted.DriveLease.Clone()
	blocked, err := baseRuns.GetByWorkspaceID(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || blocked.State != common.RunStateBlockedOnDecision {
		t.Fatalf("terminal resolution wrote Run lifecycle directly: run=%+v err=%v", blocked, err)
	}

	duplicate := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
	module.handleRuntimeGovernanceResolution(event, duplicate)
	duplicateData, duplicateErr := duplicate.Get()
	duplicateResponse, duplicateOK := duplicateData.(orchestratorevents.CommitRuntimeGovernanceResolutionResult)
	if duplicateErr != nil || !duplicateOK || duplicateResponse.Disposition != orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate || background.asyncFunctionCalls != 0 {
		t.Fatalf("duplicate terminal resolution=%+v timers=%d err=%v", duplicateData, background.asyncFunctionCalls, duplicateErr)
	}
	duplicateAggregate, err := owner.runtimeController.Load(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || duplicateAggregate.DriveLease == nil || !runtimecontract.SameRuntimeDriveLeaseIdentity(*duplicateAggregate.DriveLease, terminalLease) {
		t.Fatalf("duplicate terminal resolution rewrote successor=%+v err=%v", duplicateAggregate.DriveLease, err)
	}

	var retry *runtimeRecoveryLeaseRequest
	module.finishScheduledRuntimeDrive(ctx, scheduledRuntimeDriveRequest{
		workspaceID: facts.WorkspaceID, runID: facts.RunID, source: "old_governance_wait_terminal", driveKey: driveKey,
	}, &retry)
	if background.asyncFunctionCalls != 1 || background.lastAsyncFunction == nil {
		t.Fatalf("old gate exit did not mount terminal successor: calls=%d", background.asyncFunctionCalls)
	}
	firstTimer := background.lastAsyncFunction
	firstTimer()
	partial, err := owner.runtimeController.Load(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || len(partial.DeliveryAcknowledgements) != 1 ||
		partial.DeliveryAcknowledgements[0].Owner != runtimecontract.RuntimeDeliveryOwnerResult ||
		partial.Facts.Recovery.TerminalDeliveryAttempts != 1 || partial.DriveLease == nil ||
		partial.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled || partial.DriveLease.FailureSignature != "terminal-delivery" ||
		resultCalls != 1 || background.asyncFunctionCalls != 2 {
		t.Fatalf("partial terminal delivery=%+v lease=%+v recovery=%+v resultCalls=%d timers=%d err=%v", partial.DeliveryAcknowledgements, partial.DriveLease, partial.Facts.Recovery, resultCalls, background.asyncFunctionCalls, err)
	}
	partialRevision := partial.StateRevision
	partialDuplicate := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
	module.handleRuntimeGovernanceResolution(event, partialDuplicate)
	partialDuplicateData, partialDuplicateErr := partialDuplicate.Get()
	partialDuplicateResponse, partialDuplicateOK := partialDuplicateData.(orchestratorevents.CommitRuntimeGovernanceResolutionResult)
	afterPartialDuplicate, loadErr := owner.runtimeController.Load(ctx, facts.WorkspaceID, facts.RunID)
	if partialDuplicateErr != nil || !partialDuplicateOK ||
		partialDuplicateResponse.Disposition != orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate ||
		loadErr != nil || afterPartialDuplicate.StateRevision != partialRevision || afterPartialDuplicate.DriveLease == nil ||
		!runtimecontract.SameRuntimeDriveLeaseIdentity(*afterPartialDuplicate.DriveLease, *partial.DriveLease) ||
		resultCalls != 1 || background.asyncFunctionCalls != 2 {
		t.Fatalf("partial-ack duplicate did not join exact terminal successor: response=%+v revision=%d/%d lease=%+v resultCalls=%d timers=%d err=%v loadErr=%v", partialDuplicateData, partialRevision, afterPartialDuplicate.StateRevision, afterPartialDuplicate.DriveLease, resultCalls, background.asyncFunctionCalls, partialDuplicateErr, loadErr)
	}
	partialLease := partial.DriveLease.Clone()
	if err := module.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	stopped, err := baseRuns.GetByWorkspaceID(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || stopped.RuntimeState == nil || stopped.RuntimeState.DriveLease == nil ||
		!runtimecontract.SameRuntimeDriveLeaseIdentity(*stopped.RuntimeState.DriveLease, partialLease) {
		t.Fatalf("Stop lost partial terminal successor: run=%+v err=%v", stopped, err)
	}

	workspaceCalls := 0
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceFactTerminalDelivery, func(event magicevent.Event, result magicevent.Result) {
		workspaceCalls++
		command, ok := event.Data().(workspaceevents.RuntimeTerminalDeliveryFact)
		if !ok {
			result.Set(nil, nil)
			return
		}
		identity := command.Identity()
		ack := runtimecontract.RuntimeDeliveryAcknowledgement{
			WorkspaceID: identity.WorkspaceID, RunID: identity.RunID, TerminalDeliveryID: identity.TerminalDeliveryID,
			Owner: runtimecontract.RuntimeDeliveryOwnerWorkspace, AcknowledgedState: runtimecontract.RuntimeDeliveryAcknowledgedWorkspaceReleased,
			AuthoritySnapshotID: identity.AuthoritySnapshotID, CausationID: command.CausationID, Outcome: identity.Outcome,
		}
		result.Set(workspaceevents.WorkspaceLifecycleCommandResult{DeliveryAck: &ack}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceFactTerminalDelivery) })

	restartedOwner := newRuntimeTestOwner()
	if err := restartedOwner.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	restartedBackground := &recordingBackgroundRoutine{}
	restartedModule := newTestBiz(restartedOwner, hub, restartedBackground)
	if err := restartedModule.recoverRuntimeStartup(ctx); err != nil {
		t.Fatalf("recover partial terminal successor: %v", err)
	}
	if restartedBackground.asyncFunctionCalls != 1 || restartedBackground.lastAsyncFunction == nil {
		t.Fatalf("fresh restart did not mount partial terminal successor: calls=%d", restartedBackground.asyncFunctionCalls)
	}
	restartedBackground.lastAsyncFunction()
	replayed, err := restartedOwner.runtimeController.Load(ctx, facts.WorkspaceID, facts.RunID)
	terminalRun, runErr := baseRuns.GetByWorkspaceID(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || runErr != nil || len(replayed.DeliveryAcknowledgements) != 3 || resultCalls != 1 || workspaceCalls != 1 ||
		!runtimeTerminalDeliveryFullyAcknowledged(replayed) || replayed.Facts.Recovery.TerminalDeliveryAttempts != 1 ||
		replayed.DriveLease != nil || terminalRun.State != common.RunStateFailed {
		t.Fatalf("fresh terminal replay did not converge the exact Result->Workspace->Run chain: acks=%+v recovery=%+v lease=%+v resultCalls=%d workspaceCalls=%d run=%+v err=%v runErr=%v", replayed.DeliveryAcknowledgements, replayed.Facts.Recovery, replayed.DriveLease, resultCalls, workspaceCalls, terminalRun, err, runErr)
	}
	fullyAcknowledgedRevision := terminalRun.Revision
	fullyAcknowledgedTimerCalls := restartedBackground.asyncFunctionCalls
	fullyAcknowledgedDuplicate := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
	restartedModule.handleRuntimeGovernanceResolution(event, fullyAcknowledgedDuplicate)
	fullyAcknowledgedData, fullyAcknowledgedErr := fullyAcknowledgedDuplicate.Get()
	fullyAcknowledgedResponse, fullyAcknowledgedOK := fullyAcknowledgedData.(orchestratorevents.CommitRuntimeGovernanceResolutionResult)
	afterFullyAcknowledgedDuplicate, runErr := baseRuns.GetByWorkspaceID(ctx, facts.WorkspaceID, facts.RunID)
	afterFullyAcknowledgedAggregate, loadErr := restartedOwner.runtimeController.Load(ctx, facts.WorkspaceID, facts.RunID)
	if fullyAcknowledgedErr != nil || !fullyAcknowledgedOK ||
		fullyAcknowledgedResponse.Disposition != orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate ||
		runErr != nil || loadErr != nil || afterFullyAcknowledgedDuplicate.Revision != fullyAcknowledgedRevision ||
		afterFullyAcknowledgedAggregate.StateRevision != replayed.StateRevision || afterFullyAcknowledgedAggregate.DriveLease != nil ||
		restartedBackground.asyncFunctionCalls != fullyAcknowledgedTimerCalls {
		t.Fatalf("fully acknowledged terminal duplicate changed delivery state: response=%+v runRevision=%d/%d aggregateRevision=%d/%d lease=%+v timers=%d/%d err=%v runErr=%v loadErr=%v", fullyAcknowledgedData, fullyAcknowledgedRevision, afterFullyAcknowledgedDuplicate.Revision, replayed.StateRevision, afterFullyAcknowledgedAggregate.StateRevision, afterFullyAcknowledgedAggregate.DriveLease, fullyAcknowledgedTimerCalls, restartedBackground.asyncFunctionCalls, fullyAcknowledgedErr, runErr, loadErr)
	}
}

func TestRuntimeStartupRecoveryDeliversPersistedGovernanceContinuationToBlockedRun(t *testing.T) {
	ctx := context.Background()
	baseRuns := memory.NewRunRepository()
	guardedRuns := &rejectGovernanceContinuationCASRepository{RunRepository: baseRuns}
	firstHub := magicevent.NewHub(16)
	definition := runtimeDefinition()
	definition.CapabilityContract = append(definition.CapabilityContract, "decision/resolution")
	runInput := runtimeRunForTest(common.RunStateActive, definition)
	if err := baseRuns.Create(ctx, runInput); err != nil {
		t.Fatal(err)
	}
	firstRunOwner := runmodule.New()
	if err := firstRunOwner.Configure(runbiz.Dependencies{Runs: guardedRuns, EventHub: firstHub}); err != nil {
		t.Fatal(err)
	}
	if err := firstRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	firstOwner := newRuntimeTestOwner()
	if err := firstOwner.ConfigureRuntimeRepository(runtimeStateRepository{hub: firstHub}); err != nil {
		t.Fatal(err)
	}
	firstBackground := &recordingBackgroundRoutine{}
	firstModule := newTestBiz(firstOwner, firstHub, firstBackground)
	facts, err := runInput.RuntimeBootstrap.InitialFacts(runInput.WorkspaceID, runInput.ID)
	if err != nil {
		t.Fatal(err)
	}
	facts.Capabilities.GovernanceKinds = []string{"decision/resolution"}
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileEdit}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceDependency, Signature: "persisted-continuation", Message: "governance required",
		GovernanceDependency: &runtimecontract.ExternalDependency{Kind: "external-approval", SubjectRef: "subject-1", Reason: "governance required"},
	}
	initialized, err := firstOwner.InitializeRuntime(ctx, facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(initialized.Facts.Resources))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := runtimeTestControllerSealGovernanceDispatch(firstOwner.runtimeController, ctx, facts.WorkspaceID, facts.RunID, manifest, runtimecontract.RuntimeCandidate{
		Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: *facts.Failure.GovernanceDependency},
	})
	if err != nil || sealed.ActiveDispatch == nil {
		t.Fatalf("seal governance dispatch=%+v err=%v", sealed, err)
	}
	request := runtimecontract.RuntimeGovernanceRequestRecord{
		TicketID: "ticket-persisted-continuation", AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID: sealed.ActiveDispatch.ID, DependencyKind: "external-approval", SubjectRef: "subject-1",
		AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(runtimecontract.ExternalDependency{}.AuthorizationContext),
	}
	boundEvent, err := firstOwner.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventGovernanceRequestObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: facts.WorkspaceID, RunID: facts.RunID},
		Authority: &runtime.RuntimeInternalAuthorityPayload{GovernanceRequest: &request},
	})
	if err != nil {
		t.Fatal(err)
	}
	boundRecord := runtimeStateRecordFromAggregate(boundEvent.Aggregate)
	waitHandoff, governed, err := runtimeGovernanceLifecycleHandoffFromState(&boundRecord)
	if err != nil || !governed || waitHandoff.Kind != runtimeGovernanceLifecycleWaitDecision {
		t.Fatalf("persisted governance wait handoff=%+v governed=%v err=%v", waitHandoff, governed, err)
	}
	if err := firstModule.applyRunGovernanceWait(ctx, facts.WorkspaceID, facts.RunID, waitHandoff.AuthoritySnapshotID, waitHandoff.DefinitionRevision, waitHandoff.EvidenceRevision, waitHandoff.DispatchID, waitHandoff.TicketID, waitHandoff.ApprovalID, boundEvent.Aggregate.StateRevision); err != nil {
		t.Fatalf("apply persisted governance wait: %v", err)
	}
	blocked, err := baseRuns.GetByWorkspaceID(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || blocked.State != common.RunStateBlockedOnDecision || blocked.BlockingTicketID != request.TicketID {
		t.Fatalf("blocked Run=%+v err=%v", blocked, err)
	}
	resolution := runtimecontract.RuntimeGovernanceResolution{
		ResolutionID: "resolution-persisted-continuation", TicketID: request.TicketID, WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
		AuthoritySnapshotID: request.AuthoritySnapshotID, DispatchID: request.DispatchID, DependencyKind: request.DependencyKind, SubjectRef: request.SubjectRef,
		Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "continue from persisted governance resolution",
	}
	commit, err := runtimeTestControllerCommitGovernanceResolution(firstOwner.runtimeController, ctx, resolution)
	if err != nil || commit.Disposition != runtime.CommitDispositionCommitted || commit.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("commit governance resolution=%+v err=%v", commit, err)
	}

	// The governance CAS itself seals the successor before any continuation
	// attempt. A fresh startup therefore needs no heuristic classification of a
	// bare resolution cut; it mounts that exact persisted generation.
	cutRun, err := baseRuns.GetByWorkspaceID(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || cutRun.State != common.RunStateBlockedOnDecision || cutRun.RuntimeState == nil || cutRun.RuntimeState.DriveLease == nil ||
		cutRun.RuntimeLifecycleFence == nil || cutRun.RuntimeLifecycleFence.GovernanceResolutionID != resolution.ResolutionID {
		t.Fatalf("governance CAS cut Run=%+v err=%v", cutRun, err)
	}
	cutRuns := memory.NewRunRepository()
	// This fixture copies the logical Run into a distinct persistence owner;
	// the destination repository must allocate its own workspace generation.
	cutRun.WorkspaceRunGeneration = 0
	if err := cutRuns.Create(ctx, cutRun); err != nil {
		t.Fatal(err)
	}
	cutHub := magicevent.NewHub(16)
	cutDriveGuard := &failFirstRuntimeDriveReadAfterContinuationRepository{RunRepository: cutRuns, resolutionID: resolution.ResolutionID}
	cutRunOwner := runmodule.New()
	if err := cutRunOwner.Configure(runbiz.Dependencies{Runs: cutDriveGuard, EventHub: cutHub}); err != nil {
		t.Fatal(err)
	}
	if err := cutRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	cutOwner := newRuntimeTestOwner()
	if err := cutOwner.ConfigureRuntimeRepository(runtimeStateRepository{hub: cutHub}); err != nil {
		t.Fatal(err)
	}
	cutBackground := &recordingBackgroundRoutine{}
	cutModule := newTestBiz(cutOwner, cutHub, cutBackground)
	if err := cutModule.recoverRuntimeStartup(ctx); err != nil {
		t.Fatalf("recover atomic governance CAS cut: %v", err)
	}
	if cutBackground.asyncFunctionCalls != 1 || cutBackground.lastAsyncFunction == nil {
		t.Fatalf("atomic governance CAS cut schedules=%d", cutBackground.asyncFunctionCalls)
	}
	cutDriveGuard.armed = true
	cutBackground.lastAsyncFunction()
	cutContinued, err := cutRuns.GetByWorkspaceID(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || cutContinued.State != common.RunStateActive || cutContinued.RuntimeState == nil || cutContinued.RuntimeLifecycleFence == nil ||
		cutContinued.RuntimeLifecycleFence.GovernanceResolutionID != resolution.ResolutionID || cutContinued.RuntimeState.DriveLease == nil ||
		cutContinued.RuntimeState.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled || !cutDriveGuard.fired ||
		cutContinued.RuntimeState.Facts.Recovery.TerminalDeliveryAttempts != 0 {
		t.Fatalf("atomic governance CAS cut continuation=%+v err=%v", cutContinued, err)
	}
	// Model owner commit followed by response loss. Exact persisted owner state
	// is a no-op acknowledgement; active state with an old resolution is not.
	if delivered, err := cutModule.deliverPersistedRuntimeGovernanceContinuation(ctx, facts.WorkspaceID, facts.RunID); err != nil || delivered {
		t.Fatalf("exact active response-loss acknowledgement delivered=%v err=%v", delivered, err)
	}
	staleActive := runmodel.CloneRun(cutContinued)
	staleActive.RuntimeLifecycleFence.GovernanceResolutionID = "resolution-stale-response-loss"
	if err := cutRuns.Update(ctx, staleActive); err != nil {
		t.Fatal(err)
	}
	if delivered, err := cutModule.deliverPersistedRuntimeGovernanceContinuation(ctx, facts.WorkspaceID, facts.RunID); delivered || err == nil {
		t.Fatalf("stale active response-loss acknowledgement delivered=%v err=%v", delivered, err)
	}
	if err := cutRunOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	cutHub.Terminate(ctx)

	response := orchestratorevents.CommitRuntimeGovernanceResolutionResult{
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, ResolutionID: resolution.ResolutionID,
		Disposition: orchestratorevents.RuntimeGovernanceResolutionDispositionCommitted, StateRevision: commit.Aggregate.StateRevision,
		EvidenceRevision: commit.Aggregate.Facts.EvidenceRevision, AuthoritySnapshotID: commit.Aggregate.Decision.AuthoritySnapshot.ID,
		State: commit.Aggregate.Decision.State, Terminal: commit.Aggregate.Decision.Terminal,
	}
	guardedRuns.armed = true
	deliveryErr := firstModule.continueRuntimeGovernance(ctx, response, resolution, commit.Aggregate.DriveLease)
	var lifecycleErr *runtimeLifecycleOwnerDeliveryError
	if !errors.As(deliveryErr, &lifecycleErr) || lifecycleErr.handoff.Kind != runtimeGovernanceLifecycleContinuationDecision || lifecycleErr.handoff.ResolutionID != resolution.ResolutionID {
		t.Fatalf("continuation delivery error=%v typed=%+v", deliveryErr, lifecycleErr)
	}
	pending, err := firstOwner.runtimeController.Load(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || pending.DriveLease == nil || pending.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry ||
		pending.Facts.Recovery.TerminalDeliveryAttempts != 0 {
		t.Fatalf("persisted continuation lease=%+v recovery=%+v err=%v", pending.DriveLease, pending.Facts.Recovery, err)
	}
	if err := firstRunOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	firstHub.Terminate(ctx)

	// A fresh process recovers the persisted lifecycle lease. The retry derives
	// the same continuation from the aggregate, activates Run through SM-1, and
	// consumes no terminal-delivery budget.
	restartedHub := magicevent.NewHub(16)
	t.Cleanup(func() { restartedHub.Terminate(ctx) })
	restartedDriveGuard := &failFirstRuntimeDriveReadAfterContinuationRepository{RunRepository: baseRuns, resolutionID: resolution.ResolutionID}
	restartedRunOwner := runmodule.New()
	if err := restartedRunOwner.Configure(runbiz.Dependencies{Runs: restartedDriveGuard, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	restartedOwner := newRuntimeTestOwner()
	if err := restartedOwner.ConfigureRuntimeRepository(runtimeStateRepository{hub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	restartedBackground := &recordingBackgroundRoutine{}
	restartedModule := newTestBiz(restartedOwner, restartedHub, restartedBackground)
	if err := restartedModule.recoverRuntimeStartup(ctx); err != nil {
		t.Fatalf("recover persisted continuation lease: %v", err)
	}
	if restartedBackground.asyncFunctionCalls != 1 || restartedBackground.lastAsyncFunction == nil {
		t.Fatalf("restart continuation schedules=%d", restartedBackground.asyncFunctionCalls)
	}
	restartedDriveGuard.armed = true
	restartedBackground.lastAsyncFunction()
	continued, err := baseRuns.GetByWorkspaceID(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || continued.State != common.RunStateActive || continued.BlockingTicketID != "" || continued.BlockingApprovalID != "" ||
		continued.RuntimeState == nil || continued.RuntimeLifecycleFence == nil || continued.RuntimeLifecycleFence.GovernanceResolutionID != resolution.ResolutionID ||
		continued.RuntimeState.DriveLease == nil || continued.RuntimeState.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		!restartedDriveGuard.fired || continued.RuntimeState.Facts.Recovery.TerminalDeliveryAttempts != 0 {
		t.Fatalf("fresh restart continuation Run=%+v err=%v", continued, err)
	}
	loaded, err := restartedOwner.runtimeController.Load(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || loaded.DriveLease == nil || loaded.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		loaded.Facts.Recovery.TerminalDeliveryAttempts != 0 || len(loaded.GovernanceResolutions) != 1 {
		t.Fatalf("fresh restart continuation aggregate lease=%+v recovery=%+v resolutions=%d err=%v", loaded.DriveLease, loaded.Facts.Recovery, len(loaded.GovernanceResolutions), err)
	}
}
