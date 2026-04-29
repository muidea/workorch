package soul

import (
	"context"
	"errors"
	"strings"
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
		SoulMD:               "# Engineering Delivery\n\nPlan, execute, and cite evidence before completion.\n\n## Output Contract\n\nReturn LLMExecutionDecision JSON.\n",
		AllowedSkillRefs:     []string{"skill-go"},
		AllowedMCPServerRefs: []string{"mcp-repo"},
		AllowedToolKinds:     []string{"shell/command", "patch/apply"},
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
	}); !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "skill-python") {
		t.Fatalf("validate disallowed skill err = %v, want invalid argument", err)
	}

	if err := service.ValidateCapabilitySelection(ctx, soulservice.ValidateCapabilitySelectionCommand{
		SoulID:        cmd.SoulID,
		MCPServerRefs: []string{"mcp-unapproved"},
	}); !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "mcp-unapproved") {
		t.Fatalf("validate disallowed MCP err = %v, want invalid argument with MCP ref", err)
	}

	if err := service.ValidateCapabilitySelection(ctx, soulservice.ValidateCapabilitySelectionCommand{
		SoulID:    cmd.SoulID,
		ToolKinds: []string{"shell/ssh"},
	}); !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "shell/ssh") {
		t.Fatalf("validate disallowed tool kind err = %v, want invalid argument with tool kind", err)
	}

	if err := service.ValidateCapabilitySelection(ctx, soulservice.ValidateCapabilitySelectionCommand{
		SoulID:    cmd.SoulID,
		SkillRefs: []string{" "},
	}); !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "selected skill is blank") {
		t.Fatalf("validate blank skill err = %v, want invalid argument with blank skill reason", err)
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
		SoulID:           common.SoulID("soul-blank-skill"),
		Name:             "blank skill",
		SoulMD:           "# Blank Skill\n\ninstruction\n",
		AllowedSkillRefs: []string{""},
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create blank skill ref err = %v, want invalid argument", err)
	}
}

func TestServiceDeleteSoul(t *testing.T) {
	ctx := context.Background()
	service := NewService(memory.NewSoulRepository())

	cmd := soulservice.CreateSoulCommand{
		SoulID: common.SoulID("soul-delete"),
		Name:   "delete soul",
		SoulMD: "# Delete Soul\n",
	}
	if err := service.CreateSoul(ctx, cmd); err != nil {
		t.Fatalf("create soul: %v", err)
	}
	if err := service.DeleteSoul(ctx, cmd.SoulID); err != nil {
		t.Fatalf("delete soul: %v", err)
	}
	if _, err := service.GetSoul(ctx, cmd.SoulID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get deleted soul err = %v, want not found", err)
	}
	if err := service.DeleteSoul(ctx, ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("delete blank soul err = %v, want invalid argument", err)
	}
	if err := service.DeleteSoul(ctx, cmd.SoulID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("delete missing soul err = %v, want not found", err)
	}
}
