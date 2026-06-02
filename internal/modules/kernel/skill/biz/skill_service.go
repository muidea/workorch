package skill

import (
	"context"
	"strings"

	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	skillservice "github.com/muidea/workorch/internal/modules/kernel/skill/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service struct {
	skills repository.SkillRepository
}

func NewService(skills repository.SkillRepository) *Service {
	return &Service{skills: skills}
}

func (s *Service) CreateSkill(ctx context.Context, cmd skillservice.CreateSkillCommand) error {
	if err := validateSkillCommand(cmd); err != nil {
		return err
	}
	return s.skills.Create(ctx, skillFromCommand(cmd))
}

func (s *Service) UpdateSkill(ctx context.Context, cmd skillservice.UpdateSkillCommand) error {
	if err := validateSkillCommand(cmd); err != nil {
		return err
	}
	return s.skills.Update(ctx, skillFromCommand(cmd))
}

func (s *Service) GetSkill(ctx context.Context, id common.SkillID) (*skillmodel.Skill, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.skills.Get(ctx, id)
}

func (s *Service) ListSkills(ctx context.Context) ([]*skillmodel.Skill, error) {
	return s.skills.List(ctx)
}

func validateSkillCommand(cmd skillservice.CreateSkillCommand) error {
	if cmd.SkillID == "" ||
		strings.TrimSpace(cmd.Name) == "" ||
		strings.TrimSpace(cmd.Version) == "" ||
		strings.TrimSpace(cmd.Instruction) == "" {
		return common.ErrInvalidArgument
	}
	if hasBlank(cmd.TriggerRules) ||
		hasBlank(cmd.ReferenceRefs) ||
		hasBlank(cmd.RequiredToolKinds) ||
		hasBlank(cmd.RequiredMCPServerRefs) ||
		hasBlank(cmd.RequiredCapabilityRefs) ||
		hasBlank(cmd.Constraints) {
		return common.ErrInvalidArgument
	}
	return nil
}

func skillFromCommand(cmd skillservice.CreateSkillCommand) *skillmodel.Skill {
	return &skillmodel.Skill{
		ID:                     cmd.SkillID,
		Name:                   cmd.Name,
		Version:                cmd.Version,
		Description:            cmd.Description,
		TriggerRules:           cloneStrings(cmd.TriggerRules),
		Instruction:            cmd.Instruction,
		ReferenceRefs:          cloneStrings(cmd.ReferenceRefs),
		RequiredToolKinds:      cloneStrings(cmd.RequiredToolKinds),
		RequiredMCPServerRefs:  cloneStrings(cmd.RequiredMCPServerRefs),
		RequiredCapabilityRefs: cloneStrings(cmd.RequiredCapabilityRefs),
		Constraints:            cloneStrings(cmd.Constraints),
	}
}

func hasBlank(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}

func cloneStrings(input []string) []string {
	if input == nil {
		return nil
	}
	return append([]string(nil), input...)
}
