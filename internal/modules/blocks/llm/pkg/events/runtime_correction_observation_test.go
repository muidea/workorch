package events

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeCandidateCommandsCarryOnlyBoundedCorrectionObservations(t *testing.T) {
	candidate := RuntimeCandidateCorrectionObservationForEventTest()
	semantic := RuntimeSemanticCorrectionObservationForEventTest()
	commands := make([][]byte, 0, 4)
	for _, encoded := range [][]byte{
		mustMarshalRuntimeCorrectionCommand(t, RequestRuntimeCandidateCommand{CandidateCorrectionObservation: &candidate}),
		mustMarshalRuntimeCorrectionCommand(t, PreflightRuntimeCandidateCommand{CandidateCorrectionObservation: &candidate}),
		mustMarshalRuntimeCorrectionCommand(t, RequestRuntimeCandidateCommand{SemanticCorrectionObservation: &semantic}),
		mustMarshalRuntimeCorrectionCommand(t, PreflightRuntimeCandidateCommand{SemanticCorrectionObservation: &semantic}),
	} {
		commands = append(commands, encoded)
	}
	for index, encoded := range commands {
		t.Run(string(rune('0'+index)), func(t *testing.T) {
			text := string(encoded)
			for _, forbidden := range []string{"Reason", "OriginalMessage", "DetailsRef", "ValidationDiagnostic", "GovernanceDependency", "private owner text"} {
				if strings.Contains(text, forbidden) {
					t.Fatalf("EventHub command leaked %q: %s", forbidden, text)
				}
			}
		})
	}
}

func mustMarshalRuntimeCorrectionCommand[T RequestRuntimeCandidateCommand | PreflightRuntimeCandidateCommand](t *testing.T, command T) []byte {
	t.Helper()
	encoded, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestRuntimeProviderDocumentOutcomeContentIsVolatileJSONExcluded(t *testing.T) {
	outcome := RuntimeProviderDocumentOutcome{
		Framing: runtimecontract.RuntimeProviderDocumentFraming{
			Disposition: runtimecontract.RuntimeProviderDocumentComplete, ExpectedDocumentHash: "expected-1",
			ResponseHash: runtimecontract.StableRuntimeProviderResponseHash([]byte(`{"operations":[]}`)), ResponseBytes: int64(len(`{"operations":[]}`)),
		},
		Content: []byte(`{"operations":[]}`),
	}
	encoded, err := json.Marshal(outcome)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "operations") || strings.Contains(string(encoded), "Content") {
		t.Fatalf("volatile complete content entered EventHub JSON: %s", encoded)
	}
}

func RuntimeCandidateCorrectionObservationForEventTest() runtimecontract.RuntimeCandidateCorrectionObservation {
	return runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{Path: "$.operations", Required: []string{"operations"}},
	}
}

func RuntimeSemanticCorrectionObservationForEventTest() runtimecontract.RuntimeSemanticCorrectionObservation {
	return runtimecontract.RuntimeSemanticCorrectionObservation{
		SchemaVersion: runtimecontract.RuntimeSemanticCorrectionObservationSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM, Class: "schema", Code: "semantic_document_nonconformant", Signature: "semantic-document-nonconformant",
		SemanticViolations: []runtimecontract.RuntimeSemanticContractViolation{{
			Kind: runtimecontract.RuntimeSemanticContractViolationCoverageOrderDocument, Path: "$.coverageOrder",
			RequiredCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		}},
	}
}
