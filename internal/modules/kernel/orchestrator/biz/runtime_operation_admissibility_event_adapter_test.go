package orchestrator

// Owner Biz operation tests.

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mustRuntimeOperationAuthorizationIdentity(t *testing.T, kind string) runtimecontract.RuntimeCandidateOperationManifest {
	t.Helper()
	authorization, err := runtimecontract.RuntimeOperationAuthorizationIdentity(kind)
	if err != nil {
		t.Fatalf("runtime operation authorization identity for %q: %v", kind, err)
	}
	return authorization
}

func TestAcquireRuntimeOperationAdmissibilityPrerequisiteRejectsInvalidScopeAndOwnerResult(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(4)
	t.Cleanup(func() { hub.Terminate(ctx) })
	valid := runtimecontract.RuntimeOperationAdmissibilityRequest{
		WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: "snapshot-v2", PrerequisiteID: "admissibility-file",
		OwnerModuleID: runtimecontract.RuntimeModuleFile, WorkspaceDeliverableDirectory: t.TempDir(), RequiredKinds: []string{runtimecontract.StepKindFileEdit}, MaxItems: 4,
	}
	if _, err := AcquireRuntimeOperationAdmissibilityPrerequisite(ctx, hub, runtimecontract.RuntimeOperationAdmissibilityRequest{}); err == nil {
		t.Fatal("acquire accepted an empty workspace/run/authority scope")
	}
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	observer.Subscribe(fileevents.TopicAcquireRuntimeOperationAdmissibility, func(_ magicevent.Event, result magicevent.Result) {
		inventory, err := runtimecontract.NewRuntimeOperationAdmissibility(valid.PrerequisiteID, runtimecontract.RuntimeModuleShell, []string{runtimecontract.StepKindShellCommand}, nil, nil, nil, "wrong-owner")
		if err != nil {
			t.Fatal(err)
		}
		result.Set(fileevents.AcquireRuntimeOperationAdmissibilityResult{Acquisition: runtimecontract.RuntimeOperationAdmissibilityAcquisition{
			WorkspaceID: valid.WorkspaceID, RunID: valid.RunID, AuthoritySnapshotID: valid.AuthoritySnapshotID, Admissibility: inventory,
		}}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(fileevents.TopicAcquireRuntimeOperationAdmissibility) })
	if _, err := AcquireRuntimeOperationAdmissibilityPrerequisite(ctx, hub, valid); err == nil {
		t.Fatal("acquire accepted a mismatched owner inventory")
	}
}

func TestProbeRuntimeOperationFeasibilityRejectsMalformedResult(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(4)
	t.Cleanup(func() { hub.Terminate(ctx) })
	inventory, err := runtimecontract.NewRuntimeOperationAdmissibility(
		"admissibility-file", runtimecontract.RuntimeModuleFile, []string{runtimecontract.StepKindFileEdit}, []string{"target.txt"}, nil, nil, "revision-v2",
	)
	if err != nil {
		t.Fatal(err)
	}
	oldText, newText := "old", "new"
	command := runtimecontract.RuntimeOperationFeasibilityRequest{
		WorkspaceID: "ws-v2", RunID: "run-v2", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-v2"},
		WorkspaceDeliverableDirectory: t.TempDir(), Admissibility: inventory,
		CandidateManifestHash: "manifest-test", OperationAuthorization: mustRuntimeOperationAuthorizationIdentity(t, runtimecontract.StepKindFileEdit),
		Operation: runtimecontract.RuntimeOperation{ID: "edit-1", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"target.txt"}, Parameters: []runtimecontract.OperationParameter{
			{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText},
		}},
	}
	if _, err := ProbeRuntimeOperationFeasibility(ctx, hub, runtimecontract.RuntimeOperationFeasibilityRequest{}); err == nil {
		t.Fatal("probe accepted an empty workspace/run/authority scope")
	}
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	observer.Subscribe(fileevents.TopicProbeRuntimeOperationFeasibility, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(fileevents.ProbeRuntimeOperationFeasibilityResult{
			OperationID: "wrong-operation", AdmissibilitySnapshotID: "wrong-snapshot", Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible,
		}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(fileevents.TopicProbeRuntimeOperationFeasibility) })
	if _, err := ProbeRuntimeOperationFeasibility(ctx, hub, command); err == nil {
		t.Fatal("probe accepted a mismatched operation/snapshot result")
	}
	observer.Unsubscribe(fileevents.TopicProbeRuntimeOperationFeasibility)
	observer.Subscribe(fileevents.TopicProbeRuntimeOperationFeasibility, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(fileevents.ProbeRuntimeOperationFeasibilityResult{
			OperationID: command.Operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID,
			CandidateManifestHash: "", Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible,
		}, nil)
	})
	if _, err := ProbeRuntimeOperationFeasibility(ctx, hub, command); err == nil {
		t.Fatal("probe accepted an owner result without the exact candidate manifest identity")
	}

	if err := runtimecontract.ValidateRuntimeOperationAdmissibilityRequest(runtimecontract.RuntimeOperationAdmissibilityRequest{
		WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: "snapshot-v2", PrerequisiteID: "admissibility-file",
		OwnerModuleID: runtimecontract.RuntimeModuleFile, WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		RequiredKinds: []string{runtimecontract.StepKindFileEdit}, MaxItems: runtimecontract.RuntimeOwnerInventoryItemsMaxCount + 1,
	}, runtimecontract.RuntimeModuleFile); err == nil {
		t.Fatal("inventory command accepted an out-of-bound owner inventory size")
	}
}
