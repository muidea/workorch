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
		return magicevent.NewEventWithContext(topic, "test", soulevents.CommandDestination, magicevent.NewHeader(), ctx, data)
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
}
