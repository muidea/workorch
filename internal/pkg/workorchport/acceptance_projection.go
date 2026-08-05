package workorchport

import (
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func AcceptanceProjectionInfoFromProjection(item projectioncontract.AcceptanceProjection) AcceptanceProjectionInfo {
	ret := AcceptanceProjectionInfo{
		ActiveSliceID:    item.ActiveSliceID,
		Summary:          item.Summary,
		Slices:           cloneAcceptanceSlices(item.Slices),
		CurrentFocus:     cloneCoverageFocus(item.CurrentFocus),
		Budget:           cloneConvergenceBudget(item.Budget),
		RepairDirective:  RuntimeRepairDirectiveInfoFromProjection(item.RepairDirective),
		CapabilityStates: cloneRuntimeCapabilityStates(item.CapabilityStates),
		Governance:       append([]projectioncontract.RuntimeGovernanceState(nil), item.Governance...),
		Operations:       append([]projectioncontract.RuntimeOperationStatus(nil), item.Operations...),
		SemanticPlanning: RuntimeSemanticPlanningInfoFromProjection(item.SemanticPlanning),
		Audit:            item.Audit,
	}
	return ret
}

func cloneRuntimeCapabilityStates(items []projectioncontract.RuntimeCapabilityState) []projectioncontract.RuntimeCapabilityState {
	ret := append([]projectioncontract.RuntimeCapabilityState(nil), items...)
	for idx := range ret {
		ret[idx].RequiredOperationSequence = append([]projectioncontract.RuntimeRequiredOperationState(nil), items[idx].RequiredOperationSequence...)
		for sequenceIdx := range ret[idx].RequiredOperationSequence {
			ret[idx].RequiredOperationSequence[sequenceIdx].DependsOnRequirementIDs = append([]string(nil), items[idx].RequiredOperationSequence[sequenceIdx].DependsOnRequirementIDs...)
		}
		ret[idx].InvalidatedByReceiptRefs = append([]string(nil), items[idx].InvalidatedByReceiptRefs...)
	}
	return ret
}

func RuntimeRepairDirectiveInfoFromProjection(item *projectioncontract.RuntimeRepairDirective) *RuntimeRepairDirectiveInfo {
	if item == nil {
		return nil
	}
	ret := &RuntimeRepairDirectiveInfo{
		ID: item.ID, FocusID: item.FocusID, Trigger: item.Trigger, RequiredAction: item.RequiredAction,
		RequiredEffectScopes: append([]string(nil), item.RequiredEffectScopes...), RequiredEvidenceKinds: append([]string(nil), item.RequiredEvidenceKinds...),
		AllowedOperationKinds: append([]string(nil), item.AllowedOperationKinds...), CoverageWorkItemIDs: append([]string(nil), item.CoverageWorkItemIDs...),
		TargetRefs: append([]string(nil), item.TargetRefs...), SupersedesEvidenceRevision: item.SupersedesEvidenceRevision, RepairAttempt: item.RepairAttempt,
	}
	if item.Closure != nil {
		ret.Closure = &projectioncontract.RuntimeRepairClosure{
			ID: item.Closure.ID, GapSignature: item.Closure.GapSignature, BaselineEvidenceRevision: item.Closure.BaselineEvidenceRevision,
			CoverageWorkItemIDs: append([]string(nil), item.Closure.CoverageWorkItemIDs...), RequiredEvidenceKinds: append([]string(nil), item.Closure.RequiredEvidenceKinds...),
			GroundedTargetRefs: append([]string(nil), item.Closure.GroundedTargetRefs...), RequireValidationAfterStateChange: item.Closure.RequireValidationAfterStateChange,
		}
	}
	return ret
}

func RuntimeSemanticPlanningInfoFromProjection(item *projectioncontract.RuntimeSemanticPlanning) *RuntimeSemanticPlanningInfo {
	if item == nil {
		return nil
	}
	ret := &RuntimeSemanticPlanningInfo{
		WorkspaceID: common.WorkspaceID(item.WorkspaceID), RunID: common.RunID(item.RunID),
		DefinitionRevision: item.DefinitionRevision, StateRevision: item.StateRevision,
		State: item.State, Phase: item.Phase, Attempt: item.Attempt,
		CatalogRefreshAttempts: item.CatalogRefreshAttempts, LogicalRequestID: item.LogicalRequestID,
		ManifestHash: item.ManifestHash, DocumentHash: item.DocumentHash,
		FeasibilityBatchID: item.FeasibilityBatchID, FeasibilitySetHash: item.FeasibilitySetHash,
		SealedBindingSetHash: item.SealedBindingSetHash,
	}
	for _, value := range item.Items {
		ret.Items = append(ret.Items, runtimeSemanticPlanningItemInfoFromProjection(value))
	}
	for _, value := range item.Rejections {
		ret.Rejections = append(ret.Rejections, runtimeSemanticDiagnosticFromProjection(value))
	}
	for _, value := range item.FeasibilityGaps {
		gap := RuntimeSemanticFeasibilityGapInfo{
			RefreshGeneration: value.RefreshGeneration, ManifestHash: value.ManifestHash,
			DocumentHash: value.DocumentHash, FeasibilityBatchID: value.FeasibilityBatchID,
			FeasibilitySetHash: value.FeasibilitySetHash,
		}
		for _, observation := range value.Items {
			gap.Items = append(gap.Items, runtimeSemanticPlanningItemInfoFromProjection(observation))
		}
		for _, rejection := range value.Rejections {
			gap.Rejections = append(gap.Rejections, runtimeSemanticDiagnosticFromProjection(rejection))
		}
		ret.FeasibilityGaps = append(ret.FeasibilityGaps, gap)
	}
	if item.Failure != nil {
		value := runtimeSemanticDiagnosticFromProjection(*item.Failure)
		ret.Failure = &value
	}
	return ret
}

func runtimeSemanticPlanningItemInfoFromProjection(item projectioncontract.RuntimeSemanticPlanningItem) RuntimeSemanticPlanningItemInfo {
	ret := RuntimeSemanticPlanningItemInfo{
		CoverageWorkItemID: item.CoverageWorkItemID, ProposalHash: item.ProposalHash,
		OwnerModuleID: item.OwnerModuleID, OperationKind: item.OperationKind, Status: item.Status,
		TargetRefs: append([]string(nil), item.TargetRefs...), ValidationRecipeID: item.ValidationRecipeID,
		RequiredCheckRefs: append([]string(nil), item.RequiredCheckRefs...), ObservedRevision: item.ObservedRevision,
	}
	if item.Diagnostic != nil {
		value := runtimeSemanticDiagnosticFromProjection(*item.Diagnostic)
		ret.Diagnostic = &value
	}
	return ret
}

func runtimeSemanticDiagnosticFromProjection(item projectioncontract.RuntimeSemanticDiagnostic) projectioncontract.RuntimeSemanticDiagnostic {
	ret := item
	ret.TargetRefs = append([]string(nil), item.TargetRefs...)
	ret.FieldViolations = runtimecontract.CloneRuntimeFieldViolations(item.FieldViolations)
	return ret
}

func cloneAcceptanceSlices(items []projectioncontract.AcceptanceSlice) []projectioncontract.AcceptanceSlice {
	ret := make([]projectioncontract.AcceptanceSlice, 0, len(items))
	for _, item := range items {
		value := item
		value.CoverageWorkItemIDs = append([]string(nil), item.CoverageWorkItemIDs...)
		value.RequiredEvidenceKinds = append([]string(nil), item.RequiredEvidenceKinds...)
		ret = append(ret, value)
	}
	return ret
}

func cloneCoverageFocus(item *projectioncontract.CoverageFocus) *projectioncontract.CoverageFocus {
	if item == nil {
		return nil
	}
	ret := *item
	ret.CoverageWorkItemIDs = append([]string(nil), item.CoverageWorkItemIDs...)
	ret.RequiredEvidenceKinds = append([]string(nil), item.RequiredEvidenceKinds...)
	ret.AcceptedEvidenceRefs = append([]string(nil), item.AcceptedEvidenceRefs...)
	ret.RejectedEvidenceRefs = append([]string(nil), item.RejectedEvidenceRefs...)
	ret.TargetRefs = append([]string(nil), item.TargetRefs...)
	return &ret
}

func cloneConvergenceBudget(item *projectioncontract.ConvergenceBudget) *projectioncontract.ConvergenceBudget {
	if item == nil {
		return nil
	}
	ret := *item
	return &ret
}
