package orchestrator

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeReconciliationRequestSeparatesRestartActivationFromResumeScope(t *testing.T) {
	run := &runmodel.Run{
		ID:                  "run-002",
		WorkspaceID:         "ws-1",
		ActivationRequestID: "restart-1",
		ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
		DefinitionRevision:  "definition-2",
	}
	binding := runtimecontract.RuntimeCWIReconciliationBindingRef{
		CoverageWorkItemID:    "cwi-1",
		BindingID:             "binding-1",
		BindingHash:           "binding-hash-1",
		OwnerModuleIDs:        []string{runtimecontract.RuntimeModuleFile},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
	}
	aggregate := runtime.RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-002", DefinitionRevision: "definition-2",
		ReconciliationContract: runtimecontract.RuntimeCWIReconciliationContract{
			SchemaVersion:  runtimecontract.RuntimeCWIReconciliationContractSchemaCurrent,
			BindingSetHash: "binding-set-1",
			Bindings:       []runtimecontract.RuntimeCWIReconciliationBindingRef{binding},
		},
	}}

	restart := runtimeReconciliationRequestForRun(run, aggregate, "", 0)
	if restart.Scope != runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull || restart.RequestID != "runtime-reconciliation/restart-1/workspace_full/binding-set/binding-set-1/generation/1" {
		t.Fatalf("restart activation request = %+v", restart)
	}
	if restart.Generation != 1 || restart.BindingSetHash != "binding-set-1" || len(restart.Bindings) != 1 {
		t.Fatalf("restart activation lost sealed reconciliation identity: %+v", restart)
	}

	// The persisted Run keeps its workspace_full activation fact. A later
	// continuation is nevertheless scoped to the same current Run and must not
	// inherit the wider replacement admission scope.
	resume := runtimeReconciliationRequestForRun(run, aggregate, "resume-2", 0)
	if resume.Scope != runtimecontract.RuntimeCWIReconciliationScopeCurrentRun || resume.RequestID != "runtime-reconciliation/resume/resume-2/current_run/binding-set/binding-set-1/generation/1" {
		t.Fatalf("resume request = %+v", resume)
	}
	if resume.WorkspaceID != common.WorkspaceID("ws-1") || resume.RunID != common.RunID("run-002") || resume.DefinitionRevision != "definition-2" {
		t.Fatalf("resume changed sealed Run identity: %+v", resume)
	}
	if resume.BindingSetHash != restart.BindingSetHash || len(resume.Bindings) != len(restart.Bindings) {
		t.Fatalf("resume did not reuse sealed semantic contract: restart=%+v resume=%+v", restart, resume)
	}
}

func TestRuntimeReconciliationRequestNeverReactivatesHistoricalGeneration(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-002", WorkspaceID: "ws-1", ActivationRequestID: "restart-1",
		ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
	}
	binding := runtimecontract.RuntimeCWIReconciliationBindingRef{
		CoverageWorkItemID: "cwi-1", BindingID: "binding-1", BindingHash: "binding-hash-1",
		OwnerModuleIDs: []string{runtimecontract.RuntimeModuleFile}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
	}
	aggregate := runtime.RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-002", DefinitionRevision: "definition-2",
		ReconciliationContract: runtimecontract.RuntimeCWIReconciliationContract{
			SchemaVersion:  runtimecontract.RuntimeCWIReconciliationContractSchemaCurrent,
			BindingSetHash: "binding-set-1", Bindings: []runtimecontract.RuntimeCWIReconciliationBindingRef{binding},
		},
	}}
	first := runtimeReconciliationRequestForRun(run, aggregate, "", 0)
	aggregate.Reconciliation.History = []runtimecontract.RuntimeCWIReconciliationGeneration{{
		Superseded: true, Request: first, RequestHash: runtimecontract.StableRuntimeCWIReconciliationRequestHash(first),
	}}
	next := runtimeReconciliationRequestForRun(run, aggregate, "", 0)
	if next.RequestID == first.RequestID || next.Generation != first.Generation+1 {
		t.Fatalf("historical reconciliation request was reused: first=%+v next=%+v", first, next)
	}
	if runtimeReconciliationRequestBaseIdentity(next.RequestID) != runtimeReconciliationRequestBaseIdentity(first.RequestID) {
		t.Fatalf("same binding set did not preserve its request intent: first=%q next=%q", first.RequestID, next.RequestID)
	}
}
