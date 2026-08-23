package orchestrator

// Owner Biz lifecycle tests.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if module.runtimeController != nil {
		t.Fatal("new module runtime controller is non-nil before Configure")
	}

	deps := moduleContractDependencies()
	recoveryObserver := magicevent.NewSimpleObserver(runevents.CommandDestination, deps.CommandHub)
	recoveryObserver.Subscribe(runevents.TopicRunCommandRuntimeRecoveryInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{}, nil)
		}
	})
	t.Cleanup(func() {
		recoveryObserver.Unsubscribe(runevents.TopicRunCommandRuntimeRecoveryInventory)
		deps.CommandHub.Terminate(context.Background())
	})
	if err := module.Configure(deps); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.runtimeController == nil {
		t.Fatal("runtime controller is nil after Configure")
	}
	if module.RuntimeRepositoryConfigured() == false {
		t.Fatal("Runtime repository is not configured")
	}
	if module.EventHub() == nil {
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
	if module.runtimeController != nil || module.started || len(module.rawTopics) != 0 {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestGMMR1SM4OrchestratorStartFailsWithoutRecoveryInventory(t *testing.T) {
	deps := moduleContractDependencies()
	t.Cleanup(func() { deps.CommandHub.Terminate(context.Background()) })
	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err == nil {
		t.Fatal("Start succeeded without the required Run recovery inventory subscription")
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop after failed Start: %v", err)
	}
}

func TestGMMR1SM4StartupRecoverySchedulesBootstrapOnlyActiveRun(t *testing.T) {
	deps := moduleContractDependencies()
	background := &recordingBackgroundRoutine{}
	deps.BackgroundRoutine = background
	recoveryObserver := magicevent.NewSimpleObserver(runevents.CommandDestination, deps.CommandHub)
	recoveryObserver.Subscribe(runevents.TopicRunCommandRuntimeRecoveryInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{Items: []runevents.RuntimeRecoveryInventoryItem{{
				WorkspaceID:         "ws-bootstrap-only",
				RunID:               "run-bootstrap-only",
				State:               common.RunStateActive,
				HasRuntimeBootstrap: true,
			}}}, nil)
		}
	})
	recoveryObserver.Subscribe(runevents.TopicRunCommandRuntimeStateRecordReadiness, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeStateRecordReadinessResult{Ready: true}, nil)
		}
	})
	t.Cleanup(func() {
		recoveryObserver.Unsubscribe(runevents.TopicRunCommandRuntimeRecoveryInventory)
		recoveryObserver.Unsubscribe(runevents.TopicRunCommandRuntimeStateRecordReadiness)
		deps.CommandHub.Terminate(context.Background())
	})

	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if background.asyncFunctionCalls != 1 || background.lastAsyncFunction == nil {
		t.Fatalf("startup recovery schedules = %d, want one bootstrap-only drive", background.asyncFunctionCalls)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestGMMR3StartupRecoveryWaitsForRunCommandsReady(t *testing.T) {
	deps := moduleContractDependencies()
	background := &startupReadyRecordingBackgroundRoutine{scheduled: make(chan struct{}, 1)}
	deps.BackgroundRoutine = background
	runs := memoryrepo.NewRunRepository()
	bootstrap := runtimecontract.RuntimeBootstrap{SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent}
	if err := runs.Create(context.Background(), &runmodel.Run{
		ID: "run-startup-ready", WorkspaceID: "ws-startup-ready", State: common.RunStateActive, RuntimeBootstrap: &bootstrap,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: deps.CommandHub}); err != nil {
		t.Fatal(err)
	}
	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = module.Stop(context.Background())
		_ = runOwner.Stop(context.Background())
		deps.CommandHub.Terminate(context.Background())
	})
	if err := module.Start(); err != nil {
		t.Fatalf("orchestrator Start: %v", err)
	}
	select {
	case <-background.scheduled:
		t.Fatal("startup drive was scheduled before Run commands were ready")
	default:
	}
	if err := runOwner.Start(); err != nil {
		t.Fatalf("run owner Start: %v", err)
	}
	select {
	case <-background.scheduled:
	case <-time.After(time.Second):
		t.Fatal("startup drive was not released by Run commands-ready fact")
	}
	for _, causationID := range []string{"run-owner-commands-ready", "run-owner-commands-ready-duplicate"} {
		deps.CommandHub.Post(magicevent.NewEventWithContext(
			runevents.TopicRunRuntimeCommandsReady,
			runevents.CommandDestination,
			runevents.NotificationDestination,
			magicevent.NewHeader(),
			context.Background(),
			runevents.RuntimeCommandsReadyData{CausationID: causationID},
		))
	}
	select {
	case <-background.scheduled:
		t.Fatal("duplicate Run commands-ready fact released startup recovery twice")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestGMMR2StartupPriorOwnerFactContractSchemaMarksRunFreshActivationWithoutDrive(t *testing.T) {
	deps := moduleContractDependencies()
	background := &recordingBackgroundRoutine{}
	deps.BackgroundRoutine = background
	observer := magicevent.NewSimpleObserver(runevents.CommandDestination, deps.CommandHub)
	llmCalls := observeRuntimeLLMCalls(t, deps.CommandHub)
	fileCalls := observeRuntimeFileCalls(t, deps.CommandHub)
	record := runtimeStateRecordFromAggregate(runtimeAggregate(t, 11))
	record.Facts.DefinitionFocuses[0].RequiredOwnerFacts = []runtimecontract.RuntimeOwnerFactRequirement{{
		OwnerModuleID: runtimecontract.RuntimeModuleShell,
		FactKind:      runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
	}}
	record.Facts.DefinitionFocuses[0].OwnerFactConvergences = nil
	// This is the exact persisted shape produced before owner-fact convergence
	// became part of the sealed Runtime schema. Prove that the fixture would
	// fail the new deep contract if the schema gate incorrectly admitted it.
	currentSchema := record.Clone()
	currentSchema.SchemaVersion = runtimecontract.RuntimeStateSchemaVersionCurrent
	if err := currentSchema.Validate(); err == nil {
		t.Fatal("prior owner-fact record unexpectedly satisfies the current Runtime schema")
	}
	record.SchemaVersion = 1
	item := runevents.RuntimeRecoveryInventoryItem{
		WorkspaceID:               record.Facts.WorkspaceID,
		RunID:                     record.Facts.RunID,
		State:                     common.RunStateActive,
		HasRuntimeBootstrap:       true,
		HasRuntimeState:           true,
		RuntimeStateRevision:      record.StateRevision,
		RuntimeStateSchemaVersion: record.SchemaVersion,
		RuntimeStateRef:           runevents.RuntimeStateRef(record.Facts.WorkspaceID, record.Facts.RunID, record.StateRevision),
	}
	var received *runevents.RuntimeStateIncompatibleFact
	observer.Subscribe(runevents.TopicRunCommandRuntimeRecoveryInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{Items: []runevents.RuntimeRecoveryInventoryItem{item}}, nil)
		}
	})
	observer.Subscribe(runevents.TopicRunCommandRuntimeStateRecord, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeStateRecordResult{
				WorkspaceID: item.WorkspaceID, RunID: item.RunID, StatePresent: true,
				RuntimeStateRef: item.RuntimeStateRef, RuntimeStateRevision: item.RuntimeStateRevision,
				RuntimeState: &record,
			}, nil)
		}
	})
	observer.Subscribe(runevents.TopicRunCommandRuntimeStateRecordReadiness, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeStateRecordReadinessResult{Ready: true}, nil)
		}
	})
	observer.Subscribe(runevents.TopicRunFactRuntimeStateIncompatible, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(runevents.RuntimeStateIncompatibleFact)
		if !ok {
			if result != nil {
				result.Set(nil, nil)
			}
			return
		}
		copy := command
		received = &copy
		if result != nil {
			result.Set(runevents.RunLifecycleCommandResult{Transitioned: true, RuntimeActivationDisposition: runtimecontract.RuntimeActivationDispositionFreshActivationRequired}, nil)
		}
	})
	t.Cleanup(func() {
		observer.Unsubscribe(runevents.TopicRunCommandRuntimeRecoveryInventory)
		observer.Unsubscribe(runevents.TopicRunCommandRuntimeStateRecord)
		observer.Unsubscribe(runevents.TopicRunCommandRuntimeStateRecordReadiness)
		observer.Unsubscribe(runevents.TopicRunFactRuntimeStateIncompatible)
		deps.CommandHub.Terminate(context.Background())
	})

	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	if background.asyncFunctionCalls != 0 {
		t.Fatalf("unsupported schema scheduled %d background drives", background.asyncFunctionCalls)
	}
	llmCalls.assertNone(t)
	fileCalls.assertNone(t)
	if received == nil || received.WorkspaceID != item.WorkspaceID || received.RunID != item.RunID ||
		received.RuntimeStateRef != item.RuntimeStateRef ||
		received.Incompatibility.Reason != runtimecontract.RuntimeStateIncompatibilityReasonSchemaUnsupported ||
		received.Incompatibility.ActualSchemaVersion != 1 ||
		received.Incompatibility.SupportedSchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent ||
		received.Feedback.Domain != runtimecontract.RuntimeFeedbackDomainDrive || received.Feedback.StableCategory != runtimecontract.RuntimeFeedbackCategoryStateSchemaUnsupported ||
		received.Feedback.OriginalDiagnostic == nil {
		t.Fatalf("schema incompatibility fact=%+v", received)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestGMMR2StartupCurrentSchemaDuplicateCWILedgerMarksRunFreshActivationWithoutDrive(t *testing.T) {
	deps := moduleContractDependencies()
	background := &recordingBackgroundRoutine{}
	deps.BackgroundRoutine = background
	observer := magicevent.NewSimpleObserver(runevents.CommandDestination, deps.CommandHub)
	record := runtimeStateRecordFromAggregate(runtimeAggregate(t, 12))
	record.Facts.CompletedFocuses = []runtimecontract.CoverageFocus{record.Facts.Focus.Normalized()}
	item := runevents.RuntimeRecoveryInventoryItem{
		WorkspaceID:               record.Facts.WorkspaceID,
		RunID:                     record.Facts.RunID,
		State:                     common.RunStateActive,
		HasRuntimeBootstrap:       true,
		HasRuntimeState:           true,
		RuntimeStateRevision:      record.StateRevision,
		RuntimeStateSchemaVersion: record.SchemaVersion,
		RuntimeStateRef:           runevents.RuntimeStateRef(record.Facts.WorkspaceID, record.Facts.RunID, record.StateRevision),
	}
	var received *runevents.RuntimeStateIncompatibleFact
	observer.Subscribe(runevents.TopicRunCommandRuntimeRecoveryInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{Items: []runevents.RuntimeRecoveryInventoryItem{item}}, nil)
		}
	})
	observer.Subscribe(runevents.TopicRunCommandRuntimeStateRecord, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeStateRecordResult{
				WorkspaceID: item.WorkspaceID, RunID: item.RunID, StatePresent: true,
				RuntimeStateRef: item.RuntimeStateRef, RuntimeStateRevision: item.RuntimeStateRevision,
				RuntimeState: &record,
			}, nil)
		}
	})
	observer.Subscribe(runevents.TopicRunCommandRuntimeStateRecordReadiness, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeStateRecordReadinessResult{Ready: true}, nil)
		}
	})
	observer.Subscribe(runevents.TopicRunFactRuntimeStateIncompatible, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(runevents.RuntimeStateIncompatibleFact)
		if !ok {
			if result != nil {
				result.Set(nil, nil)
			}
			return
		}
		copy := command
		received = &copy
		if result != nil {
			result.Set(runevents.RunLifecycleCommandResult{Transitioned: true, RuntimeActivationDisposition: runtimecontract.RuntimeActivationDispositionFreshActivationRequired}, nil)
		}
	})
	llmCalls := observeRuntimeLLMCalls(t, deps.CommandHub)
	fileCalls := observeRuntimeFileCalls(t, deps.CommandHub)
	t.Cleanup(func() {
		observer.Unsubscribe(runevents.TopicRunCommandRuntimeRecoveryInventory)
		observer.Unsubscribe(runevents.TopicRunCommandRuntimeStateRecord)
		observer.Unsubscribe(runevents.TopicRunCommandRuntimeStateRecordReadiness)
		observer.Unsubscribe(runevents.TopicRunFactRuntimeStateIncompatible)
		deps.CommandHub.Terminate(context.Background())
	})

	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	if background.asyncFunctionCalls != 0 {
		t.Fatalf("invalid CWI ledger scheduled %d background drives", background.asyncFunctionCalls)
	}
	if received == nil || received.WorkspaceID != item.WorkspaceID || received.RunID != item.RunID ||
		received.RuntimeStateRef != item.RuntimeStateRef ||
		received.Incompatibility.Reason != runtimecontract.RuntimeStateIncompatibilityReasonCWILedgerInvalid ||
		received.Incompatibility.ActualSchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent ||
		received.Incompatibility.SupportedSchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent ||
		received.Feedback.Domain != runtimecontract.RuntimeFeedbackDomainDrive ||
		received.Feedback.StableCategory != runtimecontract.RuntimeFeedbackCategoryStateCWILedgerInvalid ||
		received.Feedback.OriginalDiagnostic == nil {
		t.Fatalf("CWI ledger incompatibility fact=%+v", received)
	}
	llmCalls.assertNone(t)
	fileCalls.assertNone(t)
	if err := module.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestGMMR2DirectDriveSchemaV6MarksRunFreshActivationWithoutRetry(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-v2")
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(context.Background()) })

	runs := memoryrepo.NewRunRepository()
	bootstrap := runtimeBootstrap()
	legacy := runtimeStateRecordFromAggregate(runtimeAggregate(t, 11))
	legacy.SchemaVersion = 6
	run := runtimeRunForTestBootstrap(common.RunStateActive, bootstrap)
	run.RuntimeState = &legacy
	if err := runs.Create(ctx, run); err != nil {
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

	llmCalls := observeRuntimeLLMCalls(t, hub)
	fileCalls := observeRuntimeFileCalls(t, hub)

	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	command := orchestratorevents.DriveRuntimeCommand{WorkspaceID: run.WorkspaceID, RunID: run.ID}

	if err := module.scheduleRuntimeDrive(ctx, run.WorkspaceID, run.ID, "resume-schema-incompatible"); err != nil {
		t.Fatalf("schedule legacy drive: %v", err)
	}
	stored, err := runs.GetByWorkspaceID(ctx, run.WorkspaceID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != common.RunStateActive || stored.RuntimeActivationStatus == nil ||
		stored.RuntimeActivationStatus.Disposition != runtimecontract.RuntimeActivationDispositionFreshActivationRequired ||
		stored.RuntimeActivationStatus.Incompatibility.Reason != runtimecontract.RuntimeStateIncompatibilityReasonSchemaUnsupported ||
		stored.RuntimeActivationStatus.Incompatibility.ActualSchemaVersion != legacy.SchemaVersion ||
		stored.RuntimeActivationStatus.RuntimeStateRef != runevents.RuntimeStateRef(run.WorkspaceID, run.ID, legacy.StateRevision) ||
		stored.RuntimeState == nil || stored.RuntimeState.SchemaVersion != legacy.SchemaVersion ||
		stored.RuntimeState.StateRevision != legacy.StateRevision || stored.RuntimeState.DriveLease != nil {
		t.Fatalf("direct legacy drive did not preserve fail-closed Run boundary: %+v", stored)
	}
	llmCalls.assertNone(t)
	fileCalls.assertNone(t)
	firstRunRevision := stored.Revision

	_, err = module.driveRuntime(ctx, command)
	var fresh *runtimeFreshActivationRequiredError
	if !errors.As(err, &fresh) || fresh.RuntimeStateRef != runevents.RuntimeStateRef(run.WorkspaceID, run.ID, legacy.StateRevision) {
		t.Fatalf("direct fresh-activation boundary error=%v fresh=%+v", err, fresh)
	}
	if err := module.scheduleRuntimeDrive(ctx, run.WorkspaceID, run.ID, "repeat-resume-schema-incompatible"); err != nil {
		t.Fatalf("repeat schedule legacy drive: %v", err)
	}
	stored, err = runs.GetByWorkspaceID(ctx, run.WorkspaceID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Revision != firstRunRevision || stored.RuntimeActivationStatus == nil ||
		stored.RuntimeState == nil || stored.RuntimeState.SchemaVersion != legacy.SchemaVersion {
		t.Fatalf("repeat legacy drive mutated Run or retried authority: %+v", stored)
	}
	llmCalls.assertNone(t)
	fileCalls.assertNone(t)
}

func TestGMMR2DirectDriveCurrentSchemaDuplicateCWILedgerMarksRunFreshActivationWithoutSideEffects(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-v2")
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(context.Background()) })

	runs := memoryrepo.NewRunRepository()
	bootstrap := runtimeBootstrap()
	ambiguous := runtimeStateRecordFromAggregate(runtimeAggregate(t, 12))
	ambiguous.Facts.CompletedFocuses = []runtimecontract.CoverageFocus{ambiguous.Facts.Focus.Normalized()}
	run := runtimeRunForTestBootstrap(common.RunStateActive, bootstrap)
	run.RuntimeState = &ambiguous
	if err := runs.Create(ctx, run); err != nil {
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

	llmCalls := observeRuntimeLLMCalls(t, hub)
	fileCalls := observeRuntimeFileCalls(t, hub)

	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	command := orchestratorevents.DriveRuntimeCommand{WorkspaceID: run.WorkspaceID, RunID: run.ID}

	if err := module.scheduleRuntimeDrive(ctx, run.WorkspaceID, run.ID, "resume-current-schema-cwi-ledger-incompatible"); err != nil {
		t.Fatalf("schedule current-schema ambiguous drive: %v", err)
	}
	stored, err := runs.GetByWorkspaceID(ctx, run.WorkspaceID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != common.RunStateActive || stored.RuntimeActivationStatus == nil ||
		stored.RuntimeActivationStatus.Disposition != runtimecontract.RuntimeActivationDispositionFreshActivationRequired ||
		stored.RuntimeActivationStatus.Incompatibility.Reason != runtimecontract.RuntimeStateIncompatibilityReasonCWILedgerInvalid ||
		stored.RuntimeActivationStatus.Incompatibility.ActualSchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent ||
		stored.RuntimeActivationStatus.Incompatibility.SupportedSchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent ||
		stored.RuntimeActivationStatus.RuntimeStateRef != runevents.RuntimeStateRef(run.WorkspaceID, run.ID, ambiguous.StateRevision) ||
		stored.RuntimeState == nil || stored.RuntimeState.SchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent ||
		stored.RuntimeState.StateRevision != ambiguous.StateRevision || len(stored.RuntimeState.Facts.CompletedFocuses) != 1 ||
		stored.RuntimeState.DriveLease != nil {
		t.Fatalf("current-schema duplicate CWI drive did not preserve fail-closed Run boundary: %+v", stored)
	}
	llmCalls.assertNone(t)
	fileCalls.assertNone(t)
	firstRunRevision := stored.Revision

	_, err = module.driveRuntime(ctx, command)
	var fresh *runtimeFreshActivationRequiredError
	if !errors.As(err, &fresh) || fresh.RuntimeStateRef != runevents.RuntimeStateRef(run.WorkspaceID, run.ID, ambiguous.StateRevision) {
		t.Fatalf("direct current-schema fresh-activation boundary error=%v fresh=%+v", err, fresh)
	}
	if err := module.scheduleRuntimeDrive(ctx, run.WorkspaceID, run.ID, "repeat-resume-current-schema-cwi-ledger-incompatible"); err != nil {
		t.Fatalf("repeat schedule current-schema ambiguous drive: %v", err)
	}
	stored, err = runs.GetByWorkspaceID(ctx, run.WorkspaceID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Revision != firstRunRevision || stored.RuntimeActivationStatus == nil ||
		stored.RuntimeState == nil || stored.RuntimeState.SchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent ||
		len(stored.RuntimeState.Facts.CompletedFocuses) != 1 || stored.RuntimeState.DriveLease != nil {
		t.Fatalf("repeat current-schema ambiguous drive mutated Run or retried authority: %+v", stored)
	}
	llmCalls.assertNone(t)
	fileCalls.assertNone(t)
}

// runtimeLLMCallObserver records every Runtime-specific LLM entry point. The
// persisted aggregate gate must run before SM-2 capability acquisition and
// before every SM-3 provider preflight/request exchange.
type runtimeLLMCallObserver struct {
	mu    sync.Mutex
	calls map[string]int
}

func observeRuntimeLLMCalls(t *testing.T, hub magicevent.Hub) *runtimeLLMCallObserver {
	t.Helper()
	spy := &runtimeLLMCallObserver{calls: make(map[string]int)}
	observer := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	topics := []string{
		llmevents.TopicAcquireRuntimeCapabilities,
		llmevents.TopicAcquireProviderCeiling,
		llmevents.TopicPreflightRuntimeCandidate,
		llmevents.TopicRequestRuntimeCandidate,
	}
	observe := func(event magicevent.Event, result magicevent.Result) {
		spy.mu.Lock()
		spy.calls[event.ID()]++
		spy.mu.Unlock()
		if result != nil {
			result.Set(nil, nil)
		}
	}
	for _, topic := range topics {
		observer.Subscribe(topic, observe)
	}
	t.Cleanup(func() {
		for _, topic := range topics {
			observer.Unsubscribe(topic)
		}
	})
	return spy
}

func (s *runtimeLLMCallObserver) assertNone(t *testing.T) {
	t.Helper()
	if s == nil {
		t.Fatal("missing Runtime LLM call observer")
	}
	s.mu.Lock()
	calls := make(map[string]int, len(s.calls))
	for topic, count := range s.calls {
		calls[topic] = count
	}
	s.mu.Unlock()
	if len(calls) != 0 {
		t.Fatalf("invalid CWI ledger reached Runtime LLM owner: %+v", calls)
	}
}

// runtimeFileCallObserver records every File Runtime entry point reachable
// from SM-2/SM-3/SM-4. It is test-only: it cannot grant authority or alter
// File owner state, and proves the persisted-state boundary is before all
// capability owner I/O.
type runtimeFileCallObserver struct {
	mu    sync.Mutex
	calls map[string]int
}

func observeRuntimeFileCalls(t *testing.T, hub magicevent.Hub) *runtimeFileCallObserver {
	t.Helper()
	spy := &runtimeFileCallObserver{calls: make(map[string]int)}
	ownerObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	prerequisiteObserver := magicevent.NewSimpleObserver(fileevents.PrerequisiteCommandDestination, hub)
	ownerTopics := []string{
		fileevents.TopicAcquireRuntimeCapabilities,
		fileevents.TopicAcquireRuntimeOperationAdmissibility,
		fileevents.TopicProbeRuntimeOperationFeasibility,
		fileevents.TopicProbeRuntimeSemanticProofFeasibility,
		fileevents.TopicQueryRuntimeCWIReconciliation,
		fileevents.TopicExecuteFileListOperation,
		fileevents.TopicExecuteFileReadOperation,
		fileevents.TopicExecuteFileEditOperation,
		fileevents.TopicExecuteFileWriteOperation,
		fileevents.TopicExecutePatchApplyOperation,
		fileevents.TopicExecutePatchPreviewOperation,
	}
	prerequisiteTopics := []string{
		fileevents.TopicAcquireTargetSnapshot,
		fileevents.TopicAcquireRuntimeTargetSelection,
		fileevents.TopicAcquireWorkspaceManifest,
		fileevents.TopicConfirmRuntimeRepairSurface,
	}
	observe := func(event magicevent.Event, result magicevent.Result) {
		spy.mu.Lock()
		spy.calls[event.ID()]++
		spy.mu.Unlock()
		if result != nil {
			result.Set(nil, nil)
		}
	}
	for _, topic := range ownerTopics {
		ownerObserver.Subscribe(topic, observe)
	}
	for _, topic := range prerequisiteTopics {
		prerequisiteObserver.Subscribe(topic, observe)
	}
	t.Cleanup(func() {
		for _, topic := range ownerTopics {
			ownerObserver.Unsubscribe(topic)
		}
		for _, topic := range prerequisiteTopics {
			prerequisiteObserver.Unsubscribe(topic)
		}
	})
	return spy
}

func (s *runtimeFileCallObserver) assertNone(t *testing.T) {
	t.Helper()
	if s == nil {
		t.Fatal("missing File Runtime call observer")
	}
	s.mu.Lock()
	calls := make(map[string]int, len(s.calls))
	for topic, count := range s.calls {
		calls[topic] = count
	}
	s.mu.Unlock()
	if len(calls) != 0 {
		t.Fatalf("invalid CWI ledger reached File Runtime owner: %+v", calls)
	}
}

func TestModuleConfigureRequiresCoreDependencies(t *testing.T) {
	base := moduleContractDependencies()
	tests := []struct {
		name string
		deps Dependencies
	}{
		{name: "missing tool calls", deps: func() Dependencies { d := base; d.ToolCalls = nil; return d }()},
		{name: "missing patches", deps: func() Dependencies { d := base; d.Patches = nil; return d }()},
		{name: "missing validation results", deps: func() Dependencies { d := base; d.ValidationResults = nil; return d }()},
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

func TestScheduleRuntimeDriveCoalescesDuplicateRunRequests(t *testing.T) {
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(newRuntimeTestOwner(), nil, background)
	if err := module.scheduleRuntimeDrive(context.Background(), "ws-drive-singleflight", "run-drive-singleflight", "first"); err != nil {
		t.Fatal(err)
	}
	if err := module.scheduleRuntimeDrive(context.Background(), "ws-drive-singleflight", "run-drive-singleflight", "duplicate"); err != nil {
		t.Fatal(err)
	}
	if background.asyncFunctionCalls != 1 {
		t.Fatalf("background calls = %d, want one coalesced drive", background.asyncFunctionCalls)
	}
	if !module.beginRuntimeDrive("ws-drive-singleflight\x00run-other") {
		t.Fatal("different run was unexpectedly coalesced")
	}
}

func TestScheduleRuntimeDriveReleasesKeyWhenSchedulingFails(t *testing.T) {
	background := &recordingBackgroundRoutine{err: errors.New("scheduler unavailable")}
	module := newTestBiz(newRuntimeTestOwner(), nil, background)
	for attempt := 0; attempt < 2; attempt++ {
		if err := module.scheduleRuntimeDrive(context.Background(), "ws-drive-retry", "run-drive-retry", "retry"); !errors.Is(err, background.err) {
			t.Fatalf("attempt %d err = %v, want scheduler error", attempt, err)
		}
	}
	if background.asyncFunctionCalls != 2 {
		t.Fatalf("background calls = %d, want scheduling retried after key release", background.asyncFunctionCalls)
	}
}

func TestPersistedRuntimeRecoveryLeaseCoalescesResumeAndTimerChains(t *testing.T) {
	ctx := context.Background()
	initial := runtimeAggregate(t, 1)
	repository := &inMemoryRuntimeStateRepository{aggregate: &initial}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	background := &recordingBackgroundRoutine{}
	hub := magicevent.NewHub(8)
	module := newTestBiz(owner, hub, background)
	t.Cleanup(func() { hub.Terminate(context.Background()) })

	key := runtimeDriveKey(initial.Facts.WorkspaceID, initial.Facts.RunID)
	if !module.beginRuntimeDrive(key) {
		t.Fatal("expected initial drive gate")
	}
	request := runtimeRecoveryLeaseRequest{
		Reason: runtimecontract.RuntimeDriveLeaseReasonCASReload, FailureSignature: "cas/1/2", Delay: time.Hour, Source: "test",
	}
	if !module.scheduleRuntimeRecoveryLease(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, request) {
		t.Fatal("initial persisted recovery lease was not scheduled")
	}
	if !module.scheduleRuntimeRecoveryLease(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, request) {
		t.Fatal("duplicate persisted recovery lease was not recognized")
	}
	if background.asyncFunctionCalls != 1 {
		t.Fatalf("timer tasks = %d, want exactly one persisted lease timer", background.asyncFunctionCalls)
	}
	if err := module.scheduleRuntimeDrive(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, "duplicate_resume"); err != nil {
		t.Fatal(err)
	}
	if background.asyncFunctionCalls != 1 {
		t.Fatalf("duplicate resume scheduled a parallel drive/timer: %d", background.asyncFunctionCalls)
	}
	if repository.aggregate.DriveLease == nil || repository.aggregate.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled || repository.aggregate.DriveLease.RecoveryGeneration != 1 {
		t.Fatalf("persisted recovery lease = %+v", repository.aggregate.DriveLease)
	}
}

func TestSemanticDocumentPendingSchedulesOnePersistedLeaseWithoutBusyLoop(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	bootstrap := runtimeBootstrap()
	bootstrap.Activation.ExecutionContract.CandidateDocumentRecovery = runtimecontract.RuntimeCandidateDocumentRecoveryPolicy{MaxAttempts: 2}
	runs := memoryrepo.NewRunRepository()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-semantic-pending", WorkspaceID: "ws-semantic-pending",
		GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
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

	repository := runtimeStateRepository{hub: hub}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := bootstrap.InitialFacts("ws-semantic-pending", "run-semantic-pending")
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, bootstrap.Definition, bootstrap.Requirements)
	if err != nil {
		t.Fatal(err)
	}
	prepared := initialized
	prepared.Facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Selection: runtimecontract.ProviderInferenceSelection{Target: prepared.Facts.Activation.ProviderSelection.Target}, ContextWindowTokens: 8192,
		ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048, MaxInputTokens: 4096, MaxBytes: 65536,
		CapabilityRevision: "semantic-capability-v1", ConfigurationRevision: "semantic-config-v1",
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "semantic-file-catalog", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt"}, ObservedRevision: "semantic-file-v1",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	prepared.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	prepared.StateRevision++
	if err := repository.Save(ctx, prepared, initialized.StateRevision); err != nil {
		t.Fatal(err)
	}

	requestCount := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requestCount++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		outcome := llmevents.RuntimeProviderDocumentOutcome{
			Framing: runtimecontract.RuntimeProviderDocumentFraming{
				Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: command.ExpectedDocument.Hash,
				ResponseHash: runtimecontract.StableRuntimeProviderResponseHash([]byte(`{"proposals":[`)), ResponseBytes: int64(len(`{"proposals":[`)),
				Code: "max_output_tokens", Signature: "provider-output-limit",
			},
		}
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
			DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: command.ExpectedDocument.Hash,
			DocumentOutcome:      &outcome,
		}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(owner, hub, background)
	if err := module.scheduleRuntimeDrive(ctx, "ws-semantic-pending", "run-semantic-pending", "semantic-pending"); err != nil {
		t.Fatal(err)
	}
	queuedDrive := background.lastAsyncFunction
	if queuedDrive == nil || background.asyncFunctionCalls != 1 {
		t.Fatalf("initial semantic drive scheduling calls=%d", background.asyncFunctionCalls)
	}
	queuedDrive()
	if requestCount != 1 || background.asyncFunctionCalls != 2 {
		t.Fatalf("semantic pending requests=%d background calls=%d, want one provider request and one lease timer", requestCount, background.asyncFunctionCalls)
	}
	persisted, err := owner.loadRuntime(ctx, "ws-semantic-pending", "run-semantic-pending")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.DriveLease == nil || persisted.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		persisted.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonPersistedPending ||
		persisted.Facts.Recovery != prepared.Facts.Recovery || len(persisted.CandidateAttempts) != 0 || persisted.ActiveDispatch != nil {
		t.Fatalf("semantic pending lease/authority=%+v", persisted)
	}
	if err := module.scheduleRuntimeDrive(ctx, "ws-semantic-pending", "run-semantic-pending", "duplicate-resume"); err != nil {
		t.Fatal(err)
	}
	if requestCount != 1 || background.asyncFunctionCalls != 2 {
		t.Fatalf("duplicate resume created a second semantic chain: requests=%d background=%d", requestCount, background.asyncFunctionCalls)
	}
}

func TestScheduleRuntimeDriveSeparatesSameRunIDAcrossWorkspaces(t *testing.T) {
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(newRuntimeTestOwner(), nil, background)
	for _, workspaceID := range []common.WorkspaceID{"ws-drive-a", "ws-drive-b"} {
		if err := module.scheduleRuntimeDrive(context.Background(), workspaceID, "run-shared", "workspace-isolation"); err != nil {
			t.Fatal(err)
		}
		if err := module.scheduleRuntimeDrive(context.Background(), workspaceID, "run-shared", "duplicate"); err != nil {
			t.Fatal(err)
		}
	}
	if background.asyncFunctionCalls != 2 {
		t.Fatalf("workspace-scoped drives=%d, want two independent chains", background.asyncFunctionCalls)
	}
}

func TestScheduledRuntimeDriveReleasesSingleflightKeyWhenOwnerStopsBeforeTaskStarts(t *testing.T) {
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(newRuntimeTestOwner(), nil, background)
	const driveKey = "ws-drive-stopped\x00run-drive-stopped"
	if err := module.scheduleRuntimeDrive(context.Background(), "ws-drive-stopped", "run-drive-stopped", "late-start"); err != nil {
		t.Fatal(err)
	}
	if background.lastAsyncFunction == nil {
		t.Fatal("expected queued Runtime drive")
	}
	// Model the lifecycle state observed by a queued task after Stop has
	// acquired its write lock and cleared the owner authority.
	module.lifecycleMu.Lock()
	module.runtimeController = nil
	module.lifecycleMu.Unlock()
	background.lastAsyncFunction()

	module.driveMu.Lock()
	_, queued := module.drivingRuns[driveKey]
	module.driveMu.Unlock()
	if queued {
		t.Fatal("late-start drive retained its singleflight key after owner shutdown")
	}
}

func TestStopWaitsForRuntimeDriveLifecycleReadLockBeforeClearingDependencies(t *testing.T) {
	module := newTestBiz(newRuntimeTestOwner(), nil, nil)
	entered := make(chan struct{})
	release := make(chan struct{})
	go func() {
		module.lifecycleMu.RLock()
		close(entered)
		<-release
		module.lifecycleMu.RUnlock()
	}()
	<-entered

	stopped := make(chan error, 1)
	go func() { stopped <- module.Stop(context.Background()) }()
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned before active Runtime drive left lifecycle read lock: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if module.runtimeController != nil || module.evidenceRecords != nil {
		t.Fatalf("Stop retained owner dependencies: controller=%+v evidence=%+v", module.runtimeController, module.evidenceRecords)
	}
}

func TestStopWaitsForQueuedDecisionEvidenceHandlerBeforeClearingDependencies(t *testing.T) {
	base := memoryrepo.NewEvidenceRecordRepository()
	blocking := &blockingEvidenceRecordRepository{
		EvidenceRecordRepository: base,
		createEntered:            make(chan struct{}),
		releaseCreate:            make(chan struct{}),
	}
	module := newTestBiz(nil, nil, nil)
	module.evidenceRecords = blocking
	event := magicevent.NewEventWithContext(
		decisionevents.TopicDecisionCommandRecordEvidence,
		decisioncommon.ModuleID,
		ModuleID,
		magicevent.NewHeader(),
		context.Background(),
		decisionevents.RecordDecisionEvidenceCommand{
			EvidenceRecordID: "evidence-lifecycle", WorkspaceID: "ws-lifecycle", RunID: "run-lifecycle",
			TicketID: "ticket-lifecycle", ResolutionID: "resolution-lifecycle", ResolutionType: "continue",
			DecisionKind: "runtime_governance", Reason: "test lifecycle lock",
		},
	)
	handled := make(chan struct{})
	go func() {
		module.handleDecisionEvidenceCommand(event, nil)
		close(handled)
	}()
	<-blocking.createEntered

	stopped := make(chan error, 1)
	go func() { stopped <- module.Stop(context.Background()) }()
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned while queued evidence handler retained owner dependencies: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(blocking.releaseCreate)
	<-handled
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if module.evidenceRecords != nil || module.runtimeController != nil {
		t.Fatalf("Stop retained owner dependencies after queued handler: evidence=%+v controller=%+v", module.evidenceRecords, module.runtimeController)
	}
	items, err := base.ListByRun(common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-lifecycle"), "run-lifecycle")
	if err != nil || len(items) != 1 || items[0].Kind != runtimecontract.EvidenceKindDecision {
		t.Fatalf("queued evidence persistence=%+v err=%v", items, err)
	}
}

func TestStopWaitsForQueuedRuntimeOperationReceiptHandlerBeforeClearingDependencies(t *testing.T) {
	repository := &blockingRuntimeStateRepository{loadEntered: make(chan struct{}), releaseLoad: make(chan struct{})}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, nil, nil)
	event := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt,
		"test",
		ModuleID,
		magicevent.NewHeader(),
		context.Background(),
		orchestratorevents.QueryRuntimeOperationReceiptCommand{
			WorkspaceID: "ws-lifecycle", RunID: "run-lifecycle", DispatchID: "dispatch-lifecycle",
			OperationID: "operation-lifecycle", ExecutionKey: "execution-lifecycle", Fence: 1,
		},
	)
	handled := make(chan struct{})
	go func() {
		module.handleRuntimeOperationReceipt(event, magicevent.NewResult(event.ID(), event.Source(), event.Destination()))
		close(handled)
	}()
	<-repository.loadEntered

	stopped := make(chan error, 1)
	go func() { stopped <- module.Stop(context.Background()) }()
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned while queued operation-receipt handler retained owner dependencies: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(repository.releaseLoad)
	<-handled
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if module.runtimeController != nil {
		t.Fatalf("Stop retained Runtime controller after queued receipt handler: %+v", module.runtimeController)
	}
}

func TestStopWaitsForQueuedRuntimeGovernanceResolutionHandlerBeforeClearingDependencies(t *testing.T) {
	repository := &blockingRuntimeStateRepository{loadEntered: make(chan struct{}), releaseLoad: make(chan struct{})}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, nil, nil)
	event := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution,
		decisioncommon.ModuleID,
		ModuleID,
		magicevent.NewHeader(),
		context.Background(),
		orchestratorevents.CommitRuntimeGovernanceResolutionCommand{Resolution: runtimecontract.RuntimeGovernanceResolution{
			ResolutionID: "resolution-lifecycle", TicketID: "ticket-lifecycle", WorkspaceID: "ws-lifecycle", RunID: "run-lifecycle",
			AuthoritySnapshotID: "authority-lifecycle", DispatchID: "dispatch-lifecycle", DependencyKind: "decision", SubjectRef: "subject-lifecycle",
			Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "continue lifecycle test",
		}},
	)
	handled := make(chan struct{})
	go func() {
		module.handleRuntimeGovernanceResolution(event, magicevent.NewResult(event.ID(), event.Source(), event.Destination()))
		close(handled)
	}()
	<-repository.loadEntered

	stopped := make(chan error, 1)
	go func() { stopped <- module.Stop(context.Background()) }()
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned while queued governance handler retained owner dependencies: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(repository.releaseLoad)
	<-handled
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if module.runtimeController != nil {
		t.Fatalf("Stop retained Runtime controller after queued governance handler: %+v", module.runtimeController)
	}
}

