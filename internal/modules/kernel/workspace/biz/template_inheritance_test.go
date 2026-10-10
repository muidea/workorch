package workspace

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestTemplateOwnerCommandsResolveAndFenceDescendants(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceTemplateRepository()
	owner := &Biz{templates: repo}
	command := func(topic string, data any) (workspaceCommandResponse, error) {
		return owner.executeWorkspaceTemplateCommand(ctx, magicevent.NewEventWithContext(topic, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, data))
	}
	create := workspaceevents.TopicWorkspaceTemplateCommandCreate
	if _, err := command(create, workspaceevents.CreateWorkspaceTemplateCommand{TemplateID: "parent", GoalText: "goal", DoDItems: []string{"required"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := command(create, workspaceevents.CreateWorkspaceTemplateCommand{TemplateID: "child", ParentTemplateIDs: []string{"parent"}, DoDItems: []string{"other"}}); err != nil {
		t.Fatal(err)
	}
	read, err := command(workspaceevents.TopicWorkspaceTemplateCommandGet, workspaceevents.GetWorkspaceTemplateCommand{TemplateID: "child"})
	if err != nil || read.Template.GoalText != "goal" || len(read.Template.DoDItems) != 2 || len(read.Template.EffectiveSources) != 2 {
		t.Fatalf("owner effective read: %+v %v", read, err)
	}
	if _, err := command(workspaceevents.TopicWorkspaceTemplateCommandUpdate, workspaceevents.UpdateWorkspaceTemplateCommand{TemplateID: "child", ParentTemplateIDs: []string{"parent"}, GoalText: "goal"}); err != nil {
		t.Fatal(err)
	}
	if _, err := command(workspaceevents.TopicWorkspaceTemplateCommandUpdate, workspaceevents.UpdateWorkspaceTemplateCommand{TemplateID: "parent", GoalText: "changed"}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changing inherited Goal without child agreement: %v", err)
	}
	if _, err := command(workspaceevents.TopicWorkspaceTemplateCommandDelete, workspaceevents.DeleteWorkspaceTemplateCommand{TemplateID: "parent"}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("deleting referenced parent: %v", err)
	}
	if _, err := command(workspaceevents.TopicWorkspaceTemplateCommandUpdate, workspaceevents.UpdateWorkspaceTemplateCommand{TemplateID: "parent", GoalText: "goal", Description: "updated"}); err != nil {
		t.Fatal(err)
	}
	newRead, err := command(workspaceevents.TopicWorkspaceTemplateCommandGet, workspaceevents.GetWorkspaceTemplateCommand{TemplateID: "child"})
	if err != nil || workspacemodel.WorkspaceTemplateSnapshotHash(templateFromEventForTest(newRead.Template)) == workspacemodel.WorkspaceTemplateSnapshotHash(templateFromEventForTest(read.Template)) {
		t.Fatalf("parent edit not reflected in child read: %v", err)
	}
}

func templateFromEventForTest(item *workspaceevents.WorkspaceTemplate) *workspacemodel.WorkspaceTemplate {
	return &workspacemodel.WorkspaceTemplate{ID: item.ID, ParentTemplateIDs: item.ParentTemplateIDs, EffectiveSources: item.EffectiveSources, Name: item.Name, Description: item.Description, GoalText: item.GoalText, DoDItems: item.DoDItems}
}
