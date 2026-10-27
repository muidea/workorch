package memory

import (
	"testing"

	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
)

func TestMemoryCommandTopicsHaveOneSemanticFamily(t *testing.T) {
	expected := map[string]memoryCommandFamily{
		memoryevents.TopicObservePreparationInventory:        memoryCommandFamilyItem,
		memoryevents.TopicMemoryCommandCreate:                memoryCommandFamilyItem,
		memoryevents.TopicMemoryCommandGet:                   memoryCommandFamilyItem,
		memoryevents.TopicMemoryCommandListByWorkspace:       memoryCommandFamilyItem,
		memoryevents.TopicMemoryCommandSearch:                memoryCommandFamilyItem,
		memoryevents.TopicMemoryCommandUpdate:                memoryCommandFamilyItem,
		memoryevents.TopicMemoryCommandDelete:                memoryCommandFamilyItem,
		memoryevents.TopicMemoryRecallTraceCommandUpsert:     memoryCommandFamilyRecallTrace,
		memoryevents.TopicMemoryRecallTraceCommandList:       memoryCommandFamilyRecallTrace,
		memoryevents.TopicMemoryCommandSealWorkspaceDeletion: memoryCommandFamilyWorkspaceDeletion,
		memoryevents.TopicMemoryCommandPurgeWorkspace:        memoryCommandFamilyWorkspaceDeletion,
	}
	topics := memoryCommandTopics()
	if len(topics) != len(expected) {
		t.Fatalf("unexpected command topic count: got %d want %d", len(topics), len(expected))
	}
	seen := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if _, duplicate := seen[topic]; duplicate {
			t.Fatalf("duplicate command topic %q", topic)
		}
		seen[topic] = struct{}{}
		family, ok := memoryCommandFamilyForTopic(topic)
		if !ok {
			t.Fatalf("command topic %q has no family", topic)
		}
		if family != expected[topic] {
			t.Fatalf("command topic %q family = %d, want %d", topic, family, expected[topic])
		}
	}
	if _, ok := memoryCommandFamilyForTopic("/memory/command/unknown"); ok {
		t.Fatal("unknown command topic must fail closed")
	}
}
