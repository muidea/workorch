package mcp

import (
	"context"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMCPOwnerReconciliationDoesNotTreatCatalogReadinessAsBehaviorProof(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	command := mcpevents.QueryRuntimeCWIReconciliationCommand{
		WorkspaceID:        "ws-mcp-reconciliation",
		RunID:              "run-mcp-reconciliation",
		RequestID:          "request-mcp-reconciliation",
		Generation:         1,
		DefinitionRevision: "definition-mcp-reconciliation",
		BindingSetHash:     "binding-set-mcp-reconciliation",
		OwnerModuleID:      modulecommon.ModuleID,
		Binding: runtimecontract.RuntimeCWIReconciliationBindingRef{
			CoverageWorkItemID:    common.CoverageWorkItemID("cwi-mcp"),
			BindingID:             "binding-mcp",
			BindingHash:           "binding-hash-mcp",
			OwnerModuleIDs:        []string{modulecommon.ModuleID},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		},
		WorkspaceDeliverableDirectory: t.TempDir(),
		WorkspaceBaselineRevision:     "mcp-catalog-reconciliation",
		WorkspaceBaselineRef:          "mcp-catalog/mcp-catalog-reconciliation",
	}
	response := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicQueryRuntimeCWIReconciliation,
		runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID, magicevent.NewHeader(), ctx, command,
	))
	data, _ := response.Get()
	if response.Error() != nil {
		t.Fatalf("response error: %v", response.Error())
	}
	result, ok := data.(mcpevents.QueryRuntimeCWIReconciliationResult)
	if !ok {
		t.Fatalf("response type = %T", data)
	}
	if result.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable {
		t.Fatalf("catalog-only observation status = %q, want unverifiable", result.Observation.Status)
	}
	if result.Observation.OriginalDiagnostic == nil || result.Observation.OriginalDiagnostic.OriginalCode != "behavior_probe_unavailable" {
		t.Fatalf("catalog-only diagnostic = %+v", result.Observation.OriginalDiagnostic)
	}
}

func TestMCPOwnerReconciliationPreservesDeclaredRecipeAsUnverifiableWithoutGrantingCredit(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	serverID, tool := "mcp-reconciliation-ready", "repo.summary"
	capability := testMCPToolCapability(common.MCPServerID(serverID), tool, mcpmodel.ValidationProofApplicabilityDirect)
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner.runtimeAdapter = NewRuntimeAdapter(staticMCPServerGetter{server: &mcpmodel.Server{
		ID: common.MCPServerID(serverID), Name: "reconciliation ready", Transport: "http", Endpoint: "http://127.0.0.1:9",
		Enabled: true, AllowedTools: []string{tool}, TimeoutMillis: 1000,
	}, capabilities: []*mcpmodel.Capability{capability}})
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	recipeID := runtimecontract.RuntimeMCPToolValidationRecipeID(serverID, tool)
	command := mcpevents.QueryRuntimeCWIReconciliationCommand{
		WorkspaceID: "ws-mcp-ready", RunID: "run-mcp-ready", RequestID: "request-mcp-ready", Generation: 1,
		DefinitionRevision: "definition-mcp-ready", BindingSetHash: "binding-set-mcp-ready", OwnerModuleID: modulecommon.ModuleID,
		Binding: runtimecontract.RuntimeCWIReconciliationBindingRef{
			CoverageWorkItemID: "cwi-mcp-ready", BindingID: "binding-mcp-ready", BindingHash: "binding-hash-mcp-ready",
			OwnerModuleIDs: []string{modulecommon.ModuleID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			AllowedOperationKinds: []string{runtimecontract.StepKindMCPToolCall}, ValidationRecipeIDs: []string{recipeID}, RequiredCheckRefs: []string{recipeID},
		},
		WorkspaceDeliverableDirectory: t.TempDir(), WorkspaceBaselineRevision: "baseline-mcp-ready", WorkspaceBaselineRef: "workspace-tree/baseline-mcp-ready",
	}
	response := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicQueryRuntimeCWIReconciliation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	data, resultErr := response.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	result, ok := data.(mcpevents.QueryRuntimeCWIReconciliationResult)
	wantTarget := []string{serverID + "/" + tool}
	if !ok || result.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable ||
		!reflect.DeepEqual(result.Observation.TargetRefs, wantTarget) || !reflect.DeepEqual(result.Observation.CheckRefs, []string{recipeID}) ||
		result.Observation.OriginalDiagnostic == nil || result.Observation.OriginalDiagnostic.OriginalCode != "behavior_probe_unavailable" {
		t.Fatalf("ready MCP recipe observation = %#v", data)
	}
}
