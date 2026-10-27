package events

import (
	"errors"
	"math"
	"reflect"
	"testing"

	llmmodel "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestProviderRequestTraceReferencesAreStableAndRoundTrip(t *testing.T) {
	input := llmmodel.ProviderRequest{TraceRefs: map[string]string{"templateId": "template-1", "requestId": "request-1"}}
	contract := ProviderRequestFromModel(input)
	want := []TraceReference{{Name: "requestId", Ref: "request-1"}, {Name: "templateId", Ref: "template-1"}}
	if !reflect.DeepEqual(contract.TraceRefs, want) {
		t.Fatalf("TraceRefs = %+v, want stable %+v", contract.TraceRefs, want)
	}
	input.TraceRefs["requestId"] = "mutated"
	model, err := contract.ToModel()
	if err != nil {
		t.Fatal(err)
	}
	if model.TraceRefs["requestId"] != "request-1" {
		t.Fatalf("round trip TraceRefs = %+v", model.TraceRefs)
	}
}

func TestConcreteProviderResultsValidateExactCommandFacts(t *testing.T) {
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"}
	completionCommand := CompleteProviderCommand{Target: target, Request: ProviderRequest{Model: target.ModelID}}
	completion := CompleteProviderResult{Target: target, Completion: ProviderCompletionData{Content: "ok"}}
	if err := completion.ValidateFor(completionCommand); err != nil {
		t.Fatalf("valid completion: %v", err)
	}
	driftedCompletion := completion
	driftedCompletion.Target.ModelID = "other"
	if err := driftedCompletion.ValidateFor(completionCommand); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("completion target drift error = %v", err)
	}
	emptyCompletion := completion
	emptyCompletion.Completion.Content = ""
	if err := emptyCompletion.ValidateFor(completionCommand); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty completion error = %v", err)
	}
	nonCanonicalCompletion := completion
	nonCanonicalCompletion.Target.ModelID = " model-1 "
	if err := nonCanonicalCompletion.ValidateFor(completionCommand); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("non-canonical completion error = %v", err)
	}

	embeddingCommand := GenerateEmbeddingCommand{WorkspaceID: "workspace-1", Target: target, Input: "input"}
	embedding := GenerateEmbeddingResult{Target: target, Embedding: llmmodel.ProviderEmbeddingResult{
		ProviderID: target.ProviderID, Model: target.ModelID, Input: "input", Vector: []float64{0.25},
		PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming, ProviderSubmitted: true, RequestBytes: 8, ResponseBytes: 16,
	}}
	if err := embedding.ValidateFor(embeddingCommand); err != nil {
		t.Fatalf("valid embedding: %v", err)
	}
	spacedCommand := embeddingCommand
	spacedCommand.Input = "  input\n"
	if err := embedding.ValidateFor(spacedCommand); err == nil {
		t.Fatal("trimmed input must not match original command")
	}
	spacedResult := embedding
	spacedResult.Embedding.Input = spacedCommand.Input
	if err := spacedResult.ValidateFor(spacedCommand); err != nil {
		t.Fatal(err)
	}
	nonFinite := embedding
	nonFinite.Embedding.Vector = []float64{math.Inf(1)}
	if err := nonFinite.ValidateFor(embeddingCommand); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("non-finite embedding error = %v", err)
	}
	missingPhysicalFact := embedding
	missingPhysicalFact.Embedding.ProviderSubmitted = false
	if err := missingPhysicalFact.ValidateFor(embeddingCommand); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("missing physical fact error = %v", err)
	}
	nonCanonicalEmbedding := embedding
	nonCanonicalEmbedding.Embedding.ProviderID = " ai-proxy "
	if err := nonCanonicalEmbedding.ValidateFor(embeddingCommand); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("non-canonical embedding error = %v", err)
	}
}

func TestProviderRequestRejectsAmbiguousTraceReferences(t *testing.T) {
	for _, refs := range [][]TraceReference{
		{{Name: "", Ref: "ref"}},
		{{Name: "requestId", Ref: ""}},
		{{Name: "requestId", Ref: "one"}, {Name: "requestId", Ref: "two"}},
	} {
		if _, err := (ProviderRequest{TraceRefs: refs}).ToModel(); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("Model error = %v for %+v, want invalid argument", err, refs)
		}
	}
}
