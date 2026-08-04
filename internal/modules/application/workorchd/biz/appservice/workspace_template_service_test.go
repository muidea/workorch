package appservice

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestReadWorkspaceTemplateWithBackendUsesFocusedWorkspacePort(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	created, err := CreateWorkspaceTemplateWithBackend(ctx, "file", dataRoot, workorchmanagementcontract.WorkspaceTemplate{
		ID:       " template-read ",
		Name:     " Template Read ",
		GoalText: "ship it",
		DoDItems: []string{"done"},
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}

	read, err := ReadWorkspaceTemplateWithBackend(ctx, "file", dataRoot, " template-read ")
	if err != nil {
		t.Fatalf("ReadWorkspaceTemplateWithBackend: %v", err)
	}
	if read.ID != created.ID || read.Name != created.Name || read.GoalText != "ship it" {
		t.Fatalf("read = %+v, want created template %+v", read, created)
	}
}
