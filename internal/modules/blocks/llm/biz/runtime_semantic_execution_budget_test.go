package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeSemanticExecutionBudgetIsVolatileAndIntegrityProtected(t *testing.T) {
	request := runtimeSemanticProviderTestRequest(t)
	before, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	c := &request.SemanticContext
	c.ExecutionBudget.PhysicalDimensions[0].Unknown++
	c.ExecutionBudget.PhysicalDimensions[0].Remaining--
	c.ExecutionBudget.VisibleProofBatches[0].Used++
	c.ExecutionBudget.VisibleDurableDispatches[0].Used++
	c.ExecutionBudget.Hash = runtimecontract.StableRuntimeSemanticExecutionBudgetHash(c.ExecutionBudget)
	after, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 4 {
		if !reflect.DeepEqual(before.PromptContext.Sections[i], after.PromptContext.Sections[i]) {
			t.Fatalf("budget changed C%d", i)
		}
	}
	if before.ContextBudget.CachePlan.StablePrefixHash != after.ContextBudget.CachePlan.StablePrefixHash {
		t.Fatal("budget changed stable prefix")
	}
	var tail runtimeSemanticTailProjection
	if err := json.Unmarshal([]byte(after.PromptContext.Sections[4].Content), &tail); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tail.ExecutionBudget, c.ExecutionBudget) || tail.CoverageProgress.Satisfied != 0 {
		t.Fatal("budget lost or converted to acceptance")
	}
	for _, mutation := range []string{"missing", "refund", "proof refund", "durable refund", "durable missing", "source"} {
		t.Run(mutation, func(t *testing.T) {
			provider, err := runtimeProviderRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			value := tail
			value.ExecutionBudget = tail.ExecutionBudget.Clone()
			switch mutation {
			case "missing":
				value.ExecutionBudget = runtimecontract.RuntimeSemanticExecutionBudget{}
			case "refund":
				value.ExecutionBudget.PhysicalDimensions[0].Unknown--
				value.ExecutionBudget.PhysicalDimensions[0].Remaining++
			case "proof refund":
				value.ExecutionBudget.VisibleProofBatches[0].Used--
			case "durable refund":
				value.ExecutionBudget.VisibleDurableDispatches[0].Used--
			case "durable missing":
				value.ExecutionBudget.VisibleDurableDispatches = nil
			case "source":
				value.ExecutionBudget.SourceBudgetViewHash = "other"
			}
			value.ExecutionBudget.Hash = runtimecontract.StableRuntimeSemanticExecutionBudgetHash(value.ExecutionBudget)
			encoded, _ := json.Marshal(value)
			provider.PromptContext.Sections[4].Content = string(encoded)
			err = validateRuntimeSemanticContextIntegrity(*c, request.SemanticManifest, request.CandidateAttempt, request.SemanticCorrectionObservation, request.SemanticCorrectionDirective, provider.ContextBudget, provider.PromptContext)
			if err == nil || !strings.Contains(err.Error(), "P3.semantic_execution_budget") {
				t.Fatalf("post-render mutation accepted: %v", err)
			}
		})
	}
	c.ExecutionBudget = runtimecontract.RuntimeSemanticExecutionBudget{}
	if _, err := runtimeProviderRequest(request); err == nil {
		t.Fatal("missing budget accepted before provider")
	}
}
