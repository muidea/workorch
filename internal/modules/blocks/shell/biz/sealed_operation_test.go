package shell

import (
	"testing"

	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func sealedShellTestCommand(t *testing.T, adapter *RuntimeAdapter, executionContext runtimecontract.RuntimeOperationExecutionContext, kind string, data shellevents.OperationData, targetRole string, recipes []runtimecontract.RuntimeValidationRecipe, validationScope []string) shellevents.ExecuteOperationCommand {
	t.Helper()
	operation := runtimecontract.RuntimeOperation{
		ID: executionContext.OperationID, Kind: kind,
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), executionContext.CoverageWorkItemIDs...),
		TargetRefs:          append([]string(nil), executionContext.TargetRefs...),
	}
	if kind == runtimecontract.RuntimeBlockKindManagedValidationSession {
		recipeID := data.ValidationRecipeID
		operation.Parameters = []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}}
	} else {
		command := data.Command
		stateChanging := data.StateChanging
		operation.Parameters = []runtimecontract.OperationParameter{
			{Name: "command", String: &command},
			{Name: "args", StringList: append([]string(nil), data.Args...)},
			{Name: "stateChanging", Boolean: &stateChanging},
		}
		if data.TimeoutMillis > 0 {
			timeout := data.TimeoutMillis
			operation.Parameters = append(operation.Parameters, runtimecontract.OperationParameter{Name: "timeoutMillis", Integer: &timeout})
		}
		if data.VerificationScope != "" {
			value := data.VerificationScope
			operation.Parameters = append(operation.Parameters, runtimecontract.OperationParameter{Name: "verificationScope", String: &value})
		}
		if data.BoundedReason != "" {
			value := data.BoundedReason
			operation.Parameters = append(operation.Parameters, runtimecontract.OperationParameter{Name: "boundedReason", String: &value})
		}
	}
	catalog, err := adapter.AcquireRuntimeOperationAdmissibility(executionContext.WorkspaceDeliverableDirectory, "shell-owner-catalog", []string{kind})
	if err != nil {
		t.Fatal(err)
	}
	catalog.TargetRefs = append([]string(nil), executionContext.TargetRefs...)
	catalog.ValidationRecipes = runtimecontract.NormalizeRuntimeValidationRecipes(recipes)
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	observation := adapter.runtimeAdmissibilityObservation(executionContext.WorkspaceDeliverableDirectory, catalog.PrerequisiteID, catalog, catalog.TargetRefs)
	sealed, err := shellevents.SealRuntimeOperation(operation, catalog, shellevents.RuntimeExecutionSealInput{
		CatalogObservation: observation, TargetRole: targetRole,
		ValidationScopeTargetRefs: validationScope, EvidencePolicies: executionContext.EvidencePolicies,
	})
	if err != nil {
		t.Fatal(err)
	}
	executionContext.Execution.AdmissibilitySnapshotID = sealed.Authorization.CatalogSnapshotID
	executionContext.Execution.AcquisitionBinding = sealed.Authorization.AcquisitionBinding
	return shellevents.ExecuteOperationCommand{RuntimeOperationExecutionContext: executionContext, SealedOperation: sealed}
}
