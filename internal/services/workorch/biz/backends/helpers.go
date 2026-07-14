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

func commonSoulBindingStrings(input map[string]common.SoulID) map[string]string {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		if key == "" || value == "" {
			continue
		}
		ret[key] = string(value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func capabilityIDStrings(ids []common.MCPCapabilityID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			out = append(out, string(id))
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func memoryIDStrings(ids []common.MemoryID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			out = append(out, string(id))
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func skillIDStrings(ids []common.SkillID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

func mcpServerIDStrings(ids []common.MCPServerID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}
