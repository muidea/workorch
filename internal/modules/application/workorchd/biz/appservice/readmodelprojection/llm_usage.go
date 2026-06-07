package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func MapLLMUsageTracesFromEventRecords(records []readmodelcore.EventRecord, runID string) []readmodelcore.LLMUsageTrace {
	if len(records) == 0 {
		return nil
	}
	ret := make([]readmodelcore.LLMUsageTrace, 0)
	for _, item := range records {
		if !isLLMUsageEventRecordForRun(item, runID) {
			continue
		}
		trace, ok := llmUsageTraceFromEventRecord(item)
		if !ok {
			continue
		}
		ret = append(ret, trace)
	}
	if len(ret) == 0 {
		return nil
	}
	sortLLMUsageTraces(ret)
	return ret
}
