package appmodelcore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func NormalizeSoulBindings(input map[string]string) (map[string]common.SoulID, error) {
	if len(input) == 0 {
		return nil, nil
	}
	ret := map[string]common.SoulID{}
	for key, value := range input {
		stage := strings.TrimSpace(key)
		if !IsSoulBindingStage(stage) {
			return nil, common.ErrInvalidArgument
		}
		soulID := common.SoulID(strings.TrimSpace(value))
		if soulID == "" {
			continue
		}
		ret[stage] = soulID
	}
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func MapSoulBindingIDs(input map[string]common.SoulID) map[string]string {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]string{}
	for _, stage := range SoulBindingStages() {
		if value := input[stage]; value != "" {
			ret[stage] = string(value)
		}
	}
	return ret
}
