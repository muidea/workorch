package activationports

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestGenerateEmbeddingRejectsCompletionResultOnEmbeddingTopic(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	observer := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	observer.Subscribe(llmevents.TopicLLMCommandGenerateEmbedding, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(llmevents.CompleteProviderResult{}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(llmevents.TopicLLMCommandGenerateEmbedding) })
	time.Sleep(10 * time.Millisecond)
	_, err := GenerateEmbedding(ctx, hub, "workspace-1", runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "embedding-model"}, "input")
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("cross-topic result error = %v, want invalid argument", err)
	}
}

func TestGenerateEmbeddingRejectsInvalidConcreteResult(t *testing.T) {
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "embedding-model"}
	valid := llmevents.GenerateEmbeddingResult{Target: target, Embedding: llmmodels.ProviderEmbeddingResult{
		ProviderID: target.ProviderID, Model: target.ModelID, Input: "input", Vector: []float64{0.5},
		PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming, ProviderSubmitted: true, RequestBytes: 8, ResponseBytes: 16,
	}}
	for _, test := range []struct {
		name   string
		mutate func(*llmevents.GenerateEmbeddingResult)
	}{
		{name: "empty", mutate: func(result *llmevents.GenerateEmbeddingResult) { *result = llmevents.GenerateEmbeddingResult{} }},
		{name: "target drift", mutate: func(result *llmevents.GenerateEmbeddingResult) { result.Target.ModelID = "other" }},
		{name: "empty vector", mutate: func(result *llmevents.GenerateEmbeddingResult) { result.Embedding.Vector = nil }},
		{name: "non-finite vector", mutate: func(result *llmevents.GenerateEmbeddingResult) { result.Embedding.Vector = []float64{math.NaN()} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			response := valid
			response.Embedding.Vector = append([]float64(nil), valid.Embedding.Vector...)
			test.mutate(&response)
			observer := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
			observer.Subscribe(llmevents.TopicLLMCommandGenerateEmbedding, func(_ magicevent.Event, result magicevent.Result) { result.Set(response, nil) })
			t.Cleanup(func() { observer.Unsubscribe(llmevents.TopicLLMCommandGenerateEmbedding) })
			time.Sleep(10 * time.Millisecond)
			_, err := GenerateEmbedding(ctx, hub, "workspace-1", target, "input")
			if !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("invalid concrete result error = %v, want invalid argument", err)
			}
		})
	}
}

func TestGenerateEmbeddingReturnsDetachedVectorAndUsage(t *testing.T) {
	const input = "  input\n"
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "embedding-model"}
	sharedVector := []float64{0.25, 0.75}
	sharedUsage := &llmaudit.ProviderUsage{InputTokens: 3, TotalTokens: 3}
	response := llmevents.GenerateEmbeddingResult{Target: target, Embedding: llmmodels.ProviderEmbeddingResult{
		ProviderID: target.ProviderID, Model: target.ModelID, Input: input, Vector: sharedVector, Usage: sharedUsage,
		PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming, ProviderSubmitted: true, RequestBytes: 8, ResponseBytes: 16,
	}}
	observer := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	observer.Subscribe(llmevents.TopicLLMCommandGenerateEmbedding, func(ev magicevent.Event, result magicevent.Result) {
		if command, ok := ev.Data().(llmevents.GenerateEmbeddingCommand); !ok || command.Input != input {
			t.Errorf("original input was changed: %#v", ev.Data())
		}
		result.Set(response, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(llmevents.TopicLLMCommandGenerateEmbedding) })
	time.Sleep(10 * time.Millisecond)

	embedding, err := GenerateEmbedding(ctx, hub, "workspace-1", target, input)
	if err != nil {
		t.Fatal(err)
	}
	embedding.Vector[0] = 99
	embedding.Usage.TotalTokens = 99
	if sharedVector[0] != 0.25 || sharedUsage.TotalTokens != 3 {
		t.Fatalf("embedding result aliases owner response: vector=%v usage=%+v", sharedVector, sharedUsage)
	}
}
