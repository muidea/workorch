package run

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func newRunModuleTestHub(t *testing.T) magicevent.Hub {
	t.Helper()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	return hub
}

func TestBuildRuntimeRecoveryInventoryContainsOnlyRunOwnedOpaqueFacts(t *testing.T) {
	bootstrapOnly := runtimecontract.RuntimeBootstrap{SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent}
	runs := []*runmodel.Run{
		// A bootstrap-only activation is a valid SM-4 initialization recovery
		// target even before the first aggregate record has been persisted.
		{ID: "run-bootstrap-only", WorkspaceID: "ws-0", State: common.RunStateActive, RuntimeBootstrap: &bootstrapOnly},
		{ID: "run-without-bootstrap", WorkspaceID: "ws-0", State: common.RunStateActive,
			RuntimeState: &runtimecontract.RuntimeStateRecord{StateRevision: 1, Decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange}}},
		{ID: "run-active", WorkspaceID: "ws-1", State: common.RunStateActive, RuntimeBootstrap: &bootstrapOnly,
			RuntimeState: &runtimecontract.RuntimeStateRecord{StateRevision: 1, Decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange}}},
		{ID: "run-sealed", WorkspaceID: "ws-1", State: common.RunStateActive, RuntimeBootstrap: &bootstrapOnly,
			RuntimeState: &runtimecontract.RuntimeStateRecord{StateRevision: 2, Decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange}, ActiveDispatch: &runtimecontract.RuntimeDispatchRecord{ID: "dispatch-1"}}},
		{ID: "run-blocked", WorkspaceID: "ws-2", State: common.RunStateBlockedOnDecision, BlockingTicketID: "ticket-1", RuntimeBootstrap: &bootstrapOnly,
			RuntimeState: &runtimecontract.RuntimeStateRecord{StateRevision: 3, Decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedGovernance}}},
		{ID: "run-terminal-pending", WorkspaceID: "ws-3", State: common.RunStateActive, RuntimeBootstrap: &bootstrapOnly,
			RuntimeState: &runtimecontract.RuntimeStateRecord{StateRevision: 4, Decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateComplete}, Facts: runtimecontract.RuntimeFacts{Recovery: runtimecontract.RuntimeRecoveryLedger{TerminalDeliveryAttempts: 2}}}},
		{ID: "run-fresh-activation-required", WorkspaceID: "ws-4", State: common.RunStateActive, RuntimeBootstrap: &bootstrapOnly,
			RuntimeState:            &runtimecontract.RuntimeStateRecord{SchemaVersion: 2, StateRevision: 5},
			RuntimeActivationStatus: &runmodel.RuntimeActivationStatus{Disposition: runtimecontract.RuntimeActivationDispositionFreshActivationRequired, RuntimeStateRef: "runtime-state/ws-4/run-fresh-activation-required/5", ActualSchemaVersion: 2, SupportedSchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent}},
	}
	result := runbiz.BuildRuntimeRecoveryInventory(runs, runevents.QueryRuntimeRecoveryInventoryCommand{Limit: 10})
	if len(result.Items) != 6 {
		t.Fatalf("recovery items = %+v", result.Items)
	}
	items := map[common.RunID]runevents.RuntimeRecoveryInventoryItem{}
	for _, item := range result.Items {
		items[item.RunID] = item
	}
	if _, ok := items["run-without-bootstrap"]; ok {
		t.Fatalf("run without typed bootstrap unexpectedly entered recovery inventory: %+v", items["run-without-bootstrap"])
	}
	bootstrap := items["run-bootstrap-only"]
	if bootstrap.State != common.RunStateActive || !bootstrap.HasRuntimeBootstrap || bootstrap.HasRuntimeState || bootstrap.RuntimeStateRef != "" {
		t.Fatalf("bootstrap-only inventory = %+v", bootstrap)
	}
	sealed := items["run-sealed"]
	if sealed.State != common.RunStateActive || !sealed.HasRuntimeState || sealed.RuntimeStateRevision != 2 || sealed.RuntimeStateRef != "runtime-state/ws-1/run-sealed/2" {
		t.Fatalf("sealed opaque inventory = %+v", sealed)
	}
	blocked := items["run-blocked"]
	if blocked.State != common.RunStateBlockedOnDecision || !blocked.HasRuntimeState || blocked.RuntimeStateRevision != 3 {
		t.Fatalf("blocked lifecycle inventory = %+v", blocked)
	}
	fresh := items["run-fresh-activation-required"]
	if fresh.RuntimeActivationDisposition != runtimecontract.RuntimeActivationDispositionFreshActivationRequired || fresh.RuntimeStateSchemaVersion != 2 {
		t.Fatalf("fresh-activation inventory = %+v", fresh)
	}
}

