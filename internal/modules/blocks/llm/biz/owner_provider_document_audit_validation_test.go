package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 此处仅注入审计 payload 校验失败；Hub.Post 无 ack/error，不能据此声称验证了投递失败。
// 已知 provider 文档必须原样返回，不能被审计校验失败覆盖。
func TestCompleteProviderDocumentAuditValidationFailurePreservesKnownOutcome(t *testing.T) {
	ctx := context.Background()
	const document = "  {\"document\":true}\r\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []any{map[string]any{"id": "chat-model", "supported_endpoints": []string{"/v1/responses"}, "contextWindowTokens": 8192, "maxOutputTokens": 1024}}})
		case "/v1/responses":
			w.Header().Set("Content-Type", "text/event-stream")
			encoded, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output_text": document}})
			_, _ = w.Write(append(append([]byte("data: "), encoded...), '\n', '\n'))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configured := workorchconfig.DefaultLLMConfig()
	configured.AIProxy.BaseURL, configured.AIProxy.APIKey = server.URL, "synthetic-key"
	configured.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "chat-model"}
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	owner, err := New(hub, nil, Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return configured, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Teardown(ctx)

	command := llmevents.CompleteProviderDocumentCommand{
		RequestIdentity: "permit-audit-validation", Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "chat-model"},
		ExpectedConfigurationRevision: owner.activeLLMConfigurationRevision,
		Request: llmevents.ProviderRequest{
			// provider 通用请求允许空 Workspace；审计对非空 Run 要求 Workspace scope。
			WorkspaceID: "", RunID: "run-audit-validation", Model: "chat-model", Kind: "authoring-document",
			ContextBudget: &llmmodels.ProviderContextBudget{},
			PromptContext: &llmmodels.PromptContext{Sections: []llmmodels.PromptSection{{ID: "C0", Content: "stable contract"}}},
		},
	}
	if err := command.Validate(); err != nil {
		t.Fatalf("synthetic request must reach provider owner: %v", err)
	}
	result := hub.Send(magicevent.NewEventWithContext(llmevents.TopicLLMCommandCompleteProviderDocument, llmApplicationOwnerSource, llmevents.CommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil {
		t.Fatal("missing provider-document result")
	}
	value, resultErr := result.Get()
	if resultErr != nil {
		t.Fatalf("audit validation failure replaced provider outcome: %v", resultErr)
	}
	response, ok := value.(llmevents.CompleteProviderDocumentResult)
	if !ok || response.ValidateFor(command) != nil {
		t.Fatalf("invalid typed result: %T", value)
	}
	if response.AuditPublicationFailureClass != "audit_publication_failed" {
		t.Fatalf("audit validation failure class = %q", response.AuditPublicationFailureClass)
	}
	if response.Document == nil || !response.Document.Content.Complete || string(response.Document.Content.Content) != document || !response.ProviderSubmitted {
		t.Fatalf("audit validation failure covered or changed the known provider outcome: document=%+v failure=%+v incomplete=%+v submitted=%v", response.Document, response.Failure, response.Incomplete, response.ProviderSubmitted)
	}
}
