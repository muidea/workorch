package workspace

import (
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
)

func TestWorkspaceCommandTopicsHaveOneSemanticFamily(t *testing.T) {
	expected := map[string]workspaceCommandFamily{
		workspaceevents.TopicWorkspaceCommandCreate:             workspaceCommandFamilyDefinition,
		workspaceevents.TopicWorkspaceCommandGet:                workspaceCommandFamilyDefinition,
		workspaceevents.TopicWorkspaceCommandList:               workspaceCommandFamilyDefinition,
		workspaceevents.TopicWorkspaceCommandUpdate:             workspaceCommandFamilyDefinition,
		workspaceevents.TopicWorkspaceCommandUpdateLLMSelection: workspaceCommandFamilyDefinition,
		workspaceevents.TopicWorkspaceCommandDelete:             workspaceCommandFamilyDefinition,
		workspaceevents.TopicWorkspaceCommandDefineGoal:         workspaceCommandFamilyGoal,
		workspaceevents.TopicWorkspaceCommandUpdateGoal:         workspaceCommandFamilyGoal,
		workspaceevents.TopicWorkspaceCommandDeleteGoal:         workspaceCommandFamilyGoal,
		workspaceevents.TopicWorkspaceCommandGetGoal:            workspaceCommandFamilyGoal,
		workspaceevents.TopicWorkspaceCommandListGoalsByWS:      workspaceCommandFamilyGoal,
		workspaceevents.TopicWorkspaceCommandDefineDoD:          workspaceCommandFamilyDoD,
		workspaceevents.TopicWorkspaceCommandUpdateDoD:          workspaceCommandFamilyDoD,
		workspaceevents.TopicWorkspaceCommandDeleteDoD:          workspaceCommandFamilyDoD,
		workspaceevents.TopicWorkspaceCommandGetDoD:             workspaceCommandFamilyDoD,
		workspaceevents.TopicWorkspaceCommandListDoDsByWS:       workspaceCommandFamilyDoD,
		workspaceevents.TopicWorkspaceTemplateCommandCreate:     workspaceCommandFamilyTemplate,
		workspaceevents.TopicWorkspaceTemplateCommandGet:        workspaceCommandFamilyTemplate,
		workspaceevents.TopicWorkspaceTemplateCommandList:       workspaceCommandFamilyTemplate,
		workspaceevents.TopicWorkspaceTemplateCommandUpdate:     workspaceCommandFamilyTemplate,
		workspaceevents.TopicWorkspaceTemplateCommandDelete:     workspaceCommandFamilyTemplate,
	}
	topics := workspaceCommandTopics()
	if len(topics) != len(expected) {
		t.Fatalf("unexpected command topic count: got %d want %d", len(topics), len(expected))
	}
	seen := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if _, duplicate := seen[topic]; duplicate {
			t.Fatalf("duplicate command topic %q", topic)
		}
		seen[topic] = struct{}{}
		family, ok := workspaceCommandFamilyForTopic(topic)
		if !ok {
			t.Fatalf("command topic %q has no family", topic)
		}
		if family != expected[topic] {
			t.Fatalf("command topic %q family = %d, want %d", topic, family, expected[topic])
		}
	}
	if _, ok := workspaceCommandFamilyForTopic("/workspace/command/unknown"); ok {
		t.Fatal("unknown command topic must fail closed")
	}
}
