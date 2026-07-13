package events

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeOperationAdapter interface {
	ExecuteRuntimeStep(context.Context, runtimecontract.StepExecutionRequest) (runtimecontract.StepExecutionResult, error)
}

func ExecuteRuntimeOperation(ctx context.Context, command ExecuteRuntimeOperationCommand, adapter RuntimeOperationAdapter) (ExecuteRuntimeOperationResult, error) {
	if adapter == nil || command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.DispatchID) == "" ||
		strings.TrimSpace(command.Transaction.AuthoritySnapshotID) == "" || strings.TrimSpace(command.Operation.ID) == "" || strings.TrimSpace(command.Operation.Kind) == "" ||
		strings.TrimSpace(command.EvidenceKind) == "" {
		return ExecuteRuntimeOperationResult{}, fmt.Errorf("invalid runtime operation command")
	}
	operation := command.Operation.Normalized()
	input, err := runtimeOperationInput(operation)
	if err != nil {
		return ExecuteRuntimeOperationResult{}, err
	}
	result, executeErr := adapter.ExecuteRuntimeStep(ctx, runtimecontract.StepExecutionRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		StepID: commandOperationStepID(command.DispatchID, operation.ID), Kind: operation.Kind,
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		WorkspaceInternalDirectory:    command.WorkspaceInternalDirectory,
		Input:                         input,
		EvidencePolicies:              command.EvidencePolicies,
	})
	eventID := "runtime-operation|" + command.DispatchID + "|" + operation.ID
	response := ExecuteRuntimeOperationResult{
		EventID: eventID, OperationID: operation.ID, EvidenceKind: command.EvidenceKind,
		Status:    runtimecontract.RuntimeOperationExecutionStatusAccepted,
		SourceRef: "workorch://runtime-operations/" + command.DispatchID + "/" + operation.ID,
		Evidence:  runtimeOperationEvidence(result.Output),
	}
	if executeErr != nil {
		response.Status = runtimecontract.RuntimeOperationExecutionStatusRejected
		failureClass := runtimecontract.FailureClassExecutionFailure
		failureSignature := eventID
		failureMessage := executeErr.Error()
		if value, ok := result.Output["failureClass"].(string); ok && strings.TrimSpace(value) != "" {
			failureClass = runtimecontract.FailureClass(strings.TrimSpace(value))
		}
		if value, ok := result.Output["failureSignature"].(string); ok && strings.TrimSpace(value) != "" {
			failureSignature = strings.TrimSpace(value)
		}
		if value, ok := result.Output["error"].(string); ok && strings.TrimSpace(value) != "" {
			failureMessage = strings.TrimSpace(value)
		}
		response.Failure = &runtimecontract.FailureFact{
			Class: failureClass, Signature: failureSignature,
			Message: failureMessage, TargetRefs: operation.TargetRefs,
		}
	}
	return response, nil
}

func runtimeOperationInput(operation runtimecontract.RuntimeOperation) (map[string]any, error) {
	ret := make(map[string]any, len(operation.Parameters)+3)
	for _, parameter := range operation.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if name == "" {
			return nil, fmt.Errorf("runtime operation parameter requires a name")
		}
		switch {
		case parameter.String != nil:
			ret[name] = *parameter.String
		case parameter.Integer != nil:
			ret[name] = *parameter.Integer
		case parameter.Boolean != nil:
			ret[name] = *parameter.Boolean
		case len(parameter.StringList) > 0:
			ret[name] = append([]string(nil), parameter.StringList...)
		default:
			return nil, fmt.Errorf("runtime operation parameter %q has no concrete value", name)
		}
	}
	ret["operationId"] = operation.ID
	coverage := make([]string, 0, len(operation.CoverageWorkItemIDs))
	for _, id := range operation.CoverageWorkItemIDs {
		coverage = append(coverage, string(id))
	}
	ret["coverageWorkItemIds"] = coverage
	ret["targetRefs"] = append([]string(nil), operation.TargetRefs...)
	return ret, nil
}

func commandOperationStepID(dispatchID string, operationID string) common.StepID {
	return common.StepID(strings.TrimSpace(dispatchID) + "/" + strings.TrimSpace(operationID))
}

func runtimeOperationEvidence(output map[string]any) runtimecontract.RuntimeOperationEvidence {
	ret := runtimecontract.RuntimeOperationEvidence{}
	if len(output) == 0 {
		return ret
	}
	for _, key := range []string{"summary", "message", "status"} {
		if value, ok := output[key].(string); ok && strings.TrimSpace(value) != "" {
			ret.Summary = strings.TrimSpace(value)
			break
		}
	}
	if value, ok := output["contentHash"].(string); ok {
		ret.ContentHash = strings.TrimSpace(value)
	}
	switch value := output["exitCode"].(type) {
	case int:
		converted := int64(value)
		ret.ExitCode = &converted
	case int64:
		converted := value
		ret.ExitCode = &converted
	case float64:
		converted := int64(value)
		ret.ExitCode = &converted
	}
	if values, ok := output["evidenceRefs"].([]string); ok {
		ret.EvidenceRefs = append([]string(nil), values...)
	}
	return ret
}
