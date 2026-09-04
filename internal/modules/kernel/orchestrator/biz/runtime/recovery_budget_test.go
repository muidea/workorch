package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeControllerRejectsSplitCASRecoveryBudget(t *testing.T) {
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
	initialSaves := repository.saves
	first, consumed, err := controller.consumeRecoveryBudget(context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetCASReload)
	if !errors.Is(err, common.ErrInvalidArgument) || consumed || repository.saves != initialSaves || first.StateRevision != 0 {
		t.Fatalf("split CAS budget crossed the atomic recovery boundary: aggregate=%+v consumed=%v saves=%d/%d err=%v", first, consumed, repository.saves, initialSaves, err)
	}
	scheduled, consumed, err := controller.consumeRecoveryBudget(context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetDriveSchedule)
	if err != nil || !consumed || scheduled.Facts.Recovery.DriveScheduleAttempts != 1 || scheduled.Facts.Recovery.CASReloadAttempts != 0 {
		t.Fatalf("schedule budget = %+v consumed=%v err=%v", scheduled.Facts.Recovery, consumed, err)
	}
}

func TestRuntimeOperationRecoveryInFlightClaimBlocksNextAuthorizationUntilCallReturns(t *testing.T) {
	repository := &controllerRepository{}
	controller, _, operationID := runtimeSealReplayProofOperation(t, repository)
	if _, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", operationID); err != nil {
		t.Fatal(err)
	}
	first, consumed, err := controller.consumeRecoveryBudget(context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetOperationRecovery, operationID)
	if err != nil || !consumed || len(first.OperationRecoveryAuthorizations) != 1 {
		t.Fatalf("first operation recovery authorization aggregate=%+v consumed=%v err=%v", first, consumed, err)
	}
	firstAuthorization := first.OperationRecoveryAuthorizations[0]
	claimedAt := time.Now().UTC()
	claimed, claimID, changed, err := controller.driveDelivery.claimOperationRecoveryAuthorization(context.Background(), "ws-v2", "run-v2", operationID, firstAuthorization.AuthorizationID, claimedAt.Format(time.RFC3339Nano))
	if err != nil || !changed || claimID == "" || claimed.OperationRecoveryAuthorizations[0].State != runtimecontract.RuntimeOperationRecoveryAuthorizationInFlight {
		t.Fatalf("claim first recovery authorization aggregate=%+v claim=%q changed=%v err=%v", claimed, claimID, changed, err)
	}
	saves := repository.saves
	duplicate, duplicateClaimID, changed, err := controller.driveDelivery.claimOperationRecoveryAuthorization(context.Background(), "ws-v2", "run-v2", operationID, firstAuthorization.AuthorizationID, claimedAt.Format(time.RFC3339Nano))
	if err != nil || changed || duplicateClaimID != "" || duplicate.StateRevision != claimed.StateRevision || repository.saves != saves {
		t.Fatalf("duplicate recovery claim mutated state revision=%d/%d saves=%d/%d changed=%v err=%v", duplicate.StateRevision, claimed.StateRevision, repository.saves, saves, changed, err)
	}
	_, consumed, err = controller.consumeRecoveryBudget(context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetOperationRecovery, operationID)
	var inFlight *runtimeOperationRecoveryInFlightError
	if !errors.As(err, &inFlight) || consumed || repository.saves != saves {
		t.Fatalf("active recovery claim admitted another grant: consumed=%v saves=%d/%d err=%v", consumed, repository.saves, saves, err)
	}
	uncertain, changed, err := controller.driveDelivery.completeOperationRecoveryCall(context.Background(), "ws-v2", "run-v2", operationID, firstAuthorization.AuthorizationID, claimID)
	if err != nil || !changed || uncertain.OperationRecoveryAuthorizations[0].State != runtimecontract.RuntimeOperationRecoveryAuthorizationUncertain {
		t.Fatalf("complete recovery call without receipt: aggregate=%+v changed=%v err=%v", uncertain, changed, err)
	}
	second, consumed, err := controller.consumeRecoveryBudget(context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetOperationRecovery, operationID)
	if err != nil || !consumed || len(second.OperationRecoveryAuthorizations) != 2 ||
		second.OperationRecoveryAuthorizations[1].RecoveryAttempt != 2 || second.OperationRecoveryAuthorizations[1].State != runtimecontract.RuntimeOperationRecoveryAuthorizationIssued ||
		second.OperationRecoveryAuthorizations[1].IssuedAtStateRevision != second.StateRevision {
		t.Fatalf("second operation recovery authorization aggregate=%+v consumed=%v err=%v", second, consumed, err)
	}
}

