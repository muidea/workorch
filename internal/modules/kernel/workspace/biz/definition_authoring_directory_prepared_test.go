package workspace

import (
	"context"
	"errors"
	"reflect"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestAuthoringDirectoryPreparedIsIdentityBoundIdempotentAndRestartable(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	template := &workspace.WorkspaceTemplate{ID: "step01-template", Name: "模板"}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, template); err != nil {
		t.Fatal(err)
	}
	id := workspace.DefinitionAuthoringWorkspaceID("step01-prepared")
	create := workspaceevents.CreateDefinitionAuthoringCommand{RequestID: "step01-prepared", WorkspaceID: id, Title: "草稿", DeliverableDirectory: root + "/project", InternalDirectory: root + "/internal", Input: workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: id, Revision: 1, TemplateID: template.ID, GoalText: "完整目标\n第二行", ResourceRefs: []string{"b", "a"}, AcceptanceRequirements: []string{"要求二", "要求一"}}}
	if _, err := owners.workspace.createDefinitionAuthoring(ctx, create); err != nil {
		t.Fatal(err)
	}
	query := workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id}
	initial, err := owners.workspace.queryDefinitionAuthoring(ctx, query)
	if err != nil || initial.Creation == nil || initial.Creation.DirectoryPrepared || initial.Creation.Replay == nil {
		t.Fatalf("initial: %+v %v", initial, err)
	}
	if initial.Creation.Replay.CreationInputHash() != create.CreationInputHash() {
		t.Fatal("owner guessed different replay")
	}
	command := workspaceevents.RecordAuthoringDirectoryPreparedCommand{Creation: create}
	bad := command
	bad.Creation.Title = "漂移"
	if _, err := owners.workspace.recordAuthoringDirectoryPrepared(ctx, bad); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("identity drift: %v", err)
	}
	repo := owners.workspace.workspaces
	cause := errors.New("injected pre-write CAS failure")
	owners.workspace.workspaces = &failPreparationDiagnosticCASRepository{WorkspaceRepository: repo, cause: cause}
	if _, err := owners.workspace.recordAuthoringDirectoryPrepared(ctx, command); !errors.Is(err, cause) {
		t.Fatalf("CAS: %v", err)
	}
	owners.workspace.workspaces = repo
	current, err := owners.workspace.queryDefinitionAuthoring(ctx, query)
	if err != nil || !reflect.DeepEqual(initial, current) {
		t.Fatalf("failed write fabricated success: %v", err)
	}
	receipt, err := owners.workspace.recordAuthoringDirectoryPrepared(ctx, command)
	if err != nil || receipt.ValidateFor(command) != nil {
		t.Fatalf("complete: %+v %v", receipt, err)
	}
	complete, err := owners.workspace.queryDefinitionAuthoring(ctx, query)
	if err != nil || !complete.Creation.DirectoryPrepared || complete.WorkspaceRevision != initial.WorkspaceRevision+1 || !reflect.DeepEqual(initial.Record, complete.Record) {
		t.Fatalf("completion changed input: %+v %v", complete, err)
	}
	if _, err := owners.workspace.recordAuthoringDirectoryPrepared(ctx, command); err != nil {
		t.Fatal(err)
	}
	duplicate, err := owners.workspace.queryDefinitionAuthoring(ctx, query)
	if err != nil || !reflect.DeepEqual(complete, duplicate) {
		t.Fatal("duplicate changed creation")
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	defer owners.close()
	restored, err := owners.workspace.queryDefinitionAuthoring(ctx, query)
	if err != nil || !reflect.DeepEqual(restored, complete) {
		t.Fatalf("owner restart lost creation: %v", err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || stored.ActiveRunID != "" || stored.StartupInput != nil || stored.ActivationAdmission != nil || stored.DefinitionAuthoringConfirmation != nil || len(stored.DefinitionAuthoring.DerivationRequests) != 0 {
		t.Fatalf("draft created execution or confirmation: %+v %v", stored, err)
	}
}
