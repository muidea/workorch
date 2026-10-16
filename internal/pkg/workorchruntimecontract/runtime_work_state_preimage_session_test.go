package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func workStatePreimageGate(r RuntimeStateRecord, session *runtimeSemanticValidationSession) error {
	return validateRuntimeImplementationWorkStateInSession(r.Definition, r.Facts, r.ImplementationWork, r.SemanticPlanning, r.ActiveDispatch, r.CompletedDispatches, r.OperationReceipts, r.OperationGovernanceBindings, r.ExecutionBudget, session)
}

func workStatePreimageOriginal(r RuntimeStateRecord) error {
	return referenceWorkStateCompilerSession(r.Definition, r.Facts, r.ImplementationWork, r.SemanticPlanning, r.ActiveDispatch, r.CompletedDispatches, r.OperationReceipts, r.OperationGovernanceBindings, r.ExecutionBudget)
}

func mutateWorkStatePreimage(r *RuntimeStateRecord, arm uint8, raw string) {
	switch arm % 14 {
	case 0:
		r.Definition.StartupInputHash = raw
	case 1:
		r.Facts.WorkspaceID = ""
	case 2:
		r.Facts.RunID = ""
	case 3:
		r.ImplementationWork.PurposeChoices = append(r.ImplementationWork.PurposeChoices, RuntimeImplementationWorkPurposeChoice{})
	case 4:
		r.ImplementationWork.Rounds = append(r.ImplementationWork.Rounds, RuntimeImplementationWorkRound{})
	case 5:
		r.SemanticPlanning.History = nil
	case 6:
		r.ActiveDispatch = &RuntimeDispatchRecord{}
	case 7:
		r.CompletedDispatches = nil
	case 8:
		r.OperationReceipts = nil
	case 9:
		r.OperationGovernanceBindings = append(r.OperationGovernanceBindings, RuntimeOperationGovernanceBinding{})
	case 10:
		r.ExecutionBudget.Policy.PolicyHash = raw
	case 11:
		if len(r.Facts.DefinitionFocuses) != 0 {
			r.Facts.DefinitionFocuses[0].AssessmentText = raw
		}
	case 12:
		r.Facts.Resources.Manifest = nil
	case 13:
		r.SemanticPlanning.DefinitionSeed.Hash = raw
	}
}

func assertWorkStatePreimageSession(t testing.TB, r RuntimeStateRecord) {
	t.Helper()
	saved := r.Clone()
	session := newRuntimeSemanticValidationSession()
	for range 2 {
		got, want := workStatePreimageGate(r, session), workStatePreimageOriginal(r)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("complete work state first error changed: %v != %v", got, want)
		}
		if want == nil && len(session.workStates) != 1 || want != nil && len(session.workStates) != 0 {
			t.Fatal("complete successful work preimage not reused, or failure registered")
		}
	}
	if !reflect.DeepEqual(r, saved) {
		t.Fatal("complete work gate mutated original raw input")
	}
	for arm := uint8(0); arm < 14; arm++ {
		changed := saved.Clone()
		mutateWorkStatePreimage(&changed, arm, "same hash\x00原文漂移")
		entries := len(session.workStates)
		got, want := workStatePreimageGate(changed, session), workStatePreimageOriginal(changed)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("mutation %d borrowed complete old work success: %v != %v", arm, got, want)
		}
		if want != nil && len(session.workStates) != entries {
			t.Fatal("rejected work source registered as successful")
		}
	}
	body, _ := json.Marshal(saved)
	var loaded RuntimeStateRecord
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(workStatePreimageGate(loaded, newRuntimeSemanticValidationSession())) != fmt.Sprint(workStatePreimageOriginal(loaded)) {
		t.Fatal("fresh JSON reload changed independent work gate")
	}
	if workStatePreimageOriginal(r) == nil && len(r.OperationReceipts) != 0 {
		prior := session.workStates[0].Clone()
		r.OperationReceipts[0].Fence++
		if fmt.Sprint(workStatePreimageGate(r, session)) != fmt.Sprint(workStatePreimageOriginal(r)) || !reflect.DeepEqual(prior, session.workStates[0]) {
			t.Fatal("caller alias borrowed or corrupted detached work source preimage")
		}
	}
}

func TestWorkStatePreimageSessionCompleteOracle(t *testing.T) {
	assertWorkStatePreimageSession(t, RuntimeStateRecord{})
	in := workCompilerSessionFixture(t)
	facts := RuntimeFacts{WorkspaceID: in.Sources.WorkspaceID, RunID: in.Sources.RunID, DefinitionSeed: in.Sources.Seed, DefinitionFocuses: in.Sources.Focuses}
	facts.Activation.ExecutionContract = in.Execution
	assertWorkStatePreimageSession(t, RuntimeStateRecord{Definition: in.Definition, Facts: facts})
}

func TestWorkStatePreimageSessionReadOnlyArchiveOracle(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		if err := workStatePreimageOriginal(run.RuntimeState); err != nil {
			t.Fatal("original complete archived work gate rejected", err)
		}
		assertWorkStatePreimageSession(t, run.RuntimeState)
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func FuzzWorkStatePreimageSessionCompleteOracle(f *testing.F) {
	for arm := uint8(0); arm < 14; arm++ {
		f.Add(arm, "原文\x00")
	}
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		r := RuntimeStateRecord{}
		mutateWorkStatePreimage(&r, arm, raw)
		assertWorkStatePreimageSession(t, r)
	})
}
