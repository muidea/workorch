package run

import (
	"testing"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
)

func TestRunCommandTopicsHaveOneSemanticFamily(t *testing.T) {
	expected := map[string]runCommandFamily{
		runevents.TopicRunCommandGet:                         runCommandFamilyRunQuery,
		runevents.TopicRunCommandList:                        runCommandFamilyRunQuery,
		runevents.TopicRunCommandListByWorkspace:             runCommandFamilyRunQuery,
		runevents.TopicRunCommandLifecycleFactInventory:      runCommandFamilyRunQuery,
		runevents.TopicRunCommandGetStep:                     runCommandFamilyStepQuery,
		runevents.TopicRunCommandListStepsByRun:              runCommandFamilyStepQuery,
		runevents.TopicRunCommandListStepSummariesByRun:      runCommandFamilyStepQuery,
		runevents.TopicRunCommandGetRuntimeCheckpoint:        runCommandFamilyCheckpointQuery,
		runevents.TopicRunCommandListCheckpointsByRun:        runCommandFamilyCheckpointQuery,
		runevents.TopicRunCommandStoreRuntimeStateRecord:     runCommandFamilyRuntimeStateStore,
		runevents.TopicRunCommandWorkspaceRetirementFact:     runCommandFamilyWorkspaceRetirement,
		runevents.TopicRunCommandPrepareWorkspaceRetirement:  runCommandFamilyWorkspaceRetirement,
		runevents.TopicRunCommandFinalizeWorkspaceRetirement: runCommandFamilyWorkspaceRetirement,
		runevents.TopicRunCommandPurgeWorkspace:              runCommandFamilyWorkspaceRetirement,
	}
	topics := runCommandTopics()
	if len(topics) != len(expected) {
		t.Fatalf("unexpected command topic count: got %d want %d", len(topics), len(expected))
	}
	seen := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if _, duplicate := seen[topic]; duplicate {
			t.Fatalf("duplicate command topic %q", topic)
		}
		seen[topic] = struct{}{}
		family, ok := runCommandFamilyForTopic(topic)
		if !ok {
			t.Fatalf("command topic %q has no family", topic)
		}
		if family != expected[topic] {
			t.Fatalf("command topic %q family = %d, want %d", topic, family, expected[topic])
		}
	}
	if _, ok := runCommandFamilyForTopic("/run/command/unknown"); ok {
		t.Fatal("unknown command topic must fail closed")
	}
}
