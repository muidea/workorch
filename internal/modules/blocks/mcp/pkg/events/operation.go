package events

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func OperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (OperationData, error) {
	operation = operation.Normalized()
	switch operation.Kind {
	case runtimecontract.StepKindMCPToolCall, runtimecontract.StepKindMCPResourceRead, runtimecontract.StepKindMCPPromptGet:
	default:
		return OperationData{}, fmt.Errorf("operation kind %q is not an mcp operation", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return OperationData{}, err
	}
	ret := OperationData{}
	for _, p := range operation.Parameters {
		switch p.Name {
		case "serverId":
			ret.ServerID = *p.String
		case "tool":
			ret.Tool = *p.String
		case "uri", "resourceUri":
			ret.URI = *p.String
		case "prompt", "name":
			ret.Prompt = *p.String
		case "summary":
			ret.Summary = *p.String
		case "stream":
			ret.Stream = *p.Boolean
		default:
			if name, ok := strings.CutPrefix(p.Name, "input."); ok {
				ret.Input = append(ret.Input, operationParameterWithName(name, p))
			} else if name, ok := strings.CutPrefix(p.Name, "arguments."); ok {
				ret.Arguments = append(ret.Arguments, operationParameterWithName(name, p))
			}
		}
	}
	return ret, nil
}

func operationParameterWithName(name string, p runtimecontract.OperationParameter) runtimecontract.OperationParameter {
	ret := runtimecontract.OperationParameter{Name: strings.TrimSpace(name), StringList: append([]string(nil), p.StringList...)}
	if p.String != nil {
		value := *p.String
		ret.String = &value
	}
	if p.Integer != nil {
		value := *p.Integer
		ret.Integer = &value
	}
	if p.Boolean != nil {
		value := *p.Boolean
		ret.Boolean = &value
	}
	return ret
}
