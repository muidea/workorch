package events

import (
	"fmt"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func SSHOperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (SSHOperationData, error) {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.StepKindSSHCommand {
		return SSHOperationData{}, fmt.Errorf("operation kind %q is not ssh/command", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return SSHOperationData{}, err
	}
	ret := SSHOperationData{}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
		case "host":
			ret.Host = *parameter.String
		case "port":
			ret.Port = *parameter.Integer
		case "username":
			ret.Username = *parameter.String
		case "password":
			ret.Password = *parameter.String
		case "privateKey":
			ret.PrivateKey = *parameter.String
		case "privateKeyPath":
			ret.PrivateKeyPath = *parameter.String
		case "command":
			ret.Command = *parameter.String
		case "timeoutMillis":
			ret.TimeoutMillis = *parameter.Integer
		}
	}
	return ret, nil
}
