package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func compareWorkStateCompilerSession(t *testing.T, r RuntimeStateRecord) {
	t.Helper()
	before, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	got := ValidateRuntimeImplementationWorkState(r.Definition, r.Facts, r.ImplementationWork, r.SemanticPlanning, r.ActiveDispatch, r.CompletedDispatches, r.OperationReceipts, r.OperationGovernanceBindings, r.ExecutionBudget)
	want := referenceWorkStateCompilerSession(r.Definition, r.Facts, r.ImplementationWork, r.SemanticPlanning, r.ActiveDispatch, r.CompletedDispatches, r.OperationReceipts, r.OperationGovernanceBindings, r.ExecutionBudget)
	if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
		t.Fatalf("work state first error differs: %v != %v", got, want)
	}
	after, err := json.Marshal(r)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("work state compiler changed raw source")
	}
	var loaded RuntimeStateRecord
	if err := json.Unmarshal(before, &loaded); err != nil {
		t.Fatal(err)
	}
	reloaded := ValidateRuntimeImplementationWorkState(loaded.Definition, loaded.Facts, loaded.ImplementationWork, loaded.SemanticPlanning, loaded.ActiveDispatch, loaded.CompletedDispatches, loaded.OperationReceipts, loaded.OperationGovernanceBindings, loaded.ExecutionBudget)
	if cwiFocusBindingsError(got) != cwiFocusBindingsError(reloaded) {
		t.Fatal("work state rejection changed after reload")
	}
}

func TestWorkStateCompilerSessionEmptyAndRejectingSource(t *testing.T) {
	compareWorkStateCompilerSession(t, RuntimeStateRecord{})
	in := workCompilerSessionFixture(t)
	facts := RuntimeFacts{WorkspaceID: in.Sources.WorkspaceID, RunID: in.Sources.RunID, DefinitionSeed: in.Sources.Seed, DefinitionFocuses: in.Sources.Focuses}
	facts.Activation.ExecutionContract = in.Execution
	r := RuntimeStateRecord{Definition: in.Definition, Facts: facts}
	r.ImplementationWork.PurposeChoices = []RuntimeImplementationWorkPurposeChoice{{}}
	compareWorkStateCompilerSession(t, r)
}

// Read-only, explicit archived public input; never admits, drives or writes a Run.
func TestWorkStateCompilerSessionExplicitArchiveEquivalence(t *testing.T) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		t.Skip("explicit local archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState RuntimeStateRecord }
	if err := json.Unmarshal(body, &runs); err != nil {
		t.Fatal(err)
	}
	for _, run := range runs {
		base := run.RuntimeState
		if len(base.ImplementationWork.PurposeChoices) == 0 {
			t.Fatal("archive has no implementation purpose history")
		}
		if err := ValidateRuntimeImplementationWorkState(base.Definition, base.Facts, base.ImplementationWork, base.SemanticPlanning, base.ActiveDispatch, base.CompletedDispatches, base.OperationReceipts, base.OperationGovernanceBindings, base.ExecutionBudget); err != nil {
			t.Fatal(err)
		}
		compareWorkStateCompilerSession(t, base)
		for _, mutate := range []func(*RuntimeStateRecord){
			func(r *RuntimeStateRecord) { r.Definition.StartupInputHash = "drift" },
			func(r *RuntimeStateRecord) { r.ImplementationWork.PurposeChoices[0].DocumentHash = "drift" },
			func(r *RuntimeStateRecord) { r.Facts.Activation.ExecutionContract.OperationContractRevision = "drift" },
			func(r *RuntimeStateRecord) {
				r.ImplementationWork.PurposeChoices = append(r.ImplementationWork.PurposeChoices, r.ImplementationWork.PurposeChoices[0])
			},
			func(r *RuntimeStateRecord) {
				for i := range r.SemanticPlanning.History {
					m := r.SemanticPlanning.History[i].Snapshot.Manifest
					if m != nil && len(m.ProofObligations) == 1 && m.ProofObligations[0].ImplementationWork != nil {
						m.ProofObligations[0].ImplementationWork.MutationInputCatalogs[0].SnapshotID = "same-hash-raw-drift"
						return
					}
				}
				t.Fatal("archive missing work manifest")
			},
		} {
			changed := base.Clone()
			mutate(&changed)
			compareWorkStateCompilerSession(t, changed)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("read-only test changed archive")
	}
}
