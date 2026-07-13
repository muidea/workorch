package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func isLLMUsageEventRecordForRun(item readmodelcore.EventRecord, runID string) bool {
	if item.Topic != "/run/llm_usage/recorded" {
		return false
	}
	return runID == "" || item.RunID == runID
}
