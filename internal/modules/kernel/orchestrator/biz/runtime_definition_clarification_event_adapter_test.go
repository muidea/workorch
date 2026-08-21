package orchestrator

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
)

func runtimeDefinitionClarificationCommandForTest() decisionevents.OpenRuntimeDefinitionClarificationRequestCommand {
	command := decisionevents.OpenRuntimeDefinitionClarificationRequestCommand{
		SchemaVersion: decisionevents.DefinitionClarificationSchemaVersionCurrent, WorkspaceID: "workspace-1", RunID: "run-1",
		DefinitionRevision: "definition-1", StartupInputHash: "startup-1", AuthoritySnapshotID: "authority-1", ClarificationDispatchID: "clarification-dispatch-1",
		FocusRef: "focus-1", LogicalRequestID: "semantic-request-1", SemanticDocumentHash: "semantic-document-1",
		GapKind: decisionevents.RuntimeDefinitionGapImplicitOneOf, OffendingItemRefs: []string{"dod-item-1"},
		CanonicalGapSignatureVersion: "v1", CanonicalGapSignature: "implicit-one-of:dod-item-1", RequiredActionKind: decisionevents.DefinitionClarificationActionReviseAndRestart,
	}
	command.CanonicalGapSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash("runtime-signature", command.CanonicalGapSignatureVersion, command.CanonicalGapSignature)
	command.TicketID = decisionevents.StableRuntimeDefinitionClarificationTicketID(command)
	return command
}

func TestOpenRuntimeDefinitionClarificationValidatesDecisionBinding(t *testing.T) {
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	command := runtimeDefinitionClarificationCommandForTest()
	observer := magicevent.NewSimpleObserverWithMatchID("/orchestrator/consumers/runtime-definition-clarification-test", decisionevents.CommandDestination, hub)
	t.Cleanup(func() { observer.Unsubscribe(decisionevents.TopicOpenRuntimeDefinitionClarification) })
	observer.Subscribe(decisionevents.TopicOpenRuntimeDefinitionClarification, func(event magicevent.Event, result magicevent.Result) {
		received, ok := event.Data().(decisionevents.OpenRuntimeDefinitionClarificationRequestCommand)
		if !ok || event.Source() != ModuleID {
			t.Errorf("runtime clarification request = %#v source=%q", event.Data(), event.Source())
			return
		}
		result.Set(decisionevents.OpenRuntimeDefinitionClarificationRequestResult{
			TicketID: received.TicketID, WorkspaceID: received.WorkspaceID, RunID: received.RunID, AuthoritySnapshotID: received.AuthoritySnapshotID,
			ClarificationDispatchID: received.ClarificationDispatchID, Disposition: decisionevents.DefinitionClarificationCreated, ClarificationRequestBindingHash: received.BindingHash(),
		}, nil)
	})
	result, err := OpenRuntimeDefinitionClarification(context.Background(), hub, command)
	if err != nil {
		t.Fatalf("open runtime Definition clarification: %v", err)
	}
	if result.TicketID != command.TicketID || result.ClarificationRequestBindingHash != command.BindingHash() {
		t.Fatalf("runtime clarification result = %+v", result)
	}
}
