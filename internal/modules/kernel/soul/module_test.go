package soul

import (
	"context"
	"testing"

	soulcontracts "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/contracts"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/events"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

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
		Topic:       events.TopicSoulCommandDelete,
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
