package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type countingCandidateProvider struct{ calls atomic.Int32 }

func (p *countingCandidateProvider) GenerateCandidate(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
	p.calls.Add(1)
	return llmmodels.RuntimeCandidateGenerationResult{}, nil
}

func TestProviderRequestAuditPreservesPhysicalModeAcrossTerminalOutcomes(t *testing.T) {
	now := time.Now().UTC()
	metadata := providerExecutionMetadata{
		Target:               runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "chat-model"},
		PhysicalResponseMode: llmaudit.PhysicalResponseModeStreaming,
		Submitted:            true,
	}
	for _, testCase := range []struct {
		name       string
		callErr    error
		wantStatus string
	}{
		{name: "success", wantStatus: llmmodels.RuntimeCandidateStatusCompleted},
		{name: "transport", callErr: context.DeadlineExceeded, wantStatus: llmmodels.RuntimeCandidateStatusFailed},
		{name: "output limit", callErr: &ProviderOutputLimitError{Payload: []byte(`{"partial":`)}, wantStatus: llmmodels.RuntimeCandidateStatusFailed},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			audit := providerCommandAudit(llmmodels.ProviderRequest{}, metadata, 17, 23, now, now, testCase.callErr)
			if audit.PhysicalResponseMode != llmaudit.PhysicalResponseModeStreaming || audit.Status != testCase.wantStatus {
				t.Fatalf("provider request audit = %+v", audit)
			}
		})
	}
}

func TestPublishLLMUsageRecordedRejectsInvalidAuditInsteadOfDroppingSilently(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}

	auditObserver := magicevent.NewSimpleObserver("/llm/consumers/invalid-audit-test", hub)
	audits := make(chan llmevents.LLMUsageRecordedData, 1)
	auditObserver.Subscribe(llmevents.TopicLLMUsageRecorded, func(event magicevent.Event, _ magicevent.Result) {
		if data, ok := event.Data().(llmevents.LLMUsageRecordedData); ok {
			audits <- data
		}
	})
	t.Cleanup(func() { auditObserver.Unsubscribe(llmevents.TopicLLMUsageRecorded) })

	if err := owner.publishLLMUsageRecorded(ctx, nil, nil, "", ""); err == nil {
		t.Fatal("missing LLM usage audit was silently accepted")
	}
	audit := &llmmodels.ProviderRequestAudit{RequestID: "invalid-mode", Status: llmmodels.RuntimeCandidateStatusFailed}
	if err := owner.publishLLMUsageRecorded(ctx, audit, nil, "", ""); err == nil {
		t.Fatal("invalid physical response mode was silently dropped")
	}
	select {
	case recorded := <-audits:
		t.Fatalf("invalid audit was published: %+v", recorded)
	case <-time.After(50 * time.Millisecond):
	}
	audit.PhysicalResponseMode = " non_streaming "
	if err := owner.publishLLMUsageRecorded(ctx, audit, nil, "", ""); err == nil {
		t.Fatal("non-canonical physical response mode was normalized before publication")
	}

	audit.RequestID = "preflight-mode"
	audit.PhysicalResponseMode = llmaudit.PhysicalResponseModeNonStreaming
	if err := owner.publishLLMUsageRecorded(ctx, audit, nil, "", ""); err != nil {
		t.Fatalf("canonical preflight audit publish failed: %v", err)
	}
	select {
	case recorded := <-audits:
		if recorded.ProviderSubmitted || recorded.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming {
			t.Fatalf("recorded preflight audit = %+v", recorded)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canonical preflight audit was not published")
	}

	audit = &llmmodels.ProviderRequestAudit{
		RequestID: "physical-1", LogicalRequestID: "logical-1", PhysicalCallID: "physical-1",
		WorkspaceID: "workspace-1", RunID: "run-1", Kind: "runtime_candidate",
		Target:    runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "chat-model"},
		Operation: "responses", StablePrefixHash: "stable-prefix-1",
		PhysicalResponseMode: llmaudit.PhysicalResponseModeStreaming, ProviderSubmitted: true,
		Status: llmmodels.RuntimeCandidateStatusCompleted, RequestBytes: 32,
		ContextBudget: &llmmodels.ProviderContextBudget{
			ProviderID: "ai-proxy", ModelID: "chat-model", ProviderSubmitted: true,
		},
	}
	if err := owner.publishLLMUsageRecorded(ctx, audit, nil, "", ""); err != nil {
		t.Fatalf("runtime identity audit publish failed: %v", err)
	}
	select {
	case recorded := <-audits:
		if recorded.RequestID != "physical-1" || recorded.LogicalRequestID != "logical-1" || recorded.PhysicalCallID != "physical-1" {
			t.Fatalf("runtime usage identity = %+v", recorded)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runtime usage identity audit was not published")
	}
}

