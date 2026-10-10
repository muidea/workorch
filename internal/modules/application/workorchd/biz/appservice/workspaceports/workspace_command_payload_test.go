package workspaceports

import (
	"context"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type workspaceCommandTestHub struct {
	raw       magicevent.Hub
	observers map[string]magicevent.SimpleObserver
	captured  map[string][]any
	sources   map[string][]string
}

func newWorkspaceCommandTestHub(t *testing.T) *workspaceCommandTestHub {
	t.Helper()
	raw := magicevent.NewHub(64)
	ret := &workspaceCommandTestHub{
		raw:       raw,
		observers: map[string]magicevent.SimpleObserver{},
		captured:  map[string][]any{},
		sources:   map[string][]string{},
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
		h.sources[topic] = append(h.sources[topic], ev.Source())
		data, err := handler(ev.Data())
		if err != nil {
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
			return
		}
		result.Set(data, nil)
	})
}

func (h *workspaceCommandTestHub) source(t *testing.T, topic string, index int) string {
	t.Helper()
	items := h.sources[topic]
	if index < 0 || index >= len(items) {
		t.Fatalf("captured source for topic %s index %d not found; count=%d", topic, index, len(items))
	}
	return items[index]
}

func TestSubmitDefinitionAdmissionIntentUsesRuntimeAuthoritySource(t *testing.T) {
	ctx := context.Background()
	testHub := newWorkspaceCommandTestHub(t)
	candidate := (workspacemodel.DefinitionAdmissionCandidate{
		SchemaVersion:     workspacemodel.DefinitionAdmissionCandidateSchemaVersionCurrent,
		WorkspaceID:       "ws-definition-admission-source",
		CandidateID:       "workspace-definition",
		CandidateRevision: 1,
		GoalInput: workspacemodel.DefinitionCandidateItem{
			CandidateItemRef: "goal:goal-source",
			Text:             "verify definition admission source",
			SourceIdentity:   "goal:goal-source@revision-1",
		},
		DoDInputs: []workspacemodel.DefinitionCandidateItem{{
			CandidateItemRef: "dod:dod-source:item:000001",
			Text:             "candidate is accepted",
			SourceIdentity:   "dod:dod-source@revision-1",
		}},
		WorkspaceTemplateSnapshot: workspacemodel.DefinitionTemplateSnapshot{
			TemplateRef: "template-source", Revision: "template:revision-1", SnapshotHash: "template-hash",
		},
		RuntimeProfileContractsHash:       runtimecontract.StableRuntimeProfileContractsHash(nil),
		AdmissionCompilerVersion:          workspacemodel.DefinitionAdmissionCompilerVersionCurrent,
		ResolvedProjectWorkspaceDirectory: "/tmp/workorch/ws-definition-admission-source",
		ProposedRelationInputs: []workspacemodel.DefinitionProposedRelationInput{{
			RelationRef: "terminal-all-of", RelationKind: "all_of",
			SourceItemRefs: []string{"dod:dod-source:item:000001"}, TargetItemRefs: []string{"goal:goal-source"},
		}},
	}).WithHash()
	testHub.subscribe(workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceDefinitionAdmissionIntent, func(data any) (any, error) {
		command := data.(workspaceevents.DefinitionAdmissionIntentCommand)
		startup := (workspacemodel.WorkspaceStartupInputContract{
			WorkspaceID: command.Candidate.WorkspaceID, DefinitionRevision: command.Candidate.GoalInput.SourceIdentity + "|" + command.Candidate.DoDInputs[0].SourceIdentity,
			GoalDefinitionRef: "goal-item", GoalTextHash: workspacemodel.StableDefinitionTextHash(command.Candidate.GoalInput.Text),
			DoDDefinitionRefs: []string{"dod-item"}, DoDItemSetHash: workspacemodel.StableDefinitionStringSetHash([]string{command.Candidate.DoDInputs[0].Text}),
			WorkspaceTemplateRef: command.Candidate.WorkspaceTemplateSnapshot.TemplateRef, TemplateRevision: command.Candidate.WorkspaceTemplateSnapshot.Revision,
			TemplateSnapshotHash: command.Candidate.WorkspaceTemplateSnapshot.SnapshotHash, RuntimeProfileContractsHash: command.Candidate.RuntimeProfileContractsHash,
			ResolvedProjectWorkspaceDirectory: command.Candidate.ResolvedProjectWorkspaceDirectory,
			CandidateItemToItemRefMappings:    []workspacemodel.CandidateItemToItemRefMapping{{CandidateItemRef: command.Candidate.GoalInput.CandidateItemRef, ItemRef: "goal-item"}, {CandidateItemRef: command.Candidate.DoDInputs[0].CandidateItemRef, ItemRef: "dod-item"}},
		}).WithHash()
		activation := workspacemodel.WorkspaceActivationAdmission{
			WorkspaceID: command.Candidate.WorkspaceID, CandidateID: command.Candidate.CandidateID, CandidateRevision: command.Candidate.CandidateRevision, CandidateHash: command.Candidate.CandidateHash,
			DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash, State: workspacemodel.WorkspaceActivationEligible,
		}
		return workspaceevents.DefinitionAdmissionIntentResult{
			Candidate: command.Candidate,
			AdmissionResult: (workspacemodel.DefinitionAdmissionResult{
				WorkspaceID: command.Candidate.WorkspaceID, CandidateID: command.Candidate.CandidateID,
				CandidateRevision: command.Candidate.CandidateRevision, CandidateHash: command.Candidate.CandidateHash,
				Disposition: workspacemodel.DefinitionAdmissionAdmittedAllOf, AdmissionCompilerVersion: command.Candidate.AdmissionCompilerVersion,
				CanonicalAdmittedInputHash: "test-admitted-all-of",
			}).WithHash(),
			CandidateDisposition: workspaceevents.DefinitionAdmissionCandidateCreated,
			Disposition:          workspaceevents.DefinitionAdmissionIntentEligible,
			StartupInput:         &startup,
			ActivationAdmission:  &activation,
		}, nil
	})

	if _, err := SubmitDefinitionAdmissionIntent(ctx, testHub.raw, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: candidate}); err != nil {
		t.Fatalf("SubmitDefinitionAdmissionIntent: %v", err)
	}
	if got := testHub.source(t, workspaceevents.TopicWorkspaceDefinitionAdmissionIntent, 0); got != "application/workorchd" {
		t.Fatalf("definition admission source = %q, want application/workorchd", got)
	}
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
	testHub.subscribe(workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandCreate, func(data any) (any, error) {
		command := data.(workspaceevents.CreateWorkspaceCommand)
		return workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: command.WorkspaceID}}, nil
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
		testHub.subscribe(workspaceevents.CommandDestination, topic, func(data any) (any, error) {
			switch command := data.(type) {
			case workspaceevents.CreateWorkspaceTemplateCommand:
				return workspaceevents.WorkspaceTemplateResult{Template: &workspaceevents.WorkspaceTemplate{ID: command.TemplateID}}, nil
			case workspaceevents.UpdateWorkspaceTemplateCommand:
				return workspaceevents.WorkspaceTemplateResult{Template: &workspaceevents.WorkspaceTemplate{ID: command.TemplateID}}, nil
			case workspaceevents.DeleteWorkspaceTemplateCommand:
				return workspaceevents.DeleteWorkspaceTemplateResult{TemplateID: command.TemplateID}, nil
			default:
				return nil, common.ErrInvalidArgument
			}
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

func TestDeleteWorkspaceViewSendsOnlyWorkspaceOwnerCommand(t *testing.T) {
	testHub := newWorkspaceCommandTestHub(t)
	workspaceID := common.WorkspaceID("ws-delete-command-payload")
	request := appmodelcore.DeleteWorkspaceCommand{RequestID: "delete-request-1", Purge: true, ExpectedRevision: 7}
	testHub.subscribe(workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandDelete, func(data any) (any, error) {
		cmd := data.(workspaceevents.DeleteWorkspaceCommand)
		return workspaceevents.DeleteWorkspaceResult{
			WorkspaceID:       cmd.WorkspaceID,
			DeletionID:        workspaceevents.StableWorkspaceDeletionID(cmd.WorkspaceID, cmd.RequestID),
			RequestID:         cmd.RequestID,
			Mode:              cmd.Mode(),
			Disposition:       workspaceevents.DeleteWorkspaceCompleted,
			DeletionRevision:  8,
			WorkspaceRevision: 9,
			Completed:         true,
		}, nil
	})
	if err := DeleteWorkspaceView(context.Background(), testHub.raw, DeleteWorkspaceViewRequest{WorkspaceID: string(workspaceID), Command: request}); err != nil {
		t.Fatalf("DeleteWorkspaceView: %v", err)
	}
	cmd, ok := testHub.command(t, workspaceevents.TopicWorkspaceCommandDelete, 0).(workspaceevents.DeleteWorkspaceCommand)
	if !ok || cmd.WorkspaceID != workspaceID || cmd.RequestID != request.RequestID || !cmd.Purge || cmd.ExpectedRevision != 7 {
		t.Fatalf("command = %#v, want exact Workspace deletion intent", cmd)
	}
	if got := testHub.source(t, workspaceevents.TopicWorkspaceCommandDelete, 0); got != workspaceDeletionIngressSource {
		t.Fatalf("deletion command source = %q, want %q", got, workspaceDeletionIngressSource)
	}
	if len(testHub.captured) != 1 {
		t.Fatalf("captured topics = %#v, want only Workspace deletion command", testHub.captured)
	}
}

func TestDeleteWorkspaceViewDerivesStableRequestIdentity(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-delete-stable-request")
	var commands []workspaceevents.DeleteWorkspaceCommand
	testHub := newWorkspaceCommandTestHub(t)
	testHub.subscribe(workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandDelete, func(data any) (any, error) {
		cmd := data.(workspaceevents.DeleteWorkspaceCommand)
		commands = append(commands, cmd)
		return workspaceevents.DeleteWorkspaceResult{
			WorkspaceID:       cmd.WorkspaceID,
			DeletionID:        workspaceevents.StableWorkspaceDeletionID(cmd.WorkspaceID, cmd.RequestID),
			RequestID:         cmd.RequestID,
			Mode:              cmd.Mode(),
			Disposition:       workspaceevents.DeleteWorkspaceDuplicate,
			DeletionRevision:  3,
			WorkspaceRevision: 4,
			Completed:         true,
		}, nil
	})
	request := DeleteWorkspaceViewRequest{WorkspaceID: string(workspaceID), Command: appmodelcore.DeleteWorkspaceCommand{Purge: true}}
	if err := DeleteWorkspaceView(context.Background(), testHub.raw, request); err != nil {
		t.Fatal(err)
	}
	if err := DeleteWorkspaceView(context.Background(), testHub.raw, request); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 || commands[0].RequestID == "" || commands[0].RequestID != commands[1].RequestID {
		t.Fatalf("commands = %#v, want stable non-empty request identity", commands)
	}
}
