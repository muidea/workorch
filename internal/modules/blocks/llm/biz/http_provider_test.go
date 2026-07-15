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
	var gotRawPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotRawPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		raw, _ := json.Marshal(gotRawPayload)
		if err := json.Unmarshal(raw, &gotPayload); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"{\"completionState\":\"completed\",\"dodAssessment\":{\"satisfied\":true,\"summary\":\"done with evidence\",\"items\":[{\"item\":\"done\",\"state\":\"satisfied\",\"evidenceRefs\":[\"evidence-1\"]}]}}"}}]
		}`))
	}))
	defer server.Close()

	provider := NewHTTPProvider(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		BaseURL:  server.URL + "/v1",
		APIKey:   "secret-value",
		Model:    "test-model",
		Timeout:  5 * time.Second,
		InferenceProfile: llmmodels.ProviderInferenceProfile{
			MaxOutputTokens:      65536,
			ReasoningEffort:      "high",
			ThinkingBudgetTokens: 32768,
		},
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
	if gotPayload.Model != "test-model" || len(gotPayload.Messages) != 2 || gotPayload.ResponseFormat["type"] != "json_object" {
		t.Fatalf("payload = %+v, want chat completion json request", gotPayload)
	}
	if gotPayload.MaxCompletionTokens != 65536 ||
		gotPayload.ReasoningEffort != "high" ||
		gotPayload.Thinking == nil ||
		gotPayload.Thinking.BudgetTokens != 32768 {
		t.Fatalf("payload = %+v, want configured inference profile", gotPayload)
	}
	if _, ok := gotRawPayload["prompt_cache_key"]; ok {
		t.Fatalf("payload = %+v, compatible default must not send prompt_cache_key", gotRawPayload)
	}
	joined := ""
	for _, message := range gotPayload.Messages {
		joined += "\n" + message.Content
	}
	if gotPayload.Messages[0].Role != "system" ||
		!strings.Contains(gotPayload.Messages[0].Content, "C0.runtime-candidate-contract") ||
		!strings.Contains(gotPayload.Messages[0].Content, "C1.goal-dod") {
		t.Fatalf("first message = %+v, want stable provider prefix", gotPayload.Messages[0])
	}
	if strings.Contains(gotPayload.Messages[0].Content, "C3.runtime-authority") {
		t.Fatalf("first message leaked dynamic sections: %s", gotPayload.Messages[0].Content)
	}
	if !strings.Contains(joined, "C3.runtime-authority") {
		t.Fatalf("messages = %+v, want dynamic tail after stable prefix", gotPayload.Messages)
	}
	if !strings.Contains(joined, `"goal":"goal-1"`) ||
		!strings.Contains(joined, `"action":"produce_assessment"`) {
		t.Fatalf("messages = %+v, want typed Runtime V2 prompt sections", gotPayload.Messages)
	}
	if !strings.Contains(string(result.Payload), `"completionState":"completed"`) {
		t.Fatalf("payload = %s, want decision json", result.Payload)
	}
	if result.Usage != nil {
		t.Fatalf("usage = %+v, want nil when provider response has no usage", result.Usage)
	}
}

func TestHTTPProviderCompleteUsesAnthropicMessagesShapeAndDeclaredCacheControl(t *testing.T) {
	var gotPath string
	var gotAPIKey string
	var gotVersion string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"type":"message",
			"role":"assistant",
			"model":"claude-test",
			"content":[{"type":"text","text":"{\"completionState\":\"completed\",\"dodAssessment\":{\"satisfied\":true,\"summary\":\"anthropic http done\",\"items\":[{\"item\":\"done\",\"state\":\"satisfied\",\"evidenceRefs\":[\"evidence-1\"]}]}}"}],
			"usage":{"input_tokens":11,"output_tokens":7,"cache_read_input_tokens":5,"cache_creation_input_tokens":6}
		}`))
	}))
	defer server.Close()

	provider := NewHTTPProvider(llmmodels.ProviderConfig{
		Provider:   llmcommon.ProviderCompatible,
		APISurface: llmcommon.ProviderSurfaceAnthropicCompatibleHTTP,
		BaseURL:    server.URL + "/v1",
		APIKey:     "secret-value",
		Model:      "claude-test",
		Timeout:    5 * time.Second,
		CacheCapability: llmmodels.ProviderCacheCapability{
			Surface:             llmcommon.ProviderSurfaceAnthropicCompatibleHTTP,
			Mode:                llmcommon.ProviderCacheModeCompatibleDeclared,
			MessageCacheControl: true,
		},
	})
	result, err := provider.Complete(context.Background(), llmProviderRequest())
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("path = %q, want /v1/messages", gotPath)
	}
	if gotAPIKey != "secret-value" || gotVersion != "2023-06-01" {
		t.Fatalf("headers = %q/%q, want anthropic headers", gotAPIKey, gotVersion)
	}
	if gotPayload["model"] != "claude-test" || gotPayload["max_tokens"] == nil || gotPayload["system"] == nil {
		t.Fatalf("payload = %+v, want anthropic messages request", gotPayload)
	}
	systemBlocks, ok := gotPayload["system"].([]any)
	if !ok || len(systemBlocks) == 0 {
		t.Fatalf("system = %#v, want text block array", gotPayload["system"])
	}
	firstSystem, ok := systemBlocks[0].(map[string]any)
	if !ok {
		t.Fatalf("first system block = %#v, want object", systemBlocks[0])
	}
	if cacheControl, ok := firstSystem["cache_control"].(map[string]any); !ok || cacheControl["type"] != "ephemeral" {
		t.Fatalf("system block = %+v, want declared anthropic cache_control", firstSystem)
	}
	messages, _ := json.Marshal(gotPayload["messages"])
	if !strings.Contains(string(messages), "C3.runtime-authority") {
		t.Fatalf("messages = %s, want dynamic tail after stable prefix", messages)
	}
	if !strings.Contains(string(result.Payload), `"summary":"anthropic http done"`) {
		t.Fatalf("payload = %s, want anthropic decision content", result.Payload)
	}
	if result.Usage == nil ||
		result.Usage.InputTokens != 11 ||
		result.Usage.OutputTokens != 7 ||
		result.Usage.TotalTokens != 18 ||
		result.Usage.CacheReadTokens != 5 ||
		result.Usage.CacheWriteTokens != 6 ||
		result.Usage.CacheCreationTokens != 6 {
		t.Fatalf("usage = %+v, want parsed anthropic usage", result.Usage)
	}
}

