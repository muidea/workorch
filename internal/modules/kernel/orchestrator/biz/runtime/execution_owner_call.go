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
	ownerCatalog := runtimeOwnerCatalogForOperation(
		x.dispatch.OperationAdmissibilities,
		x.dispatch.OperationAdmissibilityObservations,
		operation,
		prepared.execution.AdmissibilitySnapshotID,
		prepared.execution.AcquisitionBinding,
	)
	result, err := x.executor.ExecuteRuntimeOperation(x.ctx, RuntimeOperationExecutionRequest{
		WorkspaceID: x.workspaceID, RunID: x.runID, DispatchID: x.dispatch.ID,
		Transaction: x.dispatch.Transaction, Operation: operation, Execution: prepared.execution,
		GovernanceGrant:                    prepared.governanceGrant,
		EvidenceKind:                       evidenceKindForDecision(x.aggregate.Decision),
		TargetRole:                         x.aggregate.Facts.Focus.TargetRole,
		EvidencePolicies:                   x.aggregate.Facts.Activation.EvidencePolicies,
		ValidationRecipes:                  runtimecontract.NormalizeRuntimeValidationRecipes(x.dispatch.ValidationRecipes),
		ValidationScopeTargetRefs:          runtimeValidationScopeTargetRefs(x.aggregate.Facts, operation),
		CapabilityStates:                   append([]runtimecontract.RuntimeCapabilityStateSnapshot(nil), x.aggregate.Facts.Resources.CapabilityStates...),
		WorkspaceTreeObservation:           runtimeWorkspaceManifestObservation(x.aggregate),
		CompletedRequiredOperationReceipts: completedRequiredOperationReceiptBindings(*x.aggregate.ActiveDispatch, x.aggregate.OperationReceipts, x.completed),
		OwnerCatalog:                       ownerCatalog,
		OwnerCatalogObservation:            runtimeOwnerCatalogObservationForOperation(x.dispatch.OperationAdmissibilityObservations, ownerCatalog),
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
	// The persisted claim is the pre-call write-ahead fence for both ordinary and
	// governance-authorized execution. Only the replay class/proof sealed by SM-3
	// may turn a missing receipt into a bounded SM-4 recovery handoff.
	if runtimeOperationHasReplayAuthority(prepared.execution) {
		// The re-drive reloads only persisted aggregate facts and hands recovery
		// budget acquisition back to SM-4.
		return activeDispatchOwnerCall{flow: activeDispatchExecutionRetry}, nil
	}
	if _, markErr := x.component.markOperationExecutionOutcomeUnknown(x.ctx, x.workspaceID, x.runID, operation.ID); markErr != nil {
		return activeDispatchOwnerCall{}, markErr
	}
	return activeDispatchOwnerCall{}, err
}

func runtimeOwnerCatalogObservationForOperation(values []runtimecontract.RuntimePrerequisiteObservation, catalog *runtimecontract.RuntimeOperationAdmissibility) *runtimecontract.RuntimePrerequisiteObservation {
	if catalog == nil {
		return nil
	}
	catalogValue := catalog.Normalized()
	for _, raw := range values {
		observation := raw.Normalized()
		if observation.PrerequisiteID != catalogValue.PrerequisiteID || observation.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility ||
			observation.OwnerModuleID != catalogValue.OwnerModuleID || observation.ContentHash != catalogValue.SnapshotID || observation.Validate() != nil {
			continue
		}
		observation.Dependencies = append([]runtimecontract.RuntimeFreshnessDependency(nil), observation.Dependencies...)
		return &observation
	}
	return nil
}

func runtimeValidationScopeTargetRefs(facts runtimecontract.RuntimeFacts, operation runtimecontract.RuntimeOperation) []string {
	if runtimecontract.CanonicalRuntimeBlockKind(operation.Kind) != runtimecontract.RuntimeBlockKindManagedValidationSession {
		return nil
	}
	facts = facts.Normalized()
	if facts.Resources.Manifest == nil {
		return nil
	}
	return append([]string(nil), facts.Resources.Manifest.TargetRefs...)
}

func runtimeOwnerCatalogForOperation(values []runtimecontract.RuntimeOperationAdmissibility, observations []runtimecontract.RuntimePrerequisiteObservation, operation runtimecontract.RuntimeOperation, requiredSnapshotID, requiredAcquisitionBinding string) *runtimecontract.RuntimeOperationAdmissibility {
	owner := runtimecontract.RuntimeModuleForOperationKind(operation.Kind)
	canonicalKind := runtimecontract.CanonicalRuntimeBlockKind(operation.Kind)
	requiredSnapshotID = strings.TrimSpace(requiredSnapshotID)
	requiredAcquisitionBinding = strings.TrimSpace(requiredAcquisitionBinding)
	// SM-3 execution authority comes only from the catalog identity sealed into
	// the execution record.  Looking up an arbitrary current catalog by owner and
	// operation kind would silently replace missing authority after restart.
	if requiredSnapshotID == "" {
		return nil
	}
	for _, catalog := range values {
		catalog = catalog.Normalized()
		if catalog.OwnerModuleID != owner || catalog.SnapshotID != requiredSnapshotID || catalog.AcquisitionBinding != requiredAcquisitionBinding ||
			!runtimeCatalogContainsOperationKind(catalog, canonicalKind) || catalog.Validate() != nil {
			continue
		}
		if runtimeOwnerCatalogObservationForOperation(observations, &catalog) == nil {
			continue
		}
		value := catalog
		return &value
	}
	return nil
}

func runtimeCatalogContainsOperationKind(catalog runtimecontract.RuntimeOperationAdmissibility, kind string) bool {
	for _, available := range catalog.OperationKinds {
		if runtimecontract.CanonicalRuntimeBlockKind(available) == kind {
			return true
		}
	}
	return false
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
