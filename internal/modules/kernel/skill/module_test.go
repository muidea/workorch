package skill

import (
	"context"
	"errors"
	"testing"

	skillcontracts "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/contracts"
	skillevents "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/events"
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if module.Service() != nil {
		t.Fatal("new module service is non-nil before Configure")
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(skillModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.Service() == nil {
		t.Fatal("service is nil after Configure")
	}
	if module.bizPtr == nil || module.servicePtr == nil || module.eventHub == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstSubscriptionCount := len(module.unsub)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe skill commands")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(module.unsub) != firstSubscriptionCount {
		t.Fatalf("second Start subscription count = %d, want %d", len(module.unsub), firstSubscriptionCount)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.Service() != nil ||
		module.bizPtr != nil ||
		module.servicePtr != nil ||
		module.eventHub != nil ||
		len(module.unsub) != 0 {
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
	if module.Service() == nil {
		t.Fatal("service is nil after Configure without EventHub")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start without EventHub should remain no-op: %v", err)
	}
	if len(module.unsub) != 0 {
		t.Fatalf("subscriptions without EventHub = %d, want 0", len(module.unsub))
	}
}

func TestModuleSkillCommandsUseRegistryContract(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
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

	err := hub.Publish(ctx, events.Envelope{
		Topic:       skillevents.TopicSkillCommandCreate,
		SubjectType: "skill",
		SubjectID:   "skill-contract",
		Payload: map[string]any{
			events.PayloadKeyCommand: skillcontracts.CreateSkillCommand{
				SkillID:      "skill-contract",
				Name:         "Contract Skill",
				Version:      "1.0.0",
				TriggerRules: []string{"contract"},
				Instruction:  "Use structured refs only.",
			},
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("create skill err = %v, want response", err)
	}
	item, ok := response.Payload[events.PayloadKeySkill].(*skillmodel.Skill)
	if !ok || item == nil || item.ID != "skill-contract" || item.Version != "1.0.0" {
		t.Fatalf("skill = %+v, want created skill", response.Payload[events.PayloadKeySkill])
	}

	err = hub.Publish(ctx, events.Envelope{Topic: skillevents.TopicSkillCommandList})
	response, ok = events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("list skills err = %v, want response", err)
	}
	items, ok := response.Payload[events.PayloadKeySkills].([]*skillmodel.Skill)
	if !ok || len(items) != 1 || items[0].ID != "skill-contract" {
		t.Fatalf("skills = %+v, want created skill", response.Payload[events.PayloadKeySkills])
	}
}

func skillModuleContractDependencies() Dependencies {
	return Dependencies{
		Skills:   memory.NewSkillRepository(),
		EventHub: events.NewMemoryHub(),
	}
}
