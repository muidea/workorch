package events

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func OperationDataFromRuntimeOperation(operation runtimecontract.RuntimeOperation) (OperationData, error) {
	operation = operation.Normalized()
	switch operation.Kind {
	case runtimecontract.StepKindShellCommand, runtimecontract.RuntimeBlockKindManagedValidationSession:
	default:
		return OperationData{}, fmt.Errorf("operation kind %q is not a shell-owned runtime operation", operation.Kind)
	}
	if err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil {
		return OperationData{}, err
	}
	ret := OperationData{TargetRefs: append([]string(nil), operation.TargetRefs...)}
	if operation.Kind == runtimecontract.RuntimeBlockKindManagedValidationSession {
		ret.ManagedValidation = runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			OriginalNodeKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			CleanupPolicy:    "process_group_on_timeout_or_completion", AutoCleanup: true, AutoCleanupSet: true,
		}
	}
	for _, parameter := range operation.Parameters {
		switch parameter.Name {
		case "recipeId":
			if parameter.String != nil {
				ret.ValidationRecipeID = strings.TrimSpace(*parameter.String)
			}
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
	if operation.Kind == runtimecontract.RuntimeBlockKindManagedValidationSession {
		if ret.ValidationRecipeID == "" {
			return OperationData{}, fmt.Errorf("managed validation operation requires recipeId")
		}
		if ret.ValidationScratchAutoCleanupSet {
			ret.ManagedValidation.AutoCleanup = ret.ValidationScratchAutoCleanup
			ret.ManagedValidation.AutoCleanupSet = true
		}
	}
	return ret, nil
}
