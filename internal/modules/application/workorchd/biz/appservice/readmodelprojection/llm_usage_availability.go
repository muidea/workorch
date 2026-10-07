package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

// complete 仅表示该次有界查询未触及上限且所有返回记录均可解析，不代表全部历史。
func summarizeQueriedLLMUsage(traces []projectioncontract.LLMUsageTrace, observedRecords int, loaded, limited bool, limit int, scope string) projectioncontract.LLMUsageSummary {
	ret := summarizeLLMUsage(traces)
	ret.Scope = scope
	ret.RecordLimit = limit
	ret.ObservedRecords = observedRecords
	ret.Availability = "not_loaded"
	if !loaded {
		return ret
	}
	ret.Availability = "complete"
	if limited || (limit > 0 && observedRecords >= limit) || observedRecords != len(traces) {
		ret.Availability = "partial"
	}
	return ret
}

func mergeWorkspaceLLMUsageSummary(traces []projectioncontract.LLMUsageTrace, workspace, run projectioncontract.LLMUsageSummary) projectioncontract.LLMUsageSummary {
	ret := summarizeLLMUsage(traces)
	ret.Scope = "workspace"
	ret.RecordLimit = workspace.RecordLimit
	ret.ObservedRecords = workspace.ObservedRecords
	ret.Availability = workspace.Availability
	if ret.Availability == "" {
		ret.Availability = "not_loaded"
	}
	// Run 查询不包含创建期。追加 Run 数据不能提高 Workspace 查询的完整性。
	if run.Availability != "" && run.Availability != "not_loaded" {
		if workspace.Availability != "complete" || run.Availability == "partial" || len(traces) > workspace.EntryTotal {
			ret.Availability = "partial"
		}
	}
	return ret
}
