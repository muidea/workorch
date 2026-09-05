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
		operation.BindingRefs = []string{"binding/shell-test/" + recipeID}
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
	sealInput := shellevents.RuntimeExecutionSealInput{
		TargetRole: targetRole, ValidationScopeTargetRefs: validationScope, EvidencePolicies: executionContext.EvidencePolicies,
	}
	if kind == runtimecontract.RuntimeBlockKindManagedValidationSession {
		requirement := shellValidationProofRequirementForTest(t, data.ValidationRecipeID, "cwi-shell-seal-test")
		attestations := shellValidationCheckAttestationsForTest(requirement.RequiredCheckRefs)
		proofBinding, bindErr := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, data.ValidationRecipeID, attestations...)
		if bindErr != nil {
			t.Fatal(bindErr)
		}
		catalog.ValidationRecipeRefs = []string{data.ValidationRecipeID}
		catalog.ValidationSurfaces = []runtimecontract.RuntimeValidationSurface{{
			ID: data.ValidationRecipeID, OwnerModuleID: runtimecontract.RuntimeModuleShell,
			OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			Readiness:     runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "shell-test-managed-validation-schema",
			InputBindingRef: "shell-test-managed-validation-input", CheckRefs: append([]string(nil), requirement.RequiredCheckRefs...),
			ProofApplicability:       runtimecontract.RuntimeValidationProofDirect,
			ProofRequirementBindings: []runtimecontract.RuntimeValidationProofRequirementBinding{proofBinding},
			EffectScope:              runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		}}
		sealInput.ValidationProofBinding = proofBinding
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	observation := adapter.runtimeAdmissibilityObservation(executionContext.WorkspaceDeliverableDirectory, catalog.PrerequisiteID, catalog, catalog.TargetRefs)
	sealInput.CatalogObservation = observation
	sealed, err := shellevents.SealRuntimeOperation(operation, catalog, sealInput)
	if err != nil {
		t.Fatal(err)
	}
	executionContext.Execution.AdmissibilitySnapshotID = sealed.Authorization.CatalogSnapshotID
	executionContext.Execution.AcquisitionBinding = sealed.Authorization.AcquisitionBinding
	executionContext.Execution.OperationHash = runtimecontract.StableRuntimeOperationHash(operation)
	executionContext.Execution.IdempotencyClass = runtimecontract.ResolveOperationIdempotencyClass(operation, []runtimecontract.RuntimeOperationPolicyFact{sealed.Authorization.Policy})
	executionContext.Execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: operation.ID, OperationHash: executionContext.Execution.OperationHash, ExecutionKey: executionContext.Execution.ExecutionKey,
		ExpectedEffectScope: executionContext.Execution.ExpectedEffectScope, IdempotencyClass: executionContext.Execution.IdempotencyClass,
		AdmissibilitySnapshotID: executionContext.Execution.AdmissibilitySnapshotID, AcquisitionBinding: executionContext.Execution.AcquisitionBinding,
		ReplayProof: executionContext.Execution.ReplayProof,
	})
	return shellevents.ExecuteOperationCommand{RuntimeOperationExecutionContext: executionContext, SealedOperation: sealed}
}

func shellValidationCheckAttestationsForTest(requiredCheckRefs []string) []runtimecontract.RuntimeValidationCheckAttestationBinding {
	ret := make([]runtimecontract.RuntimeValidationCheckAttestationBinding, 0, len(requiredCheckRefs))
	for _, checkRef := range requiredCheckRefs {
		ret = append(ret, runtimecontract.RuntimeValidationCheckAttestationBinding{
			RequiredCheckRef: checkRef, ObservationProtocolRef: shellGoTestObservationProtocolRef,
			NativeCheckRef: "TestWorkOrchCWI_SHELLSEALTEST",
		})
	}
	return runtimecontract.NormalizeRuntimeValidationCheckAttestationBindings(ret)
}
