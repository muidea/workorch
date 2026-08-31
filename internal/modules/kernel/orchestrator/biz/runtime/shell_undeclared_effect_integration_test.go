package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// TestShellUndeclaredWorkspaceEffectCommitsAuditWithoutTargetAuthority crosses
// the real Shell -> EventHub -> Orchestrator receipt boundary. The unexpected
// path is retained as an owner-observed resource change, but cannot enter the
// authority-shaped diagnostic fields of the rejected receipt or Runtime facts.
func TestShellUndeclaredWorkspaceEffectCommitsAuditWithoutTargetAuthority(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	shellOwner, err := shellbiz.New(hub, nil, shellbiz.Dependencies{RuntimeAdapter: shellbiz.NewRuntimeAdapter()})
	if err != nil {
		t.Fatal(err)
	}
	if err := shellOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shellOwner.Teardown(ctx) })

	workspaceDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceDirectory, "generated.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateChanging := true
	manifestObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: "manifest:shell-undeclared-effect", Kind: runtimecontract.PrerequisiteKindManifest,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, ContentHash: "tree-shell-undeclared-effect",
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: "tree-shell-undeclared-effect",
		}},
	}
	manifestObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(manifestObservation)
	acquisitionCommand := shellevents.AcquireRuntimeOperationAdmissibilityCommand{
		WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: "authority-shell-undeclared-effect",
		PrerequisiteID: "operation_admissibility:shell-undeclared-effect", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		WorkspaceDeliverableDirectory: workspaceDirectory, RequiredKinds: []string{runtimecontract.StepKindShellCommand},
		RequiredTargetRefs: []string{"generated.txt"}, MaxItems: 4,
		WorkspaceManifestBinding: runtimecontract.RuntimeWorkspaceManifestBinding{
			Manifest:    runtimecontract.WorkspaceManifest{Revision: manifestObservation.ContentHash, TargetRefs: []string{"generated.txt"}},
			Observation: manifestObservation,
		},
	}
	acquisitionResult := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleShell,
		magicevent.NewHeader(), ctx, acquisitionCommand,
	))
	if acquisitionResult == nil {
		t.Fatal("Shell owner returned no operation admissibility result")
	}
	acquisitionData, acquisitionErr := acquisitionResult.Get()
	if acquisitionErr != nil {
		t.Fatalf("Shell operation admissibility error: %v", acquisitionErr)
	}
	acquisition, ok := acquisitionData.(shellevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok {
		t.Fatalf("Shell operation admissibility result = %T", acquisitionData)
	}
	catalog := acquisition.Acquisition.Admissibility
	observation := acquisition.Acquisition.Observation
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.StepKindShellCommand},
	}

	facts := controllerFacts()
	// The declared operation target is part of the immutable Definition focus;
	// only the additional rogue path is undeclared owner-observed audit data.
	facts.Focus.TargetRefs = []string{"generated.txt"}
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindShellCommand}
	facts.Activation.OperationPolicies = nil
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized := runtimeTestInitializeAndSealSemanticContract(t, ctx, controller, facts, controllerDefinition(), nil)
	command := "sh"
	args := []string{"-c", "printf declared > generated.txt; printf unexpected > rogue.txt"}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		Kind: runtimecontract.StepKindShellCommand, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		TargetRefs: []string{"generated.txt"}, Parameters: []runtimecontract.OperationParameter{
			{Name: "command", String: &command}, {Name: "args", StringList: args}, {Name: "stateChanging", Boolean: &stateChanging},
		},
	}}}}}
	sealed, err := beginAndAcceptCandidate(t, controller, initialized, candidate)
	if err != nil {
		t.Fatal(err)
	}
	registerRuntimeOperationReceiptOwner(t, hub, controller)
	persisted, err := controller.executeActiveDispatch(ctx, facts.WorkspaceID, facts.RunID, runtimeObservedShellExecutor{
		hub: hub, deliverableDirectory: workspaceDirectory,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.OperationReceipts) != 1 || len(persisted.CompletedDispatches) != 1 || persisted.ActiveDispatch != nil {
		t.Fatalf("rejected validation-owner receipt did not complete through Runtime owner: %+v", persisted)
	}
	receipt := persisted.OperationReceipts[0].Normalized()
	diagnostic := receipt.Feedback.OriginalDiagnostic
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || !receipt.Changed ||
		receipt.ObservedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable || diagnostic == nil ||
		diagnostic.OriginalClass != string(runtimecontract.FailureClassEffectMismatch) || diagnostic.OriginalCode != "undeclared_workspace_effect" {
		t.Fatalf("committed undeclared-effect receipt = %+v", receipt)
	}
	if len(receipt.TargetRefs) != 1 || receipt.TargetRefs[0] != "generated.txt" || len(diagnostic.TargetRefs) != 0 ||
		diagnostic.TargetObservation != nil || diagnostic.ValidationFailureObservation != nil {
		t.Fatalf("undeclared audit subject escaped into target authority: receipt=%+v diagnostic=%+v", receipt.TargetRefs, diagnostic)
	}
	changedTargets := map[string]bool{}
	for _, change := range receipt.ResourceChanges {
		if change.Domain == runtimecontract.RuntimeResourceWorkspaceTarget && change.OwnerModuleID == runtimecontract.RuntimeModuleFile {
			changedTargets[change.SubjectRef] = true
		}
	}
	if !changedTargets["generated.txt"] || !changedTargets["rogue.txt"] {
		t.Fatalf("receipt lost declared/undeclared mutation audit: %+v", receipt.ResourceChanges)
	}
	if persisted.Facts.Failure == nil || len(persisted.Facts.Failure.TargetRefs) != 0 ||
		persisted.Facts.Failure.TargetObservation != nil || persisted.Facts.Resources.FailureTarget != nil {
		t.Fatalf("undeclared effect became Runtime repair-target authority: %+v", persisted.Facts)
	}
	dispatch := persisted.CompletedDispatches[0]
	execution := dispatch.OperationExecutions[0]
	replayed, found, err := controller.QueryOperationReceipt(ctx, facts.WorkspaceID, facts.RunID, dispatch.ID, execution.OperationID, execution.ExecutionKey, execution.Fence, runtimecontract.RuntimeModuleShell, nil)
	if err != nil || !found || runtimecontract.StableRuntimeOperationReceiptHash(replayed) != runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
		t.Fatalf("real receipt ingress did not persist exact replay fact: found=%t receipt=%+v err=%v sealed=%+v", found, replayed, err, sealed.ActiveDispatch)
	}
}
