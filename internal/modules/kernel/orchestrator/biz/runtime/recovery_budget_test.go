package runtime

import (
	"context"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeControllerPersistsIndependentRecoveryBudget(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	recoveryPolicy := runtimecontract.DefaultRuntimeRecoveryPolicy()
	recoveryPolicy.MaxProviderTransportAttempts = 1
	recoveryPolicy.MaxOperationRecoveryAttempts = 2
	recoveryPolicy.MaxCASReloadAttempts = 3
	recoveryPolicy.MaxTerminalDeliveryAttempts = 4
	facts.RecoveryPolicy = recoveryPolicy
	facts.ExecutionBudgetPolicy = runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(facts.ConvergencePolicy, facts.RecoveryPolicy)
	if _, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	first, consumed, err := controller.consumeRecoveryBudget(context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetCASReload)
	if err != nil || !consumed || first.Facts.Recovery.CASReloadAttempts != 1 {
		t.Fatalf("CAS budget = %+v consumed=%v err=%v", first.Facts.Recovery, consumed, err)
	}
	second, consumed, err := controller.consumeRecoveryBudget(context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetOperationRecovery)
	if err != nil || !consumed || second.Facts.Recovery.OperationRecoveryAttempts != 1 || second.Facts.Recovery.CASReloadAttempts != 1 {
		t.Fatalf("operation budget = %+v consumed=%v err=%v", second.Facts.Recovery, consumed, err)
	}
	scheduled, consumed, err := controller.consumeRecoveryBudget(context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetDriveSchedule)
	if err != nil || !consumed || scheduled.Facts.Recovery.DriveScheduleAttempts != 1 || scheduled.Facts.Recovery.CASReloadAttempts != 1 {
		t.Fatalf("schedule budget = %+v consumed=%v err=%v", scheduled.Facts.Recovery, consumed, err)
	}
}

func TestReduceRuntimeStateRoutesExhaustedProviderTransportToTerminal(t *testing.T) {
	facts := controllerFacts()
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
	recoveryPolicy := runtimecontract.DefaultRuntimeRecoveryPolicy()
	recoveryPolicy.MaxProviderTransportAttempts = 2
	facts.RecoveryPolicy = recoveryPolicy
	facts.ExecutionBudgetPolicy = runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(facts.ConvergencePolicy, facts.RecoveryPolicy)
	facts.Recovery.ProviderTransportAttempts = 2
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassProviderTransient, Code: "provider_transport_exhausted",
		Signature: "provider-transport-exhausted", Message: "provider remains unavailable",
	}
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != "provider_transport_exhausted" {
		t.Fatalf("decision = %+v, want provider_transport_exhausted terminal", decision)
	}
}
