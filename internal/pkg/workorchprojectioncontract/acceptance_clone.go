package workorchprojectioncontract

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

func CloneAcceptanceProjection(item AcceptanceProjection) AcceptanceProjection {
	item.Slices = cloneAcceptanceSlices(item.Slices)
	item.CurrentFocus = cloneCoverageFocus(item.CurrentFocus)
	item.Budget = cloneConvergenceBudget(item.Budget)
	item.RepairDirective = cloneRuntimeRepairDirective(item.RepairDirective)
	item.CapabilityStates = cloneRuntimeCapabilityStates(item.CapabilityStates)
	item.Governance = append([]RuntimeGovernanceState(nil), item.Governance...)
	item.Operations = append([]RuntimeOperationStatus(nil), item.Operations...)
	item.SemanticPlanning = cloneRuntimeSemanticPlanning(item.SemanticPlanning)
	item.Audit = CloneRuntimeAudit(item.Audit)
	return item
}

func cloneRuntimeCapabilityStates(items []RuntimeCapabilityState) []RuntimeCapabilityState {
	if items == nil {
		return nil
	}
	ret := append([]RuntimeCapabilityState(nil), items...)
	for idx := range ret {
		ret[idx].RequiredOperationSequence = runtimecontract.CloneRequiredOperationVariants(items[idx].RequiredOperationSequence)
		for sequenceIdx := range ret[idx].RequiredOperationSequence {
			ret[idx].RequiredOperationSequence[sequenceIdx].DependsOnRequirementIDs = append([]string(nil), items[idx].RequiredOperationSequence[sequenceIdx].DependsOnRequirementIDs...)
		}
		ret[idx].InvalidatedByReceiptRefs = append([]string(nil), items[idx].InvalidatedByReceiptRefs...)
	}
	return ret
}

func cloneRuntimeRepairDirective(item *RuntimeRepairDirective) *RuntimeRepairDirective {
	if item == nil {
		return nil
	}
	ret := *item
	ret.RequiredEffectScopes = append([]string(nil), item.RequiredEffectScopes...)
	ret.RequiredEvidenceKinds = append([]string(nil), item.RequiredEvidenceKinds...)
	ret.AllowedOperationKinds = append([]string(nil), item.AllowedOperationKinds...)
	ret.CoverageWorkItemIDs = append([]string(nil), item.CoverageWorkItemIDs...)
	ret.TargetRefs = append([]string(nil), item.TargetRefs...)
	if item.Closure != nil {
		closure := *item.Closure
		closure.CoverageWorkItemIDs = append([]string(nil), item.Closure.CoverageWorkItemIDs...)
		closure.RequiredEvidenceKinds = append([]string(nil), item.Closure.RequiredEvidenceKinds...)
		closure.GroundedTargetRefs = append([]string(nil), item.Closure.GroundedTargetRefs...)
		ret.Closure = &closure
	}
	return &ret
}

func cloneRuntimeSemanticPlanning(item *RuntimeSemanticPlanning) *RuntimeSemanticPlanning {
	if item == nil {
		return nil
	}
	ret := *item
	ret.Items = cloneRuntimeSemanticPlanningItems(item.Items)
	ret.Rejections = cloneRuntimeSemanticDiagnostics(item.Rejections)
	if item.FeasibilityGaps != nil {
		ret.FeasibilityGaps = make([]RuntimeSemanticFeasibilityGap, 0, len(item.FeasibilityGaps))
		for _, gap := range item.FeasibilityGaps {
			gap.Items = cloneRuntimeSemanticPlanningItems(gap.Items)
			gap.Rejections = cloneRuntimeSemanticDiagnostics(gap.Rejections)
			ret.FeasibilityGaps = append(ret.FeasibilityGaps, gap)
		}
	}
	if item.Failure != nil {
		failure := cloneRuntimeSemanticDiagnostic(*item.Failure)
		ret.Failure = &failure
	}
	return &ret
}

func cloneRuntimeSemanticPlanningItems(items []RuntimeSemanticPlanningItem) []RuntimeSemanticPlanningItem {
	if items == nil {
		return nil
	}
	ret := make([]RuntimeSemanticPlanningItem, 0, len(items))
	for _, item := range items {
		item.TargetRefs = append([]string(nil), item.TargetRefs...)
		item.RequiredCheckRefs = append([]string(nil), item.RequiredCheckRefs...)
		if item.Diagnostic != nil {
			diagnostic := cloneRuntimeSemanticDiagnostic(*item.Diagnostic)
			item.Diagnostic = &diagnostic
		}
		ret = append(ret, item)
	}
	return ret
}

func cloneRuntimeSemanticDiagnostics(items []RuntimeSemanticDiagnostic) []RuntimeSemanticDiagnostic {
	if items == nil {
		return nil
	}
	ret := make([]RuntimeSemanticDiagnostic, 0, len(items))
	for _, item := range items {
		ret = append(ret, cloneRuntimeSemanticDiagnostic(item))
	}
	return ret
}

func cloneRuntimeSemanticDiagnostic(item RuntimeSemanticDiagnostic) RuntimeSemanticDiagnostic {
	item.TargetRefs = append([]string(nil), item.TargetRefs...)
	item.FieldViolations = runtimecontract.CloneRuntimeFieldViolations(item.FieldViolations)
	return item
}

func cloneAcceptanceSlices(items []AcceptanceSlice) []AcceptanceSlice {
	if items == nil {
		return nil
	}
	ret := make([]AcceptanceSlice, 0, len(items))
	for _, item := range items {
		item.CoverageWorkItemIDs = append([]string(nil), item.CoverageWorkItemIDs...)
		item.RequiredEvidenceKinds = append([]string(nil), item.RequiredEvidenceKinds...)
		ret = append(ret, item)
	}
	return ret
}

func cloneCoverageFocus(item *CoverageFocus) *CoverageFocus {
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

func cloneConvergenceBudget(item *ConvergenceBudget) *ConvergenceBudget {
	if item == nil {
		return nil
	}
	ret := *item
	return &ret
}
