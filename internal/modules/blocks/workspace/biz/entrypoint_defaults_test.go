package workspace

import (
	"context"
	_ "embed"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// Frozen full content from Git, never reconstructed from current defaults.
//
//go:embed testdata/software-dev-template-93a5d468.json
var preEntrypointTemplateJSON []byte

//go:embed testdata/software-dev-template-52643362.json
var preInputRepairTemplateJSON []byte

func TestEntrypointTemplateUpgradeReopensReviewedPreviousDefault(t *testing.T) {
	assertReviewedTemplateUpgrade(t, preEntrypointTemplateJSON, "93a5d468")
}

func TestInputRepairTemplateUpgradeReopensReviewedPreviousDefault(t *testing.T) {
	assertReviewedTemplateUpgrade(t, preInputRepairTemplateJSON, "52643362")
}

func assertReviewedTemplateUpgrade(t *testing.T, previous []byte, revision string) {
	t.Helper()
	var raw workorchmanagementcontract.WorkspaceTemplate
	if err := json.Unmarshal(previous, &raw); err != nil {
		t.Fatal(err)
	}
	old, err := builtinWorkspaceTemplate(raw)
	if err != nil {
		t.Fatal(err)
	}
	hash := workspacemodel.StableWorkspaceTemplateContentHash(old)
	if workorchdefaults.KnownBuiltinContentRevision("workspace_template", old.ID, hash) != revision {
		t.Fatalf("previous template %s is not reviewed: %s", revision, hash)
	}
	for _, backend := range []string{"memory", "file"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "templates.json")
			var repo repository.WorkspaceTemplateRepository = memoryrepo.NewWorkspaceTemplateRepository()
			if backend == "file" {
				repo = filerepo.NewWorkspaceTemplateRepository(path)
			}
			if err := repo.Create(ctx, old); err != nil {
				t.Fatal(err)
			}
			var upgraded *workspacemodel.WorkspaceTemplate
			for attempt := 0; attempt < 2; attempt++ {
				if backend == "file" {
					repo = filerepo.NewWorkspaceTemplateRepository(path)
				}
				owner := &Biz{templates: repo}
				if err := owner.ensureCurrentBuiltinWorkspaceTemplateDefaults(ctx); err != nil {
					t.Fatal(err)
				}
				current, err := repo.Get(ctx, old.ID)
				if err != nil || workspacemodel.StableWorkspaceTemplateContentHash(current) == hash {
					t.Fatalf("entrypoint template not loaded: %+v %v", current, err)
				}
				if attempt == 0 {
					upgraded = current
				} else if !reflect.DeepEqual(upgraded, current) {
					t.Fatal("reopen rewrote an already upgraded template")
				}
			}
			custom, err := builtinWorkspaceTemplate(raw)
			if err != nil {
				t.Fatal(err)
			}
			custom.Description += " user customization"
			if err := repo.Update(ctx, custom); err != nil {
				t.Fatal(err)
			}
			before, err := repo.Get(ctx, old.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := (&Biz{templates: repo}).ensureCurrentBuiltinWorkspaceTemplateDefaults(ctx); err != nil {
				t.Fatal(err)
			}
			after, err := repo.Get(ctx, old.ID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("entrypoint upgrade overwrote a customized template")
			}
		})
	}
}
