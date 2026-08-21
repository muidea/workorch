package llm

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

type llmRoundTripFunc func(*http.Request) (*http.Response, error)

func (f llmRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestHTTPProviderTransportErrorsNeverExposeRequestMaterial(t *testing.T) {
	const (
		credential = "credential-should-not-cross-boundary"
		prompt     = "prompt-should-not-cross-boundary"
		endpoint   = "https://endpoint-should-not-cross-boundary.example/private"
		rawDetail  = "raw-transport-detail-should-not-cross-boundary"
	)
	variants := []struct {
		name string
		err  error
		code string
	}{
		{name: "deadline", err: context.DeadlineExceeded, code: "timeout"},
		{name: "cancel", err: context.Canceled, code: "request_canceled"},
		{name: "dns", err: &net.DNSError{Err: rawDetail, Name: endpoint}, code: "dns_resolution_failed"},
		{name: "tls", err: tls.RecordHeaderError{Msg: rawDetail}, code: "tls_verification_failed"},
		{name: "generic", err: errors.New(rawDetail + " " + credential + " " + endpoint), code: "request_failed"},
	}
	operations := []struct {
		name string
		call func(*HTTPProvider) error
	}{
		{name: "complete", call: func(provider *HTTPProvider) error {
			request := llmProviderRequest()
			request.PromptContext.Sections[1].Content = prompt
			_, err := provider.Complete(context.Background(), request)
			return err
		}},
		{name: "stream", call: func(provider *HTTPProvider) error {
			request := llmProviderRequest()
			request.PromptContext.Sections[1].Content = prompt
			_, err := provider.StreamComplete(context.Background(), request, nil)
			return err
		}},
		{name: "embedding", call: func(provider *HTTPProvider) error {
			_, err := provider.Embed(context.Background(), llmmodels.ProviderEmbeddingRequest{Input: prompt, Model: "embedding-model"})
			return err
		}},
	}
	for _, variant := range variants {
		for _, operation := range operations {
			t.Run(operation.name+"/"+variant.name, func(t *testing.T) {
				config := testAIProxyProviderConfig(endpoint)
				config.APIKey = credential
				provider := NewHTTPProviderWithClient(config, &http.Client{Transport: llmRoundTripFunc(func(request *http.Request) (*http.Response, error) {
					return nil, variant.err
				})})
				err := operation.call(provider)
				if err == nil || !strings.Contains(err.Error(), variant.code) {
					t.Fatalf("error = %v, want stable code %q", err, variant.code)
				}
				for _, forbidden := range []string{credential, prompt, endpoint, rawDetail} {
					if strings.Contains(err.Error(), forbidden) {
						t.Fatalf("error leaked %q: %v", forbidden, err)
					}
				}
			})
		}
	}
}

func TestHTTPProviderStatusAndProtocolErrorsNeverExposeRawBody(t *testing.T) {
	const rawBody = "raw-body credential endpoint prompt should remain private"
	operations := []struct {
		name string
		call func(*HTTPProvider) error
	}{
		{name: "complete", call: func(provider *HTTPProvider) error {
			_, err := provider.Complete(context.Background(), llmProviderRequest())
			return err
		}},
		{name: "stream", call: func(provider *HTTPProvider) error {
			_, err := provider.StreamComplete(context.Background(), llmProviderRequest(), nil)
			return err
		}},
		{name: "embedding", call: func(provider *HTTPProvider) error {
			_, err := provider.Embed(context.Background(), llmmodels.ProviderEmbeddingRequest{Input: "safe input", Model: "embedding-model"})
			return err
		}},
	}
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		for _, operation := range operations {
			t.Run(operation.name+"/"+http.StatusText(status), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, rawBody, status)
				}))
				defer server.Close()
				err := operation.call(NewHTTPProvider(testAIProxyProviderConfig(server.URL)))
				var statusErr *ProviderHTTPStatusError
				if !errors.As(err, &statusErr) || statusErr.StatusCode != status || strings.Contains(err.Error(), rawBody) {
					t.Fatalf("status error = %v, want status-only %d", err, status)
				}
			})
		}
	}

	protocolOperations := []struct {
		name        string
		contentType string
		body        string
		call        func(*HTTPProvider) error
	}{
		{name: "complete", contentType: "application/json", body: rawBody, call: operations[0].call},
		{name: "stream", contentType: "text/event-stream", body: "data: " + rawBody + "\n\n", call: operations[1].call},
		{name: "embedding", contentType: "application/json", body: rawBody, call: operations[2].call},
	}
	for _, operation := range protocolOperations {
		t.Run(operation.name+"/malformed-2xx", func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", operation.contentType)
				_, _ = w.Write([]byte(operation.body))
			}))
			defer server.Close()
			err := operation.call(NewHTTPProvider(testAIProxyProviderConfig(server.URL)))
			var protocolErr *ProviderProtocolError
			if !errors.As(err, &protocolErr) || strings.Contains(err.Error(), rawBody) {
				t.Fatalf("protocol error = %v, want bounded typed diagnostic", err)
			}
		})
	}
}
