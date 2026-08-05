package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorchresultcontract"

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
