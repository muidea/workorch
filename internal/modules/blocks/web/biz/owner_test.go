package web

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWebAdmissibilityObservationReturnsTypedConfigurationStale(t *testing.T) {
	inventory := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "admissibility-web", OwnerModuleID: ModuleID, OperationKinds: []string{StepKindSearch}, SelectorRefs: []string{"provider-a"}, ObservedRevision: "providers-v1"}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	observation := webAdmissibilityObservation(inventory.PrerequisiteID, inventory)
	if err := observation.Validate(); err != nil {
		t.Fatal(err)
	}
	stale := webAdmissibilityStale(&observation, inventory, "providers-v2")
	if stale == nil || stale.Domain != runtimecontract.RuntimeResourceProviderConfig || stale.ExpectedRevision != "providers-v1" || stale.ObservedRevision != "providers-v2" {
		t.Fatalf("web configuration stale = %+v", stale)
	}
}

func TestWebOwnerProbeDistinguishesConfigurationDriftFromUnavailableConfiguration(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })

	providers := map[string]ProviderConfig{"provider-a": {ID: "provider-a"}}
	revision := uint64(1)
	adapter := NewRuntimeAdapterFromConfigLoaderWithRevision(func() (string, map[string]ProviderConfig, error) {
		return "provider-a", providers, nil
	}, func() uint64 { return revision }, nil)
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: adapter}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	_, observedRevision, err := adapter.OperationAdmissibility()
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := orchestratorevents.NewRuntimeOperationAdmissibility("admissibility-web", ModuleID, []string{StepKindSearch}, nil, []string{"provider-a"}, nil, observedRevision)
	if err != nil {
		t.Fatal(err)
	}
	observation := webAdmissibilityObservation(inventory.PrerequisiteID, inventory)

	// A confirmed configuration revision change is the only freshness result.
	providers = map[string]ProviderConfig{"provider-b": {ID: "provider-b"}}
	revision++
	stale := sendWebFeasibilityProbe(t, ctx, hub, inventory, observation)
	if stale.Feasible || stale.Stale == nil || stale.Failure != nil || stale.Stale.ExpectedRevision != observedRevision {
		t.Fatalf("configuration drift response = %+v, want typed stale only", stale)
	}

	// A configuration read failure is an ordinary owner failure, never stale.
	owner.runtimeAdapter = NewRuntimeAdapterFromConfigLoaderWithRevision(func() (string, map[string]ProviderConfig, error) {
		return "", nil, errors.New("configuration backend unavailable")
	}, func() uint64 { return 1 }, nil)
	unavailable := sendWebFeasibilityProbe(t, ctx, hub, inventory, observation)
	if unavailable.Feasible || unavailable.Stale != nil || unavailable.Failure == nil || unavailable.Failure.Code != "web_provider_configuration_unavailable" {
		t.Fatalf("configuration unavailable response = %+v, want ordinary failure without stale", unavailable)
	}
}

func sendWebFeasibilityProbe(t *testing.T, ctx context.Context, hub magicevent.Hub, inventory runtimecontract.RuntimeOperationAdmissibility, observation runtimecontract.RuntimePrerequisiteObservation) orchestratorevents.ProbeRuntimeOperationFeasibilityResult {
	t.Helper()
	query := "runtime freshness"
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility,
		runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.ProbeRuntimeOperationFeasibilityCommand{
			WorkspaceID: "ws-web-freshness", RunID: "run-web-freshness",
			Transaction:                   runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-web-freshness"},
			WorkspaceDeliverableDirectory: t.TempDir(), Admissibility: inventory, Observation: &observation,
			Operation: runtimecontract.RuntimeOperation{ID: "web-search", Kind: StepKindSearch, Parameters: []runtimecontract.OperationParameter{{Name: "query", String: &query}}},
		},
	))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(orchestratorevents.ProbeRuntimeOperationFeasibilityResult)
	if !ok {
		t.Fatalf("probe response type = %T", data)
	}
	return response
}

func TestApprovalGetterPublishesScopedOwnerCommand(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(8)
	approvalID := common.ApprovalID("approval-web-scope")
	runID := common.RunID("run-web-scope")
	var captured approvalevents.GetApprovalCommand
	var source string
	observer := magicevent.NewSimpleObserver(approvalevents.CommandDestination, rawHub)
	observer.Subscribe(approvalevents.TopicApprovalCommandGet, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(approvalevents.GetApprovalCommand)
		if !ok {
			t.Fatalf("command type = %T, want GetApprovalCommand", ev.Data())
		}
		captured, source = cmd, ev.Source()
		result.Set(approvalevents.ApprovalCommandResult{Approval: &approvalmodel.Approval{ID: approvalID, RunID: runID, State: common.ApprovalStateApproved}}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(approvalevents.TopicApprovalCommandGet); rawHub.Terminate(ctx) })
	item, err := (webApprovalGetter{hub: rawHub}).GetForRun(ctx, approvalID, runID)
	if err != nil {
		t.Fatalf("GetForRun() error = %v", err)
	}
	if item == nil || item.ID != approvalID {
		t.Fatalf("approval = %+v, want %s", item, approvalID)
	}
	if source != ModuleID {
		t.Fatalf("source = %q, want %q", source, ModuleID)
	}
	if captured.ApprovalID != approvalID || captured.RunID != runID {
		t.Fatalf("command = %+v, want approval/run scope", captured)
	}
}