func TestHTTPProviderAnthropicCompatibleDefaultDoesNotSendCacheControl(t *testing.T) {
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"type":"message",
			"role":"assistant",
			"model":"claude-test",
			"content":[{"type":"text","text":"{\"completionState\":\"completed\",\"dodAssessment\":{\"satisfied\":true,\"summary\":\"ok\",\"items\":[]}}"}],
			"usage":{"input_tokens":1,"output_tokens":1}
		}`))
	}))
	defer server.Close()

	provider := NewHTTPProvider(llmmodels.ProviderConfig{
		Provider:   llmcommon.ProviderCompatible,
		APISurface: llmcommon.ProviderSurfaceAnthropicCompatibleHTTP,
		BaseURL:    server.URL + "/v1",
		APIKey:     "secret-value",
		Model:      "claude-test",
		Timeout:    5 * time.Second,
	})
	if _, err := provider.Complete(context.Background(), llmProviderRequest()); err != nil {
		t.Fatalf("complete: %v", err)
	}
	systemBlocks, ok := gotPayload["system"].([]any)
	if !ok || len(systemBlocks) == 0 {
		t.Fatalf("system = %#v, want text block array", gotPayload["system"])
	}
	firstSystem, ok := systemBlocks[0].(map[string]any)
	if !ok {
		t.Fatalf("first system block = %#v, want object", systemBlocks[0])
	}
	if _, ok := firstSystem["cache_control"]; ok {
		t.Fatalf("system block = %+v, cache_control must not be sent by default", firstSystem)
	}
}

func TestHTTPProviderStreamCompleteParsesAnthropicMessagesDeltas(t *testing.T) {
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"{\\\"completionState\\\":\\\"continue\\\"\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\",\\\"nextAction\\\":{\\\"kind\\\":\\\"continue_run\\\"}}\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":11,\"output_tokens\":7,\"cache_read_input_tokens\":5,\"cache_creation_input_tokens\":6}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := NewHTTPProvider(llmmodels.ProviderConfig{
		Provider:   llmcommon.ProviderCompatible,
		APISurface: llmcommon.ProviderSurfaceAnthropicCompatibleHTTP,
		BaseURL:    server.URL + "/v1",
		APIKey:     "secret-value",
		Model:      "claude-test",
		Timeout:    5 * time.Second,
		CacheCapability: llmmodels.ProviderCacheCapability{
			Surface:             llmcommon.ProviderSurfaceAnthropicCompatibleHTTP,
			Mode:                llmcommon.ProviderCacheModeCompatibleDeclared,
			MessageCacheControl: true,
		},
	})
	events := []llmmodels.ProviderStreamEvent{}
	result, err := provider.StreamComplete(context.Background(), llmProviderRequest(), func(event llmmodels.ProviderStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("stream complete: %v", err)
	}
	if gotPayload["stream"] != true {
		t.Fatalf("payload = %+v, want stream=true", gotPayload)
	}
	if gotPayload["system"] == nil {
		t.Fatalf("payload = %+v, want system prompt", gotPayload)
	}
	if !strings.Contains(string(result.Payload), `"completionState":"continue"`) || len(events) != 3 || events[0].Delta == "" || events[2].Type != "done" {
		t.Fatalf("payload=%s events=%+v, want streamed anthropic decision and done event", result.Payload, events)
	}
	if result.Usage == nil ||
		result.Usage.InputTokens != 11 ||
		result.Usage.OutputTokens != 7 ||
		result.Usage.TotalTokens != 18 ||
		result.Usage.CacheReadTokens != 5 ||
		result.Usage.CacheWriteTokens != 6 {
		t.Fatalf("usage = %+v, want stream usage", result.Usage)
	}
}

func TestHTTPProviderSendsPromptCacheKeyOnlyWhenCapabilityDeclared(t *testing.T) {
	var gotPayload chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"{\"completionState\":\"completed\",\"dodAssessment\":{\"satisfied\":true,\"summary\":\"done\",\"items\":[]}}"}}]
		}`))
	}))
	defer server.Close()

	provider := NewHTTPProvider(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		BaseURL:  server.URL + "/v1",
		APIKey:   "secret-value",
		Model:    "test-model",
		Timeout:  5 * time.Second,
		CacheCapability: llmmodels.ProviderCacheCapability{
			Surface:        llmcommon.ProviderSurfaceOpenAICompatibleHTTP,
			Mode:           llmcommon.ProviderCacheModeCompatibleDeclared,
			PromptCacheKey: true,
		},
	})
	request := llmProviderRequest()
	request.ContextBudget = &llmmodels.ProviderContextBudget{
		CachePlan: &llmmodels.ProviderCachePlan{CacheKey: "sha256:stable-cache-key"},
	}

	if _, err := provider.Complete(context.Background(), request); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if gotPayload.PromptCacheKey != "sha256:stable-cache-key" {
		t.Fatalf("prompt_cache_key = %q, want declared cache key", gotPayload.PromptCacheKey)
	}
}

