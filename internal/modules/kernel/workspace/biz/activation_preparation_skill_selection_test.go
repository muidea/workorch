package workspace

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	skillowner "github.com/muidea/workorch/internal/modules/blocks/skill/biz"
	soulowner "github.com/muidea/workorch/internal/modules/blocks/soul/biz"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func TestPreparationSkillSelectionRealOwnersExactSourcesAndNoQueryEffects(t *testing.T) {
	for _, scenario := range []string{"all_sources", "execution_allowlist", "missing_global", "bad_workspace_metadata", "seal_drift", "initial_stale", "no_loader"} {
		t.Run(scenario, func(t *testing.T) {
			owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
			defer owners.close()
			directory, globalRoot, bin := t.TempDir(), filepath.Join(t.TempDir(), "codex"), t.TempDir()
			t.Setenv("CODEX_SKILLS_DIR", globalRoot)
			t.Setenv("PATH", bin)
			raw := []byte("---\r\nname: exact installed skill\r\ntriggers: ['用户验收']\r\nrequired_tools: [shell]\r\nrequired_capability_refs: [cap-a]\r\nreference_refs: [references/a.md]\r\n---\r\n原始完整指令\r\n")
			for _, root := range []string{globalRoot, filepath.Join(directory, ".agents", "skills")} {
				if err := os.MkdirAll(filepath.Join(root, "common"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "common", "SKILL.md"), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			w := seedPreparationEligibleWithConfiguration(t, owners, "ws-skill-selection", directory, func(w *workspace.Workspace) {
				w.DefaultSoulID = "workspace-soul"
				w.SoulBindings = map[string]common.SoulID{"execution": "execution-soul"}
			})
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			soulRoot := t.TempDir()
			soulRepo := filerepository.NewSoulRepository(filepath.Join(soulRoot, "souls.json"))
			for _, id := range []common.SoulID{"workspace-soul", "execution-soul"} {
				allowed := []string{"not-the-execution-allowlist"}
				if id == "execution-soul" {
					allowed = []string{"common", "registry-skill"}
					if scenario == "execution_allowlist" {
						allowed = []string{"skillhub:global:common"}
					}
				}
				item := &soulmodel.Soul{ID: id, Revision: 2, Name: "owner soul", SoulMD: "Original soul instruction\n", SourceProvenance: soulmodel.SoulSourceUserManaged, AllowedSkillRefs: allowed}
				item.ContentHash = soulmodel.StableSoulContentHash(item)
				if err := soulRepo.Create(context.Background(), item); err != nil {
					t.Fatal(err)
				}
			}
			souls, err := soulowner.New(owners.hub, nil, soulowner.Dependencies{Souls: soulRepo})
			if err != nil {
				t.Fatal(err)
			}
			if err := souls.Start(); err != nil {
				t.Fatal(err)
			}
			defer souls.Teardown(context.Background())
			skillPath := filepath.Join(t.TempDir(), "skills.json")
			skillRepo := filerepository.NewSkillRepository(skillPath)
			item := preparationSkillFixture()
			item.TriggerRules = []string{"用户验收"}
			item.Provenance.ContentHash = workorchskillcontract.StableContentHash(item.Content())
			if err := skillRepo.Create(context.Background(), item); err != nil {
				t.Fatal(err)
			}
			skills, err := skillowner.New(owners.hub, nil, skillowner.Dependencies{Skills: skillRepo})
			if err != nil {
				t.Fatal(err)
			}
			if err := skills.Start(); err != nil {
				t.Fatal(err)
			}
			defer skills.Teardown(context.Background())
			script := fmt.Sprintf(`#!/bin/sh
printf X >> "$0.started"
if [ "$#" = 3 ] && [ "$3" = --global ]; then
printf '{"scope":"global","skill_count":1,"items":[{"skill_id":"common","agent":"codex","status":"synced","target_path":"%s/common","actual_hash":"opaque-directory-hash"}]}'
else
printf '{"scope":"project","project_path":"%%s","skill_count":1,"items":[{"skill_id":"common","status":"modified","local_path":"%%s/.agents/skills/common/SKILL.md"}]}' "$PWD" "$PWD"
fi
`, globalRoot)
			if err := os.WriteFile(filepath.Join(bin, "skill-hub"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			if scenario == "bad_workspace_metadata" {
				if err := os.WriteFile(filepath.Join(directory, ".agents", "skills", "common", "SKILL.md"), []byte("---\nrequired_tools: [shell]\nrequired_tools: []\n---\noriginal"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "missing_global" {
				t.Setenv("CODEX_SKILLS_DIR", filepath.Join(t.TempDir(), "absent"))
			}
			beforeSkills, _ := os.ReadFile(skillPath)
			beforeSouls := preparationSoulTree(t, soulRoot)
			app := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
			command := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision}
			ctx := configcore.ContextWithLoader(context.Background(), func() (workorchconfig.Config, error) { return workorchconfig.Config{}, nil })
			if scenario == "no_loader" {
				ctx = context.Background()
			}
			if scenario == "initial_stale" {
				command.ExpectedWorkspaceRevision++
			}
			if scenario == "seal_drift" {
				owners.workspace.workspaces = &preparationRevisionReadRepository{WorkspaceRepository: owners.workspace.workspaces, driftAt: 12}
			}
			if _, err := app.ReadStartupPreparationView(ctx, w.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(bin, "skill-hub.started")); !os.IsNotExist(err) {
				t.Fatal("persisted-state GET invoked status")
			}
			// Reset a test-only read counter so the drift is at the selection's
			// final seal read, after real source observations have completed.
			if observed, ok := owners.workspace.workspaces.(*preparationRevisionReadRepository); ok {
				observed.reads = 0
			}
			value, err := app.ObservePreparationSkillSelection(ctx, command)
			success := scenario == "all_sources" || scenario == "execution_allowlist"
			if (err == nil) != success || value.SealBindingCurrent != success {
				t.Fatalf("unexpected selection: %s %v", value.FailureCode, err)
			}
			if success {
				want := []string{"skillhub:workspace:ws-skill-selection:common", "registry-skill", "skillhub:global:common"}
				if scenario == "execution_allowlist" {
					want = []string{"skillhub:global:common"}
				}
				ids := []string{}
				for _, item := range value.SelectedSkills {
					ids = append(ids, item.ID)
					if item.Source == "skill-hub" && item.Instruction != string(raw) {
						t.Fatal("selected instruction changed")
					}
				}
				if !value.SelectionCompiled || !reflect.DeepEqual(ids, want) || value.ExecutionSoulID != "execution-soul" || value.GoalText != w.DefinitionAuthoringConfirmation.Goal.Text || !reflect.DeepEqual(value.DoDItems, w.DefinitionAuthoringConfirmation.DoD.Items) || value.Workspace.Skills[0].Status != "modified" || value.Global.Skills[0].ActualHash != "opaque-directory-hash" {
					t.Fatalf("lost exact source, selection order or original status: %+v", ids)
				}
				// A selected modified item remains a modified diagnostic fact,
				// not a resource PASS. Caller changes cannot mutate the catalog.
				value.SelectedSkills[0].TriggerRules[0] = "caller mutation"
				if value.Workspace.Skills[0].TriggerRules[0] != "用户验收" || value.Global.Skills[0].TriggerRules[0] != "用户验收" {
					t.Fatal("selected material aliases catalog")
				}
			} else if scenario == "seal_drift" {
				if !value.SelectionCompiled || len(value.SelectedSkills) != 3 || value.FailureCode != "seal_reobservation_failed" {
					t.Fatal("late seal failure lost already observed selection")
				}
			} else if value.SelectionCompiled {
				t.Fatal("partial source became compiled selection")
			}
			if scenario == "bad_workspace_metadata" && (value.Workspace.FailureCode != "invalid_metadata" || !value.Workspace.Observation.InstructionsReobserved || len(value.Global.Skills) != 1) {
				t.Fatal("metadata error lost complete raw or preceding sources")
			}
			wantCalls := 2
			if scenario == "initial_stale" || scenario == "no_loader" {
				wantCalls = 0
			}
			if scenario == "missing_global" {
				wantCalls = 1
			}
			marker, readErr := os.ReadFile(filepath.Join(bin, "skill-hub.started"))
			if len(marker) != wantCalls || wantCalls > 0 && readErr != nil {
				t.Fatalf("status retried or scope count wrong: %d wanted %d", len(marker), wantCalls)
			}
			if stored, err := owners.repositories.Workspaces.Get(context.Background(), w.ID); err != nil || !reflect.DeepEqual(stored, w) {
				t.Fatal("preparation changed Workspace")
			}
			afterSkills, _ := os.ReadFile(skillPath)
			if !bytes.Equal(beforeSkills, afterSkills) {
				t.Fatal("selection changed registry persistence")
			}
			if !reflect.DeepEqual(beforeSouls, preparationSoulTree(t, soulRoot)) {
				t.Fatal("selection changed Soul persistence")
			}
		})
	}
}
