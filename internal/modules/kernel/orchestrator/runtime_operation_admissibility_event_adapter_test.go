package orchestrator

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAcquireRuntimeOperationAdmissibilityPrerequisiteRejectsInvalidScopeAndOwnerResult(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(4)
	t.Cleanup(func() { hub.Terminate(ctx) })
	valid := orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand{
		WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: "snapshot-v2", PrerequisiteID: "admissibility-file",
		OwnerModuleID: runtimecontract.RuntimeModuleFile, WorkspaceDeliverableDirectory: t.TempDir(), RequiredKinds: []string{runtimecontract.StepKindFileEdit}, MaxItems: 4,
	}
	if _, err := AcquireRuntimeOperationAdmissibilityPrerequisite(ctx, hub, orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand{}); err == nil {
		t.Fatal("acquire accepted an empty workspace/run/authority scope")
	}
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, func(_ magicevent.Event, result magicevent.Result) {
		inventory, err := orchestratorevents.NewRuntimeOperationAdmissibility(valid.PrerequisiteID, runtimecontract.RuntimeModuleShell, []string{runtimecontract.StepKindShellCommand}, nil, nil, nil, "wrong-owner")
		if err != nil {
			t.Fatal(err)
		}
		result.Set(orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{PrerequisiteID: valid.PrerequisiteID, Admissibility: inventory}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility) })
	if _, err := AcquireRuntimeOperationAdmissibilityPrerequisite(ctx, hub, valid); err == nil {
		t.Fatal("acquire accepted a mismatched owner inventory")
	}
}

func TestProbeRuntimeOperationFeasibilityRejectsMalformedResult(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(4)
	t.Cleanup(func() { hub.Terminate(ctx) })
	inventory, err := orchestratorevents.NewRuntimeOperationAdmissibility(
		"admissibility-file", runtimecontract.RuntimeModuleFile, []string{runtimecontract.StepKindFileEdit}, []string{"target.txt"}, nil, nil, "revision-v2",
	)
	if err != nil {
		t.Fatal(err)
	}
	path, oldText, newText := "target.txt", "old", "new"
	command := orchestratorevents.ProbeRuntimeOperationFeasibilityCommand{
		WorkspaceID: "ws-v2", RunID: "run-v2", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-v2"},
		WorkspaceDeliverableDirectory: t.TempDir(), Admissibility: inventory,
		Operation: runtimecontract.RuntimeOperation{ID: "edit-1", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"target.txt"}, Parameters: []runtimecontract.OperationParameter{
			{Name: "path", String: &path}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText},
		}},
	}
	if _, err := ProbeRuntimeOperationFeasibility(ctx, hub, orchestratorevents.ProbeRuntimeOperationFeasibilityCommand{}); err == nil {
		t.Fatal("probe accepted an empty workspace/run/authority scope")
	}
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{
			OperationID: "wrong-operation", AdmissibilitySnapshotID: "wrong-snapshot", Feasible: true,
		}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility) })
	if _, err := ProbeRuntimeOperationFeasibility(ctx, hub, command); err == nil {
		t.Fatal("probe accepted a mismatched operation/snapshot result")
	}

	if err := orchestratorevents.ValidateRuntimeOperationAdmissibilityCommand(orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand{
		WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: "snapshot-v2", PrerequisiteID: "admissibility-file",
		OwnerModuleID: runtimecontract.RuntimeModuleFile, WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		RequiredKinds: []string{runtimecontract.StepKindFileEdit}, MaxItems: runtimecontract.RuntimeCandidateContextItemsMaxCount + 1,
	}, runtimecontract.RuntimeModuleFile); err == nil {
		t.Fatal("inventory command accepted an out-of-bound candidate context size")
	}
}
