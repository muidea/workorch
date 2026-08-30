package run

import (
	"context"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRunStoreAndQueryPreserveOpaqueRuntimeStateRawShape(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	hub := newRunModuleTestHub(t)
	runs := memory.NewRunRepository()
	bootstrap := runtimecontract.RuntimeBootstrap{}
	initial := runtimecontract.RuntimeStateRecord{StateRevision: 1}
	item := &runmodel.Run{
		ID:               "run-raw-runtime-state",
		WorkspaceID:      "ws-raw-runtime-state",
		State:            common.RunStateActive,
		CreatedAt:        "2026-08-28T00:00:00Z",
		RuntimeBootstrap: &bootstrap,
		RuntimeState:     &initial,
	}
	if err := runs.Create(ctx, item); err != nil {
		t.Fatalf("create Run: %v", err)
	}
	module := New()
	if err := module.Configure(runTestDependencies(runs, nil, nil, hub, nil)); err != nil {
		t.Fatalf("configure Run: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("start Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Errorf("stop Run: %v", err)
		}
	})

	raw := runOpaqueRuntimeStateRawFixture()
	expected := runOpaqueRuntimeStateRawFixture()
	expectedRevision := initial.StateRevision
	store := hub.Send(magicevent.NewEventWithContext(
		runevents.TopicRunCommandStoreRuntimeStateRecord,
		"kernel/orchestrator",
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		runevents.StoreRuntimeStateRecordCommand{
			WorkspaceID:                  item.WorkspaceID,
			RunID:                        item.ID,
			RuntimeState:                 &raw,
			ExpectedRuntimeStateRevision: &expectedRevision,
		},
	))
	if store == nil {
		t.Fatal("Run store returned nil result")
	}
	storeData, err := store.Get()
	if err != nil {
		t.Fatalf("Run store: %v", err)
	}
	storeResult, ok := storeData.(runevents.StoreRuntimeStateRecordResult)
	if !ok || storeResult.RuntimeStateUpdate == nil || storeResult.RuntimeStateUpdate.Feedback.StableCategory != runtimecontract.RuntimeFeedbackCategoryAccepted {
		t.Fatalf("Run store result = %#v", storeData)
	}

	// Store must detach the caller-owned graph before returning.
	raw.ProcessedEventIDs[0] = "caller mutation"
	raw.OperationReceipts[0].TargetRefs[0] = "caller target mutation"
	first := queryRunOpaqueRuntimeState(t, ctx, hub, item.WorkspaceID, item.ID, expected.StateRevision)
	if !reflect.DeepEqual(*first, expected) {
		t.Fatalf("Run store/query changed opaque Runtime state\nwant: %#v\ngot:  %#v", expected, *first)
	}
	if first.Definition.CapabilityContract == nil || first.Diagnostics == nil ||
		first.OperationReceipts[1].TargetRefs == nil || first.PrerequisiteInvalidations[1].ObservationIDs == nil {
		t.Fatal("Run store/query collapsed a non-nil empty collection")
	}

	// Query must also detach its returned graph from repository-owned storage.
	first.ProcessedEventIDs[0] = "query mutation"
	first.OperationReceipts[0].TargetRefs[0] = "query target mutation"
	second := queryRunOpaqueRuntimeState(t, ctx, hub, item.WorkspaceID, item.ID, expected.StateRevision)
	if !reflect.DeepEqual(*second, expected) {
		t.Fatalf("Run query leaked repository aliases\nwant: %#v\ngot:  %#v", expected, *second)
	}
}

func runOpaqueRuntimeStateRawFixture() runtimecontract.RuntimeStateRecord {
	return runtimecontract.RuntimeStateRecord{
		StateRevision: 2,
		Definition: runtimecontract.RuntimeDefinitionContext{
			GoalDefinition:     "  raw goal definition  \n",
			DoDDefinition:      "\n  raw dod definition  ",
			CapabilityContract: []string{},
		},
		Facts: runtimecontract.RuntimeFacts{
			RemainingFocuses: []runtimecontract.CoverageFocus{
				{ID: " focus-b ", RequiredEvidenceKinds: []string{" kind-z ", " kind-z ", " kind-a "}},
				{ID: " focus-a ", RequiredEvidenceKinds: []string{}},
			},
		},
		ProcessedEventIDs: []string{" event-z ", " event-z ", " event-a "},
		Diagnostics:       []runtimecontract.RuntimeDiagnosticRecord{},
		OperationReceipts: []runtimecontract.RuntimeOperationReceipt{
			{ExecutionKey: " receipt-z ", OperationID: " operation-b ", TargetRefs: []string{" target-b ", " target-b ", " target-a "}},
			{ExecutionKey: " receipt-z ", OperationID: " operation-a ", TargetRefs: []string{}},
		},
		PrerequisiteInvalidations: []runtimecontract.RuntimePrerequisiteInvalidationRecord{
			{Signature: " invalidation-z ", PrerequisiteIDs: []string{" prerequisite-b ", " prerequisite-b ", " prerequisite-a "}},
			{Signature: " invalidation-z ", ObservationIDs: []string{}},
		},
	}
}

func queryRunOpaqueRuntimeState(t *testing.T, ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID, revision uint64) *runtimecontract.RuntimeStateRecord {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		runevents.TopicRunCommandRuntimeStateRecord,
		"kernel/orchestrator",
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		runevents.QueryRuntimeStateRecordCommand{
			WorkspaceID:                  workspaceID,
			RunID:                        runID,
			ExpectedRuntimeStateRef:      runevents.RuntimeStateRef(workspaceID, runID, revision),
			ExpectedRuntimeStateRevision: revision,
		},
	))
	if result == nil {
		t.Fatal("Run query returned nil result")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("Run query: %v", err)
	}
	response, ok := data.(runevents.QueryRuntimeStateRecordResult)
	if !ok || !response.StatePresent || response.RuntimeState == nil {
		t.Fatalf("Run query result = %#v", data)
	}
	return response.RuntimeState
}
