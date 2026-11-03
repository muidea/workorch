package runtimeprofiledecode

import (
	"encoding/json"
	"testing"
)

func TestSourceMetadataIsPreservedWithoutCurrentConfigurationPermission(t *testing.T) {
	work := map[string]any{"protocolRef": "runtime-implementation-work/v1", "inputRevisionProtocolRef": "validation/input-revision/v1", "maxInputRevisionsPerCWI": 4}
	input := map[string]any{"profileContracts": map[string]any{"p": map[string]any{"evidenceRequirements": []any{map[string]any{"id": "*", "scope": "dodItem", "implementationWorkPolicy": work}}}}}
	original, _ := json.Marshal(input)
	if ValidateInputShape(input) == nil {
		t.Fatal("current configuration accepted retired metadata")
	}
	if err := ValidateSourceShape(input); err != nil {
		t.Fatal(err)
	}
	profiles := DecodeSource(input["profileContracts"])
	p := profiles[0].EvidenceRequirements[0].ImplementationWorkPolicy
	if p == nil || p.MaxInputRevisionsPerCWI != 4 || p.ExecutionMethod().MaxInputRevisionsPerCWI != 0 {
		t.Fatal("source metadata lost or became an executable limit")
	}
	after, _ := json.Marshal(input)
	if string(after) != string(original) {
		t.Fatal("source modified")
	}
	for name, value := range map[string]any{"negative": -1, "fraction": 1.5, "text": "4", "null": nil} {
		t.Run(name, func(t *testing.T) {
			work["maxInputRevisionsPerCWI"] = value
			if ValidateSourceShape(input) == nil {
				t.Fatal("malformed source accepted")
			}
		})
	}
	work["maxInputRevisionsPerCWI"] = 4
	delete(work, "inputRevisionProtocolRef")
	if ValidateSourceShape(input) == nil {
		t.Fatal("metadata without protocol accepted")
	}
	work["inputRevisionProtocolRef"] = "validation/input-revision/v1"
	for _, key := range []string{"unknownField", "maxRoundsPerCWI", "compatibilityMode"} {
		work[key] = 4
		if ValidateSourceShape(input) == nil {
			t.Fatalf("source path admitted %s", key)
		}
		delete(work, key)
	}
}
