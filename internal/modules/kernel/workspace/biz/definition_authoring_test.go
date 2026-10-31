package workspace

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	appmodelcore "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestAuthoringLedgerCASPublishesWorkspaceRefreshOnlyAfterWrite(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	const observerID = "/workspace/consumers/blocks/workspace/authoring-summary-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	notifications := make(chan workspaceevents.WorkspaceChangedData, 8)
	observer.Subscribe(workspaceevents.TopicWorkspaceStateChanged, func(event magicevent.Event, _ magicevent.Result) {
		if data, ok := event.Data().(workspaceevents.WorkspaceChangedData); ok {
			notifications <- data
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(workspaceevents.TopicWorkspaceStateChanged) })
	workspaces := memory.NewWorkspaceRepository()
	templates := memory.NewWorkspaceTemplateRepository()
	if err := templates.Create(ctx, &workspacemodel.WorkspaceTemplate{ID: "template"}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-authoring-summary", TemplateID: "template"}); err != nil {
		t.Fatal(err)
	}
	owner, err := New(hub, nil, workspaceOwnerTestDependencies(hub, nil, workspaces, workspaces, memory.NewGoalRepository(), memory.NewDoDRepository(), templates))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(context.Background()) })
	current, err := workspaces.Get(ctx, "ws-authoring-summary")
	if err != nil {
		t.Fatal(err)
	}
	input := workspacemodel.DefinitionAuthoringInput{Version: 1, WorkspaceID: current.ID, Revision: 1, GoalText: "goal", TemplateID: "template"}
	command := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save", Input: input, ExpectedWorkspaceRevision: current.Revision}
	_, err = owner.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: current.ID, Authoring: &command})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case notification := <-notifications:
		if notification.WorkspaceID != current.ID {
			t.Fatalf("refresh notification scope = %q", notification.WorkspaceID)
		}
	case <-time.After(time.Second):
		t.Fatal("successful authoring CAS did not publish the existing Workspace refresh notification")
	}
	// Exact save replay does not CAS, so it must not produce another refresh.
	if _, err := owner.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: current.ID, Authoring: &command}); err != nil {
		t.Fatal(err)
	}
	select {
	case notification := <-notifications:
		t.Fatalf("authoring replay emitted duplicate notification: %+v", notification)
	case <-time.After(20 * time.Millisecond):
	}
}

func recordDerivationForReadSummary(record workspacemodel.DefinitionAuthoringRecord) (workspacemodel.DefinitionDerivationRequest, error) {
	updated, _, _, err := record.ReserveDerivation("derive/"+record.InputHash, "provider", nil)
	if err != nil {
		return workspacemodel.DefinitionDerivationRequest{WireContractRef: workspacemodel.DefinitionAuthoringOwnerBound, WireContractHash: workspacemodel.AuthoringWireHash(workspacemodel.DefinitionAuthoringOwnerBound, "derive")}, err
	}
	return updated.DerivationRequests[0], nil
}

func TestAuthoringWorkspaceGetListAndPublicProjectionExposeOnlyCurrentSummary(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	workspaces := memory.NewWorkspaceRepository()
	templates := memory.NewWorkspaceTemplateRepository()
	if err := templates.Create(ctx, &workspacemodel.WorkspaceTemplate{ID: "summary-template"}); err != nil {
		t.Fatal(err)
	}
	input := workspacemodel.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-authoring-summary-read", Revision: 1, GoalText: "goal", TemplateID: "summary-template"}
	template := workspacemodel.WorkspaceTemplate{ID: input.TemplateID}
	record := workspacemodel.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspacemodel.WorkspaceTemplateSnapshotHash(&template)}
	derivation, err := recordDerivationForReadSummary(record)
	if err != nil {
		t.Fatal(err)
	}
	record.DerivationRequests = []workspacemodel.DefinitionDerivationRequest{derivation}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: input.WorkspaceID, TemplateID: input.TemplateID, DefinitionAuthoring: &record}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-no-authoring"}); err != nil {
		t.Fatal(err)
	}
	owner, err := New(hub, nil, workspaceOwnerTestDependencies(hub, nil, workspaces, workspaces, memory.NewGoalRepository(), memory.NewDoDRepository(), templates))
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(context.Background()) })
	getResult := hub.Send(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGet, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.GetWorkspaceCommand{WorkspaceID: input.WorkspaceID}))
	if getResult == nil {
		t.Fatal("GetWorkspace returned nil result")
	}
	value, resultErr := getResult.Get()
	if resultErr != nil {
		t.Fatalf("GetWorkspace result error: %v", resultErr)
	}
	workspaceResult, ok := value.(workspaceevents.WorkspaceResult)
	if !ok || workspaceResult.Workspace == nil || workspaceResult.Workspace.AuthoringSummary == nil || workspaceResult.Workspace.AuthoringSummary.State != "outcome_unresolved" || workspaceResult.Workspace.AuthoringSummary.InputRevision != 1 {
		t.Fatalf("GetWorkspace summary = %#v", value)
	}
	listResult := hub.Send(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandList, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.ListWorkspacesCommand{}))
	if listResult == nil {
		t.Fatal("ListWorkspaces returned nil result")
	}
	value, resultErr = listResult.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	listed, ok := value.(workspaceevents.WorkspaceListResult)
	if !ok || len(listed.Workspaces) != 2 {
		t.Fatalf("ListWorkspaces result = %#v", value)
	}
	for _, item := range listed.Workspaces {
		if item.ID == input.WorkspaceID {
			if item.AuthoringSummary == nil || item.AuthoringSummary.State != "outcome_unresolved" {
				t.Fatalf("list summary = %+v", item.AuthoringSummary)
			}
		} else if item.AuthoringSummary != nil {
			t.Fatalf("workspace without authoring exposed summary: %+v", item.AuthoringSummary)
		}
	}
	model := workspaceeventmapping.Workspace(workspaceResult.Workspace)
	if model == nil || model.DefinitionAuthoring != nil || model.DefinitionSpecificationLedger != nil || model.AuthoringSummary == nil {
		t.Fatal("read DTO mapping leaked or reconstructed owner ledger")
	}
	appModel := workspacecore.MapWorkspace(model)
	public := appmodelcore.ProjectWorkspace(appModel)
	wire, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	var projected map[string]any
	if err := json.Unmarshal(wire, &projected); err != nil {
		t.Fatal(err)
	}
	summary, ok := projected["authoringSummary"].(map[string]any)
	if !ok || summary["state"] != "outcome_unresolved" || summary["inputRevision"] != float64(1) {
		t.Fatalf("public summary JSON = %s", wire)
	}
	if _, present := projected["definitionAuthoring"]; present {
		t.Fatalf("public workspace leaked ledger: %s", wire)
	}
	// 公共投影重建不得创建 Workspace owner 创作状态。
	if reconstructed := workspacecore.WorkspaceModelFromView(appModel); reconstructed == nil || reconstructed.DefinitionAuthoring != nil || reconstructed.AuthoringSummary != nil {
		t.Fatal("public read observation was rebuilt as owner state")
	}
}

