package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MapLLMUsageTracesFromEventRecords(records []projectioncontract.EventRecord, runID string) []projectioncontract.LLMUsageTrace {
	if len(records) == 0 {
		return nil
	}
	ret := make([]projectioncontract.LLMUsageTrace, 0)
	seen := make(map[string]struct{}, len(records))
	for _, item := range records {
		if !isLLMUsageEventRecordForRun(item, runID) {
			continue
		}
		trace, ok := llmUsageTraceFromEventRecord(item)
		if !ok {
			continue
		}
		// Eventstream delivery is at-least-once. A read projection must be
		// replay-safe and must not make one immutable audit event appear as two
		// provider submissions merely because the same EventRecord was read twice.
		identity := llmUsageTraceMergeKey(trace)
		if identity != "" {
			if _, duplicate := seen[identity]; duplicate {
				continue
			}
			seen[identity] = struct{}{}
		}
		ret = append(ret, trace)
	}
	if len(ret) == 0 {
		return nil
	}
	sortLLMUsageTraces(ret)
	return ret
}
