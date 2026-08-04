package contextregistrycore

import (
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func SoulFromPayload(payload map[string]any, key string) (workorchmanagementcontract.Soul, error) {
	item, ok := payload[key].(*soulmodel.Soul)
	if !ok {
		return workorchmanagementcontract.Soul{}, common.ErrInvalidArgument
	}
	return MapSoul(item), nil
}

func SoulsFromPayload(payload map[string]any, key string) ([]workorchmanagementcontract.Soul, error) {
	items, ok := payload[key].([]*soulmodel.Soul)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]workorchmanagementcontract.Soul, 0, len(items))
	for _, item := range items {
		ret = append(ret, MapSoul(item))
	}
	return ret, nil
}

func SkillFromPayload(payload map[string]any, key string) (workorchmanagementcontract.Skill, error) {
	item, ok := payload[key].(*skillmodel.Skill)
	if !ok {
		return workorchmanagementcontract.Skill{}, common.ErrInvalidArgument
	}
	return MapRepositorySkill(item), nil
}

func SkillsFromPayload(payload map[string]any, key string) ([]workorchmanagementcontract.Skill, error) {
	items, ok := payload[key].([]*skillmodel.Skill)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]workorchmanagementcontract.Skill, 0, len(items))
	for _, item := range items {
		ret = append(ret, MapRepositorySkill(item))
	}
	return ret, nil
}
