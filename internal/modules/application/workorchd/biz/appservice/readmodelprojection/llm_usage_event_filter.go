package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/events"
)

func isLLMUsageEventRecordForRun(item readmodelcore.EventRecord, runID string) bool {
	if item.Topic != events.TopicRunLLMUsageRecorded {
		return false
	}
	return runID == "" || item.RunID == runID
}
