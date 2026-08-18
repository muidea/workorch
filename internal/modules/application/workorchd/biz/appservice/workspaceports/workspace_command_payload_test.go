package workspaceports

import (
	"context"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type workspaceCommandTestHub struct {
	raw       magicevent.Hub
	observers map[string]magicevent.SimpleObserver
	captured  map[string][]any
}

func newWorkspaceCommandTestHub(t *testing.T) *workspaceCommandTestHub {
	t.Helper()
	raw := magicevent.NewHub(64)
	ret := &workspaceCommandTestHub{
		raw:       raw,
		observers: map[string]magicevent.SimpleObserver{},
		captured:  map[string][]any{},
	}
	t.Cleanup(func() { raw.Terminate(context.Background()) })
	return ret
}

func (h *workspaceCommandTestHub) subscribe(destination string, topic string, handler func(any) (any, error)) {
	observer := h.observers[destination]
	if observer == nil {
		observer = magicevent.NewSimpleObserver(destination, h.raw)
		h.observers[destination] = observer
	}
	observer.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
		h.captured[topic] = append(h.captured[topic], ev.Data())
		data, err := handler(ev.Data())
		if err != nil {
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
			return
		}
		result.Set(data, nil)
	})
}

func (h *workspaceCommandTestHub) command(t *testing.T, topic string, index int) any {
	t.Helper()
	items := h.captured[topic]
	if index < 0 || index >= len(items) {
		t.Fatalf("captured topic %s index %d not found; count=%d", topic, index, len(items))
	}
	return items[index]
}

func TestCreateWorkspaceRecordSendsOwnerCommand(t *testing.T) {
	ctx := context.Background()
	testHub := newWorkspaceCommandTestHub(t)
	testHub.subscribe(workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandCreate, func(any) (any, error) {
		return workspaceevents.WorkspaceCommandResult{}, nil
	})

	workspaceID := common.WorkspaceID("ws-command-payload")
	if err := CreateWorkspaceRecord(ctx, testHub.raw, workspaceID, "command workspace", "desc", "/tmp/workorch/ws-command-payload", "/tmp/workorch/ws-command-payload/.workorch"); err != nil {
		t.Fatalf("CreateWorkspaceRecord: %v", err)
	}

	cmd, ok := testHub.command(t, workspaceevents.TopicWorkspaceCommandCreate, 0).(workspaceevents.CreateWorkspaceCommand)
	if !ok {
		t.Fatalf("command = %T, want CreateWorkspaceCommand", testHub.command(t, workspaceevents.TopicWorkspaceCommandCreate, 0))
	}
	if cmd.WorkspaceID != workspaceID || cmd.Title != "command workspace" || cmd.DeliverableDirectory == "" || cmd.InternalDirectory == "" {
		t.Fatalf("command = %+v, want populated create workspace command", cmd)
	}
}

func TestWorkspaceTemplateRecordsSendOwnerCommands(t *testing.T) {
	ctx := context.Background()
	testHub := newWorkspaceCommandTestHub(t)
	for _, topic := range []string{
		workspaceevents.TopicWorkspaceTemplateCommandCreate,
		workspaceevents.TopicWorkspaceTemplateCommandUpdate,
		workspaceevents.TopicWorkspaceTemplateCommandDelete,
	} {
		testHub.subscribe(workspaceevents.CommandDestination, topic, func(any) (any, error) {
			return workspaceevents.WorkspaceCommandResult{}, nil
		})
	}
	template := &workspacemodel.WorkspaceTemplate{
		ID:            "template-command-payload",
		Name:          "Template Command Payload",
		GoalText:      "ship",
		DoDItems:      []string{"done"},
		DefaultSoulID: "soul-default",
		SoulBindings:  map[string]string{"execution": "soul-exec"},
		ContextHints: workorcheventpayload.New(
			workorcheventpayload.BoolField("managedConfig", true),
		),
		EvidencePolicies: []workorcheventpayload.Payload{workorcheventpayload.New(
			workorcheventpayload.StringField("kind", "validation"),
		)},
	}

	if err := CreateWorkspaceTemplateRecord(ctx, testHub.raw, template); err != nil {
		t.Fatalf("CreateWorkspaceTemplateRecord: %v", err)
	}
	if err := UpdateWorkspaceTemplateRecord(ctx, testHub.raw, template); err != nil {
		t.Fatalf("UpdateWorkspaceTemplateRecord: %v", err)
	}
	if err := DeleteWorkspaceTemplateRecord(ctx, testHub.raw, template.ID); err != nil {
		t.Fatalf("DeleteWorkspaceTemplateRecord: %v", err)
	}

	createCmd, ok := testHub.command(t, workspaceevents.TopicWorkspaceTemplateCommandCreate, 0).(workspaceevents.CreateWorkspaceTemplateCommand)
	if !ok || createCmd.TemplateID != template.ID || createCmd.Name != template.Name {
		t.Fatalf("create command = %#v, want template %s", createCmd, template.ID)
	}
	updateCmd, ok := testHub.command(t, workspaceevents.TopicWorkspaceTemplateCommandUpdate, 0).(workspaceevents.UpdateWorkspaceTemplateCommand)
	if !ok || updateCmd.TemplateID != template.ID || updateCmd.Name != template.Name {
		t.Fatalf("update command = %#v, want template %s", updateCmd, template.ID)
	}
	deleteCmd, ok := testHub.command(t, workspaceevents.TopicWorkspaceTemplateCommandDelete, 0).(workspaceevents.DeleteWorkspaceTemplateCommand)
	if !ok || deleteCmd.TemplateID != template.ID {
		t.Fatalf("delete command = %#v, want template %s", deleteCmd, template.ID)
	}
}

