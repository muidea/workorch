package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestHTTPProviderCompleteUsesAIProxyResponses(t *testing.T) {
	var gotPath string
	var gotAuthorization string
	var got responsesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"completed","output_text":"{\"ok\":true}","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`))
	}))
	defer server.Close()

	provider := NewHTTPProvider(testAIProxyProviderConfig(server.URL + "/v1"))
	request := llmProviderRequest()
	request.ReasoningEffort = "high"
	request.MaxOutputTokens = 8
	result, err := provider.Complete(context.Background(), request)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q, want /v1/responses", gotPath)
	}
	if gotAuthorization != "Bearer secret-value" {
		t.Fatalf("authorization = %q, want bearer token", gotAuthorization)
	}
	if got.Model != "test-model" || got.MaxOutputTokens != 8 || got.Reasoning == nil || got.Reasoning.Effort != "high" {
		t.Fatalf("request = %+v", got)
	}
	if !strings.Contains(got.Input, "C0.runtime-candidate-contract") || !strings.Contains(got.Input, "C3.runtime-authority") {
		t.Fatalf("Responses input = %q, want rendered Runtime prompt sections", got.Input)
	}
	if string(result.Payload) != `{"ok":true}` || result.Usage == nil || result.Usage.TotalTokens != 5 {
		t.Fatalf("result = %+v", result)
	}
}

func TestHTTPProviderStreamUsesAIProxyResponses(t *testing.T) {
	var got responsesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q, want /v1/responses", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"hidden reasoning\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"{\\\"ok\\\":\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"true}\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"total_tokens\":5}}}\n\n"))
	}))
	defer server.Close()

	provider := NewHTTPProvider(testAIProxyProviderConfig(server.URL + "/v1"))
	events := []llmmodels.ProviderStreamEvent{}
	result, err := provider.StreamComplete(context.Background(), llmProviderRequest(), func(event llmmodels.ProviderStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("stream complete: %v", err)
	}
	if !got.Stream {
		t.Fatal("stream = false, want true")
	}
	if string(result.Payload) != `{"ok":true}` || result.Usage == nil || result.Usage.TotalTokens != 5 {
		t.Fatalf("result = %+v", result)
	}
	if len(events) != 3 || events[0].Delta != `{"ok":` || events[2].Type != "done" {
		t.Fatalf("events = %+v", events)
	}
}

func TestHTTPProviderOutputLimitPreservesOnlyOutputText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"hidden reasoning\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"{\\\"operations\\\":[\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"usage\":{\"input_tokens\":20,\"output_tokens\":8,\"total_tokens\":28}}}\n\n"))
	}))
	defer server.Close()

	result, err := NewHTTPProvider(testAIProxyProviderConfig(server.URL+"/v1")).StreamComplete(context.Background(), llmProviderRequest(), nil)
	var limitErr *ProviderOutputLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("error = %v, want output limit", err)
	}
	if string(result.Payload) != `{"operations":[` || string(limitErr.Payload) != `{"operations":[` {
		t.Fatalf("partial payload result=%q error=%q", result.Payload, limitErr.Payload)
	}
}

