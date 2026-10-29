package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
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
	if !strings.Contains(string(got.Input), "C0.runtime-candidate-contract") || !strings.Contains(string(got.Input), "C3.runtime-authority") {
		t.Fatalf("Responses input = %q, want rendered Runtime prompt sections", got.Input)
	}
	if string(result.Payload) != `{"ok":true}` || result.Usage == nil || result.Usage.TotalTokens != 5 || !result.ProviderSubmitted || result.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming {
		t.Fatalf("result = %+v", result)
	}
}

func TestHTTPProviderNonStreamingCompletedEmptyPreservesUsageWithoutSecondCall(t *testing.T) {
	var calls atomic.Int32
	body := []byte(`{"status":"completed","usage":{"input_tokens":11,"input_tokens_details":{"cached_tokens":3},"output_tokens":7,"output_tokens_details":{"reasoning_tokens":7},"total_tokens":18}}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var got responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got.Stream || r.Header.Get("Accept") != "application/json" {
			t.Fatalf("non-stream request = %+v accept=%q", got, r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	result, err := NewHTTPProvider(testAIProxyProviderConfig(server.URL+"/v1")).Complete(context.Background(), llmProviderRequest())
	var protocolErr *ProviderProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Code != providerFinalOutputMissingCode || protocolErr.ResponseBytes != len(body) {
		t.Fatalf("completed-empty error = %#v (%v)", protocolErr, err)
	}
	if len(result.Payload) != 0 || result.Usage == nil || result.Usage.InputTokens != 11 || result.Usage.CacheReadTokens != 3 ||
		result.Usage.OutputTokens != 7 || result.Usage.TotalTokens != 18 || !result.ProviderSubmitted ||
		result.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming || result.ResponseBytes != len(body) {
		t.Fatalf("completed-empty result = %+v", result)
	}
	if calls.Load() != 1 {
		t.Fatalf("physical provider calls = %d, want 1", calls.Load())
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
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"input_tokens_details\":{\"cached_tokens\":2},\"output_tokens\":2,\"total_tokens\":5}}}\n\n"))
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
	if string(result.Payload) != `{"ok":true}` || result.Usage == nil || result.Usage.TotalTokens != 5 || result.Usage.CacheReadTokens != 2 || !result.ProviderSubmitted || result.PhysicalResponseMode != llmaudit.PhysicalResponseModeStreaming {
		t.Fatalf("result = %+v", result)
	}
	if len(events) != 3 || events[0].Delta != `{"ok":` || events[2].Type != "done" {
		t.Fatalf("events = %+v", events)
	}
}

func TestHTTPProviderStreamReasoningOnlyCompletedKeepsUsageAndMissingFinalOutput(t *testing.T) {
	var calls atomic.Int32
	body := strings.Join([]string{
		`data: {"type":"response.reasoning_text.delta","delta":"private reasoning"}`,
		`data: {"type":"response.reasoning_text.done","text":"private reasoning"}`,
		`data: {"type":"response.content_part.added","part":{"type":"reasoning_text","text":"private reasoning"}}`,
		`data: {"type":"response.content_part.done","part":{"type":"reasoning_text","text":"private reasoning"}}`,
		`data: {"type":"response.output_item.done","item":{"type":"reasoning","content":[{"type":"reasoning_text","text":"private reasoning"}]}}`,
		`data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"reasoning","content":[{"type":"reasoning_text","text":"private reasoning"}]}],"usage":{"input_tokens":48497,"input_tokens_details":{"cached_tokens":48384},"output_tokens":1206,"output_tokens_details":{"reasoning_tokens":1206},"total_tokens":49703}}}`,
	}, "\n\n") + "\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	result, err := NewHTTPProvider(testAIProxyProviderConfig(server.URL+"/v1")).StreamComplete(context.Background(), llmProviderRequest(), nil)
	var protocolErr *ProviderProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Code != providerFinalOutputMissingCode || protocolErr.ResponseBytes != len(body) {
		t.Fatalf("reasoning-only terminal error = %#v (%v)", protocolErr, err)
	}
	if len(result.Payload) != 0 || result.Usage == nil || result.Usage.InputTokens != 48497 ||
		result.Usage.CacheReadTokens != 48384 || result.Usage.OutputTokens != 1206 || result.Usage.TotalTokens != 49703 ||
		!result.ProviderSubmitted || result.PhysicalResponseMode != llmaudit.PhysicalResponseModeStreaming || result.ResponseBytes != len(body) {
		t.Fatalf("reasoning-only terminal result = %+v", result)
	}
	if calls.Load() != 1 {
		t.Fatalf("physical provider calls = %d, want 1", calls.Load())
	}
}

func TestResponsesUsageRejectsInvalidCachedInputTokens(t *testing.T) {
	tests := []struct {
		name  string
		usage responsesUsage
	}{
		{name: "negative", usage: responsesUsage{InputTokens: 3, InputTokensDetails: responsesInputTokenDetails{CachedTokens: -1}}},
		{name: "exceeds input", usage: responsesUsage{InputTokens: 3, InputTokensDetails: responsesInputTokenDetails{CachedTokens: 4}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usage, err := test.usage.ProviderUsage()
			var protocolErr *ProviderProtocolError
			if usage != nil || !errors.As(err, &protocolErr) || protocolErr.Code != "invalid_response_usage" {
				t.Fatalf("usage=%+v error=%v, want invalid_response_usage", usage, err)
			}
		})
	}
}

func TestHTTPProviderRejectsInvalidCachedInputUsageInStreamAndNonStream(t *testing.T) {
	invalid := []struct {
		name   string
		cached int
	}{
		{name: "negative", cached: -1},
		{name: "exceeds input", cached: 4},
	}
	for _, mode := range []string{"non-stream", "stream"} {
		for _, test := range invalid {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				response := responsesResponse{
					Status:     "completed",
					OutputText: `{"ok":true}`,
					Usage: responsesUsage{
						InputTokens:        3,
						InputTokensDetails: responsesInputTokenDetails{CachedTokens: test.cached},
						OutputTokens:       2,
						TotalTokens:        5,
					},
				}
				encoded, err := json.Marshal(response)
				if err != nil {
					t.Fatal(err)
				}
				body := encoded
				contentType := "application/json"
				if mode == "stream" {
					encoded, err = json.Marshal(responsesStreamEvent{Type: "response.completed", Response: &response})
					if err != nil {
						t.Fatal(err)
					}
					body = []byte("data: " + string(encoded) + "\n\n")
					contentType = "text/event-stream"
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", contentType)
					_, _ = w.Write(body)
				}))
				defer server.Close()

				provider := NewHTTPProvider(testAIProxyProviderConfig(server.URL))
				var result llmmodels.ProviderCompletionResult
				if mode == "stream" {
					result, err = provider.StreamComplete(context.Background(), llmProviderRequest(), nil)
				} else {
					result, err = provider.Complete(context.Background(), llmProviderRequest())
				}
				var protocolErr *ProviderProtocolError
				if !errors.As(err, &protocolErr) || protocolErr.Code != "invalid_response_usage" || protocolErr.ResponseBytes != len(body) {
					t.Fatalf("invalid cached usage error = %#v (%v)", protocolErr, err)
				}
				if !result.ProviderSubmitted || result.ResponseBytes != len(body) || len(result.Payload) != 0 || result.Usage != nil {
					t.Fatalf("invalid cached usage result = %+v", result)
				}
				wantMode := llmaudit.PhysicalResponseModeNonStreaming
				if mode == "stream" {
					wantMode = llmaudit.PhysicalResponseModeStreaming
				}
				if result.PhysicalResponseMode != wantMode {
					t.Fatalf("physical response mode = %q, want %q", result.PhysicalResponseMode, wantMode)
				}
			})
		}
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
	if !result.ProviderSubmitted || result.PhysicalResponseMode != llmaudit.PhysicalResponseModeStreaming {
		t.Fatalf("output-limit physical response mode = %q", result.PhysicalResponseMode)
	}
}

func TestHTTPProviderOutputLimitWithoutFinalOutputKeepsTypedStopReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"hidden reasoning consumed the budget\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"usage\":{\"input_tokens\":20,\"output_tokens\":8,\"total_tokens\":28}}}\n\n"))
	}))
	defer server.Close()

	result, err := NewHTTPProvider(testAIProxyProviderConfig(server.URL+"/v1")).StreamComplete(context.Background(), llmProviderRequest(), nil)
	var limitErr *ProviderOutputLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("error = %v, want output limit", err)
	}
	if len(result.Payload) != 0 || len(limitErr.Payload) != 0 {
		t.Fatalf("reasoning-only output limit exposed document payload: result=%q error=%q", result.Payload, limitErr.Payload)
	}
	if result.Usage == nil || result.Usage.OutputTokens != 8 || !result.ProviderSubmitted || result.PhysicalResponseMode != llmaudit.PhysicalResponseModeStreaming {
		t.Fatalf("reasoning-only output-limit result = %+v", result)
	}
}

func TestHTTPProviderNonStreamTerminalStatusFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode string
	}{
		{
			name:     "incomplete non-budget reason",
			body:     `{"status":"incomplete","incomplete_details":{"reason":"content_filter"},"output_text":"{\"ok\":true}","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`,
			wantCode: "response_incomplete_content_filter",
		},
		{
			name:     "completed contradicts incomplete details",
			body:     `{"status":"completed","incomplete_details":{"reason":"max_output_tokens"},"output_text":"{\"ok\":true}","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`,
			wantCode: "response_completed_with_incomplete_details",
		},
		{
			name:     "missing terminal status",
			body:     `{"output_text":"{\"ok\":true}","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`,
			wantCode: "response_terminal_status_missing",
		},
		{
			name:     "unknown terminal status",
			body:     `{"status":"failed","output_text":"{\"ok\":true}","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`,
			wantCode: "response_terminal_status_failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			result, err := NewHTTPProvider(testAIProxyProviderConfig(server.URL+"/v1")).Complete(context.Background(), llmProviderRequest())
			var protocolErr *ProviderProtocolError
			if !errors.As(err, &protocolErr) || protocolErr.Code != test.wantCode || protocolErr.ResponseBytes != len(test.body) {
				t.Fatalf("terminal error = %#v (%v), want code=%q bytes=%d", protocolErr, err, test.wantCode, len(test.body))
			}
			if result.Payload != nil || result.Usage != nil || result.ResponseBytes != len(test.body) {
				t.Fatalf("protocol failure exposed rejected payload/usage: %+v", result)
			}
			var outputLimitErr *ProviderOutputLimitError
			if errors.As(err, &outputLimitErr) {
				t.Fatalf("protocol defect became output-limit outcome: %#v", outputLimitErr)
			}
		})
	}
}

