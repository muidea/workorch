package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func mutateWorkRoundPreimage(r *RuntimeImplementationWorkRound, arm uint8, raw string) {
	switch arm % 20 {
	case 0:
		r.Hash = raw
	case 1:
		r.WorkspaceID = ""
	case 2:
		r.StartupInputHash = raw
	case 3:
		r.Ordinal++
	case 4:
		r.PrecedingReceiptSetHash = raw
	case 5:
		r.ScopeRequest.Policy.TargetSelectorCandidateRefs = []string{raw}
	case 6:
		r.ScopeRequest.ManifestBinding.Manifest.TargetRefs = []string{raw}
	case 7:
		r.ScopeObservation = nil
	case 8:
		r.WorkingBinding = nil
	case 9:
		r.DispatchAuthorityHash = raw
	case 10:
		r.CompletedReceiptSetHash, r.FailedReceiptHash = "completed", "failed"
	case 11:
		r.OutcomeUnknownExecutionKey, r.OutcomeUnknownFence = "unknown", 1
	case 12:
		r.GovernanceDispatchIDs = []string{}
	case 13:
		r.GovernanceDispatchIDs = []string{"duplicate", "duplicate"}
	case 14:
		r.GovernanceReceiptHash = raw
	case 15:
		r.ReceiptDisposition = RuntimeImplementationWorkReceiptDisposition(raw)
	default:
		mutateWorkRoundCatalogSession(r, arm-16, raw)
	}
}

func assertWorkRoundPreimageOracle(t testing.TB, r RuntimeImplementationWorkRound, session *runtimeSemanticValidationSession) {
	t.Helper()
	before, _ := json.Marshal(r)
	got, want := r.validateInSemanticSession(session), referenceWorkRoundCatalogSession(r)
	if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
		t.Fatalf("complete round first error differs: %v != %v", got, want)
	}
	after, _ := json.Marshal(r)
	if !bytes.Equal(before, after) {
		t.Fatal("round raw input changed")
	}
}

func TestWorkRoundPreimageCompleteGateAndDetachedRaw(t *testing.T) {
	ledger, _ := implementationWorkLedgerFixture(t)
	r := ledger.Rounds[0]
	session := newRuntimeSemanticValidationSession()
	assertWorkRoundPreimageOracle(t, r, session)
	assertWorkRoundPreimageOracle(t, r.Clone(), session)
	if len(session.workRounds) != 1 || !reflect.DeepEqual(session.workRounds[0], r) {
		t.Fatal("successful full raw preimage was not retained once")
	}
	for arm := uint8(0); arm < 20; arm++ {
		changed := r.Clone()
		mutateWorkRoundPreimage(&changed, arm, "raw\x00漂移")
		// Retaining the old hash cannot authorize the changed body.
		changed.Hash = r.Hash
		assertWorkRoundPreimageOracle(t, changed, session)
		assertWorkRoundPreimageOracle(t, r, session)
		assertWorkRoundPreimageOracle(t, changed, newRuntimeSemanticValidationSession())
	}
	if len(session.workRounds) != 1 {
		t.Fatal("failed gate contributed a success preimage")
	}
	// Mutating a caller alias must not mutate the saved exact raw source.
	original := r.Clone()
	r.WorkingBinding.OwnerCatalog.TargetRefs[0] = "caller alias drift"
	if !reflect.DeepEqual(original, session.workRounds[0]) {
		t.Fatal("source aliases overwrite saved preimage")
	}
	assertWorkRoundPreimageOracle(t, r, session)
	body, _ := json.Marshal(original)
	var loaded RuntimeImplementationWorkRound
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	assertWorkRoundPreimageOracle(t, loaded, session)
	assertWorkRoundPreimageOracle(t, loaded, newRuntimeSemanticValidationSession())
}

func FuzzWorkRoundPreimageFullOracle(f *testing.F) {
	for arm := uint8(0); arm < 20; arm++ {
		f.Add(arm, "原\x00文")
	}
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		ledger, _ := implementationWorkLedgerFixture(t)
		r := ledger.Rounds[0]
		session := newRuntimeSemanticValidationSession()
		assertWorkRoundPreimageOracle(t, r, session)
		mutateWorkRoundPreimage(&r, arm, raw)
		assertWorkRoundPreimageOracle(t, r, session)
	})
}

func TestWorkRoundPreimageReadOnlyArchiveOracle(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		session := newRuntimeSemanticValidationSession()
		for _, r := range run.RuntimeState.ImplementationWork.Rounds {
			for range 3 {
				assertWorkRoundPreimageOracle(t, r, session)
			}
			for arm := uint8(0); arm < 20; arm++ {
				changed := r.Clone()
				mutateWorkRoundPreimage(&changed, arm, "raw drift")
				changed.Hash = r.Hash
				assertWorkRoundPreimageOracle(t, changed, session)
			}
		}
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func BenchmarkWorkRoundPreimageCompleteCall(b *testing.B) {
	_, _, runs := runtimeStateConsumerArchive(b)
	var rounds []RuntimeImplementationWorkRound
	for _, run := range runs {
		rounds = run.RuntimeState.ImplementationWork.Rounds
		break
	}
	if len(rounds) == 0 {
		b.Fatal("archive has no work rounds")
	}
	for _, shared := range []bool{false, true} {
		name := "independent"
		if shared {
			name = "same_complete_call"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				var session *runtimeSemanticValidationSession
				if shared {
					session = newRuntimeSemanticValidationSession()
				}
				for range 3 {
					for _, r := range rounds {
						if err := r.validateInSemanticSession(session); err != nil {
							b.Fatal(err)
						}
					}
				}
			}
		})
	}
}
