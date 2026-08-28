package workorchruntimecontract_test

import (
	"errors"
	"strings"
	"testing"

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

func TestValidateRuntimeOperationFeasibilityRequestRequiresSealedIdentity(t *testing.T) {
	inventory, err := runtimecontract.NewRuntimeOperationAdmissibility(
		"admissibility-file", runtimecontract.RuntimeModuleFile, []string{runtimecontract.StepKindFileEdit}, []string{"target.txt"}, nil, nil, "revision-v2",
	)
	if err != nil {
		t.Fatal(err)
	}
	authorization := mustRuntimeOperationAuthorizationIdentity(t, runtimecontract.StepKindFileEdit)
	oldText, newText := "old", "new"
	base := runtimecontract.RuntimeOperationFeasibilityRequest{
		WorkspaceID: "ws-v2", RunID: "run-v2", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-v2"},
		WorkspaceDeliverableDirectory: t.TempDir(), Admissibility: inventory,
		CandidateManifestHash: "manifest-n5", OperationAuthorization: authorization,
		Operation: runtimecontract.RuntimeOperation{ID: "edit-1", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"target.txt"}, Parameters: []runtimecontract.OperationParameter{
			{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText},
		}},
	}
	if err := runtimecontract.ValidateRuntimeOperationFeasibilityRequest(base, runtimecontract.RuntimeModuleFile); err != nil {
		t.Fatalf("valid sealed feasibility command rejected: %v", err)
	}

	missingHash := base
	missingHash.CandidateManifestHash = ""
	if err := runtimecontract.ValidateRuntimeOperationFeasibilityRequest(missingHash, runtimecontract.RuntimeModuleFile); err == nil || !strings.Contains(err.Error(), "manifest hash") {
		t.Fatalf("missing manifest hash error = %v", err)
	}

	missingAuth := base
	missingAuth.OperationAuthorization = runtimecontract.RuntimeCandidateOperationManifest{}
	if err := runtimecontract.ValidateRuntimeOperationFeasibilityRequest(missingAuth, runtimecontract.RuntimeModuleFile); err == nil || !strings.Contains(err.Error(), "operation authorization") {
		t.Fatalf("missing authorization error = %v", err)
	}
	missingTargetAuthorization := base
	missingTargetAuthorization.OperationAuthorization.Target = runtimecontract.RuntimeOperationTargetAuthorization{}
	if err := runtimecontract.ValidateRuntimeOperationFeasibilityRequest(missingTargetAuthorization, runtimecontract.RuntimeModuleFile); err == nil || !strings.Contains(err.Error(), "target authorization") {
		t.Fatalf("missing target authorization error = %v", err)
	}

	wrongOwner := base
	if err := runtimecontract.ValidateRuntimeOperationFeasibilityRequest(wrongOwner, runtimecontract.RuntimeModuleShell); err == nil {
		t.Fatal("wrong owner receiver accepted sealed file command")
	}
}

