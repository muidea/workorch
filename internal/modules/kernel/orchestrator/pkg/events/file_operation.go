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

func FileEditOperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (FileEditOperationData, error) {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.StepKindFileEdit {
		return FileEditOperationData{}, fmt.Errorf("operation kind %q is not file/edit", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return FileEditOperationData{}, err
	}
	ret := FileEditOperationData{}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
		case "path":
			ret.Path = *parameter.String
		case "oldText":
			ret.OldText = *parameter.String
		case "newText":
			ret.NewText = *parameter.String
		case "baseHash":
			ret.BaseHash = *parameter.String
		case "occurrence":
			ret.Occurrence = *parameter.Integer
		}
	}
	return ret, nil
}

func PatchApplyOperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (PatchApplyOperationData, error) {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.RuntimeBlockKindPatchApply {
		return PatchApplyOperationData{}, fmt.Errorf("operation kind %q is not patch/apply", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return PatchApplyOperationData{}, err
	}
	ret := PatchApplyOperationData{}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
		case "targetRef":
			ret.TargetRef = *parameter.String
		case "path":
			if ret.TargetRef == "" {
				ret.TargetRef = *parameter.String
			}
		case "patchType":
			ret.PatchType = *parameter.String
		case "content":
			ret.Content = *parameter.String
			ret.ContentPresent = true
		case "diff":
			ret.Diff = *parameter.String
		case "oldText":
			ret.OldText = *parameter.String
		case "newText":
			ret.NewText = *parameter.String
			ret.NewTextPresent = true
		case "baseHash":
			ret.BaseHash = *parameter.String
		case "occurrence":
			ret.Occurrence = *parameter.Integer
		case "coverageTargets":
			ret.CoverageTargets = append([]string(nil), parameter.StringList...)
		case "summary":
			ret.Summary = *parameter.String
		}
	}
	return ret, nil
}

func RuntimeFileOperationFailure(transaction runtimecontract.AuthorityTransaction, operationKind string, failureCode string, failureMessage string, targetRefs []string) *runtimecontract.FailureFact {
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

func StableRuntimeOperationFailureSignature(transaction runtimecontract.AuthorityTransaction, operationKind string, failureCode string, targetRefs []string) string {
	targetRefs = normalizedRuntimeOperationTargets(targetRefs)
	h := sha256.New()
	for _, value := range []string{strings.TrimSpace(operationKind), strings.TrimSpace(failureCode), strings.TrimSpace(transaction.RequiredOutputHash), strings.Join(targetRefs, "\x00")} {
		_, _ = fmt.Fprintf(h, "%d:%s|", len(value), value)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func RuntimeFileOperationEventID(dispatchID string, operationID string) string {
	return "runtime-operation|" + strings.TrimSpace(dispatchID) + "|" + strings.TrimSpace(operationID)
}

func RuntimeFileOperationSourceRef(dispatchID string, operationID string) string {
	return "workorch://runtime-operations/" + strings.TrimSpace(dispatchID) + "/" + strings.TrimSpace(operationID)
}

func ValidateFileOperationScope(workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, transaction runtimecontract.AuthorityTransaction, operationID string, evidenceKind string, workspaceDirectory string) error {
	if workspaceID == "" || runID == "" || strings.TrimSpace(dispatchID) == "" || strings.TrimSpace(transaction.AuthoritySnapshotID) == "" ||
		strings.TrimSpace(operationID) == "" || strings.TrimSpace(evidenceKind) == "" || strings.TrimSpace(workspaceDirectory) == "" {
		return fmt.Errorf("invalid runtime file operation command")
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
