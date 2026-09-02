package file

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFileRuntimeCandidateConformanceClassifiesUnifiedDiffWithoutWorkspaceIO(t *testing.T) {
	valid := "--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n"
	validCreate := "--- /dev/null\n+++ b/main.go\n@@ -0,0 +1 @@\n+new\n"
	missingHeaders := "@@ -1 +1 @@\n-old\n+new\n"
	targetMismatch := "--- a/other.go\n+++ b/other.go\n@@ -1 +1 @@\n-old\n+new\n"
	tests := []struct {
		name       string
		diff       string
		policy     runtimecontract.RuntimeOperationTargetPolicy
		outcome    runtimecontract.RuntimeOperationCandidateConformanceOutcome
		actual     runtimecontract.RuntimeOperationCandidateActualShape
		constraint string
	}{
		{name: "existing target update", diff: valid, policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, outcome: runtimecontract.RuntimeOperationCandidateConformant},
		{name: "prospective target create", diff: validCreate, policy: runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget, outcome: runtimecontract.RuntimeOperationCandidateConformant},
		{name: "invalid syntax", diff: missingHeaders, policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, outcome: runtimecontract.RuntimeOperationCandidateNonconformant, actual: runtimecontract.RuntimeOperationCandidateActualShapeInvalidSyntax, constraint: "patch/unified_diff/diff/single_file"},
		{name: "target mismatch", diff: targetMismatch, policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, outcome: runtimecontract.RuntimeOperationCandidateNonconformant, actual: runtimecontract.RuntimeOperationCandidateActualShapeTargetMismatch, constraint: "patch/unified_diff/diff/single_file"},
		{name: "existing authorization rejects add-file diff", diff: validCreate, policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, outcome: runtimecontract.RuntimeOperationCandidateNonconformant, actual: runtimecontract.RuntimeOperationCandidateActualShapeTargetModeMismatch, constraint: "patch/unified_diff/diff/target_mode"},
		{name: "prospective authorization rejects update diff", diff: valid, policy: runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget, outcome: runtimecontract.RuntimeOperationCandidateNonconformant, actual: runtimecontract.RuntimeOperationCandidateActualShapeTargetModeMismatch, constraint: "patch/unified_diff/diff/target_mode"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			patchType := "unified_diff"
			operation := runtimecontract.RuntimeOperation{
				ID: "patch-1", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"main.go"},
				Parameters: []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "diff", String: &testCase.diff}},
			}
			authorization := fileCandidateConformanceAuthorization(t, operation.Kind, testCase.policy, "main.go")
			observation := NewRuntimeAdapter().ProbeRuntimeOperationCandidateConformance(operation, authorization)
			observation.CandidateManifestHash = "manifest-1"
			if observation.Outcome != testCase.outcome {
				t.Fatalf("outcome = %q, want %q: %+v", observation.Outcome, testCase.outcome, observation)
			}
			if testCase.actual == "" {
				if observation.Violation != nil {
					t.Fatalf("conformant observation carries violation: %+v", observation.Violation)
				}
			} else if observation.Violation == nil || observation.Violation.ActualShape != testCase.actual || observation.Violation.ConstraintRef != testCase.constraint {
				t.Fatalf("violation = %+v, want actual=%q", observation.Violation, testCase.actual)
			}
			request := runtimecontract.RuntimeOperationCandidateConformanceRequest{
				WorkspaceID: "ws-1", RunID: "run-1", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
				CandidateManifestHash: "manifest-1", Operation: operation, OperationAuthorization: authorization,
			}
			if err := observation.ValidateForRequest(request, ModuleID); err != nil {
				t.Fatalf("validate observation: %v", err)
			}
		})
	}
}

func fileCandidateConformanceAuthorization(t *testing.T, kind string, policy runtimecontract.RuntimeOperationTargetPolicy, targetRef string) runtimecontract.RuntimeCandidateOperationManifest {
	t.Helper()
	contract, found := runtimecontract.RuntimeOperationContractForKind(kind)
	if !found {
		t.Fatalf("operation contract %q is missing", kind)
	}
	return runtimecontract.RuntimeCandidateOperationManifest{
		Kind: contract.Kind, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		Parameters: contract.Parameters, Variants: contract.Variants,
		Target: func() runtimecontract.RuntimeOperationTargetAuthorization {
			switch policy {
			case runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget:
				return runtimecontract.RuntimeOperationTargetAuthorization{
					Policy: policy, OwnerSelectorRefs: []string{"workspace-relative-new-file-target"}, ProspectiveTargetRefs: []string{targetRef},
				}
			default:
				return runtimecontract.RuntimeOperationTargetAuthorization{
					Policy: policy, OwnerScopeTargetRefs: []string{targetRef},
				}
			}
		}(),
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
	}.Normalized()
}
