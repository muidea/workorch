package web

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mustRuntimeOperationAuthorizationIdentity(t *testing.T, kind string) runtimecontract.RuntimeCandidateOperationManifest {
	t.Helper()
	authorization, err := runtimecontract.RuntimeOperationAuthorizationIdentity(kind)
	if err != nil {
		t.Fatalf("runtime operation authorization identity for %q: %v", kind, err)
	}
	return authorization
}

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

	providers := map[string]webmodels.ProviderConfig{"provider-a": {ID: "provider-a"}}
	revision := uint64(1)
	adapter := NewRuntimeAdapterFromConfigLoaderWithRevision(func() (string, map[string]webmodels.ProviderConfig, error) {
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
	inventory, err := runtimecontract.NewRuntimeOperationAdmissibility("admissibility-web", ModuleID, []string{StepKindSearch}, nil, []string{"provider-a"}, nil, observedRevision)
	if err != nil {
		t.Fatal(err)
	}
	observation := webAdmissibilityObservation(inventory.PrerequisiteID, inventory)

	// A confirmed configuration revision change is the only freshness result.
	providers = map[string]webmodels.ProviderConfig{"provider-b": {ID: "provider-b"}}
	revision++
	stale := sendWebFeasibilityProbe(t, ctx, hub, inventory, observation)
	if stale.Outcome != runtimecontract.RuntimeOwnerFeasibilityStale || stale.Stale == nil || stale.Feedback != nil || stale.Stale.ExpectedRevision != observedRevision {
		t.Fatalf("configuration drift response = %+v, want typed stale only", stale)
	}

	// A configuration read failure is an ordinary owner failure, never stale.
	owner.runtimeAdapter = NewRuntimeAdapterFromConfigLoaderWithRevision(func() (string, map[string]webmodels.ProviderConfig, error) {
		return "", nil, errors.New("configuration backend unavailable")
	}, func() uint64 { return 1 }, nil)
	unavailable := sendWebFeasibilityProbe(t, ctx, hub, inventory, observation)
	if unavailable.Outcome != runtimecontract.RuntimeOwnerFeasibilityInfeasible || unavailable.Stale != nil || unavailable.Feedback == nil || unavailable.Feedback.OriginalDiagnostic == nil || unavailable.Feedback.OriginalDiagnostic.OriginalCode != "web_provider_configuration_unavailable" {
		t.Fatalf("configuration unavailable response = %+v, want ordinary failure without stale", unavailable)
	}
}

func sendWebFeasibilityProbe(t *testing.T, ctx context.Context, hub magicevent.Hub, inventory runtimecontract.RuntimeOperationAdmissibility, observation runtimecontract.RuntimePrerequisiteObservation) webevents.ProbeRuntimeOperationFeasibilityResult {
	t.Helper()
	query := "runtime freshness"
	result := hub.Send(magicevent.NewEventWithContext(
		webevents.TopicProbeRuntimeOperationFeasibility,
		runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx,
		webevents.ProbeRuntimeOperationFeasibilityCommand{
			WorkspaceID: "ws-web-freshness", RunID: "run-web-freshness",
			Transaction:                   runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-web-freshness"},
			WorkspaceDeliverableDirectory: t.TempDir(), Admissibility: inventory, Observation: &observation,
			CandidateManifestHash: "manifest-web-freshness", OperationAuthorization: mustRuntimeOperationAuthorizationIdentity(t, StepKindSearch),
			Operation: runtimecontract.RuntimeOperation{ID: "web-search", Kind: StepKindSearch, Parameters: []runtimecontract.OperationParameter{{Name: "query", String: &query}}},
		},
	))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(webevents.ProbeRuntimeOperationFeasibilityResult)
	if !ok {
		t.Fatalf("probe response type = %T", data)
	}
	return response
}

func TestApprovalGetterPublishesScopedOwnerCommand(t *testing.T) {
	const workspaceID = common.WorkspaceID("ws-web-scope")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
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
		result.Set(approvalevents.GetApprovalResult{Approval: &approvalmodel.Approval{ID: approvalID, WorkspaceID: workspaceID, RunID: runID, State: common.ApprovalStateApproved}}, nil)
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
	if captured.ApprovalID != approvalID || captured.WorkspaceID != workspaceID || captured.RunID != runID {
		t.Fatalf("command = %+v, want approval/run scope", captured)
	}
}

func TestRuntimeApprovalGrantBindsExactWebAuthorizationTarget(t *testing.T) {
	authorization := workorcheventpayload.New(
		workorcheventpayload.StringField("providerId", "provider-a"),
		workorcheventpayload.StringField("query", "release notes"),
	)
	grant := &runtimecontract.RuntimeOperationGovernanceGrant{
		ResolutionID: "resolution-web-grant", ApprovalID: "approval-web-grant",
		ExecutionKey: "execution-web-grant", GovernanceFence: 1, ResumeFence: 2,
		DependencyKind: string(runtimecontract.GovernanceApproval), SubjectRef: "release notes",
		RequestedAction: StepKindSearch, AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(authorization),
		GovernanceReceiptHash: "receipt-web-grant",
	}
	approvalID, err := validateRuntimeApprovalGrant(grant, StepKindSearch, "release notes", authorization)
	if err != nil || approvalID != "approval-web-grant" {
		t.Fatalf("exact grant approvalID=%q err=%v", approvalID, err)
	}
	changedTarget := workorcheventpayload.New(
		workorcheventpayload.StringField("providerId", "provider-b"),
		workorcheventpayload.StringField("query", "release notes"),
	)
	if _, err := validateRuntimeApprovalGrant(grant, StepKindSearch, "release notes", changedTarget); err == nil {
		t.Fatal("grant authorized changed provider target")
	}
	if _, err := validateRuntimeApprovalGrant(grant, StepKindSearch, "security advisories", authorization); err == nil {
		t.Fatal("grant authorized changed subject")
	}
}
