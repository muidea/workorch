package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func MapResultQuality(snapshot readmodelcore.RunSnapshot) []readmodelcore.ResultQualityTrace {
	results := runScopedResults(snapshot)
	if len(results) == 0 {
		return nil
	}
	evidenceIndex := runEvidenceRefIndex(snapshot)
	weakValidationRefs := weakValidationEvidenceRefIndex(snapshot)
	requiredDoDLeafItems := runRequiredDoDLeafItems(snapshot)
	ret := make([]readmodelcore.ResultQualityTrace, 0, len(results))
	for _, item := range results {
		quality := newResultQualityTrace(item)
		summary := resultQualitySummaryFieldsFor(item)
		quality.EvidenceRefs = summary.evidenceRefs
		checkResultQualitySummaryFields(&quality, summary)
		checkResultQualityDoDAssessment(&quality, item, requiredDoDLeafItems, evidenceIndex, weakValidationRefs, summary.evidenceRefs)
		ret = append(ret, quality)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
