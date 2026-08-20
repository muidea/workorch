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

// SealedRuntimeOperation is the immutable File execution payload copied from
// the Orchestrator's persisted dispatch. File derives its concrete adapter data
// from Operation only after validating PayloadHash and the execution context;
// there is no second caller-controlled Data value that can drift from the seal.
type SealedRuntimeOperation struct {
	Operation   runtimecontract.RuntimeOperation
	PayloadHash string
}

func SealRuntimeOperation(operation runtimecontract.RuntimeOperation) (SealedRuntimeOperation, error) {
	operation = operation.Normalized()
	if runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleFile {
		return SealedRuntimeOperation{}, fmt.Errorf("runtime operation kind %q is not owned by File", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return SealedRuntimeOperation{}, err
	}
	payload, err := json.Marshal(operation)
	if err != nil {
		return SealedRuntimeOperation{}, fmt.Errorf("marshal sealed File operation: %w", err)
	}
	// Detach pointer and slice fields before the value crosses EventHub.
	var detached runtimecontract.RuntimeOperation
	if err := json.Unmarshal(payload, &detached); err != nil {
		return SealedRuntimeOperation{}, fmt.Errorf("clone sealed File operation: %w", err)
	}
	sum := sha256.Sum256(payload)
	return SealedRuntimeOperation{Operation: detached, PayloadHash: hex.EncodeToString(sum[:])}, nil
}

// ValidateForExecution proves that the received File payload is the exact
// operation sealed for this command identity. The returned operation is a
// detached normalized copy suitable for owner-local decoding.
func (s SealedRuntimeOperation) ValidateForExecution(context runtimecontract.RuntimeOperationExecutionContext, expectedKind string) (runtimecontract.RuntimeOperation, error) {
	operation := s.Operation.Normalized()
	if operation.Kind != strings.TrimSpace(expectedKind) {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("sealed File operation kind %q does not match %q", operation.Kind, expectedKind)
	}
	if runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleFile {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("sealed runtime operation is not owned by File")
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return runtimecontract.RuntimeOperation{}, err
	}
	if operation.ID != strings.TrimSpace(context.OperationID) {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("sealed File operation identity mismatch")
	}
	if !sameExecutionStrings(operation.TargetRefs, context.TargetRefs) || !sameExecutionCoverage(operation.CoverageWorkItemIDs, context.CoverageWorkItemIDs) {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("sealed File operation scope mismatch")
	}
	payload, err := json.Marshal(operation)
	if err != nil {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("marshal received File operation: %w", err)
	}
	sum := sha256.Sum256(payload)
	expectedHash := hex.EncodeToString(sum[:])
	actualHash := strings.TrimSpace(s.PayloadHash)
	if len(actualHash) != len(expectedHash) || subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) != 1 {
		return runtimecontract.RuntimeOperation{}, fmt.Errorf("sealed File operation payload hash mismatch")
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
	ret := FileReadOperationData{Path: operation.TargetRefs[0]}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
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
	ret := FileEditOperationData{Path: operation.TargetRefs[0]}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
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
	ret := FileWriteOperationData{TargetRef: operation.TargetRefs[0]}
	for _, parameter := range operation.Parameters {
		if parameter.Name == "content" {
			ret.Content = *parameter.String
			ret.ContentPresent = true
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
	return patchOperationData(operation), nil
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
	ret := PatchApplyOperationData{TargetRef: operation.TargetRefs[0]}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
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
