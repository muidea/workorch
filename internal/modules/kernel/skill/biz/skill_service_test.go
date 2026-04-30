package skill

import (
	"context"
	"errors"
	"testing"

	skillservice "github.com/muidea/workorch/internal/modules/kernel/skill/service"
	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestServiceCreateAndListSkill(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewSkillRepository())

	cmd := skillservice.CreateSkillCommand{
		SkillID:                common.SkillID("skill-local-go-service"),
		Name:                   "local go service",
		Version:                "v1",
		TriggerRules:           []string{"go", "service"},
		Instruction:            "Use small verified steps.",
		ReferenceRefs:          []string{"docs/structure.md"},
		RequiredToolKinds:      []string{"shell/command"},
		RequiredMCPServerRefs:  []string{"mcp-local-context"},
		RequiredCapabilityRefs: []string{"mcp-local-context:tool:repo.summary"},
		Constraints:            []string{"cite evidence"},
	}
	if err := service.CreateSkill(ctx, cmd); err != nil {
		t.Fatalf("create skill: %v", err)
	}
	got, err := service.GetSkill(ctx, cmd.SkillID)
	if err != nil {
		t.Fatalf("get skill: %v", err)
	}
	if got.Name != cmd.Name || got.RequiredMCPServerRefs[0] != "mcp-local-context" || got.RequiredCapabilityRefs[0] != "mcp-local-context:tool:repo.summary" {
		t.Fatalf("skill = %+v, want command values", got)
	}
	list, err := service.ListSkills(ctx)
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("skills len = %d, want 1", len(list))
	}
}

func TestServiceRejectsInvalidSkill(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewSkillRepository())

	err := service.CreateSkill(ctx, skillservice.CreateSkillCommand{
		SkillID: common.SkillID("skill-invalid"),
		Name:    "invalid",
		Version: "v1",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create invalid skill err = %v, want invalid argument", err)
	}
}
