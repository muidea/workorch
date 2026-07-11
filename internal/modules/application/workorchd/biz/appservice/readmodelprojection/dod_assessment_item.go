package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
)

func mapDoDAssessmentItem(item resultmodel.DoDAssessmentItem) readmodelcore.DoDAssessmentItem {
	return readmodelcore.DoDAssessmentItem{
		Item:          item.Item,
		State:         item.State,
		EvidenceRefs:  cloneReadModelStringSlice(item.EvidenceRefs),
		FailureReason: item.FailureReason,
		Confidence:    item.Confidence,
		SourceNote:    item.SourceNote,
	}
}
