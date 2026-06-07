package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

type resultQualitySummaryFields struct {
	goalOutcome         string
	verificationSummary string
	knownLimitations    []string
	residualRisks       []string
	nextActions         []string
	evidenceRefs        []string
}

func resultQualitySummaryFieldsFor(item readmodelcore.Result) resultQualitySummaryFields {
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
