package workspacecore

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestResolveDirectoryLayoutNormalizesExplicitPathsAndRejectsEscape(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	relative, err := ResolveDirectoryLayout("memory", "", "workspace-a", " ./projects/../project-a ")
	if err != nil {
		t.Fatal(err)
	}
	if relative.DeliverableDirectory != "project-a" || relative.InternalDirectory != filepath.Join(root, ".workorch-data", "internal", "workspaces", "workspace-a") {
		t.Fatalf("relative layout=%+v", relative)
	}

	absoluteInput := filepath.Join(root, "projects", "..", "project-b")
	absolute, err := ResolveDirectoryLayout("memory", "", "workspace-b", absoluteInput)
	if err != nil {
		t.Fatal(err)
	}
	if absolute.DeliverableDirectory != filepath.Clean(absoluteInput) {
		t.Fatalf("absolute deliverable=%q want=%q", absolute.DeliverableDirectory, filepath.Clean(absoluteInput))
	}

	for _, input := range []string{"../outside", filepath.Join("..", "..", "outside")} {
		if _, err := ResolveDirectoryLayout("memory", "", "workspace-c", input); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("escape input %q err=%v, want invalid argument", input, err)
		}
	}
}

func TestResolveDirectoryLayoutDefaultsAreWorkspaceIsolated(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	first, err := ResolveDirectoryLayout("memory", "", "workspace-a", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveDirectoryLayout("memory", "", "workspace-b", "")
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Join(root, ".workorch-workspaces")
	if first.DeliverableDirectory != filepath.Join(wantRoot, "workspace-a") || second.DeliverableDirectory != filepath.Join(wantRoot, "workspace-b") || first.DeliverableDirectory == second.DeliverableDirectory || first.InternalDirectory == second.InternalDirectory {
		t.Fatalf("isolated layouts first=%+v second=%+v", first, second)
	}
}

func TestResolveDirectoryLayoutRejectsUnsafeWorkspaceSegments(t *testing.T) {
	for _, workspaceID := range []string{"", " ", ".", "..", "../escape", "nested/workspace", `nested\workspace`, "/absolute"} {
		if _, err := ResolveDirectoryLayout("memory", "", workspaceID, ""); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("workspace ID %q err=%v, want invalid argument", workspaceID, err)
		}
	}
}

func TestResolveDirectoryLayoutCleansFileBackendDataRoot(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	layout, err := ResolveDirectoryLayout("file", filepath.Join("state", "..", "data"), "workspace-a", "")
	if err != nil {
		t.Fatal(err)
	}
	if !layout.Managed || layout.DeliverableRoot != ".workorch-workspaces" || layout.DeliverableDirectory != filepath.Join(".workorch-workspaces", "workspace-a") || layout.InternalDirectory != filepath.Join("data", "internal", "workspaces", "workspace-a") {
		t.Fatalf("file layout=%+v", layout)
	}
}
