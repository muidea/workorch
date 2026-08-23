package runtime

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDeriveTerminalOutcomeMatrixFailsClosed(t *testing.T) {
	terminal := func(code, summary string) *runtimecontract.RuntimeTerminalDecision {
		return &runtimecontract.RuntimeTerminalDecision{Code: code, Summary: summary}
	}
	termination := func(kind runtimecontract.RuntimeTerminationKind, reason string) *runtimecontract.RuntimeTerminationRecord {
		return &runtimecontract.RuntimeTerminationRecord{Kind: kind, RequestedBy: "application/workorchd", Reason: reason, Revision: 2, RequestedAt: "2026-08-23T00:00:00Z"}
	}
	tests := []struct {
		name         string
		decision     runtimecontract.RuntimeDecision
		termination  *runtimecontract.RuntimeTerminationRecord
		wantKind     runtimecontract.TerminalOutcomeKind
		wantCode     string
		wantTerminal bool
		wantErr      bool
	}{
		{name: "active", decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange}},
		{name: "completed", decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateComplete, Terminal: terminal("coverage_satisfied", "coverage satisfied")}, wantKind: runtimecontract.TerminalOutcomeKindCompleted, wantCode: "coverage_satisfied", wantTerminal: true},
		{name: "failed", decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateFailed, Terminal: terminal("provider_protocol_failure", "provider protocol failed")}, wantKind: runtimecontract.TerminalOutcomeKindFailed, wantCode: "provider_protocol_failure", wantTerminal: true},
		{name: "cancelled", decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange}, termination: termination(runtimecontract.RuntimeTerminationCancelled, "cancel requested"), wantKind: runtimecontract.TerminalOutcomeKindCancelled, wantCode: "administrative_cancelled", wantTerminal: true},
		{name: "superseded", decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange}, termination: termination(runtimecontract.RuntimeTerminationSuperseded, "replacement requested"), wantKind: runtimecontract.TerminalOutcomeKindSuperseded, wantCode: "administrative_superseded", wantTerminal: true},
		{name: "nonterminal_with_detail", decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange, Terminal: terminal("illegal", "illegal")}, wantErr: true},
		{name: "terminal_without_detail", decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateFailed}, wantErr: true},
		{name: "dual_terminal", decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateFailed, Terminal: terminal("failed", "failed")}, termination: termination(runtimecontract.RuntimeTerminationCancelled, "cancel requested"), wantErr: true},
		{name: "invalid_admin", decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange}, termination: termination("unknown", "invalid"), wantErr: true},
		{name: "empty_terminal_summary", decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateComplete, Terminal: terminal("coverage_satisfied", "")}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outcome, terminal, err := DeriveTerminalOutcome(tc.decision, tc.termination)
			if (err != nil) != tc.wantErr {
				t.Fatalf("DeriveTerminalOutcome err = %v, wantErr=%t", err, tc.wantErr)
			}
			if tc.wantErr {
				if terminal || outcome != (runtimecontract.TerminalOutcome{}) {
					t.Fatalf("invalid terminal input returned authority: terminal=%t outcome=%+v", terminal, outcome)
				}
				return
			}
			if terminal != tc.wantTerminal || outcome.Kind != tc.wantKind || outcome.Code != tc.wantCode {
				t.Fatalf("terminal outcome = terminal:%t outcome:%+v", terminal, outcome)
			}
		})
	}
}
