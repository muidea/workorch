package llm

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
)

func TestHTTPProviderRejectsInvalidCompletionUsageWithoutExposingResult(t *testing.T) {
	tests := []struct {
		name  string
		usage responsesUsage
	}{
		{name: "negative input", usage: responsesUsage{InputTokens: -1}},
		{name: "negative output", usage: responsesUsage{OutputTokens: -1}},
		{name: "negative total", usage: responsesUsage{TotalTokens: -1}},
		{name: "derived total overflow", usage: responsesUsage{InputTokens: math.MaxInt, OutputTokens: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(responsesResponse{Status: "completed", OutputText: "provider-private-output", Usage: test.usage})
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(body)
			}))
			defer server.Close()

			result, completionErr := NewHTTPProvider(testAIProxyProviderConfig(server.URL)).Complete(context.Background(), llmProviderRequest())
			assertProviderProtocolResultRejected(t, completionErr, "invalid_response_usage", len(body), result.Payload, result.Usage)
			if calls.Load() != 1 {
				t.Fatalf("physical provider calls = %d, want 1", calls.Load())
			}
		})
	}
}

func TestResponsesUsageDerivedTotalAcceptsExactIntBoundary(t *testing.T) {
	usage, err := (responsesUsage{InputTokens: math.MaxInt - 1, OutputTokens: 1}).ProviderUsage()
	if err != nil || usage == nil || usage.TotalTokens != math.MaxInt {
		t.Fatalf("exact derived total usage=%+v error=%v", usage, err)
	}
}

func TestHTTPProviderRejectsInvalidStreamUsageWithoutExposingResult(t *testing.T) {
	tests := []struct {
		name  string
		usage responsesUsage
	}{
		{name: "negative", usage: responsesUsage{OutputTokens: -1}},
		{name: "derived total overflow", usage: responsesUsage{InputTokens: math.MaxInt, OutputTokens: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event, err := json.Marshal(responsesStreamEvent{
				Type: "response.completed",
				Response: &responsesResponse{
					Status:     "completed",
					OutputText: "provider-private-output",
					Usage:      test.usage,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			body := []byte("data: " + string(event) + "\n\n")
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write(body)
			}))
			defer server.Close()

			result, completionErr := NewHTTPProvider(testAIProxyProviderConfig(server.URL)).StreamComplete(context.Background(), llmProviderRequest(), nil)
			assertProviderProtocolResultRejected(t, completionErr, "invalid_response_usage", len(body), result.Payload, result.Usage)
			if calls.Load() != 1 {
				t.Fatalf("physical provider calls = %d, want 1", calls.Load())
			}
		})
	}
}

func TestHTTPProviderRejectsInvalidEmbeddingUsageWithoutExposingResult(t *testing.T) {
	tests := []struct {
		name  string
		usage embeddingUsage
	}{
		{name: "negative prompt", usage: embeddingUsage{PromptTokens: -1}},
		{name: "negative input", usage: embeddingUsage{InputTokens: -1}},
		{name: "negative total", usage: embeddingUsage{TotalTokens: -1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(embeddingResponse{
				Data: []struct {
					Embedding []float64 `json:"embedding"`
				}{{Embedding: []float64{0.1, 0.2}}},
				Usage: test.usage,
			})
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(body)
			}))
			defer server.Close()

			result, embeddingErr := NewHTTPProvider(testAIProxyProviderConfig(server.URL)).Embed(context.Background(), llmmodels.ProviderEmbeddingRequest{Input: "workspace goal", Model: "embedding-model"})
			var protocolErr *ProviderProtocolError
			if !errors.As(embeddingErr, &protocolErr) || protocolErr.Code != "invalid_embedding_usage" || protocolErr.ResponseBytes != len(body) {
				t.Fatalf("embedding protocol error = %#v (%v)", protocolErr, embeddingErr)
			}
			if result.ResponseBytes != len(body) || result.Usage != nil || result.Vector != nil {
				t.Fatalf("invalid usage exposed embedding result: %+v", result)
			}
			if calls.Load() != 1 {
				t.Fatalf("physical provider calls = %d, want 1", calls.Load())
			}
		})
	}
}

func TestProviderResponseJSONMediaTypeValidation(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantError   bool
	}{
		{name: "application json", contentType: "application/json"},
		{name: "application structured json", contentType: "application/problem+json; charset=utf-8"},
		{name: "missing", wantError: true},
		{name: "malformed", contentType: "application/json; charset", wantError: true},
		{name: "non application structured json", contentType: "text/problem+json", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := make(http.Header)
			if test.contentType != "" {
				header.Set("Content-Type", test.contentType)
			}
			err := validateProviderResponseMediaType(header, providerResponseMediaTypeJSON)
			if !test.wantError {
				if err != nil {
					t.Fatalf("media type rejected: %v", err)
				}
				return
			}
			var protocolErr *ProviderProtocolError
			if !errors.As(err, &protocolErr) || protocolErr.Code != "response_media_type_mismatch" {
				t.Fatalf("media type error = %#v (%v)", protocolErr, err)
			}
		})
	}
}

