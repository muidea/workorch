package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
)

func TestPreparationSkillHubApplicationBindsCurrentStatusAndInstalledInstructions(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, ".agents", "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	w := seedPreparationEligibleInDirectory(t, owners, "ws-skillhub-observation", directory)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	command := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision}
	bin := t.TempDir()
	globalRoot := filepath.Join(t.TempDir(), "codex")
	t.Setenv("CODEX_SKILLS_DIR", globalRoot)
	for _, root := range []string{globalRoot, filepath.Join(directory, ".agents", "skills")} {
		if err := os.MkdirAll(filepath.Join(root, "s"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "s", "SKILL.md"), []byte("原始指令\r\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	script := fmt.Sprintf(`#!/bin/sh
printf status-effect >> "$0.started"
if [ "$#" = 3 ] && [ "$3" = --global ]; then
printf '{"scope":"global","skill_count":1,"items":[{"skill_id":"s","agent":"codex","status":"synced","target_path":"%s/s"}]}'
else
printf '{"scope":"project","project_path":"%%s","skill_count":1,"items":[{"skill_id":"s","status":"modified","local_path":"%%s/.agents/skills/s/SKILL.md"}]}' "$PWD" "$PWD"
fi
`, globalRoot)
	if err := os.WriteFile(filepath.Join(bin, "skill-hub"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	b := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
	for i := 0; i < 2; i++ {
		if _, err := b.ReadStartupPreparationView(context.Background(), w.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(bin, "skill-hub.started")); !os.IsNotExist(err) {
		t.Fatal("persisted preparation query executed status")
	}
	for i := 0; i < 2; i++ {
		value, err := b.ObservePreparationSkillHub(context.Background(), command)
		if err != nil || !value.BindingCurrent || !value.Global.Inventory.InventoryComplete || !value.Workspace.Inventory.InventoryComplete || !value.Global.InstructionsReobserved || !value.Workspace.InstructionsReobserved || value.SnapshotHash == "" || value.Workspace.Inventory.Status.Scope != "project" || value.Workspace.Inventory.Status.Items[0].Status != "modified" {
			t.Fatalf("actual status contract or installed files lost: %v", err)
		}
	}
	if marker, err := os.ReadFile(filepath.Join(bin, "skill-hub.started")); err != nil || string(marker) != "status-effectstatus-effectstatus-effectstatus-effect" {
		t.Fatal("explicit observations did not execute exactly one status per scope")
	}
	stored, err := owners.repositories.Workspaces.Get(context.Background(), w.ID)
	if err != nil || !reflect.DeepEqual(stored, w) {
		t.Fatal("observation changed Workspace")
	}
	t.Setenv("PATH", t.TempDir())
	command.ExpectedWorkspaceRevision++
	value, err := b.ObservePreparationSkillHub(context.Background(), command)
	if err == nil || value.Global.Inventory.Version != "" {
		t.Fatal("stale initial seal started external observation")
	}
	command.ExpectedWorkspaceRevision--
	t.Setenv("PATH", bin)
	owners.workspace.workspaces = &preparationRevisionReadRepository{WorkspaceRepository: owners.workspace.workspaces, driftAt: 3}
	value, err = b.ObservePreparationSkillHub(context.Background(), command)
	if err == nil || value.BindingCurrent || !value.Global.InstructionsReobserved || !value.Workspace.InstructionsReobserved {
		t.Fatal("end seal drift lost facts or acquired credit")
	}
}

func TestPreparationSkillHubApplicationMissingCommandIsNotEmptyInventory(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-skillhub-unavailable")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	b := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
	value, err := b.ObservePreparationSkillHub(context.Background(), workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
	if err == nil || value.BindingCurrent || value.Global.Inventory.InventoryComplete || value.Global.Inventory.CommandStarted || value.Global.Inventory.FailureCode != "command_failed" || value.Workspace.Inventory.Version != "" {
		t.Fatal("missing command became empty inventory or retried")
	}
	stored, err := owners.repositories.Workspaces.Get(context.Background(), w.ID)
	if err != nil || !reflect.DeepEqual(stored, w) {
		t.Fatal("failed observation changed Workspace")
	}
}
