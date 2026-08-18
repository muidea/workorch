package workspace

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
)

func TestEnsureBuiltinWorkspaceTemplateDefaultsSeedsMissingAndPreservesExisting(t *testing.T) {
	testCases := []struct {
		name string
		new  func(*testing.T) repository.WorkspaceTemplateRepository
	}{
		{name: "memory", new: func(*testing.T) repository.WorkspaceTemplateRepository {
			return memoryrepo.NewWorkspaceTemplateRepository()
		}},
		{name: "file", new: func(t *testing.T) repository.WorkspaceTemplateRepository {
			return filerepo.NewWorkspaceTemplateRepository(filepath.Join(t.TempDir(), "workspace_templates.json"))
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			repo := testCase.new(t)
			defaults, err := workorchdefaults.BuiltinWorkspaceTemplateDefaults()
			if err != nil || len(defaults) == 0 {
				t.Fatalf("load workspace template defaults len=%d err=%v", len(defaults), err)
			}

			existing := &workspacemodel.WorkspaceTemplate{
				ID:            defaults[0].ID,
				Name:          "user-owned template name",
				Description:   "preserve this customized built-in ID",
				GoalText:      "custom goal",
				DoDItems:      []string{"custom acceptance"},
				DefaultSoulID: "custom-soul",
				SoulBindings:  map[string]string{"planning": "custom-planning-soul"},
			}
			if err := repo.Create(ctx, existing); err != nil {
				t.Fatalf("create customized template: %v", err)
			}
			existing, err = repo.Get(ctx, existing.ID)
			if err != nil {
				t.Fatalf("read persisted customized template: %v", err)
			}

			for attempt := 1; attempt <= 2; attempt++ {
				if err := ensureBuiltinWorkspaceTemplateDefaults(ctx, repo); err != nil {
					t.Fatalf("ensure attempt %d: %v", attempt, err)
				}
				got, err := repo.Get(ctx, existing.ID)
				if err != nil {
					t.Fatalf("get customized template after attempt %d: %v", attempt, err)
				}
				if !reflect.DeepEqual(got, existing) {
					t.Fatalf("customized template after attempt %d = %+v, want %+v", attempt, got, existing)
				}
			}
			items, err := repo.List(ctx)
			if err != nil || len(items) != len(defaults) {
				t.Fatalf("seeded templates len=%d want=%d err=%v", len(items), len(defaults), err)
			}
		})
	}
}
