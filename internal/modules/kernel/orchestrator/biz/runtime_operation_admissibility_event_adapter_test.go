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
	descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(kind)
	if !ok {
		t.Fatalf("runtime operation authorization identity has unknown kind %q", kind)
	}
	return runtimecontract.RuntimeCandidateOperationManifest{
		Kind: descriptor.Kind, OwnerModuleID: descriptor.OwnerModuleID,
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy:            runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			OwnerSelectorRefs: []string{"test/owner-feasibility"},
		},
	}.Normalized()
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

func TestProbeRuntimeOperationFeasibilityAcceptsManifestBoundCandidateApplicabilityViolation(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(4)
	t.Cleanup(func() { hub.Terminate(ctx) })
	contract, found := runtimecontract.RuntimeOperationContractForKind(runtimecontract.RuntimeBlockKindPatchApply)
	if !found {
		t.Fatal("patch/apply operation contract is missing")
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(runtimecontract.RuntimeBlockKindPatchApply)
	if !found {
		t.Fatal("patch/apply operation descriptor is missing")
	}
	inventory, err := runtimecontract.NewRuntimeOperationAdmissibility(
		"admissibility-file", runtimecontract.RuntimeModuleFile, []string{runtimecontract.RuntimeBlockKindPatchApply}, []string{"main.go"}, nil, nil, "tree-v2",
	)
	if err != nil {
		t.Fatal(err)
	}
	patchType := "unified_diff"
	diff := "--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n"
	command := runtimecontract.RuntimeOperationFeasibilityRequest{
		WorkspaceID: "ws-v2", RunID: "run-v2", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-v2"},
		WorkspaceDeliverableDirectory: t.TempDir(), Admissibility: inventory, CandidateManifestHash: "manifest-applicability",
		OperationAuthorization: runtimecontract.RuntimeCandidateOperationManifest{
			Kind: contract.Kind, OwnerModuleID: descriptor.OwnerModuleID, OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
			Parameters: contract.Parameters, Variants: contract.Variants,
			Target: runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{"file/workspace-regular-files"}},
		}.Normalized(),
		Operation: runtimecontract.RuntimeOperation{ID: "patch-main", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"main.go"}, Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff},
		}},
	}
	response, err := fileevents.NewProbeRuntimeOperationCandidateConstraintViolationResult(fileevents.ProbeRuntimeOperationFeasibilityCommand(command), runtimecontract.RuntimeOperationCandidateConstraintViolation{
		ConstraintRef: "patch/unified_diff/diff/observed_target", ParameterName: "diff",
		RequiredShape: runtimecontract.RuntimeOperationCandidateRequiredShapeUnifiedDiffApplicableToObservedTarget,
		ActualShape:   runtimecontract.RuntimeOperationCandidateActualShapeObservedTargetContextMismatch,
		TargetRef:     "main.go", ObservedRevision: inventory.ObservedRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	ownerResponse := runtimecontract.RuntimeOperationFeasibilityObservation(response)
	ownerResponse.OwnerPolicy = runtimecontract.RuntimeOwnerPolicyIdentity{OwnerModuleID: runtimecontract.RuntimeModuleFile, PolicyRevision: "file-owner-feasibility/v1", CatalogSnapshotID: inventory.SnapshotID}
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	observer.Subscribe(fileevents.TopicProbeRuntimeOperationFeasibility, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(fileevents.ProbeRuntimeOperationFeasibilityResult(ownerResponse), nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(fileevents.TopicProbeRuntimeOperationFeasibility) })

	observed, err := ProbeRuntimeOperationFeasibility(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Outcome != runtimecontract.RuntimeOwnerFeasibilityCandidateConstraintViolated || observed.CandidateConstraintViolation == nil ||
		observed.CandidateConstraintViolation.TargetRef != "main.go" || observed.CandidateConstraintViolation.ObservedRevision != inventory.ObservedRevision || observed.Feedback != nil {
		t.Fatalf("candidate applicability observation = %+v", observed)
	}

	observer.Unsubscribe(fileevents.TopicProbeRuntimeOperationFeasibility)
	tampered := ownerResponse
	violation := *ownerResponse.CandidateConstraintViolation
	violation.ObservedRevision = "forged-tree-revision"
	tampered.CandidateConstraintViolation = &violation
	observer.Subscribe(fileevents.TopicProbeRuntimeOperationFeasibility, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(fileevents.ProbeRuntimeOperationFeasibilityResult(tampered), nil)
	})
	if _, err := ProbeRuntimeOperationFeasibility(ctx, hub, command); err == nil {
		t.Fatal("candidate applicability adapter accepted a forged owner revision")
	}
}
