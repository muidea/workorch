package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
)

func isLLMUsageEventRecordForRun(item readmodelcore.EventRecord, runID string) bool {
	if item.Topic != runevents.TopicRunLLMUsageRecorded {
		return false
	}
	return runID == "" || item.RunID == runID
}
