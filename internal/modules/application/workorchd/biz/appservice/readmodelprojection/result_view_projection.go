package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ProjectResultView(item projectioncontract.Result) projectioncontract.Result {
	return projectioncontract.Result{
		ID:            item.ID,
		WorkspaceID:   item.WorkspaceID,
		RunID:         item.RunID,
		GoalID:        item.GoalID,
		DoDID:         item.DoDID,
		State:         item.State,
		Title:         item.Title,
		Summary:       item.Summary,
		SourceType:    item.SourceType,
		SourceRef:     item.SourceRef,
		EvidenceRefs:  cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs:  cloneReadModelStringSlice(item.ArtifactRefs),
		DoDAssessment: cloneResultViewDoDAssessment(item.DoDAssessment),
		SummaryDetail: cloneResultViewSummary(item.SummaryDetail),
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}
}

func cloneResultViewSummary(item *projectioncontract.ResultSummary) *projectioncontract.ResultSummary {
	if item == nil {
		return nil
	}
	ret := &projectioncontract.ResultSummary{
		GoalOutcome:           item.GoalOutcome,
		ImplementationSummary: item.ImplementationSummary,
		VerificationSummary:   item.VerificationSummary,
		EvidenceRefs:          cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs:          cloneReadModelStringSlice(item.ArtifactRefs),
		KnownLimitations:      cloneReadModelStringSlice(item.KnownLimitations),
		ResidualRisks:         cloneReadModelStringSlice(item.ResidualRisks),
		NextActions:           cloneReadModelStringSlice(item.NextActions),
		Diagnostics:           cloneReadModelMap(item.Diagnostics),
	}
	if len(item.DoDItems) > 0 {
		ret.DoDItems = make([]projectioncontract.DoDAssessmentItem, 0, len(item.DoDItems))
		for _, dodItem := range item.DoDItems {
			ret.DoDItems = append(ret.DoDItems, cloneResultViewDoDAssessmentItem(dodItem))
		}
	}
	return ret
}

func cloneResultViewDoDAssessment(item *projectioncontract.DoDAssessment) *projectioncontract.DoDAssessment {
	if item == nil {
		return nil
	}
	ret := &projectioncontract.DoDAssessment{
		Satisfied:            item.Satisfied,
		Summary:              item.Summary,
		GoalCoverage:         cloneResultViewDoDCoverage(item.GoalCoverage),
		AcceptanceDimensions: cloneResultViewDoDCoverage(item.AcceptanceDimensions),
		DeliverableCoverage:  cloneResultViewDoDCoverage(item.DeliverableCoverage),
	}
	if len(item.Items) > 0 {
		ret.Items = make([]projectioncontract.DoDAssessmentItem, 0, len(item.Items))
		for _, assessmentItem := range item.Items {
			ret.Items = append(ret.Items, cloneResultViewDoDAssessmentItem(assessmentItem))
		}
	}
	return ret
}

func cloneResultViewDoDAssessmentItem(item projectioncontract.DoDAssessmentItem) projectioncontract.DoDAssessmentItem {
	item.EvidenceRefs = cloneReadModelStringSlice(item.EvidenceRefs)
	return item
}

func cloneResultViewDoDCoverage(item *projectioncontract.DoDCoverage) *projectioncontract.DoDCoverage {
	if item == nil {
		return nil
	}
	ret := &projectioncontract.DoDCoverage{
		Satisfied: item.Satisfied,
		Summary:   item.Summary,
	}
	if len(item.Items) > 0 {
		ret.Items = make([]projectioncontract.DoDCoverageItem, 0, len(item.Items))
		for _, coverageItem := range item.Items {
			coverageItem.EvidenceRefs = cloneReadModelStringSlice(coverageItem.EvidenceRefs)
			coverageItem.ArtifactRefs = cloneReadModelStringSlice(coverageItem.ArtifactRefs)
			ret.Items = append(ret.Items, coverageItem)
		}
	}
	return ret
}
