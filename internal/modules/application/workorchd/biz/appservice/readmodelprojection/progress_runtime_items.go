package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func stepProgressItem(item readmodelcore.StepView, recoveredHistoricalFailures bool) projectioncontract.ProgressItem {
	return projectioncontract.ProgressItem{
		ID:           item.ID,
		Kind:         "step",
		Title:        firstNonEmptyTimelineString(item.Kind, item.ID),
		State:        RecoveredProgressItemState(item.State, recoveredHistoricalFailures),
		EvidenceRefs: []string{item.ID},
	}
}
