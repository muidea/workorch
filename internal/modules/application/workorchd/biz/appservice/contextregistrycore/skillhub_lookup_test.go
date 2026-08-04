package contextregistrycore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type fakeSkillHubCatalog struct {
	global       []workorchmanagementcontract.Skill
	workspaces   map[string][]workorchmanagementcontract.Skill
	globalErr    error
	workspaceErr error
}

func (f fakeSkillHubCatalog) ListGlobalSkills(context.Context) ([]workorchmanagementcontract.Skill, error) {
	if f.globalErr != nil {
		return nil, f.globalErr
	}
	return append([]workorchmanagementcontract.Skill(nil), f.global...), nil
}

func (f fakeSkillHubCatalog) ListWorkspaceSkills(_ context.Context, workspaceID string) ([]workorchmanagementcontract.Skill, error) {
	if f.workspaceErr != nil {
		return nil, f.workspaceErr
	}
	return append([]workorchmanagementcontract.Skill(nil), f.workspaces[workspaceID]...), nil
}

func TestListSkillHubSkillsForQueryAddsWorkspaceSkillsWhenAvailable(t *testing.T) {
	ctx := context.Background()
	catalog := fakeSkillHubCatalog{
		global: []workorchmanagementcontract.Skill{{ID: "skillhub:global:global-skill"}},
		workspaces: map[string][]workorchmanagementcontract.Skill{
			"ws-001": {{ID: "skillhub:workspace:ws-001:workspace-skill"}},
		},
	}

	items := ListSkillHubSkillsForQuery(ctx, catalog, SkillQuery{WorkspaceID: " ws-001 "})
	got := skillIDs(items)
	want := []string{"skillhub:global:global-skill", "skillhub:workspace:ws-001:workspace-skill"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %+v, want %+v", got, want)
	}
}

func TestListSkillHubSkillsForQueryKeepsGlobalWhenWorkspaceFails(t *testing.T) {
	ctx := context.Background()
	catalog := fakeSkillHubCatalog{
		global:       []workorchmanagementcontract.Skill{{ID: "skillhub:global:global-skill"}},
		workspaceErr: errors.New("workspace unavailable"),
	}

	items := ListSkillHubSkillsForQuery(ctx, catalog, SkillQuery{WorkspaceID: "ws-001"})
	got := skillIDs(items)
	want := []string{"skillhub:global:global-skill"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %+v, want %+v", got, want)
	}
}

func TestReadSkillHubSkillResolvesWorkspaceRefUsingEmbeddedWorkspace(t *testing.T) {
	ctx := context.Background()
	ref := "skillhub:workspace:ws-ref:workspace-skill"
	catalog := fakeSkillHubCatalog{
		workspaces: map[string][]workorchmanagementcontract.Skill{
			"ws-ref": {{ID: ref, SourceSkillID: "workspace-skill"}},
		},
	}

	item, err := ReadSkillHubSkill(ctx, catalog, ref, SkillQuery{WorkspaceID: "ws-query"})
	if err != nil {
		t.Fatalf("ReadSkillHubSkill: %v", err)
	}
	if item.ID != ref {
		t.Fatalf("item.ID = %q, want %q", item.ID, ref)
	}
}

func TestReadSkillHubSkillRequiresWorkspaceForUnscopedWorkspaceRef(t *testing.T) {
	ctx := context.Background()

	_, err := ReadSkillHubSkill(ctx, fakeSkillHubCatalog{}, "skillhub:workspace:workspace-skill", SkillQuery{})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestSkillHubInstructionPathAcceptsFileOrDirectory(t *testing.T) {
	dir := filepath.Join("tmp", "skill")
	file := filepath.Join(dir, "SKILL.md")
	tests := []struct {
		name      string
		candidate string
		want      string
	}{
		{name: "empty", candidate: " ", want: ""},
		{name: "directory", candidate: dir, want: file},
		{name: "file", candidate: file, want: file},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SkillHubInstructionPath(tt.candidate); got != tt.want {
				t.Fatalf("path = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadSkillHubInstructionAndMetadataReadsFirstAvailableSkillFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	skillDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: File Skill
triggerRules:
  - file trigger
---

Use file skill.
`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	instruction, metadata := ReadSkillHubInstructionAndMetadata(SkillHubStatusItem{
		TargetPath: missing,
		LocalPath:  skillDir,
	})
	if instruction == "" {
		t.Fatalf("instruction is empty")
	}
	if metadata["name"] != "File Skill" || metadata["triggerRules"] != "file trigger" {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func skillIDs(items []workorchmanagementcontract.Skill) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return ret
}
