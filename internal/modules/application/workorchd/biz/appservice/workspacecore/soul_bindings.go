package workspacecore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func SoulBindingsFromView(input map[string]string) map[string]common.SoulID {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]common.SoulID, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		ret[key] = common.SoulID(value)
	}
	return ret
}
