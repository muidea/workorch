package approval

import (
	"testing"

	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
)

func TestApprovalCommandTopicsHaveOneSemanticFamily(t *testing.T) {
	expected := map[string]approvalCommandFamily{
		approvalevents.TopicApprovalCommandGet:                   approvalCommandFamilyQuery,
		approvalevents.TopicApprovalCommandList:                  approvalCommandFamilyQuery,
		approvalevents.TopicApprovalCommandListByRun:             approvalCommandFamilyQuery,
		approvalevents.TopicApprovalCommandListByWorkspace:       approvalCommandFamilyQuery,
		approvalevents.TopicApprovalCommandEnsureApproved:        approvalCommandFamilyResolution,
		approvalevents.TopicApprovalCommandEnsureRejected:        approvalCommandFamilyResolution,
		approvalevents.TopicApprovalCommandPurgeWorkspace:        approvalCommandFamilyPurge,
		approvalevents.TopicApprovalCommandSealWorkspaceDeletion: approvalCommandFamilyPurge,
	}
	topics := approvalCommandTopics()
	if len(topics) != len(expected) {
		t.Fatalf("unexpected command topic count: got %d want %d", len(topics), len(expected))
	}
	seen := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if _, duplicate := seen[topic]; duplicate {
			t.Fatalf("duplicate command topic %q", topic)
		}
		seen[topic] = struct{}{}
		family, ok := approvalCommandFamilyForTopic(topic)
		if !ok {
			t.Fatalf("command topic %q has no family", topic)
		}
		if family != expected[topic] {
			t.Fatalf("command topic %q family = %d, want %d", topic, family, expected[topic])
		}
	}
	if _, ok := approvalCommandFamilyForTopic("/approval/command/unknown"); ok {
		t.Fatal("unknown command topic must fail closed")
	}
}
