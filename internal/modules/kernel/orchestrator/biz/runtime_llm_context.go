package orchestrator

import (
	"sort"
	"strings"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

const (
	maxLLMContextStringBytes       = 4096
	maxLLMContextEvidenceTextBytes = 1200
	maxLLMContextSliceItems        = 20
	maxLLMContextMapEntries        = 32
)

func compactStepInputForLLMContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := compactLLMContextMap(input, 0)
	delete(ret, "executionContext")
	delete(ret, "failedStepInput")
	delete(ret, "failedStepOutput")
	delete(ret, "previousStepOutput")
	delete(ret, stepInputKeyExecutionContextPackage)
	delete(ret, stepInputKeyWorkspaceFileManifest)
	delete(ret, stepOutputKeyExecutionGraph)
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		ret["repairActionPackage"] = compactLLMContextMap(pkg, 0)
	}
	return ret
}

func compactStepOutputForLLMContext(output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}
	ret := compactLLMContextMap(output, 0)
	return stripRuntimeLLMBinding(ret)
}

func compactWorkspaceFileManifest(manifest []map[string]any) []map[string]any {
	if len(manifest) == 0 {
		return nil
	}
	limit := min(len(manifest), maxLLMContextSliceItems)
	ret := make([]map[string]any, 0, limit)
	for idx := 0; idx < limit; idx++ {
		ret = append(ret, compactLLMContextMap(manifest[idx], 0))
	}
	return ret
}

func compactExecutionGraphForLLMContext(graph runtimemodel.BlockExecutionGraph) runtimemodel.BlockExecutionGraph {
	if len(graph.Nodes) == 0 {
		return graph
	}
	nodes := make([]runtimemodel.BlockExecutionNode, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		compactNode := runtimemodel.BlockExecutionNode{
			ID:   strings.TrimSpace(node.ID),
			Kind: strings.TrimSpace(node.Kind),
		}
		if len(node.DependsOn) > 0 {
			limit := min(len(node.DependsOn), 10)
			compactNode.DependsOn = append([]string(nil), node.DependsOn[:limit]...)
		}
		if branch := strings.TrimSpace(node.Branch); branch != "" {
			compactNode.Branch = truncateLLMContextString(branch)
		}
		if len(node.Input) > 0 {
			compactNode.Input = compactLLMContextMap(node.Input, 0)
		}
		nodes = append(nodes, compactNode)
	}
	return runtimemodel.BlockExecutionGraph{Nodes: nodes}
}

func compactLLMContextMap(input map[string]any, depth int) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, min(len(input), maxLLMContextMapEntries))
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(ret) >= maxLLMContextMapEntries {
			break
		}
		if shouldSkipLLMContextKey(key) {
			continue
		}
		if isLargeEvidenceTextKey(key) {
			ret[key] = compactLLMContextEvidenceText(input[key])
			continue
		}
		ret[key] = compactLLMContextValue(input[key], depth+1)
	}
	return ret
}

func compactLLMContextValue(value any, depth int) any {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return truncateLLMContextString(v)
	case map[string]any:
		if depth >= 3 {
			return compactLLMContextLeafSummary(v)
		}
		return compactLLMContextMap(v, depth)
	case []map[string]any:
		if depth >= 3 {
			return compactLLMContextLeafSummary(map[string]any{"items": len(v)})
		}
		limit := min(len(v), maxLLMContextSliceItems)
		ret := make([]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			ret = append(ret, compactLLMContextMap(v[idx], depth))
		}
		return ret
	case []any:
		if depth >= 3 {
			return compactLLMContextLeafSummary(map[string]any{"items": len(v)})
		}
		limit := min(len(v), maxLLMContextSliceItems)
		ret := make([]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			ret = append(ret, compactLLMContextValue(v[idx], depth))
		}
		return ret
	case []string:
		limit := min(len(v), maxLLMContextSliceItems)
		ret := make([]string, 0, limit)
		for idx := 0; idx < limit; idx++ {
			ret = append(ret, truncateLLMContextString(v[idx]))
		}
		return ret
	default:
		return v
	}
}

func compactLLMContextLeafSummary(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, min(len(input), maxLLMContextMapEntries))
	for key, value := range input {
		if shouldSkipLLMContextKey(key) {
			continue
		}
		switch v := value.(type) {
		case string:
			ret[key] = truncateLLMContextString(v)
		case []string:
			ret[key] = len(v)
		case []any:
			ret[key] = len(v)
		case []map[string]any:
			ret[key] = len(v)
		case map[string]any:
			ret[key] = len(v)
		default:
			ret[key] = v
		}
		if len(ret) >= maxLLMContextMapEntries {
			break
		}
	}
	return ret
}

func shouldSkipLLMContextKey(key string) bool {
	switch key {
	case "executionContext",
		"failedStepInput",
		"failedStepOutput",
		"llmProviderId",
		"previousStepOutput",
		"provider",
		"providerId",
		stepInputKeyExecutionContextPackage,
		stepInputKeyWorkspaceFileManifest:
		return true
	default:
		return false
	}
}

func truncateLLMContextString(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxLLMContextStringBytes {
		return value
	}
	return value[:maxLLMContextStringBytes] + "..."
}

func isLargeEvidenceTextKey(key string) bool {
	switch strings.TrimSpace(key) {
	case "stdout", "stderr":
		return true
	default:
		return false
	}
}

func compactLLMContextEvidenceText(value any) any {
	text, ok := value.(string)
	if !ok {
		return compactLLMContextValue(value, 1)
	}
	text = strings.TrimSpace(text)
	if len(text) <= maxLLMContextEvidenceTextBytes {
		return text
	}
	return map[string]any{
		"textPreview": text[:maxLLMContextEvidenceTextBytes] + "...",
		"bytes":       len(text),
		"truncated":   true,
	}
}

func stripRuntimeLLMBinding(input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	delete(input, "provider")
	delete(input, "providerId")
	delete(input, "llmProviderId")
	if budget := mapFromValue(input["contextBudget"]); len(budget) > 0 {
		delete(budget, "providerId")
		delete(budget, "model")
		delete(budget, "profile")
		input["contextBudget"] = budget
	}
	if usage := mapFromValue(input["llmUsage"]); len(usage) > 0 {
		delete(usage, "provider")
		delete(usage, "providerId")
		delete(usage, "model")
		if budget := mapFromValue(usage["contextBudget"]); len(budget) > 0 {
			delete(budget, "providerId")
			delete(budget, "model")
			delete(budget, "profile")
			usage["contextBudget"] = budget
		}
		input["llmUsage"] = usage
	}
	return input
}
