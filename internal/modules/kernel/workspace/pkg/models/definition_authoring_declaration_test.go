package workspace

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestAuthoringDeclarationSchemaGuidesGenerationWithoutChangingHistoricalFailure(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(DefinitionAuthoringMappingSchema), &schema); err != nil {
		t.Fatal(err)
	}
	properties := func(value map[string]any) map[string]any { return value["properties"].(map[string]any) }
	mapping := properties(schema)["mapping"].(map[string]any)
	obligation := properties(mapping)["Obligations"].(map[string]any)["items"].(map[string]any)
	declaration := properties(properties(obligation)["Verification"].(map[string]any))["Declaration"].(map[string]any)
	if declaration["minLength"] != float64(1) || declaration["pattern"] != `\S` {
		t.Fatalf("generation schema lost nonblank declaration: %+v", declaration)
	}
	pattern := regexp.MustCompile(declaration["pattern"].(string))
	if !pattern.MatchString("具体能力说明，尚未执行") || pattern.MatchString(" \t\r\n") {
		t.Fatal("generation schema pattern does not match declaration semantics")
	}

	// Old semantic failures retain their typed completion identity: a new
	// transport schema must not turn them into earlier decoder failures.
	record, original, _ := fullSizeMappingFixture(t)
	for i := range original.Obligations {
		original.Obligations[i].Verification.Declaration = ""
	}
	audit := mappingAuditFixture(t, original)
	decoded, err := DecodeDefinitionAuthoringMapping(audit.Document.Content)
	if err != nil || !reflect.DeepEqual(decoded, original) {
		t.Fatalf("historical mapping decode changed: %v", err)
	}
	_, semanticErr := CompileDefinitionAuthoringMapping(record.DerivationRequests[0], decoded)
	if semanticErr == nil || !strings.Contains(semanticErr.Error(), "has no traceable obligation and verification") {
		t.Fatalf("historical first semantic failure changed: %v", semanticErr)
	}
	record, request, _, err := record.ReserveSpecification(DefinitionAuthoringSpecificationRequest{RequestID: "historical-empty", DerivationPermitID: record.DerivationRequests[0].PermitID, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "mock-provider"})
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteMappingFailure(request.PermitID, decoded, audit, MappingFailureDiagnostic(semanticErr, audit))
	if err != nil || record.Validate() != nil {
		t.Fatalf("historical failure ledger rejected: %v", err)
	}
	failed := record.SpecificationRequests[0]
	reparsed, _, err := ReparseMappingFailure(failed)
	if err != nil || !reflect.DeepEqual(reparsed, original) || failed.CompletionIdentity == "" {
		t.Fatalf("historical raw reparse/completion changed: %v", err)
	}
}
