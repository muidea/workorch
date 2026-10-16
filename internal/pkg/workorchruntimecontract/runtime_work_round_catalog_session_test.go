package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"testing"
)

func compareWorkRoundCatalogSession(t testing.TB, round RuntimeImplementationWorkRound, session *runtimeSemanticValidationSession) {
	t.Helper()
	before, _ := json.Marshal(round)
	got, want := round.validateInSemanticSession(session), referenceWorkRoundCatalogSession(round)
	if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
		t.Fatalf("round first error differs: %v != %v", got, want)
	}
	if round.WorkingBinding != nil {
		got = round.WorkingBinding.validateForRoundInSemanticSession(round, session)
		want = referenceWorkingBindingCatalogSession(*round.WorkingBinding, round)
		if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
			t.Fatalf("working binding first error differs: %v != %v", got, want)
		}
	}
	after, _ := json.Marshal(round)
	if !bytes.Equal(before, after) {
		t.Fatal("round gate changed raw original")
	}
}

func mutateWorkRoundCatalogSession(r *RuntimeImplementationWorkRound, arm uint8, raw string) {
	if r.WorkingBinding == nil {
		return
	}
	b := r.WorkingBinding
	switch arm % 10 {
	case 0:
		b.OwnerCatalog.SnapshotID = raw
	case 1:
		b.OwnerCatalog.TargetRefs = append(b.OwnerCatalog.TargetRefs, "", raw)
	case 2:
		b.OwnerCatalog.OwnerModuleID = raw
	case 3:
		b.MutationInputCatalogs = []RuntimeOperationAdmissibility{b.OwnerCatalog.Clone(), b.OwnerCatalog.Clone()}
	case 4:
		b.MutationInputCatalogs = []RuntimeOperationAdmissibility{b.OwnerCatalog.Clone()}
		b.MutationInputCatalogs[0].SnapshotID = raw
	case 5:
		b.MutationInputCatalogs = []RuntimeOperationAdmissibility{}
	case 6:
		b.ExcludedTargetRefs = []string{raw}
	case 7:
		b.CatalogObservation.ContentHash = raw
	case 8:
		b.Proposals = nil
	case 9:
		r.ScopeObservation = nil
	}
	rehashImplementationWorkRound(r)
}

func TestWorkRoundCatalogSessionRawOracleAndLaterMutation(t *testing.T) {
	ledger, _ := implementationWorkLedgerFixture(t)
	base := ledger.Rounds[0]
	base.WorkingBinding.MutationInputCatalogs = []RuntimeOperationAdmissibility{base.WorkingBinding.OwnerCatalog.Clone()}
	rehashImplementationWorkRound(&base)
	if err := referenceWorkRoundCatalogSession(base); err != nil {
		t.Fatal(err)
	}
	session := newRuntimeSemanticValidationSession()
	for range 2 {
		compareWorkRoundCatalogSession(t, base, session)
	}
	for arm := uint8(0); arm < 10; arm++ {
		changed := base.Clone()
		mutateWorkRoundCatalogSession(&changed, arm, "same-hash raw\x00drift")
		compareWorkRoundCatalogSession(t, changed, session)
		compareWorkRoundCatalogSession(t, base, session)
	}
	// Input and public return aliases cannot overwrite the session's raw success preimage.
	base.WorkingBinding.OwnerCatalog.TargetRefs[0] = "source mutation"
	compareWorkRoundCatalogSession(t, base, session)
}

func FuzzWorkRoundCatalogSessionOracle(f *testing.F) {
	for arm := uint8(0); arm < 10; arm++ {
		f.Add(arm, "原\x00文")
	}
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		ledger, _ := implementationWorkLedgerFixture(t)
		r := ledger.Rounds[0]
		session := newRuntimeSemanticValidationSession()
		compareWorkRoundCatalogSession(t, r, session)
		mutateWorkRoundCatalogSession(&r, arm, raw)
		compareWorkRoundCatalogSession(t, r, session)
		body, _ := json.Marshal(r)
		var loaded RuntimeImplementationWorkRound
		if err := json.Unmarshal(body, &loaded); err != nil {
			t.Fatal(err)
		}
		compareWorkRoundCatalogSession(t, loaded, session)
	})
}

func TestWorkRoundCatalogSessionExplicitArchiveOracle(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		session := newRuntimeSemanticValidationSession()
		if len(run.RuntimeState.ImplementationWork.Rounds) == 0 {
			t.Fatal("no actual work rounds")
		}
		for _, round := range run.RuntimeState.ImplementationWork.Rounds {
			if err := referenceWorkRoundCatalogSession(round); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				compareWorkRoundCatalogSession(t, round, session)
			}
			for arm := uint8(0); arm < 10; arm++ {
				changed := round.Clone()
				mutateWorkRoundCatalogSession(&changed, arm, "same-hash raw drift")
				compareWorkRoundCatalogSession(t, changed, session)
			}
		}
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}
