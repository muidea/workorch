package workspace

import (
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
	"reflect"
	"strings"
	"testing"
	"time"
)

func preparationRegistryEvidenceFixture() WorkspacePreparationSkillRegistryEvidence {
	at := time.Now().UTC()
	s := &skillmodel.Skill{ID: "skill", Name: "Original", Version: "v1", Instruction: "原始指令\n第二行", TriggerRules: []string{"go"}, Constraints: []string{}}
	s.Provenance = workorchskillcontract.Provenance{Source: workorchskillcontract.SourceWorkOrchRegistry, Scope: workorchskillcontract.ScopeGlobal, ContentHash: workorchskillcontract.StableContentHash(s.Content())}
	c := skillmodel.PreparationRegistryCatalog{Version: skillmodel.RegistryCatalogVersion, WorkspaceID: "ws", OwnerEpoch: "skill-preparation-epoch:" + strings.Repeat("A", 26), ObservedAt: at, Skills: []*skillmodel.Skill{s}}
	c.CatalogHash, _ = skillmodel.RegistryCatalogHash(c.Skills)
	f := PreparationSkillCatalogFacts{Version: "workorch-preparation-skill-registry-facts/v1", WorkspaceID: "ws", WorkspaceRevision: 2, SnapshotHash: "snapshot", ReobservedSnapshotHash: "snapshot", Catalog: c, ReobservedCatalog: c.Clone(), StartedAt: at, CompletedAt: at}
	e := WorkspacePreparationSkillRegistryEvidence{Version: PreparationSkillRegistryEvidenceVersion, Ref: PreparationSkillRegistryRefPrefix + strings.Repeat("a", 32), RequestID: "original", PayloadHash: "payload", Facts: f}
	e.ContentHash = e.Hash()
	return e
}
func TestPreparationSkillRegistryEvidenceRequiresCompleteValues(t *testing.T) {
	e := preparationRegistryEvidenceFixture()
	if e.Validate() != nil {
		t.Fatal("valid full facts rejected")
	}
	for _, mutate := range []func(*WorkspacePreparationSkillRegistryEvidence){
		func(e *WorkspacePreparationSkillRegistryEvidence) { e.Ref = e.ContentHash },
		func(e *WorkspacePreparationSkillRegistryEvidence) { e.Facts.ReobservedCatalog.Skills = nil },
		func(e *WorkspacePreparationSkillRegistryEvidence) {
			e.Facts.ReobservedCatalog.CatalogHash = e.ContentHash
		},
		func(e *WorkspacePreparationSkillRegistryEvidence) { e.Facts.ReobservedCatalog.Skills[0].Name = "other" },
		func(e *WorkspacePreparationSkillRegistryEvidence) {
			e.Facts.ReobservedCatalog.Skills[0].Constraints = nil
		},
		func(e *WorkspacePreparationSkillRegistryEvidence) {
			e.Facts.ReobservedCatalog.OwnerEpoch = "skill-preparation-epoch:" + strings.Repeat("B", 26)
		},
		func(e *WorkspacePreparationSkillRegistryEvidence) { e.Facts.Catalog.WorkspaceID = "other" },
		func(e *WorkspacePreparationSkillRegistryEvidence) { e.Facts.ReobservedSnapshotHash = "other" },
		func(e *WorkspacePreparationSkillRegistryEvidence) {
			e.Facts.Catalog.ObservedAt = e.Facts.StartedAt.Add(-time.Second)
		},
		func(e *WorkspacePreparationSkillRegistryEvidence) {
			e.Facts.ReobservedCatalog.ObservedAt = e.Facts.CompletedAt.Add(time.Second)
		},
	} {
		bad := e.Clone()
		mutate(&bad)
		bad.ContentHash = bad.Hash()
		if bad.Validate() == nil {
			t.Fatal("rehash/missing raw catalog bypassed validation")
		}
	}
	copy := e.Clone()
	if !reflect.DeepEqual(copy, e) {
		t.Fatal("clone changed raw empty-list shape")
	}
	copy.Facts.Catalog.Skills[0].TriggerRules[0] = "caller"
	copy.Facts.ReobservedCatalog.Skills[0].Instruction = "caller"
	if e.Validate() != nil {
		t.Fatal("clone aliases original")
	}
	// A genuinely empty complete directory is history, not missing observation.
	for _, c := range []*skillmodel.PreparationRegistryCatalog{&e.Facts.Catalog, &e.Facts.ReobservedCatalog} {
		c.Skills = []*skillmodel.Skill{}
		c.CatalogHash, _ = skillmodel.RegistryCatalogHash(c.Skills)
	}
	e.ContentHash = e.Hash()
	if e.Validate() != nil {
		t.Fatal("complete empty catalog rejected")
	}
}
