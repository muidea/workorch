package workspace

import (
	"reflect"
	"strings"
	"testing"
	"time"

	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
)

func preparationSoulEvidenceFixture() WorkspacePreparationSoulEvidence {
	at := time.Now().UTC()
	s := &soulmodel.Soul{ID: "soul", Revision: 1, Name: "Original", SoulMD: "原文\r\n", SourceProvenance: soulmodel.SoulSourceUserManaged, AllowedSkillRefs: []string{"same"}, AllowedToolKinds: []string{}}
	s.ContentHash = soulmodel.StableSoulContentHash(s)
	plan, lookup, _ := PreparationSoulPlan("soul", nil, "")
	f := PreparationSoulFacts{Version: "workorch-preparation-souls/v1", WorkspaceID: "ws", WorkspaceRevision: 2, SnapshotHash: "snapshot", ReobservedSnapshotHash: "snapshot", ConfigurationHash: "config", WorkspaceDefaultSoulID: "soul", DefaultLookupSoulID: lookup, Selections: plan, Souls: []*soulmodel.Soul{s}, ReobservedSouls: []*soulmodel.Soul{soulmodel.CloneSoul(s)}, StartedAt: at, CompletedAt: at}
	e := WorkspacePreparationSoulEvidence{Version: PreparationSoulEvidenceVersion, Ref: PreparationSoulRefPrefix + strings.Repeat("a", 32), RequestID: "original", PayloadHash: "payload", Facts: f}
	e.ContentHash = e.Hash()
	return e
}

func TestPreparationSoulEvidenceRequiresCompleteValuesNotHashes(t *testing.T) {
	e := preparationSoulEvidenceFixture()
	if e.Validate() != nil {
		t.Fatal("valid complete observation rejected")
	}
	for _, mutate := range []func(*WorkspacePreparationSoulEvidence){
		func(e *WorkspacePreparationSoulEvidence) { e.Ref = e.ContentHash },
		func(e *WorkspacePreparationSoulEvidence) { e.Facts.ReobservedSouls = nil },
		func(e *WorkspacePreparationSoulEvidence) { e.Facts.ReobservedSouls[0].Revision++ },
		func(e *WorkspacePreparationSoulEvidence) {
			e.Facts.ReobservedSouls[0].AllowedSkillRefs = nil
			e.Facts.ReobservedSouls[0].AllowedMCPServerRefs = []string{"same"}
		},
		func(e *WorkspacePreparationSoulEvidence) { e.Facts.ReobservedSouls[0].AllowedToolKinds = nil },
		func(e *WorkspacePreparationSoulEvidence) { e.Facts.ReobservedSystemDefaultSoulID = "new" },
		func(e *WorkspacePreparationSoulEvidence) { e.Facts.ReobservedSnapshotHash = "new" },
		func(e *WorkspacePreparationSoulEvidence) { e.Facts.Selections[0].Source = "system_default" },
		func(e *WorkspacePreparationSoulEvidence) { e.Facts.Selections = e.Facts.Selections[:5] },
		func(e *WorkspacePreparationSoulEvidence) { e.Facts.Souls = append(e.Facts.Souls, e.Facts.Souls[0]) },
		func(e *WorkspacePreparationSoulEvidence) { e.Facts.CompletedAt = e.Facts.StartedAt.Add(-time.Second) },
	} {
		bad := e.Clone()
		mutate(&bad)
		bad.ContentHash = bad.Hash()
		if bad.Validate() == nil {
			t.Fatal("rehash/omission bypassed full-record check")
		}
	}
	clone := e.Clone()
	if !reflect.DeepEqual(clone, e) {
		t.Fatal("clone altered empty-list shape")
	}
	clone.Facts.Souls[0].AllowedSkillRefs[0] = "changed"
	clone.Facts.ReobservedSouls[0].SoulMD = "changed"
	clone.Facts.Selections[0].Source = "changed"
	if e.Validate() != nil {
		t.Fatal("clone aliases original facts")
	}
}
