package workspaceports

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceTemplateViewUsesOwnerPorts(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	hub := runtime.Exports().Infrastructure.EventHub

	created, err := CreateWorkspaceTemplate(ctx, hub, workspacecore.WorkspaceTemplate{
		ID:       " template-port ",
		Name:     " Template Port ",
		GoalText: "ship it",
		DoDItems: []string{" done ", ""},
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceTemplate: %v", err)
	}
	if created.ID != "template-port" || created.Name != "Template Port" {
		t.Fatalf("created = %+v, want normalized id/name", created)
	}
	if len(created.DoDItems) != 1 || created.DoDItems[0] != "done" {
		t.Fatalf("created DoDItems = %#v, want normalized done", created.DoDItems)
	}

	items, err := ListWorkspaceTemplates(ctx, hub)
	if err != nil {
		t.Fatalf("ListWorkspaceTemplates: %v", err)
	}
	if !workspaceTemplateListContains(items, "template-port") {
		t.Fatalf("templates = %#v, want template-port", items)
	}

	read, err := ReadWorkspaceTemplate(ctx, hub, " template-port ")
	if err != nil {
		t.Fatalf("ReadWorkspaceTemplate: %v", err)
	}
	if read.ID != "template-port" || read.Name != "Template Port" {
		t.Fatalf("read = %+v, want template-port", read)
	}

	updated, err := UpdateWorkspaceTemplate(ctx, hub, " template-port ", workspacecore.WorkspaceTemplate{
		Name:        "Updated Template",
		Description: "updated",
	})
	if err != nil {
		t.Fatalf("UpdateWorkspaceTemplate: %v", err)
	}
	if updated.ID != "template-port" || updated.Name != "Updated Template" {
		t.Fatalf("updated = %+v, want updated template-port", updated)
	}

	if err := DeleteWorkspaceTemplate(ctx, hub, " template-port "); err != nil {
		t.Fatalf("DeleteWorkspaceTemplate: %v", err)
	}
	items, err = ListWorkspaceTemplates(ctx, hub)
	if err != nil {
		t.Fatalf("ListWorkspaceTemplates after delete: %v", err)
	}
	if workspaceTemplateListContains(items, "template-port") {
		t.Fatalf("templates after delete = %#v, want template-port removed", items)
	}
}

func workspaceTemplateListContains(items []workspacecore.WorkspaceTemplate, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func TestDeleteWorkspaceTemplateRejectsBlankID(t *testing.T) {
	if err := DeleteWorkspaceTemplate(context.Background(), nil, " "); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("DeleteWorkspaceTemplate err = %v, want ErrInvalidArgument", err)
	}
}

func TestReadWorkspaceTemplateRejectsBlankID(t *testing.T) {
	if _, err := ReadWorkspaceTemplate(context.Background(), nil, " "); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ReadWorkspaceTemplate err = %v, want ErrInvalidArgument", err)
	}
}
