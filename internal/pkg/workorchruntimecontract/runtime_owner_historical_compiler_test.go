package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"testing"
)

func TestOwnerHistoricalCompilerRetiredGenerationAndPurposeOracle(t *testing.T) {
	base := assessmentPreimageSessionFixture(t)
	for _, arm := range []string{"unchanged", "raw manifest", "raw document", "owner", "retirement", "nested empty", "current phase", "history order", "history duplicate", "source seed"} {
		t.Run(arm, func(t *testing.T) {
			s := newRuntimeSemanticValidationSession()
			if err := validateRuntimeSemanticPlanningLedgerInSession(base, s); err != nil {
				t.Fatal(err)
			}
			if _, err := runtimeAssessmentPurposePreimagesFromSemanticLedgerInSession(base, false, s); err != nil {
				t.Fatal(err)
			}
			changed := cloneRuntimeSemanticPlanningLedgerRaw(base)
			switch arm {
			case "raw manifest":
				changed.History[0].Snapshot.Manifest.OwnerCatalogs[0].ObservedRevision = "same hash different raw"
			case "raw document":
				changed.History[0].Snapshot.BatchDocuments[0].Proposals[0].Target.ProspectiveTargetRefs = []string{"different"}
			case "owner":
				changed.History[0].Snapshot.Feasibilities[0].OwnerModuleID = RuntimeModuleShell
			case "retirement":
				changed.History[0].RetiredAtStateRevision = 0
			case "nested empty":
				changed.History[0].Snapshot.History = []RuntimeSemanticPlanningGeneration{}
			case "current phase":
				changed.Phase = RuntimeSemanticPlanningFailed
			case "history order":
				changed.History[1].RetiredAtStateRevision = changed.History[0].RetiredAtStateRevision
			case "history duplicate":
				changed.History = append(changed.History, changed.History[0])
			case "source seed":
				changed.DefinitionSeed.PolicyHash = "same hash different raw"
			}
			for range 2 {
				got := validateRuntimeSemanticPlanningLedgerInSession(changed, s)
				want := referenceSemanticLedgerBeforeSeedSession(changed, changed.Normalized(), newRuntimeSemanticValidationSession())
				if fmt.Sprint(got) != fmt.Sprint(want) {
					t.Fatalf("current source admission hid a raw error: %v != %v", got, want)
				}
				gp, ge := runtimeAssessmentPurposePreimagesFromSemanticLedgerInSession(changed, false, s)
				wp, we := referenceAssessmentPurposePreimagesFromSemanticLedger(changed, false)
				if fmt.Sprint(ge) != fmt.Sprint(we) || !reflect.DeepEqual(gp, wp) {
					t.Fatalf("retired purpose product/first error changed: %v != %v", ge, we)
				}
			}

		})
	}
}

func TestOwnerHistoricalCompilerGenerationChecksBothOriginalParameters(t *testing.T) {
	g := assessmentPreimageSessionFixture(t).History[0]
	s := newRuntimeSemanticValidationSession()
	if err := validateRuntimeSemanticPlanningGenerationNormalized(g, g.Normalized(), s); err != nil {
		t.Fatal(err)
	}
	for parameter := range 2 {
		raw, normalized := cloneRuntimeSemanticPlanningGenerationRaw(g), g.Normalized()
		if parameter == 0 {
			raw.PrerequisiteIDs = append(raw.PrerequisiteIDs, raw.PrerequisiteIDs[0])
		} else {
			normalized.RetiredAtStateRevision = 0
		}
		got := validateRuntimeSemanticPlanningGenerationNormalized(raw, normalized, s)
		want := referenceSemanticGenerationBeforeSeedSession(raw, normalized, newRuntimeSemanticValidationSession())
		if got == nil || fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("parameter %d bypassed complete original gate: %v != %v", parameter, got, want)
		}
	}
}

func FuzzOwnerHistoricalCompilerFullGate(f *testing.F) {
	for arm := uint8(0); arm < 18; arm++ {
		f.Add(arm, "原文\x00")
	}
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		base := assessmentPreimageSessionFixture(t)
		s := newRuntimeSemanticValidationSession()
		if err := validateRuntimeSemanticPlanningLedgerInSession(base, s); err != nil {
			t.Fatal(err)
		}
		if _, err := runtimeAssessmentPurposePreimagesFromSemanticLedgerInSession(base, false, s); err != nil {
			t.Fatal(err)
		}
		r := RuntimeStateRecord{SemanticPlanning: cloneRuntimeSemanticPlanningLedgerRaw(base)}
		mutateRuntimeStateConsumerSession(&r, arm, raw)
		got := validateRuntimeSemanticPlanningLedgerInSession(r.SemanticPlanning, s)
		want := referenceSemanticLedgerBeforeSeedSession(r.SemanticPlanning, r.SemanticPlanning.Normalized(), newRuntimeSemanticValidationSession())
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("retired-history fuzz first error: %v != %v", got, want)
		}
		gp, ge := runtimeAssessmentPurposePreimagesFromSemanticLedgerInSession(r.SemanticPlanning, false, s)
		wp, we := referenceAssessmentPurposePreimagesFromSemanticLedger(r.SemanticPlanning, false)
		if fmt.Sprint(ge) != fmt.Sprint(we) || !reflect.DeepEqual(gp, wp) {
			t.Fatalf("retired-purpose fuzz differs: %v != %v", ge, we)
		}
	})
}
