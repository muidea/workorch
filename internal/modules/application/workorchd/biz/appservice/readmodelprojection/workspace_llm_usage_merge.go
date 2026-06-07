package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func MergeWorkspaceLLMUsageTraces(existing []readmodelcore.LLMUsageTrace, latest []readmodelcore.LLMUsageTrace) []readmodelcore.LLMUsageTrace {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]readmodelcore.LLMUsageTrace, 0, len(existing)+len(latest))
	for _, item := range existing {
		key := llmUsageTraceMergeKey(item)
		seen[key] = len(out)
		out = append(out, item)
	}
	for _, item := range latest {
		key := llmUsageTraceMergeKey(item)
		if idx, ok := seen[key]; ok {
			out[idx] = item
			continue
		}
		seen[key] = len(out)
		out = append(out, item)
	}
	sortLLMUsageTraces(out)
	return out
}