func TestHTTPProviderRejectsUnknownHTTPSurfaceBeforeSubmitting(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := NewHTTPProvider(llmmodels.ProviderConfig{
		Provider:   llmcommon.ProviderCompatible,
		APISurface: "unsupported_surface",
		BaseURL:    server.URL + "/v1",
		APIKey:     "secret-value",
		Model:      "test-model",
		Timeout:    5 * time.Second,
	})
	_, err := provider.Complete(context.Background(), llmProviderRequest())
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("complete err = %v, want invalid argument", err)
	}
	if called {
		t.Fatalf("unsupported surface should fail before submitting request")
	}
}

func TestHTTPProviderCompleteParsesNonStreamingUsage(t *testing.T) {
	var gotPayload chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"{\"completionState\":\"completed\",\"dodAssessment\":{\"satisfied\":true,\"summary\":\"usage parsed\",\"items\":[{\"item\":\"usage parsed\",\"state\":\"satisfied\",\"evidenceRefs\":[\"evidence-1\"]}]}}"}}],
			"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18,"prompt_cache_hit_tokens":5,"prompt_cache_miss_tokens":6}
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
	result, err := provider.Complete(context.Background(), llmProviderRequest())
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if gotPayload.Stream {
		t.Fatalf("stream flag = true, want false")
	}
	if gotPayload.ResponseFormat == nil || gotPayload.ResponseFormat["type"] != "json_object" {
		t.Fatalf("payload response_format = %+v, want json_object", gotPayload.ResponseFormat)
	}
	if result.Usage == nil ||
		result.Usage.InputTokens != 11 ||
		result.Usage.OutputTokens != 7 ||
		result.Usage.TotalTokens != 18 ||
		result.Usage.CacheReadTokens != 5 ||
		result.Usage.CacheWriteTokens != 6 {
		t.Fatalf("usage = %+v, want parsed non-stream usage", result.Usage)
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

func TestHTTPProviderCompleteRejectsProviderErrorObjectWithHTTP200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":{"message":"model overloaded","type":"server_error","code":"upstream_failed"}}`))
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
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "llm provider protocol invalid") ||
		!strings.Contains(err.Error(), "model overloaded") {
		t.Fatalf("complete err = %v, want provider protocol invalid error object", err)
	}
}