func TestPurgeWorkspaceRecordsSendsOwnerCommands(t *testing.T) {
	ctx := context.Background()
	testHub := newWorkspaceCommandTestHub(t)
	workspaceID := common.WorkspaceID("ws-purge-command-payload")
	runID := common.RunID("run-purge-command-payload")
	memoryID := common.MemoryID("memory-purge-command-payload")
	ticketID := common.TicketID("ticket-purge-command-payload")
	approvalID := common.ApprovalID("approval-purge-command-payload")

	testHub.subscribe(runevents.CommandDestination, runevents.TopicRunCommandListByWorkspace, func(any) (any, error) {
		return runevents.ListRunsByWorkspaceResult{Runs: []*runmodel.Run{{ID: runID, WorkspaceID: workspaceID}}}, nil
	})
	testHub.subscribe(orchestratorevents.CommandDestination, orchestratorevents.TopicOrchestratorCommandPurgeRun, emptyOrchestratorResult)
	testHub.subscribe(memoryevents.CommandDestination, memoryevents.TopicMemoryCommandListByWorkspace, func(any) (any, error) {
		return memoryevents.MemoryCommandResult{Items: []*memorymodel.MemoryItem{{ID: memoryID, WorkspaceID: workspaceID, RunID: runID}}}, nil
	})
	testHub.subscribe(decisionevents.CommandDestination, decisionevents.TopicDecisionCommandListTicketsByWS, func(any) (any, error) {
		return decisionevents.DecisionCommandResult{Tickets: []*decisionmodel.Ticket{{ID: ticketID, WorkspaceID: workspaceID, RunID: runID}}}, nil
	})
	testHub.subscribe(approvalevents.CommandDestination, approvalevents.TopicApprovalCommandListByWorkspace, func(any) (any, error) {
		return approvalevents.ApprovalCommandResult{Approvals: []*approvalmodel.Approval{{ID: approvalID, WorkspaceID: workspaceID, RunID: runID}}}, nil
	})
	testHub.subscribe(resultevents.CommandDestination, resultevents.TopicArtifactCommandListByWorkspace, func(any) (any, error) {
		return resultevents.ListArtifactsByWorkspaceResult{Artifacts: []*resultmodel.Artifact{}}, nil
	})
	testHub.subscribe(resultevents.CommandDestination, resultevents.TopicResultCommandListByWorkspace, func(any) (any, error) {
		return resultevents.ListResultsByWorkspaceResult{Results: []*resultmodel.Result{}}, nil
	})
	testHub.subscribe(memoryevents.CommandDestination, memoryevents.TopicMemoryCommandDelete, emptyMemoryResult)
	testHub.subscribe(decisionevents.CommandDestination, decisionevents.TopicDecisionCommandDeleteResolution, emptyDecisionResult)
	testHub.subscribe(decisionevents.CommandDestination, decisionevents.TopicDecisionCommandDeleteTicket, emptyDecisionResult)
	testHub.subscribe(approvalevents.CommandDestination, approvalevents.TopicApprovalCommandDelete, emptyApprovalResult)
	testHub.subscribe(runevents.CommandDestination, runevents.TopicRunCommandDelete, emptyRunResult)

	if err := PurgeWorkspaceRecords(ctx, testHub.raw, workspaceID); err != nil {
		t.Fatalf("PurgeWorkspaceRecords: %v", err)
	}

	orchestratorCmd, ok := testHub.command(t, orchestratorevents.TopicOrchestratorCommandPurgeRun, 0).(orchestratorevents.PurgeRunCommand)
	if !ok || orchestratorCmd.RunID != runID {
		t.Fatalf("orchestrator purge command = %#v, want run %s", orchestratorCmd, runID)
	}
	memoryCmd, ok := testHub.command(t, memoryevents.TopicMemoryCommandDelete, 0).(memoryevents.DeleteMemoryCommand)
	if !ok || memoryCmd.MemoryID != memoryID {
		t.Fatalf("memory delete command = %#v, want memory %s", memoryCmd, memoryID)
	}
	resolutionCmd, ok := testHub.command(t, decisionevents.TopicDecisionCommandDeleteResolution, 0).(decisionevents.DeleteResolutionCommand)
	if !ok || resolutionCmd.TicketID != ticketID {
		t.Fatalf("resolution delete command = %#v, want ticket %s", resolutionCmd, ticketID)
	}
	ticketCmd, ok := testHub.command(t, decisionevents.TopicDecisionCommandDeleteTicket, 0).(decisionevents.DeleteTicketCommand)
	if !ok || ticketCmd.TicketID != ticketID {
		t.Fatalf("ticket delete command = %#v, want ticket %s", ticketCmd, ticketID)
	}
	approvalCmd, ok := testHub.command(t, approvalevents.TopicApprovalCommandDelete, 0).(approvalevents.DeleteApprovalCommand)
	if !ok || approvalCmd.ApprovalID != approvalID {
		t.Fatalf("approval delete command = %#v, want approval %s", approvalCmd, approvalID)
	}
	runCmd, ok := testHub.command(t, runevents.TopicRunCommandDelete, 0).(runevents.DeleteRunCommand)
	if !ok || runCmd.RunID != runID {
		t.Fatalf("run delete command = %#v, want run %s", runCmd, runID)
	}
}

