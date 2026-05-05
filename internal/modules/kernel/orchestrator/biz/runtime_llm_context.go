package orchestrator

import (
	"sort"
	"strings"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

const (
	maxLLMContextStringBytes = 4096
	maxLLMContextSliceItems  = 20
	maxLLMContextMapEntries  = 32
)

func compactStepInputForLLMContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := compactLLMContextMap(input, 0)
	delete(ret, stepInputKeyExecutionContextPackage)
	delete(ret, stepInputKeyWorkspaceFileManifest)
	return ret
}

func compactStepOutputForLLMContext(output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}
	return compactLLMContextMap(output, 0)
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
	case stepInputKeyExecutionContextPackage, stepInputKeyWorkspaceFileManifest:
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
