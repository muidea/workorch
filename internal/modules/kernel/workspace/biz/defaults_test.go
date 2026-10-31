package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestBuiltinTemplatesAreCurrentWithoutPersistingAcrossRestart(t *testing.T) {
	for _, backend := range []string{"memory", "file"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "templates.json")
			var repo repository.WorkspaceTemplateRepository = memoryrepo.NewWorkspaceTemplateRepository()
			reopen := func() {
				if backend == "file" {
					repo = filerepo.NewWorkspaceTemplateRepository(path)
				}
			}
			reopen()
			for i := 0; i < 2; i++ {
				reopen()
				owner := &Biz{templates: repo}
				if err := owner.ensureCurrentBuiltinWorkspaceTemplateDefaults(ctx); err != nil {
					t.Fatal(err)
				}
				stored, err := repo.List(ctx)
				if err != nil || len(stored) != 0 {
					t.Fatalf("seeded durable templates: %d %v", len(stored), err)
				}
				if backend == "file" {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatalf("builtin created file: %v", err)
					}
				}
				current, err := owner.resolvedTemplate(ctx, "software-dev")
				if err != nil {
					t.Fatal(err)
				}
				raw, err := json.Marshal(current)
				if err != nil {
					t.Fatal(err)
				}
				var declaration map[string]any
				if err := json.Unmarshal(raw, &declaration); err != nil {
					t.Fatal(err)
				}
				profile := declaration["contextHints"].(map[string]any)["profileContracts"].(map[string]any)["software-dev"].(map[string]any)
				if profile["deliverableExpectations"].([]any)[0].(map[string]any)["requiredEvidencePurposeContractRef"] != "runtime/profile-deliverable-evidence-purpose" {
					t.Fatal("required profile purpose not current")
				}
				found := false
				for _, r := range profile["evidenceRequirements"].([]any) {
					p := r.(map[string]any)["implementationWorkPolicy"]
					if p != nil {
						found = true
						if p.(map[string]any)["inputRevisionProtocolRef"] != "validation/input-revision/v1" {
							t.Fatal("revision policy not current")
						}
					}
				}
				if !found {
					t.Fatal("missing work policy")
				}
			}
		})
	}
}

func TestBuiltinCatalogIgnoresPersistedOverridesAndPreservesFrozenSnapshot(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "templates.json")
	repo := filerepo.NewWorkspaceTemplateRepository(path)
	declarations, err := builtinWorkspaceTemplates()
	if err != nil {
		t.Fatal(err)
	}
	historical := declarations[0]
	historical.Description = "unknown customized old builtin"
	custom := &workspacemodel.WorkspaceTemplate{ID: "project-specific", Name: "custom", ParentTemplateIDs: []string{"software-dev"}}
	if err := repo.Create(ctx, historical); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, custom); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Keep exactly the old declaration as an existing Workspace's sealed input.
	frozen := historical.Clone()
	frozenBytes, _ := json.Marshal(frozen)
	frozenHash := workspacemodel.WorkspaceTemplateSnapshotHash(&frozen)
	for attempt := 0; attempt < 2; attempt++ {
		owner := &Biz{templates: filerepo.NewWorkspaceTemplateRepository(path)}
		if err := owner.ensureCurrentBuiltinWorkspaceTemplateDefaults(ctx); err != nil {
			t.Fatal(err)
		}
		current, err := owner.resolvedTemplate(ctx, historical.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Description == historical.Description || workspacemodel.WorkspaceTemplateSnapshotHash(current) == frozenHash {
			t.Fatal("persisted builtin overrode program declaration")
		}
		current.DoDItems[0] = "mutated detached query"
		inherited, err := owner.resolvedTemplate(ctx, custom.ID)
		if err != nil || len(inherited.EffectiveSources) != 2 || inherited.DoDItems[0] == current.DoDItems[0] {
			t.Fatal("custom inheritance did not consume detached current builtin", err)
		}
		ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandList, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.ListWorkspaceTemplatesCommand{})
		result, err := owner.executeWorkspaceTemplateCommand(ctx, ev)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, item := range result.Templates {
			if seen[item.ID] {
				t.Fatal("duplicate builtin")
			}
			seen[item.ID] = true
			if item.ID == "software-dev" && !item.Builtin {
				t.Fatal("missing builtin provenance")
			}
			if item.ID == custom.ID && item.Builtin {
				t.Fatal("custom marked builtin")
			}
		}
	}
	after, _ := os.ReadFile(path)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("query/startup modified durable catalog")
	}
	afterFrozen, _ := json.Marshal(frozen)
	if !reflect.DeepEqual(afterFrozen, frozenBytes) || workspacemodel.WorkspaceTemplateSnapshotHash(&frozen) != frozenHash {
		t.Fatal("existing sealed snapshot changed")
	}
}
