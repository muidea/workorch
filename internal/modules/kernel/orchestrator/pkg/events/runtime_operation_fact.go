package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func RuntimeOperationFailure(transaction runtimecontract.AuthorityTransaction, operationKind string, failureCode string, failureMessage string, targetRefs []string) *runtimecontract.FailureFact {
	failureCode = strings.TrimSpace(failureCode)
	if failureCode == "" {
		failureCode = "runtime_operation_failed"
	}
	targetRefs = normalizedRuntimeOperationTargets(targetRefs)
	return &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Code: failureCode, Signature: StableRuntimeOperationFailureSignature(transaction, operationKind, failureCode, targetRefs),
		Message: strings.TrimSpace(failureMessage), TargetRefs: targetRefs,
	}
}

func RuntimeOperationFailureWithTargetObservation(transaction runtimecontract.AuthorityTransaction, operationKind string, failureCode string, failureMessage string, targetRefs []string, observation *runtimecontract.RuntimeFailureTargetObservation) *runtimecontract.FailureFact {
	failure := RuntimeOperationFailure(transaction, operationKind, failureCode, failureMessage, targetRefs)
	if observation == nil {
		return failure
	}
	value := observation.Normalized()
	if err := value.Validate(); err != nil || !sameRuntimeOperationTargets(failure.TargetRefs, value.TargetRefs) {
		return failure
	}
	failure.TargetObservation = &value
	return failure
}

func StableRuntimeOperationFailureSignature(transaction runtimecontract.AuthorityTransaction, operationKind string, failureCode string, targetRefs []string) string {
	targetRefs = normalizedRuntimeOperationTargets(targetRefs)
	h := sha256.New()
	for _, value := range []string{strings.TrimSpace(operationKind), strings.TrimSpace(failureCode), strings.TrimSpace(transaction.RequiredOutputHash), strings.Join(targetRefs, "\x00")} {
		_, _ = fmt.Fprintf(h, "%d:%s|", len(value), value)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func RuntimeOperationEventID(dispatchID string, operationID string) string {
	return "runtime-operation|" + strings.TrimSpace(dispatchID) + "|" + strings.TrimSpace(operationID)
}

func RuntimeOperationSourceRef(dispatchID string, operationID string) string {
	return "workorch://runtime-operations/" + strings.TrimSpace(dispatchID) + "/" + strings.TrimSpace(operationID)
}

func ValidateRuntimeOperationScope(workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, transaction runtimecontract.AuthorityTransaction, operationID string, evidenceKind string, workspaceDirectory string) error {
	if workspaceID == "" || runID == "" || strings.TrimSpace(dispatchID) == "" || strings.TrimSpace(transaction.AuthoritySnapshotID) == "" ||
		strings.TrimSpace(operationID) == "" || strings.TrimSpace(evidenceKind) == "" || strings.TrimSpace(workspaceDirectory) == "" {
		return fmt.Errorf("invalid runtime operation command")
	}
	return nil
}

func normalizedRuntimeOperationTargets(values []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
}

func sameRuntimeOperationTargets(left, right []string) bool {
	left = normalizedRuntimeOperationTargets(left)
	right = normalizedRuntimeOperationTargets(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}
