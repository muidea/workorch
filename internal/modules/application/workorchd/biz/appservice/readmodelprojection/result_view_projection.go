package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ProjectResultView(item readmodelcore.Result) readmodelcore.Result {
	return readmodelcore.Result{
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

func cloneResultViewSummary(item *readmodelcore.ResultSummary) *readmodelcore.ResultSummary {
	if item == nil {
		return nil
	}
	ret := &readmodelcore.ResultSummary{
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
		ret.DoDItems = make([]readmodelcore.DoDAssessmentItem, 0, len(item.DoDItems))
		for _, dodItem := range item.DoDItems {
			ret.DoDItems = append(ret.DoDItems, cloneResultViewDoDAssessmentItem(dodItem))
		}
	}
	return ret
}

func cloneResultViewDoDAssessment(item *readmodelcore.DoDAssessment) *readmodelcore.DoDAssessment {
	if item == nil {
		return nil
	}
	ret := &readmodelcore.DoDAssessment{
		Satisfied:            item.Satisfied,
		Summary:              item.Summary,
		GoalCoverage:         cloneResultViewDoDCoverage(item.GoalCoverage),
		AcceptanceDimensions: cloneResultViewDoDCoverage(item.AcceptanceDimensions),
		DeliverableCoverage:  cloneResultViewDoDCoverage(item.DeliverableCoverage),
	}
	if len(item.Items) > 0 {
		ret.Items = make([]readmodelcore.DoDAssessmentItem, 0, len(item.Items))
		for _, assessmentItem := range item.Items {
			ret.Items = append(ret.Items, cloneResultViewDoDAssessmentItem(assessmentItem))
		}
	}
	return ret
}

func cloneResultViewDoDAssessmentItem(item readmodelcore.DoDAssessmentItem) readmodelcore.DoDAssessmentItem {
	item.EvidenceRefs = cloneReadModelStringSlice(item.EvidenceRefs)
	return item
}

func cloneResultViewDoDCoverage(item *readmodelcore.DoDCoverage) *readmodelcore.DoDCoverage {
	if item == nil {
		return nil
	}
	ret := &readmodelcore.DoDCoverage{
		Satisfied: item.Satisfied,
		Summary:   item.Summary,
	}
	if len(item.Items) > 0 {
		ret.Items = make([]readmodelcore.DoDCoverageItem, 0, len(item.Items))
		for _, coverageItem := range item.Items {
			coverageItem.EvidenceRefs = cloneReadModelStringSlice(coverageItem.EvidenceRefs)
			coverageItem.ArtifactRefs = cloneReadModelStringSlice(coverageItem.ArtifactRefs)
			ret.Items = append(ret.Items, coverageItem)
		}
	}
	return ret
}
