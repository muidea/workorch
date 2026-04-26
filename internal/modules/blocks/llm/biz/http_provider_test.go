package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestHTTPProviderCompleteUsesChatCompletionsAndExtractsDecision(t *testing.T) {
	var gotPath string
	var gotAuthorization string
	var gotPayload chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"{\"completionState\":\"completed\",\"dodAssessment\":{\"satisfied\":true,\"items\":[{\"item\":\"done\",\"state\":\"satisfied\",\"evidenceRefs\":[\"evidence-1\"]}]}}"}}]
		}`))
	}))
	defer server.Close()

	provider := NewHTTPProvider(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		BaseURL:  server.URL + "/v1",
		APIKey:   "secret-value",
		Model:    "test-model",
		Timeout:  5 * time.Second,
	})
	payload, err := provider.Complete(context.Background(), llmProviderRequest())
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q, want /v1/chat/completions", gotPath)
	}
	if gotAuthorization != "Bearer secret-value" {
		t.Fatalf("authorization = %q, want bearer secret", gotAuthorization)
	}
	if gotPayload.Model != "test-model" || len(gotPayload.Messages) != 2 || gotPayload.ResponseFormat["type"] != "json_object" {
		t.Fatalf("payload = %+v, want chat completion json request", gotPayload)
	}
	if !strings.Contains(gotPayload.Messages[0].Content, "Do not invent a workorch CLI") ||
		!strings.Contains(gotPayload.Messages[0].Content, "runtimeCapabilities") ||
		!strings.Contains(gotPayload.Messages[1].Content, "Do not use a command named workorch") ||
		!strings.Contains(gotPayload.Messages[1].Content, "runtimeCapabilities.serviceAPIs") ||
		!strings.Contains(gotPayload.Messages[1].Content, "runtimeCapabilities") {
		t.Fatalf("messages = %+v, want runtime capability boundary", gotPayload.Messages)
	}
	if !strings.Contains(string(payload), `"completionState":"completed"`) {
		t.Fatalf("payload = %s, want decision json", payload)
	}
}

func TestHTTPProviderCompleteReportsProviderErrorWithoutLeakingSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream rejected secret-value", http.StatusUnauthorized)
	}))
	defer server.Close()

	provider := NewHTTPProvider(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		BaseURL:  server.URL,
		APIKey:   "secret-value",
		Model:    "test-model",
		Timeout:  5 * time.Second,
	})
	_, err := provider.Complete(context.Background(), llmProviderRequest())
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("complete err = %v, want invalid argument", err)
	}
	if strings.Contains(err.Error(), "secret-value") || !strings.Contains(err.Error(), "secr...alue") {
		t.Fatalf("error = %q, want redacted provider body", err.Error())
	}
}

func TestHTTPProviderStreamCompleteParsesChatCompletionDeltas(t *testing.T) {
	var gotPayload chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"completionState\\\":\\\"continue\\\"\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\",\\\"shellCommands\\\":[{\\\"command\\\":\\\"echo\\\",\\\"args\\\":[\\\"ok\\\"]}]}\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := NewHTTPProvider(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		BaseURL:  server.URL + "/v1",
		APIKey:   "secret-value",
		Model:    "test-model",
		Timeout:  5 * time.Second,
	})
	events := []llmmodels.ProviderStreamEvent{}
	payload, err := provider.StreamComplete(context.Background(), llmProviderRequest(), func(event llmmodels.ProviderStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("stream complete: %v", err)
	}
	if !gotPayload.Stream {
		t.Fatalf("stream flag = false, want true")
	}
	if !strings.Contains(string(payload), `"completionState":"continue"`) || len(events) != 3 || events[0].Delta == "" || events[2].Type != "done" {
		t.Fatalf("payload=%s events=%+v, want streamed decision and done event", payload, events)
	}
}

func TestHTTPProviderEmbedUsesEmbeddingsEndpoint(t *testing.T) {
	var gotPath string
	var gotAuthorization string
	var gotPayload embeddingRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer server.Close()

	provider := NewHTTPProvider(llmmodels.ProviderConfig{
		ID:       "embedding",
		Provider: llmcommon.ProviderCompatible,
		BaseURL:  server.URL + "/v1",
		APIKey:   "secret-value",
		Model:    "embedding-model",
		Timeout:  5 * time.Second,
	})
	result, err := provider.Embed(context.Background(), llmmodels.ProviderEmbeddingRequest{Input: "workspace goal"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if gotPath != "/v1/embeddings" {
		t.Fatalf("path = %q, want /v1/embeddings", gotPath)
	}
	if gotAuthorization != "Bearer secret-value" {
		t.Fatalf("authorization = %q, want bearer secret", gotAuthorization)
	}
	if gotPayload.Model != "embedding-model" || gotPayload.Input != "workspace goal" {
		t.Fatalf("payload = %+v, want embedding request", gotPayload)
	}
	if result.ProviderID != "embedding" || result.Model != "embedding-model" || len(result.Vector) != 3 {
		t.Fatalf("embedding result = %+v, want vector", result)
	}
}

func TestChatCompletionsURLAcceptsFullEndpoint(t *testing.T) {
	got, err := chatCompletionsURL("http://127.0.0.1:11434/v1/chat/completions")
	if err != nil {
		t.Fatalf("chat completions url: %v", err)
	}
	if got != "http://127.0.0.1:11434/v1/chat/completions" {
		t.Fatalf("url = %q, want unchanged endpoint", got)
	}
}

func llmProviderRequest() llmmodels.ProviderRequest {
	return llmmodels.ProviderRequest{
		WorkspaceID:         "workspace-1",
		RunID:               "run-1",
		StepID:              "run-1/step-1",
		Kind:                llmcommon.StepKindWorkspaceExecution,
		Model:               "test-model",
		Input:               map[string]any{"goalId": "goal-1", "doDId": "dod-1"},
		Schema:              decisionSchemaName,
		RuntimeCapabilities: declaredRuntimeCapabilities(),
	}
}