func TestPurgeWorkspaceRecordsTreatsMissingChildRecordsAsAlreadyPurged(t *testing.T) {
	ctx := context.Background()
	testHub := newWorkspaceCommandTestHub(t)
	workspaceID := common.WorkspaceID("ws-purge-missing-children")
	runID := common.RunID("run-purge-missing-children")
	resultListCalled := false

	testHub.subscribe(runevents.CommandDestination, runevents.TopicRunCommandListByWorkspace, func(any) (any, error) {
		return runevents.ListRunsByWorkspaceResult{Runs: []*runmodel.Run{{ID: runID, WorkspaceID: workspaceID}}}, nil
	})
	for _, command := range []struct {
		destination string
		topic       string
	}{
		{orchestratorevents.CommandDestination, orchestratorevents.TopicOrchestratorCommandPurgeRun},
		{memoryevents.CommandDestination, memoryevents.TopicMemoryCommandListByWorkspace},
		{decisionevents.CommandDestination, decisionevents.TopicDecisionCommandListTicketsByWS},
		{approvalevents.CommandDestination, approvalevents.TopicApprovalCommandListByWorkspace},
		{resultevents.CommandDestination, resultevents.TopicArtifactCommandListByWorkspace},
		{runevents.CommandDestination, runevents.TopicRunCommandDelete},
	} {
		testHub.subscribe(command.destination, command.topic, func(any) (any, error) { return nil, common.ErrNotFound })
	}
	testHub.subscribe(resultevents.CommandDestination, resultevents.TopicResultCommandListByWorkspace, func(any) (any, error) {
		resultListCalled = true
		return nil, common.ErrNotFound
	})

	if err := PurgeWorkspaceRecords(ctx, testHub.raw, workspaceID); err != nil {
		t.Fatalf("PurgeWorkspaceRecords err = %v, want nil", err)
	}
	if !resultListCalled {
		t.Fatalf("result purge was skipped after missing artifact records")
	}
}

func TestPurgeWorkspaceEventStreamTreatsMissingRecordsAsAlreadyPurged(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "not_found", err: common.ErrNotFound},
		{name: "conflict", err: common.ErrConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testHub := newWorkspaceCommandTestHub(t)
			testHub.subscribe(eventstreamevents.CommandDestination, eventstreamevents.TopicEventStreamCommandPurgeWorkspace, func(any) (any, error) {
				return nil, tc.err
			})
			if err := PurgeWorkspaceEventStream(context.Background(), testHub.raw, "ws-eventstream-already-purged"); err != nil {
				t.Fatalf("PurgeWorkspaceEventStream err = %v, want nil", err)
			}
		})
	}
}

