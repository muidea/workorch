package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func terminalHistoricalAcceptanceFixture(t *testing.T) RuntimeStateRecord {
	t.Helper()
	sources := historicalAcceptanceProofFixture(t)
	proof, err := CompileRuntimeHistoricalAcceptanceProof(sources, "history", "original-assessment")
	if err != nil {
		t.Fatal(err)
	}
	for index := range sources.Facts.Coverage {
		row := &sources.Facts.Coverage[index]
		row.State, row.CWIConvergenceState, row.LastAssessmentID = RuntimeCoverageItemStateSatisfied, RuntimeCWIConvergenceStateSettled, proof.AssessmentDispatchID
		row.EvidenceRefs = append([]string(nil), proof.AcceptanceEvidenceRefs...)
		if row.ID == "history" {
			row.HistoricalAcceptanceProofs = []RuntimeHistoricalAcceptanceProof{proof.Clone()}
			row.EvidenceRefs = normalizeStrings(append(append([]string(nil), proof.BehaviorEvidenceRefs...), proof.AcceptanceEvidenceRefs...))
		}
	}
	sources.Facts.EvidenceRevision = runtimeEvidenceRevisionForSequence(3)
	sources.Facts.Assessments[0].Revision = sources.Facts.EvidenceRevision
	observation := sources.Facts.EvidenceApplicabilityObservations[0].CloneRaw()
	observation.Request.EvidenceRevision = sources.Facts.EvidenceRevision
	observation.Request.IssuedAtStateRevision, observation.RecordedAtStateRevision = 12, 13
	observation.Request = observation.Request.WithID()
	sources.Facts.EvidenceApplicabilityObservations = append(sources.Facts.EvidenceApplicabilityObservations, observation)
	sources.Facts.RecoveryPolicy = DefaultRuntimeRecoveryPolicy()
	return RuntimeStateRecord{ResultFeedbackVersion: RuntimeResultFeedbackVersionCurrent, StateRevision: 14, Definition: sources.Definition, Facts: sources.Facts, SemanticPlanning: sources.Semantic,
		CompletedDispatches: sources.Completed, OperationReceipts: sources.Receipts, ProcessedEventIDs: sources.ProcessedEventIDs, EvidenceSequence: 3,
		Decision: RuntimeDecision{State: ConvergenceStateComplete, AuthoritySnapshot: AuthoritySnapshot{ID: "terminal-authority"}, Terminal: &RuntimeTerminalDecision{Code: "coverage_complete", Summary: "原 all-of 验收完成"}}}
}

func TestTerminalHistoricalAcceptancePreservesBothSourcesAndOriginalChecks(t *testing.T) {
	source := terminalHistoricalAcceptanceFixture(t)
	before, _ := json.Marshal(source)
	if err := validateRuntimeTerminalEvidenceApplicability(source); err != nil {
		t.Fatal(err)
	}
	ownerSet, err := CompileRuntimeTerminalOwnerReadDependencies(source)
	if err != nil || len(ownerSet.Entries) != 1 || ownerSet.Entries[0].Revision != "actually-read-current-bytes" {
		t.Fatalf("historical input/postimage entered current union: %+v %v", ownerSet, err)
	}
	outcomes := DeriveRuntimeTerminalRequirementOutcomesFromState(source, ownerSet)
	var historical RuntimeRequirementOutcome
	for _, outcome := range outcomes {
		if err := outcome.Validate(); err != nil {
			t.Fatal(err)
		}
		if outcome.CoverageWorkItemID == "history" {
			historical = outcome
		}
	}
	if historical.CoverageState != RuntimeRequirementCovered || historical.AssessmentState != RuntimeRequirementSatisfied || len(historical.AcceptedEvidenceRefs) != 2 || len(historical.EvidenceItems) != 2 || len(historical.SourceObligations) == 0 {
		t.Fatalf("historical material lost independent all-of sources: %+v", historical)
	}
	for _, outcome := range historical.SourceObligations {
		if outcome.CurrentBindingRef == nil || outcome.CurrentBindingRef.BindingID == source.Facts.Coverage[0].CurrentBindingRef.BindingID || len(outcome.AcceptedEvidenceRefs) != 1 || outcome.AssessmentState != RuntimeRequirementSatisfied {
			t.Fatal("source criterion did not consume independently mapped ordinary proof")
		}
		found := false
		for _, original := range source.Facts.DefinitionSeed.CoverageItems {
			if original.ID == "history" {
				for _, check := range original.SourceObligationChecks {
					found = found || check == outcome.Check
				}
			}
		}
		if !found {
			t.Fatal("historical original criterion/check identity rewritten as consumer")
		}
	}
	material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
	if err != nil {
		t.Fatalf("mapped terminal material: %v", err)
	}
	if err := material.RuntimeCloseout.ValidateRaw(); err != nil {
		t.Fatalf("mapped formal closeout: %v", err)
	}
	if err := runtimeTerminalResultIndex(source, material); err != nil {
		t.Fatalf("formal Result omitted mapped original sources: %v", err)
	}
	var reload RuntimeStateRecord
	if err := json.Unmarshal(before, &reload); err != nil {
		t.Fatal(err)
	}
	repeated, err := DeriveRuntimeTerminalResultMaterialFromState(reload)
	if err != nil || !reflect.DeepEqual(material, repeated) {
		t.Fatalf("mapped material changed on restart: %v", err)
	}
	after, _ := json.Marshal(source)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("material compiler changed original accepted evidence/credit")
	}
}

func TestTerminalHistoricalAcceptanceRejectsUnsettledStaleAndGenerationBorrowing(t *testing.T) {
	for name, mutate := range map[string]func(*RuntimeStateRecord){
		"no settlement":     func(s *RuntimeStateRecord) { s.Facts.Coverage[0].HistoricalAcceptanceProofs = nil },
		"consumer reopened": func(s *RuntimeStateRecord) { s.Facts.Coverage[1].State = RuntimeCoverageItemStateOpen },
		"new revision":      func(s *RuntimeStateRecord) { s.Facts.EvidenceRevision = runtimeEvidenceRevisionForSequence(4) },
		"new generation":    func(s *RuntimeStateRecord) { s.Reconciliation.RequestHash = "new-resume-generation" },
		"missing current observation": func(s *RuntimeStateRecord) {
			s.Facts.EvidenceApplicabilityObservations = s.Facts.EvidenceApplicabilityObservations[:1]
		},
		"changed actual bytes": func(s *RuntimeStateRecord) {
			o := &s.Facts.EvidenceApplicabilityObservations[1]
			o.CurrentDependencies.Entries[0].Revision = "actually-changed-bytes"
			*o.CurrentDependencies = o.CurrentDependencies.WithHash()
		},
	} {
		t.Run(name, func(t *testing.T) {
			source := terminalHistoricalAcceptanceFixture(t)
			mutate(&source)
			before, _ := json.Marshal(source)
			if _, err := DeriveRuntimeTerminalResultMaterialFromState(source); err == nil {
				t.Fatal("stale/partial settlement sealed successful mapped material")
			}
			after, _ := json.Marshal(source)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("terminal rejection rewrote original state")
			}
		})
	}
}
