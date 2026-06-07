package workspaceports

import (
	"context"
	"testing"

	approvalcontracts "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/contracts"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreamcontracts "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/contracts"
	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	orchestratorcontracts "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/contracts"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func TestCreateWorkspaceRecordPublishesCommandPayload(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(events.TopicWorkspaceCommandCreate, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(envelope)
	})

	workspaceID := common.WorkspaceID("ws-command-payload")
	if err := CreateWorkspaceRecord(ctx, hub, workspaceID, "command workspace", "desc", "/tmp/workorch/ws-command-payload", "/tmp/workorch/ws-command-payload/.workorch"); err != nil {
		t.Fatalf("CreateWorkspaceRecord: %v", err)
	}

	published := hub.Published()
	if len(published) != 1 {
		t.Fatalf("published len = %d, want 1", len(published))
	}
	cmd, ok := published[0].Payload[events.PayloadKeyCommand].(workspacecontracts.CreateWorkspaceCommand)
	if !ok {
		t.Fatalf("payload command = %T, want CreateWorkspaceCommand", published[0].Payload[events.PayloadKeyCommand])
	}
	if cmd.WorkspaceID != workspaceID || cmd.Title != "command workspace" || cmd.DeliverableDirectory == "" || cmd.InternalDirectory == "" {
		t.Fatalf("command = %+v, want populated create workspace command", cmd)
	}
	if _, ok := published[0].Payload[events.PayloadKeyWorkspaceID]; ok {
		t.Fatalf("payload contains legacy workspaceId field")
	}
}

func TestWorkspaceTemplateRecordsPublishCommandPayloads(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	for _, topic := range []string{
		events.TopicWorkspaceTemplateCommandCreate,
		events.TopicWorkspaceTemplateCommandUpdate,
		events.TopicWorkspaceTemplateCommandDelete,
	} {
		hub.Subscribe(topic, func(_ context.Context, envelope events.Envelope) error {
			return events.Response(envelope)
		})
	}
	template := &repository.WorkspaceTemplate{
		ID:               "template-command-payload",
		Name:             "Template Command Payload",
		GoalText:         "ship",
		DoDItems:         []string{"done"},
		DefaultSoulID:    "soul-default",
		SoulBindings:     map[string]string{"execution": "soul-exec"},
		ContextHints:     map[string]any{"managedConfig": true},
		EvidencePolicies: []map[string]any{{"kind": "validation"}},
	}

	if err := CreateWorkspaceTemplateRecord(ctx, hub, template); err != nil {
		t.Fatalf("CreateWorkspaceTemplateRecord: %v", err)
	}
	if err := UpdateWorkspaceTemplateRecord(ctx, hub, template); err != nil {
		t.Fatalf("UpdateWorkspaceTemplateRecord: %v", err)
	}
	if err := DeleteWorkspaceTemplateRecord(ctx, hub, template.ID); err != nil {
		t.Fatalf("DeleteWorkspaceTemplateRecord: %v", err)
	}

	createEnvelope := publishedByTopic(t, hub, events.TopicWorkspaceTemplateCommandCreate)
	createCmd, ok := createEnvelope.Payload[events.PayloadKeyCommand].(workspacecontracts.CreateWorkspaceTemplateCommand)
	if !ok {
		t.Fatalf("create payload command = %T, want CreateWorkspaceTemplateCommand", createEnvelope.Payload[events.PayloadKeyCommand])
	}
	if createCmd.TemplateID != template.ID || createCmd.Name != template.Name || createCmd.SoulBindings["execution"] != "soul-exec" {
		t.Fatalf("create command = %+v, want populated template command", createCmd)
	}
	assertNoWorkspaceTemplateLegacyPayload(t, createEnvelope)

	updateEnvelope := publishedByTopic(t, hub, events.TopicWorkspaceTemplateCommandUpdate)
	updateCmd, ok := updateEnvelope.Payload[events.PayloadKeyCommand].(workspacecontracts.UpdateWorkspaceTemplateCommand)
	if !ok {
		t.Fatalf("update payload command = %T, want UpdateWorkspaceTemplateCommand", updateEnvelope.Payload[events.PayloadKeyCommand])
	}
	if updateCmd.TemplateID != template.ID || updateCmd.Name != template.Name || updateCmd.ContextHints["managedConfig"] != true {
		t.Fatalf("update command = %+v, want populated template command", updateCmd)
	}
	assertNoWorkspaceTemplateLegacyPayload(t, updateEnvelope)

	deleteEnvelope := publishedByTopic(t, hub, events.TopicWorkspaceTemplateCommandDelete)
	deleteCmd, ok := deleteEnvelope.Payload[events.PayloadKeyCommand].(workspacecontracts.DeleteWorkspaceTemplateCommand)
	if !ok || deleteCmd.TemplateID != template.ID {
		t.Fatalf("delete payload command = %#v, want DeleteWorkspaceTemplateCommand(%s)", deleteEnvelope.Payload[events.PayloadKeyCommand], template.ID)
	}
	assertNoWorkspaceTemplateLegacyPayload(t, deleteEnvelope)
}

