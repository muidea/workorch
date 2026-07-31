package git

import (
	"context"
	"os/exec"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestGitOwnerReconciliationDoesNotTreatRepositoryRevisionAsSemanticProof(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	receipts, err := NewCapabilityStateReceiptStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner, err := New(hub, nil, Dependencies{RuntimeAdapter: NewRuntimeAdapter(false), CapabilityStateReceipts: receipts})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	workspace := t.TempDir()
	if output, err := exec.Command("git", "-C", workspace, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, output)
	}
	binding := runtimecontract.RuntimeCWIReconciliationBindingRef{
		CoverageWorkItemID:    common.CoverageWorkItemID("cwi-git"),
		BindingID:             "binding-git",
		BindingHash:           "binding-hash-git",
		OwnerModuleIDs:        []string{modulecommon.ModuleID},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AllowedOperationKinds: []string{modulecommon.StepKindRead},
		RequiredCheckRefs:     []string{"check/commit-subject"},
	}
	command := orchestratorevents.QueryRuntimeCWIReconciliationCommand{
		WorkspaceID:                   "ws-git-reconciliation",
		RunID:                         "run-git-reconciliation",
		RequestID:                     "request-git-reconciliation",
		Generation:                    1,
		DefinitionRevision:            "definition-git-reconciliation",
		BindingSetHash:                "binding-set-git-reconciliation",
		OwnerModuleID:                 modulecommon.ModuleID,
		Binding:                       binding,
		WorkspaceDeliverableDirectory: workspace,
		WorkspaceBaselineRevision:     "tree-git-reconciliation",
		WorkspaceBaselineRef:          "workspace-tree/tree-git-reconciliation",
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
		t.Fatalf("repository-only observation status = %q, want unverifiable", result.Observation.Status)
	}
	if result.Observation.OriginalDiagnostic == nil || result.Observation.OriginalDiagnostic.OriginalCode != "git_proof_incomplete" {
		t.Fatalf("repository-only diagnostic = %+v", result.Observation.OriginalDiagnostic)
	}
}
