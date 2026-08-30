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

func TestMCPRegistryCommandSourceAuthorizationMatrix(t *testing.T) {
	tests := []struct {
		topic   string
		allowed []string
	}{
		{mcpevents.TopicMCPCommandCreateServer, []string{applicationContextRegistrySource}},
		{mcpevents.TopicMCPCommandGetServer, []string{applicationContextRegistrySource, applicationOwnerReadSource, mcpExecutionSource}},
		{mcpevents.TopicMCPCommandListServers, []string{applicationContextRegistrySource}},
		{mcpevents.TopicMCPCommandUpdateServer, []string{applicationContextRegistrySource}},
		{mcpevents.TopicMCPCommandCreateCapability, []string{applicationContextRegistrySource}},
		{mcpevents.TopicMCPCommandGetCapability, []string{applicationContextRegistrySource, applicationOwnerReadSource}},
		{mcpevents.TopicMCPCommandListCapabilities, []string{applicationContextRegistrySource, applicationReadModelSource}},
		{mcpevents.TopicMCPCommandUpdateCapability, []string{applicationContextRegistrySource}},
		{mcpevents.TopicMCPCommandReadRegistrySnapshot, []string{mcpExecutionSource}},
		{mcpevents.TopicMCPCommandReadSelectionSnapshot, []string{applicationOwnerReadSource}},
		{mcpevents.TopicMCPCommandEnsureDefaults, []string{systemCommandSource}},
	}
	allSources := []string{
		applicationContextRegistrySource, applicationOwnerReadSource, applicationReadModelSource,
		mcpExecutionSource, systemCommandSource, "untrusted/module", " application/workorchd/context-registry ",
	}
	for _, testCase := range tests {
		t.Run(testCase.topic, func(t *testing.T) {
			allowed := make(map[string]bool, len(testCase.allowed))
			for _, source := range testCase.allowed {
				allowed[source] = true
			}
			for _, source := range allSources {
				if got := mcpRegistryCommandSourceAllowed(testCase.topic, source); got != allowed[source] {
					t.Errorf("source %q allowed=%t, want %t", source, got, allowed[source])
				}
			}
		})
	}
}
