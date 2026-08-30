package skill

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/common"
	skillservice "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func TestServiceCreateAndListSkill(t *testing.T) {
	ctx := context.Background()
	service := newService(backend.NewSkillRepository())

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
	created, err := service.createSkill(ctx, cmd)
	if err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if created.Provenance.Source != workorchskillcontract.SourceWorkOrchRegistry || created.Provenance.Scope != workorchskillcontract.ScopeGlobal || created.Provenance.ContentHash != workorchskillcontract.StableContentHash(created.Content()) {
		t.Fatalf("created provenance = %+v", created.Provenance)
	}
	got, err := service.getSkill(ctx, cmd.SkillID)
	if err != nil {
		t.Fatalf("get skill: %v", err)
	}
	if got.Name != cmd.Name || got.RequiredMCPServerRefs[0] != "mcp-local-context" || got.RequiredCapabilityRefs[0] != "mcp-local-context:tool:repo.summary" {
		t.Fatalf("skill = %+v, want command values", got)
	}
	list, nextCursor, err := service.listSkills(ctx, "", 0)
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if len(list) != 1 || nextCursor != "" {
		t.Fatalf("skills len/cursor = %d/%q, want 1/end", len(list), nextCursor)
	}
	cmd.TriggerRules[0] = "mutated-command"
	created.TriggerRules[0] = "mutated-result"
	if reread, readErr := service.getSkill(ctx, cmd.SkillID); readErr != nil || reread.TriggerRules[0] != "go" {
		t.Fatalf("owner state shares caller slices: item=%+v err=%v", reread, readErr)
	}
}

