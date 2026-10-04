package llm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

func TestProviderDocumentObservationPreservesWireAndContent(t *testing.T) {
	for _, tc := range []struct {
		name, wire, document string
		status               int
		stream, complete     bool
	}{
		{name: "complete", wire: `{"status":"completed","output_text":"  {}\r\n"}`, document: "  {}\r\n", complete: true},
		{name: "empty", wire: `{"status":"completed","output_text":""}`, complete: true},
		{name: "limit", wire: `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output_text":" {"}`, document: " {"},
		{name: "http", wire: "原始 provider 错误\r\n", status: http.StatusBadRequest},
		{name: "invalid-wire", wire: "{\"output_text\":\"\xff\"}"},
		{name: "surrogate", wire: `{"status":"completed","output_text":"\ud800"}`},
		{name: "stream", stream: true, wire: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"  {}\\n\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"  {}\\n\"}}\n\n", document: "  {}\n", complete: true},
		{name: "stream-partial", stream: true, wire: "data: {\"type\":\"response.output_text.delta\",\"delta\":\" {\"}\n\n", document: " {"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if tc.stream {
					w.Header().Set("Content-Type", "text/event-stream")
				} else {
					w.Header().Set("Content-Type", "application/json")
				}
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				_, _ = io.WriteString(w, tc.wire)
			}))
			defer server.Close()
			provider := NewHTTPProvider(models.ProviderConfig{ID: "mock", BaseURL: server.URL, Model: "model", Timeout: time.Second, Capabilities: models.ProviderCapabilities{Responses: true}, InferenceProfile: models.ProviderInferenceProfile{MaxOutputTokens: 1024}, ContextProfile: models.ProviderContextProfile{ContextWindowTokens: 8192, MaxInputTokens: 7168, ReservedOutputTokens: 1024, MaxRequestBytes: 28672, EstimatedTokensPerByte: 0.25}})
			request := models.ProviderRequest{Model: "model", CaptureDocumentObservation: true}
			var result models.ProviderCompletionResult
			var err error
			if tc.stream {
				result, err = provider.StreamComplete(context.Background(), request, nil)
			} else {
				result, err = provider.Complete(context.Background(), request)
			}
			if calls != 1 || (err == nil) != tc.complete {
				t.Fatalf("calls=%d complete=%v error=%v", calls, tc.complete, err)
			}
			if result.WireObservation == nil || result.WireObservation.Validate() != nil || !result.WireObservation.Complete || !bytes.Equal(result.WireObservation.Content, []byte(tc.wire)) {
				t.Fatalf("wire observation drift: %+v", result.WireObservation)
			}
			if tc.complete || tc.document != "" {
				if result.DocumentObservation == nil || result.DocumentObservation.Validate() != nil || result.DocumentObservation.Complete != tc.complete || string(result.DocumentObservation.Content) != tc.document {
					t.Fatalf("document observation drift: %+v", result.DocumentObservation)
				}
			}
		})
	}
}

func TestProviderDocumentObservationIncludesLimitProbeAndDropsOversizeWhole(t *testing.T) {
	raw := strings.Repeat("x", models.ProviderDocumentObservationLimit+1)
	observed := newProviderDocumentObservationReader(strings.NewReader(raw))
	limited := newProviderResponseLimitReader(observed, models.ProviderDocumentObservationLimit)
	_, err := io.ReadAll(limited)
	if err == nil {
		t.Fatal("wire limit not enforced")
	}
	observation := observed.observation()
	hash := sha256.Sum256([]byte(raw))
	if observation.Validate() != nil || observation.ObservedBytes != uint64(len(raw)) || observation.SHA256 != hex.EncodeToString(hash[:]) || observation.Saved || observation.Content != nil || observation.Complete {
		t.Fatalf("probe/oversize observation invalid: %+v", observation)
	}
}

func TestProviderDocumentTransportLeafPreservesOriginalWithoutRequestWrapper(t *testing.T) {
	leaf := errors.New("原始 transport 错误\r\n\xff")
	wrapped := &url.Error{Op: "Post", URL: "https://synthetic-secret@example.invalid/?apiKey=synthetic-key", Err: leaf}
	classified := providerRequestError(wrapped)
	var transport *ProviderTransportError
	if !errors.As(classified, &transport) || transport.diagnosticCause != leaf {
		t.Fatal("transport leaf was replaced or lost")
	}
	if strings.Contains(classified.Error(), "synthetic") || strings.Contains(classified.Error(), "原始") {
		t.Fatal("ordinary error exposed raw transport/request material")
	}
	observed := models.NewProviderObservedContent([]byte(transport.diagnosticCause.Error()), true, "")
	if observed.Validate() != nil || !bytes.Equal(observed.Content, []byte(leaf.Error())) {
		t.Fatal("transport diagnostic altered invalid UTF-8 bytes")
	}
}

func TestProviderJSONTextLossless(t *testing.T) {
	for _, input := range []string{`{"x":"😀"}`, `{"x":"\\ud800"}`, `{"x":"原文�"}`} {
		if !providerJSONTextLossless([]byte(input)) {
			t.Fatalf("合法无损编码被拒绝: %s", input)
		}
	}
	for _, input := range []string{`{"x":"\ud800"}`, `{"x":"\udc00"}`, `{"x":"\ud800A"}`, string([]byte{'"', 0xff, '"'})} {
		if providerJSONTextLossless([]byte(input)) {
			t.Fatalf("有损编码未拒绝: %q", input)
		}
	}
}
