package events

import (
	"fmt"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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
