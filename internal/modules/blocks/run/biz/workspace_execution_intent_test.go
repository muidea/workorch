package run

import (
	"context"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWorkspaceExecutionResumeRetainsCurrentRunOpaqueCWILedger(t *testing.T) {
	biz, runs, hub := newLifecycleTestBiz(t)
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-1")
	bootstrap := restartTestBootstrap(t, "ws-1")
	state := runtimecontract.RuntimeStateRecord{
		SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
		StateRevision: 17,
		// Run treats aggregate contents as opaque. This marker represents a
		// current-run canonical CWI ledger that only Orchestrator may interpret.
		ProcessedEventIDs: []string{"cwi-ledger/settled/cwi-1"},
	}
	run := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1",
		State: common.RunStateActive, RuntimeBootstrap: &bootstrap, RuntimeState: &state,
		ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
	}
	if err := runs.Create(ctx, run); err != nil {
		t.Fatal(err)
	}

	continuations := make(chan runevents.RunContinuationRequestedData, 1)
	observer := magicevent.NewSimpleObserver(runevents.NotificationDestination, hub)
	observer.Subscribe(runevents.TopicRunContinuationRequested, func(event magicevent.Event, result magicevent.Result) {
		data, ok := event.Data().(runevents.RunContinuationRequestedData)
		if ok {
			continuations <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunContinuationRequested) })

	command := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: "ws-1", RunID: "run-1", RequestID: "resume-current-run",
		Kind:   runtimecontract.RuntimeExecutionIntentResume,
		GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1",
	}
	result := hub.Send(magicevent.NewEventWithContext(
		runevents.TopicRunCommandWorkspaceExecutionIntent,
		workspaceExecutionIntentRunSource,
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	admission, ok := data.(runevents.WorkspaceExecutionIntentResult)
	if !ok || admission.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested ||
		admission.RunID != run.ID || admission.Reason != runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted {
		t.Fatalf("resume admission = %+v", data)
	}

	select {
	case continuation := <-continuations:
		if continuation.WorkspaceID != run.WorkspaceID || continuation.RunID != run.ID {
			t.Fatalf("resume continuation switched Run scope: %+v", continuation)
		}
	case <-time.After(time.Second):
		t.Fatal("resume did not publish a current-run continuation fact")
	}

	stored, err := runs.GetByWorkspaceID(ctx, run.WorkspaceID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != common.RunStateActive || stored.Revision != 1 || stored.RuntimeState == nil ||
		stored.RuntimeState.SchemaVersion != state.SchemaVersion || stored.RuntimeState.StateRevision != state.StateRevision ||
		len(stored.RuntimeState.ProcessedEventIDs) != 1 || stored.RuntimeState.ProcessedEventIDs[0] != state.ProcessedEventIDs[0] ||
		stored.ActivationRequestID != "" || stored.ReconciliationScope != runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull {
		t.Fatalf("resume mutated current Run or opaque CWI ledger: %+v", stored)
	}
	items, err := runs.ListByWorkspaceID(ctx, run.WorkspaceID)
	if err != nil || len(items) != 1 {
		t.Fatalf("resume created a replacement Run: items=%+v err=%v", items, err)
	}
	_ = biz
}

func TestWorkspaceExecutionResumeRejectsFreshActivationRequirementWithoutContinuation(t *testing.T) {
	_, runs, hub := newLifecycleTestBiz(t)
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-1")
	bootstrap := restartTestBootstrap(t, "ws-1")
	stateRevision := uint64(17)
	stateRef := runevents.RuntimeStateRef("ws-1", "run-1", stateRevision)
	run := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1",
		State: common.RunStateActive, RuntimeBootstrap: &bootstrap,
		RuntimeState: &runtimecontract.RuntimeStateRecord{SchemaVersion: 3, StateRevision: stateRevision},
		RuntimeActivationStatus: &runmodel.RuntimeActivationStatus{
			Disposition:     runtimecontract.RuntimeActivationDispositionFreshActivationRequired,
			RuntimeStateRef: stateRef,
			Incompatibility: runtimecontract.RuntimeStateIncompatibility{
				Reason:                 runtimecontract.RuntimeStateIncompatibilityReasonSchemaUnsupported,
				ActualSchemaVersion:    3,
				SupportedSchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
			},
			Feedback: &runtimecontract.RuntimeFeedback{
				Domain: runtimecontract.RuntimeFeedbackDomainDrive, StableCategory: runtimecontract.RuntimeFeedbackCategoryStateSchemaUnsupported,
				WorkspaceID: "ws-1", RunID: "run-1",
			},
		},
	}
	if err := runs.Create(ctx, run); err != nil {
		t.Fatal(err)
	}

	continuations := make(chan runevents.RunContinuationRequestedData, 1)
	observer := magicevent.NewSimpleObserver(runevents.NotificationDestination, hub)
	observer.Subscribe(runevents.TopicRunContinuationRequested, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(runevents.RunContinuationRequestedData); ok {
			continuations <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunContinuationRequested) })

	command := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: "ws-1", RunID: "run-1", RequestID: "resume-schema-incompatible",
		Kind: runtimecontract.RuntimeExecutionIntentResume, GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1",
	}
	result := hub.Send(magicevent.NewEventWithContext(
		runevents.TopicRunCommandWorkspaceExecutionIntent,
		workspaceExecutionIntentRunSource,
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	admission, ok := data.(runevents.WorkspaceExecutionIntentResult)
	if !ok || admission.Outcome != runtimecontract.RuntimeExecutionIntentFreshActivationRequired ||
		admission.Reason != runtimecontract.RuntimeExecutionIntentReasonRuntimeStateIncompatible || admission.RunID != run.ID {
		t.Fatalf("resume admission = %+v", data)
	}
	select {
	case continuation := <-continuations:
		t.Fatalf("schema-incompatible resume published continuation: %+v", continuation)
	case <-time.After(50 * time.Millisecond):
	}
	stored, err := runs.GetByWorkspaceID(ctx, run.WorkspaceID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != common.RunStateActive || stored.RuntimeActivationStatus == nil ||
		stored.RuntimeActivationStatus.Disposition != runtimecontract.RuntimeActivationDispositionFreshActivationRequired ||
		stored.RuntimeState == nil || stored.RuntimeState.SchemaVersion != 3 ||
		stored.RuntimeState.StateRevision != stateRevision {
		t.Fatalf("fresh-activation resume rewrote Run: %+v", stored)
	}
}
