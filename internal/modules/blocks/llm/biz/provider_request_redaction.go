package llm

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

var providerRepairSnapshotContentPaths = []string{
	"input.failureTargetRepairContext[]",
	"input.patchRepairContext[]",
	"input.repairActionPackage.targetSnapshots[]",
}

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
	restoreProviderContractIdentifiers(input, redacted)
	restoreSafeProviderRepairSnapshotContents(path, input, redacted)
	collectRedactionPaths(path, input, redacted, paths)
	return redacted
}

func restoreProviderContractIdentifiers(original map[string]any, redacted map[string]any) {
	if len(original) == 0 || len(redacted) == 0 {
		return
	}
	for key, value := range original {
		redactedValue, ok := redacted[key]
		if !ok {
			continue
		}
		if providerContractIdentifierKey(key) {
			redacted[key] = value
			continue
		}
		originalMap := mapFromAny(value)
		redactedMap := mapFromAny(redactedValue)
		if len(originalMap) > 0 && len(redactedMap) > 0 {
			restoreProviderContractIdentifiers(originalMap, redactedMap)
			redacted[key] = redactedMap
			continue
		}
		originalItems := mapSliceFromAny(value)
		redactedItems := mapSliceFromAny(redactedValue)
		if len(originalItems) > 0 && len(redactedItems) > 0 {
			for idx := range originalItems {
				if idx >= len(redactedItems) {
					break
				}
				restoreProviderContractIdentifiers(originalItems[idx], redactedItems[idx])
			}
			redacted[key] = redactedItems
		}
	}
}

func providerContractIdentifierKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(normalized)
	switch normalized {
	case "coveragetargettoken":
		return true
	default:
		return false
	}
}

func restoreSafeProviderRepairSnapshotContents(path string, original map[string]any, redacted map[string]any) {
	if len(original) == 0 || len(redacted) == 0 {
		return
	}
	restore := func(field string) {
		originalItems := mapSliceFromAny(original[field])
		redactedItems := mapSliceFromAny(redacted[field])
		if len(originalItems) == 0 || len(redactedItems) == 0 {
			return
		}
		restoreProviderRepairSnapshotArray(joinRedactionPath(path, field)+"[]", originalItems, redactedItems)
		redacted[field] = redactedItems
	}
	restore("failureTargetRepairContext")
	restore("patchRepairContext")
	originalPackage := mapFromAny(original["repairActionPackage"])
	redactedPackage := mapFromAny(redacted["repairActionPackage"])
	if len(originalPackage) == 0 || len(redactedPackage) == 0 {
		return
	}
	originalTargets := mapSliceFromAny(originalPackage["targetSnapshots"])
	redactedTargets := mapSliceFromAny(redactedPackage["targetSnapshots"])
	if len(originalTargets) == 0 || len(redactedTargets) == 0 {
		return
	}
	restoreProviderRepairSnapshotArray(joinRedactionPath(joinRedactionPath(path, "repairActionPackage"), "targetSnapshots")+"[]", originalTargets, redactedTargets)
	redactedPackage["targetSnapshots"] = redactedTargets
	redacted["repairActionPackage"] = redactedPackage
}

func restoreProviderRepairSnapshotArray(path string, original []map[string]any, redacted []map[string]any) {
	for idx := range original {
		if idx >= len(redacted) {
			break
		}
		if !providerRepairSnapshotContentPreserved(path, original[idx]) {
			continue
		}
		content := rawStringValue(original[idx]["content"])
		if content == "" {
			continue
		}
		redacted[idx]["content"] = content
	}
}

func providerRepairSnapshotContentPreserved(path string, item map[string]any) bool {
	if len(item) == 0 || !providerRepairSnapshotPathAllowed(path) {
		return false
	}
	if strings.TrimSpace(rawStringValue(item["content"])) == "" || strings.TrimSpace(stringValue(item["contentHash"])) == "" {
		return false
	}
	return boolValue(item["contentComplete"]) &&
		boolValue(item["safeForFileWrite"]) &&
		!boolValue(item["redacted"]) &&
		!boolValue(item["truncated"]) &&
		!boolValue(item["isBinary"])
}

func rawStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func providerRepairSnapshotPathAllowed(path string) bool {
	path = strings.TrimSpace(path)
	for _, candidate := range providerRepairSnapshotContentPaths {
		if path == candidate {
			return true
		}
	}
	return false
}

func mapFromAny(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		if value == nil {
			return nil
		}
		payload, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		var ret map[string]any
		if err := json.Unmarshal(payload, &ret); err != nil {
			return nil
		}
		return ret
	}
}

func mapSliceFromAny(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped := mapFromAny(item); len(mapped) > 0 {
				ret = append(ret, mapped)
			}
		}
		return ret
	default:
		return nil
	}
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
	default:
		if beforeItems, ok := redactionPathSliceFromAny(beforeTyped); ok {
			afterItems, _ := redactionPathSliceFromAny(after)
			for idx, beforeValue := range beforeItems {
				afterValue := any(nil)
				if idx < len(afterItems) {
					afterValue = afterItems[idx]
				}
				collectRedactionPaths(path+"[]", beforeValue, afterValue, paths)
			}
			return
		}
		*paths = append(*paths, path)
	}
}

func redactionPathSliceFromAny(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []map[string]any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, item)
		}
		return ret, true
	case []string:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, item)
		}
		return ret, true
	default:
		return nil, false
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
