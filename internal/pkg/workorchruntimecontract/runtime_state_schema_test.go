package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestRuntimeStateSerializationContainsCanonicalDecisionOnly(t *testing.T) {
	record := RuntimeStateRecord{
		SchemaVersion: RuntimeStateSchemaVersionCurrent,
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

func TestRuntimeStateJSONNonCurrentSchemaFailsWithTypedError(t *testing.T) {
	var record RuntimeStateRecord
	if err := json.Unmarshal([]byte(`{"SchemaVersion":8}`), &record); err != nil {
		t.Fatalf("decode non-current runtime state: %v", err)
	}
	err := record.Validate()
	if !errors.Is(err, ErrUnsupportedRuntimeStateSchema) {
		t.Fatalf("non-current schema error = %v, want typed unsupported-schema error", err)
	}
	var schemaErr *RuntimeStateSchemaVersionError
	if !errors.As(err, &schemaErr) || schemaErr.Actual != 8 || schemaErr.Supported != RuntimeStateSchemaVersionCurrent {
		t.Fatalf("non-current schema typed detail = %+v", schemaErr)
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
	typed := NewRuntimeStateRecoveryLedgerIncompatibilityError(err)
	value, ok := RuntimeStateIncompatibilityFromError(typed)
	if !ok || value.Reason != RuntimeStateIncompatibilityReasonRecoveryLedgerInvalid ||
		value.ActualSchemaVersion != RuntimeStateSchemaVersionCurrent || value.SupportedSchemaVersion != RuntimeStateSchemaVersionCurrent {
		t.Fatalf("recovery-ledger incompatibility = %+v, ok=%t", value, ok)
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
