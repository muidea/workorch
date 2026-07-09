package soul

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	soulevents "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/events"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
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
	if err := module.Configure(soulModuleContractDependencies()); err != nil {
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
	firstSubscriptionCount := len(module.topics)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe soul commands")
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
	if module.Service() != nil ||
		module.bizPtr != nil ||
		module.servicePtr != nil ||
		module.eventHub != nil ||
		len(module.topics) != 0 {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresSoulRepository(t *testing.T) {
	if err := New().Configure(Dependencies{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Configure err = %v, want invalid argument", err)
	}
	module := New()
	if err := module.Configure(Dependencies{Souls: memoryrepo.NewSoulRepository()}); err != nil {
		t.Fatalf("Configure without EventHub: %v", err)
	}
	if module.Service() == nil {
		t.Fatal("service is nil after Configure without EventHub")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start without EventHub should remain no-op: %v", err)
	}
	if len(module.topics) != 0 {
		t.Fatalf("subscriptions without EventHub = %d, want 0", len(module.topics))
	}
}

func TestModuleSoulCommandsUseRegistryContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	module := New()
	if err := module.Configure(Dependencies{Souls: memoryrepo.NewSoulRepository(), EventHub: hub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(context.Background()); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	createResult := sendSoulCommand(t, ctx, hub, soulevents.TopicSoulCommandCreate,
		soulevents.CreateSoulCommand{
			SoulID:          "soul-contract",
			Name:            "Contract Soul",
			SoulMD:          "# Contract Soul",
			BusinessDomains: []string{"contract"},
		},
	)
	if createResult.Soul == nil || createResult.Soul.ID != "soul-contract" || createResult.Soul.Name != "Contract Soul" {
		t.Fatalf("soul = %+v, want created soul", createResult.Soul)
	}

	listResult := sendSoulCommand(t, ctx, hub, soulevents.TopicSoulCommandList, soulevents.ListSoulsCommand{})
	if len(listResult.Souls) != 1 || listResult.Souls[0].ID != "soul-contract" {
		t.Fatalf("souls = %+v, want created soul", listResult.Souls)
	}
}

func TestDeleteSoulCommandAcceptsContractPayload(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	repo := memoryrepo.NewSoulRepository()
	if err := repo.Create(ctx, &soulmodel.Soul{ID: "soul-delete-contract", Name: "Delete Contract", SoulMD: "# soul"}); err != nil {
		t.Fatalf("seed soul: %v", err)
	}
	module := New()
	if err := module.Configure(Dependencies{Souls: repo, EventHub: hub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(context.Background()); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	_ = sendSoulCommand(t, ctx, hub, soulevents.TopicSoulCommandDelete, soulevents.DeleteSoulCommand{SoulID: "soul-delete-contract"})
	if _, err := repo.Get(ctx, "soul-delete-contract"); err == nil {
		t.Fatalf("soul still exists after delete")
	}
}

func soulModuleContractDependencies() Dependencies {
	return Dependencies{
		Souls:    memoryrepo.NewSoulRepository(),
		EventHub: magicevent.NewHub(16),
	}
}

func sendSoulCommand[Command any](t *testing.T, ctx context.Context, hub magicevent.Hub, topic string, command Command) soulevents.SoulCommandResult {
	t.Helper()
	ev := magicevent.NewEventWithContext(topic, "test", soulevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := hub.Send(ev)
	if result == nil || result.Error() != nil {
		t.Fatalf("send %s error: %v", topic, result)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("result %s: %v", topic, err)
	}
	typed, ok := data.(soulevents.SoulCommandResult)
	if !ok {
		t.Fatalf("result %s type = %T", topic, data)
	}
	return typed
}