func TestProviderCommandsPublishAuditAndRejectPostBeforePhysicalCall(t *testing.T) {
	ctx := context.Background()
	var embeddingCalls atomic.Int32
	var responseCalls atomic.Int32
	var catalogCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			catalogCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": "list",
				"data": []map[string]any{
					{"id": "chat-model", "supported_endpoints": []string{"/v1/responses"}, "contextWindowTokens": 8192, "maxOutputTokens": 1024},
					{"id": "embedding-model", "supported_endpoints": []string{"/v1/embeddings"}, "contextWindowTokens": 512},
				},
			})
		case "/v1/embeddings":
			embeddingCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}],"usage":{"prompt_tokens":7,"total_tokens":7}}`))
		case "/v1/responses":
			responseCalls.Add(1)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"ok\"}}\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configured := workorchconfig.DefaultLLMConfig()
	configured.AIProxy.BaseURL = server.URL
	configured.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "chat-model"}
	configured.DefaultEmbedding = &workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "embedding-model"}
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	candidateProvider := &countingCandidateProvider{}
	owner, err := New(hub, nil, Dependencies{
		LoadLLMConfig:     func() (workorchconfig.LLMConfig, error) { return configured, nil },
		CandidateProvider: candidateProvider,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	auditObserver := magicevent.NewSimpleObserver("/llm/consumers/provider-command-audit-test", hub)
	audits := make(chan llmevents.LLMUsageRecordedData, 2)
	auditObserver.Subscribe(llmevents.TopicLLMUsageRecorded, func(event magicevent.Event, _ magicevent.Result) {
		if data, ok := event.Data().(llmevents.LLMUsageRecordedData); ok {
			audits <- data
		}
	})
	t.Cleanup(func() { auditObserver.Unsubscribe(llmevents.TopicLLMUsageRecorded) })

	embeddingTarget := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "embedding-model"}
	result := hub.Send(magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandGenerateEmbedding, "application/workorchd", llmevents.CommandDestination,
		magicevent.NewHeader(), ctx, llmevents.GenerateEmbeddingCommand{WorkspaceID: "workspace-1", Target: embeddingTarget, Input: "workspace goal"},
	))
	if result == nil || result.Error() != nil {
		t.Fatalf("embedding result = %+v", result)
	}
	select {
	case audit := <-audits:
		if audit.Operation != workorchconfig.LLMOperationEmbeddings || audit.WorkspaceID != "workspace-1" || audit.RunID != "" || !audit.ProviderSubmitted || audit.RequestBytes == 0 || audit.ResponseBytes == 0 || audit.InputTokens != 7 || audit.TotalTokens != 7 || audit.ContextBudget.ActualTokens != 7 || audit.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming {
			t.Fatalf("embedding audit = %+v", audit)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("embedding call did not publish usage audit")
	}

	hub.Post(magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandGenerateEmbedding, "application/workorchd", llmevents.CommandDestination,
		magicevent.NewHeader(), ctx, llmevents.GenerateEmbeddingCommand{WorkspaceID: "workspace-1", Target: embeddingTarget, Input: "must not execute"},
	))
	hub.Post(magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandCompleteProvider, "application/workorchd", llmevents.CommandDestination,
		magicevent.NewHeader(), ctx, llmevents.CompleteProviderCommand{Request: llmevents.ProviderRequestFromModel(llmmodels.ProviderRequest{Kind: "post-must-not-execute"})},
	))
	hub.Post(magicevent.NewEventWithContext(
		llmevents.TopicRequestRuntimeCandidate, "kernel/orchestrator", llmevents.CommandDestination,
		magicevent.NewHeader(), ctx, llmevents.RequestRuntimeCandidateCommand{},
	))
	hub.Post(magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandRefreshProviderModelCatalog, "application/workorchd", llmevents.CommandDestination,
		magicevent.NewHeader(), ctx, llmevents.RefreshProviderModelCatalogCommand{},
	))
	time.Sleep(100 * time.Millisecond)
	if embeddingCalls.Load() != 1 || responseCalls.Load() != 0 || candidateProvider.calls.Load() != 0 || catalogCalls.Load() != 1 {
		t.Fatalf("Post physical calls: embedding=%d completion=%d candidate=%d catalog=%d", embeddingCalls.Load(), responseCalls.Load(), candidateProvider.calls.Load(), catalogCalls.Load())
	}
}
