package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
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

func TestRuntimeCandidateReasoningOnlyOutputLimitPublishesSingleIncompleteDocumentFact(t *testing.T) {
	ctx := context.Background()
	var responseCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		responseCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"hidden reasoning consumed the budget\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"usage\":{\"input_tokens\":20,\"output_tokens\":8,\"total_tokens\":28}}}\n\n"))
	}))
	defer server.Close()

	configured := workorchconfig.DefaultLLMConfig()
	configured.AIProxy.BaseURL = server.URL
	configured.AIProxy.APIKey = "test-key"
	configured.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "model-1"}

	request := runtimeSemanticProviderTestRequest(t)
	request.SemanticContext.ProviderCeiling.ConfigurationRevision = TargetConfigurationRevision(configured, request.SemanticContext.ProviderCeiling.Selection.Target)
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion:            runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:             request.DocumentKind,
		PlanStep:                 request.SemanticContext.PlanStep,
		ManifestHash:             request.SemanticManifest.Hash,
		DefinitionSeedHash:       request.SemanticManifest.DefinitionSeedHash,
		LogicalRequestID:         request.RequestIdentity.LogicalRequestID,
		StablePrefixHash:         request.StablePrefixHash,
		BatchID:                  request.SemanticContext.BatchID,
		BatchSequence:            request.SemanticContext.BatchSequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.SemanticContext.BatchCoverageWorkItemIDs...),
		CoverageOrderHash:        "coverage-order-test",
		MaxOutputTokens:          request.SemanticContext.ProviderCeiling.EffectiveMaxOutputTokens,
		MaxRecoveryAttempts:      2,
	}
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	request.ExpectedDocument = &expected

	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil, Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) {
		return configured, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	auditObserver := magicevent.NewSimpleObserver("/llm/consumers/reasoning-only-output-limit-test", hub)
	audits := make(chan llmevents.LLMUsageRecordedData, 2)
	auditObserver.Subscribe(llmevents.TopicLLMUsageRecorded, func(event magicevent.Event, _ magicevent.Result) {
		if data, ok := event.Data().(llmevents.LLMUsageRecordedData); ok {
			audits <- data
		}
	})
	t.Cleanup(func() { auditObserver.Unsubscribe(llmevents.TopicLLMUsageRecorded) })

	command := llmevents.RequestRuntimeCandidateCommand{
		WorkspaceID: request.SemanticContext.WorkspaceID, RunID: request.SemanticContext.RunID,
		DocumentKind: request.DocumentKind, RequestIdentity: request.RequestIdentity, ExpectedDocument: request.ExpectedDocument,
		StablePrefixHash: request.StablePrefixHash, SemanticManifest: request.SemanticManifest, SemanticContext: request.SemanticContext,
	}
	magicResult := hub.Send(magicevent.NewEventWithContext(
		llmevents.TopicRequestRuntimeCandidate, runtimecontract.RuntimeModuleOrchestrator, llmevents.CommandDestination,
		magicevent.NewHeader(), ctx, command,
	))
	if magicResult == nil {
		t.Fatal("runtime candidate command returned no result")
	}
	data, requestErr := magicResult.Get()
	if requestErr != nil {
		t.Fatalf("runtime candidate command: %v", requestErr)
	}
	response, ok := data.(llmevents.RequestRuntimeCandidateResult)
	if !ok {
		t.Fatalf("runtime candidate result type = %T", data)
	}
	if err := response.Validate(); err != nil {
		t.Fatalf("runtime candidate result validation: %v", err)
	}
	if response.Feedback != nil || response.Stale != nil || response.DocumentOutcome == nil {
		t.Fatalf("runtime candidate result is not one exclusive document fact: %+v", response)
	}
	framing := response.DocumentOutcome.Framing
	if framing.Disposition != runtimecontract.RuntimeProviderDocumentIncomplete ||
		framing.Code != "max_output_tokens" || framing.Signature != "provider-output-limit" ||
		framing.ResponseHash != "" || framing.ResponseBytes != 0 || response.DocumentOutcome.Content != nil {
		t.Fatalf("reasoning-only document outcome = %+v", response.DocumentOutcome)
	}
	if response.Audit == nil || !response.Audit.ProviderSubmitted ||
		response.Audit.Status != llmmodels.RuntimeCandidateStatusCompleted || response.Audit.FailureClass != "" {
		t.Fatalf("reasoning-only request audit = %+v", response.Audit)
	}
	if response.Usage == nil || response.Usage.InputTokens != 20 || response.Usage.OutputTokens != 8 || response.Usage.TotalTokens != 28 {
		t.Fatalf("reasoning-only usage = %+v", response.Usage)
	}
	if responseCalls.Load() != 1 {
		t.Fatalf("physical response calls = %d, want 1", responseCalls.Load())
	}

	select {
	case audit := <-audits:
		if !audit.ProviderSubmitted || audit.Status != llmmodels.RuntimeCandidateStatusCompleted || audit.FailureClass != "" {
			t.Fatalf("published reasoning-only usage audit = %+v", audit)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reasoning-only candidate did not publish usage audit")
	}
	select {
	case audit := <-audits:
		t.Fatalf("reasoning-only candidate published duplicate usage audit: %+v", audit)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRuntimeCandidateReasoningOnlyCompletedPublishesSingleEmptyDocumentFact(t *testing.T) {
	ctx := context.Background()
	var responseCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		responseCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"hidden reasoning consumed every output token\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":20,\"input_tokens_details\":{\"cached_tokens\":2},\"output_tokens\":8,\"output_tokens_details\":{\"reasoning_tokens\":8},\"total_tokens\":28}}}\n\n"))
	}))
	defer server.Close()

	configured := workorchconfig.DefaultLLMConfig()
	configured.AIProxy.BaseURL = server.URL
	configured.AIProxy.APIKey = "test-key"
	configured.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "model-1"}

	request := runtimeSemanticProviderTestRequest(t)
	request.SemanticContext.ProviderCeiling.ConfigurationRevision = TargetConfigurationRevision(configured, request.SemanticContext.ProviderCeiling.Selection.Target)
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion:            runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:             request.DocumentKind,
		PlanStep:                 request.SemanticContext.PlanStep,
		ManifestHash:             request.SemanticManifest.Hash,
		DefinitionSeedHash:       request.SemanticManifest.DefinitionSeedHash,
		LogicalRequestID:         request.RequestIdentity.LogicalRequestID,
		StablePrefixHash:         request.StablePrefixHash,
		BatchID:                  request.SemanticContext.BatchID,
		BatchSequence:            request.SemanticContext.BatchSequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.SemanticContext.BatchCoverageWorkItemIDs...),
		CoverageOrderHash:        "coverage-order-test",
		MaxOutputTokens:          request.SemanticContext.ProviderCeiling.EffectiveMaxOutputTokens,
		MaxRecoveryAttempts:      2,
	}
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	request.ExpectedDocument = &expected

	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil, Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) {
		return configured, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	auditObserver := magicevent.NewSimpleObserver("/llm/consumers/reasoning-only-completed-test", hub)
	audits := make(chan llmevents.LLMUsageRecordedData, 2)
	auditObserver.Subscribe(llmevents.TopicLLMUsageRecorded, func(event magicevent.Event, _ magicevent.Result) {
		if data, ok := event.Data().(llmevents.LLMUsageRecordedData); ok {
			audits <- data
		}
	})
	t.Cleanup(func() { auditObserver.Unsubscribe(llmevents.TopicLLMUsageRecorded) })

	command := llmevents.RequestRuntimeCandidateCommand{
		WorkspaceID: request.SemanticContext.WorkspaceID, RunID: request.SemanticContext.RunID,
		DocumentKind: request.DocumentKind, RequestIdentity: request.RequestIdentity, ExpectedDocument: request.ExpectedDocument,
		StablePrefixHash: request.StablePrefixHash, SemanticManifest: request.SemanticManifest, SemanticContext: request.SemanticContext,
	}
	magicResult := hub.Send(magicevent.NewEventWithContext(
		llmevents.TopicRequestRuntimeCandidate, runtimecontract.RuntimeModuleOrchestrator, llmevents.CommandDestination,
		magicevent.NewHeader(), ctx, command,
	))
	if magicResult == nil {
		t.Fatal("runtime candidate command returned no result")
	}
	data, requestErr := magicResult.Get()
	if requestErr != nil {
		t.Fatalf("runtime candidate command: %v", requestErr)
	}
	response, ok := data.(llmevents.RequestRuntimeCandidateResult)
	if !ok {
		t.Fatalf("runtime candidate result type = %T", data)
	}
	if err := response.Validate(); err != nil {
		t.Fatalf("runtime candidate result validation: %v", err)
	}
	if response.Feedback != nil || response.Stale != nil || response.DocumentOutcome == nil {
		t.Fatalf("runtime candidate result is not one exclusive document fact: %+v", response)
	}
	framing := response.DocumentOutcome.Framing
	if framing.Disposition != runtimecontract.RuntimeProviderDocumentEmpty ||
		framing.Code != "provider_final_output_missing" || framing.Signature != "provider-final-output-missing" ||
		framing.ResponseHash != "" || framing.ResponseBytes != 0 || response.DocumentOutcome.Content != nil {
		t.Fatalf("reasoning-only completed document outcome = %+v", response.DocumentOutcome)
	}
	if response.Audit == nil || !response.Audit.ProviderSubmitted ||
		response.Audit.Status != llmmodels.RuntimeCandidateStatusCompleted || response.Audit.FailureClass != "" ||
		response.Audit.PhysicalResponseMode != llmaudit.PhysicalResponseModeStreaming || response.Audit.RequestBytes == 0 ||
		response.Audit.ResponseBytes == 0 || response.Audit.ContextBudget.ActualTokens != 28 {
		t.Fatalf("reasoning-only completed request audit = %+v", response.Audit)
	}
	if response.Usage == nil || response.Usage.InputTokens != 20 || response.Usage.CacheReadTokens != 2 ||
		response.Usage.OutputTokens != 8 || response.Usage.TotalTokens != 28 {
		t.Fatalf("reasoning-only completed usage = %+v", response.Usage)
	}
	if responseCalls.Load() != 1 {
		t.Fatalf("physical response calls = %d, want 1", responseCalls.Load())
	}

	select {
	case audit := <-audits:
		if !audit.ProviderSubmitted || audit.Status != llmmodels.RuntimeCandidateStatusCompleted || audit.FailureClass != "" ||
			audit.PhysicalResponseMode != llmaudit.PhysicalResponseModeStreaming || audit.InputTokens != 20 ||
			audit.CacheUsage.CacheReadTokens != 2 || audit.OutputTokens != 8 || audit.TotalTokens != 28 || audit.ContextBudget.ActualTokens != 28 {
			t.Fatalf("published reasoning-only completed usage audit = %+v", audit)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reasoning-only completed candidate did not publish usage audit")
	}
	select {
	case audit := <-audits:
		t.Fatalf("reasoning-only completed candidate published duplicate usage audit: %+v", audit)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestGenericProviderCompletionStreamingCompletedEmptyReturnsErrorAndPublishesSingleFailedAudit(t *testing.T) {
	ctx := context.Background()
	var responseCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": "list",
				"data": []map[string]any{{
					"id": "chat-model", "supported_endpoints": []string{"/v1/responses"},
					"contextWindowTokens": 8192, "maxOutputTokens": 1024,
				}},
			})
		case "/v1/responses":
			responseCalls.Add(1)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"reasoning is not final output\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":20,\"output_tokens\":8,\"total_tokens\":28}}}\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configured := workorchconfig.DefaultLLMConfig()
	configured.AIProxy.BaseURL = server.URL
	configured.AIProxy.APIKey = "test-key"
	configured.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "chat-model"}

	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil, Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) {
		return configured, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	auditObserver := magicevent.NewSimpleObserver("/llm/consumers/generic-completed-empty-test", hub)
	audits := make(chan llmevents.LLMUsageRecordedData, 2)
	auditObserver.Subscribe(llmevents.TopicLLMUsageRecorded, func(event magicevent.Event, _ magicevent.Result) {
		if data, ok := event.Data().(llmevents.LLMUsageRecordedData); ok {
			audits <- data
		}
	})
	t.Cleanup(func() { auditObserver.Unsubscribe(llmevents.TopicLLMUsageRecorded) })

	request := llmProviderRequest()
	request.WorkspaceID = ""
	request.RunID = ""
	request.StepID = ""
	request.Kind = "provider_completion"
	request.Model = "chat-model"
	magicResult := hub.Send(magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandCompleteProvider, llmSetupDraftAppserviceSource, llmevents.CommandDestination,
		magicevent.NewHeader(), ctx, llmevents.CompleteProviderCommand{Request: llmevents.ProviderRequestFromModel(request)},
	))
	if magicResult == nil {
		t.Fatal("generic provider completion returned no result")
	}
	data, completionErr := magicResult.Get()
	if completionErr == nil {
		t.Fatalf("generic completed-empty result = %#v, want provider protocol error", data)
	}
	if data != nil {
		t.Fatalf("generic completed-empty returned completion data on error: %#v", data)
	}
	var protocolErr *ProviderProtocolError
	if !errors.As(completionErr, &protocolErr) || protocolErr == nil || protocolErr.Code != providerFinalOutputMissingCode {
		t.Fatalf("generic completed-empty error = %v, want %s provider protocol error", completionErr, providerFinalOutputMissingCode)
	}
	if responseCalls.Load() != 1 {
		t.Fatalf("physical response calls = %d, want 1", responseCalls.Load())
	}

	select {
	case audit := <-audits:
		if err := audit.Validate(); err != nil {
			t.Fatalf("generic completed-empty usage audit validation: %v", err)
		}
		if !audit.ProviderSubmitted || audit.Status != llmmodels.RuntimeCandidateStatusFailed ||
			audit.FailureClass != string(runtimecontract.RuntimeCandidateFailureClassProviderProtocol) ||
			audit.PhysicalResponseMode != llmaudit.PhysicalResponseModeStreaming || audit.Kind != "provider_completion" ||
			audit.RequestBytes == 0 || audit.ResponseBytes == 0 {
			t.Fatalf("generic completed-empty usage audit = %+v", audit)
		}
		if audit.InputTokens != 20 || audit.OutputTokens != 8 || audit.TotalTokens != 28 ||
			audit.Estimated || audit.UsageSource != modulecommon.UsageSourceProviderNative ||
			audit.ContextBudget.ActualTokens != 28 {
			t.Fatalf("generic completed-empty native usage = %+v", audit)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("generic completed-empty completion did not publish usage audit")
	}
	select {
	case audit := <-audits:
		t.Fatalf("generic completed-empty completion published duplicate usage audit: %+v", audit)
	case <-time.After(50 * time.Millisecond):
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
		llmevents.TopicLLMCommandGenerateEmbedding, llmApplicationOwnerSource, llmevents.CommandDestination,
		magicevent.NewHeader(), ctx, llmevents.GenerateEmbeddingCommand{WorkspaceID: "workspace-1", Target: embeddingTarget, Input: "workspace goal"},
	))
	if result == nil || result.Error() != nil {
		t.Fatalf("embedding result = %+v", result)
	}
	resultData, resultErr := result.Get()
	embeddingResult, ok := resultData.(llmevents.GenerateEmbeddingResult)
	if resultErr != nil || !ok || !embeddingResult.Target.Same(embeddingTarget) || len(embeddingResult.Embedding.Vector) != 3 {
		t.Fatalf("embedding concrete result = %#v err=%v", resultData, resultErr)
	}
	select {
	case audit := <-audits:
		if audit.Operation != workorchconfig.LLMOperationEmbeddings || audit.WorkspaceID != "workspace-1" || audit.RunID != "" || !audit.ProviderSubmitted || audit.RequestBytes == 0 || audit.ResponseBytes == 0 || audit.InputTokens != 7 || audit.TotalTokens != 7 || audit.ContextBudget.ActualTokens != 7 || audit.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming {
			t.Fatalf("embedding audit = %+v", audit)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("embedding call did not publish usage audit")
	}

	completionTarget := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "chat-model"}
	completionRequest := llmProviderRequest()
	completionRequest.Kind = "provider_completion"
	completionRequest.Model = completionTarget.ModelID
	result = hub.Send(magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandCompleteProvider, llmSetupDraftAppserviceSource, llmevents.CommandDestination,
		magicevent.NewHeader(), ctx, llmevents.CompleteProviderCommand{Target: completionTarget, Request: llmevents.ProviderRequestFromModel(completionRequest)},
	))
	if result == nil {
		t.Fatal("completion returned no result")
	}
	resultData, resultErr = result.Get()
	completionResult, ok := resultData.(llmevents.CompleteProviderResult)
	if resultErr != nil || !ok || !completionResult.Target.Same(completionTarget) || completionResult.Completion.Content != "ok" {
		t.Fatalf("completion concrete result = %#v err=%v", resultData, resultErr)
	}
	select {
	case audit := <-audits:
		if audit.Operation != workorchconfig.LLMOperationResponses || audit.Kind != "provider_completion" ||
			!audit.ProviderSubmitted || audit.PhysicalResponseMode != llmaudit.PhysicalResponseModeStreaming {
			t.Fatalf("completion audit = %+v", audit)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("completion call did not publish usage audit")
	}

	hub.Post(magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandGenerateEmbedding, llmApplicationOwnerSource, llmevents.CommandDestination,
		magicevent.NewHeader(), ctx, llmevents.GenerateEmbeddingCommand{WorkspaceID: "workspace-1", Target: embeddingTarget, Input: "must not execute"},
	))
	hub.Post(magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandCompleteProvider, llmSetupDraftAppserviceSource, llmevents.CommandDestination,
		magicevent.NewHeader(), ctx, llmevents.CompleteProviderCommand{Request: llmevents.ProviderRequestFromModel(llmmodels.ProviderRequest{Kind: "post-must-not-execute"})},
	))
	hub.Post(magicevent.NewEventWithContext(
		llmevents.TopicRequestRuntimeCandidate, "kernel/orchestrator", llmevents.CommandDestination,
		magicevent.NewHeader(), ctx, llmevents.RequestRuntimeCandidateCommand{},
	))
	hub.Post(magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandRefreshProviderModelCatalog, llmApplicationOwnerSource, llmevents.CommandDestination,
		magicevent.NewHeader(), ctx, llmevents.RefreshProviderModelCatalogCommand{},
	))
	time.Sleep(100 * time.Millisecond)
	if embeddingCalls.Load() != 1 || responseCalls.Load() != 1 || candidateProvider.calls.Load() != 0 || catalogCalls.Load() != 1 {
		t.Fatalf("Post physical calls: embedding=%d completion=%d candidate=%d catalog=%d", embeddingCalls.Load(), responseCalls.Load(), candidateProvider.calls.Load(), catalogCalls.Load())
	}

	owner.Teardown(ctx)
	for _, event := range []magicevent.Event{
		magicevent.NewEventWithContext(
			llmevents.TopicLLMCommandGenerateEmbedding, llmApplicationOwnerSource, llmevents.CommandDestination,
			magicevent.NewHeader(), ctx, llmevents.GenerateEmbeddingCommand{WorkspaceID: "workspace-1", Target: embeddingTarget, Input: "must not execute after stop"},
		),
		magicevent.NewEventWithContext(
			llmevents.TopicLLMCommandCompleteProvider, llmSetupDraftAppserviceSource, llmevents.CommandDestination,
			magicevent.NewHeader(), ctx, llmevents.CompleteProviderCommand{Target: completionTarget, Request: llmevents.ProviderRequestFromModel(completionRequest)},
		),
		magicevent.NewEventWithContext(
			llmevents.TopicLLMCommandRefreshProviderModelCatalog, llmApplicationOwnerSource, llmevents.CommandDestination,
			magicevent.NewHeader(), ctx, llmevents.RefreshProviderModelCatalogCommand{},
		),
	} {
		hub.Send(event)
	}
	if embeddingCalls.Load() != 1 || responseCalls.Load() != 1 || catalogCalls.Load() != 1 {
		t.Fatalf("post-Stop physical calls: embedding=%d completion=%d catalog=%d", embeddingCalls.Load(), responseCalls.Load(), catalogCalls.Load())
	}
}
