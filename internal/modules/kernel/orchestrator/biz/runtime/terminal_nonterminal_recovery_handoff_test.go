package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeTerminalNonterminalRecoveryFixture(t *testing.T, claim bool) RuntimeAggregate {
	t.Helper()
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	cause := &RuntimeCASConflictError{
		WorkspaceID:      initialized.Facts.WorkspaceID,
		RunID:            initialized.Facts.RunID,
		ExpectedRevision: initialized.StateRevision,
		ObservedRevision: initialized.StateRevision + 1,
	}
	request, err := NewRuntimeNonTerminalRecoveryRequest(
		initialized.Facts.WorkspaceID,
		initialized.Facts.RunID,
		0,
		initialized.StateRevision,
		"2026-09-04T12:00:00Z",
		cause,
	)
	if err != nil {
		t.Fatal(err)
	}
	scheduled, err := controller.ApplyInternalEvent(ctx, runtimeNonTerminalRecoveryEvent(request, cause))
	if err != nil || scheduled.DriveLease == nil {
		t.Fatalf("schedule exact nonterminal recovery: result=%+v err=%v", scheduled, err)
	}
	aggregate := scheduled.Aggregate
	if claim {
		claimed, running, changed, claimErr := controller.claimDriveLease(
			ctx,
			initialized.Facts.WorkspaceID,
			initialized.Facts.RunID,
			*scheduled.DriveLease,
			scheduled.DriveLease.DueAt,
		)
		if claimErr != nil || !changed || running.State != runtimecontract.RuntimeDriveLeaseRunning {
			t.Fatalf("claim exact nonterminal recovery: lease=%+v changed=%v err=%v", running, changed, claimErr)
		}
		aggregate = claimed
	}

	// Model the ordinary SM-2 convergence CAS reached by the worker holding
	// this exact SM-4 authority. The lease is deliberately left untouched until
	// the owner-exit settlement performs first terminal delivery.
	aggregate.Facts.Failure = &runtimecontract.FailureFact{
		Class:     runtimecontract.FailureClassRuntimeContractMisalignment,
		Code:      "runtime_contract_defect",
		Signature: "terminal-running-nonterminal-recovery",
		Message:   "sealed contract mismatch",
	}
	decision, err := ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil {
		t.Fatalf("fixture did not converge terminal: %+v", decision)
	}
	aggregate.Decision = decision
	aggregate.StateRevision++
	return aggregate
}

