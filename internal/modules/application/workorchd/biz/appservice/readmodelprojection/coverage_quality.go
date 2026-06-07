package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func MapDoDEvidence(snapshot readmodelcore.RunSnapshot) []readmodelcore.DoDEvidenceTrace {
	evidenceIndex := runEvidenceRefIndex(snapshot)
	results := runScopedResults(snapshot)
	if len(results) == 0 {
		return nil
	}
	ret := make([]readmodelcore.DoDEvidenceTrace, 0, len(results))
	for _, item := range results {
		if item.DoDAssessment == nil {
			continue
		}
		if snapshot.Run.DoDID != "" && item.DoDID != snapshot.Run.DoDID {
			continue
		}
		ret = append(ret, dodEvidenceTraceFromResult(item, evidenceIndex))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