func TestDeleteWorkspaceViewDoesNotFailAfterWorkspaceDeletedWhenPostPurgeCleanupIsAlreadyGone(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "not_found", err: common.ErrNotFound},
		{name: "conflict", err: common.ErrConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testHub := newWorkspaceCommandTestHub(t)
			workspaceID := common.WorkspaceID("ws-delete-post-purge-" + tc.name)
			testHub.subscribe(workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandGet, func(any) (any, error) {
				return workspaceevents.WorkspaceCommandResult{Workspace: &workspaceevents.Workspace{ID: workspaceID}}, nil
			})
			testHub.subscribe(runevents.CommandDestination, runevents.TopicRunCommandListByWorkspace, func(any) (any, error) {
				return runevents.ListRunsByWorkspaceResult{Runs: []*runmodel.Run{}}, nil
			})
			testHub.subscribe(memoryevents.CommandDestination, memoryevents.TopicMemoryCommandListByWorkspace, func(any) (any, error) {
				return memoryevents.MemoryCommandResult{Items: []*memorymodel.MemoryItem{}}, nil
			})
			testHub.subscribe(decisionevents.CommandDestination, decisionevents.TopicDecisionCommandListTicketsByWS, func(any) (any, error) {
				return decisionevents.DecisionCommandResult{Tickets: []*decisionmodel.Ticket{}}, nil
			})
			testHub.subscribe(approvalevents.CommandDestination, approvalevents.TopicApprovalCommandListByWorkspace, func(any) (any, error) {
				return approvalevents.ApprovalCommandResult{Approvals: []*approvalmodel.Approval{}}, nil
			})
			testHub.subscribe(resultevents.CommandDestination, resultevents.TopicArtifactCommandListByWorkspace, func(any) (any, error) {
				return resultevents.ListArtifactsByWorkspaceResult{Artifacts: []*resultmodel.Artifact{}}, nil
			})
			testHub.subscribe(resultevents.CommandDestination, resultevents.TopicResultCommandListByWorkspace, func(any) (any, error) {
				return resultevents.ListResultsByWorkspaceResult{Results: []*resultmodel.Result{}}, nil
			})
			testHub.subscribe(workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandDelete, func(any) (any, error) {
				return workspaceevents.WorkspaceCommandResult{}, nil
			})
			testHub.subscribe(eventstreamevents.CommandDestination, eventstreamevents.TopicEventStreamCommandPurgeWorkspace, func(any) (any, error) {
				return nil, tc.err
			})

			err := DeleteWorkspaceView(context.Background(), testHub.raw, DeleteWorkspaceViewRequest{
				WorkspaceID: string(workspaceID),
				Command:     appmodelcore.DeleteWorkspaceCommand{Purge: true},
			})
			if err != nil {
				t.Fatalf("DeleteWorkspaceView err = %v, want nil", err)
			}
		})
	}
}

func TestDeleteWorkspaceRecordSendsOwnerCommand(t *testing.T) {
	testHub := newWorkspaceCommandTestHub(t)
	testHub.subscribe(workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandDelete, func(any) (any, error) {
		return workspaceevents.WorkspaceCommandResult{}, nil
	})
	workspaceID := common.WorkspaceID("ws-delete-command-payload")
	if err := DeleteWorkspaceRecord(context.Background(), testHub.raw, workspaceID); err != nil {
		t.Fatalf("DeleteWorkspaceRecord: %v", err)
	}
	cmd, ok := testHub.command(t, workspaceevents.TopicWorkspaceCommandDelete, 0).(workspaceevents.DeleteWorkspaceCommand)
	if !ok || cmd.WorkspaceID != workspaceID {
		t.Fatalf("command = %#v, want DeleteWorkspaceCommand(%s)", cmd, workspaceID)
	}
}

func TestPurgeWorkspaceEventStreamSendsOwnerCommand(t *testing.T) {
	testHub := newWorkspaceCommandTestHub(t)
	testHub.subscribe(eventstreamevents.CommandDestination, eventstreamevents.TopicEventStreamCommandPurgeWorkspace, func(any) (any, error) {
		return eventstreamevents.EventStreamCommandResult{}, nil
	})
	workspaceID := common.WorkspaceID("ws-eventstream-purge-command")
	if err := PurgeWorkspaceEventStream(context.Background(), testHub.raw, workspaceID); err != nil {
		t.Fatalf("PurgeWorkspaceEventStream: %v", err)
	}
	cmd, ok := testHub.command(t, eventstreamevents.TopicEventStreamCommandPurgeWorkspace, 0).(eventstreamevents.PurgeWorkspaceCommand)
	if !ok || cmd.WorkspaceID != workspaceID {
		t.Fatalf("command = %#v, want PurgeWorkspaceCommand(%s)", cmd, workspaceID)
	}
}

func emptyOrchestratorResult(any) (any, error) {
	return orchestratorevents.OrchestratorCommandResult{}, nil
}

func emptyMemoryResult(any) (any, error) {
	return memoryevents.MemoryCommandResult{}, nil
}

func emptyDecisionResult(any) (any, error) {
	return decisionevents.DecisionCommandResult{}, nil
}

func emptyApprovalResult(any) (any, error) {
	return approvalevents.ApprovalCommandResult{}, nil
}

func emptyRunResult(any) (any, error) {
	return runevents.DeleteRunResult{Deleted: true}, nil
}