func TestHTTPProviderCompleteRejectsMissingChoicesAsProviderSchemaInvalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-empty","choices":[]}`))
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
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "llm provider response schema invalid") {
		t.Fatalf("complete err = %v, want response schema invalid", err)
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
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"total_tokens\":18,\"prompt_cache_hit_tokens\":5,\"prompt_cache_miss_tokens\":6}}\n\n"))
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
	result, err := provider.StreamComplete(context.Background(), llmProviderRequest(), func(event llmmodels.ProviderStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("stream complete: %v", err)
	}
	if !gotPayload.Stream {
		t.Fatalf("stream flag = false, want true")
	}
	if gotPayload.StreamOptions == nil || !gotPayload.StreamOptions.IncludeUsage {
		t.Fatalf("stream options = %+v, want include_usage=true", gotPayload.StreamOptions)
	}
	if !strings.Contains(string(result.Payload), `"completionState":"continue"`) || len(events) != 3 || events[0].Delta == "" || events[2].Type != "done" {
		t.Fatalf("payload=%s events=%+v, want streamed decision and done event", result.Payload, events)
	}
	if result.Usage == nil ||
		result.Usage.InputTokens != 11 ||
		result.Usage.OutputTokens != 7 ||
		result.Usage.TotalTokens != 18 ||
		result.Usage.CacheReadTokens != 5 ||
		result.Usage.CacheWriteTokens != 6 {
		t.Fatalf("usage = %+v, want stream usage", result.Usage)
	}
}

func TestHTTPProviderStreamCompleteRejectsProviderErrorEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"error":{"message":"stream rejected","type":"server_error"}}` + "\n\n"))
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
	_, err := provider.StreamComplete(context.Background(), llmProviderRequest(), nil)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "llm provider protocol invalid") ||
		!strings.Contains(err.Error(), "stream rejected") {
		t.Fatalf("stream complete err = %v, want provider protocol invalid event", err)
	}
}

func TestHTTPProviderStreamCompleteAcceptsLargeSSELine(t *testing.T) {
	largeDecision := `{"completionState":"continue","executionGraph":{"nodes":[{"id":"write-large","kind":"patch/apply","input":{"patchType":"file_write","targetRef":"large.txt","content":"` + strings.Repeat("x", 2*1024*1024) + `"}}]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		payload, err := json.Marshal(map[string]any{
			"choices": []map[string]any{{
				"delta": map[string]any{"content": largeDecision},
			}},
		})
		if err != nil {
			t.Fatalf("marshal stream chunk: %v", err)
		}
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(payload)
		_, _ = w.Write([]byte("\n\n"))
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
	result, err := provider.StreamComplete(context.Background(), llmProviderRequest(), nil)
	if err != nil {
		t.Fatalf("stream complete: %v", err)
	}
	if !strings.Contains(string(result.Payload), strings.Repeat("x", 1024)) {
		t.Fatalf("payload missing large streamed content")
	}
}

func TestHTTPProviderStreamCompleteRejectsEOFBeforeDone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"completionState\\\":\\\"continue\\\"\"}}]}\n\n"))
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
	_, err := provider.StreamComplete(context.Background(), llmProviderRequest(), func(event llmmodels.ProviderStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "stream incomplete") {
		t.Fatalf("stream complete err = %v, want stream incomplete invalid argument", err)
	}
	if len(events) != 1 || events[0].Type != "delta" {
		t.Fatalf("events = %+v, want only partial delta without done", events)
	}
}

func TestHTTPProviderStreamCompleteRejectsContentBeyondLimit(t *testing.T) {
	stream := strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"content\":\"12345\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"67890\"}}]}\n\n" +
			"data: [DONE]\n\n",
	)
	events := []llmmodels.ProviderStreamEvent{}
	_, _, err := readChatCompletionStreamWithLimits(stream, func(event llmmodels.ProviderStreamEvent) error {
		events = append(events, event)
		return nil
	}, providerStreamLimits{MaxContentBytes: 8, MaxDataEvents: 16})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "stream incomplete") ||
		!strings.Contains(err.Error(), "content byte limit") {
		t.Fatalf("stream err = %v, want content byte limit stream incomplete", err)
	}
	if len(events) != 1 || events[0].Delta != "12345" {
		t.Fatalf("events = %+v, want only emitted content before limit breach", events)
	}
}

