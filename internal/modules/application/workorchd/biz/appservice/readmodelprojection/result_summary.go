package readmodelprojection

import (
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mapResultSummary(item resultmodel.ResultSummary) *projectioncontract.ResultSummary {
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
	ret := &projectioncontract.ResultSummary{
		GoalOutcome:           item.GoalOutcome,
		ImplementationSummary: item.ImplementationSummary,
		VerificationSummary:   item.VerificationSummary,
		EvidenceRefs:          cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs:          resultSummaryArtifactRefs(item),
		KnownLimitations:      cloneReadModelStringSlice(item.KnownLimitations),
		ResidualRisks:         cloneReadModelStringSlice(item.ResidualRisks),
		NextActions:           cloneReadModelStringSlice(item.NextActions),
		Diagnostics:           cloneReadModelMap(item.Diagnostics.Map()),
	}
	ret.DoDItems = resultSummaryDoDItems(item)
	return ret
}
