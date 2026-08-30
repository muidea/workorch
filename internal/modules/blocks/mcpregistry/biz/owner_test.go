package mcp

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestMCPRegistryEnsureDefaultsRequiresExactSystemSource(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	registry := memoryrepo.NewMCPRegistryRepository()
	owner, err := New(hub, nil, Dependencies{Registry: registry, EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	event := magicevent.NewEventWithContext(
		mcpevents.TopicMCPCommandEnsureDefaults, "application/workorchd", mcpevents.CommandDestination,
		magicevent.NewHeader(), ctx, mcpevents.EnsureDefaultsCommand{},
	)
	result := magicevent.NewResult(mcpevents.TopicMCPCommandEnsureDefaults, "application/workorchd", mcpevents.CommandDestination)
	owner.handleCommand(event, result)
	if _, err := result.Get(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("wrong-source ensure error = %v, want invalid argument", err)
	}
	snapshot, err := registry.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 0 || len(snapshot.Servers) != 0 || len(snapshot.Capabilities) != 0 {
		t.Fatalf("wrong-source ensure mutated registry: %+v", snapshot)
	}
	exactEvent := magicevent.NewEventWithContext(
		mcpevents.TopicMCPCommandEnsureDefaults, systemCommandSource, mcpevents.CommandDestination,
		magicevent.NewHeader(), ctx, mcpevents.EnsureDefaultsCommand{},
	)
	exactResult := magicevent.NewResult(mcpevents.TopicMCPCommandEnsureDefaults, systemCommandSource, mcpevents.CommandDestination)
	owner.handleCommand(exactEvent, exactResult)
	data, resultErr := exactResult.Get()
	response, ok := data.(mcpevents.EnsureDefaultsResult)
	if resultErr != nil || !ok || !response.Ensured {
		t.Fatalf("exact-source ensure result = %#v err=%v", data, resultErr)
	}
	snapshot, err = registry.Load(ctx)
	if err != nil || snapshot.Revision == 0 || len(snapshot.Servers) == 0 || len(snapshot.Capabilities) == 0 {
		t.Fatalf("exact-source ensure did not seed registry: %+v err=%v", snapshot, err)
	}
}
