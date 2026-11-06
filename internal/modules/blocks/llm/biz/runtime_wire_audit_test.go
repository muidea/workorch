package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestGenerateCandidateRetainsActualWireAudit(t *testing.T) {
	for _, scenario := range []string{"success", "empty", "limit", "http-error", "config-stale", "cached-zero", "cached-positive"} {
		t.Run(scenario, func(t *testing.T) { checkGenerateCandidateWireAudit(t, scenario) })
	}
}

func checkGenerateCandidateWireAudit(t *testing.T, scenario string) {
	t.Helper()
	calls := 0
	var wire responsesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			t.Error(err)
			http.Error(w, "bad request", 400)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if scenario == "http-error" {
			http.Error(w, "fixed mock provider rejection", http.StatusBadRequest)
			return
		}
		body := `{"type":"response.completed","response":{"status":"completed","output_text":"{}","usage":{"input_tokens":100,"output_tokens":1,"total_tokens":101}}}`
		switch scenario {
		case "empty":
			body = strings.Replace(body, `"output_text":"{}"`, `"output_text":""`, 1)
		case "limit":
			body = strings.Replace(body, `"type":"response.completed"`, `"type":"response.incomplete"`, 1)
			body = strings.Replace(body, `"status":"completed"`, `"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}`, 1)
		case "cached-zero":
			body = strings.Replace(body, `"input_tokens":100`, `"input_tokens":100,"input_tokens_details":{"cached_tokens":0}`, 1)
		case "cached-positive":
			body = strings.Replace(body, `"input_tokens":100`, `"input_tokens":100,"input_tokens_details":{"cached_tokens":64}`, 1)
		}
		_, _ = w.Write([]byte("data: " + body + "\n\n"))
	}))
	defer server.Close()
	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL = server.URL
	cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "model-1"}
	hub := magicevent.NewHub(16)
	ctx := context.Background()
	defer hub.Terminate(ctx)
	owner, err := New(hub, nil, Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return cfg, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Teardown(ctx)
	r, _ := frozenProviderFixture(t)
	r.SemanticContext.ProviderCeiling.ConfigurationRevision = TargetConfigurationRevision(cfg, r.SemanticContext.ProviderCeiling.Selection.Target)
	if scenario == "config-stale" {
		r.SemanticContext.ProviderCeiling.ConfigurationRevision = "different-config"
	}
	p, err := runtimeProviderRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	r.FrozenInput, r.FrozenInputRef, err = freezeRuntimeProviderInput(r, p)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(r)
	got, err := owner.GenerateCandidate(ctx, r)
	if scenario == "config-stale" {
		if err == nil || calls != 0 || got.Audit == nil || got.Audit.ProviderSubmitted || got.Audit.ContextBudget == nil || got.Audit.ContextBudget.WireCachePrefixHash != "" {
			t.Fatal("pre-submit refusal acquired wire/submission facts", err)
		}
		return
	}
	wantError := scenario == "limit" || scenario == "http-error"
	if (err != nil) != wantError || calls != 1 || got.Audit == nil || !got.Audit.ProviderSubmitted || got.ReachedProviderLimit != (scenario == "limit") {
		t.Fatalf("mock submission failed: calls=%d result=%+v err=%v", calls, got, err)
	}
	want := cloneProviderContextBudget(p.ContextBudget)
	observeResponsesCacheWire(want, wire)
	budget := got.Audit.ContextBudget
	if budget == nil || budget.WireCachePrefixHash == "" || budget.WireCacheMode != want.WireCacheMode || budget.WireSchemaHash != want.WireSchemaHash || budget.WireCachePrefixHash != want.WireCachePrefixHash || budget.WireCacheBreakpoints != want.WireCacheBreakpoints {
		t.Fatalf("Runtime audit lost adapter wire observations: got=%+v want=%+v", budget, want)
	}
	after, _ := json.Marshal(r)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("audit mutated frozen input or invented native cache usage")
	}
	if scenario != "http-error" {
		known := scenario == "cached-zero" || scenario == "cached-positive"
		if got.Usage == nil || got.Usage.CacheReadTokensKnown != known || scenario == "cached-positive" && got.Usage.CacheReadTokens != 64 || scenario == "cached-zero" && got.Usage.CacheReadTokens != 0 {
			t.Fatal("native cache known/unknown changed", got.Usage)
		}
	}
	observation := providerContextBudgetAudit(budget).CacheObservation
	if !reflect.DeepEqual(observation.WireCachePrefixes, want.WireCachePrefixes) || len(observation.WireCachePrefixes) < 2 {
		t.Fatal("owner notification lost breakpoint inventory")
	}
	if observation.WireCachePrefixHash != want.WireCachePrefixHash {
		t.Fatal("wire observation lost before owner event projection")
	}
}
