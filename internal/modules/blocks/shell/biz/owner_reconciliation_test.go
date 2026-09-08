package shell

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
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
		shellevents.TopicQueryRuntimeCWIReconciliation,
		runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID, magicevent.NewHeader(), ctx, command,
	))
	data, err := response.Get()
	if err != nil {
		t.Fatal(err)
	}
	result, ok := data.(shellevents.QueryRuntimeCWIReconciliationResult)
	if !ok {
		t.Fatalf("response type = %T", data)
	}
	if result.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable {
		t.Fatalf("generic recipe observation status = %q, want unverifiable", result.Observation.Status)
	}
	if result.Observation.OriginalDiagnostic == nil || result.Observation.OriginalDiagnostic.OriginalCode != "validation_recipe_resolution_failed" {
		t.Fatalf("generic recipe diagnostic = %+v", result.Observation.OriginalDiagnostic)
	}
	if ref := runtimecontract.StableRuntimeCWIReconciliationObservationRef(result.Observation); ref == "" {
		t.Fatal("generic recipe response observation did not produce stable lineage ref")
	}
}

func TestShellOwnerReconciliationAcceptsOwnerNativeStructuredCheck(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(modulecommon.ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	workspace := newGoValidationWorkspace(t, `package fixture

import "testing"

func TestWorkOrchCWI_SHELLRECONCILIATION(t *testing.T) {
	if false {
		t.Fatal("unreachable")
	}
}
`)
	const checkRef = "workorch-cwi-check-shell-reconciliation"
	recipe := runtimecontract.RuntimeValidationRecipe{
		ID: "structured-presence", Command: "go", Args: []string{"test", "./..."},
	}
	command := shellReconciliationQueryCommand(t, workspace, recipe)
	response := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicQueryRuntimeCWIReconciliation,
		runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID, magicevent.NewHeader(), ctx, command,
	))
	data, err := response.Get()
	if err != nil {
		t.Fatal(err)
	}
	result, ok := data.(shellevents.QueryRuntimeCWIReconciliationResult)
	if !ok {
		t.Fatalf("response type = %T", data)
	}
	if result.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
		t.Fatalf("structured check observation status = %q, want satisfied; diagnostic=%+v observation=%+v", result.Observation.Status, result.Observation.OriginalDiagnostic, result.Observation)
	}
	if len(result.Observation.CheckRefs) != 1 || !shellStringIncluded(result.Observation.CheckRefs, checkRef) {
		t.Fatalf("structured check refs = %#v, want only %q", result.Observation.CheckRefs, checkRef)
	}
}

func shellReconciliationQueryCommand(t *testing.T, workspace string, recipe runtimecontract.RuntimeValidationRecipe) shellevents.QueryRuntimeCWIReconciliationCommand {
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
		RequiredCheckRefs:     []string{"workorch-cwi-check-shell-reconciliation"},
		ValidationCheckAttestations: []runtimecontract.RuntimeValidationCheckAttestationBinding{{
			RequiredCheckRef: "workorch-cwi-check-shell-reconciliation", ObservationProtocolRef: shellGoTestObservationProtocolRef,
			NativeCheckRef: "TestWorkOrchCWI_SHELLRECONCILIATION",
		}},
	}
	if err := binding.Validate(); err != nil {
		t.Fatalf("shell binding: %v", err)
	}
	return shellevents.QueryRuntimeCWIReconciliationCommand{
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

func checkEntrypointReconciliation(t *testing.T, request RuntimeCommandRequest) {
	t.Helper()
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(modulecommon.ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	recipe := runtimecontract.RuntimeValidationRecipe{ID: request.ValidationRecipeID, Command: "go", Args: []string{"test", "./..."}, TimeoutMillis: 60000}
	command := shellReconciliationQueryCommand(t, request.WorkspaceDeliverableDirectory, recipe)
	command.Binding.TargetRefs = request.TargetRefs
	command.Binding.RequiredCheckRefs = []string{request.ValidationCheckAttestations[0].RequiredCheckRef}
	command.Binding.ValidationCheckAttestations = request.ValidationCheckAttestations
	for _, complete := range []bool{true, false, true} {
		command.ValidationScopeTargetRefs = nil
		if complete {
			command.ValidationScopeTargetRefs = request.ValidationScopeTargetRefs
		}
		response := hub.Send(magicevent.NewEventWithContext(shellevents.TopicQueryRuntimeCWIReconciliation, runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID, magicevent.NewHeader(), ctx, command))
		data, err := response.Get()
		if err != nil {
			t.Fatal(err)
		}
		observation := data.(shellevents.QueryRuntimeCWIReconciliationResult).Observation
		if complete && observation.Status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
			t.Fatalf("same v4 native/supplement proof failed reconciliation: %+v", observation)
		}
		if !complete && (observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable || observation.OriginalDiagnostic == nil || observation.OriginalDiagnostic.OriginalCode != "validation_behavior_contract_rejected") {
			t.Fatalf("missing sealed scope borrowed targets or lost preflight cause: %+v", observation)
		}
	}
}
