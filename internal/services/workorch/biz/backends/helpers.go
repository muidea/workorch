package backend

import (
	"github.com/muidea/workorch/internal/pkg/common"
)

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func commonSoulBindings(input map[string]string) map[string]common.SoulID {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]common.SoulID, len(input))
	for key, value := range input {
		if value == "" {
			continue
		}
		ret[key] = common.SoulID(value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func cloneTemplateEvidencePolicies(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		cloned := make(map[string]any, len(item))
		for key, value := range item {
			cloned[key] = cloneTemplateEvidencePolicyValue(value)
		}
		ret = append(ret, cloned)
	}
	return ret
}

func cloneTemplateEvidencePolicyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, value := range typed {
			cloned[key] = cloneTemplateEvidencePolicyValue(value)
		}
		return cloned
	case []map[string]any:
		return cloneTemplateEvidencePolicies(typed)
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneTemplateEvidencePolicyValue(item))
		}
		return ret
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}