func TestHTTPProviderEmbeddingMediaTypeGuardAndStructuredJSON(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantError   bool
	}{
		{name: "structured json", contentType: "application/vnd.workorch.embedding+json; charset=utf-8"},
		{name: "event stream mismatch", contentType: "text/event-stream", wantError: true},
		{name: "missing", wantError: true},
		{name: "malformed", contentType: "application/json; charset", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				if test.contentType != "" {
					w.Header().Set("Content-Type", test.contentType)
				}
				_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1]}],"usage":{"input_tokens":1,"total_tokens":1}}`))
			}))
			defer server.Close()

			result, err := NewHTTPProvider(testAIProxyProviderConfig(server.URL)).Embed(context.Background(), llmmodels.ProviderEmbeddingRequest{Input: "x", Model: "embedding-model"})
			if calls.Load() != 1 {
				t.Fatalf("physical provider calls = %d, want 1", calls.Load())
			}
			if !test.wantError {
				if err != nil || len(result.Vector) != 1 {
					t.Fatalf("structured JSON result=%+v error=%v", result, err)
				}
				return
			}
			var protocolErr *ProviderProtocolError
			if !errors.As(err, &protocolErr) || protocolErr.Code != "response_media_type_mismatch" || result.ResponseBytes != 0 || result.Vector != nil || result.Usage != nil {
				t.Fatalf("media type result=%+v protocol=%#v error=%v", result, protocolErr, err)
			}
		})
	}
}

func TestResponsesStreamEventLimitExactAndPlusOne(t *testing.T) {
	event := func(value responsesStreamEvent) string {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return "data: " + string(encoded) + "\n\n"
	}
	completed := responsesStreamEvent{Type: "response.completed", Response: &responsesResponse{Status: "completed"}}
	limits := providerStreamLimits{MaxContentBytes: 2, MaxDataEvents: 2, MaxLineBytes: 4096}
	exact := event(responsesStreamEvent{Type: "response.output_text.delta", Delta: "ok"}) + event(completed)
	content, usage, err := readResponsesStream(strings.NewReader(exact), nil, limits)
	if err != nil || content != "ok" || usage != nil {
		t.Fatalf("exact event limit content=%q usage=%+v error=%v", content, usage, err)
	}

	plusOne := event(responsesStreamEvent{Type: "response.reasoning_summary_text.delta", Delta: "ignored"}) +
		event(responsesStreamEvent{Type: "response.reasoning_summary_text.delta", Delta: "ignored"}) + event(completed)
	content, usage, err = readResponsesStream(strings.NewReader(plusOne), nil, limits)
	var protocolErr *ProviderProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Code != "stream_event_limit_exceeded" || content != "" || usage != nil {
		t.Fatalf("event limit+1 content=%q usage=%+v protocol=%#v error=%v", content, usage, protocolErr, err)
	}
}

func TestResponsesStreamLineLimitExactAndPlusOne(t *testing.T) {
	event, err := json.Marshal(responsesStreamEvent{
		Type:     "response.completed",
		Response: &responsesResponse{Status: "completed", OutputText: "ok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	line := "data: " + string(event)
	limits := providerStreamLimits{MaxContentBytes: 2, MaxDataEvents: 1, MaxLineBytes: len(line)}
	content, usage, err := readResponsesStream(strings.NewReader(line+"\n"), nil, limits)
	if err != nil || content != "ok" || usage != nil {
		t.Fatalf("exact line limit content=%q usage=%+v error=%v", content, usage, err)
	}

	content, usage, err = readResponsesStream(strings.NewReader(line+" \n"), nil, limits)
	var protocolErr *ProviderProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Code != "stream_line_limit_exceeded" || content != "" || usage != nil {
		t.Fatalf("line limit+1 content=%q usage=%+v protocol=%#v error=%v", content, usage, protocolErr, err)
	}
}

func assertProviderProtocolResultRejected(t *testing.T, err error, code string, responseBytes int, payload []byte, usage *llmaudit.ProviderUsage) {
	t.Helper()
	var protocolErr *ProviderProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Code != code || protocolErr.ResponseBytes != responseBytes {
		t.Fatalf("protocol error = %#v (%v), want code=%q bytes=%d", protocolErr, err, code, responseBytes)
	}
	if payload != nil || usage != nil {
		t.Fatalf("protocol failure exposed payload=%q usage=%+v", payload, usage)
	}
}
