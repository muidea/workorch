package file

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/preparationcompiler"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// This complete synthetic original owner image was sealed with source count 4.
// It has a reviewed all-of definition and a blocked preparation archive; it
// contains no live data and grants no preparation readiness or execution credit.
func TestWorkspaceSourceMetadataSurvivesStrictFileReadAndReopen(t *testing.T) {
	original, err := os.ReadFile("testdata/workspace-source-metadata.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "workspace.json")
	if err = os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		repo := NewWorkspaceRepository(path)
		w, err := repo.Get(context.Background(), "ws-source-metadata")
		if err != nil {
			t.Fatal("original owner read rejected", err)
		}
		list, err := repo.List(context.Background())
		if err != nil || len(list) != 1 || list[0].ID != w.ID {
			t.Fatal("workspace list", err)
		}
		if _, present, err := w.ValidateActivationAdmissionPersistence(); err != nil || !present {
			t.Fatal("original definition lost", err)
		}
		if err = preparationcompiler.ValidatePreparedPersistence(w); err != nil {
			t.Fatal("original preparation rejected", err)
		}
		policy, err := preparationcompiler.CompileOriginalPreparedExecutionPolicy(w, w.StartupPreparation.Attempts[0])
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, p := range policy.ExecutionContract.ProfileContracts {
			for _, r := range p.EvidenceRequirements {
				if work := r.ImplementationWorkPolicy; work != nil {
					count++
					if work.MaxInputRevisionsPerCWI != 4 || work.ExecutionMethod().MaxInputRevisionsPerCWI != 0 {
						t.Fatal("source metadata rewritten or made executable")
					}
				}
			}
		}
		if count != 1 || rc.StableRuntimeProfileContractsHash(policy.ExecutionContract.ProfileContracts) != w.StartupPreparation.Attempts[0].DefinitionCompilation.ProfileContractsHash {
			t.Fatal("original profile hash changed")
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(original, after) {
			t.Fatal("read rewrote original owner bytes", err)
		}
	}
}

func TestWorkspaceSourceMetadataDoesNotRelaxCorruptionRejection(t *testing.T) {
	original, err := os.ReadFile("testdata/workspace-source-metadata.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"unknown", "metadata", "process", "goal"} {
		t.Run(field, func(t *testing.T) {
			raw := append([]byte(nil), original...)
			switch field {
			case "unknown":
				var rows map[string]map[string]json.RawMessage
				if err := json.Unmarshal(raw, &rows); err != nil {
					t.Fatal(err)
				}
				rows["ws-source-metadata"]["UndeclaredField"] = json.RawMessage(`true`)
				raw, err = json.Marshal(rows)
				if err != nil {
					t.Fatal(err)
				}
			case "metadata":
				raw = bytes.Replace(raw, []byte(`"maxInputRevisionsPerCWI":4`), []byte(`"maxInputRevisionsPerCWI":5`), 1)
			case "process":
				raw = bytes.Replace(raw, []byte(`"MaxRuntimeRepairAttempts":3`), []byte(`"MaxRuntimeRepairAttempts":2`), 1)
			case "goal":
				var rows map[string]map[string]json.RawMessage
				if err := json.Unmarshal(raw, &rows); err != nil {
					t.Fatal(err)
				}
				var record map[string]json.RawMessage
				if err := json.Unmarshal(rows["ws-source-metadata"]["DefinitionAuthoring"], &record); err != nil {
					t.Fatal(err)
				}
				var input map[string]json.RawMessage
				if err := json.Unmarshal(record["Input"], &input); err != nil {
					t.Fatal(err)
				}
				input["GoalText"] = json.RawMessage(`"tampered"`)
				record["Input"], _ = json.Marshal(input)
				rows["ws-source-metadata"]["DefinitionAuthoring"], _ = json.Marshal(record)
				raw, _ = json.Marshal(rows)
			}
			if bytes.Equal(raw, original) {
				t.Fatal("mutation did not change fixture")
			}
			path := filepath.Join(t.TempDir(), "workspace.json")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewWorkspaceRepository(path).Get(context.Background(), "ws-source-metadata"); err == nil {
				t.Fatal("corrupt original source accepted")
			}
		})
	}
}
