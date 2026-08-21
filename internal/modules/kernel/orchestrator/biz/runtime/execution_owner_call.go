package runtime

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type activeDispatchOwnerCall struct {
	result RuntimeOperationExecutionResult
	flow   activeDispatchExecutionFlow
}

func (x *activeDispatchExecution) callOperationOwner(operation runtimecontract.RuntimeOperation, prepared preparedActiveDispatchOperation) (activeDispatchOwnerCall, error) {
	result, err := x.executor.ExecuteRuntimeOperation(x.ctx, RuntimeOperationExecutionRequest{
		WorkspaceID: x.workspaceID, RunID: x.runID, DispatchID: x.dispatch.ID,
		Transaction: x.dispatch.Transaction, Operation: operation, Execution: prepared.execution,
		GovernanceGrant:                    prepared.governanceGrant,
		EvidenceKind:                       evidenceKindForDecision(x.aggregate.Decision),
		TargetRole:                         x.aggregate.Facts.Focus.TargetRole,
		EvidencePolicies:                   x.aggregate.Facts.Activation.EvidencePolicies,
		ValidationRecipes:                  runtimecontract.NormalizeRuntimeValidationRecipes(x.dispatch.ValidationRecipes),
		CapabilityStates:                   append([]runtimecontract.RuntimeCapabilityStateSnapshot(nil), x.aggregate.Facts.Resources.CapabilityStates...),
		WorkspaceTreeObservation:           runtimeWorkspaceManifestObservation(x.aggregate),
		CompletedRequiredOperationReceipts: completedRequiredOperationReceiptBindings(*x.aggregate.ActiveDispatch, x.aggregate.OperationReceipts, x.completed),
		OwnerCatalog:                       runtimeOwnerCatalogForOperation(x.aggregate.Facts.Resources.OperationAdmissibilities, operation),
	})
	if err == nil {
		return activeDispatchOwnerCall{result: result, flow: activeDispatchExecutionReady}, nil
	}
	// The owner may have committed its terminal receipt while the response was
	// lost. Reload the exact ExecutionKey+Fence before declaring outcome_unknown.
	recovered, found, recoverErr := x.component.recoverOperationReceiptAfterExecuteError(
		x.ctx, x.workspaceID, x.runID, x.dispatch, operation, prepared.execution,
	)
	if recoverErr != nil {
		return activeDispatchOwnerCall{}, recoverErr
	}
	if found {
		x.aggregate = recovered
		return activeDispatchOwnerCall{flow: activeDispatchExecutionStop}, nil
	}
	if prepared.execution.ReplayProof != nil {
		// The re-drive reloads only persisted aggregate facts and hands recovery
		// budget acquisition back to SM-4.
		return activeDispatchOwnerCall{flow: activeDispatchExecutionRetry}, nil
	}
	_, _ = x.component.markOperationExecutionOutcomeUnknown(x.ctx, x.workspaceID, x.runID, operation.ID)
	return activeDispatchOwnerCall{}, err
}

func runtimeOwnerCatalogForOperation(values []runtimecontract.RuntimeOperationAdmissibility, operation runtimecontract.RuntimeOperation) *runtimecontract.RuntimeOperationAdmissibility {
	owner := runtimecontract.RuntimeModuleForOperationKind(operation.Kind)
	catalog, found := runtimeOperationAdmissibilityForKind(values, owner, operation.Kind)
	if !found {
		return nil
	}
	return &catalog
}

func runtimeWorkspaceManifestObservation(aggregate RuntimeAggregate) *runtimecontract.RuntimePrerequisiteObservation {
	if aggregate.Facts.Resources.Manifest == nil {
		return nil
	}
	revision := strings.TrimSpace(aggregate.Facts.Resources.Manifest.Revision)
	for _, observation := range aggregate.Facts.Resources.PrerequisiteObservations {
		observation = observation.Normalized()
		if observation.Kind != runtimecontract.PrerequisiteKindManifest || observation.OwnerModuleID != runtimecontract.RuntimeModuleFile || observation.ContentHash != revision {
			continue
		}
		value := observation
		value.Dependencies = append([]runtimecontract.RuntimeFreshnessDependency(nil), observation.Dependencies...)
		return &value
	}
	return nil
}

func validateOwnerReceiptIdentity(operation runtimecontract.RuntimeOperation, execution runtimecontract.RuntimeOperationExecutionRecord, result *RuntimeOperationExecutionResult) error {
	result.Receipt = result.Receipt.Normalized()
	if err := result.Receipt.Validate(); err != nil {
		return fmt.Errorf("owner receipt: %w", err)
	}
	if result.Receipt.ExecutionKey != execution.ExecutionKey || result.Receipt.OperationID != operation.ID {
		return fmt.Errorf("owner receipt does not match claimed execution")
	}
	return nil
}
