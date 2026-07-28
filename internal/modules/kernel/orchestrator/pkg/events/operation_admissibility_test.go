package events

import (
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestValidateRuntimeOperationFeasibilityCommandRequiresSealedIdentity(t *testing.T) {
	inventory, err := NewRuntimeOperationAdmissibility(
		"admissibility-file", runtimecontract.RuntimeModuleFile, []string{runtimecontract.StepKindFileEdit}, []string{"target.txt"}, nil, nil, "revision-v2",
	)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := RuntimeOperationAuthorizationIdentity(runtimecontract.StepKindFileEdit)
	if err != nil {
		t.Fatal(err)
	}
	oldText, newText := "old", "new"
	base := ProbeRuntimeOperationFeasibilityCommand{
		WorkspaceID: "ws-v2", RunID: "run-v2", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-v2"},
		WorkspaceDeliverableDirectory: t.TempDir(), Admissibility: inventory,
		CandidateManifestHash: "manifest-n5", OperationAuthorization: authorization,
		Operation: runtimecontract.RuntimeOperation{ID: "edit-1", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"target.txt"}, Parameters: []runtimecontract.OperationParameter{
			{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText},
		}},
	}
	if err := ValidateRuntimeOperationFeasibilityCommand(base, runtimecontract.RuntimeModuleFile); err != nil {
		t.Fatalf("valid sealed feasibility command rejected: %v", err)
	}

	missingHash := base
	missingHash.CandidateManifestHash = ""
	if err := ValidateRuntimeOperationFeasibilityCommand(missingHash, runtimecontract.RuntimeModuleFile); err == nil || !strings.Contains(err.Error(), "manifest hash") {
		t.Fatalf("missing manifest hash error = %v", err)
	}

	missingAuth := base
	missingAuth.OperationAuthorization = runtimecontract.RuntimeCandidateOperationManifest{}
	if err := ValidateRuntimeOperationFeasibilityCommand(missingAuth, runtimecontract.RuntimeModuleFile); err == nil || !strings.Contains(err.Error(), "operation authorization") {
		t.Fatalf("missing authorization error = %v", err)
	}

	wrongOwner := base
	if err := ValidateRuntimeOperationFeasibilityCommand(wrongOwner, runtimecontract.RuntimeModuleShell); err == nil {
		t.Fatal("wrong owner receiver accepted sealed file command")
	}
}

func TestNewProbeRuntimeOperationFeasibilityResultEnforcesOuterXOR(t *testing.T) {
	inventory, err := NewRuntimeOperationAdmissibility(
		"admissibility-file", runtimecontract.RuntimeModuleFile, []string{runtimecontract.StepKindFileEdit}, []string{"target.txt"}, nil, nil, "revision-v2",
	)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := RuntimeOperationAuthorizationIdentity(runtimecontract.StepKindFileEdit)
	if err != nil {
		t.Fatal(err)
	}
	path := "target.txt"
	command := ProbeRuntimeOperationFeasibilityCommand{
		WorkspaceID: "ws-v2", RunID: "run-v2", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-v2"},
		WorkspaceDeliverableDirectory: t.TempDir(), Admissibility: inventory, CapabilityStateBinding: inventory.AcquisitionBinding,
		CandidateManifestHash: "manifest-n5", OperationAuthorization: authorization,
		Operation: runtimecontract.RuntimeOperation{ID: "edit-1", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{path}},
	}

	feasible, err := NewProbeRuntimeOperationFeasibilityResult(command, true, nil, nil, nil, nil)
	if err != nil || !feasible.Feasible || feasible.Failure != nil || feasible.Stale != nil || feasible.CandidateManifestHash != "manifest-n5" {
		t.Fatalf("feasible observation = %+v err=%v", feasible, err)
	}

	failure := RuntimeOperationInfeasibleFailure(command.Operation, "operation_infeasible", "not currently feasible")
	infeasible, err := NewProbeRuntimeOperationFeasibilityResult(command, false, failure, nil, nil, nil)
	if err != nil || infeasible.Feasible || infeasible.Failure == nil || infeasible.Stale != nil {
		t.Fatalf("infeasible observation = %+v err=%v", infeasible, err)
	}

	stale := &runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: inventory.PrerequisiteID, ObservationID: "observation-file",
		OwnerModuleID: runtimecontract.RuntimeModuleFile, Domain: runtimecontract.RuntimeResourceWorkspaceTree,
		SubjectRef: "workspace_deliverable", ExpectedRevision: "revision-v2", ObservedRevision: "revision-v3",
		Reason: "workspace tree revision drifted",
	}
	staleResult, err := NewProbeRuntimeOperationFeasibilityResult(command, false, nil, stale, nil, nil)
	if err != nil || staleResult.Feasible || staleResult.Failure != nil || staleResult.Stale == nil {
		t.Fatalf("stale observation = %+v err=%v", staleResult, err)
	}

	if _, err := NewProbeRuntimeOperationFeasibilityResult(command, true, failure, nil, nil, nil); err == nil {
		t.Fatal("feasible+failure mixed observation accepted")
	}
	if _, err := NewProbeRuntimeOperationFeasibilityResult(command, false, failure, stale, nil, nil); err == nil {
		t.Fatal("stale+failure mixed observation accepted")
	}
	if _, err := NewProbeRuntimeOperationFeasibilityResult(command, false, nil, nil, nil, nil); err == nil {
		t.Fatal("empty non-feasible observation accepted")
	}
}
