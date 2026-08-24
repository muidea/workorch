package workorchruntimecontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRuntimePersistenceRejectsRetiredAndUnknownFields(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		target  any
	}{
		{name: "state schema version", payload: `{"SchemaVersion":2}`, target: &RuntimeStateRecord{}},
		{name: "state unknown", payload: `{"UnknownRuntimeField":true}`, target: &RuntimeStateRecord{}},
		{name: "bootstrap schema version", payload: `{"SchemaVersion":2}`, target: &RuntimeBootstrap{}},
		{name: "bootstrap unknown", payload: `{"UnknownBootstrapField":true}`, target: &RuntimeBootstrap{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := json.Unmarshal([]byte(tc.payload), tc.target)
			if err == nil || !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("decode err = %v, want strict unknown-field rejection", err)
			}
		})
	}
}

func TestRuntimePersistenceCurrentShapeRoundTripsWithoutVersionField(t *testing.T) {
	for _, value := range []any{RuntimeStateRecord{}, RuntimeBootstrap{}} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var topLevel map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &topLevel); err != nil {
			t.Fatalf("decode current persistence object: %v", err)
		}
		if _, found := topLevel["SchemaVersion"]; found {
			t.Fatalf("current persistence contains top-level version field: %s", encoded)
		}
		switch value.(type) {
		case RuntimeStateRecord:
			var decoded RuntimeStateRecord
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("decode current runtime state: %v", err)
			}
		case RuntimeBootstrap:
			var decoded RuntimeBootstrap
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("decode current runtime bootstrap: %v", err)
			}
		}
	}
}

func TestRuntimePolicyNormalizationDoesNotBackfillMissingPersistedFields(t *testing.T) {
	execution := (RuntimeExecutionContract{}).Normalized()
	if execution.StateChangePolicy != "" {
		t.Fatalf("execution state-change policy was backfilled: %+v", execution)
	}
	if err := (RuntimeExecutionContract{}).Validate(); err == nil {
		t.Fatal("missing execution state-change policy was accepted")
	}

	slice := (RuntimeAcceptanceSlicePolicy{
		SchemaVersion: RuntimeAcceptanceSlicePolicySchemaCurrent,
		MaxWorkItems:  1, MaxFocuses: 1, MaxCandidateContextItems: 1,
	}).Normalized()
	if slice.CompletionPolicy != "" || slice.ResultPolicy != "" {
		t.Fatalf("acceptance slice policy was backfilled: %+v", slice)
	}
	if err := slice.Validate(); err == nil {
		t.Fatal("acceptance slice policy with missing current fields was accepted")
	}

	capability := normalizeRuntimeMCPCapabilities([]RuntimeMCPCapabilityContext{{
		ID: "capability-1", ServerID: "server-1", Kind: "tool", Name: "tool-1",
	}})
	if len(capability) != 1 || capability[0].OperationKind != "" {
		t.Fatalf("MCP operation kind was derived from a persisted registry kind: %+v", capability)
	}

	if got := (RuntimeConvergencePolicy{}).Normalized(); got != (RuntimeConvergencePolicy{}) {
		t.Fatalf("zero convergence policy was backfilled: %+v", got)
	}
	if err := (RuntimeConvergencePolicy{}).Validate(); err == nil {
		t.Fatal("missing convergence policy was accepted")
	}
	if got := (RuntimeRecoveryPolicy{}).Normalized(); got != (RuntimeRecoveryPolicy{}) {
		t.Fatalf("zero recovery policy was backfilled: %+v", got)
	}
	if err := (RuntimeRecoveryPolicy{}).Validate(); err == nil {
		t.Fatal("missing recovery policy was accepted")
	}

	bootstrap := RuntimeBootstrap{
		ConvergencePolicy: DefaultRuntimeConvergencePolicy(),
		RecoveryPolicy:    DefaultRuntimeRecoveryPolicy(),
	}.Normalized()
	if bootstrap.ExecutionBudgetPolicy.PolicyRevision != "" {
		t.Fatalf("bootstrap execution budget policy was derived during normalization: %+v", bootstrap.ExecutionBudgetPolicy)
	}
	facts := RuntimeFacts{
		ConvergencePolicy: DefaultRuntimeConvergencePolicy(),
		RecoveryPolicy:    DefaultRuntimeRecoveryPolicy(),
	}.Normalized()
	if facts.ExecutionBudgetPolicy.PolicyRevision != "" {
		t.Fatalf("runtime facts execution budget policy was derived during normalization: %+v", facts.ExecutionBudgetPolicy)
	}
}

