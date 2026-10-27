package workspace

import (
	"bytes"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func TestPreparationSelectedSkillReferencesReproduceFullOriginalSources(t *testing.T) {
	d, r, s, e := preparationSelectionFixture(t)
	for _, catalog := range []*skillmodel.PreparationRegistryCatalog{&r.Facts.Catalog, &r.Facts.ReobservedCatalog} {
		catalog.Skills[0].ReferenceRefs = []string{"references/registry"}
		catalog.Skills[0].Provenance.ContentHash = workorchskillcontract.StableContentHash(catalog.Skills[0].Content())
		catalog.CatalogHash, _ = skillmodel.RegistryCatalogHash(catalog.Skills)
	}
	r.ContentHash = r.Hash()
	for _, f := range []*PreparationSkillHubStatusFacts{e.Global, e.Project} {
		raw := []byte("---\ntriggers: [go]\nreference_refs: [references/a]\n---\noriginal")
		for _, files := range [][]skillhubpreparation.Instruction{f.Installed.First, f.Installed.Second} {
			files[0].Raw = bytes.Clone(raw)
			files[0].RawSHA256 = fileobservation.Digest(raw)
			files[0].ObservedSize = int64(len(raw))
			files[0].ReadBytes = uint64(len(raw))
		}
		plan, code := skillhubpreparation.ReferencePlan(f.Installed.Root, f.Installed.First)
		if code != "" {
			t.Fatal(code)
		}
		body := []byte("exact resource\x00\xff")
		file := skillhubpreparation.Instruction{SkillID: plan[0].SkillID, Path: plan[0].Path, Raw: body, RawSHA256: fileobservation.Digest(body), ReadBytes: uint64(len(body)), ObservedSize: int64(len(body)), Complete: true}
		ref := skillhubpreparation.Reference{Ref: plan[0].Ref, File: file}
		f.Installed.References = &skillhubpreparation.References{Version: skillhubpreparation.ReferencesVersion, StartedAt: f.Installed.StartedAt, CompletedAt: f.Installed.CompletedAt, First: []skillhubpreparation.Reference{ref}, Second: []skillhubpreparation.Reference{ref}}
	}
	e.ContentHash = e.Hash()
	selected := CompilePreparationSkillSelection(d, r, &s, &e)
	// Registry fixture declares a reference but has no filesystem provenance.
	refs := CompilePreparationSkillReferences(selected, &e)
	if selected.FailureCode != "" || refs.State != "source_unresolved" {
		t.Fatal("registry guessed a root", selected.FailureCode, refs.State)
	}
	var installedSelected []PreparationSelectedSkill
	for _, item := range selected.Selected {
		if item.Provenance.Source == workorchskillcontract.SourceSkillHub {
			installedSelected = append(installedSelected, item)
		}
	}
	selected.Selected = installedSelected
	refs = CompilePreparationSkillReferences(selected, &e)
	if refs.State != "observed_checks_pending" || len(refs.Entries) != 2 || refs.Entries[0].Path == refs.Entries[1].Path {
		t.Fatal("exact per-scope roots lost", refs)
	}
	clone := refs.Clone()
	clone.Entries[0].Path = "caller"
	if reflect.DeepEqual(clone, refs) {
		t.Fatal("clone aliases")
	}
	for _, scenario := range []string{"missing", "failed", "drift"} {
		bad := e.Clone()
		switch scenario {
		case "missing":
			bad.Project.Installed.References = nil
		case "failed":
			bad.Project.Installed.References.PreserveFailure("reference_unavailable", nil)
		case "drift":
			bad.Project.Installed.References.Second[0].File.Raw = []byte("drift")
		}
		bad.ContentHash = bad.Hash()
		if CompilePreparationSkillReferences(selected, &bad).State == "observed_checks_pending" {
			t.Fatal("missing or changed original accepted", scenario)
		}
	}
}