type blockingEvidenceRecordRepository struct {
	repository.EvidenceRecordRepository
	createEntered chan struct{}
	releaseCreate chan struct{}
}

func (r *blockingEvidenceRecordRepository) Create(ctx context.Context, item *runtimecontract.EvidenceRecord) error {
	close(r.createEntered)
	<-r.releaseCreate
	return r.EvidenceRecordRepository.Create(ctx, item)
}

type blockingRuntimeStateRepository struct {
	loadEntered chan struct{}
	releaseLoad chan struct{}
}

func (r *blockingRuntimeStateRepository) Load(context.Context, common.WorkspaceID, common.RunID) (runtime.RuntimeAggregate, error) {
	close(r.loadEntered)
	<-r.releaseLoad
	return runtime.RuntimeAggregate{}, common.ErrNotFound
}

func (*blockingRuntimeStateRepository) Save(context.Context, runtime.RuntimeAggregate, uint64) error {
	return nil
}

func moduleContractDependencies() Dependencies {
	rawHub := magicevent.NewHub(32)
	return Dependencies{
		ToolCalls:             memoryrepo.NewToolCallRepository(),
		Patches:               memoryrepo.NewPatchRepository(),
		ValidationResults:     memoryrepo.NewValidationResultRepository(),
		EvidenceRecords:       memoryrepo.NewEvidenceRecordRepository(),
		CoverageWorkItems:     memoryrepo.NewCoverageWorkItemRepository(),
		ArtifactContinuations: memoryrepo.NewArtifactContinuationRepository(),
		CommandHub:            rawHub,
	}
}

type recordingBackgroundRoutine struct {
	asyncFunctionCalls int
	lastAsyncFunction  func()
	err                error
}

// startupReadyRecordingBackgroundRoutine observes scheduling without running
// the drive. It makes the startup handoff test independent from capability
// owner setup and avoids a test-local second recovery path.
type startupReadyRecordingBackgroundRoutine struct {
	recordingBackgroundRoutine
	scheduled chan struct{}
}

func (r *startupReadyRecordingBackgroundRoutine) AsyncFunction(function func()) error {
	if function == nil {
		return errors.New("function is nil")
	}
	select {
	case r.scheduled <- struct{}{}:
	default:
	}
	return nil
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