func TestWorkspaceDTOAuthoringSummaryReplacesStaleObservation(t *testing.T) {
	stale := &workspacemodel.AuthoringSummary{State: "failed", FailurePhase: "review", FailureCode: "stale", ReviewCount: 4, InputRevision: 99}
	withoutRecord := workspaceDTOWithAuthoringSummary(&workspacemodel.Workspace{ID: "ws-stale-summary", AuthoringSummary: stale})
	if withoutRecord == nil || withoutRecord.AuthoringSummary != nil {
		t.Fatalf("Workspace without owner authoring record retained stale observation: %+v", withoutRecord)
	}
}

func TestDefinitionAuthoringReducerSavesWithoutAdmission(t *testing.T) {
	w := &workspacemodel.Workspace{ID: "ws", Revision: 1, TemplateID: "t"}
	template := &workspacemodel.WorkspaceTemplate{ID: "t"}
	input := workspacemodel.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, TemplateID: "t", GoalText: "目标", AcceptanceRequirements: []string{"条件"}}
	cmd := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-test", Input: input, ExpectedWorkspaceRevision: 1}
	o := definitionSpecificationObservation{workspace: w, template: template}
	first, err := reduceDefinitionAuthoring(o, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if first.Next == nil || first.Next.DefinitionAuthoring == nil || w.DefinitionAuthoring != nil {
		t.Fatal("missing detached record")
	}
	if first.Next.StartupInput != nil || first.Next.ActivationAdmission != nil || first.Next.ActiveRunID != "" {
		t.Fatal("draft promoted authority")
	}
	first.Authoring.Record.Input.AcceptanceRequirements[0] = "外部改写"
	if first.Next.DefinitionAuthoring.Input.AcceptanceRequirements[0] != "条件" {
		t.Fatal("result aliases persisted record")
	}
	o.workspace = first.Next
	o.workspace.Revision++
	replay, err := reduceDefinitionAuthoring(o, cmd)
	if err != nil || replay.Next != nil {
		t.Fatalf("duplicate must not write: %v", err)
	}
	cmd.Input.GoalText = "新目标"
	if _, err := reduceDefinitionAuthoring(o, cmd); err == nil {
		t.Fatal("same revision changed input accepted")
	}
	cmd.Input.Revision++
	if _, err := reduceDefinitionAuthoring(o, cmd); err == nil {
		t.Fatal("stale CAS accepted")
	}
	cmd.ExpectedWorkspaceRevision = o.workspace.Revision
	cmd.ExpectedInputHash = o.workspace.DefinitionAuthoring.InputHash
	cmd.RequestID = "new-save"
	if _, err := reduceDefinitionAuthoring(o, cmd); err != nil {
		t.Fatal(err)
	}
	cmd.Input = input
	template.Description = "模板已变"
	if _, err := reduceDefinitionAuthoring(o, cmd); err == nil {
		t.Fatal("template drift silently replayed")
	}
}

func TestDefinitionAuthoringReducerRejectsExistingAuthority(t *testing.T) {
	cmd := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-test", Input: workspacemodel.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, TemplateID: "t", GoalText: "目标"}, ExpectedWorkspaceRevision: 1}
	for _, mutate := range []func(*workspacemodel.Workspace){
		func(w *workspacemodel.Workspace) { w.StartupInput = &workspacemodel.WorkspaceStartupInputContract{} },
		func(w *workspacemodel.Workspace) {
			w.ActivationAdmission = &workspacemodel.WorkspaceActivationAdmission{}
		},
		func(w *workspacemodel.Workspace) { w.ActiveRunID = "run" },
		func(w *workspacemodel.Workspace) {
			w.DefinitionSpecificationLedger = &workspacemodel.DefinitionSpecificationRequestLedger{}
		},
	} {
		w := &workspacemodel.Workspace{ID: "ws", Revision: 1, TemplateID: "t"}
		mutate(w)
		if _, err := reduceDefinitionAuthoring(definitionSpecificationObservation{workspace: w, template: &workspacemodel.WorkspaceTemplate{ID: "t"}}, cmd); err == nil {
			t.Fatal("existing authority accepted new creation draft")
		}
	}
}
