package workorchmanagementcontract

import "testing"

func TestPreparationRegistrationViewKeepsOutcomeAndBudgetConsistent(t *testing.T) {
	base := PreparationResourceInitializationView{RequestID: "original", ExpectedWorkspaceRevision: 1, CommittedWorkspaceRevision: 4, State: "blocked", Attempts: 1, RegistrationState: "unresolved", RegistrationAttempts: 1}
	if err := base.Validate(4); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*PreparationResourceInitializationView){
		"unknown_state":        func(v *PreparationResourceInitializationView) { v.RegistrationState = "ready" },
		"missing_attempt":      func(v *PreparationResourceInitializationView) { v.RegistrationAttempts = 0 },
		"exhausted_plus_one":   func(v *PreparationResourceInitializationView) { v.RegistrationAttempts = 5 },
		"unresolved_completed": func(v *PreparationResourceInitializationView) { v.State = "completed" },
		"success_blocked":      func(v *PreparationResourceInitializationView) { v.RegistrationState = "succeeded" },
		"pending_blocked":      func(v *PreparationResourceInitializationView) { v.RegistrationState = "pending" },
	} {
		t.Run(name, func(t *testing.T) {
			v := base
			change(&v)
			if v.Validate(4) == nil {
				t.Fatal("accepted inconsistent public continuation")
			}
		})
	}
}
