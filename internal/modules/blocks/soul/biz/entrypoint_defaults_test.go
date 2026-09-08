package soul

import (
	"context"
	_ "embed"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// Full Git-frozen metadata and bodies; later guidance cannot move this baseline.
//
//go:embed testdata/software-dev-souls-93a5d468.json
var preEntrypointSoulsJSON []byte

func TestEntrypointSoulsUpgradeReopensReviewedPreviousDefaults(t *testing.T) {
	var previous []workorchmanagementcontract.Soul
	if err := json.Unmarshal(preEntrypointSoulsJSON, &previous); err != nil || len(previous) != 3 {
		t.Fatalf("previous software-dev Souls: %d %v", len(previous), err)
	}
	for _, raw := range previous {
		t.Run(raw.ID, func(t *testing.T) {
			cmd := soulDefaultCreateCommand(raw)
			value, err := normalizedSoulFromCommand(cmd)
			if err != nil {
				t.Fatal(err)
			}
			hash := soulmodel.StableSoulContentHash(value)
			if workorchdefaults.KnownBuiltinContentRevision("soul", raw.ID, hash) != "93a5d468" {
				t.Fatalf("pre-entrypoint Soul is not reviewed: %s %s", raw.ID, hash)
			}
			for _, backend := range []string{"memory", "file"} {
				t.Run(backend, func(t *testing.T) {
					ctx := context.Background()
					path := filepath.Join(t.TempDir(), "souls")
					var repo repository.SoulRepository = memoryrepo.NewSoulRepository()
					if backend == "file" {
						repo = filerepo.NewSoulRepository(path)
					}
					old, err := newService(repo).createSoul(ctx, cmd, soulmodel.SoulSourceBuiltinSeed)
					if err != nil {
						t.Fatal(err)
					}
					var upgraded *soulmodel.Soul
					for attempt := 0; attempt < 2; attempt++ {
						if backend == "file" {
							repo = filerepo.NewSoulRepository(path)
						}
						s := newService(repo)
						if err := s.ensureCurrentBuiltinSoulDefaults(ctx); err != nil {
							t.Fatal(err)
						}
						current, err := s.getSoul(ctx, old.ID)
						if err != nil || current.Revision != old.Revision+1 || current.ContentHash == old.ContentHash || current.SourceProvenance != soulmodel.SoulSourceBuiltinSeed {
							t.Fatalf("entrypoint guidance not loaded: %+v %v", current, err)
						}
						if attempt == 0 {
							upgraded = current
						} else if !reflect.DeepEqual(upgraded, current) {
							t.Fatal("reopen rewrote an already upgraded Soul")
						}
						if old.ContentHash != hash || old.SoulMD != value.SoulMD {
							t.Fatal("upgrade mutated prior Soul snapshot")
						}
					}
					custom := cmd
					custom.SoulMD += "\nUser customization."
					s := newService(repo)
					before, err := s.updateSoul(ctx, custom)
					if err != nil {
						t.Fatal(err)
					}
					if err := s.ensureCurrentBuiltinSoulDefaults(ctx); err != nil {
						t.Fatal(err)
					}
					after, err := s.getSoul(ctx, old.ID)
					if err != nil || !reflect.DeepEqual(before, after) {
						t.Fatal("entrypoint upgrade overwrote customized guidance")
					}
				})
			}
		})
	}
}
