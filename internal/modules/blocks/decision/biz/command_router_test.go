package decision

import (
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
)

func TestDecisionCommandTopicsHaveOneSemanticFamily(t *testing.T) {
	expected := map[string]decisionCommandFamily{
		decisionevents.TopicDecisionCommandGetTicket:             decisionCommandFamilyTicketQuery,
		decisionevents.TopicDecisionCommandListTickets:           decisionCommandFamilyTicketQuery,
		decisionevents.TopicDecisionCommandListTicketsByRun:      decisionCommandFamilyTicketQuery,
		decisionevents.TopicDecisionCommandListTicketsByWS:       decisionCommandFamilyTicketQuery,
		decisionevents.TopicDecisionCommandEnsureResolved:        decisionCommandFamilyResolution,
		decisionevents.TopicDecisionCommandGetResolution:         decisionCommandFamilyResolution,
		decisionevents.TopicDecisionCommandPurgeWorkspace:        decisionCommandFamilyAggregatePurge,
		decisionevents.TopicDecisionCommandSealWorkspaceDeletion: decisionCommandFamilyAggregatePurge,
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

func TestDecisionOwnerCommandInventoryCoversAllSeventeenTypedTopics(t *testing.T) {
	expected := map[string]struct{}{
		decisionevents.TopicRegisterContractChangeProposal: {},
		decisionevents.TopicDecisionCommandGetTicket:       {}, decisionevents.TopicDecisionCommandListTickets: {},
		decisionevents.TopicDecisionCommandListTicketsByRun: {}, decisionevents.TopicDecisionCommandListTicketsByWS: {},
		decisionevents.TopicDecisionCommandEnsureResolved: {}, decisionevents.TopicDecisionCommandGetResolution: {},
		decisionevents.TopicDecisionCommandPurgeWorkspace: {}, decisionevents.TopicDecisionCommandSealWorkspaceDeletion: {},
		decisionevents.TopicOpenRuntimeGovernanceRequest: {}, decisionevents.TopicAcquireRuntimeCapabilities: {},
		decisionevents.TopicOpenWorkspaceDefinitionClarification: {}, decisionevents.TopicReconcileWorkspaceDefinitionClarificationCandidate: {},
		decisionevents.TopicResolveWorkspaceDefinitionClarificationByAdmission: {}, decisionevents.TopicQueryWorkspaceDefinitionClarificationFact: {},
		decisionevents.TopicOpenRuntimeDefinitionClarification: {}, decisionevents.TopicResolveRuntimeDefinitionClarificationByReplacement: {},
	}
	topics := append([]string(nil), decisionCommandTopics()...)
	topics = append(topics, decisionevents.TopicOpenRuntimeGovernanceRequest, decisionevents.TopicRegisterContractChangeProposal)
	topics = append(topics, definitionClarificationTopics()...)
	topics = append(topics, decisionevents.TopicAcquireRuntimeCapabilities)
	if len(topics) != len(expected) {
		t.Fatalf("Decision command inventory count = %d, want %d", len(topics), len(expected))
	}
	seen := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if _, ok := expected[topic]; !ok {
			t.Fatalf("undeclared Decision command topic %q", topic)
		}
		if _, duplicate := seen[topic]; duplicate {
			t.Fatalf("duplicate Decision command topic %q", topic)
		}
		seen[topic] = struct{}{}
	}
	for topic := range expected {
		if _, ok := seen[topic]; !ok {
			t.Fatalf("missing Decision command topic %q", topic)
		}
	}
}
