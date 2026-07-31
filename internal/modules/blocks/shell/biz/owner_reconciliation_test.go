package shell

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestShellOwnerReconciliationDoesNotTreatGenericRecipeSuccessAsCWIProof(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(modulecommon.ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	recipe := runtimecontract.RuntimeValidationRecipe{ID: "generic-success", Command: "true"}
	command := shellReconciliationQueryCommand(t, t.TempDir(), recipe)
	response := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorQueryRuntimeCWIReconciliation,
		runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID, magicevent.NewHeader(), ctx, command,
	))
	data, err := response.Get()
	if err != nil {
		t.Fatal(err)
	}
	result, ok := data.(orchestratorevents.QueryRuntimeCWIReconciliationResult)
	if !ok {
		t.Fatalf("response type = %T", data)
	}
	if result.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable {
		t.Fatalf("generic recipe observation status = %q, want unverifiable", result.Observation.Status)
	}
	if result.Observation.OriginalDiagnostic == nil || result.Observation.OriginalDiagnostic.OriginalCode != "validation_check_declaration_unavailable" {
		t.Fatalf("generic recipe diagnostic = %+v", result.Observation.OriginalDiagnostic)
	}
	if result.OwnerReceiptRef == "" {
		t.Fatal("generic recipe response omitted owner receipt ref")
	}
}

func TestShellOwnerReconciliationAcceptsExplicitStructuredCheck(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(modulecommon.ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "fixture.go"), []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const checkRef = "file/present"
	recipe := runtimecontract.RuntimeValidationRecipe{
		ID:      "structured-presence",
		Command: "sh",
		Args:    []string{"-c", "test -f fixture.go && printf 'WORKORCH_CHECK_PASSED=file/present\\n'"},
	}
	command := shellReconciliationQueryCommand(t, workspace, recipe)
	command.Binding.RequiredCheckRefs = []string{checkRef}
	response := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorQueryRuntimeCWIReconciliation,
		runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID, magicevent.NewHeader(), ctx, command,
	))
	data, err := response.Get()
	if err != nil {
		t.Fatal(err)
	}
	result, ok := data.(orchestratorevents.QueryRuntimeCWIReconciliationResult)
	if !ok {
		t.Fatalf("response type = %T", data)
	}
	if result.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
		t.Fatalf("structured check observation status = %q, want satisfied; diagnostic=%+v observation=%+v", result.Observation.Status, result.Observation.OriginalDiagnostic, result.Observation)
	}
	if len(result.Observation.CheckRefs) != 1 || result.Observation.CheckRefs[0] != checkRef {
		t.Fatalf("structured check refs = %#v, want [%q]", result.Observation.CheckRefs, checkRef)
	}
}

func shellReconciliationQueryCommand(t *testing.T, workspace string, recipe runtimecontract.RuntimeValidationRecipe) orchestratorevents.QueryRuntimeCWIReconciliationCommand {
	t.Helper()
	binding := runtimecontract.RuntimeCWIReconciliationBindingRef{
		CoverageWorkItemID:    common.CoverageWorkItemID("cwi-shell"),
		BindingID:             "binding-shell",
		BindingHash:           "binding-hash-shell",
		OwnerModuleIDs:        []string{modulecommon.ModuleID},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		TargetRefs:            []string{"fixture.go"},
		ValidationRecipeIDs:   []string{recipe.ID},
	}
	if err := binding.Validate(); err != nil {
		t.Fatalf("shell binding: %v", err)
	}
	return orchestratorevents.QueryRuntimeCWIReconciliationCommand{
		WorkspaceID:                   "ws-shell-reconciliation",
		RunID:                         "run-shell-reconciliation",
		RequestID:                     "request-shell-reconciliation",
		Generation:                    1,
		DefinitionRevision:            "definition-shell-reconciliation",
		BindingSetHash:                "binding-set-shell-reconciliation",
		OwnerModuleID:                 modulecommon.ModuleID,
		Binding:                       binding,
		WorkspaceDeliverableDirectory: workspace,
		WorkspaceBaselineRevision:     "tree-shell-reconciliation",
		WorkspaceBaselineRef:          "workspace-tree/tree-shell-reconciliation",
		ValidationRecipes:             []runtimecontract.RuntimeValidationRecipe{recipe},
	}
}
