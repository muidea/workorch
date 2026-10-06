package runtime

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

// applyAuthorityInternalEvent keeps one route for the concrete authority
// payload family. Its delegates are owner-private cohesion helpers only: they
// share the controller kernel and cannot be called as mutation entries.
func (c *RuntimeController) applyAuthorityInternalEvent(ctx context.Context, event RuntimeInternalEvent) (RuntimeInternalEventResult, error) {
	switch event.Kind {
	case RuntimeInternalEventImplementationWorkScopeRequired:
		if event.Authority.ImplementationWorkScopeRequest == nil || event.Authority.ImplementationWorkScopeObservation != nil {
			return RuntimeInternalEventResult{}, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.reserveImplementationWorkScope(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.ImplementationWorkScopeRequest)
		return RuntimeInternalEventResult{Aggregate: aggregate}, err
	case RuntimeInternalEventImplementationWorkScopeObserved:
		if event.Authority.ImplementationWorkScopeObservation == nil || event.Authority.ImplementationWorkScopeRequest != nil {
			return RuntimeInternalEventResult{}, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.observeImplementationWorkScope(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.ImplementationWorkScopeObservation)
		return RuntimeInternalEventResult{Aggregate: aggregate}, err
	case RuntimeInternalEventSemanticContractRequired,
		RuntimeInternalEventSemanticCatalogInvalidated,
		RuntimeInternalEventSemanticFeasibilityRequired,
		RuntimeInternalEventSemanticFeasibilityObserved,
		RuntimeInternalEventSemanticContractSealed,
		RuntimeInternalEventSemanticContractFailed:
		return c.applyAuthorityPlanningInternalEvent(ctx, event)
	case RuntimeInternalEventOwnerFeasibilityObserved,
		RuntimeInternalEventSemanticBindingContradictionObserved:
		return c.applyAuthorityCandidateInternalEvent(ctx, event)
	case RuntimeInternalEventReconcileAuthority,
		RuntimeInternalEventReconciliationBatchObserved,
		RuntimeInternalEventSemanticCatalogScopeInvalidated,
		RuntimeInternalEventCWIBindingRetired,
		RuntimeInternalEventEligibleFocusProjected,
		RuntimeInternalEventPrerequisiteObserved,
		RuntimeInternalEventPrerequisitesReadOnlyStaleObserved,
		RuntimeInternalEventFeasibilityObserved,
		RuntimeInternalEventFailureObserved,
		RuntimeInternalEventAssessmentObserved,
		RuntimeInternalEventGovernanceRequestObserved,
		RuntimeInternalEventValidationFailureObserved,
		RuntimeInternalEventRepairSurfaceConfirmationObserved,
		RuntimeInternalEventDefinitionGapObserved:
		return c.applyAuthorityConvergenceInternalEvent(ctx, event)
	default:
		return unsupportedRuntimeInternalEvent(event.Kind)
	}
}

func (c *RuntimeController) applyAuthorityPlanningInternalEvent(ctx context.Context, event RuntimeInternalEvent) (RuntimeInternalEventResult, error) {
	result := RuntimeInternalEventResult{}
	switch event.Kind {
	case RuntimeInternalEventSemanticContractRequired:
		aggregate, err := c.planning.requireSemanticContract(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Authority.SemanticBatchID, event.Authority.SemanticRefreshGeneration)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticCatalogInvalidated:
		if event.Authority.ManifestHash == "" || event.Authority.LogicalRequestID == "" {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.invalidateSemanticCatalogGeneration(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Authority.ManifestHash, event.Authority.LogicalRequestID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticFeasibilityRequired:
		aggregate, err := c.planning.requireSemanticFeasibility(ctx, event.Scope.WorkspaceID, event.Scope.RunID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticFeasibilityObserved:
		if event.Authority.SemanticFeasibility == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.observeSemanticFeasibility(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Authority.SemanticBatchID, *event.Authority.SemanticFeasibility)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticContractSealed:
		aggregate, err := c.planning.sealSemanticContract(ctx, event.Scope.WorkspaceID, event.Scope.RunID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticContractFailed:
		if event.Authority.SemanticDiagnostic == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.planning.failSemanticContract(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Authority.SemanticFailureCode, *event.Authority.SemanticDiagnostic)
		result.Aggregate = aggregate
		return result, err
	default:
		return unsupportedRuntimeInternalEvent(event.Kind)
	}
}

func (c *RuntimeController) applyAuthorityCandidateInternalEvent(ctx context.Context, event RuntimeInternalEvent) (RuntimeInternalEventResult, error) {
	result := RuntimeInternalEventResult{}
	switch event.Kind {
	case RuntimeInternalEventOwnerFeasibilityObserved:
		if event.Authority.OwnerFeasibility == nil || event.Authority.OwnerFeasibility.SealedTargetAuthorizationContradicted() {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.invalidateCandidateAttemptForOwnerInfeasibility(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Authority.Attempt, *event.Authority.OwnerFeasibility)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticBindingContradictionObserved:
		if event.Authority.OwnerFeasibility == nil || !event.Authority.OwnerFeasibility.SealedTargetAuthorizationContradicted() {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.candidateDispatch.invalidateCandidateAttemptForOwnerInfeasibility(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Authority.Attempt, *event.Authority.OwnerFeasibility)
		result.Aggregate = aggregate
		return result, err
	default:
		return unsupportedRuntimeInternalEvent(event.Kind)
	}
}

func (c *RuntimeController) applyAuthorityConvergenceInternalEvent(ctx context.Context, event RuntimeInternalEvent) (RuntimeInternalEventResult, error) {
	result := RuntimeInternalEventResult{}
	switch event.Kind {
	case RuntimeInternalEventReconcileAuthority:
		aggregate, err := c.authority.reconcileAuthorityBeforeCandidate(ctx, event.Scope.WorkspaceID, event.Scope.RunID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventReconciliationBatchObserved:
		if event.Authority.ReconciliationBatch == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, _, err := c.authority.commitReconciliationBatch(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.ReconciliationBatch)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventSemanticCatalogScopeInvalidated:
		if event.Authority.SemanticCatalogScopeGap == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.invalidateSemanticCatalogForFocusScope(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.SemanticCatalogScopeGap)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventCWIBindingRetired:
		if event.Authority.CWIBindingRetirement == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.retireRuntimeCWIBinding(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.CWIBindingRetirement)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventEligibleFocusProjected:
		aggregate, err := c.authority.projectEligibleRuntimeFocus(ctx, event.Scope.WorkspaceID, event.Scope.RunID, event.Authority.SemanticOrderManifestHash, event.Authority.SemanticCoverageOrder)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventPrerequisiteObserved:
		if event.Authority.PrerequisiteResult == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.commitPrerequisite(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.PrerequisiteResult, event.Authority.RecoverableWaitProbeObservation)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventPrerequisitesReadOnlyStaleObserved:
		if event.Authority.Stale == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.invalidatePrerequisitesForReadOnlyStale(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.Stale)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventFeasibilityObserved:
		if event.Authority.FeasibilityFact == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.recordFeasibilityFact(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.FeasibilityFact)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventFailureObserved:
		if event.Authority.FailureFact == nil {
			return result, common.ErrInvalidArgument
		}
		var aggregate RuntimeAggregate
		var err error
		if event.Authority.CloseActiveDispatch {
			aggregate, err = c.authority.recordDriveFailureFact(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.FailureFact)
		} else {
			aggregate, err = c.authority.recordFailureFact(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.FailureFact)
		}
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventAssessmentObserved:
		aggregate, err := c.authority.commitAssessment(ctx, event.Scope.WorkspaceID, event.Scope.RunID)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventGovernanceRequestObserved:
		if event.Authority.GovernanceRequest == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.bindGovernanceRequest(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.GovernanceRequest)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventValidationFailureObserved:
		// The rejected operation receipt already persisted the capability fact.
		// This explicit owner-local handoff is the first event permitted to turn
		// that observation into repair authority.
		if event.Authority.FailureFact == nil || event.Authority.FailureFact.ValidationFailureObservation == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.recordValidationFailureObservation(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.FailureFact.ValidationFailureObservation)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventRepairSurfaceConfirmationObserved:
		if event.Authority.RepairSurfaceConfirmation == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.recordRepairSurfaceConfirmation(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.RepairSurfaceConfirmation)
		result.Aggregate = aggregate
		return result, err
	case RuntimeInternalEventDefinitionGapObserved:
		if event.Authority.DefinitionGap == nil {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.authority.observeRuntimeDefinitionGap(ctx, event.Scope.WorkspaceID, event.Scope.RunID, *event.Authority.DefinitionGap)
		result.Aggregate = aggregate
		return result, err
	default:
		return unsupportedRuntimeInternalEvent(event.Kind)
	}
}