func TestHTTPProviderStreamTerminalStatusFailsClosed(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantCode    string
		wantLimit   bool
		wantPayload string
		wantDone    int
	}{
		{
			name:        "completed envelope reports output limit",
			body:        "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"output_text\":\"{\\\"partial\\\":\",\"usage\":{\"input_tokens\":20,\"output_tokens\":8,\"total_tokens\":28}}}\n\n",
			wantLimit:   true,
			wantPayload: `{"partial":`,
		},
		{
			name:     "done before authoritative terminal",
			body:     "data: [DONE]\n\n",
			wantCode: "stream_done_before_terminal",
		},
		{
			name:        "completed then done",
			body:        "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\ndata: [DONE]\n\n",
			wantPayload: "ok",
			wantDone:    1,
		},
		{
			name:     "completed event has top level incomplete details",
			body:     "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\ndata: {\"type\":\"response.completed\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"response\":{\"status\":\"completed\",\"output_text\":\"partial\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"total_tokens\":5}}}\n\n",
			wantCode: "stream_completed_with_incomplete_details",
		},
		{
			name:     "completed event has empty top level incomplete details",
			body:     "data: {\"type\":\"response.completed\",\"incomplete_details\":{},\"response\":{\"status\":\"completed\",\"output_text\":\"ok\"}}\n\n",
			wantCode: "stream_completed_with_incomplete_details",
		},
		{
			name:     "completed event has null top level incomplete details",
			body:     "data: {\"type\":\"response.completed\",\"incomplete_details\":null,\"response\":{\"status\":\"completed\",\"output_text\":\"ok\"}}\n\n",
			wantCode: "stream_completed_with_incomplete_details",
		},
		{
			name:     "output text done conflicts with accumulated delta",
			body:     "data: {\"type\":\"response.output_text.delta\",\"delta\":\"first\"}\n\ndata: {\"type\":\"response.output_text.done\",\"text\":\"second\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n",
			wantCode: "stream_output_text_done_conflict",
		},
		{
			name:     "completed output conflicts with accumulated delta",
			body:     "data: {\"type\":\"response.output_text.delta\",\"delta\":\"first\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"second\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"total_tokens\":5}}}\n\n",
			wantCode: "stream_completed_output_conflict",
		},
		{
			name:        "done and completed output match accumulated delta",
			body:        "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.output_text.done\",\"text\":\"ok\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"ok\"}}\n\n",
			wantPayload: "ok",
			wantDone:    1,
		},
		{
			name:     "incomplete event contradicts completed response",
			body:     "data: {\"type\":\"response.incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"response\":{\"status\":\"completed\"}}\n\n",
			wantCode: "response_completed_with_incomplete_details",
		},
		{
			name:     "completed event missing response status",
			body:     "data: {\"type\":\"response.completed\",\"response\":{\"output_text\":\"ok\"}}\n\n",
			wantCode: "response_terminal_status_missing",
		},
		{
			name:     "completed event missing response",
			body:     "data: {\"type\":\"response.completed\"}\n\n",
			wantCode: "stream_completed_response_missing",
		},
		{
			name:     "completed event has unknown response status",
			body:     "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"failed\",\"output_text\":\"ok\"}}\n\n",
			wantCode: "response_terminal_status_failed",
		},
		{
			name:     "incomplete reasons conflict",
			body:     "data: {\"type\":\"response.incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"content_filter\"}}}\n\n",
			wantCode: "stream_incomplete_reason_conflict",
		},
		{
			name:     "event after incomplete terminal",
			body:     "data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"late\"}\n\n",
			wantCode: "stream_event_after_terminal",
		},
		{
			name:     "event after completed envelope output limit",
			body:     "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"output_text\":\"partial\"}}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"partial\"}}\n\n",
			wantCode: "stream_event_after_terminal",
		},
		{
			name:     "event after completed terminal",
			body:     "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"late\"}\n\n",
			wantCode: "stream_event_after_terminal",
		},
		{
			name:     "event after done marker",
			body:     "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\ndata: [DONE]\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"late\"}\n\n",
			wantCode: "stream_event_after_terminal",
		},
		{
			name:     "duplicate done marker",
			body:     "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\ndata: [DONE]\n\ndata: [DONE]\n\n",
			wantCode: "stream_duplicate_done",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			events := []llmmodels.ProviderStreamEvent{}
			result, err := NewHTTPProvider(testAIProxyProviderConfig(server.URL+"/v1")).StreamComplete(context.Background(), llmProviderRequest(), func(event llmmodels.ProviderStreamEvent) error {
				events = append(events, event)
				return nil
			})
			if test.wantLimit {
				var outputLimitErr *ProviderOutputLimitError
				if !errors.As(err, &outputLimitErr) || string(result.Payload) != test.wantPayload || string(outputLimitErr.Payload) != test.wantPayload || result.Usage == nil {
					t.Fatalf("output-limit result=%+v error=%#v (%v)", result, outputLimitErr, err)
				}
			} else if test.wantCode != "" {
				var protocolErr *ProviderProtocolError
				if !errors.As(err, &protocolErr) || protocolErr.Code != test.wantCode || protocolErr.ResponseBytes != len(test.body) {
					t.Fatalf("terminal error = %#v (%v), want code=%q bytes=%d", protocolErr, err, test.wantCode, len(test.body))
				}
				if result.Payload != nil || result.Usage != nil {
					t.Fatalf("protocol failure exposed rejected payload/usage: %+v", result)
				}
			} else if err != nil || string(result.Payload) != test.wantPayload {
				t.Fatalf("completed result=%+v error=%v", result, err)
			}
			doneEvents := 0
			for _, event := range events {
				if event.Type == "done" {
					doneEvents++
				}
			}
			if doneEvents != test.wantDone {
				t.Fatalf("done events = %d, want %d; events=%+v", doneEvents, test.wantDone, events)
			}
		})
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
	if result.ProviderID != "ai-proxy" || result.Model != "embedding-model" || len(result.Vector) != 3 || result.Usage == nil || result.Usage.InputTokens != 4 || result.Usage.TotalTokens != 4 || !result.ProviderSubmitted || result.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming {
		t.Fatalf("result = %+v", result)
	}
}

