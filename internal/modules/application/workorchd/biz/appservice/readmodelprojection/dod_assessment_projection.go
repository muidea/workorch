package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorchresultcontract"

// WD5 mechanical consolidation: same import contract; source list follows.

// source: dod_assessment.go

func mapDoDAssessment(item workorchresultcontract.DoDAssessment) *workorchresultcontract.DoDAssessment {
	if item.Summary == "" &&
		len(item.Items) == 0 &&
		item.GoalCoverage == nil &&
		item.AcceptanceDimensions == nil &&
		item.DeliverableCoverage == nil &&
		!item.Satisfied {
		return nil
	}
	ret := &workorchresultcontract.DoDAssessment{
		Satisfied:            item.Satisfied,
		Summary:              item.Summary,
		GoalCoverage:         mapDoDCoverage(item.GoalCoverage),
		AcceptanceDimensions: mapDoDCoverage(item.AcceptanceDimensions),
		DeliverableCoverage:  mapDoDCoverage(item.DeliverableCoverage),
	}
	if len(item.Items) > 0 {
		ret.Items = make([]workorchresultcontract.DoDAssessmentItem, 0, len(item.Items))
		for _, assessmentItem := range item.Items {
			ret.Items = append(ret.Items, mapDoDAssessmentItem(assessmentItem))
		}
	}
	return ret
}

// source: dod_assessment_item.go

func mapDoDAssessmentItem(item workorchresultcontract.DoDAssessmentItem) workorchresultcontract.DoDAssessmentItem {
	return workorchresultcontract.DoDAssessmentItem{
		Item:          item.Item,
		State:         item.State,
		EvidenceRefs:  cloneReadModelStringSlice(item.EvidenceRefs),
		FailureReason: item.FailureReason,
		Confidence:    item.Confidence,
		SourceNote:    item.SourceNote,
	}
}

// source: dod_coverage_mapping.go

func mapDoDCoverage(item *workorchresultcontract.DoDCoverage) *workorchresultcontract.DoDCoverage {
	if item == nil {
		return nil
	}
	ret := &workorchresultcontract.DoDCoverage{
		Satisfied: item.Satisfied,
		Summary:   item.Summary,
	}
	if len(item.Items) > 0 {
		ret.Items = make([]workorchresultcontract.DoDCoverageItem, 0, len(item.Items))
		for _, coverageItem := range item.Items {
			ret.Items = append(ret.Items, mapDoDCoverageItem(coverageItem))
		}
	}
	return ret
}

func mapDoDCoverageItem(item workorchresultcontract.DoDCoverageItem) workorchresultcontract.DoDCoverageItem {
	return workorchresultcontract.DoDCoverageItem{
		ID:           item.ID,
		Deliverable:  item.Deliverable,
		Name:         item.Name,
		Path:         item.Path,
		Goal:         item.Goal,
		GoalItem:     item.GoalItem,
		Item:         item.Item,
		Requirement:  item.Requirement,
		State:        item.State,
		EvidenceRefs: cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs: dodCoverageArtifactRefs(item),
	}
}

// source: result_summary.go

func mapResultSummary(item workorchresultcontract.ResultSummary) *workorchresultcontract.ResultSummary {
	if item.GoalOutcome == "" &&
		len(item.DoDItems) == 0 &&
		item.ImplementationSummary == "" &&
		item.VerificationSummary == "" &&
		len(item.EvidenceRefs) == 0 &&
		len(item.ArtifactRefs) == 0 &&
		len(item.KnownLimitations) == 0 &&
		len(item.ResidualRisks) == 0 &&
		len(item.NextActions) == 0 &&
		len(item.Diagnostics.Fields) == 0 {
		return nil
	}
	ret := &workorchresultcontract.ResultSummary{
		GoalOutcome:           item.GoalOutcome,
		ImplementationSummary: item.ImplementationSummary,
		VerificationSummary:   item.VerificationSummary,
		EvidenceRefs:          cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs:          resultSummaryArtifactRefs(item),
		KnownLimitations:      cloneReadModelStringSlice(item.KnownLimitations),
		ResidualRisks:         cloneReadModelStringSlice(item.ResidualRisks),
		NextActions:           cloneReadModelStringSlice(item.NextActions),
		Diagnostics:           item.Diagnostics.Clone(),
	}
	ret.DoDItems = resultSummaryDoDItems(item)
	return ret
}
