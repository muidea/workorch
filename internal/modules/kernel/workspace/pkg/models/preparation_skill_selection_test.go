package workspace

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func preparationSelectionFixture(t *testing.T) (PreparationSkillSelectionDefinition, WorkspacePreparationSkillRegistryEvidence, WorkspacePreparationSoulEvidence, WorkspacePreparationSkillHubEffects) {
	t.Helper()
	r, s := preparationRegistryEvidenceFixture(), preparationSoulEvidenceFixture()
	s.Facts.Souls[0].AllowedSkillRefs = nil
	s.Facts.Souls[0].ContentHash = soulmodel.StableSoulContentHash(s.Facts.Souls[0])
	s.Facts.ReobservedSouls = soulmodel.CloneSouls(s.Facts.Souls)
	s.ContentHash = s.Hash()
	binding := preparationRecordFixture().Attempts[0].Binding
	d := PreparationSkillSelectionDefinition{WorkspaceID: "ws", StartupInputHash: binding.StartupInputHash, DefinitionRevision: binding.DefinitionRevision, SnapshotHash: "snapshot", GoalText: "write go", DoDItems: []string{"verify go"}}
	d.GoalTextHash, d.DoDItemSetHash = StableDefinitionTextHash(d.GoalText), StableDefinitionStringSetHash(d.DoDItems)
	now := time.Now().UTC()
	e := WorkspacePreparationSkillHubEffects{Version: PreparationSkillHubEffectsVersion, Ref: PreparationSkillHubRefPrefix + strings.Repeat("b", 32), WorkspaceID: "ws", RequestID: "original", PayloadHash: "payload", ExpectedWorkspaceRevision: 1, Binding: binding, ProjectDirectory: "/selection/project", ReservedAt: now, CompletedAt: now}
	raw := []byte("---\r\nname: go\r\nversion: installed-v1\r\ntriggers: [go]\r\nrequired_tools: [shell]\r\n---\r\noriginal 指令\x00\r\n")
	for i := range 2 {
		root, scope, dir := "/selection/global", "global", ""
		item := skillhubpreparation.StatusItem{SkillID: "same", Agent: "codex", TargetPath: root + "/same", Status: "modified", Version: "repo-other", LocalVersion: "local-other"}
		if i == 1 {
			root, scope, dir = e.ProjectDirectory+"/.agents/skills", "project", e.ProjectDirectory
			item = skillhubpreparation.StatusItem{SkillID: "same", LocalPath: root + "/same/SKILL.md", Status: "conflict"}
		}
		status := skillhubpreparation.Status{Scope: scope, ProjectPath: dir, Items: []skillhubpreparation.StatusItem{item}}
		if i == 0 {
			status.Items = append(status.Items, skillhubpreparation.StatusItem{SkillID: "other-agent", Agent: "claude"})
		}
		stdout, _ := json.Marshal(status)
		f := skillHubStatusFactsFixture()
		f.Scope, f.Directory, f.StartedAt, f.CompletedAt = map[bool]string{true: "global", false: "workspace"}[i == 0], dir, now, now
		f.RawOutput, f.TotalOutputBytes, f.RawSHA256 = stdout, uint64(len(stdout)), fileobservation.Digest(stdout)
		file := skillhubpreparation.Instruction{SkillID: "same", Path: root + "/same/SKILL.md", Raw: bytes.Clone(raw), RawSHA256: fileobservation.Digest(raw), ReadBytes: uint64(len(raw)), ObservedSize: int64(len(raw)), Complete: true}
		installed := skillhubpreparation.Installed{Version: skillhubpreparation.InstalledVersion, Root: root, First: []skillhubpreparation.Instruction{file}, Second: []skillhubpreparation.Instruction{file}, StartedAt: now, CompletedAt: now}
		f.Installed = &installed
		if i == 0 {
			e.Global = &f
		} else {
			e.Project = &f
		}
	}
	e.ContentHash = e.Hash()
	if e.Validate() != nil || s.Validate() != nil || r.Validate() != nil {
		t.Fatal("invalid selection fixture")
	}
	return d, r, s, e
}

