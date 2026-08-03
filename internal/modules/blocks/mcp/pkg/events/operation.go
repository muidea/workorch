package events

import (
	"fmt"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"strings"
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
				ret.Input = append(ret.Input, argumentFromParameter(name, p))
			} else if name, ok := strings.CutPrefix(p.Name, "arguments."); ok {
				ret.Arguments = append(ret.Arguments, argumentFromParameter(name, p))
			}
		}
	}
	return ret, nil
}

func argumentFromParameter(name string, p runtimecontract.OperationParameter) Argument {
	ret := Argument{Name: strings.TrimSpace(name), StringList: append([]string(nil), p.StringList...)}
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
