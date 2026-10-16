package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"testing"
)

func workScopeSessionSource(t *testing.T) (RuntimeImplementationWorkScopeRequest, RuntimeImplementationWorkScopeSources) {
	t.Helper()
	in := workCompilerSessionFixture(t)
	r := in.Round.ScopeRequest.Clone()
	s := RuntimeImplementationWorkScopeSources{WorkspaceID: in.Sources.WorkspaceID, RunID: in.Sources.RunID, AuthoritySnapshotID: r.AuthoritySnapshotID,
		Seed: in.Sources.Seed, Focuses: in.Sources.Focuses, Profiles: in.Sources.Profiles, CoverageWorkItemID: r.CoverageWorkItemID, ManifestBinding: r.ManifestBinding}
	return r, s
}

func mutateWorkScopeSession(r *RuntimeImplementationWorkScopeRequest, s *RuntimeImplementationWorkScopeSources, arm uint8, value string) {
	switch arm % 13 {
	case 0:
	case 1:
		s.Seed.CoverageItems[0].TargetText = value
	case 2:
		s.Focuses[0].AssessmentText = value
	case 3:
		s.Profiles[0].EvidenceRequirements[0].ImplementationWorkPolicy.TargetSelectorCandidateRefs[0] = value
	case 4:
		s.ManifestBinding.Manifest.TargetRefs[0] = value
	case 5:
		s.ManifestBinding.Observation.Dependencies[0].Revision = value
	case 6:
		s.AuthoritySnapshotID = value
	case 7:
		s.CoverageWorkItemID = "wrong-cwi"
	case 8:
		s.WorkspaceID = "wrong-workspace"
	case 9:
		r.Policy.TargetSelectorCandidateRefs[0] = value
	case 10:
		r.RequestID = value
	case 11:
		s.Focuses = append(s.Focuses, cloneRuntimeDefinitionFocusRaw(s.Focuses[0]))
	case 12:
		s.Seed.CoverageItems = nil
	}
}

func compareWorkScopeSession(t *testing.T, r RuntimeImplementationWorkScopeRequest, s RuntimeImplementationWorkScopeSources, session *runtimeSemanticValidationSession) {
	t.Helper()
	before, err := json.Marshal(struct {
		R RuntimeImplementationWorkScopeRequest
		S RuntimeImplementationWorkScopeSources
	}{r, s})
	if err != nil {
		t.Fatal(err)
	}
	got := validateRuntimeWorkScopeInSession(r, s, session)
	want := referenceWorkScopeAgainstSources(r, s)
	if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
		t.Fatalf("scope gate/first error differs: %v != %v", got, want)
	}
	after, _ := json.Marshal(struct {
		R RuntimeImplementationWorkScopeRequest
		S RuntimeImplementationWorkScopeSources
	}{r, s})
	if !bytes.Equal(before, after) {
		t.Fatal("scope session changed raw source")
	}
}

func TestWorkScopeSessionExactRawSourceIsolationAndReload(t *testing.T) {
	for arm := uint8(0); arm < 13; arm++ {
		r, s := workScopeSessionSource(t)
		session := newRuntimeSemanticValidationSession()
		if err := validateRuntimeWorkScopeInSession(r, s, session); err != nil {
			t.Fatal(err)
		}
		mutateWorkScopeSession(&r, &s, arm, "old-hash raw mutation")
		compareWorkScopeSession(t, r, s, session)
		compareWorkScopeSession(t, r, s, newRuntimeSemanticValidationSession())
		body, _ := json.Marshal(struct {
			R RuntimeImplementationWorkScopeRequest
			S RuntimeImplementationWorkScopeSources
		}{r, s})
		var loaded struct {
			R RuntimeImplementationWorkScopeRequest
			S RuntimeImplementationWorkScopeSources
		}
		if err := json.Unmarshal(body, &loaded); err != nil {
			t.Fatal(err)
		}
		compareWorkScopeSession(t, loaded.R, loaded.S, newRuntimeSemanticValidationSession())
	}
}

func TestWorkLedgerScopeSessionMatchesOriginal(t *testing.T) {
	for arm := uint8(0); arm < 13; arm++ {
		ledger, source := implementationWorkLedgerFixture(t)
		r := &ledger.Rounds[0].ScopeRequest
		s := RuntimeImplementationWorkScopeSources{Seed: source.Seed, Focuses: source.Focuses, Profiles: source.Profiles, ManifestBinding: r.ManifestBinding}
		mutateWorkScopeSession(r, &s, arm, "raw-drift")
		source.Seed, source.Focuses, source.Profiles = s.Seed, s.Focuses, s.Profiles
		got := ledger.ValidateForSources(source)
		want := referenceWorkLedgerBeforeScopeSession(ledger, source)
		if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
			t.Fatalf("ledger gate differs: %v != %v", got, want)
		}
	}
}

func FuzzWorkScopeSessionMatchesIndependentCompiler(f *testing.F) {
	for arm := uint8(0); arm < 13; arm++ {
		f.Add(arm, "raw")
	}
	f.Fuzz(func(t *testing.T, arm uint8, value string) {
		if len(value) > 128 {
			t.Skip()
		}
		r, s := workScopeSessionSource(t)
		session := newRuntimeSemanticValidationSession()
		if err := validateRuntimeWorkScopeInSession(r, s, session); err != nil {
			t.Fatal(err)
		}
		mutateWorkScopeSession(&r, &s, arm, value)
		compareWorkScopeSession(t, r, s, session)
	})
}
