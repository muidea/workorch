package events

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func ShellOperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (ShellOperationData, error) {
	operation = operation.Normalized()
	if operation.Kind != runtimecontract.StepKindShellCommand {
		return ShellOperationData{}, fmt.Errorf("operation kind %q is not shell/command", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return ShellOperationData{}, err
	}
	ret := ShellOperationData{TargetRefs: append([]string(nil), operation.TargetRefs...)}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
		case "command":
			ret.Command = *parameter.String
		case "args":
			ret.Args = append([]string(nil), parameter.StringList...)
		case "timeoutMillis":
			ret.TimeoutMillis = *parameter.Integer
		case "stateChanging":
			ret.StateChanging = *parameter.Boolean
		case "targetRef":
			ret.TargetRef = strings.TrimSpace(*parameter.String)
		case "targetRefs":
			ret.TargetRefs = append([]string(nil), parameter.StringList...)
		case "relatedFailureTargets":
			ret.RelatedFailureTargets = append([]string(nil), parameter.StringList...)
		case "verificationScope":
			ret.VerificationScope = *parameter.String
		case "boundedReason":
			ret.BoundedReason = *parameter.String
		case "validationScratchDir":
			ret.ValidationScratchDirectory = *parameter.String
		case "validationScratchAutoCleanup":
			ret.ValidationScratchAutoCleanup = *parameter.Boolean
			ret.ValidationScratchAutoCleanupSet = true
		}
	}
	return ret, nil
}
