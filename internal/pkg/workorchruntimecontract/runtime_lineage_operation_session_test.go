package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func mutateLineageOperationSource(r *RuntimeStateRecord, arm uint8, raw string) {
	switch arm % 12 {
	case 0:
		r.Facts.WorkspaceID = ""
	case 1:
		r.Facts.RunID = ""
	case 2:
		r.ActiveDispatch = &RuntimeDispatchRecord{}
	case 3:
		r.CompletedDispatches = append(r.CompletedDispatches, RuntimeDispatchRecord{})
	case 4:
		r.OperationReceipts = append(r.OperationReceipts, RuntimeOperationReceipt{})
	case 5:
		r.CandidateAttempts = append(r.CandidateAttempts, RuntimeCandidateAttemptRecord{})
	case 6:
		r.CandidateRequestSessions = append(r.CandidateRequestSessions, RuntimeCandidateRequestSession{})
	case 7:
		r.OperationGovernanceBindings = append(r.OperationGovernanceBindings, RuntimeOperationGovernanceBinding{})
	case 8:
		r.GovernanceResolutions = append(r.GovernanceResolutions, RuntimeGovernanceResolutionRecord{})
	case 9:
		r.Facts.Evidence.Accepted = append(r.Facts.Evidence.Accepted, EvidenceFact{})
	case 10:
		r.Facts.Evidence.Rejected = append(r.Facts.Evidence.Rejected, EvidenceFact{})
	case 11:
		if len(r.CompletedDispatches) != 0 {
			r.CompletedDispatches[0].CandidateManifestHash = raw
		}
	}
}

func assertLineageOperationSession(t testing.TB, s RuntimeLineageAssertionSources) {
	t.Helper()
	raw := runtimeLineageOperationSourceRecord(s)
	saved := raw.Clone()
	session := &runtimeHistoricalPreimageSession{}
	for range 2 {
		got, want := runtimeValidateLineageOperationsInSession(s, session), runtimeValidateLineageOperations(s)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("complete operation first error changed: %v != %v", got, want)
		}
		if want == nil && len(session.lineageOperations) != 1 || want != nil && len(session.lineageOperations) != 0 {
			t.Fatal("success not reused exactly once, or failure registered as success")
		}
	}
	if !reflect.DeepEqual(raw, saved) {
		t.Fatal("operation gate changed original raw sources")
	}
	for arm := uint8(0); arm < 12; arm++ {
		changed := saved.Clone()
		mutateLineageOperationSource(&changed, arm, "same hash\x00原文漂移")
		source := historicalSessionArchiveSources(changed)
		entries := len(session.lineageOperations)
		got, want := runtimeValidateLineageOperationsInSession(source, session), runtimeValidateLineageOperations(source)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("mutation %d borrowed old success: %v != %v", arm, got, want)
		}
		if want != nil && len(session.lineageOperations) != entries {
			t.Fatal("rejected source added a successful preimage")
		}
		body, _ := json.Marshal(changed)
		var loaded RuntimeStateRecord
		if err := json.Unmarshal(body, &loaded); err != nil {
			t.Fatal(err)
		}
		source = historicalSessionArchiveSources(loaded)
		if fmt.Sprint(runtimeValidateLineageOperationsInSession(source, session)) != fmt.Sprint(runtimeValidateLineageOperations(source)) {
			t.Fatal("JSON reload changed complete operation gate")
		}
	}
	// Mutate the caller's original backing storage after a successful warm gate.
	if runtimeValidateLineageOperations(s) == nil && len(raw.CompletedDispatches) != 0 {
		raw.CompletedDispatches[0].CandidateManifestHash = "caller-mutated-original-alias"
		changed := historicalSessionArchiveSources(raw)
		if fmt.Sprint(runtimeValidateLineageOperationsInSession(changed, session)) != fmt.Sprint(runtimeValidateLineageOperations(changed)) || !reflect.DeepEqual(session.lineageOperations[0], saved) {
			t.Fatal("original alias changed or borrowed detached successful preimage")
		}
	}
	// A fresh independent invocation does not inherit this session's products.
	restored := historicalSessionArchiveSources(saved)
	if fmt.Sprint(runtimeValidateLineageOperationsInSession(restored, &runtimeHistoricalPreimageSession{})) != fmt.Sprint(runtimeValidateLineageOperations(restored)) {
		t.Fatal("later independent call differs")
	}
}

func TestLineageOperationSessionCompleteOracle(t *testing.T) {
	assertLineageOperationSession(t, historicalSessionFixture(t))
	assertLineageOperationSession(t, RuntimeLineageAssertionSources{})
}

func TestLineageOperationSessionReadOnlyArchiveOracle(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		s := historicalSessionArchiveSources(run.RuntimeState)
		if err := runtimeValidateLineageOperations(s); err != nil {
			t.Fatal("original complete archive gate rejected", err)
		}
		assertLineageOperationSession(t, s)
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func FuzzLineageOperationSessionCompleteOracle(f *testing.F) {
	for arm := uint8(0); arm < 12; arm++ {
		f.Add(arm, "原文\x00")
	}
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		s := historicalSessionFixture(t)
		state := runtimeLineageOperationSourceRecord(s).Clone()
		mutateLineageOperationSource(&state, arm, raw)
		assertLineageOperationSession(t, historicalSessionArchiveSources(state))
	})
}
