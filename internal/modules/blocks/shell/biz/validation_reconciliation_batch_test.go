package shell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func projectReconciliationBatchFixture(t *testing.T) shellevents.QueryRuntimeCWIReconciliationBatchCommand {
	t.Helper()
	r, _ := projectExecutionFixture(t, false)
	recipe := isolatedInputRecipe()
	recipe.AdapterRef = shellProjectAdapterForObservation(shellProjectConjunctionProtocolRef)
	recipe.InputReviewProtocolRef = runtimecontract.RuntimeValidationInputReviewProtocol
	recipe.TimeoutMillis = 60000
	c := shellevents.QueryRuntimeCWIReconciliationBatchCommand{Request: runtimecontract.RuntimeCWIReconciliationRequest{
		RequestID: "fresh-group", WorkspaceID: r.WorkspaceID, RunID: r.RunID, Generation: 1,
		Scope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, DefinitionRevision: "definition-group",
		WorkspaceBaselineRevision: "tree-before", WorkspaceBaselineRef: "workspace-tree/tree-before"}}
	for i := 0; i < 2; i++ {
		requirement := shellValidationProofRequirementForTest(t, recipe.ID, common.CoverageWorkItemID(fmt.Sprintf("group-cwi-%d", i)))
		checks, ok := shellValidationCheckAttestations(recipe, requirement)
		if !ok {
			t.Fatal("missing native declaration")
		}
		target := shellValidationNativeCheckTargetRef(requirement, checks...)
		writeNativeStructureSource(t, r.WorkspaceDeliverableDirectory, target, fmt.Sprintf("package checks\nimport (\"testing\"; delivery \"fixture/delivery\")\nfunc %s(t *testing.T){if delivery.Value()!=4{t.Fatal(\"wrong\")}}\n", checks[0].NativeCheckRef))
		binding := runtimecontract.RuntimeCWIReconciliationBindingRef{CoverageWorkItemID: common.CoverageWorkItemID(fmt.Sprintf("cwi-%d", i)), BindingID: fmt.Sprintf("binding-%d", i), BindingHash: fmt.Sprintf("source-binding-%d", i),
			OwnerModuleIDs: []string{ModuleID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRefs: []string{target}, ValidationRecipeIDs: []string{recipe.ID}, RequiredCheckRefs: requirement.RequiredCheckRefs, ValidationCheckAttestations: checks}
		c.Request.Bindings = append(c.Request.Bindings, binding)
		c.Members = append(c.Members, runtimecontract.RuntimeCWIOwnerReconciliationRequest{WorkspaceID: r.WorkspaceID, RunID: r.RunID, RequestID: c.Request.RequestID, Generation: 1, DefinitionRevision: c.Request.DefinitionRevision,
			OwnerModuleID: ModuleID, Binding: binding, WorkspaceDeliverableDirectory: r.WorkspaceDeliverableDirectory, WorkspaceInternalDirectory: r.WorkspaceInternalDirectory,
			WorkspaceBaselineRevision: c.Request.WorkspaceBaselineRevision, WorkspaceBaselineRef: c.Request.WorkspaceBaselineRef})
	}
	contract := runtimecontract.RuntimeCWIReconciliationContract{SchemaVersion: runtimecontract.RuntimeCWIReconciliationContractSchemaCurrent, DefinitionRevision: c.Request.DefinitionRevision, Bindings: c.Request.Bindings}
	c.Request.BindingSetHash = runtimecontract.StableRuntimeCWIReconciliationContractHash(contract)
	for i := range c.Members {
		c.Members[i].BindingSetHash = c.Request.BindingSetHash
	}
	if err := c.ValidateForOwner(ModuleID); err != nil {
		t.Fatal(err)
	}
	return c
}

func queryProjectBatch(t *testing.T, c shellevents.QueryRuntimeCWIReconciliationBatchCommand) shellevents.QueryRuntimeCWIReconciliationBatchResult {
	t.Helper()
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	r := hub.Send(magicevent.NewEventWithContext(shellevents.TopicQueryRuntimeCWIReconciliationBatch, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, c))
	if r == nil {
		t.Fatal("nil owner result")
	}
	data, err := r.Get()
	if err != nil {
		t.Fatal(err)
	}
	value, ok := data.(shellevents.QueryRuntimeCWIReconciliationBatchResult)
	if !ok {
		t.Fatalf("wrong owner result %T", data)
	}
	if err := c.ValidateResult(value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestReconciliationBatchWithoutFormalResultsDoesNotExecute(t *testing.T) {
	c := projectReconciliationBatchFixture(t)
	marker := filepath.Join(t.TempDir(), "unexpected-process")
	// Any attempt to run the old Go validation path leaves a marker.
	executable := filepath.Join(filepath.Dir(marker), "go")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf executed > '"+marker+"'\nexit 99\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(marker)+string(os.PathListSeparator)+os.Getenv("PATH"))
	for i := 0; i < 2; i++ {
		value := queryProjectBatch(t, c)
		for _, o := range value.Observations {
			if o.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable || o.ValidationExecution != nil || o.OriginalDiagnostic.OriginalCode != "validation_result_unavailable" {
				t.Fatalf("query manufactured proof: %+v", o)
			}
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("query executed recipe: %v", err)
	}
}
