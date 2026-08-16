package llm

import (
	"strings"
	"testing"
)

func TestCompactSemanticPlanPromptSchemaRemovesProseMetadata(t *testing.T) {
	raw := []byte(`{"type":"object","title":"large","description":"prose","properties":{"x":{"type":"string","description":"field prose","examples":["value"]}}}`)
	compact := string(compactSemanticPlanPromptSchema(raw))
	if strings.Contains(compact, "description") || strings.Contains(compact, "examples") || strings.Contains(compact, "title") {
		t.Fatalf("compact schema retained prose metadata: %s", compact)
	}
	if !strings.Contains(compact, `"properties"`) || !strings.Contains(compact, `"type":"string"`) {
		t.Fatalf("compact schema lost structural metadata: %s", compact)
	}
}

func TestRuntimeSemanticContractRequiresCompleteCompactOutput(t *testing.T) {
	contract := runtimeSemanticPlanContractCatalog()
	for _, required := range []string{"exactly one complete compact JSON object", "finish", "unknown fields", "unrequested proposals", "preface", "suffix", "markdown fences", "repeated Goal", "RequiredOwnerFacts", "postcondition, not a current owner fact", "same ownerCatalog fact", "prospective/create-only authority cannot be used for an existing owner target"} {
		if !strings.Contains(contract, required) {
			t.Fatalf("runtime semantic contract lost %q: %s", required, contract)
		}
	}
}
