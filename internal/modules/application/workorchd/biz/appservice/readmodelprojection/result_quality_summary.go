package readmodelprojection

import (
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type resultQualitySummaryFields struct {
	goalOutcome         string
	verificationSummary string
	knownLimitations    []string
	residualRisks       []string
	nextActions         []string
	evidenceRefs        []string
}

func resultQualitySummaryFieldsFor(item projectioncontract.Result) resultQualitySummaryFields {
	ret := resultQualitySummaryFields{}
	if item.SummaryDetail != nil {
		ret.goalOutcome = strings.TrimSpace(item.SummaryDetail.GoalOutcome)
		ret.verificationSummary = strings.TrimSpace(item.SummaryDetail.VerificationSummary)
		ret.knownLimitations = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.KnownLimitations))
		ret.residualRisks = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.ResidualRisks))
		ret.nextActions = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.NextActions))
		ret.evidenceRefs = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.EvidenceRefs))
	}
	if len(ret.evidenceRefs) == 0 {
		ret.evidenceRefs = compactUniqueStrings(cloneReadModelStringSlice(item.EvidenceRefs))
	}
	return ret
}
