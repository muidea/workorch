package file

import (
	"os"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const fileOwnerFeasibilityPolicyRevision = "file-owner-feasibility/create-only-write"

type fileOwnerFeasibilityPolicyInput struct {
	OperationKind string
	VariantID     string
	TargetPolicy  runtimecontract.RuntimeOperationTargetPolicy
	TargetRef     string
	TargetPath    string
}

type fileOwnerFeasibilityPolicyResult struct {
	Code       string
	Message    string
	TargetRefs []string
}

func (r fileOwnerFeasibilityPolicyResult) Feasible() bool { return strings.TrimSpace(r.Code) == "" }

// evaluateFileOwnerFeasibilityPolicy is the shared target policy for semantic
// and operation feasibility. It is side-effect free and returns only a File
// owner rule fact; callers retain their own typed wire result.
func evaluateFileOwnerFeasibilityPolicy(input fileOwnerFeasibilityPolicyInput) fileOwnerFeasibilityPolicyResult {
	input.OperationKind = runtimecontract.CanonicalRuntimeBlockKind(input.OperationKind)
	input.VariantID = strings.TrimSpace(input.VariantID)
	input.TargetRef = strings.TrimSpace(input.TargetRef)
	refs := []string{input.TargetRef}
	if input.TargetRef == "" || strings.TrimSpace(input.TargetPath) == "" {
		return fileOwnerFeasibilityPolicyResult{Code: "operation_target_missing", Message: "runtime file operation requires a concrete workspace-relative target"}
	}
	info, err := os.Lstat(input.TargetPath)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return fileOwnerFeasibilityPolicyResult{Code: "operation_target_unreadable", Message: err.Error(), TargetRefs: refs}
	}
	if exists && info.Mode()&os.ModeSymlink != 0 {
		return fileOwnerFeasibilityPolicyResult{Code: "operation_target_not_regular", Message: "runtime file operation target is a symbolic link", TargetRefs: refs}
	}
	if exists && !info.Mode().IsRegular() {
		return fileOwnerFeasibilityPolicyResult{Code: "operation_target_not_regular", Message: "runtime file operation target is not a regular file", TargetRefs: refs}
	}
	if input.TargetPolicy == runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget && exists {
		return fileOwnerFeasibilityPolicyResult{Code: "operation_target_exists", Message: "runtime prospective File target already exists", TargetRefs: refs}
	}
	if input.OperationKind == runtimecontract.StepKindFileWrite && exists {
		return fileOwnerFeasibilityPolicyResult{Code: "operation_target_exists", Message: "file/write is create-only; an existing target requires a revision-bound replacement operation", TargetRefs: refs}
	}
	if !exists && input.TargetPolicy != runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget {
		return fileOwnerFeasibilityPolicyResult{Code: "operation_target_missing", Message: "runtime file operation requires an existing regular file", TargetRefs: refs}
	}
	return fileOwnerFeasibilityPolicyResult{}
}
