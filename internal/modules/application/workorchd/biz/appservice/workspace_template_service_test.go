package appservice

import (
	"context"
	"os"
	"path/filepath"
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

func TestBuiltinWorkspaceTemplatesUseProgramContentWithoutCatalogWrites(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	for attempt := 0; attempt < 2; attempt++ {
		templates, err := ListWorkspaceTemplatesWithBackend(ctx, "file", root)
		if err != nil || len(templates) == 0 {
			t.Fatalf("templates: %d %v", len(templates), err)
		}
		for _, template := range templates {
			if !template.Builtin {
				t.Fatalf("missing read-only provenance for %s", template.ID)
			}
		}
		software, err := ReadWorkspaceTemplateWithBackend(ctx, "file", root, "software-dev")
		if err != nil || !software.Builtin {
			t.Fatalf("builtin read: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, "system", "workspace_templates.json")); !os.IsNotExist(err) {
			t.Fatalf("builtin catalog persisted: %v", err)
		}
	}
}
