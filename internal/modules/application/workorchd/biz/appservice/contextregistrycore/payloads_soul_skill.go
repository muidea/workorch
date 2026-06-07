package contextregistrycore

import (
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func SoulFromPayload(payload map[string]any, key string) (Soul, error) {
	item, ok := payload[key].(*soulmodel.Soul)
	if !ok {
		return Soul{}, common.ErrInvalidArgument
	}
	return MapSoul(item), nil
}

func SoulsFromPayload(payload map[string]any, key string) ([]Soul, error) {
	items, ok := payload[key].([]*soulmodel.Soul)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]Soul, 0, len(items))
	for _, item := range items {
		ret = append(ret, MapSoul(item))
	}
	return ret, nil
}

func SkillFromPayload(payload map[string]any, key string) (Skill, error) {
	item, ok := payload[key].(*skillmodel.Skill)
	if !ok {
		return Skill{}, common.ErrInvalidArgument
	}
	return MapRepositorySkill(item), nil
}

func SkillsFromPayload(payload map[string]any, key string) ([]Skill, error) {
	items, ok := payload[key].([]*skillmodel.Skill)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]Skill, 0, len(items))
	for _, item := range items {
		ret = append(ret, MapRepositorySkill(item))
	}
	return ret, nil
}
