package workspace

import (
	"reflect"
	"strings"
	"testing"
	"time"

	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func ownerRevalidationFixture(t *testing.T) (*Workspace, PreparationOwnerRevalidation) {
	t.Helper()
	w := memorySelectionEvaluationFixture("list_only", 1)
	a := &w.StartupPreparation.Attempts[0]
	a.OwnerRevalidationVersion = PreparationOwnerRevalidationVersion
	start := time.Now().UTC().Add(-time.Second)
	a.StartedAt, a.CompletedAt = start, start.Add(800*time.Millisecond)
	s, k := preparationSoulEvidenceFixture(), preparationRegistryEvidenceFixture()
	_, m, _ := preparationMCPSelectionFixture(t)
	s.RequestID, s.PayloadHash = a.RequestID, a.PayloadHash
	k.RequestID, k.PayloadHash = a.RequestID, a.PayloadHash
	m.RequestID, m.PayloadHash = a.RequestID, a.PayloadHash
	s.Facts.StartedAt, s.Facts.CompletedAt = start, start
	k.Facts.StartedAt, k.Facts.CompletedAt = start, start
	k.Facts.Catalog.ObservedAt, k.Facts.ReobservedCatalog.ObservedAt = start, start
	m.Facts.StartedAt, m.Facts.CompletedAt = start, start
	e := &w.PreparationMemoryInventoryEvidence[0]
	e.Facts.SelectionSource.StartedAt, e.Facts.SelectionSource.CompletedAt = start, start
	e.Facts.Observation.ObservedAt = start
	e.ContentHash = e.Hash()
	s.ContentHash, k.ContentHash, m.ContentHash = s.Hash(), k.Hash(), m.Hash()
	w.PreparationSoulEvidence, w.PreparationSkillRegistryEvidence, w.PreparationMCPRegistryEvidence = []WorkspacePreparationSoulEvidence{s}, []WorkspacePreparationSkillRegistryEvidence{k}, []WorkspacePreparationMCPRegistryEvidence{m}
	a.SoulEvidenceRef, a.SkillRegistryEvidenceRef, a.MCPRegistryEvidenceRef = s.Ref, k.Ref, m.Ref
	r := PreparationOwnerRevalidation{Version: PreparationOwnerRevalidationVersion, WorkspaceID: w.ID, RequestID: a.RequestID, PayloadHash: a.PayloadHash, WorkspaceRevision: a.ExpectedWorkspaceRevision + 1, StartedAt: start.Add(100 * time.Millisecond), CompletedAt: start.Add(700 * time.Millisecond)}
	sf, kf, mf, rf := s.Facts.Clone(), k.Facts.Clone(), e.Facts.Clone(), m.Facts.Clone()
	sf.StartedAt, sf.CompletedAt = r.StartedAt, r.CompletedAt
	kf.StartedAt, kf.CompletedAt = r.StartedAt, r.CompletedAt
	kf.Catalog.ObservedAt, kf.ReobservedCatalog.ObservedAt = r.StartedAt, r.CompletedAt
	mf.Observation.ObservedAt = r.StartedAt
	mf.SelectionSource.StartedAt, mf.SelectionSource.CompletedAt = r.StartedAt, r.CompletedAt
	rf.StartedAt, rf.CompletedAt = r.StartedAt, r.CompletedAt
	r.Souls, r.Skills, r.Memory, r.MCP = &sf, &kf, &mf, &rf
	a.SealEvidenceRef = PreparationSealRefPrefix + strings.Repeat("e", 32)
	w.PreparationSealEvidence = []WorkspacePreparationSealEvidence{{Ref: a.SealEvidenceRef, FinalOwners: &r}}
	if r.Validate() != nil || !r.MatchesOriginal(w, *a) {
		t.Fatal("invalid complete final observation fixture")
	}
	return w, r
}

func TestPreparationOwnerRevalidationRequiresFullOriginalsAndExactFinalSources(t *testing.T) {
	for name, mutate := range map[string]func(*PreparationOwnerRevalidation){
		"missing_soul":   func(r *PreparationOwnerRevalidation) { r.Souls = nil },
		"missing_skill":  func(r *PreparationOwnerRevalidation) { r.Skills = nil },
		"missing_memory": func(r *PreparationOwnerRevalidation) { r.Memory = nil },
		"missing_mcp":    func(r *PreparationOwnerRevalidation) { r.MCP = nil },
		"skill_restart": func(r *PreparationOwnerRevalidation) {
			r.Skills.Catalog.OwnerEpoch = "skill-preparation-epoch:" + strings.Repeat("B", 26)
			r.Skills.ReobservedCatalog.OwnerEpoch = r.Skills.Catalog.OwnerEpoch
		},
		"memory_restart": func(r *PreparationOwnerRevalidation) {
			r.Memory.Observation.OwnerEpoch = "memory-preparation-epoch:" + strings.Repeat("B", 26)
		},
		"soul_revision": func(r *PreparationOwnerRevalidation) {
			r.Souls.Souls[0].Revision++
			r.Souls.ReobservedSouls = soulmodel.CloneSouls(r.Souls.Souls)
		},
		"soul_allowlist": func(r *PreparationOwnerRevalidation) {
			r.Souls.Souls[0].AllowedToolKinds = nil
			r.Souls.ReobservedSouls = soulmodel.CloneSouls(r.Souls.Souls)
		},
		"skill_raw": func(r *PreparationOwnerRevalidation) {
			r.Skills.Catalog.Skills[0].Instruction += "changed"
			s := r.Skills.Catalog.Skills[0]
			s.Provenance.ContentHash = workorchskillcontract.StableContentHash(s.Content())
			r.Skills.Catalog.CatalogHash, _ = skillmodel.RegistryCatalogHash(r.Skills.Catalog.Skills)
			r.Skills.ReobservedCatalog = r.Skills.Catalog.Clone()
		},
		"memory_mode": func(r *PreparationOwnerRevalidation) {
			r.Memory.SelectionSource.Before.ConfigurationRevision = "new"
			r.Memory.SelectionSource.After.ConfigurationRevision = "new"
		},
		"memory_full_raw": func(r *PreparationOwnerRevalidation) { r.Memory.Observation.Items[0].Content += "changed" },
		"mcp_raw": func(r *PreparationOwnerRevalidation) {
			r.MCP.Before.Servers[0].Name += "changed"
			sealPreparationMCPTestSnapshot(t, &r.MCP.Before)
			r.MCP.After = r.MCP.Before.Clone()
		},
	} {
		t.Run(name, func(t *testing.T) {
			w, original := ownerRevalidationFixture(t)
			r := original.Clone()
			mutate(&r)
			if r.Validate() != nil {
				t.Fatal("negative must remain a valid raw final observation")
			}
			if r.MatchesOriginal(w, w.StartupPreparation.Attempts[0]) {
				t.Fatal("missing/drifted owner material accepted")
			}
			w.PreparationSealEvidence[0].FinalOwners = &r
			if stale, err := PreparationOwnerRevalidationStale(w, r.CompletedAt); err != nil || !stale {
				t.Fatal("known drift/absence not projected", err)
			}
			if !original.MatchesOriginal(w, w.StartupPreparation.Attempts[0]) {
				t.Fatal("clone altered original owner material")
			}
		})
	}
}

func TestPreparationOwnerRevalidationMissingFinalAndAmbiguousOriginals(t *testing.T) {
	w, r := ownerRevalidationFixture(t)
	w.PreparationSealEvidence[0].FinalOwners = nil
	if stale, err := PreparationOwnerRevalidationStale(w, r.CompletedAt); err != nil || !stale {
		t.Fatal("omitted final observation bypassed new operation contract", err)
	}
	w.StartupPreparation.Attempts[0].OwnerRevalidationVersion = ""
	if stale, err := PreparationOwnerRevalidationStale(w, r.CompletedAt); err != nil || stale {
		t.Fatal("query upgraded legacy operation", err)
	}
	w, r = ownerRevalidationFixture(t)
	w.PreparationSkillRegistryEvidence = append(w.PreparationSkillRegistryEvidence, w.PreparationSkillRegistryEvidence[0].Clone())
	if r.MatchesOriginal(w, w.StartupPreparation.Attempts[0]) {
		t.Fatal("ambiguous original accepted")
	}
	w, r = ownerRevalidationFixture(t)
	w.PreparationSoulEvidence[0].RequestID = "other-operation"
	w.PreparationSoulEvidence[0].ContentHash = w.PreparationSoulEvidence[0].Hash()
	if r.MatchesOriginal(w, w.StartupPreparation.Attempts[0]) {
		t.Fatal("other operation original accepted after rehash")
	}
}

func TestPreparationOwnerRevalidationScopeWindowAndNoRenewal(t *testing.T) {
	w, r := ownerRevalidationFixture(t)
	for _, at := range []time.Time{r.CompletedAt, r.StartedAt.Add(PreparationOwnerRevalidationMaxAge - time.Nanosecond)} {
		if stale, err := PreparationOwnerRevalidationStale(w, at); err != nil || stale {
			t.Fatal("exact historical sources rejected", err)
		}
	}
	for _, at := range []time.Time{r.CompletedAt.Add(-time.Nanosecond), r.StartedAt.Add(PreparationOwnerRevalidationMaxAge)} {
		if stale, err := PreparationOwnerRevalidationStale(w, at); err != nil || !stale {
			t.Fatal("window extended", err)
		}
	}
	for _, mutate := range []func(*PreparationOwnerRevalidation){
		func(r *PreparationOwnerRevalidation) { r.WorkspaceID = "other" },
		func(r *PreparationOwnerRevalidation) { r.RequestID = "other" },
		func(r *PreparationOwnerRevalidation) { r.PayloadHash = "other" },
		func(r *PreparationOwnerRevalidation) { r.WorkspaceRevision++ },
		func(r *PreparationOwnerRevalidation) { r.StartedAt = r.CompletedAt.Add(time.Second) },
		func(r *PreparationOwnerRevalidation) {
			r.CompletedAt = r.StartedAt.Add(PreparationOwnerRevalidationMaxAge)
		},
	} {
		bad := r.Clone()
		mutate(&bad)
		if bad.MatchesOriginal(w, w.StartupPreparation.Attempts[0]) {
			t.Fatal("wrong scope/time accepted")
		}
	}
	if !reflect.DeepEqual(r, *w.PreparationSealEvidence[0].FinalOwners) {
		t.Fatal("projection changed history")
	}
}
