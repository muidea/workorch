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
	if gotPayload.Model != "test-model" || len(gotPayload.Messages) < 3 || gotPayload.ResponseFormat["type"] != "json_object" {
		t.Fatalf("payload = %+v, want chat completion json request", gotPayload)
	}
	if gotPayload.MaxCompletionTokens != 65536 ||
		gotPayload.ReasoningEffort != "high" ||
		gotPayload.Thinking == nil ||
		gotPayload.Thinking.BudgetTokens != 32768 {
		t.Fatalf("payload = %+v, want configured inference profile", gotPayload)
	}
	joined := ""
	for _, message := range gotPayload.Messages {
		joined += "\n" + message.Content
	}
	if !strings.Contains(gotPayload.Messages[0].Content, "Do not invent a workorch CLI") ||
		!strings.Contains(gotPayload.Messages[0].Content, "runtimeCapabilities") ||
		!strings.Contains(gotPayload.Messages[0].Content, "dialogLanguage") ||
		!strings.Contains(joined, "Do not use a command named workorch") ||
		!strings.Contains(joined, "runtimeCapabilities.shellPolicy.unavailableCommands") ||
		!strings.Contains(joined, "runtimeCapabilities.serviceAPIs") ||
		!strings.Contains(joined, "Do not assume remote host access credentials") ||
		!strings.Contains(joined, "ssh/command") ||
		!strings.Contains(joined, "timeoutMillis") ||
		!strings.Contains(joined, "Conversation language: Chinese (Simplified)") ||
		!strings.Contains(joined, "DoD summaries") ||
		!strings.Contains(joined, "complete, concrete, and directly applicable") ||
		!strings.Contains(joined, "Do not represent deletion as file_write with empty content") ||
		!strings.Contains(joined, "patch/preview or patch/apply failure") ||
		!strings.Contains(joined, "assessmentOnly") ||
		!strings.Contains(joined, "Graph follow-up is a closure step") ||
		!strings.Contains(joined, "runtimeCapabilities") {
		t.Fatalf("messages = %+v, want runtime capability boundary", gotPayload.Messages)
	}
	if strings.Contains(joined, "An empty workspace file listing is not by itself missing source code") ||
		strings.Contains(joined, "new-work initialization case") ||
		strings.Contains(joined, "A test command that reports no tests") ||
		strings.Contains(joined, "implements the project") ||
		strings.Contains(joined, "go test ./...") ||
		strings.Contains(joined, "affected files") ||
		strings.Contains(joined, "implementation path") {
		t.Fatalf("messages = %+v, should not include software-dev template guidance in the core prompt", gotPayload.Messages)
	}
	if !strings.Contains(string(result.Payload), `"completionState":"completed"`) {
		t.Fatalf("payload = %s, want decision json", result.Payload)
	}
	if result.Usage != nil {
		t.Fatalf("usage = %+v, want nil when provider response has no usage", result.Usage)
	}
}

func TestHTTPProviderAssessmentOnlyPromptHidesExecutionGraphContract(t *testing.T) {
	var gotPayload chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"{\"completionState\":\"failed\",\"dodAssessment\":{\"satisfied\":false,\"summary\":\"needs more evidence\",\"goalOutcome\":\"incomplete\",\"items\":[{\"item\":\"done\",\"state\":\"failed\",\"reason\":\"missing evidence\"}]}}"}}]
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
	request := llmProviderRequest()
	request.Input["assessmentOnly"] = true
	request.RuntimeCapabilities = assessmentOnlyRuntimeCapabilities(request.RuntimeCapabilities)

	_, err := provider.Complete(context.Background(), request)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	joined := ""
	for _, message := range gotPayload.Messages {
		joined += "\n" + message.Content
	}
	if !strings.Contains(joined, "Assessment-only mode is active") ||
		!strings.Contains(joined, "Do not return executionGraph") ||
		!strings.Contains(joined, "Before returning blocked_on_decision") ||
		!strings.Contains(joined, "decision/planning/execution/validation bindings") ||
		!strings.Contains(joined, "later runtime repair or replan") ||
		!strings.Contains(joined, `"type":"assess_only"`) {
		t.Fatalf("messages = %+v, want assessment-only contract", gotPayload.Messages)
	}
	if strings.Contains(joined, "Use executionGraph when") ||
		strings.Contains(joined, `"executionGraph":{"nodes"`) ||
		strings.Contains(joined, `"blockKinds":[`) ||
		strings.Contains(joined, `"serviceAPIs":[`) {
		t.Fatalf("messages = %+v, assessment-only prompt must not expose executable graph contract", gotPayload.Messages)
	}
}

func TestHTTPProviderCompleteIncludesRepairAPIMismatchPriorityContract(t *testing.T) {
	var gotPayload chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"{\"completionState\":\"failed\",\"dodAssessment\":{\"satisfied\":false,\"summary\":\"failed\",\"goalOutcome\":\"incomplete\",\"items\":[]}}"}}]
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
	request := llmProviderRequest()
	request.Input["runtimeRepairFollowUp"] = true
	request.Input["failureTargetRepairContext"] = []map[string]any{{
		"targetRef":        "target_test.go",
		"content":          "package main\n",
		"contentHash":      "hash",
		"contentComplete":  true,
		"safeForFileWrite": true,
		"redacted":         false,
		"truncated":        false,
	}}
	request.Input["failedStepOutput"] = map[string]any{
		"stderr": "./target_test.go:10:3: unknown field Port in struct literal of type Config\n./target_test.go:11:7: undefined: newGatewayHandler\n",
	}

	_, err := provider.Complete(context.Background(), request)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	joined := ""
	for _, message := range gotPayload.Messages {
		joined += "\n" + message.Content
	}
	for _, want := range []string{
		"undefined identifiers",
		"unknown fields",
		"cannot-use type mismatches",
		"resolve those reported blockers",
		"reconcile the target against current visible API/resource definitions first",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("messages = %+v, want repair API mismatch contract %q", gotPayload.Messages, want)
		}
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
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"total_tokens\":18}}\n\n"))
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
	if result.Usage == nil || result.Usage.InputTokens != 11 || result.Usage.OutputTokens != 7 || result.Usage.TotalTokens != 18 {
		t.Fatalf("usage = %+v, want stream usage", result.Usage)
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
		SystemLocale:        "zh_CN.UTF-8",
		DialogLanguage:      "Chinese (Simplified)",
		Input:               map[string]any{"goalId": "goal-1", "doDId": "dod-1"},
		Schema:              decisionSchemaName,
		RuntimeCapabilities: declaredRuntimeCapabilities(),
	}
}
