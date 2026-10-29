package workspace

import (
	"reflect"
	"testing"
	"time"
)

// The transition gate is tested separately from full archive compilation: a
// valid completion shape cannot bypass the original accepted CAS preimage.
func TestPreparationCompletionTransitionRequiresOriginalAcceptedCAS(t *testing.T) {
	at := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	a := WorkspacePreparationAttempt{RequestID: "original", PayloadHash: "payload", ExpectedWorkspaceRevision: 1, CommittedWorkspaceRevision: 2, State: WorkspacePreparationAccepted, CreationEpoch: "epoch"}
	b := a
	b.State = WorkspacePreparationBlocked
	b.CommittedWorkspaceRevision = 3
	b.CompletedAt = at
	b.NoRunEvaluationVersion = PreparationNoRunFinalVersion
	b.Assessment = &PreparationAssessment{Outcome: "checks_passed"}
	b.DefinitionCompilation = &PreparationDefinitionCompilation{}
	b.OwnerRevalidationVersion = PreparationOwnerRevalidationVersion
	c := WorkspacePreparationCompletion{Version: PreparationCompletionVersion, CompletionID: "completion", RequestID: b.RequestID, PayloadHash: b.PayloadHash, CreationEpoch: b.CreationEpoch, Binding: b.Binding, BootstrapHash: "bootstrap", CompletedAt: at, ExpiresAt: at.Add(30 * time.Second), CommittedWorkspaceRevision: 3}
	fixture := func() (*Workspace, *Workspace) {
		before := &Workspace{Revision: 2, StartupPreparation: &WorkspacePreparationRecord{Attempts: []WorkspacePreparationAttempt{a}}}
		after := &Workspace{Revision: 3, StartupPreparation: &WorkspacePreparationRecord{Attempts: []WorkspacePreparationAttempt{b}, Completions: []WorkspacePreparationCompletion{c}}}
		return before, after
	}
	before, after := fixture()
	if err := validatePreparationCompletionTransition(before, after, true); err != nil {
		t.Fatal("original CAS rejected", err)
	}
	for _, scenario := range []string{"unknown", "settled", "non_cas", "missing_preimage", "run", "revision", "fence", "replace_history"} {
		t.Run(scenario, func(t *testing.T) {
			before, after := fixture()
			cas := true
			switch scenario {
			case "unknown":
				before.StartupPreparation.Attempts[0].State = WorkspacePreparationUnknown
			case "settled":
				before.StartupPreparation.Attempts[0].State = WorkspacePreparationBlocked
			case "non_cas":
				cas = false
			case "missing_preimage":
				before = nil
			case "run":
				after.ActiveRunID = "run"
			case "revision":
				before.Revision++
			case "fence":
				after.PreparationFence = &WorkspacePreparationFence{}
			case "replace_history":
				before.StartupPreparation.Completions = []WorkspacePreparationCompletion{c}
				after.StartupPreparation.Completions[0].BootstrapHash = "changed"
			}
			if validatePreparationCompletionTransition(before, after, cas) == nil {
				t.Fatal("invalid completion transition accepted")
			}
		})
	}
	// A later state update preserves the original completion exactly.
	before, after = fixture()
	before.StartupPreparation.Completions = append([]WorkspacePreparationCompletion(nil), after.StartupPreparation.Completions...)
	if validatePreparationCompletionTransition(before, after, true) != nil || !reflect.DeepEqual(before.StartupPreparation.Completions, after.StartupPreparation.Completions) {
		t.Fatal("unchanged history rejected")
	}
}
