package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
)

func stepProgressItem(item presenter.StepView, recoveredHistoricalFailures bool) readmodelcore.ProgressItem {
	return readmodelcore.ProgressItem{
		ID:           item.ID,
		Kind:         "step",
		Title:        firstNonEmptyTimelineString(item.Kind, item.ID),
		State:        RecoveredProgressItemState(item.State, recoveredHistoricalFailures),
		EvidenceRefs: []string{item.ID},
	}
}
