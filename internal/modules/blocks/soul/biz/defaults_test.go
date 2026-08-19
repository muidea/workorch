package soul

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestEnsureBuiltinSoulDefaultsSeedsMissingAndPreservesExisting(t *testing.T) {
	testCases := []struct {
		name string
		new  func(*testing.T) repository.SoulRepository
	}{
		{name: "memory", new: func(*testing.T) repository.SoulRepository { return memoryrepo.NewSoulRepository() }},
		{name: "file", new: func(t *testing.T) repository.SoulRepository {
			return filerepo.NewSoulRepository(filepath.Join(t.TempDir(), "souls"))
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			repo := testCase.new(t)
			defaults, err := workorchdefaults.BuiltinSoulDefaults()
			if err != nil || len(defaults) == 0 {
				t.Fatalf("load soul defaults len=%d err=%v", len(defaults), err)
			}

			existing := &soulmodel.Soul{
				ID:               common.SoulID(defaults[0].ID),
				Name:             "user-owned soul name",
				Description:      "preserve this customized built-in ID",
				BusinessDomains:  []string{"custom-domain"},
				SoulMD:           "# User customization",
				AllowedSkillRefs: []string{"custom-skill"},
			}
			if err := repo.Create(ctx, existing); err != nil {
				t.Fatalf("create customized soul: %v", err)
			}
			existing, err = repo.Get(ctx, existing.ID)
			if err != nil {
				t.Fatalf("read persisted customized soul: %v", err)
			}

			for attempt := 1; attempt <= 2; attempt++ {
				if err := ensureBuiltinSoulDefaults(ctx, NewService(repo)); err != nil {
					t.Fatalf("ensure attempt %d: %v", attempt, err)
				}
				got, err := repo.Get(ctx, existing.ID)
				if err != nil {
					t.Fatalf("get customized soul after attempt %d: %v", attempt, err)
				}
				if !reflect.DeepEqual(got, existing) {
					t.Fatalf("customized soul after attempt %d = %+v, want %+v", attempt, got, existing)
				}
			}
			items, err := repo.List(ctx)
			if err != nil || len(items) != len(defaults) {
				t.Fatalf("seeded souls len=%d want=%d err=%v", len(items), len(defaults), err)
			}
		})
	}
}

func TestEnsureSoulDefaultsRejectsInvalidSeedBeforePersistence(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewSoulRepository()
	err := ensureSoulDefaults(ctx, NewService(repo), []workorchmanagementcontract.Soul{{ID: "invalid-default", Name: "missing instructions"}})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ensure invalid defaults err = %v, want invalid argument", err)
	}
	items, listErr := repo.List(ctx)
	if listErr != nil || len(items) != 0 {
		t.Fatalf("invalid default persisted: items=%+v err=%v", items, listErr)
	}
}