func TestValidateRuntimeOperationFeasibilityRequestUsesSealedTargetAuthorization(t *testing.T) {
	exactInventory, err := runtimecontract.NewRuntimeOperationAdmissibility(
		"admissibility-file-exact", runtimecontract.RuntimeModuleFile, []string{runtimecontract.StepKindFileEdit},
		[]string{"go.mod", "main.go", "semantic_test.go"}, nil, nil, "revision-exact",
	)
	if err != nil {
		t.Fatal(err)
	}
	exactAuthorization := mustRuntimeOperationAuthorizationIdentity(t, runtimecontract.StepKindFileEdit)
	exactAuthorization.RequireTargetRefs = true
	exactAuthorization.Target = runtimecontract.RuntimeOperationTargetAuthorization{
		Policy:          runtimecontract.RuntimeOperationTargetPolicyExactTarget,
		ExactTargetRefs: []string{"go.mod", "main.go", "semantic_test.go"},
	}
	request := runtimecontract.RuntimeOperationFeasibilityRequest{
		WorkspaceID: "ws-target-auth", RunID: "run-target-auth",
		Transaction:                   runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-target-auth"},
		WorkspaceDeliverableDirectory: t.TempDir(),
		Operation: runtimecontract.RuntimeOperation{
			ID: "edit-main", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"main.go"},
		},
		Admissibility: exactInventory, CandidateManifestHash: "manifest-target-auth", OperationAuthorization: exactAuthorization,
	}
	if err := runtimecontract.ValidateRuntimeOperationFeasibilityRequest(request, runtimecontract.RuntimeModuleFile); err != nil {
		t.Fatalf("exact multi-target authorization rejected legal singleton subset: %v", err)
	}

	outside := request
	outside.Operation.TargetRefs = []string{"outside.go"}
	err = runtimecontract.ValidateRuntimeOperationFeasibilityRequest(outside, runtimecontract.RuntimeModuleFile)
	var outsideFinding *runtimecontract.RuntimeOperationTargetAuthorizationError
	if !errors.As(err, &outsideFinding) || outsideFinding.Actual != "outside.go" {
		t.Fatalf("outside feasibility target finding=%+v err=%v", outsideFinding, err)
	}

	empty := request
	empty.Operation.TargetRefs = nil
	err = runtimecontract.ValidateRuntimeOperationFeasibilityRequest(empty, runtimecontract.RuntimeModuleFile)
	var emptyFinding *runtimecontract.RuntimeOperationTargetAuthorizationError
	if !errors.As(err, &emptyFinding) || emptyFinding.Actual != "missing_targetRefs" {
		t.Fatalf("empty feasibility target finding=%+v err=%v", emptyFinding, err)
	}

	prospectiveInventory, err := runtimecontract.NewRuntimeOperationAdmissibility(
		"admissibility-file-prospective", runtimecontract.RuntimeModuleFile, []string{runtimecontract.StepKindFileWrite},
		nil, []string{"workspace-relative-create"}, []string{"workspace-relative-path"}, "revision-prospective",
	)
	if err != nil {
		t.Fatal(err)
	}
	prospectiveAuthorization := mustRuntimeOperationAuthorizationIdentity(t, runtimecontract.StepKindFileWrite)
	prospectiveAuthorization.RequireTargetRefs = true
	prospectiveAuthorization.Target = runtimecontract.RuntimeOperationTargetAuthorization{
		Policy:            runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		OwnerSelectorRefs: []string{"workspace-relative-create"}, ProspectiveConstraintRefs: []string{"workspace-relative-path"},
		ProspectiveTargetRefs: []string{"main.go", "semantic_test.go"},
	}
	prospective := request
	prospective.Operation = runtimecontract.RuntimeOperation{ID: "write-main", Kind: runtimecontract.StepKindFileWrite, TargetRefs: []string{"main.go"}}
	prospective.Admissibility = prospectiveInventory
	prospective.OperationAuthorization = prospectiveAuthorization
	if err := runtimecontract.ValidateRuntimeOperationFeasibilityRequest(prospective, runtimecontract.RuntimeModuleFile); err != nil {
		t.Fatalf("prospective singleton feasibility target rejected: %v", err)
	}
	prospective.Operation.TargetRefs = []string{"main.go", "semantic_test.go"}
	err = runtimecontract.ValidateRuntimeOperationFeasibilityRequest(prospective, runtimecontract.RuntimeModuleFile)
	var singletonFinding *runtimecontract.RuntimeOperationTargetAuthorizationError
	if !errors.As(err, &singletonFinding) || !strings.Contains(singletonFinding.Error(), "prospective_exact_singleton_required") {
		t.Fatalf("prospective non-singleton finding=%+v err=%v", singletonFinding, err)
	}
}

