package soul

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestStrongSoulCommandsIgnoreFireAndForgetPost(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	repo := memoryrepo.NewSoulRepository()
	owner, err := New(hub, nil, Dependencies{Souls: repo})
	if err != nil {
		t.Fatal(err)
	}
	eventFor := func(topic string, data any) magicevent.Event {
		source := applicationContextRegistrySource
		if topic == soulevents.TopicSoulCommandEnsureDefaults {
			source = systemCommandSource
		} else if !soulCommandSourceAllowed(topic, source) {
			t.Fatalf("Post fixture source %q is not authorized for %s", source, topic)
		}
		return magicevent.NewEventWithContext(topic, source, soulevents.CommandDestination, magicevent.NewHeader(), ctx, data)
	}

	owner.handleSoulCommand(eventFor(soulevents.TopicSoulCommandCreate, soulevents.CreateSoulCommand{SoulID: "post-create", Name: "post", SoulMD: "# post"}), nil)
	if _, err := repo.Get(ctx, "post-create"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("Post create mutated state: %v", err)
	}
	if err := repo.Create(ctx, &soulmodel.Soul{ID: "post-existing", Name: "existing", SoulMD: "# existing\n"}); err != nil {
		t.Fatal(err)
	}
	owner.handleSoulCommand(eventFor(soulevents.TopicSoulCommandUpdate, soulevents.UpdateSoulCommand{SoulID: "post-existing", Name: "updated", SoulMD: "# updated"}), nil)
	owner.handleSoulCommand(eventFor(soulevents.TopicSoulCommandDelete, soulevents.DeleteSoulCommand{SoulID: "post-existing"}), nil)
	owner.handleSystemDefaultsCommand(eventFor(soulevents.TopicSoulCommandEnsureDefaults, soulevents.EnsureDefaultsCommand{}), nil)
	item, err := repo.Get(ctx, "post-existing")
	if err != nil || item.Name != "existing" {
		t.Fatalf("Post update/delete mutated state: item=%+v err=%v", item, err)
	}
	items, err := repo.List(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("Post ensure seeded defaults: len=%d err=%v", len(items), err)
	}
	wrongSourceEvent := magicevent.NewEventWithContext(
		soulevents.TopicSoulCommandEnsureDefaults, applicationContextRegistrySource, soulevents.CommandDestination,
		magicevent.NewHeader(), ctx, soulevents.EnsureDefaultsCommand{},
	)
	wrongSourceResult := magicevent.NewResult(soulevents.TopicSoulCommandEnsureDefaults, applicationContextRegistrySource, soulevents.CommandDestination)
	owner.handleSystemDefaultsCommand(wrongSourceEvent, wrongSourceResult)
	if _, err := wrongSourceResult.Get(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("wrong-source ensure error = %v, want invalid argument", err)
	}
	items, err = repo.List(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("wrong-source ensure seeded defaults: len=%d err=%v", len(items), err)
	}
	exactSourceEvent := magicevent.NewEventWithContext(soulevents.TopicSoulCommandEnsureDefaults, systemCommandSource, soulevents.CommandDestination, magicevent.NewHeader(), ctx, soulevents.EnsureDefaultsCommand{})
	exactSourceResult := magicevent.NewResult(soulevents.TopicSoulCommandEnsureDefaults, systemCommandSource, soulevents.CommandDestination)
	owner.handleSystemDefaultsCommand(exactSourceEvent, exactSourceResult)
	data, resultErr := exactSourceResult.Get()
	response, ok := data.(soulevents.EnsureDefaultsResult)
	if resultErr != nil || !ok || !response.Ensured {
		t.Fatalf("exact-source ensure result = %#v err=%v", data, resultErr)
	}
	items, err = repo.List(ctx)
	if err != nil || len(items) <= 1 {
		t.Fatalf("exact-source ensure did not seed defaults: len=%d err=%v", len(items), err)
	}
}

func TestSoulCommandSourceAuthorizationMatrix(t *testing.T) {
	for _, topic := range []string{
		soulevents.TopicSoulCommandCreate,
		soulevents.TopicSoulCommandList,
		soulevents.TopicSoulCommandUpdate,
		soulevents.TopicSoulCommandDelete,
		soulevents.TopicSoulCommandValidateCapability,
	} {
		if !soulCommandSourceAllowed(topic, applicationContextRegistrySource) {
			t.Errorf("context-registry source rejected for %s", topic)
		}
		for _, source := range []string{applicationOwnerReadSource, "untrusted/module", " " + applicationContextRegistrySource + " "} {
			if soulCommandSourceAllowed(topic, source) {
				t.Errorf("source %q accepted for %s", source, topic)
			}
		}
	}
	for _, source := range []string{applicationContextRegistrySource, applicationOwnerReadSource} {
		if !soulCommandSourceAllowed(soulevents.TopicSoulCommandGet, source) {
			t.Errorf("read source %q rejected", source)
		}
	}
	for _, source := range []string{"untrusted/module", " " + applicationOwnerReadSource + " "} {
		if soulCommandSourceAllowed(soulevents.TopicSoulCommandGet, source) {
			t.Errorf("forged read source %q accepted", source)
		}
	}
}