func runtimeStateRecordForTerminalNonterminalRecoveryTest(t *testing.T, aggregate RuntimeAggregate) runtimecontract.RuntimeStateRecord {
	t.Helper()
	payload, err := json.Marshal(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	var record runtimecontract.RuntimeStateRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestRuntimeAggregateAllowsOnlyExactRunningNonterminalRecoveryAtConvergenceTerminal(t *testing.T) {
	valid := runtimeTerminalNonterminalRecoveryFixture(t, true)
	if valid.DriveLease == nil || valid.DriveLease.State != runtimecontract.RuntimeDriveLeaseRunning {
		t.Fatalf("fixture has no running lease: %+v", valid.DriveLease)
	}
	if err := runtimecontract.ValidateRuntimeNonTerminalRecoveryLeaseBinding(valid.Facts.Recovery, *valid.DriveLease); err != nil {
		t.Fatalf("fixture lease is not exactly request-bound: %v", err)
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("exact running handoff rejected: %v", err)
	}
	if err := runtimeStateRecordForTerminalNonterminalRecoveryTest(t, valid).Validate(); err != nil {
		t.Fatalf("persisted exact running handoff rejected: %v", err)
	}

	tests := []struct {
		name   string
		build  func(*testing.T) RuntimeAggregate
		mutate func(*RuntimeAggregate)
		want   string
	}{
		{
			name: "scheduled exact lease",
			build: func(t *testing.T) RuntimeAggregate {
				value := runtimeTerminalNonterminalRecoveryFixture(t, false)
				if err := runtimecontract.ValidateRuntimeNonTerminalRecoveryLeaseBinding(value.Facts.Recovery, *value.DriveLease); err != nil {
					t.Fatalf("scheduled tamper fixture lost exact binding: %v", err)
				}
				return value
			},
			want: "outside an exact running convergence handoff",
		},
		{
			name:  "unbound running lease",
			build: func(t *testing.T) RuntimeAggregate { return runtimeTerminalNonterminalRecoveryFixture(t, true) },
			mutate: func(value *RuntimeAggregate) {
				value.DriveLease.RecoveryRequestID = ""
			},
			want: "requires recovery request",
		},
		{
			name:  "wrong recovery request",
			build: func(t *testing.T) RuntimeAggregate { return runtimeTerminalNonterminalRecoveryFixture(t, true) },
			mutate: func(value *RuntimeAggregate) {
				value.DriveLease.RecoveryRequestID = "runtime-nonterminal-recovery-" + strings.Repeat("0", 64)
			},
			want: "no matching persisted record",
		},
		{
			name: "administrative termination",
			build: func(t *testing.T) RuntimeAggregate {
				ctx := context.Background()
				repository := &controllerRepository{}
				controller, err := NewRuntimeController(repository)
				if err != nil {
					t.Fatal(err)
				}
				initialized, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
				if err != nil {
					t.Fatal(err)
				}
				cause := &RuntimeCASConflictError{
					WorkspaceID: initialized.Facts.WorkspaceID, RunID: initialized.Facts.RunID,
					ExpectedRevision: initialized.StateRevision, ObservedRevision: initialized.StateRevision + 1,
				}
				request, err := NewRuntimeNonTerminalRecoveryRequest(
					initialized.Facts.WorkspaceID, initialized.Facts.RunID, 0, initialized.StateRevision,
					"2026-09-04T12:00:00Z", cause,
				)
				if err != nil {
					t.Fatal(err)
				}
				scheduled, err := controller.ApplyInternalEvent(ctx, runtimeNonTerminalRecoveryEvent(request, cause))
				if err != nil || scheduled.DriveLease == nil {
					t.Fatalf("schedule termination fixture: result=%+v err=%v", scheduled, err)
				}
				if _, _, claimed, err := controller.claimDriveLease(
					ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, *scheduled.DriveLease, scheduled.DriveLease.DueAt,
				); err != nil || !claimed {
					t.Fatalf("claim termination fixture: claimed=%v err=%v", claimed, err)
				}
				terminated, err := controller.requestTermination(
					ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID,
					runtimecontract.RuntimeTerminationCancelled, "blocks/run", "operator cancelled the active run",
				)
				if err != nil {
					t.Fatal(err)
				}
				retired := terminated.Facts.Recovery.RetiredTerminalDeliveryLease.Clone()
				if retired.Validate() != nil || retired.State != runtimecontract.RuntimeDriveLeaseRunning || retired.RecoveryRequestID != request.RequestID {
					t.Fatalf("termination fixture lost exact retired lease: %+v", retired)
				}
				terminated.DriveLease = &retired
				return terminated
			},
			want: "outside an exact running convergence handoff",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := test.build(t)
			if test.mutate != nil {
				test.mutate(&value)
			}
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("aggregate tamper error = %v, want containing %q", err, test.want)
			}
			if err := runtimeStateRecordForTerminalNonterminalRecoveryTest(t, value).Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("persisted-state tamper error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestRuntimeTerminalNonterminalRecoveryHandoffDoesNotConsumeAnotherRecoveryBudget(t *testing.T) {
	value := runtimeTerminalNonterminalRecoveryFixture(t, true)
	beforeCAS := value.Facts.Recovery.CASReloadAttempts
	beforeTerminal := value.Facts.Recovery.TerminalDeliveryAttempts
	if beforeCAS != 1 || beforeTerminal != 0 {
		t.Fatalf("unexpected fixture budgets: %+v", value.Facts.Recovery)
	}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	if value.Facts.Recovery.CASReloadAttempts != beforeCAS || value.Facts.Recovery.TerminalDeliveryAttempts != beforeTerminal {
		t.Fatalf("invariant validation mutated recovery budgets: %+v", value.Facts.Recovery)
	}
	if value.Termination != nil {
		t.Fatalf("ordinary convergence handoff acquired administrative terminal: %+v", value.Termination)
	}
	if value.Decision.State != runtimecontract.ConvergenceStateFailed {
		t.Fatalf("handoff lost convergence terminal: %+v", value.Decision)
	}
	if value.Facts.WorkspaceID == common.WorkspaceID("") || value.Facts.RunID == common.RunID("") {
		t.Fatal("handoff lost Runtime scope")
	}
}