func TestPreparationSkillSelectionCompilesExactSourcesWithoutReadiness(t *testing.T) {
	d, r, s, e := preparationSelectionFixture(t)
	value := CompilePreparationSkillSelection(d, r, &s, &e)
	if value.Summary() != "compiled_checks_pending" || len(value.Catalog) != 3 || len(value.Selected) != 3 || value.Selection.Policy != "goal_dod_auto_match" || value.Selected[0].Content.ID != "skillhub:workspace:ws:same" || value.Selected[0].ObservedStatus != "conflict" || value.Selected[1].ObservedStatus != "modified" {
		t.Fatalf("source filtering, score or status changed: %+v", value)
	}
	if value.Selected[0].Content.Instruction != string(e.Project.Installed.First[0].Raw) || value.Selected[1].Content.Version != "installed-v1" || value.Selected[0].Provenance.Source != workorchskillcontract.SourceSkillHub || value.Selected[2].Provenance.Source != workorchskillcontract.SourceWorkOrchRegistry {
		t.Fatal("selected original material/provenance lost")
	}
	clone := value.Clone()
	clone.Selected[0].Content.TriggerRules[0] = "changed"
	clone.Selection.Matches[0].Reasons[0] = "changed"
	clone.Catalog[0].TriggerRules[0] = "changed"
	clone.Definition.DoDItems[0] = "changed"
	if !reflect.DeepEqual(value, CompilePreparationSkillSelection(d, r, &s, &e)) {
		t.Fatal("clone aliases source")
	}
	for _, allowed := range [][]string{{"same"}, {"skillhub:global:same"}, {"missing"}} {
		ss := s.Clone()
		ss.Facts.Souls[0].AllowedSkillRefs = slices.Clone(allowed)
		ss.Facts.Souls[0].ContentHash = soulmodel.StableSoulContentHash(ss.Facts.Souls[0])
		ss.Facts.ReobservedSouls = soulmodel.CloneSouls(ss.Facts.Souls)
		ss.ContentHash = ss.Hash()
		v := CompilePreparationSkillSelection(d, r, &ss, &e)
		want := map[string]int{"same": 2, "skillhub:global:same": 1, "missing": 0}[allowed[0]]
		if v.FailureCode != "" || len(v.Selected) != want || len(v.Catalog) != 3 {
			t.Fatal("execution allowlist changed or catalog filtered before validation")
		}
	}
}

