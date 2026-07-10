package appservice

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
)

type fakeSkillHubCommandRunner struct {
	responses map[string][]byte
}

func (f fakeSkillHubCommandRunner) RunSkillHub(_ context.Context, dir string, args ...string) ([]byte, error) {
	key := skillHubCommandKey(dir, args...)
	if output, ok := f.responses[key]; ok {
		return output, nil
	}
	return nil, fmt.Errorf("unexpected skill-hub command %s", key)
}

func skillHubCommandKey(dir string, args ...string) string {
	return strings.TrimSpace(dir) + "|" + strings.Join(args, " ")
}

func withFakeSkillHubRunner(t *testing.T, runner SkillHubCommandRunner) {
	t.Helper()
	previous := skillHubRunner
	skillHubRunner = runner
	t.Cleanup(func() {
		skillHubRunner = previous
	})
}

func TestListSkillsWithBackendIncludesSkillHubGlobalSkills(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	skillDir := t.TempDir()
	writeSkillHubInstruction(t, skillDir, "Global Skill Instruction")
	withFakeSkillHubRunner(t, fakeSkillHubCommandRunner{
		responses: map[string][]byte{
			skillHubCommandKey("", "status", "--global", "--json", "--agent", "codex"): []byte(fmt.Sprintf(`{
				"scope":"global",
				"global_path":"/home/test/.skill-hub/global/skills",
				"items":[{
					"skill_id":"go-service-delivery",
					"agent":"codex",
					"status":"ok",
					"source_repository":"skills-repo",
					"version":"1.2.3",
					"source_hash":"sha256:source",
					"applied_hash":"sha256:applied",
					"actual_hash":"sha256:actual",
					"target_path":%q
				}]
			}`, skillDir)),
		},
	})

	if _, err := CreateSkillWithBackend(ctx, "file", dataRoot, CreateSkillCommand{
		ID:          "skill-repository",
		Name:        "Repository Skill",
		Version:     "0.1.0",
		Instruction: "Repository Skill Instruction",
	}); err != nil {
		t.Fatalf("CreateSkillWithBackend: %v", err)
	}

	items, err := ListSkillsWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("ListSkillsWithBackend: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2: %+v", len(items), items)
	}
	repositorySkill := findTestSkill(items, "skill-repository")
	if repositorySkill.ManagedBy != skillManagedByWorkorch || repositorySkill.Scope != skillScopeRepository {
		t.Fatalf("repository skill metadata = %+v", repositorySkill)
	}
	globalSkill := findTestSkill(items, "skillhub:global:go-service-delivery")
	if globalSkill.Source != skillSourceSkillHub ||
		globalSkill.Scope != skillScopeGlobal ||
		globalSkill.SourceSkillID != "go-service-delivery" ||
		globalSkill.Agent != "codex" ||
		globalSkill.ContentHash != "sha256:actual" ||
		!strings.Contains(globalSkill.Instruction, "Global Skill Instruction") {
		t.Fatalf("global skill metadata = %+v", globalSkill)
	}
}

func TestListSkillsWithBackendMapsSkillHubFrontMatter(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	skillDir := t.TempDir()
	writeSkillHubInstruction(t, skillDir, `---
name: Auto Delivery Skill
description: Select when Goal and DoD mention auto delivery
version: 9.9.9
triggerRules:
  - auto delivery
requiredMCPServerRefs: ["mcp-auto"]
requiredToolKinds: ["llm/chat"]
constraints:
  - preserve business boundary
---

Use this skill for auto delivery verification.
`)
	withFakeSkillHubRunner(t, fakeSkillHubCommandRunner{
		responses: map[string][]byte{
			skillHubCommandKey("", "status", "--global", "--json", "--agent", "codex"): []byte(fmt.Sprintf(`{
				"scope":"global",
				"items":[{
					"skill_id":"auto-delivery",
					"agent":"codex",
					"status":"ok",
					"version":"1.0.0",
					"target_path":%q
				}]
			}`, skillDir)),
		},
	})

	items, err := ListSkillsWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("ListSkillsWithBackend: %v", err)
	}
	item := findTestSkill(items, "skillhub:global:auto-delivery")
	if item.Name != "Auto Delivery Skill" ||
		item.Description != "Select when Goal and DoD mention auto delivery" ||
		item.Version != "1.0.0" ||
		!reflect.DeepEqual(item.TriggerRules, []string{"auto delivery"}) ||
		!reflect.DeepEqual(item.RequiredMCPServerRefs, []string{"mcp-auto"}) ||
		!reflect.DeepEqual(item.RequiredToolKinds, []string{"llm/chat"}) ||
		!reflect.DeepEqual(item.Constraints, []string{"preserve business boundary"}) {
		t.Fatalf("skill = %+v, want frontmatter metadata mapped", item)
	}
}

