package soul

import (
	"context"
	"fmt"
	"slices"
	"strings"

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
		return fmt.Errorf("%w: soulId is required", common.ErrInvalidArgument)
	}
	soul, err := s.souls.Get(ctx, cmd.SoulID)
	if err != nil {
		return err
	}
	if value, reason, ok := firstInvalidSelection(cmd.SkillRefs, soul.AllowedSkillRefs); ok {
		return invalidCapabilitySelectionError(cmd.SoulID, "skill", value, reason)
	}
	if value, reason, ok := firstInvalidSelection(cmd.MCPServerRefs, soul.AllowedMCPServerRefs); ok {
		return invalidCapabilitySelectionError(cmd.SoulID, "MCP server", value, reason)
	}
	if value, reason, ok := firstInvalidSelection(cmd.ToolKinds, soul.AllowedToolKinds); ok {
		return invalidCapabilitySelectionError(cmd.SoulID, "tool kind", value, reason)
	}
	return nil
}

func validateSoulCommand(cmd soulservice.CreateSoulCommand) error {
	if cmd.SoulID == "" {
		return fmt.Errorf("%w: soulId is required", common.ErrInvalidArgument)
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return fmt.Errorf("%w: soul name is required", common.ErrInvalidArgument)
	}
	if strings.TrimSpace(cmd.SoulMD) == "" {
		return fmt.Errorf("%w: soulMd is required", common.ErrInvalidArgument)
	}
	if hasBlank(cmd.BusinessDomains) {
		return fmt.Errorf("%w: businessDomains contains blank value", common.ErrInvalidArgument)
	}
	if hasBlank(cmd.AllowedSkillRefs) {
		return fmt.Errorf("%w: allowedSkillRefs contains blank value", common.ErrInvalidArgument)
	}
	if hasBlank(cmd.AllowedMCPServerRefs) {
		return fmt.Errorf("%w: allowedMCPServerRefs contains blank value", common.ErrInvalidArgument)
	}
	if hasBlank(cmd.AllowedToolKinds) {
		return fmt.Errorf("%w: allowedToolKinds contains blank value", common.ErrInvalidArgument)
	}
	return nil
}

func soulFromCommand(cmd soulservice.CreateSoulCommand) *soulmodel.Soul {
	return &soulmodel.Soul{
		ID:                   cmd.SoulID,
		Name:                 cmd.Name,
		Description:          cmd.Description,
		BusinessDomains:      cloneStrings(cmd.BusinessDomains),
		SoulMD:               cmd.SoulMD,
		AllowedSkillRefs:     cloneStrings(cmd.AllowedSkillRefs),
		AllowedMCPServerRefs: cloneStrings(cmd.AllowedMCPServerRefs),
		AllowedToolKinds:     cloneStrings(cmd.AllowedToolKinds),
	}
}

func firstInvalidSelection(selected []string, allowed []string) (string, string, bool) {
	for _, value := range selected {
		if strings.TrimSpace(value) == "" {
			return value, "blank", true
		}
		if !slices.Contains(allowed, value) {
			return value, "disallowed", true
		}
	}
	return "", "", false
}

func invalidCapabilitySelectionError(soulID common.SoulID, capability string, value string, reason string) error {
	if reason == "blank" {
		return fmt.Errorf("%w: soul %s selected %s is blank", common.ErrInvalidArgument, soulID, capability)
	}
	return fmt.Errorf("%w: soul %s does not allow %s %q", common.ErrInvalidArgument, soulID, capability, value)
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
