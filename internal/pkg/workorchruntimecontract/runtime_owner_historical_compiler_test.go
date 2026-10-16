package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func historicalCompilerImage(r RuntimeStateRecord) RuntimeOwnerStateSourceImage {
	return RuntimeOwnerStateSourceImage{Original: &r, Sources: historicalSessionArchiveSources(r), Revision: r.StateRevision,
		FeedbackVersion: r.ResultFeedbackVersion, BeforeSemantic: func() error { return nil }, AfterSemantic: func(*RuntimeOwnerInvariantSourceGate) error { return nil }}
}

func historicalCompilerRecord(c *RuntimeOwnerHistoricalCompiler, r RuntimeStateRecord) error {
	return c.Compile(r.Facts.WorkspaceID, r.Facts.RunID, func(gate func(RuntimeOwnerStateSourceImage) error) error { return gate(historicalCompilerImage(r)) })
}

func compareOwnerHistoricalCompiler(t testing.TB, original, changed RuntimeStateRecord) {
	t.Helper()
	c := &RuntimeOwnerHistoricalCompiler{}
	for _, r := range []RuntimeStateRecord{original, original.Clone(), changed, changed, original} {
		before := r.Clone()
		got := historicalCompilerRecord(c, r)
		want := CompileRuntimeOwnerStateTransaction(func(gate func(RuntimeOwnerStateSourceImage) error) error { return gate(historicalCompilerImage(r)) })
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("cross-event full gate first error differs: %v != %v", got, want)
		}
		if !reflect.DeepEqual(r, before) {
			t.Fatal("cross-event compiler changed raw input")
		}
		if got != nil && c.semantic != nil {
			t.Fatal("failed event retained prior or partial successes")
		}
	}
}

func TestOwnerHistoricalCompilerIncompleteImagesStayFailClosed(t *testing.T) {
	for arm := uint8(0); arm < 18; arm++ {
		r := terminalHistoricalAcceptanceFixture(t)
		if r.Validate() == nil {
			t.Fatal("incomplete consumer fixture unexpectedly became a full Runtime image")
		}
		changed := r.Clone()
		mutateRuntimeStateConsumerSession(&changed, arm, "same-hash raw\x00变化")
		compareOwnerHistoricalCompiler(t, r, changed)
		body, _ := json.Marshal(changed)
		var loaded RuntimeStateRecord
		if err := json.Unmarshal(body, &loaded); err != nil {
			t.Fatal(err)
		}
		compareOwnerHistoricalCompiler(t, r, loaded)
	}
}