func TestPurgeWorkspaceRecordsPublishesDeleteCommandPayloads(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-purge-command-payload")
	runID := common.RunID("run-purge-command-payload")
	memoryID := common.MemoryID("mem-purge-command-payload")
	ticketID := common.TicketID("ticket-purge-command-payload")
	approvalID := common.ApprovalID("approval-purge-command-payload")

	hub.Subscribe(events.TopicRunCommandListByWorkspace, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{Payload: map[string]any{
			events.PayloadKeyRuns: []*runmodel.Run{{ID: runID, WorkspaceID: workspaceID}},
		}})
	})
	hub.Subscribe(events.TopicOrchestratorCommandPurgeRun, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(envelope)
	})
	hub.Subscribe(events.TopicMemoryCommandListByWorkspace, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{Payload: map[string]any{
			events.PayloadKeyMemoryItems: []*memorymodel.MemoryItem{{ID: memoryID, WorkspaceID: workspaceID, RunID: runID}},
		}})
	})
	hub.Subscribe(events.TopicDecisionCommandListTicketsByWS, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{Payload: map[string]any{
			events.PayloadKeyTickets: []*decisionmodel.Ticket{{ID: ticketID, WorkspaceID: workspaceID, RunID: runID}},
		}})
	})
	hub.Subscribe(events.TopicApprovalCommandListByWorkspace, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{Payload: map[string]any{
			events.PayloadKeyItems: []*approvalmodel.Approval{{ID: approvalID, WorkspaceID: workspaceID, RunID: runID}},
		}})
	})
	hub.Subscribe(events.TopicArtifactCommandListByWorkspace, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{Payload: map[string]any{
			events.PayloadKeyItems: []*resultmodel.Artifact{},
		}})
	})
	hub.Subscribe(events.TopicResultCommandListByWorkspace, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{Payload: map[string]any{
			events.PayloadKeyItems: []*resultmodel.Result{},
		}})
	})
	for _, topic := range []string{
		events.TopicMemoryCommandDelete,
		events.TopicDecisionCommandDeleteResolution,
		events.TopicDecisionCommandDeleteTicket,
		events.TopicApprovalCommandDelete,
		events.TopicRunCommandDelete,
	} {
		hub.Subscribe(topic, func(_ context.Context, envelope events.Envelope) error {
			return events.Response(envelope)
		})
	}

	if err := PurgeWorkspaceRecords(ctx, hub, workspaceID); err != nil {
		t.Fatalf("PurgeWorkspaceRecords: %v", err)
	}

	orchestratorPurge := publishedByTopic(t, hub, events.TopicOrchestratorCommandPurgeRun)
	orchestratorCmd, ok := orchestratorPurge.Payload[events.PayloadKeyCommand].(orchestratorcontracts.PurgeRunCommand)
	if !ok || orchestratorCmd.RunID != runID {
		t.Fatalf("orchestrator purge command = %#v, want PurgeRunCommand(%s)", orchestratorPurge.Payload[events.PayloadKeyCommand], runID)
	}
	if _, ok := orchestratorPurge.Payload[events.PayloadKeyRunID]; ok {
		t.Fatalf("orchestrator purge payload contains legacy runId field")
	}

	memoryDelete := publishedByTopic(t, hub, events.TopicMemoryCommandDelete)
	memoryCmd, ok := memoryDelete.Payload[events.PayloadKeyCommand].(memorycontracts.DeleteMemoryItemCommand)
	if !ok || memoryCmd.MemoryID != memoryID {
		t.Fatalf("memory delete command = %#v, want DeleteMemoryItemCommand(%s)", memoryDelete.Payload[events.PayloadKeyCommand], memoryID)
	}
	if _, ok := memoryDelete.Payload[events.PayloadKeyMemoryID]; ok {
		t.Fatalf("memory delete payload contains legacy memoryId field")
	}

	resolutionDelete := publishedByTopic(t, hub, events.TopicDecisionCommandDeleteResolution)
	resolutionCmd, ok := resolutionDelete.Payload[events.PayloadKeyCommand].(decisioncontracts.DeleteResolutionCommand)
	if !ok || resolutionCmd.TicketID != ticketID {
		t.Fatalf("resolution delete command = %#v, want DeleteResolutionCommand(%s)", resolutionDelete.Payload[events.PayloadKeyCommand], ticketID)
	}
	if _, ok := resolutionDelete.Payload[events.PayloadKeyBlockingTicketID]; ok {
		t.Fatalf("resolution delete payload contains legacy blockingTicketId field")
	}

	ticketDelete := publishedByTopic(t, hub, events.TopicDecisionCommandDeleteTicket)
	ticketCmd, ok := ticketDelete.Payload[events.PayloadKeyCommand].(decisioncontracts.DeleteTicketCommand)
	if !ok || ticketCmd.TicketID != ticketID {
		t.Fatalf("ticket delete command = %#v, want DeleteTicketCommand(%s)", ticketDelete.Payload[events.PayloadKeyCommand], ticketID)
	}
	if _, ok := ticketDelete.Payload[events.PayloadKeyBlockingTicketID]; ok {
		t.Fatalf("ticket delete payload contains legacy blockingTicketId field")
	}

	approvalDelete := publishedByTopic(t, hub, events.TopicApprovalCommandDelete)
	approvalCmd, ok := approvalDelete.Payload[events.PayloadKeyCommand].(approvalcontracts.DeleteApprovalCommand)
	if !ok || approvalCmd.ApprovalID != approvalID {
		t.Fatalf("approval delete command = %#v, want DeleteApprovalCommand(%s)", approvalDelete.Payload[events.PayloadKeyCommand], approvalID)
	}
	if _, ok := approvalDelete.Payload[events.PayloadKeyApprovalID]; ok {
		t.Fatalf("approval delete payload contains legacy approvalId field")
	}

	runDelete := publishedByTopic(t, hub, events.TopicRunCommandDelete)
	runCmd, ok := runDelete.Payload[events.PayloadKeyCommand].(runcontracts.DeleteRunCommand)
	if !ok || runCmd.RunID != runID {
		t.Fatalf("run delete command = %#v, want DeleteRunCommand(%s)", runDelete.Payload[events.PayloadKeyCommand], runID)
	}
	if _, ok := runDelete.Payload[events.PayloadKeyRunID]; ok {
		t.Fatalf("run delete payload contains legacy runId field")
	}
}

