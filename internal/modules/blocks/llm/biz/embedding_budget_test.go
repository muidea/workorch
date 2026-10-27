package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

func TestEmbeddingTransportInputAndWireBoundaries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var decoded embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer server.Close()
	envelope, _ := json.Marshal(embeddingRequest{Input: "x"})
	wireBoundaryModel := strings.Repeat("m", embeddingMaxRequestBytes-len(envelope))
	for _, tc := range []struct {
		name, input, model string
		inputBudget        int
		accepted           bool
	}{
		{"unknown exact input", strings.Repeat("x", embeddingMaxInputBytes), "embedding", 0, true},
		{"unknown over input", strings.Repeat("x", embeddingMaxInputBytes+1), "embedding", 0, false},
		{"known exact input", "1234", "embedding", 4, true},
		{"known over input", "12345", "embedding", 4, false},
		{"escaped input", strings.Repeat("\x00", embeddingMaxInputBytes), "embedding", 0, true},
		{"UTF8 input and whitespace", "  中文\n", "embedding", 0, true},
		{"empty", " \n", "embedding", 0, false},
		{"invalid UTF8", string([]byte{0xff}), "embedding", 0, false},
		{"wire exact", "x", wireBoundaryModel, 0, true},
		{"wire over", "x", wireBoundaryModel + "m", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := testAIProxyProviderConfig(server.URL)
			config.ContextProfile = llmmodels.ProviderContextProfile{MaxRequestBytes: tc.inputBudget}
			before := calls.Load()
			result, err := NewHTTPProvider(config).Embed(context.Background(), llmmodels.ProviderEmbeddingRequest{Input: tc.input, Model: tc.model})
			if tc.accepted {
				if err != nil || !result.ProviderSubmitted || result.Input != tc.input || calls.Load() != before+1 || len(result.Vector) != 2 {
					t.Fatalf("accepted=%t submitted=%t inputEqual=%t calls=%d error=%v", tc.accepted, result.ProviderSubmitted, result.Input == tc.input, calls.Load()-before, err)
				}
			} else if err == nil || result.ProviderSubmitted || calls.Load() != before || len(result.Vector) != 0 {
				t.Fatalf("guard failure submitted=%t calls=%d error=%v", result.ProviderSubmitted, calls.Load()-before, err)
			}
		})
	}
}

func TestEmbeddingIndependentResponseBoundAcceptsExactCompleteBody(t *testing.T) {
	const body = `{"data":[{"embedding":[0.1,0.2]}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body + strings.Repeat(" ", embeddingMaxResponseBytes-len(body))))
	}))
	defer server.Close()
	config := testAIProxyProviderConfig(server.URL)
	// A one-byte input budget must not become a vector response size estimate.
	config.ContextProfile = llmmodels.ProviderContextProfile{MaxRequestBytes: 1}
	result, err := NewHTTPProvider(config).Embed(context.Background(), llmmodels.ProviderEmbeddingRequest{Input: "x"})
	if err != nil || result.ResponseBytes != embeddingMaxResponseBytes || len(result.Vector) != 2 {
		t.Fatalf("exact response boundary: bytes=%d vector=%v error=%v", result.ResponseBytes, result.Vector, err)
	}
}

func TestEmbeddingUnknownCapacityPreservesUpstreamRejectionWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Input != "  original\n" || request.Model != "embedding" {
			t.Errorf("original request changed: %+v err=%v", request, err)
		}
		http.Error(w, "upstream input too long", http.StatusRequestEntityTooLarge)
	}))
	defer server.Close()
	config := testAIProxyProviderConfig(server.URL)
	config.ContextProfile = llmmodels.ProviderContextProfile{}
	result, err := NewHTTPProvider(config).Embed(context.Background(), llmmodels.ProviderEmbeddingRequest{Input: "  original\n", Model: "embedding"})
	var statusErr *ProviderHTTPStatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusRequestEntityTooLarge || !result.ProviderSubmitted || len(result.Vector) != 0 || calls.Load() != 1 {
		t.Fatalf("upstream rejection: status=%v submitted=%t calls=%d error=%v", statusErr, result.ProviderSubmitted, calls.Load(), err)
	}
}
