package workorchruntimecontract

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"
)

func TestCommittedHistoricalReadsPreserveResultsAndSeparateCurrentCredit(t *testing.T) {
	s := historicalSessionFixture(t)
	proof := s.Facts.Coverage[0].HistoricalAcceptanceProofs[0]
	for i := range s.Facts.Coverage {
		row := &s.Facts.Coverage[i]
		row.State, row.CWIConvergenceState, row.LastAssessmentID = RuntimeCoverageItemStateSatisfied, RuntimeCWIConvergenceStateSettled, proof.AssessmentDispatchID
		row.EvidenceRefs = slices.Clone(proof.AcceptanceEvidenceRefs)
		if row.HistoricalBehavior != nil {
			row.EvidenceRefs = normalizeStrings(append(slices.Clone(proof.BehaviorEvidenceRefs), proof.AcceptanceEvidenceRefs...))
		}
	}
	if err := auditCommittedHistoricalResults(s, &runtimeHistoricalPreimageSession{}); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("WORKORCH_HISTORICAL_RESULT_ARCHIVE"); path != "" {
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		var reload RuntimeLineageAssertionSources
		if err := json.Unmarshal(body, &reload); err != nil {
			t.Fatal(err)
		}
		// These graphs are deliberately absent from the read API. They are not
		// credentials and may not force re-adjudication of committed products.
		reload.Semantic = RuntimeSemanticPlanningLedger{}
		reload.Facts.Resources.OperationAdmissibilities = nil
		reload.Facts.Focus = CoverageFocus{}
		if err := ValidateRuntimeHistoricalBehaviorCompletions(reload.Facts, reload.Completed); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(s.Facts.Coverage, reload.Facts.Coverage) {
			t.Fatal("read changed historical results")
		}
	}
	// Loss of ordinary current credit requires revocation, not re-adjudication
	// or re-execution of the predecessor's historical effect.
	changed := cloneRuntimeLineageSourcesRaw(s)
	var history *RuntimeCoverageItemFact
	for i := range changed.Facts.Coverage {
		row := &changed.Facts.Coverage[i]
		if row.HistoricalBehavior != nil {
			history = row
			continue
		}
		row.State, row.CWIConvergenceState, row.CurrentBindingRef = RuntimeCoverageItemStateOpen, RuntimeCWIConvergenceStateReopened, nil
	}
	if history == nil {
		t.Fatal("fixture has no committed historical result")
	}
	if ValidateRuntimeHistoricalBehaviorCompletions(changed.Facts, changed.Completed) == nil {
		t.Fatal("stale consumer retained terminal credit")
	}
	history.State, history.CWIConvergenceState, history.LastAssessmentID, history.EvidenceRefs = RuntimeCoverageItemStateOpen, RuntimeCWIConvergenceStateBound, "", nil
	if err := ValidateRuntimeHistoricalBehaviorCompletions(changed.Facts, changed.Completed); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(history.HistoricalBehavior, s.Facts.Coverage[0].HistoricalBehavior) || !reflect.DeepEqual(history.HistoricalAcceptanceProofs, s.Facts.Coverage[0].HistoricalAcceptanceProofs) {
		t.Fatal("revocation changed original results")
	}
}

func TestCommittedHistoricalReadsRejectMissingCorruptAndCrossScopeSources(t *testing.T) {
	s := historicalSessionFixture(t)
	for name, mutate := range map[string]func(*RuntimeLineageAssertionSources){
		"wrong workspace":        func(s *RuntimeLineageAssertionSources) { s.Facts.WorkspaceID = "another" },
		"wrong run":              func(s *RuntimeLineageAssertionSources) { s.Facts.RunID = "another" },
		"missing dispatch":       func(s *RuntimeLineageAssertionSources) { s.Completed = nil },
		"missing evidence":       func(s *RuntimeLineageAssertionSources) { s.Facts.Evidence.Accepted = nil },
		"changed evidence bytes": func(s *RuntimeLineageAssertionSources) { s.Facts.Evidence.Accepted[0].Summary += "changed" },
		"wrong source": func(s *RuntimeLineageAssertionSources) {
			s.Facts.Coverage[0].HistoricalBehavior.Source.AcceptanceItemRef = "another"
		},
		"duplicate behavior member": func(s *RuntimeLineageAssertionSources) {
			b := s.Facts.Coverage[0].HistoricalBehavior
			b.EvidenceRefs = append(b.EvidenceRefs, b.EvidenceRefs[0])
		},
		"borrowed acceptance member": func(s *RuntimeLineageAssertionSources) {
			p := &s.Facts.Coverage[0].HistoricalAcceptanceProofs[0]
			p.AcceptanceEvidenceRefs = p.BehaviorEvidenceRefs
		},
		"replaced acceptance manifest": func(s *RuntimeLineageAssertionSources) {
			s.Facts.Coverage[0].HistoricalAcceptanceProofs[0].AcceptanceBindingRef.SemanticManifestHash = "another"
		},
		"noncanonical binding": func(s *RuntimeLineageAssertionSources) {
			s.Facts.Coverage[0].HistoricalAcceptanceProofs[0].AcceptanceBindingRef.BindingID += " "
		},
		"duplicate assessment": func(s *RuntimeLineageAssertionSources) {
			s.Facts.Assessments = append(s.Facts.Assessments, s.Facts.Assessments[0])
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := cloneRuntimeLineageSourcesRaw(s)
			mutate(&changed)
			if ValidateRuntimeHistoricalBehaviorCompletions(changed.Facts, changed.Completed) == nil {
				t.Fatal("invalid original source accepted")
			}
		})
	}
}

func BenchmarkCommittedHistoricalResultRead(b *testing.B) {
	// The fixture is fixed, complete and nonempty. This is not a fullflow claim.
	path := os.Getenv("WORKORCH_HISTORICAL_RESULT_ARCHIVE")
	if path == "" {
		b.Skip("explicit fixed historical result archive not supplied")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var s RuntimeLineageAssertionSources
	if err := json.Unmarshal(body, &s); err != nil {
		b.Fatal(err)
	}
	for _, mode := range []string{"offline_full_replay", "committed_result_read"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var err error
				if mode == "offline_full_replay" {
					err = auditCommittedHistoricalResults(s, &runtimeHistoricalPreimageSession{})
				} else {
					err = ValidateRuntimeHistoricalBehaviorCompletions(s.Facts, s.Completed)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