func assertNoWorkspaceTemplateLegacyPayload(t *testing.T, envelope events.Envelope) {
	t.Helper()
	if _, ok := envelope.Payload[events.PayloadKeyWorkspaceTemplate]; ok {
		t.Fatalf("payload contains legacy workspaceTemplate field")
	}
	if _, ok := envelope.Payload[events.PayloadKeyTemplateID]; ok {
		t.Fatalf("payload contains legacy templateId field")
	}
}

func publishedByTopic(t *testing.T, hub *events.MemoryHub, topic string) events.Envelope {
	t.Helper()
	for _, item := range hub.Published() {
		if item.Topic == topic {
			return item
		}
	}
	t.Fatalf("published topic %s not found", topic)
	return events.Envelope{}
}

func TestDeleteWorkspaceRecordPublishesCommandPayload(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(events.TopicWorkspaceCommandDelete, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(envelope)
	})

	workspaceID := common.WorkspaceID("ws-delete-command-payload")
	if err := DeleteWorkspaceRecord(ctx, hub, workspaceID); err != nil {
		t.Fatalf("DeleteWorkspaceRecord: %v", err)
	}

	published := hub.Published()
	if len(published) != 1 {
		t.Fatalf("published len = %d, want 1", len(published))
	}
	cmd, ok := published[0].Payload[events.PayloadKeyCommand].(workspacecontracts.DeleteWorkspaceCommand)
	if !ok {
		t.Fatalf("payload command = %T, want DeleteWorkspaceCommand", published[0].Payload[events.PayloadKeyCommand])
	}
	if cmd.WorkspaceID != workspaceID {
		t.Fatalf("command workspaceID = %s, want %s", cmd.WorkspaceID, workspaceID)
	}
	if _, ok := published[0].Payload[events.PayloadKeyWorkspaceID]; ok {
		t.Fatalf("payload contains legacy workspaceId field")
	}
}

func TestPurgeWorkspaceEventStreamPublishesCommandPayload(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(events.TopicEventStreamCommandPurgeWorkspace, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(envelope)
	})

	workspaceID := common.WorkspaceID("ws-eventstream-purge-command")
	if err := PurgeWorkspaceEventStream(ctx, hub, workspaceID); err != nil {
		t.Fatalf("PurgeWorkspaceEventStream: %v", err)
	}

	published := hub.Published()
	if len(published) != 1 {
		t.Fatalf("published len = %d, want 1", len(published))
	}
	cmd, ok := published[0].Payload[events.PayloadKeyCommand].(eventstreamcontracts.PurgeWorkspaceCommand)
	if !ok || cmd.WorkspaceID != workspaceID {
		t.Fatalf("payload command = %#v, want PurgeWorkspaceCommand(%s)", published[0].Payload[events.PayloadKeyCommand], workspaceID)
	}
	if _, ok := published[0].Payload[events.PayloadKeyWorkspaceID]; ok {
		t.Fatalf("payload contains legacy workspaceId field")
	}
}
