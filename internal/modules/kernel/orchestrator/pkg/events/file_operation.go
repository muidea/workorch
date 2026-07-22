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

func FileListOperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (FileListOperationData, error) {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.StepKindFileList {
		return FileListOperationData{}, fmt.Errorf("operation kind %q is not file/list", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return FileListOperationData{}, err
	}
	ret := FileListOperationData{Path: "."}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
		case "path":
			ret.Path = *parameter.String
		case "recursive":
			ret.Recursive = *parameter.Boolean
		case "maxEntries":
			ret.MaxEntries = *parameter.Integer
		}
	}
	return ret, nil
}

func FileReadOperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (FileReadOperationData, error) {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.StepKindFileRead {
		return FileReadOperationData{}, fmt.Errorf("operation kind %q is not file/read", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return FileReadOperationData{}, err
	}
	ret := FileReadOperationData{}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
		case "path":
			ret.Path = *parameter.String
		case "optional":
			ret.Optional = *parameter.Boolean
		case "maxBytes":
			ret.MaxBytes = *parameter.Integer
		case "startLine":
			ret.StartLine = *parameter.Integer
		case "endLine":
			ret.EndLine = *parameter.Integer
		}
	}
	return ret, nil
}

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

func FileWriteOperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (FileWriteOperationData, error) {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.StepKindFileWrite {
		return FileWriteOperationData{}, fmt.Errorf("operation kind %q is not file/write", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return FileWriteOperationData{}, err
	}
	ret := FileWriteOperationData{}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
		case "path", "targetRef":
			if ret.TargetRef == "" {
				ret.TargetRef = *parameter.String
			}
		case "content":
			ret.Content = *parameter.String
			ret.ContentPresent = true
		}
	}
	if ret.TargetRef == "" && len(operation.TargetRefs) == 1 {
		ret.TargetRef = operation.TargetRefs[0]
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

func PatchPreviewOperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (PatchApplyOperationData, error) {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.RuntimeBlockKindPatchPreview {
		return PatchApplyOperationData{}, fmt.Errorf("operation kind %q is not patch/preview", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return PatchApplyOperationData{}, err
	}
	return patchOperationData(operation), nil
}

func patchOperationData(operation runtimecontract.RuntimeOperation) PatchApplyOperationData {
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
	return ret
}

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
