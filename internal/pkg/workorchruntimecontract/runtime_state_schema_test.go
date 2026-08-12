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
	if err := json.Unmarshal([]byte(`{"SchemaVersion":1}`), &record); err != nil {
		t.Fatalf("decode non-current runtime state: %v", err)
	}
	err := record.Validate()
	if !errors.Is(err, ErrUnsupportedRuntimeStateSchema) {
		t.Fatalf("non-current schema error = %v, want typed unsupported-schema error", err)
	}
	var schemaErr *RuntimeStateSchemaVersionError
	if !errors.As(err, &schemaErr) || schemaErr.Actual != 1 || schemaErr.Supported != RuntimeStateSchemaVersionCurrent {
		t.Fatalf("non-current schema typed detail = %+v", schemaErr)
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
