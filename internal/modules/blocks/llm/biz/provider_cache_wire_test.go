package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

func TestResponsesWireCacheAuditObservesActualAdapterAndFrozenInput(t *testing.T) {
	r, original := frozenProviderFixture(t)
	raw, ref, err := freezeRuntimeProviderInput(r, original)
	if err != nil {
		t.Fatal(err)
	}
	r.FrozenInput, r.FrozenInputRef = raw, ref
	r.SemanticContext.WorkContext.Content.Snapshots[0].Content = "unrelated mutable state"
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			request, err := runtimeProviderRequestForSubmission(r)
			if err != nil {
				t.Fatal(err)
			}
			var wire responsesRequest
			calls := 0
			client := &http.Client{Transport: lifecycleRoundTripper(func(req *http.Request) (*http.Response, error) {
				calls++
				if err := json.NewDecoder(req.Body).Decode(&wire); err != nil {
					return nil, err
				}
				if !samePromptSchemaJSON(wire.Input, request.FrozenSectionMessages) || wire.Stream != stream || wire.Text == nil || wire.Text.Format == nil {
					return nil, fmt.Errorf("outgoing frozen messages/schema lost")
				}
				output := `{"status":"completed","output_text":"{}","usage":{"input_tokens":100,"output_tokens":1,"total_tokens":101}}`
				if stream {
					output = "data: {\"type\":\"response.completed\",\"response\":" + output + "}\n\n"
				}
				media := "application/json"
				if stream {
					media = "text/event-stream"
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(output)), Header: http.Header{"Content-Type": []string{media}}}, nil
			})}
			config := testAIProxyProviderConfig("https://fixture.invalid/v1")
			config.Model = request.Model
			config.Capabilities.JSONSchemaResponse = true
			provider := NewHTTPProviderWithClient(config, client)
			var result models.ProviderCompletionResult
			if stream {
				result, err = provider.StreamComplete(context.Background(), request, nil)
			} else {
				result, err = provider.Complete(context.Background(), request)
			}
			if err != nil {
				t.Fatal(err)
			}
			observation := providerContextBudgetAudit(request.ContextBudget).CacheObservation
			if calls != 1 || observation.WireSchemaHash == "" || observation.WireCachePrefixHash == "" || observation.WireCacheMode != "explicit" || observation.WireCacheBreakpoints < 2 {
				t.Fatalf("actual wire audit absent: %+v", observation)
			}
			if len(observation.WireCachePrefixes) != observation.WireCacheBreakpoints || observation.WireCachePrefixes[len(observation.WireCachePrefixes)-1].Hash != observation.WireCachePrefixHash {
				t.Fatal("transport audit lost exact per-breakpoint inventory")
			}
			payload := observation.Payload()
			if prefix, _ := payload.String("wireCachePrefixHash"); prefix != observation.WireCachePrefixHash {
				t.Fatal("owner projection lost wire fingerprint")
			}
			if effective, _ := payload.String("effectiveHints"); effective != "unknown" || result.Usage == nil || result.Usage.CacheReadTokensKnown {
				t.Fatal("wire hints became remote cache proof")
			}
		})
	}
}

func TestResponsesWireCacheFingerprintSeparatesSchemaFromMessagePrefix(t *testing.T) {
	makeWire := func() responsesRequest {
		r := sectionCacheRequest("ws-a", "current")
		r.PromptContext.Sections[3].ID = "C3.semantic-authority"
		r.PromptContext.Sections[3].Content = `{"exact":"current authority"}`
		r.PromptContext.Sections = append(r.PromptContext.Sections, models.PromptSection{ID: "C4.tail", Role: "user", Content: "attempt zero"})
		return responsesRequest{Model: "model", Input: responsesWireInputForRequest(&r), PromptCacheKey: responsesPromptCacheKey(r), PromptCacheOptions: responsesPromptCacheOptions(r), Reasoning: &responsesReasoning{Effort: "high"}, Text: &responsesTextConfig{Format: &responsesTextFormat{Type: "json_schema", Name: "exact", Strict: true, Schema: map[string]any{"const": "original"}}}}
	}
	observe := func(w responsesRequest) *models.ProviderContextBudget {
		b := &models.ProviderContextBudget{}
		observeResponsesCacheWire(b, w)
		return b
	}
	base := observe(makeWire())
	for _, field := range []string{"tail", "stream", "output", "schema", "model", "reasoning", "authority", "scope"} {
		w := makeWire()
		switch field {
		case "tail", "authority":
			var messages []responsesInputMessage
			_ = json.Unmarshal(w.Input, &messages)
			index := 4
			if field == "authority" {
				index = 3
			}
			messages[index].Content[0].Text += "changed"
			w.Input, _ = json.Marshal(messages)
		case "stream":
			w.Stream = true
		case "output":
			w.MaxOutputTokens = 123
		case "schema":
			w.Text.Format.Schema = map[string]any{"const": "different"}
		case "model":
			w.Model = "different"
		case "reasoning":
			w.Reasoning.Effort = "low"
		case "scope":
			w.PromptCacheKey = "different-workspace"
		}
		got := observe(w)
		wantSame := field == "tail" || field == "stream" || field == "output"
		if (base.WireCachePrefixHash == got.WireCachePrefixHash) != wantSame {
			t.Fatalf("wrong prefix identity for %s", field)
		}
		if (base.WireSchemaHash == got.WireSchemaHash) != (field != "schema") {
			t.Fatalf("schema evidence mixed for %s", field)
		}
	}
	w := makeWire()
	var messages []responsesInputMessage
	_ = json.Unmarshal(w.Input, &messages)
	for i := range messages {
		for j := range messages[i].Content {
			messages[i].Content[j].PromptCacheBreakpoint = nil
		}
	}
	w.Input, _ = json.Marshal(messages)
	got := observe(w)
	if got.WireCachePrefixHash != "" || got.WireCacheBreakpoints != 0 || got.WireSchemaHash == "" {
		t.Fatal("missing breakpoint became observed cached prefix")
	}
	w.PromptCacheOptions = nil
	observeResponsesCacheWire(got, w)
	if got.WireSchemaHash != "" || got.WireCacheMode != "" {
		t.Fatal("stale audit retained for unrequested cache")
	}
}
