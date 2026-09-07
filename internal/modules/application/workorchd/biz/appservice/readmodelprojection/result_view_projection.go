package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

func projectResultView(item projectioncontract.Result) projectioncontract.Result {
	return projectioncontract.Result{
		OriginalDiagnostic: item.OriginalDiagnostic.CloneOriginal(),
		ID:                 item.ID,
		WorkspaceID:        item.WorkspaceID,
		RunID:              item.RunID,
		GoalID:             item.GoalID,
		DoDID:              item.DoDID,
		State:              item.State,
		Title:              item.Title,
		Summary:            item.Summary,
		SourceType:         item.SourceType,
		SourceRef:          item.SourceRef,
		EvidenceRefs:       cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs:       cloneReadModelStringSlice(item.ArtifactRefs),
		DoDAssessment:      cloneResultViewDoDAssessment(item.DoDAssessment),
		SummaryDetail:      cloneResultViewSummary(item.SummaryDetail),
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}

func cloneResultViewSummary(item *workorchresultcontract.ResultSummary) *workorchresultcontract.ResultSummary {
	if item == nil {
		return nil
	}
	ret := &workorchresultcontract.ResultSummary{
		GoalOutcome:           item.GoalOutcome,
		ImplementationSummary: item.ImplementationSummary,
		VerificationSummary:   item.VerificationSummary,
		EvidenceRefs:          cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs:          append([]common.ArtifactID(nil), item.ArtifactRefs...),
		KnownLimitations:      cloneReadModelStringSlice(item.KnownLimitations),
		ResidualRisks:         cloneReadModelStringSlice(item.ResidualRisks),
		NextActions:           cloneReadModelStringSlice(item.NextActions),
		Diagnostics:           item.Diagnostics.Clone(),
	}
	if len(item.DoDItems) > 0 {
		ret.DoDItems = make([]workorchresultcontract.DoDAssessmentItem, 0, len(item.DoDItems))
		for _, dodItem := range item.DoDItems {
			ret.DoDItems = append(ret.DoDItems, cloneResultViewDoDAssessmentItem(dodItem))
		}
	}
	return ret
}

func cloneResultViewDoDAssessment(item *workorchresultcontract.DoDAssessment) *workorchresultcontract.DoDAssessment {
	if item == nil {
		return nil
	}
	ret := &workorchresultcontract.DoDAssessment{
		Satisfied:            item.Satisfied,
		Summary:              item.Summary,
		GoalCoverage:         cloneResultViewDoDCoverage(item.GoalCoverage),
		AcceptanceDimensions: cloneResultViewDoDCoverage(item.AcceptanceDimensions),
		DeliverableCoverage:  cloneResultViewDoDCoverage(item.DeliverableCoverage),
	}
	if len(item.Items) > 0 {
		ret.Items = make([]workorchresultcontract.DoDAssessmentItem, 0, len(item.Items))
		for _, assessmentItem := range item.Items {
			ret.Items = append(ret.Items, cloneResultViewDoDAssessmentItem(assessmentItem))
		}
	}
	return ret
}

func cloneResultViewDoDAssessmentItem(item workorchresultcontract.DoDAssessmentItem) workorchresultcontract.DoDAssessmentItem {
	item.EvidenceRefs = cloneReadModelStringSlice(item.EvidenceRefs)
	return item
}

func cloneResultViewDoDCoverage(item *workorchresultcontract.DoDCoverage) *workorchresultcontract.DoDCoverage {
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
			coverageItem.EvidenceRefs = cloneReadModelStringSlice(coverageItem.EvidenceRefs)
			coverageItem.ArtifactRefs = append([]common.ArtifactID(nil), coverageItem.ArtifactRefs...)
			ret.Items = append(ret.Items, coverageItem)
		}
	}
	return ret
}