func TestPreparationSkillSelectionNeverTurnsMissingConflictOrStaleIntoEmptySuccess(t *testing.T) {
	d, r, s, e := preparationSelectionFixture(t)
	if CompilePreparationSkillSelection(d, r, nil, &e).Summary() != "sources_incomplete" || CompilePreparationSkillSelection(d, r, &s, nil).Summary() != "sources_incomplete" {
		t.Fatal("missing source credited as empty")
	}
	for name, mutate := range map[string]func(*PreparationSkillSelectionDefinition, *WorkspacePreparationSkillRegistryEvidence, *WorkspacePreparationSoulEvidence, *WorkspacePreparationSkillHubEffects){
		"goal": func(d *PreparationSkillSelectionDefinition, _ *WorkspacePreparationSkillRegistryEvidence, _ *WorkspacePreparationSoulEvidence, _ *WorkspacePreparationSkillHubEffects) {
			d.GoalText = "different"
		},
		"snapshot": func(_ *PreparationSkillSelectionDefinition, _ *WorkspacePreparationSkillRegistryEvidence, s *WorkspacePreparationSoulEvidence, _ *WorkspacePreparationSkillHubEffects) {
			s.Facts.SnapshotHash = "other"
			s.ContentHash = s.Hash()
		},
		"request": func(_ *PreparationSkillSelectionDefinition, _ *WorkspacePreparationSkillRegistryEvidence, s *WorkspacePreparationSoulEvidence, _ *WorkspacePreparationSkillHubEffects) {
			s.RequestID = "other"
			s.ContentHash = s.Hash()
		},
		"configuration": func(_ *PreparationSkillSelectionDefinition, _ *WorkspacePreparationSkillRegistryEvidence, s *WorkspacePreparationSoulEvidence, _ *WorkspacePreparationSkillHubEffects) {
			s.Facts.ConfigurationHash = "other"
			s.ContentHash = s.Hash()
		},
		"missing_second": func(_ *PreparationSkillSelectionDefinition, _ *WorkspacePreparationSkillRegistryEvidence, _ *WorkspacePreparationSoulEvidence, e *WorkspacePreparationSkillHubEffects) {
			e.Project.Installed.Second = nil
			e.ContentHash = e.Hash()
		},
		"duplicate_id": func(_ *PreparationSkillSelectionDefinition, r *WorkspacePreparationSkillRegistryEvidence, _ *WorkspacePreparationSoulEvidence, _ *WorkspacePreparationSkillHubEffects) {
			for _, c := range []*skillmodel.PreparationRegistryCatalog{&r.Facts.Catalog, &r.Facts.ReobservedCatalog} {
				c.Skills[0].ID = "skillhub:global:same"
				c.Skills[0].Provenance.ContentHash = workorchskillcontract.StableContentHash(c.Skills[0].Content())
				c.CatalogHash, _ = skillmodel.RegistryCatalogHash(c.Skills)
			}
			r.ContentHash = r.Hash()
		},
	} {
		t.Run(name, func(t *testing.T) {
			dd, rr, ss, ee := d, r.Clone(), s.Clone(), e.Clone()
			mutate(&dd, &rr, &ss, &ee)
			value := CompilePreparationSkillSelection(dd, rr, &ss, &ee)
			if value.FailureCode == "" || len(value.Selected) != 0 || len(value.Selection.Matches) != 0 {
				t.Fatal("bad source became empty success", value.FailureCode)
			}
		})
	}
}

func TestPreparationSkillSelectionCompleteEmptyCatalogStillRequiresEverySource(t *testing.T) {
	d, r, s, e := preparationSelectionFixture(t)
	for _, c := range []*skillmodel.PreparationRegistryCatalog{&r.Facts.Catalog, &r.Facts.ReobservedCatalog} {
		c.Skills = []*skillmodel.Skill{}
		c.CatalogHash, _ = skillmodel.RegistryCatalogHash(c.Skills)
	}
	r.ContentHash = r.Hash()
	for _, f := range []*PreparationSkillHubStatusFacts{e.Global, e.Project} {
		scope := "global"
		if f.Directory != "" {
			scope = "project"
		}
		f.RawOutput, _ = json.Marshal(skillhubpreparation.Status{Scope: scope, ProjectPath: f.Directory, Items: []skillhubpreparation.StatusItem{}})
		f.TotalOutputBytes, f.RawSHA256 = uint64(len(f.RawOutput)), fileobservation.Digest(f.RawOutput)
		f.Installed.First, f.Installed.Second = []skillhubpreparation.Instruction{}, []skillhubpreparation.Instruction{}
	}
	e.ContentHash = e.Hash()
	value := CompilePreparationSkillSelection(d, r, &s, &e)
	if r.Validate() != nil || e.Validate() != nil || value.Summary() != "compiled_checks_pending" || len(value.Catalog) != 0 || len(value.Selected) != 0 {
		t.Fatalf("complete empty catalog not preserved: %s", value.FailureCode)
	}
	// An absent read is different from a real, completely observed empty list.
	e.Project.Installed.Second = nil
	e.ContentHash = e.Hash()
	if CompilePreparationSkillSelection(d, r, &s, &e).Summary() != "sources_incomplete" {
		t.Fatal("missing empty observation became complete selection")
	}
}
