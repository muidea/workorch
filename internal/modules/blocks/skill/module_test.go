package skill

import (
	"context"
	"errors"
	"testing"

	skillbiz "github.com/muidea/workorch/internal/modules/blocks/skill/biz"

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
	if module.bizPtr == nil || module.bizPtr.EventHub() == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresSkillsRepository(t *testing.T) {
	if err := New().Configure(skillbiz.Dependencies{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Configure err = %v, want invalid argument", err)
	}
	module := New()
	if err := module.Configure(skillbiz.Dependencies{Skills: memory.NewSkillRepository()}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Configure without EventHub err = %v, want invalid argument", err)
	}
	if module.bizPtr != nil {
		t.Fatal("biz must not be constructed without EventHub")
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start without EventHub err = %v, want invalid argument", err)
	}
}

func TestModuleSkillCommandsUseRegistryContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	module := New()
	if err := module.Configure(skillbiz.Dependencies{Skills: memory.NewSkillRepository(), EventHub: hub}); err != nil {
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

	createResult := sendSkillCommand[skillevents.CreateSkillCommand, skillevents.CreateSkillResult](t, ctx, hub, skillevents.TopicSkillCommandCreate,
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
	if err := createResult.Skill.Provenance.Validate(); err != nil {
		t.Fatalf("created Skill provenance: %v", err)
	}

	listResult := sendSkillCommand[skillevents.ListSkillsCommand, skillevents.ListSkillsResult](t, ctx, hub, skillevents.TopicSkillCommandList, skillevents.ListSkillsCommand{})
	if len(listResult.Skills) != 1 || listResult.Skills[0].ID != "skill-contract" {
		t.Fatalf("skills = %+v, want created skill", listResult.Skills)
	}
}

func TestModuleRejectsForgedSkillSourcesAndDuplicateCreateWithoutExtraWrite(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	repository := memory.NewSkillRepository()
	module := New()
	if err := module.Configure(skillbiz.Dependencies{Skills: repository, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	command := skillevents.CreateSkillCommand{
		SkillID: "skill-source-duplicate", Name: "source duplicate", Version: "1.0.0", Instruction: "preserve exactly one owner resource",
	}
	send := func(source string) magicevent.Result {
		return hub.Send(magicevent.NewEventWithContext(
			skillevents.TopicSkillCommandCreate, source, skillevents.CommandDestination,
			magicevent.NewHeader(), ctx, command,
		))
	}
	for _, source := range []string{"untrusted/module", " application/workorchd/context-registry "} {
		result := send(source)
		if result == nil || !errors.Is(result.Error(), common.ErrInvalidArgument) {
			t.Fatalf("forged source %q result = %+v, want invalid argument", source, result)
		}
	}
	if _, err := repository.Get(ctx, command.SkillID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("forged create mutated owner: %v", err)
	}
	first := send("application/workorchd/context-registry")
	if first == nil || first.Error() != nil {
		t.Fatalf("first create = %+v", first)
	}
	second := send("application/workorchd/context-registry")
	if second == nil || !errors.Is(second.Error(), common.ErrConflict) {
		t.Fatalf("duplicate create = %+v, want conflict", second)
	}
	items, err := repository.List(ctx)
	if err != nil || len(items) != 1 || items[0].ID != command.SkillID {
		t.Fatalf("skills after duplicate = %+v err=%v", items, err)
	}
}

func TestModuleStopFencesSubsequentSkillDelivery(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	hub := &skillLifecycleHub{
		Hub:             rawHub,
		subscriptions:   map[string]int{},
		unsubscriptions: map[string]int{},
	}
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	repository := memory.NewSkillRepository()
	module := New()
	if err := module.Configure(skillbiz.Dependencies{Skills: repository, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	for _, topic := range skillLifecycleTopics() {
		if hub.subscriptions[topic] != 1 || hub.unsubscriptions[topic] != 1 {
			t.Fatalf("topic %s lifecycle subscriptions=%d unsubscriptions=%d, want 1/1", topic, hub.subscriptions[topic], hub.unsubscriptions[topic])
		}
	}
	command := skillevents.CreateSkillCommand{SkillID: "skill-after-stop", Name: "after stop", Version: "1.0.0", Instruction: "must not persist"}
	result := hub.Send(magicevent.NewEventWithContext(
		skillevents.TopicSkillCommandCreate, "application/workorchd/context-registry", skillevents.CommandDestination,
		magicevent.NewHeader(), ctx, command,
	))
	if result != nil && result.Error() == nil {
		t.Fatalf("post-Stop command unexpectedly succeeded: %+v", result)
	}
	if _, err := repository.Get(ctx, command.SkillID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("post-Stop command mutated owner: %v", err)
	}
}

type skillLifecycleHub struct {
	magicevent.Hub
	subscriptions   map[string]int
	unsubscriptions map[string]int
}

func (h *skillLifecycleHub) Subscribe(topic string, observer magicevent.Observer) {
	h.subscriptions[topic]++
	h.Hub.Subscribe(topic, observer)
}

func (h *skillLifecycleHub) Unsubscribe(topic string, observer magicevent.Observer) {
	h.unsubscriptions[topic]++
	h.Hub.Unsubscribe(topic, observer)
}

func skillLifecycleTopics() []string {
	return []string{
		skillevents.TopicSkillCommandCreate,
		skillevents.TopicSkillCommandGet,
		skillevents.TopicSkillCommandList,
	}
}

func skillModuleContractDependencies() skillbiz.Dependencies {
	return skillbiz.Dependencies{
		Skills:   memory.NewSkillRepository(),
		EventHub: magicevent.NewHub(16),
	}
}

func sendSkillCommand[Command any, Response any](t *testing.T, ctx context.Context, hub magicevent.Hub, topic string, command Command) Response {
	t.Helper()
	var zero Response
	source := "application/workorchd/appservice"
	if topic == skillevents.TopicSkillCommandCreate {
		source = "application/workorchd/context-registry"
	}
	ev := magicevent.NewEventWithContext(topic, source, skillevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := hub.Send(ev)
	if result == nil || result.Error() != nil {
		t.Fatalf("send %s error: %v", topic, result)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("result %s: %v", topic, err)
	}
	typed, ok := data.(Response)
	if !ok {
		t.Fatalf("result %s type = %T", topic, data)
		return zero
	}
	return typed
}
