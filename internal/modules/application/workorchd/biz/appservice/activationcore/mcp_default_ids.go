package activationcore

import (
	"slices"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
)

func DefaultMCPServerIDsFromContextHints(contextHints map[string]any) []string {
	return DefaultIDsFromContextHints(contextHints, "defaultMCPServerIds")
}

func DefaultCapabilityIDsFromContextHints(contextHints map[string]any) []string {
	return DefaultIDsFromContextHints(contextHints, "defaultCapabilityIds")
}

func DefaultIDsFromContextHints(contextHints map[string]any, key string) []string {
	if len(contextHints) == 0 {
		return nil
	}
	ret := make([]string, 0)
	for _, value := range contextHints {
		config, ok := value.(map[string]any)
		if !ok {
			continue
		}
		ret = AppendDefaultIDs(ret, NormalizeStringIDs(stringSliceFromAny(config[key])))
	}
	return ret
}

func AppendDefaultIDs(base []string, defaults []string) []string {
	ret := contextregistrycore.CloneStrings(base)
	for _, id := range defaults {
		id = strings.TrimSpace(id)
		if id == "" || slices.Contains(ret, id) {
			continue
		}
		ret = append(ret, id)
	}
	return ret
}

func NormalizeStringIDs(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	ret := make([]string, 0, len(input))
	seen := map[string]struct{}{}
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}
