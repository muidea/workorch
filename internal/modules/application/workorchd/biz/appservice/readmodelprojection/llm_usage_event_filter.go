package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func isLLMUsageEventRecordForRun(item projectioncontract.EventRecord, runID string) bool {
	if item.Topic != "/run/llm_usage/recorded" {
		return false
	}
	return runID == "" || item.RunID == runID
}