func TestNewRuntimeOperationFeasibilityObservationEnforcesOuterXOR(t *testing.T) {
	inventory, err := runtimecontract.NewRuntimeOperationAdmissibility(
		"admissibility-file", runtimecontract.RuntimeModuleFile, []string{runtimecontract.StepKindFileEdit}, []string{"target.txt"}, nil, nil, "revision-v2",
	)
	if err != nil {
		t.Fatal(err)
	}
	authorization := mustRuntimeOperationAuthorizationIdentity(t, runtimecontract.StepKindFileEdit)
	path := "target.txt"
	command := runtimecontract.RuntimeOperationFeasibilityRequest{
		WorkspaceID: "ws-v2", RunID: "run-v2", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-v2"},
		WorkspaceDeliverableDirectory: t.TempDir(), Admissibility: inventory, CapabilityStateBinding: inventory.AcquisitionBinding,
		CandidateManifestHash: "manifest-n5", OperationAuthorization: authorization,
		Operation: runtimecontract.RuntimeOperation{ID: "edit-1", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{path}},
	}

	feasible, err := runtimecontract.NewRuntimeOperationFeasibilityObservation(command, "", nil, nil, nil, nil)
	if err != nil || feasible.Outcome != runtimecontract.RuntimeOwnerFeasibilityFeasible || feasible.Feedback != nil || feasible.Stale != nil || feasible.CandidateManifestHash != "manifest-n5" {
		t.Fatalf("feasible observation = %+v err=%v", feasible, err)
	}

	failure := runtimecontract.RuntimeOperationInfeasibleFailure(command.Operation, "operation_target_missing", "not currently feasible")
	infeasible, err := runtimecontract.NewRuntimeOperationFeasibilityObservation(command, runtimecontract.RuntimeOwnerFeasibilityCategoryResourceAbsent, failure, nil, nil, nil)
	if err != nil || infeasible.Outcome != runtimecontract.RuntimeOwnerFeasibilityInfeasible || infeasible.Stale != nil || infeasible.Feedback == nil {
		t.Fatalf("infeasible observation = %+v err=%v", infeasible, err)
	}
	if infeasible.Feedback.Domain != runtimecontract.RuntimeFeedbackDomainFeasibility || infeasible.Feedback.StableCategory != runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOwnerFeasibilityCategoryResourceAbsent) ||
		infeasible.Feedback.WorkspaceID != command.WorkspaceID || infeasible.Feedback.RunID != command.RunID || infeasible.Feedback.AuthoritySnapshotID != command.Transaction.AuthoritySnapshotID ||
		infeasible.Feedback.OriginalDiagnostic == nil || infeasible.Feedback.OriginalDiagnostic.OwnerModuleID != runtimecontract.RuntimeModuleFile || infeasible.Feedback.OriginalDiagnostic.OperationID != command.Operation.ID ||
		infeasible.Feedback.OriginalDiagnostic.OriginalCode != failure.Code || infeasible.Feedback.OriginalDiagnostic.OriginalMessage != failure.Message {
		t.Fatalf("infeasible feedback lost owner diagnostic: %+v", infeasible.Feedback)
	}
	contradiction := infeasible
	contradictionFeedback := infeasible.Feedback.CloneOriginal()
	contradiction.Feedback = &contradictionFeedback
	contradiction.Feedback.StableCategory = runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed)
	contradiction.Feedback.OriginalDiagnostic.OriginalCode = "operation_target_exists"
	contradiction.Feedback.OriginalDiagnostic.ObservedRevision = inventory.ObservedRevision
	contradiction.OwnerPolicy = runtimecontract.RuntimeOwnerPolicyIdentity{
		OwnerModuleID: runtimecontract.RuntimeModuleFile, PolicyRevision: "file-policy-v1", CatalogSnapshotID: inventory.SnapshotID,
	}
	contradiction.ConstraintImpact = runtimecontract.RuntimeOwnerFeasibilityConstraintImpactSealedTargetAuthorizationContradicted
	contradiction, err = runtimecontract.NormalizeRuntimeOperationFeasibilityObservation(contradiction)
	if err != nil {
		t.Fatalf("explicit operation contradiction rejected: %v", err)
	}
	ownerContradiction, err := runtimecontract.RuntimeOwnerFeasibilityObservationFromOperation(contradiction)
	if err != nil || !ownerContradiction.SealedTargetAuthorizationContradicted() || ownerContradiction.OwnerPolicy != contradiction.OwnerPolicy {
		t.Fatalf("operation contradiction conversion = %+v err=%v", ownerContradiction, err)
	}
	missingObservedRevision := contradiction
	missingRevisionFeedback := contradiction.Feedback.CloneOriginal()
	missingObservedRevision.Feedback = &missingRevisionFeedback
	missingObservedRevision.Feedback.OriginalDiagnostic.ObservedRevision = ""
	if _, err := runtimecontract.NormalizeRuntimeOperationFeasibilityObservation(missingObservedRevision); err == nil {
		t.Fatal("operation contradiction without observed revision was accepted")
	}
	unknownImpact := contradiction
	unknownImpact.ConstraintImpact = "owner_private_transition"
	if _, err := runtimecontract.NormalizeRuntimeOperationFeasibilityObservation(unknownImpact); err == nil {
		t.Fatal("unknown operation constraint impact was accepted")
	}
	tampered := infeasible
	tamperedFeedback := infeasible.Feedback.CloneOriginal()
	tampered.Feedback = &tamperedFeedback
	tampered.Feedback.StableCategory = "new_unmapped_category"
	if _, err := runtimecontract.NormalizeRuntimeOperationFeasibilityObservation(tampered); err == nil {
		t.Fatal("feasibility result accepted an unknown canonical stable category")
	}
	originalChanged := infeasible
	originalChangedFeedback := infeasible.Feedback.CloneOriginal()
	originalChanged.Feedback = &originalChangedFeedback
	originalChanged.Feedback.OriginalDiagnostic.OriginalCode = "producer-specific-code"
	normalizedOriginal, err := runtimecontract.NormalizeRuntimeOperationFeasibilityObservation(originalChanged)
	if err != nil || normalizedOriginal.Feedback.OriginalDiagnostic.OriginalCode != "producer-specific-code" {
		t.Fatalf("feasibility result did not preserve producer diagnostic: %+v err=%v", normalizedOriginal.Feedback, err)
	}

	stale := &runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: inventory.PrerequisiteID, ObservationID: "observation-file",
		OwnerModuleID: runtimecontract.RuntimeModuleFile, Domain: runtimecontract.RuntimeResourceWorkspaceTree,
		SubjectRef: "workspace_deliverable", ExpectedRevision: "revision-v2", ObservedRevision: "revision-v3",
		Reason: "workspace tree revision drifted",
	}
	staleResult, err := runtimecontract.NewRuntimeOperationFeasibilityObservation(command, "", nil, stale, nil, nil)
	if err != nil || staleResult.Outcome != runtimecontract.RuntimeOwnerFeasibilityStale || staleResult.Feedback != nil || staleResult.Stale == nil {
		t.Fatalf("stale observation = %+v err=%v", staleResult, err)
	}

	if _, err := runtimecontract.NewRuntimeOperationFeasibilityObservation(command, runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed, failure, stale, nil, nil); err == nil {
		t.Fatal("stale+failure mixed observation accepted")
	}
	if _, err := runtimecontract.NormalizeRuntimeOperationFeasibilityObservation(runtimecontract.RuntimeOperationFeasibilityObservation{OperationID: command.Operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID}); err == nil {
		t.Fatal("observation without outcome accepted")
	}
	unknown := runtimecontract.RuntimeOperationInfeasibleFailure(command.Operation, "file_unmapped_code", "not currently feasible")
	if _, err := runtimecontract.NewRuntimeOperationFeasibilityObservation(command, "", unknown, nil, nil, nil); err == nil {
		t.Fatal("unclassified owner feasibility result accepted without a typed defect category")
	}
	defect, err := runtimecontract.NewRuntimeOperationFeasibilityObservation(command, runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect, unknown, nil, nil, nil)
	if err != nil || defect.Outcome != runtimecontract.RuntimeOwnerFeasibilityContractDefect || defect.Feedback == nil || defect.FailureCategory() != runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
		t.Fatalf("typed owner contract defect = %+v err=%v", defect, err)
	}
	if defect.Feedback.OriginalDiagnostic == nil || defect.Feedback.OriginalDiagnostic.OriginalCode != unknown.Code || defect.Feedback.OriginalDiagnostic.OriginalMessage != unknown.Message {
		t.Fatalf("typed owner contract defect lost original diagnostic: %+v", defect.Feedback)
	}
	ownerObservation, err := runtimecontract.RuntimeOwnerFeasibilityObservationFromOutcome(defect.Outcome, defect.Feedback, nil, nil)
	if err != nil || ownerObservation.Outcome != runtimecontract.RuntimeOwnerFeasibilityContractDefect || ownerObservation.Feedback == nil || ownerObservation.Feedback.OriginalDiagnostic.OriginalCode != unknown.Code {
		t.Fatalf("typed owner contract defect observation = %+v err=%v", ownerObservation, err)
	}
}