func TestHTTPProviderSubmissionFactUsesExactDoBoundary(t *testing.T) {
	type observation struct {
		submitted    bool
		requestBytes int
		mode         llmaudit.PhysicalResponseMode
	}
	operations := []struct {
		name          string
		submittedMode llmaudit.PhysicalResponseMode
		call          func(*HTTPProvider) (observation, error)
	}{
		{name: "complete", submittedMode: llmaudit.PhysicalResponseModeNonStreaming, call: func(provider *HTTPProvider) (observation, error) {
			result, err := provider.Complete(context.Background(), llmProviderRequest())
			return observation{submitted: result.ProviderSubmitted, requestBytes: result.RequestBytes, mode: result.PhysicalResponseMode}, err
		}},
		{name: "stream", submittedMode: llmaudit.PhysicalResponseModeStreaming, call: func(provider *HTTPProvider) (observation, error) {
			result, err := provider.StreamComplete(context.Background(), llmProviderRequest(), nil)
			return observation{submitted: result.ProviderSubmitted, requestBytes: result.RequestBytes, mode: result.PhysicalResponseMode}, err
		}},
		{name: "embedding", submittedMode: llmaudit.PhysicalResponseModeNonStreaming, call: func(provider *HTTPProvider) (observation, error) {
			result, err := provider.Embed(context.Background(), llmmodels.ProviderEmbeddingRequest{Input: "workspace goal", Model: "embedding-model"})
			return observation{submitted: result.ProviderSubmitted, requestBytes: result.RequestBytes, mode: result.PhysicalResponseMode}, err
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name+"/before Do", func(t *testing.T) {
			var calls atomic.Int32
			config := testAIProxyProviderConfig("://invalid")
			provider := NewHTTPProviderWithClient(config, &http.Client{Transport: llmRoundTripFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, errors.New("transport must not run")
			})})
			result, err := operation.call(provider)
			if err == nil || calls.Load() != 0 || result.submitted || result.mode != llmaudit.PhysicalResponseModeNonStreaming {
				t.Fatalf("pre-dispatch result=%+v calls=%d error=%v", result, calls.Load(), err)
			}
		})
		t.Run(operation.name+"/Do error", func(t *testing.T) {
			var calls atomic.Int32
			config := testAIProxyProviderConfig("http://provider.invalid")
			provider := NewHTTPProviderWithClient(config, &http.Client{Transport: llmRoundTripFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, errors.New("private transport failure")
			})})
			result, err := operation.call(provider)
			if err == nil || calls.Load() != 1 || !result.submitted || result.requestBytes <= 0 || result.mode != operation.submittedMode {
				t.Fatalf("Do-error result=%+v calls=%d error=%v", result, calls.Load(), err)
			}
		})
		t.Run(operation.name+"/response error", func(t *testing.T) {
			var calls atomic.Int32
			config := testAIProxyProviderConfig("http://provider.invalid")
			provider := NewHTTPProviderWithClient(config, &http.Client{Transport: llmRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				calls.Add(1)
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("private response")),
					Request:    request,
				}, nil
			})})
			result, err := operation.call(provider)
			if err == nil || calls.Load() != 1 || !result.submitted || result.requestBytes <= 0 || result.mode != operation.submittedMode {
				t.Fatalf("response-error result=%+v calls=%d error=%v", result, calls.Load(), err)
			}
		})
	}
}

