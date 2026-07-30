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

	feasible, err := NewProbeRuntimeOperationFeasibilityResult(command, true, "", nil, nil, nil, nil)
	if err != nil || !feasible.Feasible || feasible.Failure != nil || feasible.Stale != nil || feasible.CandidateManifestHash != "manifest-n5" {
		t.Fatalf("feasible observation = %+v err=%v", feasible, err)
	}

	failure := RuntimeOperationInfeasibleFailure(command.Operation, "operation_target_missing", "not currently feasible")
	infeasible, err := NewProbeRuntimeOperationFeasibilityResult(command, false, runtimecontract.RuntimeOwnerFeasibilityCategoryResourceAbsent, failure, nil, nil, nil)
	if err != nil || infeasible.Feasible || infeasible.Failure == nil || infeasible.Stale != nil || infeasible.Feedback == nil {
		t.Fatalf("infeasible observation = %+v err=%v", infeasible, err)
	}
	if infeasible.Feedback.Domain != runtimecontract.RuntimeFeedbackDomainFeasibility || infeasible.Feedback.StableCategory != runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOwnerFeasibilityCategoryResourceAbsent) ||
		infeasible.Feedback.WorkspaceID != command.WorkspaceID || infeasible.Feedback.RunID != command.RunID || infeasible.Feedback.AuthoritySnapshotID != command.Transaction.AuthoritySnapshotID ||
		infeasible.Feedback.OriginalDiagnostic == nil || infeasible.Feedback.OriginalDiagnostic.OwnerModuleID != runtimecontract.RuntimeModuleFile || infeasible.Feedback.OriginalDiagnostic.OperationID != command.Operation.ID ||
		infeasible.Feedback.OriginalDiagnostic.OriginalCode != failure.Code || infeasible.Feedback.OriginalDiagnostic.OriginalMessage != failure.Message {
		t.Fatalf("infeasible feedback lost owner diagnostic: %+v", infeasible.Feedback)
	}
	tampered := infeasible
	tamperedFeedback := infeasible.Feedback.CloneOriginal()
	tampered.Feedback = &tamperedFeedback
	tampered.Feedback.OriginalDiagnostic.OriginalCode = "different-code"
	if _, err := NormalizeProbeRuntimeOperationFeasibilityResult(tampered); err == nil {
		t.Fatal("feasibility result accepted diagnostic/category drift")
	}

	stale := &runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: inventory.PrerequisiteID, ObservationID: "observation-file",
		OwnerModuleID: runtimecontract.RuntimeModuleFile, Domain: runtimecontract.RuntimeResourceWorkspaceTree,
		SubjectRef: "workspace_deliverable", ExpectedRevision: "revision-v2", ObservedRevision: "revision-v3",
		Reason: "workspace tree revision drifted",
	}
	staleResult, err := NewProbeRuntimeOperationFeasibilityResult(command, false, "", nil, stale, nil, nil)
	if err != nil || staleResult.Feasible || staleResult.Failure != nil || staleResult.Stale == nil {
		t.Fatalf("stale observation = %+v err=%v", staleResult, err)
	}

	if _, err := NewProbeRuntimeOperationFeasibilityResult(command, true, runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed, failure, nil, nil, nil); err == nil {
		t.Fatal("feasible+failure mixed observation accepted")
	}
	if _, err := NewProbeRuntimeOperationFeasibilityResult(command, false, runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed, failure, stale, nil, nil); err == nil {
		t.Fatal("stale+failure mixed observation accepted")
	}
	if _, err := NewProbeRuntimeOperationFeasibilityResult(command, false, "", nil, nil, nil, nil); err == nil {
		t.Fatal("empty non-feasible observation accepted")
	}
	unknown := RuntimeOperationInfeasibleFailure(command.Operation, "file_unmapped_code", "not currently feasible")
	if _, err := NewProbeRuntimeOperationFeasibilityResult(command, false, "", unknown, nil, nil, nil); err == nil {
		t.Fatal("unclassified owner feasibility result accepted without a typed defect category")
	}
	defect, err := NewProbeRuntimeOperationFeasibilityResult(command, false, runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect, unknown, nil, nil, nil)
	if err != nil || defect.Feasible || defect.Failure == nil || defect.Feedback == nil || defect.Category != runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
		t.Fatalf("typed owner contract defect = %+v err=%v", defect, err)
	}
	if defect.Feedback.OriginalDiagnostic == nil || defect.Feedback.OriginalDiagnostic.OriginalCode != unknown.Code || defect.Feedback.OriginalDiagnostic.OriginalMessage != unknown.Message {
		t.Fatalf("typed owner contract defect lost original diagnostic: %+v", defect.Feedback)
	}
	ownerObservation, err := runtimecontract.RuntimeOwnerFeasibilityObservationFromFailure(defect.Category, defect.Feedback, defect.Failure, nil, nil)
	if err != nil || ownerObservation.Disposition != runtimecontract.RuntimeOwnerFeasibilityContractDefect || ownerObservation.Feedback == nil || ownerObservation.Feedback.OriginalDiagnostic.OriginalCode != unknown.Code {
		t.Fatalf("typed owner contract defect observation = %+v err=%v", ownerObservation, err)
	}
}
