package biz

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestDefinitionAuthoringViewSelectsCompletedRequestAfterFailedAttempt(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "交付目标", TemplateID: "t"}
	template := workspace.WorkspaceTemplate{ID: "t"}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	var err error
	record, first, _, err := record.ReserveDerivation("attempt-1", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteDerivation(first.PermitID, nil, "provider_failed")
	if err != nil {
		t.Fatal(err)
	}
	record, second, _, err := record.ReserveDerivation("derive/"+record.InputHash, "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	acceptance := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"明确验收"}}
	record, err = record.CompleteDerivation(second.PermitID, &acceptance, "")
	if err != nil {
		t.Fatal(err)
	}
	query := workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: "ws", WorkspaceRevision: 2, Record: &record, CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: record.TemplateSnapshotHash}
	view, err := definitionAuthoringView(query)
	if err != nil || view.State != "derived" || view.DerivationPermitID != second.PermitID || view.Proposal == nil || len(view.Proposal.Items) != 1 {
		t.Fatalf("historical failure masked completed request: %+v %v", view, err)
	}
}

func TestDefinitionAuthoringViewKeepsFailureAndConfirmationVisible(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "交付目标", TemplateID: "t"}
	template := workspace.WorkspaceTemplate{ID: "t"}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	var err error
	record, failed, _, err := record.ReserveDerivation("attempt-1", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteDerivation(failed.PermitID, nil, "provider_failed")
	if err != nil {
		t.Fatal(err)
	}
	record, current, _, err := record.ReserveDerivation("derive/"+record.InputHash, "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	query := workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: input.WorkspaceID, WorkspaceRevision: 2, Record: &record, CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: record.TemplateSnapshotHash}
	view, err := definitionAuthoringView(query)
	if err != nil || view.State != "outcome_unresolved" || view.DerivationPermitID != current.PermitID {
		t.Fatalf("multiple attempts should select the current stable request: %+v %v", view, err)
	}
	// Even a pending historical attempt must not hide the confirmed admission
	// and exact review identity in the management projection.
	query.Confirmation = &workspace.DefinitionAuthoringConfirmation{InputHash: record.InputHash, DerivationPermitID: current.PermitID}
	if _, err := definitionAuthoringView(query); err == nil {
		t.Fatal("invalid confirmation was accepted by the projection")
	}
}

func TestSaveDefinitionAuthoringViewReplaysLostResponse(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	defer owner.Unsubscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery)
	defer owner.Unsubscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringSave)
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
	template := workspace.WorkspaceTemplate{ID: "t"}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	if err := owner.Subscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, func(_ magicevent.Event, result magicevent.Result) {
		copy := record.Clone()
		result.Set(workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: "ws", WorkspaceRevision: 3, Record: &copy, CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: copy.TemplateSnapshotHash}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err := owner.Subscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringSave, func(_ magicevent.Event, result magicevent.Result) {
		calls++
		result.Set(workspaceevents.SaveDefinitionAuthoringResult{Record: record}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	request := workorchmanagementcontract.DefinitionAuthoringSave{Input: workorchmanagementcontract.DefinitionAuthoringInput{GoalText: "目标"}, ExpectedWorkspaceRevision: 2, ExpectedInputHash: record.InputHash}
	view, err := b.SaveDefinitionAuthoringView(ctx, "ws", request)
	if err != nil || view.InputHash != record.InputHash || calls != 0 {
		t.Fatalf("lost response retry changed owner state: %+v %v calls=%d", view, err, calls)
	}
	request.Input.GoalText = "修改"
	if _, err := b.SaveDefinitionAuthoringView(ctx, "ws", request); err != common.ErrConflict || calls != 0 {
		t.Fatalf("stale edit was accepted: %v calls=%d", err, calls)
	}
}
