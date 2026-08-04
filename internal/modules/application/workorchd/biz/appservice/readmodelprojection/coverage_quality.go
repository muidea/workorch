package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MapDoDEvidence(snapshot readmodelcore.RunSnapshotState) []projectioncontract.DoDEvidenceTrace {
	evidenceIndex := runEvidenceRefIndex(snapshot)
	results := runScopedResults(snapshot)
	if len(results) == 0 {
		return nil
	}
	ret := make([]projectioncontract.DoDEvidenceTrace, 0, len(results))
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
