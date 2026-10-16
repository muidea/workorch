package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func compareRuntimeStateConsumerSession(t testing.TB, r RuntimeStateRecord) {
	t.Helper()
	before, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		got, want := r.Validate(), referenceRuntimeStateConsumerSession(r)
		if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
			t.Fatalf("full record first error differs: %v != %v", got, want)
		}
	}
	after, err := json.Marshal(r)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("record validation changed raw input")
	}
	var loaded RuntimeStateRecord
	if err := json.Unmarshal(before, &loaded); err != nil {
		t.Fatal(err)
	}
	if cwiFocusBindingsError(loaded.Validate()) != cwiFocusBindingsError(referenceRuntimeStateConsumerSession(loaded)) {
		t.Fatal("full reloaded record first error differs")
	}
}

func mutateRuntimeStateConsumerSession(r *RuntimeStateRecord, arm uint8, raw string) {
	switch arm % 18 {
	case 0:
		r.Definition.StartupInputHash = raw
	case 1:
		r.Facts.RunID = ""
	case 2:
		r.ResultFeedbackVersion = 0
	case 3:
		r.ImplementationWork.PurposeChoices = append(r.ImplementationWork.PurposeChoices, RuntimeImplementationWorkPurposeChoice{})
	case 4:
		if len(r.ImplementationWork.Rounds) > 0 {
			r.ImplementationWork.Rounds[0].Hash = raw
		}
	case 5:
		if len(r.SemanticPlanning.History) > 0 && r.SemanticPlanning.History[0].Snapshot.Manifest != nil {
			m := r.SemanticPlanning.History[0].Snapshot.Manifest
			if len(m.OwnerCatalogs) > 0 {
				m.OwnerCatalogs[0].SnapshotID = raw
			}
		}
	case 6:
		if len(r.OperationReceipts) > 0 {
			r.OperationReceipts[0].Fence++
		}
	case 7:
		r.RepairSurfaceConfirmations = nil
	case 8:
		r.Facts.EvidenceRevision = InitialRuntimeEvidenceRevision
	case 9:
		r.SemanticPlanning.DefinitionSeed.PolicyHash = raw
	case 10:
		if r.SemanticPlanning.Manifest != nil && len(r.SemanticPlanning.Manifest.OwnerCatalogs) > 0 {
			r.SemanticPlanning.Manifest.OwnerCatalogs[0].SnapshotID = raw
		}
	case 11:
		r.SemanticPlanning.CompletedBatchIDs = append(r.SemanticPlanning.CompletedBatchIDs, "", "")
	case 12:
		if len(r.CompletedDispatches) > 0 {
			r.CompletedDispatches[0].DispatchAuthorityHash = raw
		}
	case 13:
		if len(r.Facts.Assessments) > 0 {
			r.Facts.Assessments[0].Revision = InitialRuntimeEvidenceRevision
		}
	case 14:
		if len(r.Facts.Coverage) > 0 {
			r.Facts.Coverage[0].HistoricalAcceptanceProofs = nil
		}
	case 15:
		r.Facts.DefinitionSeed.Hash = raw
	case 16:
		if len(r.SemanticPlanning.History) > 0 {
			r.SemanticPlanning.History[0].Snapshot.DefinitionSeed.Hash = raw
		}
	case 17:
		if len(r.CandidateAttempts) > 0 {
			r.CandidateAttempts[0].ManifestHash = raw
		}
	}
}

func TestRuntimeStateConsumerSessionFixtureOracle(t *testing.T) {
	for arm := uint8(0); arm < 18; arm++ {
		r := terminalHistoricalAcceptanceFixture(t)
		mutateRuntimeStateConsumerSession(&r, arm, "raw\x00变化")
		compareRuntimeStateConsumerSession(t, r)
	}
	compareRuntimeStateConsumerSession(t, RuntimeStateRecord{})
}

func FuzzRuntimeStateConsumerSessionFullOracle(f *testing.F) {
	for arm := uint8(0); arm < 18; arm++ {
		f.Add(arm, "原文\x00")
	}
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		r := terminalHistoricalAcceptanceFixture(t)
		mutateRuntimeStateConsumerSession(&r, arm, raw)
		compareRuntimeStateConsumerSession(t, r)
	})
}

// Explicit owner-managed archive, read-only: no repository, drive or provider.
func runtimeStateConsumerArchive(t testing.TB) ([]byte, string, map[string]struct{ RuntimeState RuntimeStateRecord }) {
	t.Helper()
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		t.Skip("explicit read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState RuntimeStateRecord }
	if err := json.Unmarshal(body, &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) == 0 {
		t.Fatal("empty archive")
	}
	return body, path, runs
}

func TestRuntimeStateConsumerSessionExplicitArchiveOracle(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		base := run.RuntimeState
		if err := referenceRuntimeStateConsumerSession(base); err != nil {
			t.Fatal("archive not valid before optimization", err)
		}
		compareRuntimeStateConsumerSession(t, base)
		for arm := uint8(0); arm < 18; arm++ {
			changed := base.Clone()
			mutateRuntimeStateConsumerSession(&changed, arm, "same-hash raw drift")
			compareRuntimeStateConsumerSession(t, changed)
			if err := base.Validate(); err != nil {
				t.Fatal("later call reused changed source", err)
			}
		}
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func assertReadOnlyRuntimeStateConsumerArchive(t testing.TB, body []byte, path string) {
	t.Helper()
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("read-only archive changed")
	}
}

func BenchmarkRuntimeStateConsumerSessionPublicArchive(b *testing.B) {
	_, _, runs := runtimeStateConsumerArchive(b)
	for _, run := range runs {
		for _, original := range []bool{true, false} {
			name := "shared-call"
			if original {
				name = "independent-consumers"
			}
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					var err error
					if original {
						err = referenceRuntimeStateConsumerSession(run.RuntimeState)
					} else {
						err = run.RuntimeState.Validate()
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
