package skill

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if ModuleID != "blocks/skill" {
		t.Fatalf("ModuleID = %q, want relocated stateful block ID", ModuleID)
	}
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(skillModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil || module.eventHub == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstSubscriptionCount := len(module.topics)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe skill commands")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(module.topics) != firstSubscriptionCount {
		t.Fatalf("second Start subscription count = %d, want %d", len(module.topics), firstSubscriptionCount)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil ||
		module.eventHub != nil ||
		len(module.topics) != 0 {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresSkillsRepository(t *testing.T) {
	if err := New().Configure(Dependencies{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Configure err = %v, want invalid argument", err)
	}
	module := New()
	if err := module.Configure(Dependencies{Skills: memory.NewSkillRepository()}); err != nil {
		t.Fatalf("Configure without EventHub: %v", err)
	}
	if module.bizPtr == nil {
		t.Fatal("biz is nil after Configure without EventHub")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start without EventHub should remain no-op: %v", err)
	}
	if len(module.topics) != 0 {
		t.Fatalf("subscriptions without EventHub = %d, want 0", len(module.topics))
	}
}

func TestModuleSkillCommandsUseRegistryContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	module := New()
	if err := module.Configure(Dependencies{Skills: memory.NewSkillRepository(), EventHub: hub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	createResult := sendSkillCommand(t, ctx, hub, skillevents.TopicSkillCommandCreate,
		skillevents.CreateSkillCommand{
			SkillID:      "skill-contract",
			Name:         "Contract Skill",
			Version:      "1.0.0",
			TriggerRules: []string{"contract"},
			Instruction:  "Use structured refs only.",
		},
	)
	if createResult.Skill == nil || createResult.Skill.ID != "skill-contract" || createResult.Skill.Version != "1.0.0" {
		t.Fatalf("skill = %+v, want created skill", createResult.Skill)
	}

	listResult := sendSkillCommand(t, ctx, hub, skillevents.TopicSkillCommandList, skillevents.ListSkillsCommand{})
	if len(listResult.Skills) != 1 || listResult.Skills[0].ID != "skill-contract" {
		t.Fatalf("skills = %+v, want created skill", listResult.Skills)
	}
}

func skillModuleContractDependencies() Dependencies {
	return Dependencies{
		Skills:   memory.NewSkillRepository(),
		EventHub: magicevent.NewHub(16),
	}
}

func sendSkillCommand[Command any](t *testing.T, ctx context.Context, hub magicevent.Hub, topic string, command Command) skillevents.SkillCommandResult {
	t.Helper()
	ev := magicevent.NewEventWithContext(topic, "test", skillevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := hub.Send(ev)
	if result == nil || result.Error() != nil {
		t.Fatalf("send %s error: %v", topic, result)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("result %s: %v", topic, err)
	}
	typed, ok := data.(skillevents.SkillCommandResult)
	if !ok {
		t.Fatalf("result %s type = %T", topic, data)
	}
	return typed
}
