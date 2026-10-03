package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

func TestExplicitSchemaInvalidZeroHTTP(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	for _, native := range []bool{false, true} {
		for _, schema := range []string{"", "legacy-id", "null", "[]", `"object"`, "{"} {
			config := testAIProxyProviderConfig(server.URL)
			config.Capabilities.JSONSchemaResponse = native
			p := NewHTTPProvider(config)
			request := models.ProviderRequest{Schema: schema, SchemaFormat: models.ProviderSchemaFormatJSONSchema}
			if result, err := p.Complete(context.Background(), request); err == nil || result.ProviderSubmitted {
				t.Fatal("invalid schema submitted")
			}
			if result, err := p.StreamComplete(context.Background(), request, nil); err == nil || result.ProviderSubmitted {
				t.Fatal("invalid streaming schema submitted")
			}
		}
	}
	if calls != 0 {
		t.Fatalf("invalid schema made %d calls", calls)
	}
}

func TestExplicitSchemaProviderErrorDoesNotFallback(t *testing.T) {
	for _, native := range []bool{false, true} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"synthetic unsupported format"}}`))
		}))
		config := testAIProxyProviderConfig(server.URL)
		config.Capabilities.JSONSchemaResponse = native
		request := models.ProviderRequest{Schema: `{"type":"object","additionalProperties":false,"properties":{}}`, SchemaFormat: models.ProviderSchemaFormatJSONSchema}
		if _, err := NewHTTPProvider(config).Complete(context.Background(), request); err == nil {
			t.Fatal("provider rejection accepted")
		}
		server.Close()
		if calls != 1 {
			t.Fatalf("provider error triggered %d calls", calls)
		}
	}
}