func TestResponsesJSONSchemaRequiresExplicitCapability(t *testing.T) {
	request := llmProviderRequest()
	request.Model = "json-looking-model-name"
	request.Schema = `{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"]}`

	if got := responsesTextForRequest(llmmodels.ProviderCapabilities{}, request); got != nil {
		t.Fatalf("model name enabled JSON schema without capability: %+v", got)
	}
	explicit := responsesTextForRequest(llmmodels.ProviderCapabilities{JSONSchemaResponse: true}, request)
	if explicit == nil || explicit.Format == nil || explicit.Format.Type != "json_schema" || !explicit.Format.Strict {
		t.Fatalf("explicit JSON-schema capability was not honored: %+v", explicit)
	}
	request.RawTextOutput = true
	if got := responsesTextForRequest(llmmodels.ProviderCapabilities{JSONSchemaResponse: true}, request); got != nil {
		t.Fatalf("raw-text request unexpectedly enabled JSON schema: %+v", got)
	}
}

func TestProviderPhysicalResponseModeMustMatchDispatchedPath(t *testing.T) {
	if err := validatePhysicalResponseMode(llmaudit.PhysicalResponseModeStreaming, llmaudit.PhysicalResponseModeStreaming); err != nil {
		t.Fatal(err)
	}
	for _, actual := range []llmaudit.PhysicalResponseMode{"", llmaudit.PhysicalResponseModeNonStreaming} {
		if err := validatePhysicalResponseMode(actual, llmaudit.PhysicalResponseModeStreaming); err == nil {
			t.Fatalf("physical response mode %q unexpectedly matched streaming dispatch", actual)
		}
	}
}

