package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestLLMContextBudgetTraceProjectsIntegrityMetadataWithoutPromptContent(t *testing.T) {
	sections := []workorcheventpayload.Value{{
		Kind: workorcheventpayload.ValueObject,
		Object: workorcheventpayload.New(
			workorcheventpayload.StringField("id", "C3.runtime-authority"),
			workorcheventpayload.StringField("hash", "sha256:authority"),
			workorcheventpayload.IntField("sizeBytes", 99),
			workorcheventpayload.StringArrayField("sourceRefs", []string{"runtime://focus/focus-1"}),
		).Fields,
	}}
	budget := workorcheventpayload.New(
		workorcheventpayload.BoolField("providerSubmitted", false),
		workorcheventpayload.StringField("contextGateAction", "fail_fast"),
		workorcheventpayload.StringField("contextGateReason", "runtime_context_integrity_failed"),
		workorcheventpayload.StringField("budgetFailureClass", "context_integrity"),
		workorcheventpayload.StringArrayField("redactedPaths", []string{"activation.memories[0].content"}),
		workorcheventpayload.StringArrayField("protectedFields", []string{"P0.goalDefinition", "P4.targetSnapshots"}),
		workorcheventpayload.StringArrayField("integrityWarnings", []string{"P4 snapshot unavailable"}),
		workorcheventpayload.StringArrayField("sourceRefs", []string{"runtime://authority/snapshot-1"}),
		workorcheventpayload.Field{Name: "sectionHashes", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: sections}},
	)
	got := llmContextBudgetTraceFromStep(projectioncontract.StepView{ID: "step-1"}, budget, workorcheventpayload.Payload{})
	if got.ProviderSubmitted || got.ContextGateAction != "fail_fast" || got.ContextGateReason == "" || got.BudgetFailureClass != "context_integrity" {
		t.Fatalf("integrity gate trace = %+v", got)
	}
	if len(got.RedactedPaths) != 1 || len(got.ProtectedFields) != 2 || len(got.IntegrityWarnings) != 1 || len(got.SourceRefs) != 1 || got.SourceRefs[0] != "runtime://authority/snapshot-1" {
		t.Fatalf("integrity metadata = %+v", got)
	}
	if len(got.SectionHashes) != 1 || got.SectionHashes[0].ID != "C3.runtime-authority" || got.SectionHashes[0].Hash != "sha256:authority" || got.SectionHashes[0].SizeBytes != 99 || len(got.SectionHashes[0].SourceRefs) != 1 {
		t.Fatalf("section metadata = %+v", got.SectionHashes)
	}
}
