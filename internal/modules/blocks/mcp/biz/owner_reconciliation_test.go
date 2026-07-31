package mcp

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
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

	command := orchestratorevents.QueryRuntimeCWIReconciliationCommand{
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
		orchestratorevents.TopicOrchestratorQueryRuntimeCWIReconciliation,
		runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID, magicevent.NewHeader(), ctx, command,
	))
	data, _ := response.Get()
	if response.Error() != nil {
		t.Fatalf("response error: %v", response.Error())
	}
	result, ok := data.(orchestratorevents.QueryRuntimeCWIReconciliationResult)
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
