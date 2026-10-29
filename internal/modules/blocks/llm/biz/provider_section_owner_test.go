package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSectionMessagesOwnerPublishesActualPlanAndPreservesCommandIdentity(t *testing.T) {
	var mu sync.Mutex
	var wires []responsesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"test-model","supported_endpoints":["/v1/responses"],"contextWindowTokens":8192,"maxOutputTokens":1024}]}`))
			return
		}
		var wire responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			t.Error(err)
			http.Error(w, "bad request", 400)
			return
		}
		mu.Lock()
		wires = append(wires, wire)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"{\\\"ok\\\":true}\",\"usage\":{\"input_tokens\":100,\"input_tokens_details\":{\"cached_tokens\":0,\"cache_write_tokens\":80},\"output_tokens\":2,\"total_tokens\":102}}}\n\n"))
	}))
	defer server.Close()
	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL = server.URL
	cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "test-model"}
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	owner, err := New(hub, nil, Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return cfg, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Teardown(ctx)
	auditObserver := magicevent.NewSimpleObserver("/llm/consumers/cache-audit-test", hub)
	audits := make(chan llmevents.LLMUsageRecordedData, 2)
	if err := auditObserver.Subscribe(llmevents.TopicLLMUsageRecorded, func(e magicevent.Event, _ magicevent.Result) {
		if data, ok := e.Data().(llmevents.LLMUsageRecordedData); ok {
			audits <- data
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer auditObserver.Unsubscribe(llmevents.TopicLLMUsageRecorded)
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "test-model"}
	request := sectionCacheRequest("ws-owner", "dimension_coverage")
	// A caller cannot supply a successful cache plan, and execution must not
	// mutate a shared caller budget across workspaces or successive commands.
	request.ContextBudget = &llmmodels.ProviderContextBudget{CachePlan: &llmmodels.ProviderCachePlan{CacheKey: "forged"}}
	original := llmevents.CompleteProviderDocumentCommand{RequestIdentity: "reserved", Target: target, ExpectedConfigurationRevision: owner.activeLLMConfigurationRevision, Request: llmevents.ProviderRequestFromModel(request)}
	before := original.Clone()
	originalHash, err := original.RequestHash()
	if err != nil {
		t.Fatal(err)
	}
	for _, document := range []bool{false, true} {
		topic := llmevents.TopicLLMCommandCompleteProvider
		var command any = llmevents.CompleteProviderCommand{Target: target, Request: original.Request, ExpectedConfigurationRevision: owner.activeLLMConfigurationRevision}
		if document {
			topic = llmevents.TopicLLMCommandCompleteProviderDocument
			command = original
		}
		result := hub.Send(magicevent.NewEventWithContext(topic, llmApplicationOwnerSource, llmevents.CommandDestination, magicevent.NewHeader(), ctx, command))
		if result == nil || result.Error() != nil {
			t.Fatalf("result error=%v", result.Error())
		}
		data, err := result.Get()
		if err != nil {
			t.Fatal(err)
		}
		if document {
			response, ok := data.(llmevents.CompleteProviderDocumentResult)
			if !ok || response.ValidateFor(original) != nil || response.RequestHash != originalHash || response.Usage == nil || !response.Usage.CacheWriteTokensKnown {
				t.Fatalf("document identity=%+v", data)
			}
		}
		select {
		case audit := <-audits:
			if audit.Validate() != nil || audit.WorkspaceID != "ws-owner" || audit.ContextBudget.CacheStrategy != "provider_section_messages/v1" || audit.ContextBudget.RenderedStablePrefixHash == "" || len(audit.ContextBudget.SectionHashes) != 4 || len(audit.ContextBudget.IncludedSections) != 4 || audit.ContextBudget.ProviderID != "ai-proxy" || audit.ContextBudget.ActualTokens != 102 || audit.ContextBudget.ActualBytes != audit.RequestBytes || audit.StablePrefixHash == "" || !audit.CacheUsage.CacheReadTokensKnown || !audit.CacheUsage.CacheWriteTokensKnown {
				t.Fatalf("audit lost exact rendered plan: %+v", audit)
			}
		case <-time.After(time.Second):
			t.Fatal("audit missing")
		}
	}
	if !reflect.DeepEqual(original, before) {
		t.Fatal("owner mutated caller command")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(wires) != 2 || wires[0].PromptCacheKey == "" || wires[0].PromptCacheKey == "forged" || wires[0].PromptCacheKey != wires[1].PromptCacheKey {
		t.Fatal("wire key missing or changed between generic/document paths")
	}
}
