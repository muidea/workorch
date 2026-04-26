package soul

import (
	"context"
	"slices"

	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	soulservice "github.com/muidea/workorch/internal/modules/kernel/soul/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service struct {
	souls repository.SoulRepository
}

func NewService(souls repository.SoulRepository) *Service {
	return &Service{souls: souls}
}

func (s *Service) CreateSoul(ctx context.Context, cmd soulservice.CreateSoulCommand) error {
	if err := validateSoulCommand(cmd); err != nil {
		return err
	}
	return s.souls.Create(ctx, soulFromCommand(cmd))
}

func (s *Service) UpdateSoul(ctx context.Context, cmd soulservice.UpdateSoulCommand) error {
	if err := validateSoulCommand(cmd); err != nil {
		return err
	}
	return s.souls.Update(ctx, soulFromCommand(cmd))
}

func (s *Service) GetSoul(ctx context.Context, id common.SoulID) (*soulmodel.Soul, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.souls.Get(ctx, id)
}

func (s *Service) ListSouls(ctx context.Context) ([]*soulmodel.Soul, error) {
	return s.souls.List(ctx)
}

func (s *Service) ValidateCapabilitySelection(ctx context.Context, cmd soulservice.ValidateCapabilitySelectionCommand) error {
	if cmd.SoulID == "" {
		return common.ErrInvalidArgument
	}
	soul, err := s.souls.Get(ctx, cmd.SoulID)
	if err != nil {
		return err
	}
	if !allAllowed(cmd.SkillRefs, soul.AllowedSkillRefs) {
		return common.ErrInvalidArgument
	}
	if !allAllowed(cmd.MCPServerRefs, soul.AllowedMCPServerRefs) {
		return common.ErrInvalidArgument
	}
	if !allAllowed(cmd.ToolKinds, soul.AllowedToolKinds) {
		return common.ErrInvalidArgument
	}
	return nil
}

func validateSoulCommand(cmd soulservice.CreateSoulCommand) error {
	if cmd.SoulID == "" || cmd.Name == "" || cmd.SystemInstruction == "" {
		return common.ErrInvalidArgument
	}
	if hasBlank(cmd.BusinessDomains) ||
		hasBlank(cmd.AllowedSkillRefs) ||
		hasBlank(cmd.AllowedMCPServerRefs) ||
		hasBlank(cmd.AllowedToolKinds) {
		return common.ErrInvalidArgument
	}
	return nil
}

func soulFromCommand(cmd soulservice.CreateSoulCommand) *soulmodel.Soul {
	return &soulmodel.Soul{
		ID:                   cmd.SoulID,
		Name:                 cmd.Name,
		Description:          cmd.Description,
		BusinessDomains:      cloneStrings(cmd.BusinessDomains),
		SystemInstruction:    cmd.SystemInstruction,
		PlanningPolicy:       cmd.PlanningPolicy,
		DecisionPolicy:       cmd.DecisionPolicy,
		ApprovalPolicy:       cmd.ApprovalPolicy,
		ValidationPolicy:     cmd.ValidationPolicy,
		AllowedSkillRefs:     cloneStrings(cmd.AllowedSkillRefs),
		AllowedMCPServerRefs: cloneStrings(cmd.AllowedMCPServerRefs),
		AllowedToolKinds:     cloneStrings(cmd.AllowedToolKinds),
		OutputContract:       cmd.OutputContract,
	}
}

func allAllowed(selected []string, allowed []string) bool {
	for _, value := range selected {
		if value == "" || !slices.Contains(allowed, value) {
			return false
		}
	}
	return true
}

func hasBlank(values []string) bool {
	for _, value := range values {
		if value == "" {
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