func TestListSkillsWithBackendIncludesWorkspaceSkillHubSkills(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	deliverableDirectory := t.TempDir()
	skillFile := filepath.Join(deliverableDirectory, ".agents", "skills", "workspace-delivery", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(skillFile, []byte("Workspace Skill Instruction"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	withFakeSkillHubRunner(t, fakeSkillHubCommandRunner{
		responses: map[string][]byte{
			skillHubCommandKey("", "status", "--global", "--json", "--agent", "codex"): []byte(`{"scope":"global","items":[]}`),
			skillHubCommandKey(deliverableDirectory, "status", "--json"): []byte(fmt.Sprintf(`{
				"project_path":%q,
				"items":[{
					"skill_id":"workspace-delivery",
					"status":"Synced",
					"source_repository":"skills-repo",
					"local_version":"2.0.0",
					"repo_version":"2.0.0",
					"local_path":%q,
					"repo_path":"/home/test/.skill-hub/repositories/skills-repo/skills/workspace-delivery/SKILL.md"
				}]
			}`, deliverableDirectory, skillFile)),
		},
	})

	workspace, err := CreateWorkspaceWithBackend(ctx, "file", dataRoot, "Workspace SkillHub", "workspace skillhub test", deliverableDirectory)
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	items, err := ListSkillsWithBackend(ctx, "file", dataRoot, SkillQuery{WorkspaceID: workspace.ID})
	if err != nil {
		t.Fatalf("ListSkillsWithBackend: %v", err)
	}

	wantID := "skillhub:workspace:" + workspace.ID + ":workspace-delivery"
	workspaceSkill := findTestSkill(items, wantID)
	if workspaceSkill.Scope != skillScopeWorkspace ||
		workspaceSkill.WorkspaceID != workspace.ID ||
		workspaceSkill.Version != "2.0.0" ||
		workspaceSkill.ProjectPath != deliverableDirectory ||
		!strings.Contains(workspaceSkill.Instruction, "Workspace Skill Instruction") {
		t.Fatalf("workspace skill metadata = %+v", workspaceSkill)
	}
}

func TestSelectedSkillsCanResolveSkillHubReferences(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	deliverableDirectory := t.TempDir()
	skillDir := filepath.Join(deliverableDirectory, ".agents", "skills", "runtime-workspace-skill")
	writeSkillHubInstruction(t, skillDir, "Workspace Runtime Skill")
	withFakeSkillHubRunner(t, fakeSkillHubCommandRunner{
		responses: map[string][]byte{
			skillHubCommandKey(deliverableDirectory, "status", "--json"): []byte(fmt.Sprintf(`{
				"project_path":%q,
				"items":[{
					"skill_id":"runtime-workspace-skill",
					"status":"Synced",
					"local_version":"3.0.0",
					"local_path":%q
				}]
			}`, deliverableDirectory, filepath.Join(skillDir, "SKILL.md"))),
		},
	})
	workspace, err := CreateWorkspaceWithBackend(ctx, "file", dataRoot, "Runtime Workspace", "runtime workspace skill", deliverableDirectory)
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	session, err := newRuntimeSessionWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("newRuntimeSessionWithBackend: %v", err)
	}
	defer func() {
		closeErr := session.Close(ctx)
		if closeErr != nil {
			t.Fatalf("session.Close: %v", closeErr)
		}
	}()

	ref := "skillhub:workspace:" + workspace.ID + ":runtime-workspace-skill"
	hub, err := session.RequiredCommandHub()
	if err != nil {
		t.Fatalf("session.RequiredCommandHub: %v", err)
	}
	items, err := activationports.SelectedSkills(ctx, hub, []string{ref}, workspace.ID, skillHubCatalogForEventHub(hub))
	if err != nil {
		t.Fatalf("SelectedSkills: %v", err)
	}
	if got := []string{items[0].ID, items[0].SourceSkillID, items[0].Scope}; !reflect.DeepEqual(got, []string{ref, "runtime-workspace-skill", skillScopeWorkspace}) {
		t.Fatalf("selected skill identifiers = %+v", got)
	}
}

func TestSkillHubCatalogRequiresEventHubForWorkspaceSkills(t *testing.T) {
	_, err := skillHubCatalogForEventHub(nil).ListWorkspaceSkills(context.Background(), "ws-missing-hub")
	if err == nil {
		t.Fatalf("ListWorkspaceSkills without hub succeeded")
	}
}

func writeSkillHubInstruction(t *testing.T, dir string, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func findTestSkill(items []Skill, id string) Skill {
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	return Skill{}
}
