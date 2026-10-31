package llm

import (
	"errors"
	"strings"
	"testing"
)

func TestCatalogDiagnosticDoesNotMaskOrdinaryBearerText(t *testing.T) {
	const text = "Bearer authentication is required; literal [REDACTED] is documentation"
	if got := redactCatalogError(errors.New(text)); got != text {
		t.Fatalf("ordinary diagnostic rewritten: %q", got)
	}
	const credential = "synthetic-configured-credential"
	got := redactCatalogError(errors.New("Bearer "+credential), credential)
	if strings.Contains(got, credential) {
		t.Fatal("configured credential entered public catalog diagnostic")
	}
}