func TestServiceRejectsInvalidSkill(t *testing.T) {
	ctx := context.Background()
	service := newService(backend.NewSkillRepository())

	_, err := service.createSkill(ctx, skillservice.CreateSkillCommand{
		SkillID: common.SkillID("skill-invalid"),
		Name:    "invalid",
		Version: "v1",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create invalid skill err = %v, want invalid argument", err)
	}

	_, err = service.createSkill(ctx, skillservice.CreateSkillCommand{
		SkillID:      common.SkillID("skill-blank-values"),
		Name:         "   ",
		Version:      "v1",
		Instruction:  "Use small verified steps.",
		TriggerRules: []string{"go"},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create blank skill name err = %v, want invalid argument", err)
	}

	_, err = service.createSkill(ctx, skillservice.CreateSkillCommand{
		SkillID:      common.SkillID("skill-blank-trigger"),
		Name:         "blank trigger",
		Version:      "v1",
		Instruction:  "Use small verified steps.",
		TriggerRules: []string{"go", "   "},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create blank trigger skill err = %v, want invalid argument", err)
	}

	_, err = service.createSkill(ctx, skillservice.CreateSkillCommand{
		SkillID: " skill-non-canonical ", Name: "non-canonical id", Version: "v1", Instruction: "instruction",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create non-canonical Skill id err = %v, want invalid argument", err)
	}
}

func TestServiceRejectsOversizedSkillMaterial(t *testing.T) {
	service := newService(backend.NewSkillRepository())
	_, err := service.createSkill(context.Background(), skillservice.CreateSkillCommand{
		SkillID: "skill-too-large", Name: "too large", Version: "v1",
		Instruction: strings.Repeat("x", modulecommon.SkillInstructionMaxBytes+1),
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("oversized instruction err = %v, want invalid argument", err)
	}
}

func TestServiceRejectsInvalidUTF8SkillMaterial(t *testing.T) {
	invalid := string([]byte{0xff})
	base := func() skillservice.CreateSkillCommand {
		return skillservice.CreateSkillCommand{SkillID: "skill-utf8", Name: "utf8", Version: "v1", Description: "description", Instruction: "instruction"}
	}
	for _, test := range []struct {
		name   string
		mutate func(*skillservice.CreateSkillCommand)
	}{
		{name: "id", mutate: func(command *skillservice.CreateSkillCommand) { command.SkillID = common.SkillID(invalid) }},
		{name: "name", mutate: func(command *skillservice.CreateSkillCommand) { command.Name = invalid }},
		{name: "version", mutate: func(command *skillservice.CreateSkillCommand) { command.Version = invalid }},
		{name: "description", mutate: func(command *skillservice.CreateSkillCommand) { command.Description = invalid }},
		{name: "instruction", mutate: func(command *skillservice.CreateSkillCommand) { command.Instruction = invalid }},
		{name: "trigger", mutate: func(command *skillservice.CreateSkillCommand) { command.TriggerRules = []string{invalid} }},
		{name: "reference", mutate: func(command *skillservice.CreateSkillCommand) { command.ReferenceRefs = []string{invalid} }},
		{name: "tool", mutate: func(command *skillservice.CreateSkillCommand) { command.RequiredToolKinds = []string{invalid} }},
		{name: "mcp server", mutate: func(command *skillservice.CreateSkillCommand) { command.RequiredMCPServerRefs = []string{invalid} }},
		{name: "capability", mutate: func(command *skillservice.CreateSkillCommand) { command.RequiredCapabilityRefs = []string{invalid} }},
		{name: "constraint", mutate: func(command *skillservice.CreateSkillCommand) { command.Constraints = []string{invalid} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := base()
			test.mutate(&command)
			if _, err := newService(backend.NewSkillRepository()).createSkill(context.Background(), command); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("invalid UTF-8 %s error = %v, want invalid argument", test.name, err)
			}
		})
	}
}

func TestServiceGetSkillRejectsInvalidID(t *testing.T) {
	service := newService(backend.NewSkillRepository())
	for name, id := range map[string]common.SkillID{
		"non-canonical": " skill-id ",
		"invalid UTF-8": common.SkillID(string([]byte{0xff})),
		"oversized":     common.SkillID(strings.Repeat("x", modulecommon.SkillIDMaxBytes+1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.getSkill(context.Background(), id); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("get Skill with invalid ID error = %v, want invalid argument", err)
			}
		})
	}
}

func TestServiceRejectsPersistedSkillWithMissingOrDriftedOwnerProvenance(t *testing.T) {
	repository := backend.NewSkillRepository()
	if err := repository.Create(context.Background(), &skillmodel.Skill{ID: "skill-invalid-provenance", Name: "invalid", Version: "v1", Instruction: "instruction"}); err != nil {
		t.Fatalf("seed malformed Skill: %v", err)
	}
	drifted := skillFromCommand(skillservice.CreateSkillCommand{SkillID: "skill-drifted", Name: "drifted", Version: "v1", Instruction: "original"})
	drifted.Instruction = "tampered after provenance was issued"
	if err := repository.Create(context.Background(), drifted); err != nil {
		t.Fatalf("seed drifted Skill: %v", err)
	}
	service := newService(repository)
	if _, err := service.getSkill(context.Background(), "skill-invalid-provenance"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("get malformed provenance err = %v, want conflict", err)
	}
	if _, err := service.getSkill(context.Background(), "skill-drifted"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("get drifted provenance err = %v, want conflict", err)
	}
}

func TestServiceListSkillsIsBoundedAndCursorPaged(t *testing.T) {
	service := newService(backend.NewSkillRepository())
	ctx := context.Background()
	for idx := 0; idx < modulecommon.SkillListMaxLimit+5; idx++ {
		id := common.SkillID(fmt.Sprintf("skill-%03d", idx))
		if _, err := service.createSkill(ctx, skillservice.CreateSkillCommand{SkillID: id, Name: string(id), Version: "v1", Instruction: "instruction"}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	first, cursor, err := service.listSkills(ctx, "", 0)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first) != modulecommon.SkillListDefaultLimit || cursor != "skill-099" {
		t.Fatalf("first page len/cursor = %d/%q", len(first), cursor)
	}
	clamped, maxCursor, err := service.listSkills(ctx, "", modulecommon.SkillListMaxLimit+1)
	if err != nil {
		t.Fatalf("max-clamped page: %v", err)
	}
	if len(clamped) != modulecommon.SkillListMaxLimit || maxCursor != "skill-199" {
		t.Fatalf("max-clamped page len/cursor = %d/%q", len(clamped), maxCursor)
	}
	tail, next, err := service.listSkills(ctx, maxCursor, modulecommon.SkillListMaxLimit+1)
	if err != nil {
		t.Fatalf("tail page: %v", err)
	}
	if len(tail) != 5 || next != "" || tail[0].ID != "skill-200" {
		t.Fatalf("tail page = len:%d cursor:%q first:%v", len(tail), next, tail[0].ID)
	}
	for _, cursor := range []common.SkillID{
		" non-canonical ",
		common.SkillID(string([]byte{0xff})),
		common.SkillID(strings.Repeat("x", modulecommon.SkillIDMaxBytes+1)),
	} {
		if _, _, err := service.listSkills(ctx, cursor, 1); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("invalid cursor %q error = %v, want invalid argument", cursor, err)
		}
	}
}
