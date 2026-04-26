package soul

import (
	"context"
	"errors"
	"testing"

	soulservice "github.com/muidea/workorch/internal/modules/kernel/soul/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestServiceCreateSoulAndValidateCapabilitySelection(t *testing.T) {
	ctx := context.Background()
	service := NewService(memory.NewSoulRepository())

	cmd := soulservice.CreateSoulCommand{
		SoulID:               common.SoulID("soul-engineering"),
		Name:                 "engineering delivery",
		Description:          "delivery-focused engineering soul",
		BusinessDomains:      []string{"engineering"},
		SystemInstruction:    "Plan, execute, and cite evidence before completion.",
		AllowedSkillRefs:     []string{"skill-go"},
		AllowedMCPServerRefs: []string{"mcp-repo"},
		AllowedToolKinds:     []string{"shell/command", "patch/apply"},
		OutputContract:       "LLMExecutionDecision",
	}
	if err := service.CreateSoul(ctx, cmd); err != nil {
		t.Fatalf("create soul: %v", err)
	}

	got, err := service.GetSoul(ctx, cmd.SoulID)
	if err != nil {
		t.Fatalf("get soul: %v", err)
	}
	if got.Name != cmd.Name || got.AllowedSkillRefs[0] != "skill-go" {
		t.Fatalf("soul = %+v, want created command values", got)
	}

	if err := service.ValidateCapabilitySelection(ctx, soulservice.ValidateCapabilitySelectionCommand{
		SoulID:        cmd.SoulID,
		SkillRefs:     []string{"skill-go"},
		MCPServerRefs: []string{"mcp-repo"},
		ToolKinds:     []string{"shell/command"},
	}); err != nil {
		t.Fatalf("validate allowed capability selection: %v", err)
	}

	if err := service.ValidateCapabilitySelection(ctx, soulservice.ValidateCapabilitySelectionCommand{
		SoulID:    cmd.SoulID,
		SkillRefs: []string{"skill-python"},
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("validate disallowed skill err = %v, want invalid argument", err)
	}
}

func TestServiceRejectsInvalidSoul(t *testing.T) {
	ctx := context.Background()
	service := NewService(memory.NewSoulRepository())

	if err := service.CreateSoul(ctx, soulservice.CreateSoulCommand{
		SoulID: common.SoulID("soul-invalid"),
		Name:   "invalid",
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create invalid soul err = %v, want invalid argument", err)
	}

	if err := service.CreateSoul(ctx, soulservice.CreateSoulCommand{
		SoulID:            common.SoulID("soul-blank-skill"),
		Name:              "blank skill",
		SystemInstruction: "instruction",
		AllowedSkillRefs:  []string{""},
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create blank skill ref err = %v, want invalid argument", err)
	}
}