func sendRunTestCommand[Command any](ctx context.Context, rawHub magicevent.Hub, topic string, command Command) (runevents.RunCommandResult, error) {
	ev := magicevent.NewEventWithContext(topic, "run/module_test", runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := rawHub.Send(ev)
	if result == nil {
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return runevents.RunCommandResult{}, err.Cause
		}
		return runevents.RunCommandResult{}, err
	}
	data, err := result.Get()
	if err != nil {
		return runevents.RunCommandResult{}, err
	}
	response, ok := data.(runevents.RunCommandResult)
	if !ok {
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
	return response, nil
}

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if module.bizPtr != nil {
		t.Fatal("new module biz pointer is non-nil before Configure")
	}
	rawHub := newRunModuleTestHub(t)
	if err := module.Configure(runbiz.Dependencies{
		Runs:        memory.NewRunRepository(),
		Steps:       memory.NewStepRepository(),
		Checkpoints: memory.NewRuntimeCheckpointRepository(),
		EventHub:    rawHub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil {
		t.Fatal("biz pointer is nil after Configure")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestGMMR1SM4RecoveryInventoryAvailableAfterSetupBeforeRun(t *testing.T) {
	ctx := context.Background()
	rawHub := newRunModuleTestHub(t)
	module := New()
	if err := module.Configure(runbiz.Dependencies{Runs: memory.NewRunRepository(), EventHub: rawHub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	// plugin.Setup invokes this immediately after Configure. Do not call Start:
	// Orchestrator Run precedes Run Run under the framework's current weights.
	if err := module.prepareRuntimeRecoveryInventory(); err != nil {
		t.Fatalf("Setup recovery subscription: %v", err)
	}
	result := rawHub.Send(magicevent.NewEventWithContext(
		runevents.TopicRunCommandRuntimeRecoveryInventory, "run/module_test", runevents.CommandDestination,
		magicevent.NewHeader(), ctx, runevents.QueryRuntimeRecoveryInventoryCommand{},
	))
	if result == nil {
		t.Fatal("Setup recovery inventory query returned nil result")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("Setup recovery inventory query: %v", err)
	}
	if _, ok := data.(runevents.QueryRuntimeRecoveryInventoryResult); !ok {
		t.Fatalf("Setup recovery inventory result type = %T", data)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	result = rawHub.Send(magicevent.NewEventWithContext(
		runevents.TopicRunCommandRuntimeRecoveryInventory, "run/module_test", runevents.CommandDestination,
		magicevent.NewHeader(), ctx, runevents.QueryRuntimeRecoveryInventoryCommand{},
	))
	if result != nil {
		if _, err := result.Get(); err == nil {
			t.Fatal("recovery inventory remained available after Stop")
		}
	}
}

func TestGMMR3RuntimeStateRecordReadIsAvailableDuringSetupWithoutRegularGet(t *testing.T) {
	ctx := context.Background()
	rawHub := newRunModuleTestHub(t)
	runs := memory.NewRunRepository()
	record := runtimecontract.RuntimeStateRecord{SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent, StateRevision: 7}
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-setup-record", WorkspaceID: "ws-setup-record", State: common.RunStateActive, RuntimeState: &record,
	}); err != nil {
		t.Fatal(err)
	}
	module := New()
	if err := module.Configure(runbiz.Dependencies{Runs: runs, EventHub: rawHub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	readinessResult := rawHub.Send(magicevent.NewEventWithContext(
		runevents.TopicRunCommandRuntimeStateRecordReadiness, "run/module_test", runevents.CommandDestination,
		magicevent.NewHeader(), ctx, runevents.QueryRuntimeStateRecordReadinessCommand{},
	))
	if readinessResult == nil {
		t.Fatal("Setup readiness query returned nil result")
	}
	readinessData, err := readinessResult.Get()
	if err != nil {
		t.Fatalf("Setup readiness query: %v", err)
	}
	if readiness, ok := readinessData.(runevents.QueryRuntimeStateRecordReadinessResult); !ok || readiness.Ready {
		t.Fatalf("Setup readiness = %+v, want ready=false", readinessData)
	}

	stateResult := rawHub.Send(magicevent.NewEventWithContext(
		runevents.TopicRunCommandRuntimeStateRecord, "run/module_test", runevents.CommandDestination,
		magicevent.NewHeader(), ctx, runevents.QueryRuntimeStateRecordCommand{
			WorkspaceID: "ws-setup-record", RunID: "run-setup-record",
			ExpectedRuntimeStateRef: runevents.RuntimeStateRef("ws-setup-record", "run-setup-record", 7), ExpectedRuntimeStateRevision: 7,
		},
	))
	if stateResult == nil {
		t.Fatal("Setup runtime state record query returned nil result")
	}
	stateData, err := stateResult.Get()
	if err != nil {
		t.Fatalf("Setup runtime state record query: %v", err)
	}
	state, ok := stateData.(runevents.QueryRuntimeStateRecordResult)
	if !ok || !state.StatePresent || state.RuntimeState == nil || state.RuntimeStateRevision != 7 ||
		state.RuntimeStateRef != runevents.RuntimeStateRef("ws-setup-record", "run-setup-record", 7) {
		t.Fatalf("Setup runtime state record = %+v", stateData)
	}
	// The ordinary Run command surface is intentionally unavailable until
	// Start. Startup recovery must therefore use the narrow state-record read.
	regularGet := rawHub.Send(magicevent.NewEventWithContext(
		runevents.TopicRunCommandGet, "run/module_test", runevents.CommandDestination,
		magicevent.NewHeader(), ctx, runevents.GetRunCommand{WorkspaceID: "ws-setup-record", RunID: "run-setup-record"},
	))
	if regularGet != nil {
		if _, err := regularGet.Get(); err == nil {
			t.Fatal("regular Run Get unexpectedly succeeded during Setup")
		}
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	readinessResult = rawHub.Send(magicevent.NewEventWithContext(
		runevents.TopicRunCommandRuntimeStateRecordReadiness, "run/module_test", runevents.CommandDestination,
		magicevent.NewHeader(), ctx, runevents.QueryRuntimeStateRecordReadinessCommand{},
	))
	if readinessResult == nil {
		t.Fatal("Run readiness query returned nil result after Start")
	}
	readinessData, err = readinessResult.Get()
	if err != nil {
		t.Fatalf("Run readiness query after Start: %v", err)
	}
	if readiness, ok := readinessData.(runevents.QueryRuntimeStateRecordReadinessResult); !ok || !readiness.Ready {
		t.Fatalf("Run readiness = %+v, want ready=true", readinessData)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestModuleStopWaitsForInFlightOwnerCommand(t *testing.T) {
	ctx := context.Background()
	rawHub := newRunModuleTestHub(t)
	baseRuns := memory.NewRunRepository()
	run := &runmodel.Run{ID: "run-stop-in-flight", WorkspaceID: "ws-stop-in-flight", State: common.RunStateActive, CreatedAt: "2026-07-16T00:00:00Z"}
	if err := baseRuns.Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	runs := &blockingRunUpdateRepository{
		RunRepository: baseRuns,
		started:       make(chan struct{}),
		release:       make(chan struct{}),
	}
	module := New()
	if err := module.Configure(runbiz.Dependencies{Runs: runs, EventHub: rawHub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	commandDone := make(chan error, 1)
	go func() {
		_, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandUpdate, runevents.UpdateRunCommand{
			RunID: run.ID, WorkspaceID: run.WorkspaceID,
		})
		commandDone <- err
	}()
	select {
	case <-runs.started:
	case <-time.After(time.Second):
		t.Fatal("update command did not reach repository")
	}
	stopDone := make(chan error, 1)
	go func() { stopDone <- module.Stop(ctx) }()
	select {
	case err := <-stopDone:
		t.Fatalf("Stop returned before in-flight command completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(runs.release)
	if err := <-commandDone; err != nil {
		t.Fatalf("in-flight command: %v", err)
	}
	if err := <-stopDone; err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

type blockingRunUpdateRepository struct {
	repository.RunRepository
	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func (r *blockingRunUpdateRepository) CompareAndSwap(ctx context.Context, run *runmodel.Run, expectedRevision uint64) error {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return r.RunRepository.CompareAndSwap(ctx, run, expectedRevision)
}

func TestModuleConfigureRequiresRunRepositoryAndEventHub(t *testing.T) {
	tests := []struct {
		name string
		deps runbiz.Dependencies
	}{
		{name: "missing runs", deps: runbiz.Dependencies{EventHub: magicevent.NewHub(16)}},
		{name: "missing event hub", deps: runbiz.Dependencies{Runs: memory.NewRunRepository()}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}

func TestRunCommandGetHonorsWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	rawHub := newRunModuleTestHub(t)
	runs := memory.NewRunRepository()
	runID := common.RunID("run-scoped-get")
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-run-owner-a",
		State:       common.RunStatePending,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-run-owner-b",
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("create duplicate scoped run: %v", err)
	}

	module := New()
	if err := module.Configure(runbiz.Dependencies{Runs: runs, EventHub: rawHub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	_, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandGet, runevents.GetRunCommand{
		WorkspaceID: "ws-run-owner-c",
		RunID:       runID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get err = %v, want not found", err)
	}

	response, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandGet, runevents.GetRunCommand{
		WorkspaceID: "ws-run-owner-b",
		RunID:       runID,
	})
	if err != nil {
		t.Fatalf("duplicate scoped get err = %v, want response", err)
	}
	item := response.Run
	if item == nil || item.WorkspaceID != "ws-run-owner-b" || item.State != common.RunStateActive {
		t.Fatalf("duplicate scoped response run = %+v, want workspace b running run", response.Run)
	}

	response, err = sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandGet, runevents.GetRunCommand{
		WorkspaceID: "ws-run-owner-a",
		RunID:       runID,
	})
	if err != nil {
		t.Fatalf("scoped get err = %v, want response", err)
	}
	item = response.Run
	if item == nil || item.WorkspaceID != "ws-run-owner-a" || item.ID != runID {
		t.Fatalf("response run = %+v, want scoped run", response.Run)
	}
}

func TestRunCommandListSupportsCompactStateFilter(t *testing.T) {
	ctx := context.Background()
	rawHub := newRunModuleTestHub(t)
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          "run-compact-running",
		WorkspaceID: "ws-run-compact",
		State:       common.RunStateActive,
		Priority:    3,
	}); err != nil {
		t.Fatalf("create running run: %v", err)
	}
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          "run-compact-completed",
		WorkspaceID: "ws-run-compact",
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create completed run: %v", err)
	}

	module := New()
	if err := module.Configure(runbiz.Dependencies{Runs: runs, EventHub: rawHub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	response, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandList, runevents.ListRunsCommand{
		States:  []common.RunState{common.RunStateActive},
		Compact: true,
	})
	if err != nil {
		t.Fatalf("list err = %v, want response", err)
	}
	items := response.Runs
	if len(items) != 1 || items[0].ID != "run-compact-running" {
		t.Fatalf("items = %+v, want only running run", items)
	}
	if items[0].Priority != 3 || items[0].WorkspaceID != "ws-run-compact" {
		t.Fatalf("compact item = %+v, want metadata", items[0])
	}
}

func TestRunCommandDeleteHonorsWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	rawHub := newRunModuleTestHub(t)
	runs := memory.NewRunRepository()
	runID := common.RunID("run-scoped-delete")
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-run-delete-a",
		State:       common.RunStatePending,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	module := New()
	if err := module.Configure(runbiz.Dependencies{Runs: runs, EventHub: rawHub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	_, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandDelete, runevents.DeleteRunCommand{
		WorkspaceID: "ws-run-delete-b",
		RunID:       runID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace delete err = %v, want not found", err)
	}
	if _, err := runs.Get(ctx, runID); err != nil {
		t.Fatalf("run deleted on wrong workspace: %v", err)
	}

	_, err = sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandDelete, runevents.DeleteRunCommand{
		WorkspaceID: "ws-run-delete-a",
		RunID:       runID,
	})
	if err != nil {
		t.Fatalf("scoped delete err = %v, want response", err)
	}
	if _, err := runs.Get(ctx, runID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("run still exists after scoped delete err = %v", err)
	}
}

func TestRunWriteCommandsRequireWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	rawHub := newRunModuleTestHub(t)
	runs := memory.NewRunRepository()
	runID := common.RunID("run-write-scope")
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-write-scope-a",
		State:       common.RunStatePending,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	module := New()
	if err := module.Configure(runbiz.Dependencies{Runs: runs, EventHub: rawHub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	_, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandUpdate, runevents.UpdateRunCommand{
		RunID: runID,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unscoped update err = %v, want invalid argument", err)
	}

	_, err = sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandDelete, runevents.DeleteRunCommand{
		RunID: runID,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unscoped delete err = %v, want invalid argument", err)
	}
}

func TestRunCreateCommandRequiresWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	rawHub := newRunModuleTestHub(t)
	runs := memory.NewRunRepository()
	module := New()
	if err := module.Configure(runbiz.Dependencies{Runs: runs, EventHub: rawHub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	_, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandCreate, runevents.CreateRunCommand{
		RunID: "run-create-scope",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create missing workspace err = %v, want invalid argument", err)
	}
}

func TestValidateRunUpdateProtectsRuntimeIdentity(t *testing.T) {
	definition := runtimecontract.RuntimeDefinitionContext{GoalID: "goal-v2", DoDID: "dod-v2", Revision: "definition-v2"}
	activation := runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-v2"}}
	bootstrap := runtimecontract.RuntimeBootstrap{Definition: definition, Activation: activation}
	state := runtimecontract.RuntimeStateRecord{
		StateRevision: 1,
		Definition:    definition,
		Facts: runtimecontract.RuntimeFacts{
			WorkspaceID:        "ws-v2-update",
			RunID:              "run-v2-update",
			DefinitionRevision: definition.Revision,
			Activation:         activation,
		},
	}
	current := &runmodel.Run{
		ID: "run-v2-update", WorkspaceID: "ws-v2-update", GoalID: definition.GoalID, DoDID: definition.DoDID,
		DefinitionRevision: definition.Revision, InitialStepKind: "llm/workspace_execution", RuntimeBootstrap: &bootstrap,
		RuntimeState: &state,
		State:        common.RunStateActive, CreatedAt: "2026-07-16T00:00:00Z",
	}
	revision := current.RuntimeState.StateRevision
	valid := runevents.UpdateRunCommand{RunID: current.ID, WorkspaceID: current.WorkspaceID, RuntimeState: current.RuntimeState, ExpectedRuntimeStateRevision: &revision}
	if err := runbiz.ValidateRunUpdate(current, valid); err != nil {
		t.Fatalf("valid Runtime update rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*runevents.UpdateRunCommand)
	}{
		{name: "runtime state removal", mutate: func(cmd *runevents.UpdateRunCommand) { cmd.RuntimeState = nil }},
		{name: "state definition drift", mutate: func(cmd *runevents.UpdateRunCommand) {
			cloned := cmd.RuntimeState.Clone()
			cloned.Definition.Revision = "definition-other"
			cmd.RuntimeState = &cloned
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := valid
			tc.mutate(&cmd)
			if err := runbiz.ValidateRunUpdate(current, cmd); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("err = %v, want conflict", err)
			}
		})
	}
}

func TestRunStepReadCommandsHonorWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	rawHub := newRunModuleTestHub(t)
	runs := memory.NewRunRepository()
	steps := memory.NewStepRepository()
	runID := common.RunID("run-step-read-scope")
	stepID := common.StepID("step-read-scope")
	wsCtx := common.ContextWithRuntimeWorkspaceID(ctx, "ws-step-read-a")
	if err := runs.Create(wsCtx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-step-read-a",
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := steps.Create(wsCtx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateExecuting,
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}

	module := New()
	if err := module.Configure(runbiz.Dependencies{Runs: runs, Steps: steps, EventHub: rawHub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	_, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandGetStep, runevents.GetStepCommand{
		WorkspaceID: "ws-step-read-b",
		StepID:      stepID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get step err = %v, want not found", err)
	}

	_, err = sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandListStepsByRun, runevents.ListStepsByRunCommand{
		WorkspaceID: "ws-step-read-b",
		RunID:       runID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace list steps err = %v, want not found", err)
	}

	response, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandGetStep, runevents.GetStepCommand{
		WorkspaceID: "ws-step-read-a",
		StepID:      stepID,
	})
	if err != nil {
		t.Fatalf("scoped get step err = %v, want response", err)
	}
	item := response.Step
	if item == nil || item.ID != stepID || item.RunID != runID {
		t.Fatalf("response step = %+v, want scoped step", response.Step)
	}
}

func TestRunCheckpointReadCommandsHonorWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	rawHub := newRunModuleTestHub(t)
	runs := memory.NewRunRepository()
	checkpoints := memory.NewRuntimeCheckpointRepository()
	runID := common.RunID("run-checkpoint-read-scope")
	checkpointID := common.CheckpointID("checkpoint-read-scope")
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-checkpoint-read-a",
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := checkpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:          checkpointID,
		WorkspaceID: "ws-checkpoint-read-a",
		RunID:       runID,
	}); err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}

	module := New()
	if err := module.Configure(runbiz.Dependencies{Runs: runs, Checkpoints: checkpoints, EventHub: rawHub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	_, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandGetRuntimeCheckpoint, runevents.GetRuntimeCheckpointCommand{
		WorkspaceID:  "ws-checkpoint-read-b",
		CheckpointID: checkpointID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get checkpoint err = %v, want not found", err)
	}

	_, err = sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandListCheckpointsByRun, runevents.ListRuntimeCheckpointsByRunCommand{
		WorkspaceID: "ws-checkpoint-read-b",
		RunID:       runID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace list checkpoints err = %v, want not found", err)
	}

	response, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandGetRuntimeCheckpoint, runevents.GetRuntimeCheckpointCommand{
		WorkspaceID:  "ws-checkpoint-read-a",
		CheckpointID: checkpointID,
	})
	if err != nil {
		t.Fatalf("scoped get checkpoint err = %v, want response", err)
	}
	item := response.Checkpoint
	if item == nil || item.ID != checkpointID || item.RunID != runID || item.WorkspaceID != "ws-checkpoint-read-a" {
		t.Fatalf("response checkpoint = %+v, want scoped checkpoint", response.Checkpoint)
	}
}
