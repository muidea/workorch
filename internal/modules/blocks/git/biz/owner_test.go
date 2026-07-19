package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestValidateCapabilityStatePredecessorReceiptsRequiresExactDurableBinding(t *testing.T) {
	snapshot := runtimecontract.RuntimeCapabilityStateSnapshot{RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{
		{RequirementID: "repo-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"},
		{RequirementID: "stage", Kind: runtimecontract.StepKindGitWrite, VariantID: "add", DependsOnRequirementIDs: []string{"repo-init"}},
	}}
	required := snapshot.RequiredOperationSequence[1]
	binding := runtimecontract.RuntimeRequiredOperationReceiptBinding{
		RequirementID: "repo-init", OperationID: "git-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init",
		ExecutionKey: "exec/ws/run/dispatch/git-init", Fence: 1, ReceiptRef: "receipt/git-init", ReceiptHash: "receipt-hash",
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true,
	}
	if err := validateGitCapabilityStatePredecessorReceipts(snapshot, required, []runtimecontract.RuntimeRequiredOperationReceiptBinding{binding}); err != nil {
		t.Fatalf("accepted predecessor binding: %v", err)
	}
	if err := validateGitCapabilityStatePredecessorReceipts(snapshot, required, nil); err == nil {
		t.Fatal("missing predecessor binding was accepted")
	}
	binding.VariantID = "commit"
	if err := validateGitCapabilityStatePredecessorReceipts(snapshot, required, []runtimecontract.RuntimeRequiredOperationReceiptBinding{binding}); err == nil {
		t.Fatal("variant-mismatched predecessor binding was accepted")
	}
}

func TestRuntimeOperationAdmissibilityProbeUsesCurrentRepositorySnapshot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	workspace := t.TempDir()
	readme := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readme, []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.local"},
		{"config", "user.name", "WorkOrch Test"},
		{"add", "README.md"},
		{"commit", "-m", "initial"},
	} {
		if output, err := exec.Command("git", append([]string{"-C", workspace}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	module := &Biz{runtimeAdapter: NewRuntimeAdapter(true)}
	snapshot, err := module.runtimeAdapter.CapabilityState(context.Background(), "admissibility-git", "workspace_deliverable", workspace)
	if err != nil {
		t.Fatalf("acquire read-only repository state: %v", err)
	}
	inventory, err := orchestratorevents.NewRuntimeOperationAdmissibility(
		"admissibility-git", ModuleID, []string{StepKindRead}, nil, nil,
		[]string{"repository-state-snapshot:" + snapshot.SnapshotID}, snapshot.ObservedRevision,
	)
	if err != nil {
		t.Fatal(err)
	}
	operationName := "status"
	command := orchestratorevents.ProbeRuntimeOperationFeasibilityCommand{
		WorkspaceID: "ws-git", RunID: "run-git", WorkspaceDeliverableDirectory: workspace,
		Transaction:   runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-git"},
		Admissibility: inventory,
		Operation:     runtimecontract.RuntimeOperation{ID: "status", Kind: StepKindRead, Parameters: []runtimecontract.OperationParameter{{Name: "operation", String: &operationName}}},
	}
	if failure := module.runtimeOperationFeasibility(context.Background(), command); failure != nil {
		t.Fatalf("current repository snapshot rejected admissible status: %+v", failure)
	}
	if err := os.WriteFile(readme, []byte("changed after inventory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	failure := module.runtimeOperationFeasibility(context.Background(), command)
	if failure == nil || failure.Code != "operation_admissibility_stale" {
		t.Fatalf("stale repository snapshot failure = %+v", failure)
	}
}
