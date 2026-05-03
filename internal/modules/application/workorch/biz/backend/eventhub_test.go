package backend

import (
	"context"
	"testing"

	appservice "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestEventHubBackendRoutesWorkspaceCreateThroughCLICommand(t *testing.T) {
	ctx := context.Background()
	session, err := appservice.NewRuntimeSessionWithBackend("memory", "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer session.Close(ctx)
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("exports: %v", err)
	}

	be := NewEventHub(session, exports.Infrastructure.EventHub)
	ws, err := be.Workspace().Create(ctx, CreateWorkspaceRequest{
		Title:            "eventhub workspace",
		Description:      "created through cli command event",
		WorkingDirectory: "/tmp/eventhub-workspace",
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if ws == nil || ws.ID == "" {
		t.Fatalf("workspace = %+v, want id", ws)
	}

	publishedHub, ok := exports.Infrastructure.EventHub.(interface{ Published() []events.Envelope })
	if !ok {
		t.Fatal("event hub does not expose published envelopes")
	}
	var sawCommand, sawResult bool
	for _, envelope := range publishedHub.Published() {
		switch envelope.Topic {
		case events.TopicCLICommand:
			command, ok := events.DecodeCLICommandPayload(envelope.Payload)
			if ok && command.Command == events.CommandWorkspaceCreate {
				sawCommand = true
			}
		case events.TopicCLIResult:
			result, ok := events.DecodeCLIResultPayload(envelope.Payload)
			if ok && result.OK && result.CommandID != "" {
				sawResult = true
			}
		}
	}
	if !sawCommand || !sawResult {
		t.Fatalf("published command=%v result=%v, want both", sawCommand, sawResult)
	}
}

func TestEventHubBackendReadsWorkspaceSnapshotThroughCLICommand(t *testing.T) {
	ctx := context.Background()
	session, err := appservice.NewRuntimeSessionWithBackend("memory", "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer session.Close(ctx)
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("exports: %v", err)
	}

	be := NewEventHub(session, exports.Infrastructure.EventHub)
	ws, err := be.Workspace().Create(ctx, CreateWorkspaceRequest{Title: "snapshot workspace"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	snapshotBackend, ok := be.Workspace().(interface {
		Snapshot(context.Context, common.WorkspaceID) (*WorkspaceSnapshotInfo, error)
	})
	if !ok {
		t.Fatal("workspace backend does not expose snapshot reader")
	}
	snapshot, err := snapshotBackend.Snapshot(ctx, ws.ID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot == nil || snapshot.Workspace.ID != ws.ID {
		t.Fatalf("snapshot = %+v, want workspace %s", snapshot, ws.ID)
	}
	publishedHub, ok := exports.Infrastructure.EventHub.(interface{ Published() []events.Envelope })
	if !ok {
		t.Fatal("event hub does not expose published envelopes")
	}
	for _, envelope := range publishedHub.Published() {
		if envelope.Topic != events.TopicCLICommand {
			continue
		}
		command, ok := events.DecodeCLICommandPayload(envelope.Payload)
		if ok && command.Command == events.CommandWorkspaceSnapshot {
			return
		}
	}
	t.Fatal("workspace snapshot did not publish CLI command")
}

func TestEventHubBackendManagesWorkspaceMemoryThroughCLICommands(t *testing.T) {
	ctx := context.Background()
	session, err := appservice.NewRuntimeSessionWithBackend("memory", "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer session.Close(ctx)
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("exports: %v", err)
	}

	be := NewEventHub(session, exports.Infrastructure.EventHub)
	ws, err := be.Workspace().Create(ctx, CreateWorkspaceRequest{
		Title:            "memory workspace",
		WorkingDirectory: "/tmp/memory-workspace",
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	created, err := be.Memory().Create(ctx, ws.ID, &MemoryItemInfo{
		ID:         "memory-cli-1",
		SourceType: "user",
		MemoryType: "project_context",
		Summary:    "eventhub memory",
		Content:    "created through eventhub backend",
		Tags:       []string{"eventhub"},
		Importance: 50,
		Confidence: 50,
		Visibility: "workspace",
	})
	if err != nil {
		t.Fatalf("create memory: %v", err)
	}
	if created == nil || created.ID != "memory-cli-1" || created.WorkspaceID != ws.ID {
		t.Fatalf("created memory = %+v, want persisted item in workspace %s", created, ws.ID)
	}
	list, err := be.Memory().List(ctx, ws.ID)
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v, want created memory", list)
	}
	search, err := be.Memory().Search(ctx, ws.ID, MemorySearchRequest{Query: "eventhub"})
	if err != nil {
		t.Fatalf("search memory: %v", err)
	}
	if len(search) != 1 || search[0].ID != created.ID {
		t.Fatalf("search = %+v, want created memory", search)
	}
	got, err := be.Memory().Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if got == nil || got.Summary != "eventhub memory" {
		t.Fatalf("got = %+v, want created memory", got)
	}
	updated, err := be.Memory().Update(ctx, created.ID, &MemoryItemInfo{Summary: "updated eventhub memory"})
	if err != nil {
		t.Fatalf("update memory: %v", err)
	}
	if updated == nil || updated.Summary != "updated eventhub memory" {
		t.Fatalf("updated = %+v, want updated summary", updated)
	}
	if err := be.Memory().Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete memory: %v", err)
	}
	list, err = be.Memory().List(ctx, ws.ID)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("list after delete = %+v, want empty", list)
	}

	publishedHub, ok := exports.Infrastructure.EventHub.(interface{ Published() []events.Envelope })
	if !ok {
		t.Fatal("event hub does not expose published envelopes")
	}
	var sawCreate, sawUpdate, sawDelete bool
	for _, envelope := range publishedHub.Published() {
		switch envelope.Topic {
		case events.TopicMemoryCreated:
			sawCreate = true
		case events.TopicMemoryUpdated:
			sawUpdate = true
		case events.TopicMemoryDeleted:
			sawDelete = true
		}
	}
	if !sawCreate || !sawUpdate || !sawDelete {
		t.Fatalf("memory events create=%v update=%v delete=%v, want all", sawCreate, sawUpdate, sawDelete)
	}
}

func TestConvertEnvelopeAddsBlockingSubjectIDs(t *testing.T) {
	created := convertEnvelope(events.Envelope{
		Topic:       events.TopicWorkspaceCreated,
		WorkspaceID: "ws-created",
		Payload:     events.WorkspacePayload("created", "", "/tmp/created", "active", ""),
	})
	if created.Type != EventWorkspaceCreated || created.WorkspaceID != "ws-created" {
		t.Fatalf("created event = %+v, want workspace created", created)
	}

	deleted := convertEnvelope(events.Envelope{
		Topic:       events.TopicWorkspaceDeleted,
		WorkspaceID: "ws-deleted",
		Payload:     events.WorkspacePayload("deleted", "", "/tmp/deleted", "active", ""),
	})
	if deleted.Type != EventWorkspaceDeleted || deleted.WorkspaceID != "ws-deleted" {
		t.Fatalf("deleted event = %+v, want workspace deleted", deleted)
	}

	decision := convertEnvelope(events.Envelope{
		Topic:       events.TopicTicketCreated,
		SubjectID:   "ticket-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     events.DecisionTicketPayload("missing city", "open"),
	})
	if decision.Type != EventDecisionCreated || decision.Payload["ticketId"] != "ticket-1" {
		t.Fatalf("decision event = %+v, want ticketId from subject", decision)
	}

	approval := convertEnvelope(events.Envelope{
		Topic:       events.TopicApprovalCreated,
		SubjectID:   "approval-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     events.ApprovalPayload("approve shell", "pending"),
	})
	if approval.Type != EventApprovalCreated || approval.Payload["approvalId"] != "approval-1" {
		t.Fatalf("approval event = %+v, want approvalId from subject", approval)
	}
}
