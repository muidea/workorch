package events

import (
	"fmt"

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