func TestHTTPProviderEmbedsThroughAIProxy(t *testing.T) {
	var gotPath string
	var gotAuthorization string
	var got embeddingRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}],"usage":{"prompt_tokens":4,"total_tokens":4}}`))
	}))
	defer server.Close()

	result, err := NewHTTPProvider(testAIProxyProviderConfig(server.URL+"/v1")).Embed(context.Background(), llmmodels.ProviderEmbeddingRequest{Input: "workspace goal", Model: "embedding-model"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if gotPath != "/v1/embeddings" || gotAuthorization != "Bearer secret-value" || got.Model != "embedding-model" || got.Input != "workspace goal" {
		t.Fatalf("request path=%q authorization=%q payload=%+v", gotPath, gotAuthorization, got)
	}
	if result.ProviderID != "ai-proxy" || result.Model != "embedding-model" || len(result.Vector) != 3 || result.Usage == nil || result.Usage.InputTokens != 4 || result.Usage.TotalTokens != 4 {
		t.Fatalf("result = %+v", result)
	}
}

func TestHTTPProviderOmitsBlankAPIKeyAndRedactsProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("authorization = %q, want omitted", r.Header.Get("Authorization"))
		}
		http.Error(w, "upstream rejected secret-value", http.StatusUnauthorized)
	}))
	defer server.Close()

	config := testAIProxyProviderConfig(server.URL)
	config.APIKey = "  "
	_, err := NewHTTPProvider(config).Complete(context.Background(), llmProviderRequest())
	var statusErr *ProviderHTTPStatusError
	if !errors.Is(err, common.ErrInvalidArgument) || !errors.As(err, &statusErr) || statusErr.Body != http.StatusText(http.StatusUnauthorized) || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("error = %v, want status-only provider error", err)
	}
}

func TestHTTPProviderCancelsConfiguredDeadline(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		// Draining the bounded JSON request lets the transport observe request
		// context cancellation instead of holding the connection on an unread
		// request body.
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer server.Close()

	config := testAIProxyProviderConfig(server.URL)
	config.Timeout = 100 * time.Millisecond
	completion := make(chan error, 1)
	go func() {
		_, err := NewHTTPProvider(config).Complete(context.Background(), llmProviderRequest())
		completion <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not reach server")
	}
	select {
	case err := <-completion:
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "timeout") {
			t.Fatalf("error = %v, want deadline timeout", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request did not return after deadline")
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("request context was not cancelled")
	}
}

func TestHTTPProviderRejectsCrossOriginRedirectWithoutReplayingBody(t *testing.T) {
	var targetCalls int
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalls++
	}))
	defer target.Close()
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL+r.URL.Path, status)
			}))
			defer origin.Close()

			provider := NewHTTPProvider(testAIProxyProviderConfig(origin.URL))
			_, completionErr := provider.Complete(context.Background(), llmProviderRequest())
			_, embeddingErr := provider.Embed(context.Background(), llmmodels.ProviderEmbeddingRequest{Input: "workspace goal", Model: "embedding-model"})
			for _, err := range []error{completionErr, embeddingErr} {
				var protocolErr *ProviderProtocolError
				if !errors.As(err, &protocolErr) || protocolErr.Code != "cross_origin_redirect_rejected" {
					t.Fatalf("redirect error = %v, want typed cross-origin protocol rejection", err)
				}
			}
		})
	}
	if targetCalls != 0 {
		t.Fatalf("redirect target received %d prompt/input bodies, want zero", targetCalls)
	}
}

func TestHTTPProviderReportsInvalidJSONAsProtocolWithActualBytes(t *testing.T) {
	const invalid = `{"status":`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(invalid))
	}))
	defer server.Close()

	result, err := NewHTTPProvider(testAIProxyProviderConfig(server.URL)).Complete(context.Background(), llmProviderRequest())
	var protocolErr *ProviderProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Code != "invalid_response_json" || protocolErr.ResponseBytes != len(invalid) {
		t.Fatalf("protocol error = %#v (%v)", protocolErr, err)
	}
	if result.ResponseBytes != len(invalid) {
		t.Fatalf("response bytes = %d, want %d", result.ResponseBytes, len(invalid))
	}
}

func TestHTTPProviderResponseLimitsDeriveFromResolvedProfile(t *testing.T) {
	config := testAIProxyProviderConfig("")
	config.InferenceProfile.MaxOutputTokens = 1
	config.ContextProfile.EstimatedTokensPerByte = 1
	limits, err := providerCompletionResponseLimits(config, llmmodels.ProviderRequest{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", limits.MaxWireBytes+1)))
	}))
	defer server.Close()
	config.BaseURL = server.URL

	result, err := NewHTTPProvider(config).Complete(context.Background(), llmmodels.ProviderRequest{})
	var protocolErr *ProviderProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Code != "response_byte_limit_exceeded" {
		t.Fatalf("response limit error = %v", err)
	}
	if result.ResponseBytes != limits.MaxWireBytes+1 {
		t.Fatalf("bounded response bytes = %d, want %d", result.ResponseBytes, limits.MaxWireBytes+1)
	}

	larger := config
	larger.InferenceProfile.MaxOutputTokens = 2
	largerLimits, err := providerCompletionResponseLimits(larger, llmmodels.ProviderRequest{})
	if err != nil || largerLimits.MaxContentBytes <= limits.MaxContentBytes || largerLimits.MaxDataEvents <= limits.MaxDataEvents {
		t.Fatalf("derived limits did not grow with model output capacity: small=%+v large=%+v err=%v", limits, largerLimits, err)
	}
}

func TestHTTPProviderStreamContentLimitDerivesFromOutputTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"abc\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"))
	}))
	defer server.Close()
	config := testAIProxyProviderConfig(server.URL)
	config.InferenceProfile.MaxOutputTokens = 2
	config.ContextProfile.EstimatedTokensPerByte = 1

	_, err := NewHTTPProvider(config).StreamComplete(context.Background(), llmmodels.ProviderRequest{}, nil)
	var protocolErr *ProviderProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Code != "stream_content_limit_exceeded" {
		t.Fatalf("stream content limit error = %v", err)
	}
}

func TestHTTPProviderMalformedStreamEventIsProtocolFailure(t *testing.T) {
	const malformed = `{"type":`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: " + malformed + "\n\n"))
	}))
	defer server.Close()

	_, err := NewHTTPProvider(testAIProxyProviderConfig(server.URL)).StreamComplete(context.Background(), llmmodels.ProviderRequest{}, nil)
	var protocolErr *ProviderProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Code != "invalid_stream_event_json" || protocolErr.ResponseBytes == 0 {
		t.Fatalf("malformed stream error = %#v (%v)", protocolErr, err)
	}
}

func TestHTTPProviderStreamWireLimitPrecedesPartialEventJSON(t *testing.T) {
	config := testAIProxyProviderConfig("")
	config.InferenceProfile.MaxOutputTokens = 1
	config.ContextProfile.EstimatedTokensPerByte = 1
	limits, err := providerCompletionResponseLimits(config, llmmodels.ProviderRequest{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		prefix := ": " + strings.Repeat("p", limits.MaxWireBytes/2) + "\n"
		remaining := limits.MaxWireBytes - len(prefix)
		partialEvent := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"" + strings.Repeat("x", remaining) + "\"}\n\n"
		_, _ = w.Write([]byte(prefix + partialEvent))
	}))
	defer server.Close()
	config.BaseURL = server.URL

	result, err := NewHTTPProvider(config).StreamComplete(context.Background(), llmmodels.ProviderRequest{}, nil)
	var protocolErr *ProviderProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Code != "response_byte_limit_exceeded" || result.ResponseBytes != limits.MaxWireBytes+1 {
		t.Fatalf("stream wire limit result=%+v protocol=%#v error=%v", result, protocolErr, err)
	}
}

func TestHTTPProviderEmbeddingResponseIsBoundedByModelInputProfile(t *testing.T) {
	config := testAIProxyProviderConfig("")
	config.ContextProfile.MaxRequestBytes = 1
	limit, err := providerEmbeddingResponseLimit(config, "x")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("0", limit+1)))
	}))
	defer server.Close()
	config.BaseURL = server.URL

	result, err := NewHTTPProvider(config).Embed(context.Background(), llmmodels.ProviderEmbeddingRequest{Input: "x", Model: "embedding-model"})
	var protocolErr *ProviderProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Code != "response_byte_limit_exceeded" || result.ResponseBytes != limit+1 {
		t.Fatalf("embedding limit result=%+v error=%v", result, err)
	}
}

func TestAIProxyURLsNormalizeHostOnlyBase(t *testing.T) {
	responses, err := responsesURL("http://127.0.0.1:8080")
	if err != nil || responses != "http://127.0.0.1:8080/v1/responses" {
		t.Fatalf("responses URL = %q, %v", responses, err)
	}
	embeddings, err := embeddingsURL("http://127.0.0.1:8080")
	if err != nil || embeddings != "http://127.0.0.1:8080/v1/embeddings" {
		t.Fatalf("embeddings URL = %q, %v", embeddings, err)
	}
}

func testAIProxyProviderConfig(baseURL string) llmmodels.ProviderConfig {
	return llmmodels.ProviderConfig{
		ID:      "ai-proxy",
		BaseURL: baseURL,
		APIKey:  "secret-value",
		Model:   "test-model",
		Timeout: 5 * time.Second,
		Capabilities: llmmodels.ProviderCapabilities{
			Responses:         true,
			Streaming:         true,
			NonChatOperations: []string{"embeddings"},
		},
		InferenceProfile: llmmodels.ProviderInferenceProfile{MaxOutputTokens: 1024},
		ContextProfile: llmmodels.ProviderContextProfile{
			ContextWindowTokens: 8192, MaxInputTokens: 7168, ReservedOutputTokens: 1024,
			MaxRequestBytes: 28672, EstimatedTokensPerByte: 0.25,
		},
	}
}

func llmProviderRequest() llmmodels.ProviderRequest {
	return llmmodels.ProviderRequest{
		WorkspaceID: "workspace-1",
		RunID:       "run-1",
		StepID:      "run-1/step-1",
		Kind:        "runtime_candidate",
		Model:       "test-model",
		PromptContext: &llmmodels.PromptContext{SchemaVersion: "workorch.runtime_candidate_prompt", Sections: []llmmodels.PromptSection{
			{ID: "C0.runtime-candidate-contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 0, Content: "Return one JSON object."},
			{ID: "C3.runtime-authority", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 3, Content: `{"action":"produce_assessment"}`},
		}},
	}
}
