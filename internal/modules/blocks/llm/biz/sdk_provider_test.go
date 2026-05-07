package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

func TestOpenAIProviderCompleteUsesOfficialSDKChatCompletions(t *testing.T) {
	var gotPath string
	var gotAuthorization string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"created":1,
			"model":"test-model",
			"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"{\"completionState\":\"completed\",\"dodAssessment\":{\"satisfied\":true,\"summary\":\"openai sdk done\",\"items\":[{\"item\":\"done\",\"state\":\"satisfied\",\"evidenceRefs\":[\"evidence-1\"]}]}}"}}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderOpenAI,
		BaseURL:  server.URL + "/v1",
		APIKey:   "secret-value",
		Model:    "test-model",
		Timeout:  5 * time.Second,
	})
	result, err := provider.Complete(context.Background(), llmProviderRequest())
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q, want /v1/chat/completions", gotPath)
	}
	if gotAuthorization != "Bearer secret-value" {
		t.Fatalf("authorization = %q, want bearer secret", gotAuthorization)
	}
	if gotPayload["model"] != "test-model" {
		t.Fatalf("payload model = %v, want test-model", gotPayload["model"])
	}
	if !strings.Contains(string(result.Payload), `"summary":"openai sdk done"`) {
		t.Fatalf("payload = %s, want SDK decision content", result.Payload)
	}
	if result.Usage == nil || result.Usage.InputTokens != 1 || result.Usage.OutputTokens != 1 || result.Usage.TotalTokens != 2 {
		t.Fatalf("usage = %+v, want native openai usage", result.Usage)
	}
}

func TestOpenAIProviderEmbedUsesOfficialSDKEmbeddings(t *testing.T) {
	var gotPath string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object":"list",
			"model":"text-embedding-3-small",
			"data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]}],
			"usage":{"prompt_tokens":1,"total_tokens":1}
		}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(llmmodels.ProviderConfig{
		ID:       "openai-embedding",
		Provider: llmcommon.ProviderOpenAI,
		BaseURL:  server.URL + "/v1",
		APIKey:   "secret-value",
		Model:    "text-embedding-3-small",
		Timeout:  5 * time.Second,
	})
	result, err := provider.Embed(context.Background(), llmmodels.ProviderEmbeddingRequest{Input: "workspace memory"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if gotPath != "/v1/embeddings" {
		t.Fatalf("path = %q, want /v1/embeddings", gotPath)
	}
	if gotPayload["model"] != "text-embedding-3-small" || gotPayload["input"] != "workspace memory" {
		t.Fatalf("payload = %+v, want embedding request", gotPayload)
	}
	if result.ProviderID != "openai-embedding" || len(result.Vector) != 3 {
		t.Fatalf("embedding result = %+v, want SDK vector", result)
	}
}

func TestAnthropicProviderCompleteUsesOfficialSDKMessages(t *testing.T) {
	var gotPath string
	var gotAPIKey string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg-test",
			"type":"message",
			"role":"assistant",
			"model":"claude-test",
			"content":[{"type":"text","text":"{\"completionState\":\"completed\",\"dodAssessment\":{\"satisfied\":true,\"summary\":\"anthropic sdk done\",\"items\":[{\"item\":\"done\",\"state\":\"satisfied\",\"evidenceRefs\":[\"evidence-1\"]}]}}"}],
			"stop_reason":"end_turn",
			"stop_sequence":null,
			"usage":{"input_tokens":1,"output_tokens":1}
		}`))
	}))
	defer server.Close()

	provider := NewAnthropicProvider(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderAnthropic,
		BaseURL:  server.URL,
		APIKey:   "secret-value",
		Model:    "claude-test",
		Timeout:  5 * time.Second,
	})
	result, err := provider.Complete(context.Background(), llmProviderRequest())
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("path = %q, want /v1/messages", gotPath)
	}
	if gotAPIKey != "secret-value" {
		t.Fatalf("x-api-key = %q, want secret", gotAPIKey)
	}
	if gotPayload["model"] != "claude-test" || gotPayload["max_tokens"] == nil || gotPayload["system"] == nil {
		t.Fatalf("payload = %+v, want messages request with system prompt", gotPayload)
	}
	if !strings.Contains(string(result.Payload), `"summary":"anthropic sdk done"`) {
		t.Fatalf("payload = %s, want SDK decision content", result.Payload)
	}
	if result.Usage == nil || result.Usage.InputTokens != 1 || result.Usage.OutputTokens != 1 || result.Usage.TotalTokens != 2 {
		t.Fatalf("usage = %+v, want native anthropic usage", result.Usage)
	}
}