func TestHTTPProviderExplicitJSONSchemaFailureDoesNotFallbackSecondCall(t *testing.T) {
	var calls atomic.Int32
	var got responsesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"completed","output_text":"not-json","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`))
	}))
	defer server.Close()

	config := testAIProxyProviderConfig(server.URL)
	config.Capabilities.Streaming = false
	config.Capabilities.JSONSchemaResponse = true
	request := llmProviderRequest()
	request.Schema = `{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"]}`
	result, err := NewHTTPProvider(config).Complete(context.Background(), request)
	if err != nil {
		t.Fatalf("structured provider response: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("physical provider calls = %d, want exactly one", calls.Load())
	}
	if got.Text == nil || got.Text.Format == nil || got.Text.Format.Type != "json_schema" || !got.Text.Format.Strict {
		t.Fatalf("explicit JSON schema request = %+v", got.Text)
	}
	if string(result.Payload) != "not-json" || result.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming {
		t.Fatalf("provider result = %+v", result)
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
	result, err := NewHTTPProvider(config).Complete(context.Background(), llmProviderRequest())
	var statusErr *ProviderHTTPStatusError
	if !errors.Is(err, common.ErrInvalidArgument) || !errors.As(err, &statusErr) || statusErr.Body != http.StatusText(http.StatusUnauthorized) || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("error = %v, want status-only provider error", err)
	}
	if !result.ProviderSubmitted || result.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming {
		t.Fatalf("transport failure physical response mode = %q", result.PhysicalResponseMode)
	}
}

func TestHTTPProviderStreamWireProgressResetsResponseIdleTimeout(t *testing.T) {
	const heartbeat = ": keepalive\n\n"
	terminal := "data: {\"type\":\"response.output_text.done\",\"text\":\"{\\\"ok\\\":true}\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"{\\\"ok\\\":true}\",\"usage\":{\"input_tokens\":3,\"input_tokens_details\":{\"cached_tokens\":2},\"output_tokens\":2,\"total_tokens\":5}}}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		flusher.Flush()
		for range 6 {
			time.Sleep(40 * time.Millisecond)
			_, _ = w.Write([]byte(heartbeat))
			flusher.Flush()
		}
		_, _ = w.Write([]byte(terminal))
		flusher.Flush()
	}))
	defer server.Close()

	config := testAIProxyProviderConfig(server.URL)
	config.Timeout = 200 * time.Millisecond
	config.HardTimeout = 2 * time.Second
	result, err := NewHTTPProvider(config).StreamComplete(context.Background(), llmProviderRequest(), nil)
	if err != nil {
		t.Fatalf("active stream crossed idle interval: %v", err)
	}
	wantBytes := 6*len(heartbeat) + len(terminal)
	if string(result.Payload) != `{"ok":true}` || result.Usage == nil || result.Usage.CacheReadTokens != 2 ||
		!result.ProviderSubmitted || result.PhysicalResponseMode != llmaudit.PhysicalResponseModeStreaming || result.ResponseBytes != wantBytes {
		t.Fatalf("active stream result = %+v, want responseBytes=%d", result, wantBytes)
	}
}

func TestHTTPProviderStreamResponseHeaderIdleReturnsTypedOwnerFact(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer server.Close()

	config := testAIProxyProviderConfig(server.URL)
	config.Timeout = 80 * time.Millisecond
	config.HardTimeout = 2 * time.Second
	result, err := NewHTTPProvider(config).StreamComplete(context.Background(), llmProviderRequest(), nil)
	var transportErr *ProviderTransportError
	if !errors.As(err, &transportErr) || transportErr.Code != "provider_body_idle_timeout" {
		t.Fatalf("header idle error = %#v (%v)", transportErr, err)
	}
	code, signature := transportErr.RuntimeProviderFailureIdentity()
	if code != "provider_body_idle_timeout" || signature != "provider-body-idle-timeout" {
		t.Fatalf("header idle identity = %q/%q", code, signature)
	}
	if !result.ProviderSubmitted || result.PhysicalResponseMode != llmaudit.PhysicalResponseModeStreaming ||
		result.ResponseBytes != 0 || len(result.Payload) != 0 || result.Usage != nil {
		t.Fatalf("header idle result = %+v", result)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not reach provider")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("header idle did not cancel provider request context")
	}
}