func TestOwnerHistoricalCompilerRetentionScopeAndLifetime(t *testing.T) {
	r := terminalHistoricalAcceptanceFixture(t)
	c := &RuntimeOwnerHistoricalCompiler{}
	m := semanticWorkValidationManifest(t)
	warmOwnerHistoricalCompiler(t, c, r, m)
	if c.semantic == nil || len(c.semantic.manifests) == 0 || len(c.semantic.workStates) != 0 {
		t.Fatal("no pure semantic reuse, or retained current whole-state success")
	}
	prior := c.semantic
	// A detached but identical image must preserve the privately compiled entry.
	if err := c.Compile(r.Facts.WorkspaceID, r.Facts.RunID, func(func(RuntimeOwnerStateSourceImage) error) error { return nil }); err != nil || c.semantic == nil {
		t.Fatal(err)
	}
	for key, entries := range prior.manifests {
		if !reflect.DeepEqual(entries, c.semantic.manifests[key]) {
			t.Fatal("unchanged historical subtree was not retained")
		}
		assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(m), reflect.ValueOf(entries[0].raw), "historical manifest")
	}
	// Switching the declared scope must not import previous entries. A
	// mismatched image takes the cold path and cannot populate that new scope.
	if err := c.Compile("different-workspace", "different-run", func(func(RuntimeOwnerStateSourceImage) error) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if len(c.semantic.manifests) != 0 || len(c.semantic.catalogs) != 0 {
		t.Fatal("scope switch borrowed another scope's entries")
	}
	warmOwnerHistoricalCompiler(t, c, r, m)
	var captured func(RuntimeOwnerStateSourceImage) error
	if err := c.Compile(r.Facts.WorkspaceID, r.Facts.RunID, func(gate func(RuntimeOwnerStateSourceImage) error) error {
		captured = gate
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := captured(historicalCompilerImage(r)); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatal("retained gate survived its event", err)
	}
	c.Close()
	if c.semantic != nil || c.workspace != "" || c.run != "" {
		t.Fatal("owner close retained raw data")
	}
	want := CompileRuntimeOwnerStateTransaction(func(gate func(RuntimeOwnerStateSourceImage) error) error { return gate(historicalCompilerImage(r)) })
	if err := historicalCompilerRecord(c, r); fmt.Sprint(err) != fmt.Sprint(want) || c.semantic != nil {
		t.Fatal("closed compiler changed cold validation or resurrected cache", err)
	}
	// Reconstruction starts with no compiled history; serialization cannot
	// carry successful products into a new controller/process.
	wire, _ := json.Marshal(prior)
	var fresh RuntimeOwnerHistoricalCompiler
	if err := json.Unmarshal(wire, &fresh); err != nil || fresh.semantic != nil {
		t.Fatal("serialized private products survived reconstruction", err)
	}
}

// Prepare real independently validated pure subtrees, not fabricated success
// flags. Production-shaped full-image tests also live in runtime package.
func warmOwnerHistoricalCompiler(t *testing.T, c *RuntimeOwnerHistoricalCompiler, r RuntimeStateRecord, m RuntimeSemanticPlanManifest) {
	t.Helper()
	s := newRuntimeSemanticValidationSession()
	if _, err := validateRuntimeSemanticPlanManifestInSession(m, s); err != nil {
		t.Fatal(err)
	}
	c.workspace, c.run, c.semantic = r.Facts.WorkspaceID, r.Facts.RunID, s
}

func TestOwnerHistoricalCompilerFailurePanicAndColdContention(t *testing.T) {
	r := terminalHistoricalAcceptanceFixture(t)
	c := &RuntimeOwnerHistoricalCompiler{}
	sentinel := errors.New("owner continuation rejected")
	for _, exit := range []string{"failure", "panic"} {
		warmOwnerHistoricalCompiler(t, c, r, semanticWorkValidationManifest(t))
		func() {
			defer func() {
				if value := recover(); (exit == "panic") != (value == sentinel) {
					t.Fatalf("panic changed: %v", value)
				}
			}()
			err := c.Compile(r.Facts.WorkspaceID, r.Facts.RunID, func(gate func(RuntimeOwnerStateSourceImage) error) error {
				if exit == "panic" {
					panic(sentinel)
				}
				first := gate(historicalCompilerImage(r))
				if first == nil {
					t.Fatal("invalid raw image admitted")
				}
				if err := gate(historicalCompilerImage(r)); err != first {
					t.Fatal("failure not sticky", err)
				}
				return sentinel
			})
			if err != sentinel {
				t.Fatal("first error lost", err)
			}
		}()
		if c.semantic != nil {
			t.Fatal("failure/panic retained entries")
		}
	}
	// Hold an event open while another event uses a wholly cold context.
	if err := c.Compile(r.Facts.WorkspaceID, r.Facts.RunID, func(gate func(RuntimeOwnerStateSourceImage) error) error {
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.Compile(r.Facts.WorkspaceID, r.Facts.RunID, func(func(RuntimeOwnerStateSourceImage) error) error { return nil }); err != nil {
				t.Error(err)
			}
		}()
		wg.Wait()
		if err := c.Compile(r.Facts.WorkspaceID, r.Facts.RunID, func(func(RuntimeOwnerStateSourceImage) error) error { return nil }); err != nil {
			return err
		} // Recursive compilation also stays cold.
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerHistoricalCompilerAllowlistAndEviction(t *testing.T) {
	s := newRuntimeSemanticValidationSession()
	s.workStates = []RuntimeStateRecord{{ProcessedEventIDs: []string{"never retained"}}}
	retained := retainRuntimeOwnerHistoricalSubtrees(s)
	if len(retained.workStates) != 0 {
		t.Fatal("retained whole current state")
	}
	s.seeds = make([]RuntimeDefinitionSeed, runtimeOwnerHistoricalEntryLimit+1)
	if retainRuntimeOwnerHistoricalSubtrees(s) != nil {
		t.Fatal("entry limit did not evict whole optimization")
	}
	s.seeds = nil
	s.manifests["same hash"] = []runtimeSemanticManifestValidationEntry{{validated: runtimeValidatedSemanticManifest{documents: &runtimeSemanticDocumentValidationSession{values: make([]RuntimeSemanticPlanDocument, runtimeOwnerHistoricalEntryLimit)}}}}
	if retainRuntimeOwnerHistoricalSubtrees(s) != nil {
		t.Fatal("nested documents escaped retention limit")
	}
}

func TestOwnerHistoricalCompilerRetiredGenerationAndPurposeOracle(t *testing.T) {
	base := assessmentPreimageSessionFixture(t)
	for _, arm := range []string{"unchanged", "raw manifest", "raw document", "owner", "retirement", "nested empty", "current phase", "history order", "history duplicate", "source seed"} {
		t.Run(arm, func(t *testing.T) {
			s := newRuntimeSemanticValidationSession()
			s.ownerHistory = &runtimeOwnerHistoricalSemanticCompiler{}
			if err := validateRuntimeSemanticPlanningLedgerInSession(base, s); err != nil {
				t.Fatal(err)
			}
			if _, err := runtimeAssessmentPurposePreimagesFromSemanticLedgerInSession(base, false, s); err != nil {
				t.Fatal(err)
			}
			if len(s.ownerHistory.generations) != len(base.History) || len(s.ownerHistory.purposes) != len(base.History) {
				t.Fatal("retired subtree not compiled")
			}
			prior := cloneRuntimeSemanticPlanningGenerationRaw(s.ownerHistory.generations[0].raw)
			assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(base.History[0]), reflect.ValueOf(s.ownerHistory.generations[0].raw), "retired generation")
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
					t.Fatalf("retired cache hid current/join/raw error: %v != %v", got, want)
				}
				gp, ge := runtimeAssessmentPurposePreimagesFromSemanticLedgerInSession(changed, false, s)
				wp, we := referenceAssessmentPurposePreimagesFromSemanticLedger(changed, false)
				if fmt.Sprint(ge) != fmt.Sprint(we) || !reflect.DeepEqual(gp, wp) {
					t.Fatalf("retired purpose product/first error changed: %v != %v", ge, we)
				}
			}
			if !reflect.DeepEqual(prior, s.ownerHistory.generations[0].raw) {
				t.Fatal("caller mutation changed detached history")
			}
		})
	}
}