func TestRuntimeOperationRecoveryPreCallCrashRequiresLeaseExpiryBeforeNewGrant(t *testing.T) {
	repository := &controllerRepository{}
	controller, _, operationID := runtimeSealReplayProofOperation(t, repository)
	if _, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", operationID); err != nil {
		t.Fatal(err)
	}
	issuedAt := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	first, consumed, err := controller.driveDelivery.consumeRecoveryBudget(
		context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetOperationRecovery, operationID, issuedAt.Format(time.RFC3339Nano),
	)
	if err != nil || !consumed || len(first.OperationRecoveryAuthorizations) != 1 {
		t.Fatalf("initial recovery grant aggregate=%+v consumed=%v err=%v", first, consumed, err)
	}
	authorization := first.OperationRecoveryAuthorizations[0]
	claimed, claimID, changed, err := controller.driveDelivery.claimOperationRecoveryAuthorization(
		context.Background(), "ws-v2", "run-v2", operationID, authorization.AuthorizationID, issuedAt.Format(time.RFC3339Nano),
	)
	if err != nil || !changed || claimID == "" {
		t.Fatalf("pre-call claim aggregate=%+v claim=%q changed=%v err=%v", claimed, claimID, changed, err)
	}
	deadline, err := time.Parse(time.RFC3339Nano, claimed.OperationRecoveryAuthorizations[0].LeaseDeadline)
	if err != nil {
		t.Fatal(err)
	}

	// Reconstruct a fresh controller while the CAS winner is presumed to have
	// crashed before the physical call. Persisted in-flight state is a fence,
	// not replay authority for the new process.
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	saves := repository.saves
	_, consumed, err = restarted.driveDelivery.consumeRecoveryBudget(
		context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetOperationRecovery, operationID,
		deadline.Add(-time.Nanosecond).Format(time.RFC3339Nano),
	)
	var inFlight *runtimeOperationRecoveryInFlightError
	if !errors.As(err, &inFlight) || consumed || repository.saves != saves {
		t.Fatalf("fresh process bypassed active claim consumed=%v saves=%d/%d err=%v", consumed, repository.saves, saves, err)
	}
	execution, found := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, operationID)
	if !found {
		t.Fatal("claimed recovery operation disappeared")
	}
	if _, authorized := runtimecontract.RuntimeInFlightOperationRecoveryAuthorizationForExecution(
		claimed.OperationRecoveryAuthorizations, claimID, deadline.Add(time.Nanosecond).Format(time.RFC3339Nano), claimed.ActiveDispatch.ID, execution,
	); authorized {
		t.Fatal("expired pre-call claim authorized a physical replay")
	}

	reissued, consumed, err := restarted.driveDelivery.consumeRecoveryBudget(
		context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetOperationRecovery, operationID,
		deadline.Add(time.Nanosecond).Format(time.RFC3339Nano),
	)
	if err != nil || !consumed || len(reissued.OperationRecoveryAuthorizations) != 2 ||
		reissued.OperationRecoveryAuthorizations[0].State != runtimecontract.RuntimeOperationRecoveryAuthorizationUncertain ||
		reissued.OperationRecoveryAuthorizations[1].State != runtimecontract.RuntimeOperationRecoveryAuthorizationIssued ||
		reissued.OperationRecoveryAuthorizations[1].RecoveryAttempt != 2 || repository.saves != saves+1 {
		t.Fatalf("expired pre-call claim did not atomically become uncertain before reissue: aggregate=%+v consumed=%v saves=%d/%d err=%v", reissued, consumed, repository.saves, saves+1, err)
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