func TestHTTPProviderStreamBodyIdlePreservesWireCountAndClearsPartialResult(t *testing.T) {
	const partial = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"provider-private-partial\"}\n\n"
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(partial))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(cancelled)
	}))
	defer server.Close()

	config := testAIProxyProviderConfig(server.URL)
	config.Timeout = 80 * time.Millisecond
	config.HardTimeout = 2 * time.Second
	result, err := NewHTTPProvider(config).StreamComplete(context.Background(), llmProviderRequest(), nil)
	var transportErr *ProviderTransportError
	if !errors.As(err, &transportErr) || transportErr.Code != "provider_body_idle_timeout" {
		t.Fatalf("body idle error = %#v (%v)", transportErr, err)
	}
	if !result.ProviderSubmitted || result.PhysicalResponseMode != llmaudit.PhysicalResponseModeStreaming ||
		result.ResponseBytes != len(partial) || len(result.Payload) != 0 || result.Usage != nil {
		t.Fatalf("body idle result = %+v", result)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("body idle did not cancel provider request context")
	}
}

func TestHTTPProviderStreamHardAndParentDeadlinesReturnStableTypedOwnerFact(t *testing.T) {
	tests := []struct {
		name          string
		hardTimeout   time.Duration
		parentTimeout time.Duration
	}{
		{name: "hard ceiling", hardTimeout: 140 * time.Millisecond},
		{name: "earlier parent deadline", hardTimeout: 2 * time.Second, parentTimeout: 120 * time.Millisecond},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cancelled := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				_ = r.Body.Close()
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				flusher := w.(http.Flusher)
				flusher.Flush()
				ticker := time.NewTicker(20 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-r.Context().Done():
						close(cancelled)
						return
					case <-ticker.C:
						_, _ = w.Write([]byte(": active\n\n"))
						flusher.Flush()
					}
				}
			}))
			defer server.Close()

			config := testAIProxyProviderConfig(server.URL)
			config.Timeout = 500 * time.Millisecond
			config.HardTimeout = test.hardTimeout
			var ctx context.Context = context.Background()
			cancel := func() {}
			if test.parentTimeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, test.parentTimeout)
			}
			defer cancel()
			result, err := NewHTTPProvider(config).StreamComplete(ctx, llmProviderRequest(), nil)
			var transportErr *ProviderTransportError
			if !errors.As(err, &transportErr) || transportErr.Code != "provider_request_timeout" {
				t.Fatalf("deadline error = %#v (%v)", transportErr, err)
			}
			if !result.ProviderSubmitted || result.PhysicalResponseMode != llmaudit.PhysicalResponseModeStreaming ||
				result.ResponseBytes == 0 || len(result.Payload) != 0 || result.Usage != nil {
				t.Fatalf("deadline result = %+v", result)
			}
			select {
			case <-cancelled:
			case <-time.After(time.Second):
				t.Fatal("deadline did not cancel provider request context")
			}
		})
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
		var transportErr *ProviderTransportError
		if !errors.As(err, &transportErr) || transportErr.Code != "provider_request_timeout" {
			t.Fatalf("error = %#v (%v), want typed deadline", transportErr, err)
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

func TestHTTPProviderRejectsSameOriginRedirectBeforeSecondPhysicalSubmission(t *testing.T) {
	operations := []struct {
		name string
		call func(context.Context, *HTTPProvider) error
	}{
		{
			name: "complete",
			call: func(ctx context.Context, provider *HTTPProvider) error {
				_, err := provider.Complete(ctx, llmProviderRequest())
				return err
			},
		},
		{
			name: "stream",
			call: func(ctx context.Context, provider *HTTPProvider) error {
				_, err := provider.StreamComplete(ctx, llmProviderRequest(), nil)
				return err
			},
		},
		{
			name: "embedding",
			call: func(ctx context.Context, provider *HTTPProvider) error {
				_, err := provider.Embed(ctx, llmmodels.ProviderEmbeddingRequest{Input: "workspace goal", Model: "embedding-model"})
				return err
			},
		},
	}
	for _, status := range []int{
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
	} {
		for _, operation := range operations {
			t.Run(http.StatusText(status)+"/"+operation.name, func(t *testing.T) {
				var physicalCalls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					physicalCalls.Add(1)
					w.Header().Set("Location", r.URL.RequestURI())
					w.WriteHeader(status)
				}))
				defer server.Close()

				var callerPolicyCalls atomic.Int32
				client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
					callerPolicyCalls.Add(1)
					return nil
				}}
				config := testAIProxyProviderConfig(server.URL)
				config.Timeout = time.Second
				provider := NewHTTPProviderWithClient(config, client)
				err := operation.call(context.Background(), provider)
				var protocolErr *ProviderProtocolError
				if !errors.As(err, &protocolErr) || protocolErr.Code != "redirect_rejected" {
					t.Fatalf("redirect error = %#v (%v), want typed at-most-once rejection", protocolErr, err)
				}
				if got := physicalCalls.Load(); got != 1 {
					t.Fatalf("physical submissions = %d, want exactly one", got)
				}
				if got := callerPolicyCalls.Load(); got != 0 {
					t.Fatalf("caller redirect policy calls = %d, want zero because owner rejects first", got)
				}
			})
		}
	}
}

