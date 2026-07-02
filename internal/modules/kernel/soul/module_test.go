package soul

import (
	"context"
	"errors"
	"testing"

	soulcontracts "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/contracts"
	soulevents "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/events"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
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
	firstSubscriptionCount := len(module.unsub)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe soul commands")
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
	if len(module.unsub) != 0 {
		t.Fatalf("subscriptions without EventHub = %d, want 0", len(module.unsub))
	}
}

func TestModuleSoulCommandsUseRegistryContract(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
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

	err := hub.Publish(ctx, events.Envelope{
		Topic:       soulevents.TopicSoulCommandCreate,
		SubjectType: "soul",
		SubjectID:   "soul-contract",
		Payload: map[string]any{
			events.PayloadKeyCommand: soulcontracts.CreateSoulCommand{
				SoulID:          "soul-contract",
				Name:            "Contract Soul",
				SoulMD:          "# Contract Soul",
				BusinessDomains: []string{"contract"},
			},
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("create soul err = %v, want response", err)
	}
	item, ok := response.Payload[events.PayloadKeySoul].(*soulmodel.Soul)
	if !ok || item == nil || item.ID != "soul-contract" || item.Name != "Contract Soul" {
		t.Fatalf("soul = %+v, want created soul", response.Payload[events.PayloadKeySoul])
	}

	err = hub.Publish(ctx, events.Envelope{Topic: soulevents.TopicSoulCommandList})
	response, ok = events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("list souls err = %v, want response", err)
	}
	items, ok := response.Payload[events.PayloadKeySouls].([]*soulmodel.Soul)
	if !ok || len(items) != 1 || items[0].ID != "soul-contract" {
		t.Fatalf("souls = %+v, want created soul", response.Payload[events.PayloadKeySouls])
	}
}

func TestDeleteSoulCommandAcceptsContractPayload(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
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

	err := hub.Publish(ctx, events.Envelope{
		Topic:       soulevents.TopicSoulCommandDelete,
		SubjectType: "soul",
		SubjectID:   "soul-delete-contract",
		Payload: map[string]any{
			events.PayloadKeyCommand: soulcontracts.DeleteSoulCommand{SoulID: "soul-delete-contract"},
		},
	})
	if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("delete err = %v, want response", err)
	}
	if _, err := repo.Get(ctx, "soul-delete-contract"); err == nil {
		t.Fatalf("soul still exists after delete")
	}
}

func soulModuleContractDependencies() Dependencies {
	return Dependencies{
		Souls:    memoryrepo.NewSoulRepository(),
		EventHub: events.NewMemoryHub(),
	}
}
