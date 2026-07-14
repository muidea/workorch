package workorchruntimecontract

import (
	"fmt"
	"strings"
)

type RuntimeOperationParameterType string

const (
	RuntimeOperationParameterTypeString     RuntimeOperationParameterType = "string"
	RuntimeOperationParameterTypeInteger    RuntimeOperationParameterType = "integer"
	RuntimeOperationParameterTypeBoolean    RuntimeOperationParameterType = "boolean"
	RuntimeOperationParameterTypeStringList RuntimeOperationParameterType = "string_list"
)

type RuntimeOperationParameterContract struct {
	Name        string
	Type        RuntimeOperationParameterType
	Required    bool
	Description string
}

type RuntimeOperationContract struct {
	Kind                string
	Parameters          []RuntimeOperationParameterContract
	ForbiddenParameters []string
	Rules               []string
}

func RuntimeOperationContractForKind(kind string) (RuntimeOperationContract, bool) {
	switch CanonicalRuntimeBlockKind(kind) {
	case StepKindFileEdit:
		return RuntimeOperationContract{
			Kind: StepKindFileEdit,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "path", Type: RuntimeOperationParameterTypeString, Required: true, Description: "workspace-relative file path"},
				{Name: "oldText", Type: RuntimeOperationParameterTypeString, Required: true, Description: "exact current text to replace"},
				{Name: "newText", Type: RuntimeOperationParameterTypeString, Required: true, Description: "replacement text; an empty string is allowed"},
				{Name: "baseHash", Type: RuntimeOperationParameterTypeString, Description: "optional current content hash"},
				{Name: "occurrence", Type: RuntimeOperationParameterTypeInteger, Description: "1-based match index when oldText is not unique"},
			},
			ForbiddenParameters: []string{"filePath", "fileContent", "targetRef", "targetRefs"},
			Rules: []string{
				"oldText must be non-empty and different from newText",
				"targetRefs must contain the same target as path",
			},
		}, true
	case RuntimeBlockKindPatchApply:
		return RuntimeOperationContract{
			Kind: RuntimeBlockKindPatchApply,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "targetRef", Type: RuntimeOperationParameterTypeString, Description: "preferred workspace-relative target path"},
				{Name: "path", Type: RuntimeOperationParameterTypeString, Description: "workspace-relative target path alias"},
				{Name: "patchType", Type: RuntimeOperationParameterTypeString, Required: true, Description: "file_write, text_replace, or unified_diff"},
				{Name: "content", Type: RuntimeOperationParameterTypeString, Description: "complete file_write content; an empty string is allowed"},
				{Name: "diff", Type: RuntimeOperationParameterTypeString, Description: "unified_diff content"},
				{Name: "oldText", Type: RuntimeOperationParameterTypeString, Description: "exact text_replace source text"},
				{Name: "newText", Type: RuntimeOperationParameterTypeString, Description: "text_replace replacement; an empty string is allowed"},
				{Name: "baseHash", Type: RuntimeOperationParameterTypeString, Description: "optional current content hash"},
				{Name: "occurrence", Type: RuntimeOperationParameterTypeInteger, Description: "1-based match index when oldText is not unique"},
				{Name: "coverageTargets", Type: RuntimeOperationParameterTypeStringList, Description: "coverage targets addressed by this state change"},
				{Name: "summary", Type: RuntimeOperationParameterTypeString, Description: "bounded change summary"},
			},
			ForbiddenParameters: []string{"filePath", "fileContent", "targetRefs"},
			Rules: []string{
				"file_write must provide content",
				"text_replace must provide non-empty oldText and a distinct newText",
				"unified_diff must provide non-empty diff",
				"exactly one canonical target must be resolved from targetRef or path",
				"targetRefs must contain the resolved target",
			},
		}, true
	default:
		return RuntimeOperationContract{}, false
	}
}