func TestRuntimeStateSerializationContainsCanonicalDecisionOnly(t *testing.T) {
	record := RuntimeStateRecord{
		Decision: RuntimeDecision{
			State: ConvergenceStateNeedStateChange,
			RequiredOutput: RequiredOutputContract{
				AcceptedKinds:        []string{RuntimeBlockKindPatchApply},
				AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
				RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			},
		},
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal runtime state: %v", err)
	}
	var persisted map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		t.Fatalf("decode runtime state json: %v", err)
	}
	decision := runtimeStateJSONField(t, persisted, "Decision")
	if _, found := decision["Action"]; found {
		t.Fatalf("persisted decision retains duplicate Action authority: %s", encoded)
	}
	if _, found := decision["TerminalOutcome"]; found {
		t.Fatalf("persisted decision retains delivery-only terminal outcome: %s", encoded)
	}
	requiredOutput := runtimeStateJSONField(t, decision, "RequiredOutput")
	if _, found := requiredOutput["Kind"]; found {
		t.Fatalf("persisted required output retains duplicate Kind authority: %s", encoded)
	}
}

func TestRuntimeStateTerminalDeliveryLedgerRejectsNonTerminalPollution(t *testing.T) {
	decision := RuntimeDecision{State: ConvergenceStateNeedGovernance}
	if err := ValidateRuntimeStateTerminalDeliveryLedger(decision, nil, nil, RuntimeRecoveryLedger{}); err != nil {
		t.Fatalf("clean non-terminal ledger rejected: %v", err)
	}
	err := ValidateRuntimeStateTerminalDeliveryLedger(decision, nil, nil, RuntimeRecoveryLedger{TerminalDeliveryAttempts: 1})
	if err == nil {
		t.Fatal("non-terminal terminal-delivery consumption was accepted")
	}
	ack := RuntimeDeliveryAcknowledgement{}
	if err := ValidateRuntimeStateTerminalDeliveryLedger(decision, nil, []RuntimeDeliveryAcknowledgement{ack}, RuntimeRecoveryLedger{}); err == nil {
		t.Fatal("non-terminal terminal-delivery acknowledgement was accepted")
	}
}

func TestRuntimeStateTerminalDeliveryLedgerAllowsSealedTerminalRetry(t *testing.T) {
	decision := RuntimeDecision{
		State:    ConvergenceStateFailed,
		Terminal: &RuntimeTerminalDecision{Code: "terminal", Summary: "sealed"},
	}
	if err := ValidateRuntimeStateTerminalDeliveryLedger(decision, nil, nil, RuntimeRecoveryLedger{TerminalDeliveryAttempts: 1}); err != nil {
		t.Fatalf("sealed terminal retry rejected: %v", err)
	}
	termination := &RuntimeTerminationRecord{Kind: RuntimeTerminationSuperseded, RequestedBy: "blocks/run", Reason: "replacement", Revision: 2, RequestedAt: "2026-08-23T00:00:00Z"}
	if err := ValidateRuntimeStateTerminalDeliveryLedger(RuntimeDecision{State: ConvergenceStateNeedGovernance}, termination, nil, RuntimeRecoveryLedger{TerminalDeliveryAttempts: 1}); err != nil {
		t.Fatalf("administrative terminal retry rejected: %v", err)
	}
}

func runtimeStateJSONField(t *testing.T, object map[string]json.RawMessage, name string) map[string]json.RawMessage {
	t.Helper()
	raw, found := object[name]
	if !found {
		t.Fatalf("serialized runtime state missing %s", name)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return value
}
