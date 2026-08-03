package events

import (
	"fmt"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func OperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (OperationData, error) {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.StepKindSSHCommand {
		return OperationData{}, fmt.Errorf("operation kind %q is not ssh/command", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return OperationData{}, err
	}
	ret := OperationData{}
	for _, p := range operation.Parameters {
		switch p.Name {
		case "host":
			ret.Host = *p.String
		case "port":
			ret.Port = *p.Integer
		case "username":
			ret.Username = *p.String
		case "password":
			ret.Password = *p.String
		case "privateKey":
			ret.PrivateKey = *p.String
		case "privateKeyPath":
			ret.PrivateKeyPath = *p.String
		case "command":
			ret.Command = *p.String
		case "timeoutMillis":
			ret.TimeoutMillis = *p.Integer
		}
	}
	return ret, nil
}