func ValidateRuntimeOperationContract(operation RuntimeOperation) error {
	operation = operation.Normalized()
	contract, known := RuntimeOperationContractForKind(operation.Kind)
	if !known {
		return nil
	}
	allowed := make(map[string]RuntimeOperationParameterContract, len(contract.Parameters))
	for _, parameter := range contract.Parameters {
		allowed[parameter.Name] = parameter
	}
	values := make(map[string]OperationParameter, len(operation.Parameters))
	for _, parameter := range operation.Parameters {
		definition, ok := allowed[parameter.Name]
		if !ok {
			return fmt.Errorf("kind %q does not accept parameter %q", operation.Kind, parameter.Name)
		}
		if !operationParameterMatchesType(parameter, definition.Type) {
			return fmt.Errorf("kind %q parameter %q requires %s", operation.Kind, parameter.Name, definition.Type)
		}
		values[parameter.Name] = parameter
	}
	for _, parameter := range contract.Parameters {
		if parameter.Required {
			if _, ok := values[parameter.Name]; !ok {
				return fmt.Errorf("kind %q requires parameter %q", operation.Kind, parameter.Name)
			}
		}
	}
	switch operation.Kind {
	case StepKindFileEdit:
		path := parameterString(values["path"])
		oldText := parameterRawString(values["oldText"])
		newText := parameterRawString(values["newText"])
		if path == "" || oldText == "" || oldText == newText {
			return fmt.Errorf("kind %q requires non-empty path and oldText with a distinct newText", operation.Kind)
		}
		if occurrence, ok := parameterInteger(values["occurrence"]); ok && occurrence <= 0 {
			return fmt.Errorf("kind %q occurrence must be positive", operation.Kind)
		}
		if !containsString(operation.TargetRefs, path) {
			return fmt.Errorf("kind %q targetRefs must contain path %q", operation.Kind, path)
		}
	case RuntimeBlockKindPatchApply:
		targetRef := parameterString(values["targetRef"])
		path := parameterString(values["path"])
		if targetRef == "" {
			targetRef = path
		} else if path != "" && path != targetRef {
			return fmt.Errorf("kind %q targetRef and path must identify the same target", operation.Kind)
		}
		patchType := parameterString(values["patchType"])
		if targetRef == "" || !containsString(operation.TargetRefs, targetRef) {
			return fmt.Errorf("kind %q targetRefs must contain targetRef %q", operation.Kind, targetRef)
		}
		switch patchType {
		case "file_write":
			if _, ok := values["content"]; !ok {
				return fmt.Errorf("kind %q file_write requires content", operation.Kind)
			}
		case "text_replace":
			oldText := parameterRawString(values["oldText"])
			newText, newTextPresent := values["newText"]
			if oldText == "" || !newTextPresent || oldText == parameterRawString(newText) {
				return fmt.Errorf("kind %q text_replace requires non-empty oldText and a distinct newText", operation.Kind)
			}
		case "unified_diff":
			if parameterRawString(values["diff"]) == "" {
				return fmt.Errorf("kind %q unified_diff requires diff", operation.Kind)
			}
		default:
			return fmt.Errorf("kind %q patchType %q is unsupported", operation.Kind, patchType)
		}
		if occurrence, ok := parameterInteger(values["occurrence"]); ok && occurrence <= 0 {
			return fmt.Errorf("kind %q occurrence must be positive", operation.Kind)
		}
	}
	return nil
}

func operationParameterMatchesType(parameter OperationParameter, expected RuntimeOperationParameterType) bool {
	switch expected {
	case RuntimeOperationParameterTypeString:
		return parameter.String != nil && parameter.Integer == nil && parameter.Boolean == nil && len(parameter.StringList) == 0
	case RuntimeOperationParameterTypeInteger:
		return parameter.String == nil && parameter.Integer != nil && parameter.Boolean == nil && len(parameter.StringList) == 0
	case RuntimeOperationParameterTypeBoolean:
		return parameter.String == nil && parameter.Integer == nil && parameter.Boolean != nil && len(parameter.StringList) == 0
	case RuntimeOperationParameterTypeStringList:
		return parameter.String == nil && parameter.Integer == nil && parameter.Boolean == nil && len(parameter.StringList) > 0
	default:
		return false
	}
}

func parameterString(parameter OperationParameter) string {
	if parameter.String == nil {
		return ""
	}
	return strings.TrimSpace(*parameter.String)
}

func parameterRawString(parameter OperationParameter) string {
	if parameter.String == nil {
		return ""
	}
	return *parameter.String
}

func parameterInteger(parameter OperationParameter) (int64, bool) {
	if parameter.Integer == nil {
		return 0, false
	}
	return *parameter.Integer, true
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
