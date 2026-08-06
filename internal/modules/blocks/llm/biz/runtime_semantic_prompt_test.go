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
