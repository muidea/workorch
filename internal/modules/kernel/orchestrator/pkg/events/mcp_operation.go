package events

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func MCPOperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (MCPOperationData, error) {
	operation = operation.Normalized()
	switch operation.Kind {
	case runtimecontract.StepKindMCPToolCall, runtimecontract.StepKindMCPResourceRead, runtimecontract.StepKindMCPPromptGet:
	default:
		return MCPOperationData{}, fmt.Errorf("operation kind %q is not an mcp operation", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return MCPOperationData{}, err
	}
	ret := MCPOperationData{}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
		case "serverId":
			ret.ServerID = *parameter.String
		case "tool":
			ret.Tool = *parameter.String
		case "uri", "resourceUri":
			ret.URI = *parameter.String
		case "prompt", "name":
			ret.Prompt = *parameter.String
		case "summary":
			ret.Summary = *parameter.String
		case "stream":
			ret.Stream = *parameter.Boolean
		default:
			if name, ok := strings.CutPrefix(parameter.Name, "input."); ok {
				ret.Input = append(ret.Input, mcpArgumentFromParameter(name, parameter))
			} else if name, ok := strings.CutPrefix(parameter.Name, "arguments."); ok {
				ret.Arguments = append(ret.Arguments, mcpArgumentFromParameter(name, parameter))
			}
		}
	}
	return ret, nil
}

func mcpArgumentFromParameter(name string, parameter runtimecontract.OperationParameter) MCPArgument {
	ret := MCPArgument{Name: strings.TrimSpace(name), StringList: append([]string(nil), parameter.StringList...)}
	if parameter.String != nil {
		value := *parameter.String
		ret.String = &value
	}
	if parameter.Integer != nil {
		value := *parameter.Integer
		ret.Integer = &value
	}
	if parameter.Boolean != nil {
		value := *parameter.Boolean
		ret.Boolean = &value
	}
	return ret
}
