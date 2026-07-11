package repository

import (
	"context"

	skill "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type SkillRepository interface {
	Create(context.Context, *skill.Skill) error
	Get(context.Context, common.SkillID) (*skill.Skill, error)
	List(context.Context) ([]*skill.Skill, error)
	Update(context.Context, *skill.Skill) error
}
