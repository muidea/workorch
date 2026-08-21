package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
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
	providers := map[string]webmodels.ProviderConfig{"provider-a": {ID: "provider-a", SearchEndpoint: "https://provider.invalid/search"}}
	revision := uint64(1)
	adapter := NewRuntimeAdapterFromConfigLoaderWithRevision(func() (string, map[string]webmodels.ProviderConfig, error) {
		return "provider-a", providers, nil
	}, func() uint64 { return revision })
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
	providers = map[string]webmodels.ProviderConfig{"provider-b": {ID: "provider-b", SearchEndpoint: "https://provider.invalid/search"}}
	revision++
	stale := sendWebFeasibilityProbe(t, ctx, hub, inventory, observation)
	if stale.Outcome != runtimecontract.RuntimeOwnerFeasibilityStale || stale.Stale == nil || stale.Feedback != nil || stale.Stale.ExpectedRevision != observedRevision {
		t.Fatalf("configuration drift response = %+v", stale)
	}

	owner.runtimeAdapter = NewRuntimeAdapterFromConfigLoaderWithRevision(func() (string, map[string]webmodels.ProviderConfig, error) {
		return "", nil, errors.New("configuration backend unavailable")
	}, func() uint64 { return 1 })
	unavailable := sendWebFeasibilityProbe(t, ctx, hub, inventory, observation)
	if unavailable.Outcome != runtimecontract.RuntimeOwnerFeasibilityInfeasible || unavailable.Stale != nil || unavailable.Feedback == nil || unavailable.Feedback.OriginalDiagnostic == nil || unavailable.Feedback.OriginalDiagnostic.OriginalCode != "web_provider_configuration_unavailable" {
		t.Fatalf("configuration unavailable response = %+v", unavailable)
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

func TestRuntimeApprovalGrantBindsExactWebAuthorizationTarget(t *testing.T) {
	provider := webmodels.ProviderConfig{ID: "provider-a"}
	requirement := webSearchApprovalRequirement(provider, "release notes", 12, policyDecision{Decision: policyApproval, Reasons: []string{"large_result_set"}})
	grant := runtimeWebGrant(requirement)
	if err := validateRuntimeApprovalGrant(grant, requirement); err != nil {
		t.Fatalf("exact grant: %v", err)
	}
	changedProvider := webSearchApprovalRequirement(webmodels.ProviderConfig{ID: "provider-b"}, "release notes", 12, policyDecision{Decision: policyApproval, Reasons: []string{"large_result_set"}})
	if err := validateRuntimeApprovalGrant(grant, changedProvider); err == nil {
		t.Fatal("grant authorized changed provider")
	}
	changedSubject := webSearchApprovalRequirement(provider, "security advisories", 12, policyDecision{Decision: policyApproval, Reasons: []string{"large_result_set"}})
	if err := validateRuntimeApprovalGrant(grant, changedSubject); err == nil {
		t.Fatal("grant authorized changed subject")
	}
}

func TestWebOwnerFailsClosedBeforeNetworkAndCommitsRecoverableEvidence(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	var providerCalls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		_ = json.NewEncoder(w).Encode(searchGatewayResponse{Results: []searchResult{{Title: "Owner result", URL: "https://example.com/result", Snippet: "recoverable content"}}})
	}))
	defer provider.Close()
	adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]webmodels.ProviderConfig{"http": testWebProvider(provider.URL)}, provider.Client())
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: adapter}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	rejectQuery := atomic.Bool{}
	installWebReceiptObserver(t, hub, &rejectQuery)
	operation, sealed := testWebSearchOperation(t)
	command := testWebExecuteCommand(t, operation, sealed)
	event := magicevent.NewEventWithContext(webevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command)

	owner.handleWebOperation(event, nil)
	if providerCalls.Load() != 0 {
		t.Fatal("Post-style Web call reached provider")
	}
	wrongSource := hub.Send(magicevent.NewEventWithContext(webevents.TopicExecuteOperation, runtimecontract.RuntimeModuleFile, ModuleID, magicevent.NewHeader(), ctx, command))
	if _, err := wrongSource.Get(); err == nil || providerCalls.Load() != 0 {
		t.Fatalf("wrong source err=%v calls=%d", err, providerCalls.Load())
	}
	drifted := command
	drifted.SealedOperation.PayloadHash = "drifted"
	driftedResult := hub.Send(magicevent.NewEventWithContext(webevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, drifted))
	if _, err := driftedResult.Get(); err == nil || providerCalls.Load() != 0 {
		t.Fatalf("sealed drift err=%v calls=%d", err, providerCalls.Load())
	}
	rejectQuery.Store(true)
	stale := hub.Send(event)
	if _, err := stale.Get(); err == nil || providerCalls.Load() != 0 {
		t.Fatalf("stale receipt query err=%v calls=%d", err, providerCalls.Load())
	}
	rejectQuery.Store(false)
	accepted := hub.Send(event)
	data, err := accepted.Get()
	if err != nil {
		t.Fatal(err)
	}
	receipt := data.(webevents.ExecuteOperationResult).Receipt
	if providerCalls.Load() != 1 || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || receipt.ObservedEffectScope != runtimecontract.RuntimeEffectNone || receipt.Changed ||
		!strings.Contains(receipt.Evidence.Summary, "recoverable content") || len(receipt.Evidence.EvidenceRefs) < 2 {
		t.Fatalf("receipt=%+v calls=%d", receipt, providerCalls.Load())
	}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
}

func installWebReceiptObserver(t *testing.T, hub magicevent.Hub, rejectQuery *atomic.Bool) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
		if rejectQuery != nil && rejectQuery.Load() {
			result.Set(nil, &cd.Error{Code: cd.VersionConflict, Message: "stale runtime Web dispatch"})
			return
		}
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{}, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})
}

func testWebSearchOperation(t *testing.T) (runtimecontract.RuntimeOperation, webevents.SealedRuntimeOperation) {
	t.Helper()
	query := "owner execution"
	operation := runtimecontract.RuntimeOperation{
		ID: "web-search-1", Kind: runtimecontract.StepKindWebSearch, TargetRefs: []string{"web-target"},
		Parameters: []runtimecontract.OperationParameter{{Name: "query", String: &query}},
	}
	sealed, err := webevents.SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	return operation, sealed
}

func testWebExecuteCommand(t *testing.T, operation runtimecontract.RuntimeOperation, sealed webevents.SealedRuntimeOperation) webevents.ExecuteOperationCommand {
	t.Helper()
	return webevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution:   runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/web-search-1", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectNone},
			WorkspaceID: "ws-web", RunID: "run-web", DispatchID: "dispatch-web", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-web"},
			OperationID: operation.ID, TargetRefs: append([]string(nil), operation.TargetRefs...), EvidenceKind: runtimecontract.EvidenceKindValidation,
			WorkspaceDeliverableDirectory: t.TempDir(), WorkspaceInternalDirectory: t.TempDir(),
		},
		SealedOperation: sealed,
	}
}
