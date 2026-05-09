package llm

import (
	"reflect"
	"sort"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func redactProviderRequestForLLM(request llmmodels.ProviderRequest) llmmodels.ProviderRequest {
	paths := make([]string, 0)
	request.Input = redactProviderMap("input", request.Input, &paths)
	if len(request.TraceRefs) > 0 {
		redacted := make(map[string]string, len(request.TraceRefs))
		for key, value := range request.TraceRefs {
			next := common.RedactSensitiveString(value)
			if next != value {
				paths = append(paths, "traceRefs."+key)
			}
			redacted[key] = next
		}
		request.TraceRefs = redacted
	}
	if request.PromptContext != nil {
		context := *request.PromptContext
		if len(context.Sections) > 0 {
			context.Sections = append([]llmmodels.PromptSection(nil), context.Sections...)
			for idx := range context.Sections {
				next := common.RedactSensitiveString(context.Sections[idx].Content)
				if next != context.Sections[idx].Content {
					sectionID := strings.TrimSpace(context.Sections[idx].ID)
					if sectionID == "" {
						sectionID = "section"
					}
					paths = append(paths, "promptContext.sections."+sectionID)
				}
				context.Sections[idx].Content = next
			}
		}
		if len(context.Turns) > 0 {
			context.Turns = append([]llmmodels.PromptTurn(nil), context.Turns...)
			for idx := range context.Turns {
				next := common.RedactSensitiveString(context.Turns[idx].Content)
				if next != context.Turns[idx].Content {
					turnID := context.Turns[idx].SectionID
					if strings.TrimSpace(turnID) == "" {
						turnID = context.Turns[idx].Role
					}
					paths = append(paths, "promptContext.turns."+turnID)
				}
				context.Turns[idx].Content = next
			}
		}
		request.PromptContext = &context
	}
	if len(paths) > 0 {
		if request.ContextBudget == nil {
			request.ContextBudget = &llmmodels.ProviderContextBudget{}
		}
		request.ContextBudget.RedactionApplied = true
		request.ContextBudget.RedactedPaths = uniqueStrings(append(request.ContextBudget.RedactedPaths, uniqueRedactionPaths(paths)...))
	}
	return request
}

func redactProviderMap(path string, input map[string]any, paths *[]string) map[string]any {
	if len(input) == 0 {
		return nil
	}
	redacted := common.RedactSensitiveMap(input)
	collectRedactionPaths(path, input, redacted, paths)
	return redacted
}

func collectRedactionPaths(path string, before any, after any, paths *[]string) {
	if reflect.DeepEqual(before, after) {
		return
	}
	switch beforeTyped := before.(type) {
	case map[string]any:
		afterTyped, _ := after.(map[string]any)
		for key, beforeValue := range beforeTyped {
			nextPath := joinRedactionPath(path, key)
			afterValue := any(nil)
			if afterTyped != nil {
				afterValue = afterTyped[key]
			}
			collectRedactionPaths(nextPath, beforeValue, afterValue, paths)
		}
	case []any:
		afterTyped, _ := after.([]any)
		for idx, beforeValue := range beforeTyped {
			afterValue := any(nil)
			if idx < len(afterTyped) {
				afterValue = afterTyped[idx]
			}
			collectRedactionPaths(path+"[]", beforeValue, afterValue, paths)
		}
	case []map[string]any:
		afterTyped, _ := after.([]map[string]any)
		for idx, beforeValue := range beforeTyped {
			afterValue := map[string]any(nil)
			if idx < len(afterTyped) {
				afterValue = afterTyped[idx]
			}
			collectRedactionPaths(path+"[]", beforeValue, afterValue, paths)
		}
	case []string:
		afterTyped, _ := after.([]string)
		for idx, beforeValue := range beforeTyped {
			afterValue := ""
			if idx < len(afterTyped) {
				afterValue = afterTyped[idx]
			}
			collectRedactionPaths(path+"[]", beforeValue, afterValue, paths)
		}
	default:
		*paths = append(*paths, path)
	}
}

func joinRedactionPath(base string, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "field"
	}
	if base == "" {
		return key
	}
	return base + "." + key
}

func uniqueRedactionPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		ret = append(ret, path)
	}
	sort.Strings(ret)
	return ret
}
