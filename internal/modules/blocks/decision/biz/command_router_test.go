package decision

import (
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
)

func TestDecisionCommandTopicsHaveOneSemanticFamily(t *testing.T) {
	expected := map[string]decisionCommandFamily{
		decisionevents.TopicDecisionCommandGetTicket:        decisionCommandFamilyTicketQuery,
		decisionevents.TopicDecisionCommandListTickets:      decisionCommandFamilyTicketQuery,
		decisionevents.TopicDecisionCommandListTicketsByRun: decisionCommandFamilyTicketQuery,
		decisionevents.TopicDecisionCommandListTicketsByWS:  decisionCommandFamilyTicketQuery,
		decisionevents.TopicDecisionCommandDeleteTicket:     decisionCommandFamilyTicketMutation,
		decisionevents.TopicDecisionCommandEnsureResolved:   decisionCommandFamilyResolution,
		decisionevents.TopicDecisionCommandGetResolution:    decisionCommandFamilyResolution,
		decisionevents.TopicDecisionCommandDeleteResolution: decisionCommandFamilyResolution,
	}
	topics := decisionCommandTopics()
	if len(topics) != len(expected) {
		t.Fatalf("unexpected command topic count: got %d want %d", len(topics), len(expected))
	}
	seen := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if _, duplicate := seen[topic]; duplicate {
			t.Fatalf("duplicate command topic %q", topic)
		}
		seen[topic] = struct{}{}
		family, ok := decisionCommandFamilyForTopic(topic)
		if !ok {
			t.Fatalf("command topic %q has no family", topic)
		}
		if family != expected[topic] {
			t.Fatalf("command topic %q family = %d, want %d", topic, family, expected[topic])
		}
	}
	if _, ok := decisionCommandFamilyForTopic("/decision/command/unknown"); ok {
		t.Fatal("unknown command topic must fail closed")
	}
}
