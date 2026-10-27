package workspace

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
)

func TestPreparationSelectedSkillStatusPreservesRawEnumsAndVersionConflicts(t *testing.T) {
	d, registry, soul, original := preparationSelectionFixture(t)
	for _, scenario := range []struct {
		name, status, version, want string
	}{
		{"synced", "synced", "installed-v1", "observed_freshness_pending"},
		{"modified", "modified", "installed-v1", "status_blocked"},
		{"conflict", "conflict", "installed-v1", "status_blocked"},
		{"missing", "missing", "installed-v1", "status_blocked"},
		{"unknown", "future_status", "installed-v1", "status_blocked"},
		{"case", "Synced", "installed-v1", "status_blocked"},
		{"whitespace", "synced ", "installed-v1", "status_blocked"},
		{"empty", "", "installed-v1", "status_blocked"},
		{"version", "synced", "other-local-v1", "version_conflict"},
		{"missing_version", "synced", "", "observed_freshness_pending"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			effects := original.Clone()
			for i, f := range []*PreparationSkillHubStatusFacts{effects.Global, effects.Project} {
				parsed, err := skillhubpreparation.ParseStatus(f.RawOutput, f.Directory, i == 0)
				if err != nil {
					t.Fatal(err)
				}
				parsed.Items[0].Status, parsed.Items[0].LocalVersion = scenario.status, scenario.version
				// Repository version deliberately differs. Never compare it as
				// if it described the actual independently read installation.
				parsed.Items[0].Version = "repository-other"
				f.RawOutput, _ = json.Marshal(parsed)
				f.TotalOutputBytes, f.RawSHA256 = uint64(len(f.RawOutput)), fileobservation.Digest(f.RawOutput)
			}
			effects.ContentHash = effects.Hash()
			selection := CompilePreparationSkillSelection(d, registry, &soul, &effects)
			result := CompilePreparationSkillStatus(selection, &effects)
			if selection.FailureCode != "" || result.State != scenario.want || len(result.Entries) != len(selection.Selected) {
				t.Fatal("status diagnosis drifted", selection.FailureCode, result)
			}
			for i, entry := range result.Entries {
				if entry.ObservedStatus != selection.Selected[i].ObservedStatus {
					t.Fatal("original status rewritten")
				}
			}
			copy := result.Clone()
			copy.Entries[0].Reason = "caller"
			if reflect.DeepEqual(copy, result) {
				t.Fatal("clone aliases")
			}
			missing := effects.Clone()
			missing.Project.Installed.Second = nil
			missing.ContentHash = missing.Hash()
			if CompilePreparationSkillStatus(selection, &missing).State != "sources_incomplete" || CompilePreparationSkillStatus(selection, nil).State != "sources_incomplete" {
				t.Fatal("missing source credited")
			}
		})
	}
}
