package events

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// SealedRuntimeOperation is the immutable Git execution payload copied from
// the Orchestrator's persisted dispatch. Git derives OperationData only after
// validating the full payload hash and execution scope, so caller-controlled
// fields cannot drift from the sealed operation.
type SealedRuntimeOperation struct {
	Operation   runtimecontract.RuntimeOperation
	PayloadHash string
}

func SealRuntimeOperation(operation runtimecontract.RuntimeOperation) (SealedRuntimeOperation, error) {
	operation = operation.Normalized()
	if runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleGit {
		return SealedRuntimeOperation{}, fmt.Errorf("runtime operation kind %q is not owned by Git", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return SealedRuntimeOperation{}, err
	}
	payload, err := json.Marshal(operation)
	if err != nil {
		return SealedRuntimeOperation{}, fmt.Errorf("marshal sealed Git operation: %w", err)
	}
	var detached runtimecontract.RuntimeOperation
	if err := json.Unmarshal(payload, &detached); err != nil {
		return SealedRuntimeOperation{}, fmt.Errorf("clone sealed Git operation: %w", err)
	}
	sum := sha256.Sum256(payload)
	return SealedRuntimeOperation{Operation: detached, PayloadHash: hex.EncodeToString(sum[:])}, nil
}

// ValidateForExecution returns a detached operation only when the payload and
// owner-neutral execution context exactly match the Orchestrator seal.
func (s SealedRuntimeOperation) ValidateForExecution(context runtimecontract.RuntimeOperationExecutionContext) (runtimecontract.RuntimeOperation, error) {
	operation := s.Operation.Normalized()
	if runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleGit {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("sealed runtime operation is not owned by Git")
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return runtimecontract.RuntimeOperation{}, err
	}
	if operation.ID != strings.TrimSpace(context.OperationID) {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("sealed Git operation identity mismatch")
	}
	if !sameExecutionStrings(operation.TargetRefs, context.TargetRefs) || !sameExecutionCoverage(operation.CoverageWorkItemIDs, context.CoverageWorkItemIDs) {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("sealed Git operation scope mismatch")
	}
	payload, err := json.Marshal(operation)
	if err != nil {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("marshal received Git operation: %w", err)
	}
	sum := sha256.Sum256(payload)
	expectedHash := hex.EncodeToString(sum[:])
	actualHash := strings.TrimSpace(s.PayloadHash)
	if len(actualHash) != len(expectedHash) || subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) != 1 {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("sealed Git operation payload hash mismatch")
	}
	sealed, err := SealRuntimeOperation(operation)
	if err != nil {
		return runtimecontract.RuntimeOperation{}, err
	}
	return sealed.Operation, nil
}

func sameExecutionStrings(left, right []string) bool {
	normalize := func(values []string) []string {
		seen := map[string]struct{}{}
		ret := make([]string, 0, len(values))
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, found := seen[value]; found {
				continue
			}
			seen[value] = struct{}{}
			ret = append(ret, value)
		}
		sort.Strings(ret)
		return ret
	}
	left = normalize(left)
	right = normalize(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameExecutionCoverage(left, right []common.CoverageWorkItemID) bool {
	leftValues := make([]string, 0, len(left))
	for _, value := range left {
		leftValues = append(leftValues, string(value))
	}
	rightValues := make([]string, 0, len(right))
	for _, value := range right {
		rightValues = append(rightValues, string(value))
	}
	return sameExecutionStrings(leftValues, rightValues)
}

func OperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (OperationData, error) {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.StepKindGitRead && operation.Kind != runtimecontract.StepKindGitWrite {
		return OperationData{}, fmt.Errorf("operation kind %q is not a git operation", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return OperationData{}, err
	}
	ret := OperationData{}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
		case "operation":
			ret.Operation = *parameter.String
		case "timeoutMillis":
			ret.TimeoutMillis = *parameter.Integer
		case "pathspecs":
			ret.Pathspecs = append([]string(nil), parameter.StringList...)
		case "ref":
			ret.Ref = *parameter.String
		case "baseRef":
			ret.BaseRef = *parameter.String
		case "limit":
			ret.Limit = *parameter.Integer
		case "message":
			ret.Message = *parameter.String
		case "expectedHead":
			ret.ExpectedHead = *parameter.String
		}
	}
	return ret, nil
}