func TestHTTPProviderStreamCompleteRejectsDataEventsBeyondLimit(t *testing.T) {
	stream := strings.NewReader(
		"data: {\"choices\":[]}\n\n" +
			"data: {\"choices\":[]}\n\n" +
			"data: {\"choices\":[]}\n\n" +
			"data: [DONE]\n\n",
	)
	_, _, err := readChatCompletionStreamWithLimits(stream, nil, providerStreamLimits{MaxContentBytes: 1024, MaxDataEvents: 2})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "stream incomplete") ||
		!strings.Contains(err.Error(), "data event limit") {
		t.Fatalf("stream err = %v, want data event limit stream incomplete", err)
	}
}

func TestHTTPProviderAnthropicStreamRejectsContentBeyondLimit(t *testing.T) {
	stream := strings.NewReader(
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"12345\"}}\n\n" +
			"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"67890\"}}\n\n" +
			"data: [DONE]\n\n",
	)
	_, _, err := readAnthropicHTTPStreamWithLimits(stream, nil, providerStreamLimits{MaxContentBytes: 8, MaxDataEvents: 16})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "stream incomplete") ||
		!strings.Contains(err.Error(), "content byte limit") {
		t.Fatalf("stream err = %v, want content byte limit stream incomplete", err)
	}
}

func TestHTTPProviderStreamCompleteTreatsTruncatedChunkAsIncomplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"completionState\\\":\\\"continue\\\"\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}\n\n"))
	}))
	defer server.Close()

	provider := NewHTTPProvider(llmmodels.ProviderConfig{
		Provider: llmcommon.ProviderCompatible,
		BaseURL:  server.URL + "/v1",
		APIKey:   "secret-value",
		Model:    "test-model",
		Timeout:  5 * time.Second,
	})
	_, err := provider.StreamComplete(context.Background(), llmProviderRequest(), nil)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "stream incomplete") ||
		!strings.Contains(err.Error(), "truncated stream chunk json") {
		t.Fatalf("stream complete err = %v, want truncated stream incomplete", err)
	}
	if strings.Contains(err.Error(), "provider protocol invalid") {
		t.Fatalf("stream complete err = %v, truncated chunk should not be protocol invalid", err)
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

func TestOpenAICompatibleURLsNormalizeHostOnlyBase(t *testing.T) {
	chat, err := chatCompletionsURL("http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("chat completions url: %v", err)
	}
	if chat != "http://127.0.0.1:8080/v1/chat/completions" {
		t.Fatalf("chat url = %q, want host-only base normalized with /v1", chat)
	}
	embed, err := embeddingsURL("http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("embeddings url: %v", err)
	}
	if embed != "http://127.0.0.1:8080/v1/embeddings" {
		t.Fatalf("embeddings url = %q, want host-only base normalized with /v1", embed)
	}
	chatV1, err := chatCompletionsURL("http://127.0.0.1:18181/v1")
	if err != nil {
		t.Fatalf("chat completions /v1 url: %v", err)
	}
	if chatV1 != "http://127.0.0.1:18181/v1/chat/completions" {
		t.Fatalf("chat /v1 url = %q, want unchanged /v1 root", chatV1)
	}
}

func llmProviderRequest() llmmodels.ProviderRequest {
	return llmmodels.ProviderRequest{
		WorkspaceID:    "workspace-1",
		RunID:          "run-1",
		StepID:         "run-1/step-1",
		Kind:           "runtime_v2_candidate",
		Model:          "test-model",
		SystemLocale:   "zh_CN.UTF-8",
		DialogLanguage: "Chinese (Simplified)",
		Schema:         "workorch.runtime_candidate.test.v1",
		PromptContext: &llmmodels.PromptContext{SchemaVersion: "workorch.runtime_candidate_prompt.v1", Sections: []llmmodels.PromptSection{
			{ID: "C0.runtime-candidate-contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 0, Content: "Return one JSON object."},
			{ID: "C1.goal-dod", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 1, Content: `{"goal":"goal-1","dod":"dod-1"}`},
			{ID: "C3.runtime-authority", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 3, Content: `{"action":"produce_assessment"}`},
		}},
	}
}