func TestOwnerHistoricalCompilerGenerationChecksBothOriginalParameters(t *testing.T) {
	g := assessmentPreimageSessionFixture(t).History[0]
	s := newRuntimeSemanticValidationSession()
	s.ownerHistory = &runtimeOwnerHistoricalSemanticCompiler{}
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
		if got == nil || fmt.Sprint(got) != fmt.Sprint(want) || len(s.ownerHistory.generations) != 1 {
			t.Fatalf("parameter %d bypassed complete original gate: %v != %v", parameter, got, want)
		}
	}
}

func TestOwnerHistoricalCompilerReadOnlyArchiveOracle(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		for arm := uint8(0); arm < 18; arm++ {
			changed := run.RuntimeState.Clone()
			mutateRuntimeStateConsumerSession(&changed, arm, "raw unchanged hash")
			compareOwnerHistoricalCompiler(t, run.RuntimeState, changed)
		}
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func TestOwnerHistoricalCompilerReadOnlyArchiveRetention(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		c := &RuntimeOwnerHistoricalCompiler{}
		for range 2 {
			if err := historicalCompilerRecord(c, run.RuntimeState); err != nil {
				t.Fatal(err)
			}
			if c.semantic == nil {
				t.Fatal("archive exceeds bounded retention; no cross-event optimization active")
			}
			t.Logf("retained manifests=%d catalogs=%d retired-generations=%d historical-purposes=%d", len(c.semantic.manifests), len(c.semantic.catalogs), len(c.semantic.ownerHistory.generations), len(c.semantic.ownerHistory.purposes))
		}
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
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
		s.ownerHistory = &runtimeOwnerHistoricalSemanticCompiler{}
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
