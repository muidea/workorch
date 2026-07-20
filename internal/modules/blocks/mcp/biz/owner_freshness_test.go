package mcp

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMCPAdmissibilityObservationReturnsTypedCatalogStale(t *testing.T) {
	inventory := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "admissibility-mcp", OwnerModuleID: ModuleID, OperationKinds: []string{StepKindToolCall}, SelectorRefs: []string{"server-a"}, ObservedRevision: "catalog-v1"}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	observation := mcpAdmissibilityObservation(inventory.PrerequisiteID, inventory)
	if err := observation.Validate(); err != nil {
		t.Fatal(err)
	}
	stale := mcpAdmissibilityStale(&observation, inventory, "catalog-v2")
	if stale == nil || stale.Domain != runtimecontract.RuntimeResourceOwnerCatalog || stale.ExpectedRevision != "catalog-v1" || stale.ObservedRevision != "catalog-v2" {
		t.Fatalf("mcp catalog stale = %+v", stale)
	}
}

func TestMCPOwnerProbeDistinguishesCatalogDriftFromCatalogUnavailable(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	getter := &mcpFreshnessServerGetter{servers: []*mcpmodel.Server{{ID: "server-a", Enabled: true, Transport: "http", Endpoint: "http://mcp.invalid", AllowedTools: []string{"inspect"}}}}
	adapter := NewRuntimeAdapter(getter)
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: adapter}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	_, revision, err := adapter.OperationAdmissibility(ctx)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := orchestratorevents.NewRuntimeOperationAdmissibility("admissibility-mcp", ModuleID, []string{StepKindToolCall}, nil, []string{"server-a"}, nil, revision)
	if err != nil {
		t.Fatal(err)
	}
	observation := mcpAdmissibilityObservation(inventory.PrerequisiteID, inventory)

	getter.servers = []*mcpmodel.Server{{ID: "server-b", Enabled: true, Transport: "http", Endpoint: "http://mcp.invalid", AllowedTools: []string{"inspect"}}}
	stale := sendMCPFeasibilityProbe(t, ctx, hub, inventory, observation)
	if stale.Feasible || stale.Stale == nil || stale.Failure != nil || stale.Stale.ExpectedRevision != revision {
		t.Fatalf("catalog drift response = %+v, want typed stale only", stale)
	}

	getter.err = errors.New("mcp registry unavailable")
	unavailable := sendMCPFeasibilityProbe(t, ctx, hub, inventory, observation)
	if unavailable.Feasible || unavailable.Stale != nil || unavailable.Failure == nil || unavailable.Failure.Code != "mcp_server_catalog_unavailable" {
		t.Fatalf("catalog unavailable response = %+v, want ordinary failure without stale", unavailable)
	}
}

type mcpFreshnessServerGetter struct {
	servers []*mcpmodel.Server
	err     error
}

func (g *mcpFreshnessServerGetter) GetServer(_ context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if g == nil || g.err != nil {
		return nil, g.err
	}
	for _, server := range g.servers {
		if server != nil && server.ID == id {
			return server, nil
		}
	}
	return nil, common.ErrNotFound
}

func (g *mcpFreshnessServerGetter) ListServers(context.Context) ([]*mcpmodel.Server, error) {
	if g == nil || g.err != nil {
		return nil, g.err
	}
	return append([]*mcpmodel.Server(nil), g.servers...), nil
}

func sendMCPFeasibilityProbe(t *testing.T, ctx context.Context, hub magicevent.Hub, inventory runtimecontract.RuntimeOperationAdmissibility, observation runtimecontract.RuntimePrerequisiteObservation) orchestratorevents.ProbeRuntimeOperationFeasibilityResult {
	t.Helper()
	serverID := "server-a"
	tool := "inspect"
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility,
		runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.ProbeRuntimeOperationFeasibilityCommand{
			WorkspaceID: "ws-mcp-freshness", RunID: "run-mcp-freshness",
			Transaction:                   runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-mcp-freshness"},
			WorkspaceDeliverableDirectory: t.TempDir(), Admissibility: inventory, Observation: &observation,
			Operation: runtimecontract.RuntimeOperation{ID: "mcp-tool", Kind: StepKindToolCall, Parameters: []runtimecontract.OperationParameter{{Name: "serverId", String: &serverID}, {Name: "tool", String: &tool}}},
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