func TestHTTPProviderReportsInvalidJSONAsProtocolWithActualBytes(t *testing.T) {
	const invalid = `{"status":`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
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
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.Repeat("x", limits.MaxNonStreamWireBytes+1)))
	}))
	defer server.Close()
	config.BaseURL = server.URL

	result, err := NewHTTPProvider(config).Complete(context.Background(), llmmodels.ProviderRequest{})
	var protocolErr *ProviderProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Code != "response_byte_limit_exceeded" {
		t.Fatalf("response limit error = %v", err)
	}
	if result.ResponseBytes != limits.MaxNonStreamWireBytes+1 {
		t.Fatalf("bounded response bytes = %d, want %d", result.ResponseBytes, limits.MaxNonStreamWireBytes+1)
	}
	if calls.Load() != 1 {
		t.Fatalf("physical provider calls = %d, want 1 after non-stream guard failure", calls.Load())
	}

	larger := config
	larger.InferenceProfile.MaxOutputTokens = 2
	largerLimits, err := providerCompletionResponseLimits(larger, llmmodels.ProviderRequest{})
	if err != nil || largerLimits.MaxContentBytes <= limits.MaxContentBytes ||
		largerLimits.MaxNonStreamWireBytes <= limits.MaxNonStreamWireBytes ||
		largerLimits.MaxStreamWireBytes <= limits.MaxStreamWireBytes ||
		largerLimits.MaxDataEvents <= limits.MaxDataEvents || largerLimits.MaxLineBytes <= limits.MaxLineBytes {
		t.Fatalf("derived limits did not grow with model output capacity: small=%+v large=%+v err=%v", limits, largerLimits, err)
	}
}

func TestHTTPProviderNonStreamContentLimitMatchesStreamGuard(t *testing.T) {
	config := testAIProxyProviderConfig("")
	config.InferenceProfile.MaxOutputTokens = 4
	config.ContextProfile.EstimatedTokensPerByte = 1
	limits, err := providerCompletionResponseLimits(config, llmmodels.ProviderRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if limits.MaxContentBytes != 4 {
		t.Fatalf("content limit = %d, want 4", limits.MaxContentBytes)
	}

	tests := []struct {
		name       string
		status     string
		content    string
		output     []responsesOutputItem
		incomplete responsesIncompleteDetails
		wantError  bool
	}{
		{name: "completed content exactly limit", status: "completed", content: strings.Repeat("x", limits.MaxContentBytes)},
		{name: "completed content limit plus one", status: "completed", content: strings.Repeat("x", limits.MaxContentBytes+1), wantError: true},
		{name: "trimmed whitespace cannot bypass content limit", status: "completed", content: " " + strings.Repeat("x", limits.MaxContentBytes) + " ", wantError: true},
		{
			name:   "segmented whitespace cannot bypass content limit",
			status: "completed",
			output: []responsesOutputItem{{Content: []responsesContentItem{
				{Type: "output_text", Text: "  "},
				{Type: "output_text", Text: "xxx"},
			}}},
			wantError: true,
		},
		{
			name:       "incomplete max output tokens content limit plus one",
			status:     "incomplete",
			content:    strings.Repeat("x", limits.MaxContentBytes+1),
			incomplete: responsesIncompleteDetails{Reason: "max_output_tokens"},
			wantError:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(responsesResponse{
				Status:            test.status,
				IncompleteDetails: test.incomplete,
				Output:            test.output,
				OutputText:        test.content,
				Usage:             responsesUsage{OutputTokens: len(test.content), TotalTokens: len(test.content)},
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

			providerConfig := config
			providerConfig.BaseURL = server.URL
			result, completionErr := NewHTTPProvider(providerConfig).Complete(context.Background(), llmmodels.ProviderRequest{})
			if calls.Load() != 1 {
				t.Fatalf("physical provider calls = %d, want 1", calls.Load())
			}
			if result.ResponseBytes != len(body) {
				t.Fatalf("response bytes = %d, want factual wire bytes %d", result.ResponseBytes, len(body))
			}
			if !test.wantError {
				if completionErr != nil || string(result.Payload) != test.content || result.Usage == nil {
					t.Fatalf("exact-limit result=%+v error=%v", result, completionErr)
				}
				return
			}
			var protocolErr *ProviderProtocolError
			if !errors.As(completionErr, &protocolErr) || protocolErr.Code != "stream_content_limit_exceeded" || protocolErr.ResponseBytes != len(body) {
				t.Fatalf("content guard result=%+v protocol=%#v error=%v", result, protocolErr, completionErr)
			}
			if result.Payload != nil || result.Usage != nil {
				t.Fatalf("content guard exposed rejected payload/usage: %+v", result)
			}
			var outputLimitErr *ProviderOutputLimitError
			if errors.As(completionErr, &outputLimitErr) {
				t.Fatalf("content guard became continuation outcome: %#v", outputLimitErr)
			}
		})
	}
}

func TestProviderCompletionResponseLimitsRejectStreamEnvelopeOverflow(t *testing.T) {
	config := testAIProxyProviderConfig("")
	config.InferenceProfile.MaxOutputTokens = math.MaxInt/(providerStreamEventsPerOutputToken*providerStreamEventEnvelopeBytes) + 1
	config.ContextProfile.EstimatedTokensPerByte = float64(config.InferenceProfile.MaxOutputTokens)

	_, err := providerCompletionResponseLimits(config, llmmodels.ProviderRequest{})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "stream envelope") {
		t.Fatalf("stream envelope overflow error = %v", err)
	}
}

func TestHTTPProviderStreamAllowsBoundedHighEventEnvelope(t *testing.T) {
	config := testAIProxyProviderConfig("")
	config.InferenceProfile.MaxOutputTokens = 256
	config.ContextProfile.EstimatedTokensPerByte = 1
	limits, err := providerCompletionResponseLimits(config, llmmodels.ProviderRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	for range 1_000 {
		body.WriteString("data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"r\"}\n\n")
	}
	body.WriteString("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n")
	body.WriteString("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	payload := body.String()
	if len(payload) <= limits.MaxNonStreamWireBytes || len(payload) > limits.MaxStreamWireBytes {
		t.Fatalf("high-envelope fixture bytes = %d, non-stream=%d stream=%d", len(payload), limits.MaxNonStreamWireBytes, limits.MaxStreamWireBytes)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()
	config.BaseURL = server.URL

	result, err := NewHTTPProvider(config).StreamComplete(context.Background(), llmmodels.ProviderRequest{}, nil)
	if err != nil {
		t.Fatalf("bounded high-envelope stream: %v", err)
	}
	if string(result.Payload) != "ok" || result.ResponseBytes != len(payload) || calls.Load() != 1 {
		t.Fatalf("high-envelope result=%+v calls=%d", result, calls.Load())
	}
}

func TestHTTPProviderRejectsResponseMediaTypeModeMismatchWithoutSecondCall(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantStream  bool
		wantAccept  string
		body        string
		call        func(*HTTPProvider) (llmmodels.ProviderCompletionResult, error)
	}{
		{
			name:        "non-stream received event stream",
			contentType: "text/event-stream",
			wantAccept:  "application/json",
			body:        "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"ok\"}}\n\n",
			call: func(provider *HTTPProvider) (llmmodels.ProviderCompletionResult, error) {
				return provider.Complete(context.Background(), llmProviderRequest())
			},
		},
		{
			name:        "stream received json",
			contentType: "application/json; charset=utf-8",
			wantStream:  true,
			wantAccept:  "text/event-stream",
			body:        `{"status":"completed","output_text":"ok"}`,
			call: func(provider *HTTPProvider) (llmmodels.ProviderCompletionResult, error) {
				return provider.StreamComplete(context.Background(), llmProviderRequest(), nil)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var request responsesRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if request.Stream != test.wantStream || r.Header.Get("Accept") != test.wantAccept {
					t.Fatalf("declared stream=%v accept=%q, want stream=%v accept=%q", request.Stream, r.Header.Get("Accept"), test.wantStream, test.wantAccept)
				}
				w.Header().Set("Content-Type", test.contentType)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			result, err := test.call(NewHTTPProvider(testAIProxyProviderConfig(server.URL)))
			var protocolErr *ProviderProtocolError
			if !errors.As(err, &protocolErr) || protocolErr.Code != "response_media_type_mismatch" {
				t.Fatalf("media type mismatch error = %#v (%v)", protocolErr, err)
			}
			if calls.Load() != 1 || result.ResponseBytes != 0 {
				t.Fatalf("physical provider calls=%d responseBytes=%d, want one unparsed response", calls.Load(), result.ResponseBytes)
			}
		})
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
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		remaining := limits.MaxStreamWireBytes - 32
		var prefix strings.Builder
		for remaining > 0 {
			lineBytes := min(remaining, 1024)
			if lineBytes == 1 {
				prefix.WriteByte('\n')
			} else {
				prefix.WriteByte(':')
				prefix.WriteString(strings.Repeat("p", lineBytes-2))
				prefix.WriteByte('\n')
			}
			remaining -= lineBytes
		}
		partialEvent := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"unfinished"
		_, _ = w.Write([]byte(prefix.String() + partialEvent))
	}))
	defer server.Close()
	config.BaseURL = server.URL

	result, err := NewHTTPProvider(config).StreamComplete(context.Background(), llmmodels.ProviderRequest{}, nil)
	var protocolErr *ProviderProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Code != "response_byte_limit_exceeded" || result.ResponseBytes != limits.MaxStreamWireBytes+1 {
		t.Fatalf("stream wire limit result=%+v protocol=%#v error=%v", result, protocolErr, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("physical provider calls = %d, want 1 after stream guard failure", calls.Load())
	}
}

func TestHTTPProviderEmbeddingResponseHasIndependentLocalBound(t *testing.T) {
	config := testAIProxyProviderConfig("")
	config.ContextProfile.MaxRequestBytes = 1
	limit, err := providerEmbeddingResponseLimit(config, "x")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
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
